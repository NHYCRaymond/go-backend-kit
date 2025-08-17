package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoStorage implements Storage using MongoDB with database module
type MongoStorage struct {
	db         *database.MongoDatabase
	client     *mongo.Client
	database   *mongo.Database
	collection *mongo.Collection
	ttl        time.Duration
}

// MongoDocument wraps stored data with metadata
type MongoDocument struct {
	ID        string      `bson:"_id"`
	Data      interface{} `bson:"data"`
	CreatedAt time.Time   `bson:"created_at"`
	UpdatedAt time.Time   `bson:"updated_at"`
	TTL       *time.Time  `bson:"ttl,omitempty"`
}

// NewMongoStorage creates a new MongoDB storage using database module
func NewMongoStorage(db *database.MongoDatabase, config StorageConfig) (*MongoStorage, error) {
	// Get MongoDB client from database
	clientInterface := db.GetClient()
	if clientInterface == nil {
		return nil, fmt.Errorf("mongodb not connected")
	}
	
	client, ok := clientInterface.(*mongo.Client)
	if !ok {
		return nil, fmt.Errorf("invalid mongodb client type")
	}
	
	// Get database and collection names from config
	dbName := config.Database
	if dbName == "" {
		dbName = "crawler"
	}
	
	collName := config.Table
	if collName == "" {
		collName = "storage"
	}
	
	ttl := config.TTL
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	
	database := client.Database(dbName)
	collection := database.Collection(collName)
	
	// Create TTL index if TTL is configured
	if ttl > 0 {
		indexModel := mongo.IndexModel{
			Keys: bson.D{{Key: "ttl", Value: 1}},
			Options: options.Index().
				SetExpireAfterSeconds(0). // Use the TTL field value
				SetSparse(true),
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		if _, err := collection.Indexes().CreateOne(ctx, indexModel); err != nil {
			// Index might already exist, which is fine
			fmt.Printf("Warning: Failed to create TTL index: %v\n", err)
		}
	}
	
	return &MongoStorage{
		db:         db,
		client:     client,
		database:   database,
		collection: collection,
		ttl:        ttl,
	}, nil
}

// Store stores data with default TTL
func (ms *MongoStorage) Store(ctx context.Context, key string, value interface{}) error {
	return ms.StoreWithTTL(ctx, key, value, ms.ttl)
}

// StoreWithTTL stores data with specific TTL
func (ms *MongoStorage) StoreWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	doc := MongoDocument{
		ID:        key,
		Data:      value,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	if ttl > 0 {
		expiry := time.Now().Add(ttl)
		doc.TTL = &expiry
	}
	
	opts := options.Replace().SetUpsert(true)
	_, err := ms.collection.ReplaceOne(ctx, bson.M{"_id": key}, doc, opts)
	
	return err
}

// Get retrieves data by key
func (ms *MongoStorage) Get(ctx context.Context, key string) (interface{}, error) {
	var doc MongoDocument
	err := ms.collection.FindOne(ctx, bson.M{"_id": key}).Decode(&doc)
	
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("key not found: %s", key)
		}
		return nil, err
	}
	
	return doc.Data, nil
}

// Delete removes data by key
func (ms *MongoStorage) Delete(ctx context.Context, key string) error {
	_, err := ms.collection.DeleteOne(ctx, bson.M{"_id": key})
	return err
}

// Exists checks if key exists
func (ms *MongoStorage) Exists(ctx context.Context, key string) (bool, error) {
	count, err := ms.collection.CountDocuments(ctx, bson.M{"_id": key})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// BatchStore stores multiple items
func (ms *MongoStorage) BatchStore(ctx context.Context, items map[string]interface{}) error {
	if len(items) == 0 {
		return nil
	}
	
	var operations []mongo.WriteModel
	now := time.Now()
	
	for key, value := range items {
		doc := MongoDocument{
			ID:        key,
			Data:      value,
			CreatedAt: now,
			UpdatedAt: now,
		}
		
		if ms.ttl > 0 {
			expiry := now.Add(ms.ttl)
			doc.TTL = &expiry
		}
		
		operation := mongo.NewReplaceOneModel().
			SetFilter(bson.M{"_id": key}).
			SetReplacement(doc).
			SetUpsert(true)
		
		operations = append(operations, operation)
	}
	
	opts := options.BulkWrite().SetOrdered(false)
	_, err := ms.collection.BulkWrite(ctx, operations, opts)
	
	return err
}

// BatchGet retrieves multiple items
func (ms *MongoStorage) BatchGet(ctx context.Context, keys []string) (map[string]interface{}, error) {
	filter := bson.M{"_id": bson.M{"$in": keys}}
	
	cursor, err := ms.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	
	result := make(map[string]interface{})
	
	for cursor.Next(ctx) {
		var doc MongoDocument
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		result[doc.ID] = doc.Data
	}
	
	return result, cursor.Err()
}

// Query performs a MongoDB query
func (ms *MongoStorage) Query(ctx context.Context, query interface{}) ([]interface{}, error) {
	// Support both BSON filter and simple pattern matching
	var filter interface{}
	
	switch q := query.(type) {
	case bson.M:
		filter = q
	case bson.D:
		filter = q
	case string:
		// Simple prefix matching on ID
		filter = bson.M{"_id": bson.M{"$regex": "^" + q}}
	default:
		return nil, fmt.Errorf("unsupported query type: %T", query)
	}
	
	cursor, err := ms.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	
	var results []interface{}
	
	for cursor.Next(ctx) {
		var doc MongoDocument
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		results = append(results, doc.Data)
	}
	
	return results, cursor.Err()
}

// Count returns the number of stored items
func (ms *MongoStorage) Count(ctx context.Context) (int64, error) {
	return ms.collection.CountDocuments(ctx, bson.M{})
}

// Clear removes all data from the collection
func (ms *MongoStorage) Clear(ctx context.Context) error {
	_, err := ms.collection.DeleteMany(ctx, bson.M{})
	return err
}

// Close closes the storage (MongoDB connection is managed by database module)
func (ms *MongoStorage) Close() error {
	// Connection lifecycle is managed by database module
	return nil
}

// HealthCheck performs health check
func (ms *MongoStorage) HealthCheck(ctx context.Context) error {
	return ms.db.HealthCheck(ctx)
}

// Type returns storage type
func (ms *MongoStorage) Type() string {
	return "mongodb"
}