package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/database"
	"github.com/go-redis/redis/v8"
)

// RedisStorage implements Storage using Redis with database module
type RedisStorage struct {
	db     *database.RedisDatabase
	client *redis.Client
	prefix string
	ttl    time.Duration
}

// NewRedisStorage creates a new Redis storage using database module
func NewRedisStorage(db *database.RedisDatabase, config StorageConfig) (*RedisStorage, error) {
	// Get Redis client from database
	clientInterface := db.GetClient()
	if clientInterface == nil {
		return nil, fmt.Errorf("redis database not connected")
	}
	
	client, ok := clientInterface.(*redis.Client)
	if !ok {
		return nil, fmt.Errorf("invalid redis client type")
	}
	
	ttl := config.TTL
	if ttl == 0 {
		ttl = 24 * time.Hour // Default TTL
	}
	
	return &RedisStorage{
		db:     db,
		client: client,
		prefix: config.Prefix,
		ttl:    ttl,
	}, nil
}

// Store stores data with default TTL
func (rs *RedisStorage) Store(ctx context.Context, key string, value interface{}) error {
	return rs.StoreWithTTL(ctx, key, value, rs.ttl)
}

// StoreWithTTL stores data with specific TTL
func (rs *RedisStorage) StoreWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	fullKey := rs.getKey(key)
	
	// Serialize value
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	
	return rs.client.Set(ctx, fullKey, data, ttl).Err()
}

// Get retrieves data by key
func (rs *RedisStorage) Get(ctx context.Context, key string) (interface{}, error) {
	fullKey := rs.getKey(key)
	
	data, err := rs.client.Get(ctx, fullKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("key not found: %s", key)
		}
		return nil, err
	}
	
	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %w", err)
	}
	
	return value, nil
}

// Delete removes data by key
func (rs *RedisStorage) Delete(ctx context.Context, key string) error {
	fullKey := rs.getKey(key)
	return rs.client.Del(ctx, fullKey).Err()
}

// Exists checks if key exists
func (rs *RedisStorage) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := rs.getKey(key)
	
	result, err := rs.client.Exists(ctx, fullKey).Result()
	if err != nil {
		return false, err
	}
	
	return result > 0, nil
}

// BatchStore stores multiple items
func (rs *RedisStorage) BatchStore(ctx context.Context, items map[string]interface{}) error {
	pipe := rs.client.Pipeline()
	
	for key, value := range items {
		fullKey := rs.getKey(key)
		
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
		}
		
		pipe.Set(ctx, fullKey, data, rs.ttl)
	}
	
	_, err := pipe.Exec(ctx)
	return err
}

// BatchGet retrieves multiple items
func (rs *RedisStorage) BatchGet(ctx context.Context, keys []string) (map[string]interface{}, error) {
	pipe := rs.client.Pipeline()
	
	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = rs.getKey(key)
		pipe.Get(ctx, fullKeys[i])
	}
	
	cmds, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}
	
	result := make(map[string]interface{})
	for i, cmd := range cmds {
		if i < len(keys) {
			stringCmd, ok := cmd.(*redis.StringCmd)
			if !ok {
				continue
			}
			
			data, err := stringCmd.Bytes()
			if err != nil {
				continue // Skip if key doesn't exist
			}
			
			var value interface{}
			if err := json.Unmarshal(data, &value); err == nil {
				result[keys[i]] = value
			}
		}
	}
	
	return result, nil
}

// Query performs a query using Redis patterns
func (rs *RedisStorage) Query(ctx context.Context, query interface{}) ([]interface{}, error) {
	pattern, ok := query.(string)
	if !ok {
		return nil, fmt.Errorf("invalid query type for Redis storage")
	}
	
	fullPattern := rs.getKey(pattern)
	
	// Scan for matching keys
	var cursor uint64
	var results []interface{}
	
	for {
		keys, newCursor, err := rs.client.Scan(ctx, cursor, fullPattern, 100).Result()
		if err != nil {
			return nil, err
		}
		
		// Get values for matching keys
		if len(keys) > 0 {
			pipe := rs.client.Pipeline()
			for _, key := range keys {
				pipe.Get(ctx, key)
			}
			
			cmds, err := pipe.Exec(ctx)
			if err != nil && err != redis.Nil {
				return nil, err
			}
			
			for _, cmd := range cmds {
				stringCmd, ok := cmd.(*redis.StringCmd)
				if !ok {
					continue
				}
				
				data, err := stringCmd.Bytes()
				if err != nil {
					continue
				}
				
				var value interface{}
				if err := json.Unmarshal(data, &value); err == nil {
					results = append(results, value)
				}
			}
		}
		
		cursor = newCursor
		if cursor == 0 {
			break
		}
	}
	
	return results, nil
}

// Count returns the number of stored items
func (rs *RedisStorage) Count(ctx context.Context) (int64, error) {
	pattern := rs.getKey("*")
	
	var count int64
	var cursor uint64
	
	for {
		keys, newCursor, err := rs.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return 0, err
		}
		
		count += int64(len(keys))
		
		cursor = newCursor
		if cursor == 0 {
			break
		}
	}
	
	return count, nil
}

// Clear removes all data with the prefix
func (rs *RedisStorage) Clear(ctx context.Context) error {
	pattern := rs.getKey("*")
	
	var cursor uint64
	for {
		keys, newCursor, err := rs.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		
		if len(keys) > 0 {
			if err := rs.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		
		cursor = newCursor
		if cursor == 0 {
			break
		}
	}
	
	return nil
}

// Close closes the storage (Redis connection is managed by database module)
func (rs *RedisStorage) Close() error {
	// Connection lifecycle is managed by database module
	return nil
}

// HealthCheck performs health check
func (rs *RedisStorage) HealthCheck(ctx context.Context) error {
	return rs.db.HealthCheck(ctx)
}

// Type returns storage type
func (rs *RedisStorage) Type() string {
	return "redis"
}

// getKey returns the full Redis key with prefix
func (rs *RedisStorage) getKey(key string) string {
	if rs.prefix != "" {
		return fmt.Sprintf("%s:%s", rs.prefix, key)
	}
	return key
}