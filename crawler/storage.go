package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/common"
	"github.com/NHYCRaymond/go-backend-kit/crawler/core"
	"github.com/go-redis/redis/v8"
)

// MemoryStorage implements in-memory storage
type MemoryStorage struct {
	mu    sync.RWMutex
	data  map[string]interface{}
	ttl   map[string]time.Time
}

// NewMemoryStorage creates a new memory storage
func NewMemoryStorage() core.Storage {
	storage := &MemoryStorage{
		data: make(map[string]interface{}),
		ttl:  make(map[string]time.Time),
	}
	
	// Start cleanup goroutine
	go storage.cleanup()
	
	return storage
}

// Store stores data with optional TTL
func (ms *MemoryStorage) Store(ctx context.Context, key string, value interface{}) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	ms.data[key] = value
	// Default TTL of 24 hours
	ms.ttl[key] = time.Now().Add(common.DefaultCacheTTL)
	
	return nil
}

// StoreWithTTL stores data with specific TTL
func (ms *MemoryStorage) StoreWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	ms.data[key] = value
	ms.ttl[key] = time.Now().Add(ttl)
	
	return nil
}

// Get retrieves data by key
func (ms *MemoryStorage) Get(ctx context.Context, key string) (interface{}, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	// Check if expired
	if expiry, exists := ms.ttl[key]; exists {
		if time.Now().After(expiry) {
			return nil, fmt.Errorf("key expired: %s", key)
		}
	}
	
	value, exists := ms.data[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	
	return value, nil
}

// Delete removes data by key
func (ms *MemoryStorage) Delete(ctx context.Context, key string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	delete(ms.data, key)
	delete(ms.ttl, key)
	
	return nil
}

// Exists checks if key exists
func (ms *MemoryStorage) Exists(ctx context.Context, key string) (bool, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	// Check if expired
	if expiry, exists := ms.ttl[key]; exists {
		if time.Now().After(expiry) {
			return false, nil
		}
	}
	
	_, exists := ms.data[key]
	return exists, nil
}

// BatchStore stores multiple items
func (ms *MemoryStorage) BatchStore(ctx context.Context, items map[string]interface{}) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	defaultExpiry := time.Now().Add(common.DefaultCacheTTL)
	for key, value := range items {
		ms.data[key] = value
		ms.ttl[key] = defaultExpiry
	}
	
	return nil
}

// BatchGet retrieves multiple items
func (ms *MemoryStorage) BatchGet(ctx context.Context, keys []string) (map[string]interface{}, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	result := make(map[string]interface{})
	now := time.Now()
	
	for _, key := range keys {
		// Check if expired
		if expiry, exists := ms.ttl[key]; exists {
			if now.After(expiry) {
				continue
			}
		}
		
		if value, exists := ms.data[key]; exists {
			result[key] = value
		}
	}
	
	return result, nil
}

// Query implements core.Storage interface
func (ms *MemoryStorage) Query(ctx context.Context, collection string, filter map[string]interface{}) ([]map[string]interface{}, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	var results []map[string]interface{}
	now := time.Now()
	
	// Simple implementation - return all items from collection
	prefix := collection + ":"
	for key, value := range ms.data {
		// Check if expired
		if expiry, exists := ms.ttl[key]; exists {
			if now.After(expiry) {
				continue
			}
		}
		
		if strings.HasPrefix(key, prefix) {
			if mapValue, ok := value.(map[string]interface{}); ok {
				// Simple filter matching - check if all filter keys match
				if len(filter) > 0 {
					match := true
					for fk, fv := range filter {
						if mv, exists := mapValue[fk]; !exists || mv != fv {
							match = false
							break
						}
					}
					if !match {
						continue
					}
				}
				results = append(results, mapValue)
			}
		}
	}
	
	return results, nil
}

// Count returns the number of stored items
func (ms *MemoryStorage) Count(ctx context.Context) (int64, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	count := int64(0)
	now := time.Now()
	
	for key, expiry := range ms.ttl {
		if now.Before(expiry) {
			if _, exists := ms.data[key]; exists {
				count++
			}
		}
	}
	
	return count, nil
}

// Clear removes all data
func (ms *MemoryStorage) Clear(ctx context.Context) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	ms.data = make(map[string]interface{})
	ms.ttl = make(map[string]time.Time)
	
	return nil
}

// cleanup removes expired items periodically
func (ms *MemoryStorage) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		ms.mu.Lock()
		now := time.Now()
		
		for key, expiry := range ms.ttl {
			if now.After(expiry) {
				delete(ms.data, key)
				delete(ms.ttl, key)
			}
		}
		
		ms.mu.Unlock()
	}
}

// Save implements core.Storage interface
func (ms *MemoryStorage) Save(ctx context.Context, collection string, data []map[string]interface{}) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	for i, item := range data {
		key := fmt.Sprintf("%s:%d:%d", collection, time.Now().UnixNano(), i)
		ms.data[key] = item
		ms.ttl[key] = time.Now().Add(common.DefaultCacheTTL)
	}
	
	return nil
}


// Close implements core.Storage interface
func (ms *MemoryStorage) Close() error {
	// Nothing to close for in-memory storage
	return nil
}

// RedisStorage implements Redis-based storage
type RedisStorage struct {
	client *redis.Client
	prefix string
}

// NewRedisStorage creates a new Redis storage
func NewRedisStorage(client *redis.Client, prefix string) core.Storage {
	return &RedisStorage{
		client: client,
		prefix: prefix,
	}
}

// Store stores data with default TTL
func (rs *RedisStorage) Store(ctx context.Context, key string, value interface{}) error {
	return rs.StoreWithTTL(ctx, key, value, 24*time.Hour)
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
		
		pipe.Set(ctx, fullKey, data, 24*time.Hour)
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

// QueryGeneric performs a generic query using Redis patterns
func (rs *RedisStorage) QueryGeneric(ctx context.Context, query interface{}) ([]interface{}, error) {
	pattern, ok := query.(string)
	if !ok {
		return nil, fmt.Errorf("invalid query type")
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

// Close implements core.Storage interface
func (rs *RedisStorage) Close() error {
	// Redis client is managed externally
	return nil
}

// Save implements core.Storage interface
func (rs *RedisStorage) Save(ctx context.Context, collection string, data []map[string]interface{}) error {
	pipe := rs.client.Pipeline()
	
	for i, item := range data {
		key := rs.getKey(fmt.Sprintf("%s:%d:%d", collection, time.Now().UnixNano(), i))
		jsonData, err := json.Marshal(item)
		if err != nil {
			return err
		}
		pipe.Set(ctx, key, jsonData, 24*time.Hour)
	}
	
	_, err := pipe.Exec(ctx)
	return err
}

// Query implements core.Storage interface
func (rs *RedisStorage) Query(ctx context.Context, collection string, filter map[string]interface{}) ([]map[string]interface{}, error) {
	// Redis doesn't support complex queries
	// This is a simple implementation that returns empty results
	return []map[string]interface{}{}, nil
}

// getKey returns the full Redis key with prefix
func (rs *RedisStorage) getKey(key string) string {
	if rs.prefix != "" {
		return fmt.Sprintf("%s:%s", rs.prefix, key)
	}
	return key
}