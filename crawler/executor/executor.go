package executor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/factory"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
)

// Executor executes crawler tasks
type Executor struct {
	fetcher   task.Fetcher
	extractor task.Extractor
	factory   *factory.Factory
	logger    *slog.Logger
}

// Config holds executor configuration
type Config struct {
	Fetcher   task.Fetcher
	Extractor task.Extractor
	Factory   *factory.Factory
	Logger    *slog.Logger
}

// NewExecutor creates a new task executor
func NewExecutor(config *Config) *Executor {
	return &Executor{
		fetcher:   config.Fetcher,
		extractor: config.Extractor,
		factory:   config.Factory,
		logger:    config.Logger,
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
	response, err := e.fetcher.Fetch(ctx, t)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("fetch failed: %v", err)
		e.logger.Error("Failed to fetch",
			"task_id", t.ID,
			"error", err)
		return result, err
	}
	
	result.BytesFetched = response.Size
	
	// Step 2: Extract data
	var extractedData map[string]interface{}
	if len(t.ExtractRules) > 0 {
		extractedData, err = e.extractor.Extract(ctx, response, t.ExtractRules)
		if err != nil {
			result.Status = "failed"
			result.Error = fmt.Sprintf("extraction failed: %v", err)
			e.logger.Error("Failed to extract",
				"task_id", t.ID,
				"error", err)
			return result, err
		}
		result.ItemsExtracted = len(extractedData)
	}
	
	// Step 3: Extract links for further crawling (if seed task)
	if t.Type == "seed" || t.Type == "list" {
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
	if t.StorageConf.Type != "" {
		storage, err := e.factory.CreateStorage(t.StorageConf)
		if err != nil {
			e.logger.Error("Failed to create storage",
				"task_id", t.ID,
				"storage_type", t.StorageConf.Type,
				"error", err)
		} else {
			defer storage.Close()
			
			// Add metadata to the data
			dataWithMeta := map[string]interface{}{
				"task_id":   t.ID,
				"parent_id": t.ParentID,
				"url":       t.URL,
				"timestamp": time.Now(),
				"data":      extractedData,
			}
			
			if err := storage.Save(ctx, dataWithMeta); err != nil {
				result.Status = "failed"
				result.Error = fmt.Sprintf("storage failed: %v", err)
				e.logger.Error("Failed to save data",
					"task_id", t.ID,
					"error", err)
				return result, err
			}
		}
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