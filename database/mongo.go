package database

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type MongoDatabase struct {
	client         *mongo.Client
	config         config.MongoConfig
	connected      bool
	connectionTime time.Duration
	lastError      string
	queryCount     int64
	errorCount     int64
	mutex          sync.RWMutex
}

func NewMongo(cfg config.MongoConfig) *MongoDatabase {
	return &MongoDatabase{
		config: cfg,
	}
}

func (m *MongoDatabase) Connect(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	start := time.Now()

	clientOptions := options.Client().ApplyURI(m.config.URI)

	// Set authentication if provided
	if m.config.User != "" && m.config.Password != "" {
		clientOptions.SetAuth(options.Credential{
			AuthMechanism: m.config.AuthMechanism,
			AuthSource:    "admin",
			Username:      m.config.User,
			Password:      m.config.Password,
		})
	}

	// Set connection timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		m.lastError = err.Error()
		m.errorCount++
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the primary to verify connection
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		m.lastError = err.Error()
		m.errorCount++
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	m.client = client
	m.connected = true
	m.connectionTime = time.Since(start)
	m.lastError = ""

	return nil
}

func (m *MongoDatabase) Disconnect(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.connected || m.client == nil {
		return nil
	}

	if err := m.client.Disconnect(ctx); err != nil {
		m.lastError = err.Error()
		m.errorCount++
		return err
	}

	m.connected = false
	m.client = nil
	return nil
}

func (m *MongoDatabase) HealthCheck(ctx context.Context) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.connected || m.client == nil {
		return fmt.Errorf("MongoDB not connected")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := m.client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("MongoDB ping failed: %w", err)
	}

	return nil
}

func (m *MongoDatabase) GetClient() interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.client
}

func (m *MongoDatabase) Type() DatabaseType {
	return TypeMongoDB
}

func (m *MongoDatabase) Stats() DatabaseStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return DatabaseStats{
		Type:           TypeMongoDB,
		Connected:      m.connected,
		ConnectionTime: m.connectionTime,
		TotalQueries:   m.queryCount,
		ErrorCount:     m.errorCount,
		LastError:      m.lastError,
		// MongoDB driver doesn't expose detailed connection stats
		MaxConnections:    1,
		ActiveConnections: 1,
		IdleConnections:   0,
	}
}

func (m *MongoDatabase) IncrementQueryCount() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.queryCount++
}

func (m *MongoDatabase) IncrementErrorCount() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.errorCount++
}

func (m *MongoDatabase) GetDatabase(dbName string) *mongo.Database {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if m.client == nil {
		return nil
	}
	return m.client.Database(dbName)
}

func (m *MongoDatabase) GetCollection(dbName, collectionName string) *mongo.Collection {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if m.client == nil {
		return nil
	}
	return m.client.Database(dbName).Collection(collectionName)
}

// Legacy function for backward compatibility
func NewMongoLegacy(ctx context.Context, cfg config.MongoConfig) (*mongo.Client, error) {
	mongoDB := NewMongo(cfg)
	if err := mongoDB.Connect(ctx); err != nil {
		return nil, err
	}
	return mongoDB.GetClient().(*mongo.Client), nil
}

// MongoHealthCheck checks MongoDB connection health
func MongoHealthCheck(ctx context.Context, client *mongo.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("MongoDB ping failed: %w", err)
	}

	return nil
}

// GetDatabase returns a database instance
func GetDatabase(client *mongo.Client, dbName string) *mongo.Database {
	return client.Database(dbName)
}

// GetCollection returns a collection instance
func GetCollection(client *mongo.Client, dbName, collectionName string) *mongo.Collection {
	return client.Database(dbName).Collection(collectionName)
}
