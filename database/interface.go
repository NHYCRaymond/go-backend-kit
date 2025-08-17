package database

import (
	"context"
	"time"
)

type Database interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	HealthCheck(ctx context.Context) error
	GetClient() interface{}
	Type() DatabaseType
	Stats() DatabaseStats
}

type DatabaseType string

const (
	TypeMySQL    DatabaseType = "mysql"
	TypeMongoDB  DatabaseType = "mongodb"
	TypeRedis    DatabaseType = "redis"
	TypePostgres DatabaseType = "postgres"
)

type DatabaseStats struct {
	Type              DatabaseType  `json:"type"`
	Connected         bool          `json:"connected"`
	ConnectionTime    time.Duration `json:"connection_time"`
	MaxConnections    int           `json:"max_connections"`
	ActiveConnections int           `json:"active_connections"`
	IdleConnections   int           `json:"idle_connections"`
	TotalQueries      int64         `json:"total_queries"`
	ErrorCount        int64         `json:"error_count"`
	LastError         string        `json:"last_error,omitempty"`
}

type DatabaseFactory interface {
	CreateDatabase(config interface{}) (Database, error)
	SupportedTypes() []DatabaseType
}

type DatabaseManager struct {
	databases map[string]Database
	factory   DatabaseFactory
}

func NewDatabaseManager(factory DatabaseFactory) *DatabaseManager {
	return &DatabaseManager{
		databases: make(map[string]Database),
		factory:   factory,
	}
}

func (dm *DatabaseManager) AddDatabase(name string, config interface{}) error {
	db, err := dm.factory.CreateDatabase(config)
	if err != nil {
		return err
	}
	dm.databases[name] = db
	return nil
}

func (dm *DatabaseManager) GetDatabase(name string) (Database, bool) {
	db, exists := dm.databases[name]
	return db, exists
}

func (dm *DatabaseManager) ConnectAll(ctx context.Context) error {
	for _, db := range dm.databases {
		if err := db.Connect(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (dm *DatabaseManager) DisconnectAll(ctx context.Context) error {
	for _, db := range dm.databases {
		if err := db.Disconnect(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (dm *DatabaseManager) HealthCheckAll(ctx context.Context) map[string]error {
	results := make(map[string]error)
	for name, db := range dm.databases {
		results[name] = db.HealthCheck(ctx)
	}
	return results
}

func (dm *DatabaseManager) GetAllStats() map[string]DatabaseStats {
	stats := make(map[string]DatabaseStats)
	for name, db := range dm.databases {
		stats[name] = db.Stats()
	}
	return stats
}
