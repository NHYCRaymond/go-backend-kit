package database

import (
	"fmt"

	"github.com/NHYCRaymond/go-backend-kit/config"
)

type DefaultDatabaseFactory struct{}

func NewDefaultDatabaseFactory() *DefaultDatabaseFactory {
	return &DefaultDatabaseFactory{}
}

func (f *DefaultDatabaseFactory) CreateDatabase(cfg interface{}) (Database, error) {
	switch config := cfg.(type) {
	case config.MySQLConfig:
		return NewMySQL(config), nil
	case config.RedisConfig:
		return NewRedis(config), nil
	case config.MongoConfig:
		return NewMongo(config), nil
	default:
		return nil, fmt.Errorf("unsupported database config type: %T", cfg)
	}
}

func (f *DefaultDatabaseFactory) SupportedTypes() []DatabaseType {
	return []DatabaseType{
		TypeMySQL,
		TypeRedis,
		TypeMongoDB,
	}
}
