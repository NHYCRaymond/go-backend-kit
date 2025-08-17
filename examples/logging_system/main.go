package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/database"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/NHYCRaymond/go-backend-kit/logging/adapter"
	"github.com/NHYCRaymond/go-backend-kit/logging/filters"
	"github.com/NHYCRaymond/go-backend-kit/logging/store"
)

// Example of modular logging system usage
func main() {
	ctx := context.Background()

	// 1. Setup Redis connection
	redisClient, err := database.NewRedis(config.RedisConfig{
		Address:  "localhost:6379",
		Password: "",
		DB:       0,
	})
	if err != nil {
		panic(err)
	}

	// 2. Create log store (Redis implementation)
	logStore := store.NewRedisStore(store.RedisConfig{
		Client:     redisClient,
		StreamKey:  "app:logs:stream",
		PubSubKey:  "app:logs:pubsub",
		MaxLen:     1000, // Only keep last 1000 logs
		BufferSize: 100,
	})

	// 3. Create processing pipeline
	pipeline := logging.NewPipeline()

	// Add filters
	pipeline.
		AddFilter(filters.NewLevelFilter(logging.LevelInfo)). // Filter out DEBUG
		AddFilter(filters.NewRateLimitFilter(10)).            // Max 10 logs/sec per source
		AddWriter(logStore)                                   // Write to Redis

	// 4. Create logger factory
	factory := adapter.NewLoggerFactory(pipeline)

	// 5. Use in different components
	demonstrateNodeUsage(ctx, factory)
	demonstrateHubUsage(ctx, factory)
	demonstrateTUIUsage(ctx, logStore)

	// Keep running
	select {}
}

// demonstrateNodeUsage shows how a crawler node would use the logging
func demonstrateNodeUsage(ctx context.Context, factory *adapter.LoggerFactory) {
	// Get a logger for this component
	logger := factory.GetLogger("node-001")

	// Use standard slog interface
	logger.Info("Node starting up",
		"version", "1.0.0",
		"workers", 10)

	// Simulate crawler operations
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		taskID := 1
		for {
			select {
			case <-ticker.C:
				// Log different events
				logger.Info("Processing task",
					slog.Int("task_id", taskID),
					slog.String("url", fmt.Sprintf("https://example.com/page%d", taskID)))

				if taskID%5 == 0 {
					logger.Warn("Rate limit detected",
						slog.Int("task_id", taskID),
						slog.Int("retry_after", 60))
				}

				if taskID%10 == 0 {
					logger.Error("Failed to fetch page",
						slog.Int("task_id", taskID),
						slog.String("error", "connection timeout"))
				}

				taskID++
			case <-ctx.Done():
				return
			}
		}
	}()
}

// demonstrateHubUsage shows how the hub would use logging
func demonstrateHubUsage(ctx context.Context, factory *adapter.LoggerFactory) {
	logger := factory.GetLogger("hub-main")

	logger.Info("Hub coordinator started",
		"grpc_port", 50051,
		"redis_addr", "localhost:6379")

	// Simulate hub operations
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				logger.Info("Cluster status",
					slog.Int("active_nodes", 3),
					slog.Int("pending_tasks", 42),
					slog.Int("completed_tasks", 128))
			case <-ctx.Done():
				return
			}
		}
	}()
}

// demonstrateTUIUsage shows how the TUI would consume logs
func demonstrateTUIUsage(ctx context.Context, logStore logging.Store) {
	// Subscribe to real-time logs
	logStore.Subscribe(ctx, func(entry *logging.LogEntry) {
		// This would update the TUI display
		fmt.Printf("[%s] %s: %s\n",
			entry.Level.String(),
			entry.Source,
			entry.Message)
	})

	// Query historical logs
	go func() {
		time.Sleep(10 * time.Second)

		// Read last 100 ERROR logs
		logs, err := logStore.Read(ctx, logging.ReadOptions{
			Limit: 100,
			Level: logging.LevelError,
		})
		if err != nil {
			return
		}

		fmt.Printf("\n=== Last %d ERROR logs ===\n", len(logs))
		for _, log := range logs {
			fmt.Printf("%s [%s] %s: %s\n",
				log.Timestamp.Format("15:04:05"),
				log.Level.String(),
				log.Source,
				log.Message)
		}
	}()
}