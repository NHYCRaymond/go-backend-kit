package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/executor"
	"github.com/NHYCRaymond/go-backend-kit/crawler/extractor"
	"github.com/NHYCRaymond/go-backend-kit/crawler/factory"
	"github.com/NHYCRaymond/go-backend-kit/crawler/fetcher"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// EnhancedNodeConfig contains enhanced node configuration
type EnhancedNodeConfig struct {
	*NodeConfig
	MongoDB *mongo.Database
	MySQL   *gorm.DB
}

// EnhanceNode adds executor and factory to an existing node
func EnhanceNode(node *Node, config *EnhancedNodeConfig) error {
	// Create factory for pipeline and storage
	node.factory = factory.NewFactory(config.MongoDB, config.MySQL, node.redis)

	// Create multiple extractors for different content types
	node.extractors = map[string]task.Extractor{
		"json": extractor.NewJSONExtractor(nil),
		"css":  extractor.NewCSSExtractor(),
		"html": extractor.NewCSSExtractor(), // CSS extractor works for HTML
	}

	// Create executor with default fetcher
	// Extractor will be selected dynamically based on task type
	exec := executor.NewExecutor(&executor.Config{
		Fetcher:   fetcher.NewHTTPFetcher(),
		Extractor: nil, // Will be set dynamically per task
		Factory:   node.factory,
		Logger:    node.logger,
	})

	node.executor = exec

	node.logger.Info("Node enhanced with executor and factory", "node_id", node.ID)

	return nil
}

// executeTaskEnhanced is the enhanced version of executeTask using the new executor
func (w *Worker) executeTaskEnhanced(ctx context.Context, t *task.Task, result *TaskResult) error {
	if w.Node.executor == nil {
		// This should not happen if EnhanceNode was called properly
		return fmt.Errorf("executor not available for enhanced mode")
	}

	// Select appropriate extractor based on task type
	extractorType := "css" // default
	if t.Type == "api" || t.Type == "json" {
		extractorType = "json"
	} else if t.Type == "html" || t.Type == "web" {
		extractorType = "html"
	}

	// Set the appropriate extractor for this task
	if extractor, ok := w.Node.extractors[extractorType]; ok {
		w.Node.executor.SetExtractor(extractor)
		w.Node.logger.Info("Using extractor for task",
			"task_id", t.ID,
			"task_type", t.Type,
			"extractor_type", extractorType)
	} else {
		w.Node.logger.Warn("No extractor found for type, using default",
			"task_type", t.Type,
			"extractor_type", extractorType)
	}

	w.Node.logger.Info("Executing task with enhanced executor",
		"task_id", t.ID,
		"worker_id", w.ID,
		"url", t.URL)

	// Execute using the new executor
	execResult, err := w.Node.executor.Execute(ctx, t)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return err
	}

	// Convert executor result to TaskResult, preserving task metadata
	// Create a proper result structure with URL and metadata
	resultData := map[string]interface{}{
		"url":        t.URL,
		"method":     t.Method,
		"task_type":  t.Type,
		"task_name":  t.Name,
		"extracted":  execResult.Data,  // Store extracted data separately
	}
	
	result.Status = execResult.Status
	result.Data = []map[string]interface{}{resultData}
	result.BytesRead = execResult.BytesFetched
	
	// Check if the task actually failed
	if execResult.Status == "failed" {
		result.Status = "failed"
		result.Error = execResult.Error
		return fmt.Errorf("task execution failed: %s", execResult.Error)
	}

	// Update node metrics only for successful tasks
	atomic.AddInt64(&w.Node.BytesDownloaded, execResult.BytesFetched)
	atomic.AddInt64(&w.Node.ItemsExtracted, int64(execResult.ItemsExtracted))

	// Handle extracted links for further crawling
	if len(execResult.Links) > 0 && (t.Type == "seed" || t.Type == "list") {
		w.Node.generateSubTasks(ctx, t, execResult.Links)
	}

	return nil
}

// generateSubTasks generates sub-tasks from extracted links
func (n *Node) generateSubTasks(ctx context.Context, parentTask *task.Task, links []string) {
	for _, link := range links {
		// Create sub-task
		subTask := &task.Task{
			ID:         fmt.Sprintf("%s_sub_%d", parentTask.ID, time.Now().UnixNano()),
			ParentID:   parentTask.ID,
			Name:       fmt.Sprintf("Sub-task of %s", parentTask.Name),
			URL:        link,
			Type:       "detail",                // Sub-tasks are usually detail pages
			Priority:   parentTask.Priority - 1, // Lower priority
			Method:     "GET",
			MaxRetries: parentTask.MaxRetries,
			RetryDelay: parentTask.RetryDelay,
			Timeout:    parentTask.Timeout,
			CreatedAt:  time.Now(),
			Status:     task.StatusPending,

			// Inherit configuration
			StorageConf: parentTask.StorageConf,
			PipelineID:  parentTask.PipelineID,
		}

		// Queue the sub-task
		if err := n.queueTask(ctx, subTask); err != nil {
			n.logger.Error("Failed to queue sub-task",
				"url", link,
				"error", err)
		}
	}

	n.logger.Info("Generated sub-tasks",
		"parent_task", parentTask.ID,
		"count", len(links))
}

// queueTask adds a task to the Redis queue
func (n *Node) queueTask(ctx context.Context, t *task.Task) error {
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	queueKey := n.getTaskQueueKey()
	return n.redis.LPush(ctx, queueKey, data).Err()
}

// SetExecutor sets the executor for the node
func (n *Node) SetExecutor(exec task.Executor) {
	n.executor = exec
}

// SetFactory sets the factory for the node
func (n *Node) SetFactory(f *factory.Factory) {
	n.factory = f
}
