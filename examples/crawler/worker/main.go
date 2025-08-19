package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/crawler/executor"
	"github.com/NHYCRaymond/go-backend-kit/crawler/extractor"
	"github.com/NHYCRaymond/go-backend-kit/crawler/factory"
	"github.com/NHYCRaymond/go-backend-kit/crawler/fetcher"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/NHYCRaymond/go-backend-kit/database"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	configFile  = flag.String("config", "config.yaml", "Configuration file path")
	workerID    = flag.String("id", "", "Worker ID (auto-generated if empty)")
	enableGRPC  = flag.Bool("grpc", false, "Enable gRPC connection to hub")
	hubAddr     = flag.String("hub", "localhost:50051", "Hub gRPC address")
	workerCount = flag.Int("workers", 5, "Number of concurrent workers")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set worker ID
	if *workerID == "" {
		hostname, _ := os.Hostname()
		*workerID = fmt.Sprintf("worker-%s-%d", hostname, os.Getpid())
	}

	// Initialize logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		AddSource: true,
	}))

	// Initialize context
	ctx := context.Background()

	// Initialize Redis
	redisDB := database.NewRedis(cfg.Redis)
	if err := redisDB.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisDB.Disconnect(ctx)
	redisClient := redisDB.GetClient().(*redis.Client)

	// Initialize MongoDB
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.Database.Mongo.URI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(ctx)
	
	mongodb := mongoClient.Database(cfg.Database.Mongo.Database)

	// Initialize MySQL (for storage)
	mysqlDB := database.NewMySQL(cfg.Database.MySQL)
	if err := mysqlDB.Connect(ctx); err != nil {
		logger.Warn("MySQL not available, some storage options will be disabled", "error", err)
	}
	var mysql *gorm.DB
	if mysqlDB.GetClient() != nil {
		mysql = mysqlDB.GetClient().(*gorm.DB)
	}

	// Create factory
	factoryInstance := factory.NewFactory(mongodb, mysql, redisClient)

	// Create fetcher
	httpFetcher := fetcher.NewHTTPFetcher()

	// Create extractor wrapper
	extractorWrapper := &extractor.CSSExtractor{}

	// Create executor
	executorInstance := executor.NewExecutor(&executor.Config{
		Fetcher:   httpFetcher,
		Extractor: extractorWrapper,
		Factory:   factoryInstance,
		Logger:    logger,
	})

	// Create task queue
	queue := task.NewRedisQueue(redisClient, "crawler")

	// Print worker info
	logger.Info("Worker started",
		"worker_id", *workerID,
		"workers", *workerCount,
		"redis", cfg.Redis.Address,
		"mongodb", cfg.Database.Mongo.URI,
		"grpc_enabled", *enableGRPC,
		"hub_addr", *hubAddr)

	// Print supported features
	features := []string{
		"HTTP/HTTPS fetching",
		"HTML extraction (CSS selectors)",
		"MongoDB storage",
		"Redis queue",
	}
	if mysql != nil {
		features = append(features, "MySQL storage")
	}
	
	logger.Info("Supported features",
		"features", features)

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start worker loop
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Worker loop
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				// Poll for tasks
				t, err := queue.Pop(workerCtx)
				if err != nil {
					if err.Error() != "redis: nil" {
						logger.Error("Failed to pop task", "error", err)
					}
					continue
				}

				if t == nil {
					continue
				}

				logger.Info("Processing task",
					"task_id", t.ID,
					"url", t.URL,
					"type", t.Type)

				// Execute task
				result, err := executorInstance.Execute(workerCtx, t)
				if err != nil {
					logger.Error("Task execution failed",
						"task_id", t.ID,
						"error", err)
					continue
				}

				// Log result
				resultJSON, _ := json.MarshalIndent(result, "", "  ")
				logger.Info("Task completed",
					"task_id", t.ID,
					"status", result.Status,
					"duration_ms", result.Duration,
					"items_extracted", result.ItemsExtracted,
					slog.String("result", string(resultJSON)))
			}
		}
	}()

	// Wait for signal
	sig := <-sigChan
	logger.Info("Received signal, shutting down...", "signal", sig)

	// Cancel worker context
	cancel()

	// Give workers time to finish
	time.Sleep(2 * time.Second)

	logger.Info("Worker stopped")
}