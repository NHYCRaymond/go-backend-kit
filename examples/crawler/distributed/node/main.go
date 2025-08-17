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
	"github.com/NHYCRaymond/go-backend-kit/crawler/distributed"
	"github.com/NHYCRaymond/go-backend-kit/database"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

var (
	configFile = flag.String("config", "examples/crawler/config/node.yaml", "Configuration file path")
	nodeID     = flag.String("id", "", "Override node ID from config")
	redisAddr  = flag.String("redis", "", "Override Redis address from config")
	mongoURI   = flag.String("mongo", "", "Override MongoDB URI from config")
	maxWorkers = flag.Int("workers", 0, "Override max workers from config")
)

func main() {
	flag.Parse()

	// Load configuration using Viper
	viper.SetConfigFile(*configFile)
	viper.SetConfigType("yaml")
	
	// Set defaults
	viper.SetDefault("node.max_workers", 10)
	viper.SetDefault("node.queue_size", 100)
	viper.SetDefault("redis.address", "localhost:6379")
	viper.SetDefault("crawler.redis_prefix", "crawler")
	viper.SetDefault("crawler.mode", "enhanced")
	viper.SetDefault("logger.level", "info")
	viper.SetDefault("logger.format", "json")
	viper.SetDefault("logger.output", "stdout")
	
	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Failed to read config file %s, using defaults: %v", *configFile, err)
	}

	// Initialize logger from config
	loggerConfig := config.LoggerConfig{
		Level:      viper.GetString("logger.level"),
		Format:     viper.GetString("logger.format"),
		Output:     viper.GetString("logger.output"),
		AddSource:  viper.GetBool("logger.add_source"),
	}
	if err := logging.InitLogger(loggerConfig); err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	logger := logging.GetLogger()

	// Create node configuration from config file
	// Command line flags override config file values
	nodeIDValue := viper.GetString("node.id")
	if *nodeID != "" {
		nodeIDValue = *nodeID
	}
	
	redisAddrValue := viper.GetString("redis.address")
	if *redisAddr != "" {
		redisAddrValue = *redisAddr
	}
	
	maxWorkersValue := viper.GetInt("node.max_workers")
	if *maxWorkers > 0 {
		maxWorkersValue = *maxWorkers
	}
	
	nodeConfig := &distributed.NodeConfig{
		ID:          nodeIDValue,
		MaxWorkers:  maxWorkersValue,
		QueueSize:   viper.GetInt("node.queue_size"),
		RedisAddr:   redisAddrValue,
		RedisPrefix: viper.GetString("crawler.redis_prefix"),
		LogLevel:    viper.GetString("logger.level"),
		Tags:        viper.GetStringSlice("node.tags"),
		Capabilities: viper.GetStringSlice("node.capabilities"),
		EnableGRPC:  viper.GetBool("crawler.enable_grpc"),
		HubAddr:     viper.GetString("crawler.hub_addr"),
	}
	
	// Set labels from config
	labels := make(map[string]string)
	if viper.IsSet("node.labels") {
		labelsMap := viper.GetStringMap("node.labels")
		for k, v := range labelsMap {
			if str, ok := v.(string); ok {
				labels[k] = str
			}
		}
	}
	nodeConfig.Labels = labels

	logger.Info("Starting node with configuration",
		"config_file", *configFile,
		"node_id", nodeConfig.ID,
		"workers", nodeConfig.MaxWorkers,
		"mode", viper.GetString("crawler.mode"))

	// Create node
	node, err := distributed.NewNode(nodeConfig)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}

	// Setup enhanced mode if configured
	ctx := context.Background()
	mode := viper.GetString("crawler.mode")
	logger.Info("Setting up node mode", "mode", mode)
	
	if mode == "enhanced" {
		// Get MongoDB URI (command line overrides config)
		mongoURIValue := viper.GetString("database.mongo.uri")
		if *mongoURI != "" {
			mongoURIValue = *mongoURI
		}
		
		// Connect to MongoDB for enhanced features
		mongoConfig := config.MongoConfig{
			URI:      mongoURIValue,
			Database: viper.GetString("database.mongo.database"),
		}
		
		logger.Info("Connecting to MongoDB", "uri", mongoURIValue, "database", mongoConfig.Database)
		mongoDB := database.NewMongo(mongoConfig)
		
		// Create a timeout context for MongoDB connection
		connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		
		if err := mongoDB.Connect(connectCtx); err != nil {
			logger.Warn("MongoDB not available, falling back to basic mode", "error", err)
			mode = "basic"
		} else {
			logger.Info("MongoDB connected successfully")
			mongoDatabase := mongoDB.GetDatabase(mongoConfig.Database)
			
			// Connect to MySQL if configured
			var mysqlDB *database.MySQLDatabase
			mysqlHost := viper.GetString("database.mysql.host")
			logger.Info("Checking MySQL configuration", "host", mysqlHost)
			
			if mysqlHost != "" {
				mysqlConfig := config.MySQLConfig{
					Host:     mysqlHost,
					Port:     viper.GetInt("database.mysql.port"),
					User:     viper.GetString("database.mysql.user"),
					Password: viper.GetString("database.mysql.password"),
					DBName:   viper.GetString("database.mysql.dbname"),
				}
				logger.Info("Connecting to MySQL", "host", mysqlConfig.Host, "port", mysqlConfig.Port)
				mysqlDB = database.NewMySQL(mysqlConfig)
				
				// Use timeout context for MySQL connection too
				mysqlCtx, mysqlCancel := context.WithTimeout(ctx, 10*time.Second)
				defer mysqlCancel()
				
				if err := mysqlDB.Connect(mysqlCtx); err != nil {
					logger.Warn("MySQL not available", "error", err)
					mysqlDB = nil // Ensure it's nil if connection failed
				} else {
					logger.Info("MySQL connected successfully")
				}
			}
			
			// Enhance node with executor and factory
			enhancedConfig := &distributed.EnhancedNodeConfig{
				NodeConfig: nodeConfig,
				MongoDB:    mongoDatabase,
			}
			
			// Add MySQL if available
			if mysqlDB != nil {
				if client := mysqlDB.GetClient(); client != nil {
					// The client is already a *gorm.DB
					if db, ok := client.(*gorm.DB); ok {
						enhancedConfig.MySQL = db
					}
				}
			}
			
			logger.Info("Calling EnhanceNode with config", "has_mongodb", enhancedConfig.MongoDB != nil, "has_mysql", enhancedConfig.MySQL != nil)
			if err := distributed.EnhanceNode(node, enhancedConfig); err != nil {
				logger.Error("Failed to enhance node, falling back to basic mode", "error", err)
				mode = "basic"
			} else {
				logger.Info("Node enhanced with executor and factory - ENHANCED MODE ACTIVE")
			}
		}
	}
	
	if mode == "basic" {
		logger.Warn("Running in BASIC MODE - tasks will not be actually executed")
	}

	// Start node
	if err := node.Start(ctx); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}

	logger.Info("Node started successfully",
		"node_id", node.ID,
		"workers", node.MaxWorkers,
		"mode", mode,
		"redis", nodeConfig.RedisAddr)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down node...")
	
	// Stop node
	if err := node.Stop(ctx); err != nil {
		logger.Error("Failed to stop node cleanly", "error", err)
	}
	
	logger.Info("Node stopped")
}