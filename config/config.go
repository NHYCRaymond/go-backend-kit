package config

import (
	"os"
	"strconv"

	"github.com/spf13/viper"
)

// ServerConfig holds server specific configurations
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	Env  string `mapstructure:"env"`
}

// MySQLConfig holds MySQL specific configurations
type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
}

// MongoConfig holds MongoDB specific configurations
type MongoConfig struct {
	URI           string `mapstructure:"uri"`
	Database      string `mapstructure:"database"`
	User          string `mapstructure:"user"`
	Password      string `mapstructure:"password"`
	AuthMechanism string `mapstructure:"auth_mechanism"`
}

// RedisConfig holds Redis specific configurations
type RedisConfig struct {
	Address  string `mapstructure:"address"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// RabbitMQConfig holds RabbitMQ specific configurations
type RabbitMQConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Vhost    string `mapstructure:"vhost"`
}

// MonitoringConfig holds monitoring configurations
type MonitoringConfig struct {
	Port int    `mapstructure:"port"`
	Path string `mapstructure:"path"`
}

// JWTConfig holds JWT specific configurations
type JWTConfig struct {
	SecretKey             string `mapstructure:"secret_key"`
	AccessExpirationMins  int    `mapstructure:"access_expiration_mins"`
	RefreshExpirationMins int    `mapstructure:"refresh_expiration_mins"`
}

// LoggerConfig holds logger configurations
type LoggerConfig struct {
	Level            string   `mapstructure:"level"`             // Log level: debug, info, warn, error
	Format           string   `mapstructure:"format"`            // Log format: json, text
	Output           string   `mapstructure:"output"`            // Output: stdout, file, both
	FilePath         string   `mapstructure:"file_path"`         // Log file path
	MaxSize          int      `mapstructure:"max_size"`          // Max size in MB for rotation
	MaxAge           int      `mapstructure:"max_age"`           // Max age in days
	MaxBackups       int      `mapstructure:"max_backups"`       // Max number of backups
	Compress         bool     `mapstructure:"compress"`          // Compress rotated files
	EnableRotation   bool     `mapstructure:"enable_rotation"`   // Enable log rotation
	AddSource        bool     `mapstructure:"add_source"`        // Add source code location
	SkipPaths        []string `mapstructure:"skip_paths"`        // Paths to skip logging
	SkipMethods      []string `mapstructure:"skip_methods"`      // Methods to skip logging
	MaxBodySize      int      `mapstructure:"max_body_size"`     // Max body size to log
	EnableBody       bool     `mapstructure:"enable_body"`       // Enable body logging
	SensitiveHeaders []string `mapstructure:"sensitive_headers"` // Headers to redact
}

// AuthConfig holds auth middleware configuration
type AuthConfig struct {
	SkipPaths       []string `mapstructure:"skip_paths"`
	TokenHeader     string   `mapstructure:"token_header"`
	TokenQueryParam string   `mapstructure:"token_query_param"`
	RequireBearer   bool     `mapstructure:"require_bearer"`
	EnableBlacklist bool     `mapstructure:"enable_blacklist"`
}

// RateLimitConfig holds rate limit middleware configuration
type RateLimitConfig struct {
	Enable       bool     `mapstructure:"enable"`
	Window       string   `mapstructure:"window"`
	Limit        int64    `mapstructure:"limit"`
	SkipPaths    []string `mapstructure:"skip_paths"`
	KeyGenerator string   `mapstructure:"key_generator"`
	Headers      bool     `mapstructure:"headers"`
}

// SecurityConfig holds security middleware configuration
type SecurityConfig struct {
	EnableCORS            bool     `mapstructure:"enable_cors"`
	CORSOrigins           []string `mapstructure:"cors_origins"`
	EnableSecurityHeaders bool     `mapstructure:"enable_security_headers"`
	FrameOptions          string   `mapstructure:"frame_options"`
	ContentTypeOptions    string   `mapstructure:"content_type_options"`
}

// MiddlewareConfig holds middleware configurations
type MiddlewareConfig struct {
	Logger    LoggerConfig    `mapstructure:"logger"`
	Auth      AuthConfig      `mapstructure:"auth"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Security  SecurityConfig  `mapstructure:"security"`
}

// DatabaseConfig holds database configurations
type DatabaseConfig struct {
	MySQL MySQLConfig `mapstructure:"mysql"`
	Mongo MongoConfig `mapstructure:"mongo"`
}

// NodeConfig holds node configuration for distributed crawling
type NodeConfig struct {
	ID           string            `mapstructure:"id"`
	MaxWorkers   int               `mapstructure:"max_workers"`
	QueueSize    int               `mapstructure:"queue_size"`
	Tags         []string          `mapstructure:"tags"`
	Labels       map[string]string `mapstructure:"labels"`
	Capabilities []string          `mapstructure:"capabilities"`
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	Type       string                 `mapstructure:"type"`
	Database   string                 `mapstructure:"database"`
	Table      string                 `mapstructure:"table"`
	Collection string                 `mapstructure:"collection"`
	Prefix     string                 `mapstructure:"prefix"`
	TTL        int                    `mapstructure:"ttl"`
	Options    map[string]interface{} `mapstructure:"options"`
}

// StorageConfigs holds multiple storage configurations
type StorageConfigs struct {
	Primary StorageConfig `mapstructure:"primary"`
	Cache   StorageConfig `mapstructure:"cache"`
	Queue   StorageConfig `mapstructure:"queue"`
}

// CrawlerConfig holds crawler specific configuration
type CrawlerConfig struct {
	RedisPrefix string `mapstructure:"redis_prefix"`
	EnableGRPC  bool   `mapstructure:"enable_grpc"`
	HubAddr     string `mapstructure:"hub_addr"`
}

// Config holds the application configuration
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Mongo      MongoConfig      `mapstructure:"mongo"`
	RabbitMQ   RabbitMQConfig   `mapstructure:"rabbitmq"`
	Monitoring MonitoringConfig `mapstructure:"monitoring"`
	Middleware MiddlewareConfig `mapstructure:"middleware"`
	JWT        JWTConfig        `mapstructure:"jwt"`
	Logger     LoggerConfig     `mapstructure:"logger"`
	Node       NodeConfig       `mapstructure:"node"`
	Storage    StorageConfigs   `mapstructure:"storage"`
	Crawler    CrawlerConfig    `mapstructure:"crawler"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	// Override with environment variables
	overrideFromEnv(&config)

	return &config, nil
}

// LoadFromEnv loads configuration from environment variables only
func LoadFromEnv() *Config {
	config := &Config{}

	// Server config
	if host := os.Getenv("SERVER_HOST"); host != "" {
		config.Server.Host = host
	}
	if port := os.Getenv("SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.Port = p
		}
	}
	if env := os.Getenv("SERVER_ENV"); env != "" {
		config.Server.Env = env
	}

	// Database config
	if host := os.Getenv("MYSQL_HOST"); host != "" {
		config.Database.MySQL.Host = host
	}
	if port := os.Getenv("MYSQL_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Database.MySQL.Port = p
		}
	}
	if user := os.Getenv("MYSQL_USER"); user != "" {
		config.Database.MySQL.User = user
	}
	if password := os.Getenv("MYSQL_PASSWORD"); password != "" {
		config.Database.MySQL.Password = password
	}
	if dbname := os.Getenv("MYSQL_DBNAME"); dbname != "" {
		config.Database.MySQL.DBName = dbname
	}

	// Redis config
	if addr := os.Getenv("REDIS_ADDRESS"); addr != "" {
		config.Redis.Address = addr
	}
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		config.Redis.Password = password
	}
	if db := os.Getenv("REDIS_DB"); db != "" {
		if d, err := strconv.Atoi(db); err == nil {
			config.Redis.DB = d
		}
	}

	// MongoDB config
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		config.Database.Mongo.URI = uri
	}
	if database := os.Getenv("MONGO_DATABASE"); database != "" {
		config.Database.Mongo.Database = database
	}

	// RabbitMQ config
	if host := os.Getenv("RABBITMQ_HOST"); host != "" {
		config.RabbitMQ.Host = host
	}
	if port := os.Getenv("RABBITMQ_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.RabbitMQ.Port = p
		}
	}
	if user := os.Getenv("RABBITMQ_USER"); user != "" {
		config.RabbitMQ.Username = user
	}
	if password := os.Getenv("RABBITMQ_PASSWORD"); password != "" {
		config.RabbitMQ.Password = password
	}

	// JWT config
	if secret := os.Getenv("JWT_SECRET_KEY"); secret != "" {
		config.JWT.SecretKey = secret
	}

	return config
}

// overrideFromEnv overrides configuration with environment variables
func overrideFromEnv(config *Config) {
	// Server overrides
	if host := os.Getenv("SERVER_HOST"); host != "" {
		config.Server.Host = host
	}
	if port := os.Getenv("SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.Port = p
		}
	}

	// Database overrides
	if host := os.Getenv("MYSQL_HOST"); host != "" {
		config.Database.MySQL.Host = host
	}
	if port := os.Getenv("MYSQL_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Database.MySQL.Port = p
		}
	}
	if user := os.Getenv("MYSQL_USER"); user != "" {
		config.Database.MySQL.User = user
	}
	if password := os.Getenv("MYSQL_PASSWORD"); password != "" {
		config.Database.MySQL.Password = password
	}
	if dbname := os.Getenv("MYSQL_DBNAME"); dbname != "" {
		config.Database.MySQL.DBName = dbname
	}

	// Redis overrides
	if addr := os.Getenv("REDIS_ADDRESS"); addr != "" {
		config.Redis.Address = addr
	}
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		config.Redis.Password = password
	}
	if db := os.Getenv("REDIS_DB"); db != "" {
		if d, err := strconv.Atoi(db); err == nil {
			config.Redis.DB = d
		}
	}

	// MongoDB overrides
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		config.Database.Mongo.URI = uri
	}
	if database := os.Getenv("MONGO_DATABASE"); database != "" {
		config.Database.Mongo.Database = database
	}

	// RabbitMQ overrides
	if host := os.Getenv("RABBITMQ_HOST"); host != "" {
		config.RabbitMQ.Host = host
	}
	if port := os.Getenv("RABBITMQ_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.RabbitMQ.Port = p
		}
	}
	if user := os.Getenv("RABBITMQ_USER"); user != "" {
		config.RabbitMQ.Username = user
	}
	if password := os.Getenv("RABBITMQ_PASSWORD"); password != "" {
		config.RabbitMQ.Password = password
	}

	// JWT overrides
	if secret := os.Getenv("JWT_SECRET_KEY"); secret != "" {
		config.JWT.SecretKey = secret
	}
}
