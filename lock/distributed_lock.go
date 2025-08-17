package lock

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-redis/redis/v8"
)

// DistributedLock provides distributed locking using Redis
type DistributedLock struct {
	redisClient *redis.Client
	logger      *slog.Logger
}

// NewDistributedLock creates a new distributed lock instance
func NewDistributedLock(redisClient *redis.Client, logger *slog.Logger) *DistributedLock {
	if logger == nil {
		logger = slog.Default()
	}
	return &DistributedLock{
		redisClient: redisClient,
		logger:      logger,
	}
}

// LockResult represents the result of a lock operation
type LockResult struct {
	Key       string
	Value     string
	Acquired  bool
	ExpiresAt time.Time
}

// AcquireLock attempts to acquire a distributed lock
func (dl *DistributedLock) AcquireLock(ctx context.Context, key string, value string, expiration time.Duration) (*LockResult, error) {
	lockKey := fmt.Sprintf("lock:%s", key)

	// Try to set the lock with NX (only if not exists) and EX (expiration)
	result, err := dl.redisClient.SetNX(ctx, lockKey, value, expiration).Result()
	if err != nil {
		dl.logger.Error("Failed to acquire distributed lock",
			"key", lockKey,
			"value", value,
			"error", err)
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	lockResult := &LockResult{
		Key:       lockKey,
		Value:     value,
		Acquired:  result,
		ExpiresAt: time.Now().Add(expiration),
	}

	if result {
		dl.logger.Info("Distributed lock acquired successfully",
			"key", lockKey,
			"value", value,
			"expiration", expiration)
	} else {
		dl.logger.Debug("Failed to acquire distributed lock - already held",
			"key", lockKey,
			"value", value)
	}

	return lockResult, nil
}

// ReleaseLock releases a distributed lock using Lua script for atomicity
func (dl *DistributedLock) ReleaseLock(ctx context.Context, lockResult *LockResult) error {
	// Use Lua script to ensure atomicity: only release if the value matches
	luaScript := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`

	result, err := dl.redisClient.Eval(ctx, luaScript, []string{lockResult.Key}, lockResult.Value).Result()
	if err != nil {
		dl.logger.Error("Failed to release distributed lock",
			"key", lockResult.Key,
			"value", lockResult.Value,
			"error", err)
		return fmt.Errorf("failed to release lock: %w", err)
	}

	released := result.(int64) == 1
	if released {
		dl.logger.Info("Distributed lock released successfully",
			"key", lockResult.Key,
			"value", lockResult.Value)
	} else {
		dl.logger.Warn("Lock was not released - value mismatch or already expired",
			"key", lockResult.Key,
			"value", lockResult.Value)
	}

	return nil
}

// ExtendLock extends the expiration time of an existing lock
func (dl *DistributedLock) ExtendLock(ctx context.Context, lockResult *LockResult, extension time.Duration) error {
	// Use Lua script to extend lock only if the value matches
	luaScript := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("EXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	result, err := dl.redisClient.Eval(ctx, luaScript, []string{lockResult.Key}, lockResult.Value, int(extension.Seconds())).Result()
	if err != nil {
		dl.logger.Error("Failed to extend distributed lock",
			"key", lockResult.Key,
			"value", lockResult.Value,
			"extension", extension,
			"error", err)
		return fmt.Errorf("failed to extend lock: %w", err)
	}

	extended := result.(int64) == 1
	if extended {
		lockResult.ExpiresAt = time.Now().Add(extension)
		dl.logger.Info("Distributed lock extended successfully",
			"key", lockResult.Key,
			"value", lockResult.Value,
			"new_expiration", lockResult.ExpiresAt)
	} else {
		dl.logger.Warn("Lock was not extended - value mismatch or already expired",
			"key", lockResult.Key,
			"value", lockResult.Value)
		return fmt.Errorf("lock not extended")
	}

	return nil
}

// WithLock executes a function while holding a distributed lock
func (dl *DistributedLock) WithLock(ctx context.Context, key string, expiration time.Duration, fn func(ctx context.Context) error) error {
	lockValue := fmt.Sprintf("%d_%s", time.Now().UnixNano(), key)

	// Acquire lock
	lockResult, err := dl.AcquireLock(ctx, key, lockValue, expiration)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !lockResult.Acquired {
		return fmt.Errorf("could not acquire lock for key: %s", key)
	}

	// Ensure lock is released
	defer func() {
		if err := dl.ReleaseLock(context.Background(), lockResult); err != nil {
			dl.logger.Error("Failed to release lock in defer",
				"key", lockResult.Key,
				"error", err)
		}
	}()

	// Execute the protected function
	return fn(ctx)
}

// WithLockT executes a function while holding a distributed lock and returns a typed result
func WithLockT[T any](dl *DistributedLock, ctx context.Context, key string, expiration time.Duration, fn func(ctx context.Context) (T, error)) (T, error) {
	var result T

	lockValue := fmt.Sprintf("%d_%s", time.Now().UnixNano(), key)

	// Acquire lock
	lockResult, err := dl.AcquireLock(ctx, key, lockValue, expiration)
	if err != nil {
		return result, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !lockResult.Acquired {
		return result, fmt.Errorf("could not acquire lock for key: %s", key)
	}

	// Ensure lock is released
	defer func() {
		if err := dl.ReleaseLock(context.Background(), lockResult); err != nil {
			dl.logger.Error("Failed to release lock in defer",
				"key", lockResult.Key,
				"error", err)
		}
	}()

	// Execute the protected function
	return fn(ctx)
}

// IsLocked checks if a lock is currently held
func (dl *DistributedLock) IsLocked(ctx context.Context, key string) (bool, error) {
	lockKey := fmt.Sprintf("lock:%s", key)

	result, err := dl.redisClient.Exists(ctx, lockKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check lock existence: %w", err)
	}

	return result > 0, nil
}

// GetLockInfo returns information about a lock
func (dl *DistributedLock) GetLockInfo(ctx context.Context, key string) (map[string]interface{}, error) {
	lockKey := fmt.Sprintf("lock:%s", key)

	pipe := dl.redisClient.Pipeline()
	valueCmd := pipe.Get(ctx, lockKey)
	ttlCmd := pipe.TTL(ctx, lockKey)

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get lock info: %w", err)
	}

	info := make(map[string]interface{})
	info["key"] = lockKey
	info["exists"] = valueCmd.Err() == nil

	if valueCmd.Err() == redis.Nil {
		info["value"] = nil
		info["ttl"] = -1
	} else {
		info["value"] = valueCmd.Val()
		info["ttl"] = ttlCmd.Val().Seconds()
	}

	return info, nil
}

// TryLock attempts to acquire a lock with retries
func (dl *DistributedLock) TryLock(ctx context.Context, key string, value string, expiration time.Duration, maxRetries int, retryDelay time.Duration) (*LockResult, error) {
	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}

		lockResult, err := dl.AcquireLock(ctx, key, value, expiration)
		if err != nil {
			return nil, err
		}

		if lockResult.Acquired {
			return lockResult, nil
		}
	}

	return nil, fmt.Errorf("failed to acquire lock after %d retries", maxRetries)
}

// LockOptions configures lock behavior
type LockOptions struct {
	Expiration      time.Duration
	MaxRetries      int
	RetryDelay      time.Duration
	RefreshInterval time.Duration
}

// DefaultLockOptions returns default lock options
func DefaultLockOptions() *LockOptions {
	return &LockOptions{
		Expiration:      30 * time.Second,
		MaxRetries:      3,
		RetryDelay:      100 * time.Millisecond,
		RefreshInterval: 10 * time.Second,
	}
}

// WithAutoRefresh executes a function while holding a lock with automatic refresh
func (dl *DistributedLock) WithAutoRefresh(ctx context.Context, key string, opts *LockOptions, fn func(ctx context.Context) error) error {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	lockValue := fmt.Sprintf("%d_%s", time.Now().UnixNano(), key)

	// Try to acquire lock with retries
	lockResult, err := dl.TryLock(ctx, key, lockValue, opts.Expiration, opts.MaxRetries, opts.RetryDelay)
	if err != nil {
		return err
	}

	// Create a context for the refresh goroutine
	refreshCtx, cancelRefresh := context.WithCancel(ctx)
	defer cancelRefresh()

	// Start auto-refresh goroutine
	refreshDone := make(chan struct{})
	go func() {
		defer close(refreshDone)
		ticker := time.NewTicker(opts.RefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-refreshCtx.Done():
				return
			case <-ticker.C:
				if err := dl.ExtendLock(refreshCtx, lockResult, opts.Expiration); err != nil {
					dl.logger.Error("Failed to refresh lock", "key", key, "error", err)
					return
				}
			}
		}
	}()

	// Execute the function
	fnErr := fn(ctx)

	// Stop refresh and release lock
	cancelRefresh()
	<-refreshDone

	if err := dl.ReleaseLock(context.Background(), lockResult); err != nil {
		dl.logger.Error("Failed to release lock", "key", key, "error", err)
	}

	return fnErr
}
