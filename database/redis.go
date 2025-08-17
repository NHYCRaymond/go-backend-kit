package database

import (
	"context"
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/go-redis/redis/v8"
)

type RedisDatabase struct {
	BaseDatabase
	client *redis.Client
	config config.RedisConfig
}

func NewRedis(cfg config.RedisConfig) *RedisDatabase {
	return &RedisDatabase{
		config: cfg,
	}
}

func (r *RedisDatabase) Connect(ctx context.Context) error {

	start := time.Now()

	rdb := redis.NewClient(&redis.Options{
		Addr:     r.config.Address,
		Username: r.config.Username,
		Password: r.config.Password,
		DB:       r.config.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		r.SetLastError(err.Error())
		r.IncrementErrorCount()
		return fmt.Errorf("failed to ping Redis: %w", err)
	}

	r.client = rdb
	r.SetConnected(true)
	r.SetConnectionTime(time.Since(start))
	r.SetLastError("")

	return nil
}

func (r *RedisDatabase) Disconnect(ctx context.Context) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.connected || r.client == nil {
		return nil
	}

	if err := r.client.Close(); err != nil {
		r.SetLastError(err.Error())
		r.IncrementErrorCount()
		return err
	}

	r.connected = false
	r.client = nil
	return nil
}

func (r *RedisDatabase) HealthCheck(ctx context.Context) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if !r.connected || r.client == nil {
		return fmt.Errorf("Redis not connected")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := r.client.Ping(ctx).Result(); err != nil {
		return fmt.Errorf("Redis ping failed: %w", err)
	}

	return nil
}

func (r *RedisDatabase) GetClient() interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.client
}

func (r *RedisDatabase) Type() DatabaseType {
	return TypeRedis
}

func (r *RedisDatabase) Stats() DatabaseStats {

	stats := DatabaseStats{
		Type:           TypeRedis,
		Connected:      r.IsConnected(),
		ConnectionTime: r.GetConnectionTime(),
		TotalQueries:   r.GetQueryCount(),
		ErrorCount:     r.GetErrorCount(),
		LastError:      r.GetLastError(),
	}

	if r.IsConnected() && r.client != nil {
		// Redis doesn't provide detailed connection stats like MySQL
		// We'll use approximate values
		stats.MaxConnections = 1
		stats.ActiveConnections = 1
		stats.IdleConnections = 0
	}

	return stats
}

// NewRedisLegacy Legacy function for backward compatibility
func NewRedisLegacy(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	redisDB := NewRedis(cfg)
	if err := redisDB.Connect(ctx); err != nil {
		return nil, err
	}
	return redisDB.GetClient().(*redis.Client), nil
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
