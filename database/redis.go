package database

import (
	"context"
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/go-redis/redis/v8"
)

// NewRedis creates a new Redis client and connects to the server
func NewRedis(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	return rdb, nil
}

// RedisHealthCheck checks Redis connection health
func RedisHealthCheck(ctx context.Context, rdb *redis.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		return fmt.Errorf("Redis ping failed: %w", err)
	}

	return nil
}

// DistributedLock implements distributed locking using Redis
type DistributedLock struct {
	client *redis.Client
	key    string
	value  string
	expiry time.Duration
}

// NewDistributedLock creates a new distributed lock
func NewDistributedLock(client *redis.Client, key string, expiry time.Duration) *DistributedLock {
	return &DistributedLock{
		client: client,
		key:    key,
		value:  fmt.Sprintf("lock:%d", time.Now().UnixNano()),
		expiry: expiry,
	}
}

// Lock acquires the distributed lock
func (dl *DistributedLock) Lock(ctx context.Context) error {
	success, err := dl.client.SetNX(ctx, dl.key, dl.value, dl.expiry).Result()
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !success {
		return fmt.Errorf("lock already acquired")
	}

	return nil
}

// Unlock releases the distributed lock
func (dl *DistributedLock) Unlock(ctx context.Context) error {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`

	result, err := dl.client.Eval(ctx, script, []string{dl.key}, dl.value).Result()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	if result == int64(0) {
		return fmt.Errorf("lock not found or not owned")
	}

	return nil
}

// TryLock attempts to acquire the lock with a timeout
func (dl *DistributedLock) TryLock(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("lock acquisition timeout")
		case <-ticker.C:
			if err := dl.Lock(ctx); err == nil {
				return nil
			}
		}
	}
}

// Extend extends the lock expiry
func (dl *DistributedLock) Extend(ctx context.Context, expiry time.Duration) error {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("EXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	result, err := dl.client.Eval(ctx, script, []string{dl.key}, dl.value, int(expiry.Seconds())).Result()
	if err != nil {
		return fmt.Errorf("failed to extend lock: %w", err)
	}

	if result == int64(0) {
		return fmt.Errorf("lock not found or not owned")
	}

	return nil
}