package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/compression"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
)

// CompressedCoordinator extends Coordinator with compression support
type CompressedCoordinator struct {
	*Coordinator
	optimizer *compression.StorageOptimizer
}

// NewCompressedCoordinator creates a coordinator with compression support
func NewCompressedCoordinator(coordinator *Coordinator) (*CompressedCoordinator, error) {
	// Create optimizer configuration
	optimizerConfig := &compression.OptimizerConfig{
		CompressionType:       compression.CompressionMsgpackZstd, // Best compression
		CompressionLevel:      compression.CompressionDefault,
		MinSizeForCompression: 1024, // Compress if > 1KB
		EnableRedisCompression: true,
		RedisKeyPrefix:        "crawler:compressed",
		EnableMongoCompression: true,
		MongoCompactKeys:      true,
		AutoSelectCompression: true,
	}

	optimizer, err := compression.NewStorageOptimizer(
		optimizerConfig,
		coordinator.redis,
		nil, // MongoDB will be set separately if needed
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage optimizer: %w", err)
	}

	return &CompressedCoordinator{
		Coordinator: coordinator,
		optimizer:   optimizer,
	}, nil
}

// handleTaskResultCompressed handles task result with compression
func (cc *CompressedCoordinator) handleTaskResultCompressed(ctx context.Context, result *TaskResult) {
	cc.mu.Lock()
	delete(cc.pendingTasks, result.TaskID)
	nodeID := cc.taskAssignments[result.TaskID]
	delete(cc.taskAssignments, result.TaskID)
	cc.mu.Unlock()
	
	// Update task result in MongoDB if scheduler is available
	if cc.scheduler != nil {
		success := result.Error == ""
		cc.scheduler.UpdateTaskResult(ctx, result.TaskID, success, result.Error)
	}
	
	if result.Error != "" {
		cc.logger.Error("Task failed",
			"task_id", result.TaskID,
			"node_id", nodeID,
			"error", result.Error)
		
		cc.handleTaskFailure(ctx, result.TaskID, result.Error)
	} else {
		cc.logger.Info("Task completed",
			"task_id", result.TaskID,
			"node_id", nodeID,
			"items", len(result.Data))
		
		// Store compressed result
		resultKey := fmt.Sprintf("%s:result:%s", cc.config.RedisPrefix, result.TaskID)
		
		// Use optimized storage
		if err := cc.optimizer.OptimizedRedisSet(ctx, resultKey, result, 30*time.Minute); err != nil {
			cc.logger.Error("Failed to store compressed task result",
				"task_id", result.TaskID,
				"error", err)
			
			// Fallback to uncompressed storage
			if data, err := json.Marshal(result); err == nil {
				cc.redis.Set(ctx, resultKey, data, 30*time.Minute)
			}
		} else {
			// Log compression stats
			cc.logCompressionStats(result)
		}
		
		// Process seed expansion if needed
		if t, ok := cc.pendingTasks[result.TaskID]; ok && t.Type == string(task.TypeSeed) {
			cc.processSeedResult(ctx, result)
		}
	}
}

// storeTaskCompressed stores task with compression
func (cc *CompressedCoordinator) storeTaskCompressed(ctx context.Context, t *task.Task) error {
	taskKey := fmt.Sprintf("%s:task:%s", cc.config.RedisPrefix, t.ID)
	
	// Use optimized storage with 1 hour TTL
	return cc.optimizer.OptimizedRedisSet(ctx, taskKey, t, 1*time.Hour)
}

// getTaskCompressed retrieves compressed task
func (cc *CompressedCoordinator) getTaskCompressed(ctx context.Context, taskID string) (*task.Task, error) {
	taskKey := fmt.Sprintf("%s:task:%s", cc.config.RedisPrefix, taskID)
	
	var t task.Task
	if err := cc.optimizer.OptimizedRedisGet(ctx, taskKey, &t); err != nil {
		return nil, err
	}
	
	return &t, nil
}

// batchStoreTasksCompressed stores multiple tasks with compression
func (cc *CompressedCoordinator) batchStoreTasksCompressed(ctx context.Context, tasks []*task.Task) error {
	pipe := cc.redis.Pipeline()
	
	for _, t := range tasks {
		taskKey := fmt.Sprintf("%s:task:%s", cc.config.RedisPrefix, t.ID)
		
		// Compress each task
		if err := cc.optimizer.OptimizedRedisSet(ctx, taskKey, t, 1*time.Hour); err != nil {
			cc.logger.Warn("Failed to compress task",
				"task_id", t.ID,
				"error", err)
			
			// Fallback to uncompressed
			if data, err := json.Marshal(t); err == nil {
				pipe.Set(ctx, taskKey, data, 1*time.Hour)
			}
		}
	}
	
	_, err := pipe.Exec(ctx)
	return err
}

// logCompressionStats logs compression statistics
func (cc *CompressedCoordinator) logCompressionStats(result *TaskResult) {
	// Calculate original size
	originalData, _ := json.Marshal(result)
	originalSize := len(originalData)
	
	// Get compression stats from optimizer
	stats, err := cc.optimizer.GetCompressionStats(context.Background())
	if err != nil {
		return
	}
	
	cc.logger.Debug("Compression stats",
		"task_id", result.TaskID,
		"original_size", originalSize,
		"compression_type", stats["compression_type"],
		"space_saved_mb", stats["redis_space_saved_mb"])
}

// GetCompressionMetrics returns compression metrics
func (cc *CompressedCoordinator) GetCompressionMetrics(ctx context.Context) (map[string]interface{}, error) {
	stats, err := cc.optimizer.GetCompressionStats(ctx)
	if err != nil {
		return nil, err
	}
	
	// Add coordinator-specific metrics
	stats["total_tasks"] = len(cc.pendingTasks)
	stats["compressed_results_count"] = cc.countCompressedResults(ctx)
	
	return stats, nil
}

// countCompressedResults counts compressed results in Redis
func (cc *CompressedCoordinator) countCompressedResults(ctx context.Context) int {
	pattern := fmt.Sprintf("%s:result:*", cc.config.RedisPrefix)
	keys, err := cc.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return 0
	}
	
	compressedCount := 0
	for _, key := range keys {
		data, err := cc.redis.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		
		// Check if data is compressed (has compression header)
		if len(data) > 0 && data[0] <= byte(compression.CompressionMsgpackZstd) {
			compressedCount++
		}
	}
	
	return compressedCount
}

// EnableCompression enables compression for the coordinator
func (cc *CompressedCoordinator) EnableCompression() {
	// Note: To enable compression, use CompressedCoordinator methods directly
	// instead of the base Coordinator methods
	
	cc.logger.Info("Compression enabled for coordinator",
		"compression_type", "msgpack+zstd",
		"min_size", 1024)
}