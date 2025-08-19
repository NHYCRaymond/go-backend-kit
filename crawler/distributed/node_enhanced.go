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
	"github.com/NHYCRaymond/go-backend-kit/database"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// EnhancedNodeConfig contains enhanced node configuration
type EnhancedNodeConfig struct {
	*NodeConfig
	MongoDB        *mongo.Database
	MongoDBWrapper *database.MongoDatabase  // Add wrapper for storage
	MySQL          *gorm.DB
	ScriptBase     string                   // Lua脚本基础路径
}

// EnhanceNode adds executor and factory to an existing node
func EnhanceNode(node *Node, config *EnhancedNodeConfig) error {
	// Create factory for pipeline and storage
	node.factory = factory.NewFactory(config.MongoDB, config.MySQL, node.redis)
	
	// Set MongoDB wrapper if available
	if config.MongoDBWrapper != nil {
		node.factory.SetMongoDBWrapper(config.MongoDBWrapper)
	}

	// Create multiple extractors for different content types
	node.extractors = map[string]task.Extractor{
		"json": extractor.NewJSONExtractor(nil),
		"css":  extractor.NewCSSExtractor(),
		"html": extractor.NewCSSExtractor(), // CSS extractor works for HTML
	}

	// Create executor with Lua support
	// The executor now has MongoDB and Redis for Lua scripts
	exec := executor.NewExecutor(&executor.Config{
		Fetcher:    fetcher.NewHTTPFetcher(),
		Extractor:  nil, // Will be set dynamically per task
		Factory:    node.factory,
		Logger:     node.logger,
		MongoDB:    config.MongoDB,    // Add MongoDB for Lua
		Redis:      node.redis,         // Add Redis for Lua
		Scheduler:  nil,                // Node doesn't need scheduler, tasks come from queue
		ScriptBase: config.ScriptBase,  // Pass script base path for Lua
	})

	node.executor = exec

	node.logger.Info("Node enhanced with executor and factory (Lua support enabled)", "node_id", node.ID)

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

	// Check if task has Lua script configured
	hasLuaScript := t.ProjectID != "" && t.LuaScript != ""
	
	w.Node.logger.Info("Executing task with enhanced executor",
		"task_id", t.ID,
		"worker_id", w.ID,
		"url", t.URL,
		"method", t.Method,
		"task_type", t.Type,
		"has_extract_rules", len(t.ExtractRules) > 0,
		"has_lua_script", hasLuaScript,
		"project_id", t.ProjectID,
		"lua_script", t.LuaScript)
	
	// Log request details for API tasks
	if t.Type == "api" {
		w.Node.logger.Info("📤 API Request details",
			"task_id", t.ID,
			"url", t.URL,
			"method", t.Method,
			"headers", t.Headers,
			"body", string(t.Body))
		
		// Log extraction rules
		if len(t.ExtractRules) > 0 {
			for i, rule := range t.ExtractRules {
				w.Node.logger.Info("🎯 Extraction rule",
					"task_id", t.ID,
					"rule_index", i,
					"field", rule.Field,
					"selector", rule.Selector,
					"type", rule.Type)
			}
		}
	}

	// Execute using the new executor
	execResult, err := w.Node.executor.Execute(ctx, t)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		w.Node.logger.Error("Task execution failed",
			"task_id", t.ID,
			"error", err)
		return err
	}
	
	w.Node.logger.Info("Task execution completed",
		"task_id", t.ID,
		"status", execResult.Status,
		"bytes_fetched", execResult.BytesFetched,
		"items_extracted", execResult.ItemsExtracted,
		"links_found", len(execResult.Links),
		"duration_ms", execResult.Duration)
	
	// Log extracted data for API tasks
	if t.Type == "api" && execResult.Data != nil {
		// Convert data to JSON for logging
		dataJSON, err := json.Marshal(execResult.Data)
		if err == nil {
			dataPreview := string(dataJSON)
			if len(dataPreview) > 1000 {
				dataPreview = dataPreview[:1000] + "..."
			}
			w.Node.logger.Info("📊 Extracted data from API",
				"task_id", t.ID,
				"url", t.URL,
				"data", dataPreview)
		}
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
	
	w.Node.logger.Debug("Building task result",
		"task_id", t.ID,
		"exec_status", execResult.Status,
		"exec_bytes", execResult.BytesFetched,
		"exec_items", execResult.ItemsExtracted,
		"data_keys", len(execResult.Data))
	
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
