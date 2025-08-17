package factory

import (
	"context"
	"fmt"
	"strings"
	
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// Factory creates Pipeline and Storage implementations
type Factory struct {
	mongodb *mongo.Database
	mysql   *gorm.DB
	redis   *redis.Client
	
	// Registered pipelines
	pipelines map[string]func() task.Pipeline
	
	// Registered storage backends
	storages map[string]func(config task.StorageConfig) (task.Storage, error)
}

// NewFactory creates a new factory
func NewFactory(mongodb *mongo.Database, mysql *gorm.DB, redis *redis.Client) *Factory {
	f := &Factory{
		mongodb:   mongodb,
		mysql:     mysql,
		redis:     redis,
		pipelines: make(map[string]func() task.Pipeline),
		storages:  make(map[string]func(config task.StorageConfig) (task.Storage, error)),
	}
	
	// Register default implementations
	f.registerDefaults()
	
	return f
}

// registerDefaults registers default implementations
func (f *Factory) registerDefaults() {
	// Register default pipelines
	f.RegisterPipeline("default", func() task.Pipeline {
		return &DefaultPipeline{}
	})
	
	f.RegisterPipeline("validation", func() task.Pipeline {
		return &ValidationPipeline{}
	})
	
	// Register storage backends
	f.RegisterStorage("mongodb", func(config task.StorageConfig) (task.Storage, error) {
		return NewMongoStorage(f.mongodb, config)
	})
	
	f.RegisterStorage("mysql", func(config task.StorageConfig) (task.Storage, error) {
		return NewMySQLStorage(f.mysql, config)
	})
	
	f.RegisterStorage("redis", func(config task.StorageConfig) (task.Storage, error) {
		return NewRedisStorage(f.redis, config)
	})
	
	f.RegisterStorage("file", func(config task.StorageConfig) (task.Storage, error) {
		return NewFileStorage(config)
	})
}

// RegisterPipeline registers a pipeline factory
func (f *Factory) RegisterPipeline(id string, factory func() task.Pipeline) {
	f.pipelines[id] = factory
}

// RegisterStorage registers a storage factory
func (f *Factory) RegisterStorage(typ string, factory func(config task.StorageConfig) (task.Storage, error)) {
	f.storages[typ] = factory
}

// CreatePipeline creates a pipeline by ID
func (f *Factory) CreatePipeline(id string) (task.Pipeline, error) {
	if id == "" {
		id = "default"
	}
	
	factory, exists := f.pipelines[id]
	if !exists {
		return nil, fmt.Errorf("pipeline not found: %s", id)
	}
	
	return factory(), nil
}

// CreateStorage creates a storage by configuration
func (f *Factory) CreateStorage(config task.StorageConfig) (task.Storage, error) {
	factory, exists := f.storages[config.Type]
	if !exists {
		return nil, fmt.Errorf("storage type not supported: %s", config.Type)
	}
	
	return factory(config)
}

// DefaultPipeline is the default pipeline implementation
type DefaultPipeline struct{}

func (p *DefaultPipeline) Process(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	// Default processing - just pass through
	return data, nil
}

func (p *DefaultPipeline) GetID() string {
	return "default"
}

func (p *DefaultPipeline) Validate(data map[string]interface{}) error {
	// No validation by default
	return nil
}

// ValidationPipeline adds validation to the pipeline
type ValidationPipeline struct {
	rules map[string]func(interface{}) error
}

func (p *ValidationPipeline) Process(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	// Validate first
	if err := p.Validate(data); err != nil {
		return nil, err
	}
	
	// Process data
	processed := make(map[string]interface{})
	for k, v := range data {
		// Clean and normalize data
		if str, ok := v.(string); ok {
			// Trim whitespace
			processed[k] = strings.TrimSpace(str)
		} else {
			processed[k] = v
		}
	}
	
	return processed, nil
}

func (p *ValidationPipeline) GetID() string {
	return "validation"
}

func (p *ValidationPipeline) Validate(data map[string]interface{}) error {
	for field, rule := range p.rules {
		if value, exists := data[field]; exists {
			if err := rule(value); err != nil {
				return fmt.Errorf("validation failed for field %s: %w", field, err)
			}
		}
	}
	return nil
}