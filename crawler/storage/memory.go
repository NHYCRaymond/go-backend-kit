package storage

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStorage implements in-memory storage
type MemoryStorage struct {
	mu     sync.RWMutex
	data   map[string]interface{}
	ttl    map[string]time.Time
	prefix string
}

// NewMemoryStorage creates a new memory storage
func NewMemoryStorage(config StorageConfig) *MemoryStorage {
	storage := &MemoryStorage{
		data:   make(map[string]interface{}),
		ttl:    make(map[string]time.Time),
		prefix: config.Prefix,
	}
	
	// Start cleanup goroutine
	go storage.cleanup()
	
	return storage
}

// Store stores data with default TTL
func (ms *MemoryStorage) Store(ctx context.Context, key string, value interface{}) error {
	return ms.StoreWithTTL(ctx, key, value, 24*time.Hour)
}

// StoreWithTTL stores data with specific TTL
func (ms *MemoryStorage) StoreWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	fullKey := ms.getKey(key)
	ms.data[fullKey] = value
	
	if ttl > 0 {
		ms.ttl[fullKey] = time.Now().Add(ttl)
	}
	
	return nil
}

// Get retrieves data by key
func (ms *MemoryStorage) Get(ctx context.Context, key string) (interface{}, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	fullKey := ms.getKey(key)
	
	// Check if expired
	if expiry, exists := ms.ttl[fullKey]; exists {
		if time.Now().After(expiry) {
			return nil, fmt.Errorf("key expired: %s", key)
		}
	}
	
	value, exists := ms.data[fullKey]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	
	return value, nil
}

// Delete removes data by key
func (ms *MemoryStorage) Delete(ctx context.Context, key string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	fullKey := ms.getKey(key)
	delete(ms.data, fullKey)
	delete(ms.ttl, fullKey)
	
	return nil
}

// Exists checks if key exists
func (ms *MemoryStorage) Exists(ctx context.Context, key string) (bool, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	fullKey := ms.getKey(key)
	
	// Check if expired
	if expiry, exists := ms.ttl[fullKey]; exists {
		if time.Now().After(expiry) {
			return false, nil
		}
	}
	
	_, exists := ms.data[fullKey]
	return exists, nil
}

// BatchStore stores multiple items
func (ms *MemoryStorage) BatchStore(ctx context.Context, items map[string]interface{}) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	defaultExpiry := time.Now().Add(24 * time.Hour)
	for key, value := range items {
		fullKey := ms.getKey(key)
		ms.data[fullKey] = value
		ms.ttl[fullKey] = defaultExpiry
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
		fullKey := ms.getKey(key)
		
		// Check if expired
		if expiry, exists := ms.ttl[fullKey]; exists {
			if now.After(expiry) {
				continue
			}
		}
		
		if value, exists := ms.data[fullKey]; exists {
			result[key] = value
		}
	}
	
	return result, nil
}

// Query performs a query (simple prefix matching for memory storage)
func (ms *MemoryStorage) Query(ctx context.Context, query interface{}) ([]interface{}, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	// Simple pattern matching for memory storage
	pattern, ok := query.(string)
	if !ok {
		return nil, fmt.Errorf("invalid query type for memory storage")
	}
	
	var results []interface{}
	now := time.Now()
	prefix := ms.getKey(pattern)
	
	for key, value := range ms.data {
		// Check if expired
		if expiry, exists := ms.ttl[key]; exists {
			if now.After(expiry) {
				continue
			}
		}
		
		// Simple prefix matching
		if len(prefix) > 0 && len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			results = append(results, value)
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
	
	// Count items without TTL
	for key := range ms.data {
		if _, hasTTL := ms.ttl[key]; !hasTTL {
			count++
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

// Close closes the storage
func (ms *MemoryStorage) Close() error {
	// Nothing to close for memory storage
	return nil
}

// HealthCheck performs health check
func (ms *MemoryStorage) HealthCheck(ctx context.Context) error {
	// Memory storage is always healthy if accessible
	return nil
}

// Type returns storage type
func (ms *MemoryStorage) Type() string {
	return "memory"
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

// getKey returns the full key with prefix
func (ms *MemoryStorage) getKey(key string) string {
	if ms.prefix != "" {
		return fmt.Sprintf("%s:%s", ms.prefix, key)
	}
	return key
}