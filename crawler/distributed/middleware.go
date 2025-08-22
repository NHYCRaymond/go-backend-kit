package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/common"
	"github.com/NHYCRaymond/go-backend-kit/crawler/compression"
	"github.com/go-redis/redis/v8"
)

// ResultMiddleware defines interface for processing task results
type ResultMiddleware interface {
	Process(ctx context.Context, result *TaskResult, next func(context.Context, *TaskResult) error) error
	Name() string
}

// MiddlewareChain manages a chain of middleware
type MiddlewareChain struct {
	middlewares []ResultMiddleware
}

// NewMiddlewareChain creates a new middleware chain
func NewMiddlewareChain() *MiddlewareChain {
	return &MiddlewareChain{
		middlewares: make([]ResultMiddleware, 0),
	}
}

// Use adds a middleware to the chain
func (mc *MiddlewareChain) Use(middleware ResultMiddleware) {
	mc.middlewares = append(mc.middlewares, middleware)
}

// Execute runs the middleware chain
func (mc *MiddlewareChain) Execute(ctx context.Context, result *TaskResult) error {
	if len(mc.middlewares) == 0 {
		return nil
	}

	// Build the chain
	var chain func(context.Context, *TaskResult) error
	chain = func(ctx context.Context, r *TaskResult) error {
		return nil // End of chain
	}

	// Wrap middlewares in reverse order
	for i := len(mc.middlewares) - 1; i >= 0; i-- {
		middleware := mc.middlewares[i]
		next := chain
		chain = func(ctx context.Context, r *TaskResult) error {
			return middleware.Process(ctx, r, next)
		}
	}

	return chain(ctx, result)
}

// CompressionMiddleware implements compression for task results
type CompressionMiddleware struct {
	optimizer    *compression.StorageOptimizer
	redis        *redis.Client
	redisPrefix  string
	logger       interface{ Info(string, ...interface{}); Error(string, ...interface{}) }
}

// NewCompressionMiddleware creates a new compression middleware
func NewCompressionMiddleware(redis *redis.Client, redisPrefix string, logger interface{ Info(string, ...interface{}); Error(string, ...interface{}) }) (*CompressionMiddleware, error) {
	optimizerConfig := &compression.OptimizerConfig{
		CompressionType:       compression.CompressionMsgpackZstd,
		CompressionLevel:      compression.CompressionDefault,
		MinSizeForCompression: 1024,
		EnableRedisCompression: true,
		RedisKeyPrefix:        redisPrefix + ":compressed",
		AutoSelectCompression: true,
	}

	optimizer, err := compression.NewStorageOptimizer(optimizerConfig, redis, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage optimizer: %w", err)
	}

	return &CompressionMiddleware{
		optimizer:   optimizer,
		redis:       redis,
		redisPrefix: redisPrefix,
		logger:      logger,
	}, nil
}

// Process implements ResultMiddleware
func (cm *CompressionMiddleware) Process(ctx context.Context, result *TaskResult, next func(context.Context, *TaskResult) error) error {
	// Store compressed result
	resultKey := fmt.Sprintf("%s:result:%s", cm.redisPrefix, result.TaskID)
	
	// Try to use optimized storage
	if err := cm.optimizer.OptimizedRedisSet(ctx, resultKey, result, common.DefaultResultTTL); err != nil {
		cm.logger.Error("Failed to store compressed task result",
			"task_id", result.TaskID,
			"error", err)
		
		// Fallback to uncompressed storage
		if data, err := json.Marshal(result); err == nil {
			cm.redis.Set(ctx, resultKey, data, common.DefaultResultTTL)
		}
	} else {
		// Log compression success
		originalData, _ := json.Marshal(result)
		cm.logger.Info("Result compressed",
			"task_id", result.TaskID,
			"original_size", len(originalData))
	}

	// Continue chain
	return next(ctx, result)
}

// Name returns middleware name
func (cm *CompressionMiddleware) Name() string {
	return "compression"
}

// EventPublishMiddleware publishes task completion events to Redis
type EventPublishMiddleware struct {
	redis       *redis.Client
	logger      interface{ Info(string, ...interface{}); Error(string, ...interface{}) }
}

// NewEventPublishMiddleware creates a new event publish middleware
func NewEventPublishMiddleware(redis *redis.Client, logger interface{ Info(string, ...interface{}); Error(string, ...interface{}) }) *EventPublishMiddleware {
	return &EventPublishMiddleware{
		redis:  redis,
		logger: logger,
	}
}

// Process implements ResultMiddleware
func (em *EventPublishMiddleware) Process(ctx context.Context, result *TaskResult, next func(context.Context, *TaskResult) error) error {
	// Publish event to Redis queue
	event := map[string]interface{}{
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
		if err := em.redis.RPush(ctx, common.BuildRedisKey(common.DefaultRedisPrefix, common.RedisKeyTasksCompleted), eventJSON).Err(); err != nil {
			em.logger.Error("Failed to publish task event",
				"task_id", result.TaskID,
				"error", err)
		} else {
			em.logger.Info("Published task event",
				"task_id", result.TaskID,
				"queue", common.BuildRedisKey(common.DefaultRedisPrefix, common.RedisKeyTasksCompleted))
		}
	}

	// Continue chain
	return next(ctx, result)
}

// Name returns middleware name
func (em *EventPublishMiddleware) Name() string {
	return "event_publish"
}

// MetricsMiddleware collects metrics for task results
type MetricsMiddleware struct {
	successCount int64
	failureCount int64
	totalItems   int64
	logger       interface{ Info(string, ...interface{}) }
}

// NewMetricsMiddleware creates a new metrics middleware
func NewMetricsMiddleware(logger interface{ Info(string, ...interface{}) }) *MetricsMiddleware {
	return &MetricsMiddleware{
		logger: logger,
	}
}

// Process implements ResultMiddleware
func (mm *MetricsMiddleware) Process(ctx context.Context, result *TaskResult, next func(context.Context, *TaskResult) error) error {
	// Collect metrics
	if result.Error == "" {
		mm.successCount++
		mm.totalItems += int64(len(result.Data))
	} else {
		mm.failureCount++
	}

	// Log metrics periodically
	total := mm.successCount + mm.failureCount
	if total%100 == 0 {
		mm.logger.Info("Task metrics",
			"success", mm.successCount,
			"failure", mm.failureCount,
			"total_items", mm.totalItems,
			"avg_items", mm.totalItems/mm.successCount)
	}

	// Continue chain
	return next(ctx, result)
}

// Name returns middleware name
func (mm *MetricsMiddleware) Name() string {
	return "metrics"
}

// GetMetrics returns current metrics
func (mm *MetricsMiddleware) GetMetrics() map[string]int64 {
	return map[string]int64{
		"success_count": mm.successCount,
		"failure_count": mm.failureCount,
		"total_items":   mm.totalItems,
	}
}