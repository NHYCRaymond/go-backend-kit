package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// Note: Storage implementations have been moved to crawler/storage package
// This file contains compatibility wrappers and factory-specific storage implementations

// MongoStorage implements Storage for MongoDB
type MongoStorage struct {
	db         *mongo.Database
	collection string
}

// NewMongoStorage creates a new MongoDB storage
func NewMongoStorage(db *mongo.Database, config task.StorageConfig) (task.Storage, error) {
	if config.Collection == "" {
		config.Collection = "crawled_data"
	}
	
	return &MongoStorage{
		db:         db,
		collection: config.Collection,
	}, nil
}

func (s *MongoStorage) Save(ctx context.Context, data interface{}) error {
	coll := s.db.Collection(s.collection)
	
	// Add timestamp
	doc := bson.M{
		"data":       data,
		"created_at": time.Now(),
	}
	
	_, err := coll.InsertOne(ctx, doc)
	return err
}

func (s *MongoStorage) SaveBatch(ctx context.Context, items []interface{}) error {
	if len(items) == 0 {
		return nil
	}
	
	coll := s.db.Collection(s.collection)
	
	// Convert to documents
	docs := make([]interface{}, len(items))
	for i, item := range items {
		docs[i] = bson.M{
			"data":       item,
			"created_at": time.Now(),
		}
	}
	
	_, err := coll.InsertMany(ctx, docs)
	return err
}

func (s *MongoStorage) GetType() string {
	return "mongodb"
}

func (s *MongoStorage) Close() error {
	// MongoDB client is managed externally
	return nil
}

// MySQLStorage implements Storage for MySQL
type MySQLStorage struct {
	db    *gorm.DB
	table string
}

// CrawledData represents a generic crawled data record for MySQL
type CrawledData struct {
	ID        uint      `gorm:"primaryKey"`
	TaskID    string    `gorm:"index"`
	Data      string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"index"`
}

// NewMySQLStorage creates a new MySQL storage
func NewMySQLStorage(db *gorm.DB, config task.StorageConfig) (task.Storage, error) {
	if config.Table == "" {
		config.Table = "crawled_data"
	}
	
	storage := &MySQLStorage{
		db:    db,
		table: config.Table,
	}
	
	// Auto-migrate the table
	if err := db.AutoMigrate(&CrawledData{}); err != nil {
		return nil, err
	}
	
	return storage, nil
}

func (s *MySQLStorage) Save(ctx context.Context, data interface{}) error {
	// Convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	
	record := &CrawledData{
		Data:      string(jsonData),
		CreatedAt: time.Now(),
	}
	
	return s.db.WithContext(ctx).Table(s.table).Create(record).Error
}

func (s *MySQLStorage) SaveBatch(ctx context.Context, items []interface{}) error {
	if len(items) == 0 {
		return nil
	}
	
	records := make([]*CrawledData, len(items))
	for i, item := range items {
		jsonData, err := json.Marshal(item)
		if err != nil {
			return err
		}
		
		records[i] = &CrawledData{
			Data:      string(jsonData),
			CreatedAt: time.Now(),
		}
	}
	
	return s.db.WithContext(ctx).Table(s.table).CreateInBatches(records, 100).Error
}

func (s *MySQLStorage) GetType() string {
	return "mysql"
}

func (s *MySQLStorage) Close() error {
	// DB connection is managed externally
	return nil
}

// RedisStorage implements Storage for Redis
type RedisStorage struct {
	client *redis.Client
	prefix string
}

// NewRedisStorage creates a new Redis storage
func NewRedisStorage(client *redis.Client, config task.StorageConfig) (task.Storage, error) {
	prefix := "crawled"
	if config.Options != nil {
		if p, ok := config.Options["prefix"].(string); ok {
			prefix = p
		}
	}
	
	return &RedisStorage{
		client: client,
		prefix: prefix,
	}, nil
}

func (s *RedisStorage) Save(ctx context.Context, data interface{}) error {
	// Convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	
	// Generate key with timestamp
	key := fmt.Sprintf("%s:%d", s.prefix, time.Now().UnixNano())
	
	// Set with expiration (7 days by default)
	return s.client.Set(ctx, key, jsonData, 7*24*time.Hour).Err()
}

func (s *RedisStorage) SaveBatch(ctx context.Context, items []interface{}) error {
	pipe := s.client.Pipeline()
	
	for _, item := range items {
		jsonData, err := json.Marshal(item)
		if err != nil {
			return err
		}
		
		key := fmt.Sprintf("%s:%d", s.prefix, time.Now().UnixNano())
		pipe.Set(ctx, key, jsonData, 7*24*time.Hour)
	}
	
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisStorage) GetType() string {
	return "redis"
}

func (s *RedisStorage) Close() error {
	// Redis client is managed externally
	return nil
}

// FileStorage implements Storage for file system
type FileStorage struct {
	path   string
	format string
}

// NewFileStorage creates a new file storage
func NewFileStorage(config task.StorageConfig) (task.Storage, error) {
	if config.Path == "" {
		config.Path = "./data"
	}
	
	if config.Format == "" {
		config.Format = "json"
	}
	
	// Create directory if not exists
	if err := os.MkdirAll(config.Path, 0755); err != nil {
		return nil, err
	}
	
	return &FileStorage{
		path:   config.Path,
		format: config.Format,
	}, nil
}

func (s *FileStorage) Save(ctx context.Context, data interface{}) error {
	// Generate filename with timestamp
	filename := fmt.Sprintf("%d.%s", time.Now().UnixNano(), s.format)
	filepath := filepath.Join(s.path, filename)
	
	// Marshal data based on format
	var content []byte
	var err error
	
	switch s.format {
	case "json":
		content, err = json.MarshalIndent(data, "", "  ")
	default:
		return fmt.Errorf("unsupported format: %s", s.format)
	}
	
	if err != nil {
		return err
	}
	
	return os.WriteFile(filepath, content, 0644)
}

func (s *FileStorage) SaveBatch(ctx context.Context, items []interface{}) error {
	// Save batch in a single file
	filename := fmt.Sprintf("batch_%d.%s", time.Now().UnixNano(), s.format)
	filepath := filepath.Join(s.path, filename)
	
	var content []byte
	var err error
	
	switch s.format {
	case "json":
		content, err = json.MarshalIndent(items, "", "  ")
	default:
		return fmt.Errorf("unsupported format: %s", s.format)
	}
	
	if err != nil {
		return err
	}
	
	return os.WriteFile(filepath, content, 0644)
}

func (s *FileStorage) GetType() string {
	return "file"
}

func (s *FileStorage) Close() error {
	// Nothing to close for file storage
	return nil
}