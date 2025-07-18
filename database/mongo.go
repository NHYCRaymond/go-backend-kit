package database

import (
	"context"
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// NewMongo creates a new MongoDB client and connects to the server
func NewMongo(ctx context.Context, cfg config.MongoConfig) (*mongo.Client, error) {
	clientOptions := options.Client().ApplyURI(cfg.URI)

	// Set authentication if provided
	if cfg.User != "" && cfg.Password != "" {
		clientOptions.SetAuth(options.Credential{
			AuthMechanism: cfg.AuthMechanism,
			AuthSource:    "admin",
			Username:      cfg.User,
			Password:      cfg.Password,
		})
	}

	// Set connection timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the primary to verify connection
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	return client, nil
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