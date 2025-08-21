package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/crawler/communication"
	"github.com/NHYCRaymond/go-backend-kit/crawler/distributed"
	"github.com/NHYCRaymond/go-backend-kit/crawler/scheduler"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/NHYCRaymond/go-backend-kit/database"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	configFile = flag.String("config", "config/hub.yaml", "Configuration file path")
	grpcAddr   = flag.String("grpc", "", "Override gRPC address from config")
	redisAddr  = flag.String("redis", "", "Override Redis address from config")
	mongoURI   = flag.String("mongo", "", "Override MongoDB URI from config")
	logLevel   = flag.String("log", "", "Override log level from config")
)

func main() {
	flag.Parse()

	// Load configuration using Viper
	viper.SetConfigFile(*configFile)
	viper.SetConfigType("yaml")
	
	// Set defaults
	viper.SetDefault("grpc.address", ":50051")
	viper.SetDefault("redis.address", "localhost:6379")
	viper.SetDefault("database.mongo.uri", "mongodb://localhost:27017")
	viper.SetDefault("database.mongo.database", "crawler")
	viper.SetDefault("logger.level", "info")
	viper.SetDefault("logger.format", "json")
	viper.SetDefault("logger.output", "stdout")
	viper.SetDefault("scheduler.enabled", true)
	viper.SetDefault("scheduler.queue_prefix", "crawler")
	viper.SetDefault("coordinator.redis_prefix", "crawler")
	viper.SetDefault("coordinator.max_retries", 3)
	viper.SetDefault("coordinator.task_timeout", "5m")
	viper.SetDefault("coordinator.reassign_interval", "30s")
	viper.SetDefault("coordinator.batch_size", 10)
	viper.SetDefault("dispatcher.redis_prefix", "crawler")
	viper.SetDefault("cluster.heartbeat_interval", "10s")
	viper.SetDefault("cluster.heartbeat_timeout", "30s")
	
	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: Failed to read config file %s, using defaults: %v", *configFile, err)
	}

	// Apply command line overrides
	grpcAddrValue := viper.GetString("grpc.address")
	if *grpcAddr != "" {
		grpcAddrValue = *grpcAddr
	}
	
	redisAddrValue := viper.GetString("redis.address")
	if *redisAddr != "" {
		redisAddrValue = *redisAddr
	}
	
	mongoURIValue := viper.GetString("database.mongo.uri")
	if *mongoURI != "" {
		mongoURIValue = *mongoURI
	}
	
	logLevelValue := viper.GetString("logger.level")
	if *logLevel != "" {
		logLevelValue = *logLevel
	}

	// Initialize logging
	logConfig := config.LoggerConfig{
		Level:     logLevelValue,
		Format:    viper.GetString("logger.format"),
		Output:    viper.GetString("logger.output"),
		AddSource: viper.GetBool("logger.add_source"),
	}
	if err := logging.InitLogger(logConfig); err != nil {
		log.Fatal("Failed to init logger:", err)
	}

	logger := logging.GetLogger()
	logger.Info("Starting Crawler Hub",
		"config_file", *configFile,
		"grpc_addr", grpcAddrValue,
		"redis_addr", redisAddrValue,
		"mongo_uri", mongoURIValue,
		"scheduler_enabled", viper.GetBool("scheduler.enabled"))

	// Create Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddrValue,
		Password: viper.GetString("redis.password"),
		DB:       viper.GetInt("redis.db"),
	})

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	logger.Info("Connected to Redis", "addr", redisAddrValue)

	// Create MongoDB connection
	var mongoDatabase *mongo.Database
	if viper.GetBool("scheduler.enabled") && mongoURIValue != "" {
		mongoConfig := config.MongoConfig{
			URI:      mongoURIValue,
			Database: viper.GetString("database.mongo.database"),
		}
		mongoDB := database.NewMongo(mongoConfig)
		if err := mongoDB.Connect(ctx); err != nil {
			logger.Error("Failed to connect to MongoDB, scheduler will be disabled", "error", err)
		} else {
			mongoDatabase = mongoDB.GetDatabase(mongoConfig.Database)
			logger.Info("Connected to MongoDB", "database", mongoConfig.Database)
		}
	}

	// Create Task Scheduler if MongoDB is available
	var taskScheduler *scheduler.Scheduler
	if mongoDatabase != nil {
		schedulerConfig := &scheduler.Config{
			MongoDB:     mongoDatabase,
			Redis:       redisClient,
			QueuePrefix: viper.GetString("scheduler.queue_prefix"),
			Logger:      logger,
		}
		taskScheduler = scheduler.NewScheduler(schedulerConfig)
		
		// Start scheduler
		if err := taskScheduler.Start(ctx); err != nil {
			logger.Error("Failed to start task scheduler", "error", err)
		} else {
			logger.Info("Task scheduler started successfully")
		}
	} else {
		logger.Warn("Task scheduler disabled - MongoDB not configured")
	}

	// Create Registry
	registry := distributed.NewRegistry(redisClient, "crawler")

	// Create ClusterManager
	clusterManager := distributed.NewClusterManager(registry, logger)

	// Start ClusterManager
	if err := clusterManager.Start(ctx); err != nil {
		log.Fatal("Failed to start cluster manager:", err)
	}

	// Create Dispatcher
	dispatcherConfig := &task.DispatcherConfig{
		QueueType:   task.QueueTypeRedis,
		Strategy:    task.StrategyLeastLoad,
		BatchSize:   10,
		MaxRetries:  3,
		Timeout:     30 * time.Second,
		RedisClient: redisClient,
		RedisPrefix: "crawler",
		Logger:      logger,
	}
	dispatcher := task.NewDispatcher(dispatcherConfig)

	// Start Dispatcher
	if err := dispatcher.Start(ctx); err != nil {
		log.Fatal("Failed to start dispatcher:", err)
	}

	// Create Communication Hub
	hubConfig := &communication.HubConfig{
		GRPCAddr:          grpcAddrValue,
		RedisAddr:         redisAddrValue,
		RedisPrefix:       viper.GetString("coordinator.redis_prefix"),
		HeartbeatInterval: viper.GetDuration("cluster.heartbeat_interval"),
		HeartbeatTimeout:  viper.GetDuration("cluster.heartbeat_timeout"),
	}
	hub, err := communication.NewCommunicationHub(hubConfig, logger)
	if err != nil {
		log.Fatal("Failed to create communication hub:", err)
	}

	// Create Coordinator
	coordinatorConfig := &distributed.CoordinatorConfig{
		RedisPrefix:      viper.GetString("coordinator.redis_prefix"),
		MaxRetries:       viper.GetInt("coordinator.max_retries"),
		TaskTimeout:      viper.GetDuration("coordinator.task_timeout"),
		ReassignInterval: viper.GetDuration("coordinator.reassign_interval"),
		BatchSize:        viper.GetInt("coordinator.batch_size"),
	}
	coordinator := distributed.NewCoordinator(
		coordinatorConfig,
		clusterManager,
		dispatcher,
		hub,
		redisClient,
		logger,
	)
	
	// Set scheduler if available
	if taskScheduler != nil {
		coordinator.SetScheduler(taskScheduler)
	}

	// Start Coordinator
	if err := coordinator.Start(); err != nil {
		log.Fatal("Failed to start coordinator:", err)
	}

	// Start Communication Hub (starts gRPC server)
	if err := hub.Start(ctx); err != nil {
		log.Fatal("Failed to start communication hub:", err)
	}

	logger.Info("Crawler Hub started successfully",
		"cluster_id", clusterManager.ID,
		"grpc_addr", *grpcAddr)

	// Create Task Registry and Manager
	taskRegistry := task.NewTaskRegistry(redisClient, "crawler", logger)
	taskManager := task.NewTaskManager(&task.ManagerConfig{
		Redis:  redisClient,
		Prefix: "crawler",
		Logger: logger,
	})

	// Load task configurations
	configLoader := task.NewConfigLoader(&task.LoaderConfig{
		Registry:    taskRegistry,
		Manager:     taskManager,
		Redis:       redisClient,
		Prefix:      "crawler",
		Logger:      logger,
		ConfigPaths: []string{"tasks.yaml", "tasks.json", "tasks.lua"},
		WatchFiles:  true,
	})

	// Load all configurations
	if err := configLoader.LoadAll(ctx); err != nil {
		logger.Error("Failed to load task configurations", "error", err)
	}

	// Monitor cluster status
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			nodes := clusterManager.GetNodes()
			status := clusterManager.GetStatus()
			leader := clusterManager.GetLeader()

			logger.Info("Cluster status",
				"status", status,
				"nodes", len(nodes),
				"leader", leader)

			// Log dispatcher stats
			dispatcherStats := dispatcher.GetStats()
			logger.Info("Dispatcher stats",
				"dispatched", dispatcherStats["dispatched"],
				"failed", dispatcherStats["failed"])

			// Log coordinator stats
			coordStats := coordinator.GetStats()
			logger.Info("Coordinator stats",
				"pending_tasks", coordStats["pending_tasks"],
				"assignments", coordStats["task_assignments"])
		}
	}()

	// Handle shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down Crawler Hub...")

	// Stop components in reverse order
	if err := hub.Stop(ctx); err != nil {
		logger.Error("Failed to stop hub", "error", err)
	}

	if err := coordinator.Stop(); err != nil {
		logger.Error("Failed to stop coordinator", "error", err)
	}

	if err := dispatcher.Stop(); err != nil {
		logger.Error("Failed to stop dispatcher", "error", err)
	}

	if err := clusterManager.Stop(ctx); err != nil {
		logger.Error("Failed to stop cluster manager", "error", err)
	}

	// Stop scheduler if running
	if taskScheduler != nil {
		taskScheduler.Stop()
		logger.Info("Task scheduler stopped")
	}

	// Close Redis connection
	if err := redisClient.Close(); err != nil {
		logger.Error("Failed to close Redis", "error", err)
	}

	logger.Info("Crawler Hub stopped")
}
