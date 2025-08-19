package executor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/factory"
	"github.com/NHYCRaymond/go-backend-kit/crawler/parser"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
)

// Executor executes crawler tasks
type Executor struct {
	fetcher    task.Fetcher
	extractor  task.Extractor
	factory    *factory.Factory
	logger     *slog.Logger
	mongodb    *mongo.Database
	redis      *redis.Client
	scheduler  task.TaskScheduler // 用于添加新任务
	scriptBase string             // Lua脚本基础路径
}

// Config holds executor configuration
type Config struct {
	Fetcher    task.Fetcher
	Extractor  task.Extractor
	Factory    *factory.Factory
	Logger     *slog.Logger
	MongoDB    *mongo.Database
	Redis      *redis.Client
	Scheduler  task.TaskScheduler
	ScriptBase string // Lua脚本基础路径（可选，如 "/app/scripts" 或 "./crawler/scripts"）
}

// NewExecutor creates a new task executor
func NewExecutor(config *Config) *Executor {
	// 使用默认脚本路径如果未指定
	scriptBase := config.ScriptBase
	if scriptBase == "" {
		scriptBase = "crawler/scripts"
	}

	return &Executor{
		fetcher:    config.Fetcher,
		extractor:  config.Extractor,
		factory:    config.Factory,
		logger:     config.Logger,
		mongodb:    config.MongoDB,
		redis:      config.Redis,
		scheduler:  config.Scheduler,
		scriptBase: scriptBase,
	}
}

// Execute executes a task
func (e *Executor) Execute(ctx context.Context, t *task.Task) (*task.TaskResult, error) {
	startTime := time.Now()

	result := &task.TaskResult{
		TaskID:    t.ID,
		StartTime: startTime.Unix(),
	}

	e.logger.Info("Executing task",
		"task_id", t.ID,
		"url", t.URL,
		"type", t.Type)

	// Step 1: Fetch content
	e.logger.Info("Starting fetch phase",
		"task_id", t.ID,
		"url", t.URL,
		"method", t.Method)

	response, err := e.fetcher.Fetch(ctx, t)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("fetch failed: %v", err)
		e.logger.Error("Failed to fetch",
			"task_id", t.ID,
			"error", err)
		return result, err
	}

	e.logger.Info("Fetch completed",
		"task_id", t.ID,
		"response_size", response.Size,
		"status_code", response.StatusCode,
		"body_length", len(response.Body))

	result.BytesFetched = response.Size

	// Check if task has Lua script configured
	e.logger.Info("Checking for Lua script",
		"task_id", t.ID,
		"project_id", t.ProjectID,
		"lua_script", t.LuaScript,
		"has_project", t.ProjectID != "",
		"has_script", t.LuaScript != "")

	if t.ProjectID != "" && t.LuaScript != "" {
		e.logger.Info("Using Lua script for parsing",
			"task_id", t.ID,
			"project_id", t.ProjectID,
			"lua_script", t.LuaScript,
			"script_base", e.scriptBase)

		// Create Lua engine with script base path
		luaEngine := parser.NewLuaEngineWithConfig(&parser.LuaEngineConfig{
			MongoDB:    e.mongodb,
			Redis:      e.redis,
			Logger:     e.logger,
			ScriptBase: e.scriptBase,
		})
		defer luaEngine.Close()

		e.logger.Debug("Lua engine created",
			"script_base", e.scriptBase)

		// Prepare task info for Lua script
		taskInfo := map[string]string{
			"task_id":   t.ID,
			"url":       t.URL,
			"parent_id": t.ParentID,
		}

		// Execute Lua script with context
		err := luaEngine.ExecuteWithContext(t.ProjectID, t.LuaScript, response.Body, taskInfo)
		if err != nil {
			result.Status = "failed"
			result.Error = fmt.Sprintf("lua script failed: %v", err)
			e.logger.Error("Lua script execution failed",
				"task_id", t.ID,
				"project_id", t.ProjectID,
				"script", t.LuaScript,
				"error", err)
			return result, err
		}

		// Process tasks created by Lua script
		go func() {
			for newTask := range luaEngine.GetTaskQueue() {
				if e.scheduler != nil {
					e.logger.Debug("Adding task from Lua script",
						"parent_task", t.ID,
						"new_task", newTask.ID,
						"type", newTask.Type)
					e.scheduler.AddTask(newTask)
				}
			}
		}()

		// Lua script handles its own storage, so we're done
		result.Status = "success"
		result.Message = "Lua script executed successfully"
		result.EndTime = time.Now().Unix()
		result.Duration = time.Since(startTime).Milliseconds()

		e.logger.Info("Task completed with Lua script",
			"task_id", t.ID,
			"duration_ms", result.Duration)

		return result, nil
	}

	// Step 2: Extract data (original flow)
	var extractedData map[string]interface{}
	if len(t.ExtractRules) > 0 {
		e.logger.Info("Starting extraction phase",
			"task_id", t.ID,
			"rules_count", len(t.ExtractRules),
			"extractor_type", e.extractor.GetType())

		extractedData, err = e.extractor.Extract(ctx, response, t.ExtractRules)
		if err != nil {
			result.Status = "failed"
			result.Error = fmt.Sprintf("extraction failed: %v", err)
			e.logger.Error("Failed to extract",
				"task_id", t.ID,
				"error", err)
			return result, err
		}

		// Calculate actual items extracted (could be nested)
		itemsCount := 0
		for key, value := range extractedData {
			switch v := value.(type) {
			case []interface{}:
				itemsCount += len(v)
			case []map[string]interface{}:
				itemsCount += len(v)
			default:
				itemsCount++
			}
			e.logger.Debug("Extracted field",
				"task_id", t.ID,
				"field", key,
				"type", fmt.Sprintf("%T", value))
		}

		result.ItemsExtracted = itemsCount
		e.logger.Info("Extraction completed",
			"task_id", t.ID,
			"fields_extracted", len(extractedData),
			"items_extracted", itemsCount)
	} else {
		e.logger.Info("No extraction rules defined",
			"task_id", t.ID)
	}

	// Step 3: Extract links for further crawling (if seed task)
	if t.Type == "seed" || t.Type == "list" {
		e.logger.Info("Extracting links for crawling",
			"task_id", t.ID,
			"task_type", t.Type)

		links, err := e.extractor.ExtractLinks(ctx, response)
		if err != nil {
			e.logger.Warn("Failed to extract links",
				"task_id", t.ID,
				"error", err)
		} else {
			result.Links = links
			e.logger.Info("Extracted links",
				"task_id", t.ID,
				"count", len(links))
			if len(links) > 0 && len(links) <= 5 {
				// Log first few links for debugging
				e.logger.Debug("Sample links",
					"task_id", t.ID,
					"links", links)
			}
		}
	}

	// Step 4: Process through pipeline
	if t.PipelineID != "" {
		pipeline, err := e.factory.CreatePipeline(t.PipelineID)
		if err != nil {
			e.logger.Warn("Failed to create pipeline",
				"task_id", t.ID,
				"pipeline_id", t.PipelineID,
				"error", err)
		} else {
			processedData, err := pipeline.Process(ctx, extractedData)
			if err != nil {
				result.Status = "failed"
				result.Error = fmt.Sprintf("pipeline failed: %v", err)
				e.logger.Error("Pipeline processing failed",
					"task_id", t.ID,
					"error", err)
				return result, err
			}
			extractedData = processedData
		}
	}

	// Step 5: Save to storage
	e.logger.Info("Checking storage configuration",
		"task_id", t.ID,
		"storage_type", t.StorageConf.Type,
		"storage_database", t.StorageConf.Database,
		"storage_collection", t.StorageConf.Collection)

	if t.StorageConf.Type != "" {
		e.logger.Info("Creating storage handler",
			"task_id", t.ID,
			"storage_type", t.StorageConf.Type)

		storage, err := e.factory.CreateStorage(t.StorageConf)
		if err != nil {
			e.logger.Error("Failed to create storage",
				"task_id", t.ID,
				"storage_type", t.StorageConf.Type,
				"error", err)
		} else {
			e.logger.Info("Storage handler created successfully",
				"task_id", t.ID,
				"storage_type", t.StorageConf.Type)
			defer storage.Close()

			// Add metadata to the data
			dataWithMeta := map[string]interface{}{
				"task_id":   t.ID,
				"parent_id": t.ParentID,
				"url":       t.URL,
				"timestamp": time.Now(),
				"data":      extractedData,
			}

			e.logger.Info("Saving data to storage",
				"task_id", t.ID,
				"data_fields", len(extractedData))

			if err := storage.Save(ctx, dataWithMeta); err != nil {
				result.Status = "failed"
				result.Error = fmt.Sprintf("storage failed: %v", err)
				e.logger.Error("Failed to save data",
					"task_id", t.ID,
					"error", err)
				return result, err
			}

			e.logger.Info("Data saved successfully",
				"task_id", t.ID,
				"storage_type", t.StorageConf.Type)
		}
	} else {
		e.logger.Warn("No storage configuration found",
			"task_id", t.ID,
			"storage_conf", t.StorageConf)
	}

	// Success
	result.Status = "success"
	result.Data = extractedData
	result.EndTime = time.Now().Unix()
	result.Duration = time.Since(startTime).Milliseconds()

	e.logger.Info("Task completed successfully",
		"task_id", t.ID,
		"duration_ms", result.Duration,
		"items_extracted", result.ItemsExtracted,
		"bytes_fetched", result.BytesFetched)

	return result, nil
}

// SetFetcher sets the fetcher
func (e *Executor) SetFetcher(fetcher task.Fetcher) {
	e.fetcher = fetcher
}

// SetExtractor sets the extractor
func (e *Executor) SetExtractor(extractor task.Extractor) {
	e.extractor = extractor
}

// SetPipeline sets the pipeline (not used, pipelines are created via factory)
func (e *Executor) SetPipeline(pipeline task.Pipeline) {
	// Pipeline is created dynamically from PipelineID via factory
	// This method is here to satisfy the interface
}

// SetStorage sets the storage (not used, storage is created via factory)
func (e *Executor) SetStorage(storage task.Storage) {
	// Storage is created dynamically from StorageConfig via factory
	// This method is here to satisfy the interface
}
