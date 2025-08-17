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
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	configFile  = flag.String("config", "config.yaml", "Configuration file path")
	workerID    = flag.String("id", "", "Worker ID (auto-generated if empty)")
	maxWorkers  = flag.Int("workers", 5, "Maximum concurrent workers")
	queuePrefix = flag.String("queue", "crawler", "Redis queue prefix")
)

// Worker represents a task worker
type Worker struct {
	id          string
	redis       *redis.Client
	mongodb     *mongo.Database
	executor    *executor.Executor
	logger      *slog.Logger
	queuePrefix string
	stopChan    chan struct{}
}

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	if err := logging.InitLogger(cfg.Logger); err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	logger := logging.GetLogger()

	// Initialize Redis
	redisClient, err := database.NewRedis(cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Initialize MongoDB
	ctx := context.Background()
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.Database.MongoDB.URI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(ctx)
	
	mongodb := mongoClient.Database(cfg.Database.MongoDB.Database)

	// Initialize MySQL (for storage)
	mysql, err := database.NewMySQL(cfg.Database.MySQL)
	if err != nil {
		logger.Warn("MySQL not available, some storage options will be disabled", "error", err)
	}

	// Create factory
	factoryInstance := factory.NewFactory(mongodb, mysql, redisClient)

	// Create executor
	exec := executor.NewExecutor(&executor.Config{
		Fetcher:   fetcher.NewHTTPFetcher(),
		Extractor: extractor.NewCSSExtractor(),
		Factory:   factoryInstance,
		Logger:    logger,
	})

	// Generate worker ID if not provided
	if *workerID == "" {
		*workerID = fmt.Sprintf("worker_%d", time.Now().Unix())
	}

	// Create worker
	worker := &Worker{
		id:          *workerID,
		redis:       redisClient,
		mongodb:     mongodb,
		executor:    exec,
		logger:      logger,
		queuePrefix: *queuePrefix,
		stopChan:    make(chan struct{}),
	}

	// Start worker
	logger.Info("Starting worker",
		"worker_id", worker.id,
		"max_workers", *maxWorkers,
		"queue", *queuePrefix)

	// Start worker goroutines
	for i := 0; i < *maxWorkers; i++ {
		go worker.run(ctx, i)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down worker...")
	close(worker.stopChan)
	
	// Give workers time to finish current tasks
	time.Sleep(5 * time.Second)
	
	logger.Info("Worker stopped")
}

// run starts the worker loop
func (w *Worker) run(ctx context.Context, workerNum int) {
	workerID := fmt.Sprintf("%s_%d", w.id, workerNum)
	w.logger.Info("Worker started", "worker_id", workerID)

	for {
		select {
		case <-w.stopChan:
			w.logger.Info("Worker stopping", "worker_id", workerID)
			return
		default:
			// Try to get a task from queue
			if err := w.processNextTask(ctx, workerID); err != nil {
				if err != redis.Nil {
					w.logger.Error("Failed to process task",
						"worker_id", workerID,
						"error", err)
				}
				// Wait before retry
				time.Sleep(1 * time.Second)
			}
		}
	}
}

// processNextTask gets and processes the next task from queue
func (w *Worker) processNextTask(ctx context.Context, workerID string) error {
	// Pop task from queue (blocking with timeout)
	queueKey := fmt.Sprintf("%s:queue:pending", w.queuePrefix)
	result, err := w.redis.BRPop(ctx, 5*time.Second, queueKey).Result()
	if err != nil {
		return err
	}

	if len(result) < 2 {
		return fmt.Errorf("invalid queue result")
	}

	// Deserialize task
	var t task.Task
	if err := json.Unmarshal([]byte(result[1]), &t); err != nil {
		w.logger.Error("Failed to unmarshal task", "error", err)
		return err
	}

	w.logger.Info("Processing task",
		"worker_id", workerID,
		"task_id", t.ID,
		"url", t.URL)

	// Mark task as running
	runningKey := fmt.Sprintf("%s:queue:running", w.queuePrefix)
	taskData, _ := json.Marshal(t)
	w.redis.HSet(ctx, runningKey, t.ID, taskData)

	// Execute task
	startTime := time.Now()
	taskResult, err := w.executor.Execute(ctx, &t)
	duration := time.Since(startTime)

	// Remove from running queue
	w.redis.HDel(ctx, runningKey, t.ID)

	if err != nil {
		w.logger.Error("Task execution failed",
			"worker_id", workerID,
			"task_id", t.ID,
			"error", err,
			"duration", duration)

		// Handle retry
		if t.RetryCount < t.MaxRetries {
			t.RetryCount++
			t.LastError = err.Error()
			t.Status = task.StatusRetrying
			
			// Re-queue with delay
			go func() {
				time.Sleep(t.RetryDelay)
				retryData, _ := json.Marshal(t)
				w.redis.LPush(context.Background(), queueKey, retryData)
				w.logger.Info("Task re-queued for retry",
					"task_id", t.ID,
					"retry_count", t.RetryCount)
			}()
		} else {
			// Max retries reached, mark as failed
			w.recordTaskResult(ctx, &t, taskResult, false)
		}
	} else {
		w.logger.Info("Task completed successfully",
			"worker_id", workerID,
			"task_id", t.ID,
			"duration", duration,
			"items_extracted", taskResult.ItemsExtracted)

		// Record success
		w.recordTaskResult(ctx, &t, taskResult, true)

		// Generate new tasks from extracted links (if any)
		if len(taskResult.Links) > 0 && (t.Type == "seed" || t.Type == "list") {
			w.generateSubTasks(ctx, &t, taskResult.Links)
		}
	}

	return nil
}

// recordTaskResult records task execution result in MongoDB
func (w *Worker) recordTaskResult(ctx context.Context, t *task.Task, result *task.TaskResult, success bool) {
	collection := w.mongodb.Collection("task_results")
	
	doc := map[string]interface{}{
		"task_id":     t.ID,
		"parent_id":   t.ParentID,
		"url":         t.URL,
		"success":     success,
		"status":      result.Status,
		"error":       result.Error,
		"data":        result.Data,
		"items_count": result.ItemsExtracted,
		"bytes":       result.BytesFetched,
		"duration_ms": result.Duration,
		"timestamp":   time.Now(),
	}
	
	if _, err := collection.InsertOne(ctx, doc); err != nil {
		w.logger.Error("Failed to record task result", "error", err)
	}
}

// generateSubTasks generates sub-tasks from extracted links
func (w *Worker) generateSubTasks(ctx context.Context, parentTask *task.Task, links []string) {
	queueKey := fmt.Sprintf("%s:queue:pending", w.queuePrefix)
	
	for _, link := range links {
		// Create sub-task
		subTask := &task.Task{
			ID:         fmt.Sprintf("%s_sub_%d", parentTask.ID, time.Now().UnixNano()),
			ParentID:   parentTask.ID,
			Name:       fmt.Sprintf("Sub-task of %s", parentTask.Name),
			URL:        link,
			Type:       "detail", // Sub-tasks are usually detail pages
			Priority:   parentTask.Priority - 1, // Lower priority than parent
			Method:     "GET",
			MaxRetries: parentTask.MaxRetries,
			RetryDelay: parentTask.RetryDelay,
			Timeout:    parentTask.Timeout,
			CreatedAt:  time.Now(),
			Status:     task.StatusPending,
			
			// Inherit storage configuration
			StorageConf: parentTask.StorageConf,
			PipelineID:  parentTask.PipelineID,
		}
		
		// Serialize and queue
		if data, err := json.Marshal(subTask); err == nil {
			if err := w.redis.LPush(ctx, queueKey, data).Err(); err != nil {
				w.logger.Error("Failed to queue sub-task",
					"url", link,
					"error", err)
			}
		}
	}
	
	w.logger.Info("Generated sub-tasks",
		"parent_task", parentTask.ID,
		"count", len(links))
}