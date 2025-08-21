package compression

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// StorageOptimizer optimizes storage for Redis and MongoDB
type StorageOptimizer struct {
	compressor *Compressor
	redis      *redis.Client
	mongodb    *mongo.Database
	config     *OptimizerConfig
}

// OptimizerConfig contains optimizer configuration
type OptimizerConfig struct {
	// Compression settings
	CompressionType      CompressionType
	CompressionLevel     CompressionLevel
	MinSizeForCompression int // Minimum size in bytes to trigger compression

	// Redis optimization
	EnableRedisCompression bool
	RedisKeyPrefix         string
	
	// MongoDB optimization  
	EnableMongoCompression bool
	MongoCompactKeys       bool // Use short keys for MongoDB documents
	
	// Automatic optimization
	AutoSelectCompression bool // Automatically select best compression based on data
}

// NewStorageOptimizer creates a new storage optimizer
func NewStorageOptimizer(config *OptimizerConfig, redis *redis.Client, mongodb *mongo.Database) (*StorageOptimizer, error) {
	compressor, err := NewCompressor(config.CompressionType, config.CompressionLevel)
	if err != nil {
		return nil, err
	}

	return &StorageOptimizer{
		compressor: compressor,
		redis:      redis,
		mongodb:    mongodb,
		config:     config,
	}, nil
}

// OptimizedRedisSet stores compressed data in Redis
func (so *StorageOptimizer) OptimizedRedisSet(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Marshal to JSON first
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}

	// Check if compression is worth it
	if len(data) < so.config.MinSizeForCompression {
		// Too small, store as-is
		return so.redis.Set(ctx, key, data, ttl).Err()
	}

	// Auto-select best compression if enabled
	if so.config.AutoSelectCompression {
		so.selectBestCompression(data)
	}

	// Compress the data
	compressed, err := so.compressor.CompressJSON(value)
	if err != nil {
		return fmt.Errorf("compression failed: %w", err)
	}

	// Store compression metadata
	metaKey := fmt.Sprintf("%s:meta:%s", so.config.RedisKeyPrefix, key)
	metadata := map[string]interface{}{
		"original_size":   len(data),
		"compressed_size": len(compressed),
		"compression":     so.compressor.compressionType,
		"ratio":           float64(len(compressed)) / float64(len(data)),
	}
	
	// Use pipeline for atomic operation
	pipe := so.redis.Pipeline()
	pipe.Set(ctx, key, compressed, ttl)
	pipe.HSet(ctx, metaKey, metadata)
	pipe.Expire(ctx, metaKey, ttl)
	
	_, err = pipe.Exec(ctx)
	return err
}

// OptimizedRedisGet retrieves and decompresses data from Redis
func (so *StorageOptimizer) OptimizedRedisGet(ctx context.Context, key string, value interface{}) error {
	// Get compressed data
	data, err := so.redis.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}

	// Check if data is compressed (has compression header)
	if len(data) > 0 && data[0] <= byte(CompressionMsgpackZstd) {
		// Decompress
		return so.compressor.DecompressJSON(data, value)
	}

	// Not compressed, unmarshal directly
	return json.Unmarshal(data, value)
}

// OptimizedMongoBatchInsert performs optimized batch insert to MongoDB
func (so *StorageOptimizer) OptimizedMongoBatchInsert(ctx context.Context, collection string, documents []interface{}) error {
	if len(documents) == 0 {
		return nil
	}

	optimizedDocs := make([]interface{}, 0, len(documents))
	
	for _, doc := range documents {
		// Convert to BSON for size calculation
		bsonData, err := bson.Marshal(doc)
		if err != nil {
			continue
		}

		// Optimize if large enough
		if len(bsonData) >= so.config.MinSizeForCompression && so.config.EnableMongoCompression {
			optimized := so.optimizeMongoDocument(doc)
			optimizedDocs = append(optimizedDocs, optimized)
		} else {
			optimizedDocs = append(optimizedDocs, doc)
		}
	}

	// Batch insert
	coll := so.mongodb.Collection(collection)
	_, err := coll.InsertMany(ctx, optimizedDocs)
	return err
}

// optimizeMongoDocument optimizes a MongoDB document
func (so *StorageOptimizer) optimizeMongoDocument(doc interface{}) interface{} {
	// Convert to map for manipulation
	data, err := json.Marshal(doc)
	if err != nil {
		return doc
	}

	var docMap map[string]interface{}
	if err := json.Unmarshal(data, &docMap); err != nil {
		return doc
	}

	// Compact keys if enabled
	if so.config.MongoCompactKeys {
		docMap = so.compactDocumentKeys(docMap)
	}

	// Compress large fields
	for key, value := range docMap {
		if valueBytes, err := json.Marshal(value); err == nil && len(valueBytes) > 1024 {
			// Compress large field values
			if compressed, err := so.compressor.Compress(valueBytes); err == nil {
				docMap[key] = bson.M{
					"_compressed": true,
					"_type":       so.compressor.compressionType,
					"_data":       compressed,
					"_size":       len(valueBytes),
				}
			}
		}
	}

	return docMap
}

// compactDocumentKeys compacts document keys for storage efficiency
func (so *StorageOptimizer) compactDocumentKeys(doc map[string]interface{}) map[string]interface{} {
	compactMap := map[string]string{
		"task_id":      "tid",
		"venue_id":     "vid", 
		"venue_name":   "vn",
		"time_slot":    "ts",
		"status":       "st",
		"price":        "pr",
		"available":    "av",
		"capacity":     "cp",
		"created_at":   "ca",
		"updated_at":   "ua",
		"crawled_at":   "cra",
		"source":       "src",
		"metadata":     "md",
		"raw_data":     "rd",
		"state_md5":    "sm5",
		"error":        "err",
		"result":       "res",
		"data":         "d",
		"parent_id":    "pid",
		"retry_count":  "rc",
		"max_retries":  "mr",
		"priority":     "pri",
		"type":         "typ",
		"url":          "u",
		"method":       "m",
		"headers":      "h",
		"body":         "b",
		"cookies":      "ck",
	}

	compacted := make(map[string]interface{})
	
	for key, value := range doc {
		// Use compact key if available
		compactKey := key
		if short, ok := compactMap[key]; ok {
			compactKey = short
		}

		// Recursively compact nested objects
		switch v := value.(type) {
		case map[string]interface{}:
			compacted[compactKey] = so.compactDocumentKeys(v)
		case []interface{}:
			arr := make([]interface{}, len(v))
			for i, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					arr[i] = so.compactDocumentKeys(m)
				} else {
					arr[i] = item
				}
			}
			compacted[compactKey] = arr
		default:
			compacted[compactKey] = value
		}
	}

	// Add compaction metadata
	compacted["_cpt"] = true // Mark as compacted
	
	return compacted
}

// RestoreDocument restores a compacted/compressed document
func (so *StorageOptimizer) RestoreDocument(doc map[string]interface{}) (map[string]interface{}, error) {
	// Check if document is compacted
	if compacted, ok := doc["_cpt"].(bool); ok && compacted {
		doc = so.expandDocumentKeys(doc)
		delete(doc, "_cpt")
	}

	// Decompress compressed fields
	for key, value := range doc {
		if m, ok := value.(map[string]interface{}); ok {
			if compressed, ok := m["_compressed"].(bool); ok && compressed {
				// Decompress field
				if data, ok := m["_data"].([]byte); ok {
					decompressed, err := so.compressor.Decompress(data)
					if err != nil {
						continue
					}
					
					var restored interface{}
					if err := json.Unmarshal(decompressed, &restored); err != nil {
						continue
					}
					doc[key] = restored
				}
			}
		}
	}

	return doc, nil
}

// expandDocumentKeys expands compacted keys
func (so *StorageOptimizer) expandDocumentKeys(doc map[string]interface{}) map[string]interface{} {
	expandMap := map[string]string{
		"tid":  "task_id",
		"vid":  "venue_id",
		"vn":   "venue_name",
		"ts":   "time_slot",
		"st":   "status",
		"pr":   "price",
		"av":   "available",
		"cp":   "capacity",
		"ca":   "created_at",
		"ua":   "updated_at",
		"cra":  "crawled_at",
		"src":  "source",
		"md":   "metadata",
		"rd":   "raw_data",
		"sm5":  "state_md5",
		"err":  "error",
		"res":  "result",
		"d":    "data",
		"pid":  "parent_id",
		"rc":   "retry_count",
		"mr":   "max_retries",
		"pri":  "priority",
		"typ":  "type",
		"u":    "url",
		"m":    "method",
		"h":    "headers",
		"b":    "body",
		"ck":   "cookies",
	}

	expanded := make(map[string]interface{})
	
	for key, value := range doc {
		// Skip metadata fields
		if key == "_cpt" {
			continue
		}

		// Use expanded key if available
		expandedKey := key
		if full, ok := expandMap[key]; ok {
			expandedKey = full
		}

		// Recursively expand nested objects
		switch v := value.(type) {
		case map[string]interface{}:
			expanded[expandedKey] = so.expandDocumentKeys(v)
		case []interface{}:
			arr := make([]interface{}, len(v))
			for i, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					arr[i] = so.expandDocumentKeys(m)
				} else {
					arr[i] = item
				}
			}
			expanded[expandedKey] = arr
		default:
			expanded[expandedKey] = value
		}
	}

	return expanded
}

// selectBestCompression automatically selects the best compression type
func (so *StorageOptimizer) selectBestCompression(data []byte) {
	ratios := EstimateCompressionRatio(data)
	
	// Find best compression ratio
	bestType := so.config.CompressionType
	bestRatio := 0.0
	
	for name, ratio := range ratios {
		if ratio > bestRatio {
			bestRatio = ratio
			switch name {
			case "gzip":
				bestType = CompressionGzip
			case "zlib":
				bestType = CompressionZlib
			case "zstd":
				bestType = CompressionZstd
			case "msgpack":
				bestType = CompressionMsgpack
			case "msgpack+zstd":
				bestType = CompressionMsgpackZstd
			}
		}
	}

	// Update compressor if different
	if bestType != so.compressor.compressionType {
		if newCompressor, err := NewCompressor(bestType, so.config.CompressionLevel); err == nil {
			so.compressor = newCompressor
		}
	}
}

// GetCompressionStats returns compression statistics
func (so *StorageOptimizer) GetCompressionStats(ctx context.Context) (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"compression_type":  so.compressor.compressionType,
		"compression_level": so.compressor.compressionLevel,
		"min_size":          so.config.MinSizeForCompression,
	}

	// Get Redis compression stats
	pattern := fmt.Sprintf("%s:meta:*", so.config.RedisKeyPrefix)
	keys, err := so.redis.Keys(ctx, pattern).Result()
	if err == nil && len(keys) > 0 {
		totalOriginal := 0
		totalCompressed := 0
		
		for _, key := range keys {
			meta, _ := so.redis.HGetAll(ctx, key).Result()
			if origSize, ok := meta["original_size"]; ok {
				var size int
				fmt.Sscanf(origSize, "%d", &size)
				totalOriginal += size
			}
			if compSize, ok := meta["compressed_size"]; ok {
				var size int
				fmt.Sscanf(compSize, "%d", &size)
				totalCompressed += size
			}
		}

		if totalOriginal > 0 {
			stats["redis_compression_ratio"] = float64(totalCompressed) / float64(totalOriginal)
			stats["redis_space_saved"] = totalOriginal - totalCompressed
			stats["redis_space_saved_mb"] = float64(totalOriginal-totalCompressed) / 1024 / 1024
		}
	}

	return stats, nil
}