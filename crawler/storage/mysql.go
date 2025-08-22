package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/common"
	"github.com/NHYCRaymond/go-backend-kit/database"
	"gorm.io/gorm"
)

// MySQLStorage implements Storage using MySQL with database module
type MySQLStorage struct {
	db       *database.MySQLDatabase
	client   *gorm.DB
	database string
	table    string
	ttl      time.Duration
}

// MySQLRecord represents a record in MySQL storage
type MySQLRecord struct {
	ID        string    `gorm:"primaryKey;column:id"`
	Data      string    `gorm:"type:longtext;column:data"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
	TTL       *time.Time `gorm:"column:ttl;index"`
}

// NewMySQLStorage creates a new MySQL storage using database module
func NewMySQLStorage(db *database.MySQLDatabase, config StorageConfig) (*MySQLStorage, error) {
	// Get MySQL client from database
	clientInterface := db.GetClient()
	if clientInterface == nil {
		return nil, fmt.Errorf("mysql database not connected")
	}
	
	client, ok := clientInterface.(*gorm.DB)
	if !ok {
		return nil, fmt.Errorf("invalid mysql client type")
	}
	
	table := config.Table
	if table == "" {
		table = common.DefaultStorageTable
	}
	
	ttl := time.Duration(config.TTL) * time.Second
	if ttl == 0 {
		ttl = common.DefaultCacheTTL
	}
	
	storage := &MySQLStorage{
		db:       db,
		client:   client,
		database: config.Database,
		table:    table,
		ttl:      ttl,
	}
	
	// Auto migrate table
	if err := storage.autoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate table: %w", err)
	}
	
	// Start cleanup routine for expired records
	go storage.cleanup()
	
	return storage, nil
}

// TableName returns the table name for GORM
func (r *MySQLRecord) TableName() string {
	return common.DefaultStorageTable
}

// autoMigrate creates or updates the table structure
func (ms *MySQLStorage) autoMigrate() error {
	// Set custom table name
	return ms.client.Table(ms.table).AutoMigrate(&MySQLRecord{})
}

// Store stores data with default TTL
func (ms *MySQLStorage) Store(ctx context.Context, key string, value interface{}) error {
	return ms.StoreWithTTL(ctx, key, value, ms.ttl)
}

// StoreWithTTL stores data with specific TTL
func (ms *MySQLStorage) StoreWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Serialize value
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	
	record := MySQLRecord{
		ID:        key,
		Data:      string(data),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	if ttl > 0 {
		expiry := time.Now().Add(ttl)
		record.TTL = &expiry
	}
	
	// Use upsert
	result := ms.client.Table(ms.table).Save(&record)
	
	return result.Error
}

// Get retrieves data by key
func (ms *MySQLStorage) Get(ctx context.Context, key string) (interface{}, error) {
	var record MySQLRecord
	
	result := ms.client.Table(ms.table).
		Where("id = ?", key).
		Where("(ttl IS NULL OR ttl > ?)", time.Now()).
		First(&record)
	
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("key not found: %s", key)
		}
		return nil, result.Error
	}
	
	var value interface{}
	if err := json.Unmarshal([]byte(record.Data), &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %w", err)
	}
	
	return value, nil
}

// Delete removes data by key
func (ms *MySQLStorage) Delete(ctx context.Context, key string) error {
	result := ms.client.Table(ms.table).Delete(&MySQLRecord{}, "id = ?", key)
	return result.Error
}

// Exists checks if key exists
func (ms *MySQLStorage) Exists(ctx context.Context, key string) (bool, error) {
	var count int64
	
	result := ms.client.Table(ms.table).
		Where("id = ?", key).
		Where("(ttl IS NULL OR ttl > ?)", time.Now()).
		Count(&count)
	
	if result.Error != nil {
		return false, result.Error
	}
	
	return count > 0, nil
}

// BatchStore stores multiple items
func (ms *MySQLStorage) BatchStore(ctx context.Context, items map[string]interface{}) error {
	if len(items) == 0 {
		return nil
	}
	
	return ms.client.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var expiry *time.Time
		if ms.ttl > 0 {
			e := now.Add(ms.ttl)
			expiry = &e
		}
		
		for key, value := range items {
			data, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
			}
			
			record := MySQLRecord{
				ID:        key,
				Data:      string(data),
				CreatedAt: now,
				UpdatedAt: now,
				TTL:       expiry,
			}
			
			if err := tx.Table(ms.table).Save(&record).Error; err != nil {
				return err
			}
		}
		
		return nil
	})
}

// BatchGet retrieves multiple items
func (ms *MySQLStorage) BatchGet(ctx context.Context, keys []string) (map[string]interface{}, error) {
	var records []MySQLRecord
	
	result := ms.client.Table(ms.table).
		Where("id IN ?", keys).
		Where("(ttl IS NULL OR ttl > ?)", time.Now()).
		Find(&records)
	
	if result.Error != nil {
		return nil, result.Error
	}
	
	resultMap := make(map[string]interface{})
	for _, record := range records {
		var value interface{}
		if err := json.Unmarshal([]byte(record.Data), &value); err != nil {
			continue
		}
		resultMap[record.ID] = value
	}
	
	return resultMap, nil
}

// Query performs a query
func (ms *MySQLStorage) Query(ctx context.Context, query interface{}) ([]interface{}, error) {
	var records []MySQLRecord
	
	// Support string pattern for LIKE query
	pattern, ok := query.(string)
	if !ok {
		return nil, fmt.Errorf("unsupported query type for MySQL storage")
	}
	
	result := ms.client.Table(ms.table).
		Where("id LIKE ?", pattern+"%").
		Where("(ttl IS NULL OR ttl > ?)", time.Now()).
		Find(&records)
	
	if result.Error != nil {
		return nil, result.Error
	}
	
	var results []interface{}
	for _, record := range records {
		var value interface{}
		if err := json.Unmarshal([]byte(record.Data), &value); err != nil {
			continue
		}
		results = append(results, value)
	}
	
	return results, nil
}

// Count returns the number of stored items
func (ms *MySQLStorage) Count(ctx context.Context) (int64, error) {
	var count int64
	
	result := ms.client.Table(ms.table).
		Where("(ttl IS NULL OR ttl > ?)", time.Now()).
		Count(&count)
	
	if result.Error != nil {
		return 0, result.Error
	}
	
	return count, nil
}

// Clear removes all data from the table
func (ms *MySQLStorage) Clear(ctx context.Context) error {
	result := ms.client.Table(ms.table).Delete(&MySQLRecord{}, "1 = 1")
	return result.Error
}

// Close closes the storage (MySQL connection is managed by database module)
func (ms *MySQLStorage) Close() error {
	// Connection lifecycle is managed by database module
	return nil
}

// HealthCheck performs health check
func (ms *MySQLStorage) HealthCheck(ctx context.Context) error {
	return ms.db.HealthCheck(ctx)
}

// Type returns storage type
func (ms *MySQLStorage) Type() string {
	return "mysql"
}

// cleanup removes expired records periodically
func (ms *MySQLStorage) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		ms.client.Table(ms.table).
			Where("ttl IS NOT NULL AND ttl < ?", time.Now()).
			Delete(&MySQLRecord{})
	}
}