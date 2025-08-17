package storage

import (
	"context"
	"time"
)

// Storage defines the interface for crawler data storage
type Storage interface {
	// Basic operations
	Store(ctx context.Context, key string, value interface{}) error
	StoreWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string) (interface{}, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	
	// Batch operations
	BatchStore(ctx context.Context, items map[string]interface{}) error
	BatchGet(ctx context.Context, keys []string) (map[string]interface{}, error)
	
	// Query operations
	Query(ctx context.Context, query interface{}) ([]interface{}, error)
	Count(ctx context.Context) (int64, error)
	Clear(ctx context.Context) error
	
	// Lifecycle
	Close() error
	HealthCheck(ctx context.Context) error
	Type() string
}

// StorageConfig defines storage configuration
type StorageConfig struct {
	Type     string                 `json:"type" yaml:"type"`         // redis, mongo, mysql, memory
	Database string                 `json:"database" yaml:"database"` // Database name for SQL/MongoDB
	Table    string                 `json:"table" yaml:"table"`       // Table/Collection name
	Prefix   string                 `json:"prefix" yaml:"prefix"`     // Key prefix for Redis
	TTL      time.Duration          `json:"ttl" yaml:"ttl"`           // Default TTL
	Options  map[string]interface{} `json:"options" yaml:"options"`   // Additional options
}

// StorageManager manages multiple storage backends
type StorageManager struct {
	storages map[string]Storage
	primary  string
}

// NewStorageManager creates a new storage manager
func NewStorageManager() *StorageManager {
	return &StorageManager{
		storages: make(map[string]Storage),
	}
}

// AddStorage adds a storage backend
func (sm *StorageManager) AddStorage(name string, storage Storage, isPrimary bool) {
	sm.storages[name] = storage
	if isPrimary {
		sm.primary = name
	}
}

// GetStorage returns a specific storage
func (sm *StorageManager) GetStorage(name string) (Storage, bool) {
	storage, exists := sm.storages[name]
	return storage, exists
}

// GetPrimary returns the primary storage
func (sm *StorageManager) GetPrimary() Storage {
	if sm.primary != "" {
		if storage, exists := sm.storages[sm.primary]; exists {
			return storage
		}
	}
	// Return first storage if no primary set
	for _, storage := range sm.storages {
		return storage
	}
	return nil
}

// CloseAll closes all storage backends
func (sm *StorageManager) CloseAll() error {
	for _, storage := range sm.storages {
		if err := storage.Close(); err != nil {
			return err
		}
	}
	return nil
}

// HealthCheckAll performs health check on all storages
func (sm *StorageManager) HealthCheckAll(ctx context.Context) map[string]error {
	results := make(map[string]error)
	for name, storage := range sm.storages {
		results[name] = storage.HealthCheck(ctx)
	}
	return results
}