package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/common"
	"github.com/go-redis/redis/v8"
)

// BatchAggregatorMiddleware aggregates task results by batch
// When all tasks in a batch are processed (success or failure), it publishes a batch complete event
type BatchAggregatorMiddleware struct {
	redis       *redis.Client
	logger      *slog.Logger
	redisPrefix string
	batchTTL    time.Duration
}

// NewBatchAggregatorMiddleware creates a new batch aggregator middleware
func NewBatchAggregatorMiddleware(redis *redis.Client, redisPrefix string, logger *slog.Logger) *BatchAggregatorMiddleware {
	if logger == nil {
		logger = slog.Default()
	}
	
	return &BatchAggregatorMiddleware{
		redis:       redis,
		logger:      logger,
		redisPrefix: redisPrefix,
		batchTTL:    2 * time.Hour, // Default TTL for batch tracking
	}
}

// Process implements ResultMiddleware interface
func (bm *BatchAggregatorMiddleware) Process(ctx context.Context, result *TaskResult, next func(context.Context, *TaskResult) error) error {
	// Log what we received
	bm.logger.Info("BatchAggregatorMiddleware processing result",
		"task_id", result.TaskID,
		"batch_id", result.BatchID,
		"batch_size", result.BatchSize,
		"batch_type", result.BatchType,
		"has_batch", result.BatchID != "")
	
	// If no batch ID, treat as single task and publish immediately
	if result.BatchID == "" {
		bm.logger.Info("No batch ID, publishing single task event",
			"task_id", result.TaskID)
		return bm.publishSingleTaskEvent(ctx, result, next)
	}
	
	// For batch tasks, DO NOT publish individual events
	// Only track progress and publish when batch completes
	
	// Track batch progress
	batchProcessedKey := fmt.Sprintf("%s:batch:%s:processed", bm.redisPrefix, result.BatchID)
	
	// Atomically increment processed count
	processedCount, err := bm.redis.Incr(ctx, batchProcessedKey).Result()
	if err != nil {
		bm.logger.Error("Failed to increment batch counter",
			"batch_id", result.BatchID,
			"task_id", result.TaskID,
			"error", err)
		return err
	}
	
	// Set TTL on first task (when count is 1)
	if processedCount == 1 {
		if err := bm.redis.Expire(ctx, batchProcessedKey, bm.batchTTL).Err(); err != nil {
			bm.logger.Warn("Failed to set TTL on batch counter",
				"batch_id", result.BatchID,
				"error", err)
		}
	}
	
	// Log progress (internal logging only, not publishing to Redis)
	bm.logger.Info("Batch task processed",
		"batch_id", result.BatchID,
		"task_id", result.TaskID,
		"processed", processedCount,
		"total", result.BatchSize,
		"status", result.Status)
	
	// Check if batch is complete
	if int(processedCount) >= result.BatchSize {
		// All tasks in batch are processed (either success or failure)
		bm.logger.Info("Batch complete, publishing event",
			"batch_id", result.BatchID,
			"total_tasks", result.BatchSize)
		
		// ONLY NOW publish the batch complete event
		if err := bm.publishBatchCompleteEvent(ctx, result); err != nil {
			bm.logger.Error("Failed to publish batch complete event",
				"batch_id", result.BatchID,
				"error", err)
		}
		
		// Clean up batch counter
		if err := bm.redis.Del(ctx, batchProcessedKey).Err(); err != nil {
			bm.logger.Warn("Failed to delete batch counter",
				"batch_id", result.BatchID,
				"error", err)
		}
	}
	
	// Continue to next middleware
	return next(ctx, result)
}

// publishSingleTaskEvent publishes event for non-batch tasks
func (bm *BatchAggregatorMiddleware) publishSingleTaskEvent(ctx context.Context, result *TaskResult, next func(context.Context, *TaskResult) error) error {
	// For non-batch tasks, publish individual task event
	event := map[string]interface{}{
		"type":      "task_completed",
		"task_id":   result.TaskID,
		"status":    result.Status,
		"timestamp": time.Now().Format(time.RFC3339),
		"summary": map[string]interface{}{
			"data_count": len(result.Data),
			"error":      result.Error,
			"duration":   result.Duration.String(),
		},
	}
	
	if eventJSON, err := json.Marshal(event); err == nil {
		queueKey := common.BuildRedisKey(bm.redisPrefix, common.RedisKeyTasksCompleted)
		if err := bm.redis.RPush(ctx, queueKey, eventJSON).Err(); err != nil {
			bm.logger.Error("Failed to publish single task event",
				"task_id", result.TaskID,
				"error", err)
		} else {
			bm.logger.Debug("Published single task event",
				"task_id", result.TaskID,
				"queue", queueKey)
		}
	}
	
	return next(ctx, result)
}

// publishBatchCompleteEvent publishes batch completion event
func (bm *BatchAggregatorMiddleware) publishBatchCompleteEvent(ctx context.Context, result *TaskResult) error {
	// Create batch complete event with metadata
	event := map[string]interface{}{
		"type":       "batch_completed",
		"batch_id":   result.BatchID,
		"batch_type": result.BatchType,
		"batch_size": result.BatchSize,
		"timestamp":  time.Now().Format(time.RFC3339),
		// Include batch metadata for downstream processing
		"metadata": map[string]interface{}{
			"project_id": result.ProjectID,
			"source":     result.Source,
		},
	}
	
	// Add custom metadata if present
	if result.Metadata != nil {
		for k, v := range result.Metadata {
			event["metadata"].(map[string]interface{})[k] = v
		}
	}
	
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal batch event: %w", err)
	}
	
	// Publish to completed queue
	queueKey := common.BuildRedisKey(bm.redisPrefix, common.RedisKeyTasksCompleted)
	if err := bm.redis.RPush(ctx, queueKey, eventJSON).Err(); err != nil {
		return fmt.Errorf("failed to publish batch event: %w", err)
	}
	
	bm.logger.Info("Published batch complete event",
		"batch_id", result.BatchID,
		"batch_type", result.BatchType,
		"queue", queueKey)
	
	return nil
}

// Name returns the middleware name
func (bm *BatchAggregatorMiddleware) Name() string {
	return "batch_aggregator"
}

// SetBatchTTL sets the TTL for batch tracking data
func (bm *BatchAggregatorMiddleware) SetBatchTTL(ttl time.Duration) {
	bm.batchTTL = ttl
}