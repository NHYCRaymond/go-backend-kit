package storage

import (
	"context"
	"fmt"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/database"
)

// StorageFactory creates storage instances from configuration
type StorageFactory struct {
	dbManager *database.DatabaseManager
	databases map[string]database.Database
}

// NewStorageFactory creates a new storage factory
func NewStorageFactory() *StorageFactory {
	factory := database.NewDefaultDatabaseFactory()
	manager := database.NewDatabaseManager(factory)
	
	return &StorageFactory{
		dbManager: manager,
		databases: make(map[string]database.Database),
	}
}

// InitFromConfig initializes databases from configuration
func (sf *StorageFactory) InitFromConfig(cfg *config.Config) error {
	ctx := context.Background()
	
	// Initialize Redis if configured
	if cfg.Redis.Address != "" {
		redisDB := database.NewRedis(cfg.Redis)
		if err := redisDB.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect to Redis: %w", err)
		}
		sf.databases["redis"] = redisDB
		sf.dbManager.AddDatabase("redis", cfg.Redis)
	}
	
	// Initialize MongoDB if configured
	if cfg.Mongo.URI != "" {
		mongoDB := database.NewMongo(cfg.Mongo)
		if err := mongoDB.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect to MongoDB: %w", err)
		}
		sf.databases["mongodb"] = mongoDB
		sf.dbManager.AddDatabase("mongodb", cfg.Mongo)
	}
	
	// Initialize MySQL if configured
	if cfg.Database.MySQL.Host != "" {
		mysqlDB := database.NewMySQL(cfg.Database.MySQL)
		if err := mysqlDB.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect to MySQL: %w", err)
		}
		sf.databases["mysql"] = mysqlDB
		sf.dbManager.AddDatabase("mysql", cfg.Database.MySQL)
	}
	
	return nil
}

// CreateStorage creates a storage instance based on configuration
func (sf *StorageFactory) CreateStorage(config StorageConfig) (Storage, error) {
	switch config.Type {
	case "redis":
		db, exists := sf.databases["redis"]
		if !exists {
			return nil, fmt.Errorf("Redis database not initialized")
		}
		redisDB, ok := db.(*database.RedisDatabase)
		if !ok {
			return nil, fmt.Errorf("invalid Redis database type")
		}
		return NewRedisStorage(redisDB, config)
		
	case "mongodb", "mongo":
		db, exists := sf.databases["mongodb"]
		if !exists {
			return nil, fmt.Errorf("MongoDB database not initialized")
		}
		mongoDB, ok := db.(*database.MongoDatabase)
		if !ok {
			return nil, fmt.Errorf("invalid MongoDB database type")
		}
		return NewMongoStorage(mongoDB, config)
		
	case "mysql":
		db, exists := sf.databases["mysql"]
		if !exists {
			return nil, fmt.Errorf("MySQL database not initialized")
		}
		mysqlDB, ok := db.(*database.MySQLDatabase)
		if !ok {
			return nil, fmt.Errorf("invalid MySQL database type")
		}
		return NewMySQLStorage(mysqlDB, config)
		
	case "memory":
		return NewMemoryStorage(config), nil
		
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}

// CreateStorageFromString creates storage with simple string configuration
func (sf *StorageFactory) CreateStorageFromString(storageType string, prefix string) (Storage, error) {
	config := StorageConfig{
		Type:   storageType,
		Prefix: prefix,
	}
	
	// Set default database/table names
	switch storageType {
	case "mongodb", "mongo":
		config.Database = "crawler"
		config.Table = "storage"
	case "mysql":
		config.Database = "crawler"
		config.Table = "crawler_storage"
	}
	
	return sf.CreateStorage(config)
}

// GetDatabase returns a specific database instance
func (sf *StorageFactory) GetDatabase(name string) (database.Database, bool) {
	db, exists := sf.databases[name]
	return db, exists
}

// CloseAll closes all database connections
func (sf *StorageFactory) CloseAll() error {
	ctx := context.Background()
	return sf.dbManager.DisconnectAll(ctx)
}

// HealthCheckAll performs health check on all databases
func (sf *StorageFactory) HealthCheckAll(ctx context.Context) map[string]error {
	return sf.dbManager.HealthCheckAll(ctx)
}

// GetStats returns statistics for all databases
func (sf *StorageFactory) GetStats() map[string]database.DatabaseStats {
	return sf.dbManager.GetAllStats()
}