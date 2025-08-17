package distributed

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// HubConfig represents hub configuration
type HubConfig struct {
	Server       ServerConfig       `mapstructure:"server"`
	Redis        RedisConfig        `mapstructure:"redis"`
	Cluster      ClusterConfiguration `mapstructure:"cluster"`
	Coordinator  CoordinatorConfig  `mapstructure:"coordinator"`
	Dispatcher   DispatcherConfig   `mapstructure:"dispatcher"`
	Communication CommunicationConfig `mapstructure:"communication"`
	Monitoring   MonitoringConfig   `mapstructure:"monitoring"`
	Logging      LoggingConfig      `mapstructure:"logging"`
}

// NodeConfigFile represents node configuration from file
type NodeConfigFile struct {
	Node       NodeSettings      `mapstructure:"node"`
	Hub        HubSettings       `mapstructure:"hub"`
	Redis      RedisConfig       `mapstructure:"redis"`
	Crawler    CrawlerSettings   `mapstructure:"crawler"`
	Fetcher    FetcherSettings   `mapstructure:"fetcher"`
	Extractor  ExtractorSettings `mapstructure:"extractor"`
	Storage    StorageSettings   `mapstructure:"storage"`
	Monitoring MonitoringConfig  `mapstructure:"monitoring"`
	Logging    LoggingConfig     `mapstructure:"logging"`
}

// ServerConfig represents server configuration
type ServerConfig struct {
	GRPCAddr string `mapstructure:"grpc_addr"`
	HTTPAddr string `mapstructure:"http_addr"`
}

// RedisConfig represents Redis configuration
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	Prefix   string `mapstructure:"prefix"`
}

// ClusterConfiguration represents cluster configuration
type ClusterConfiguration struct {
	ClusterID string `mapstructure:"cluster_id"`
	MaxNodes  int    `mapstructure:"max_nodes"`
}

// CommunicationConfig represents communication configuration
type CommunicationConfig struct {
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	HeartbeatTimeout  time.Duration `mapstructure:"heartbeat_timeout"`
	MaxMessageSize    int           `mapstructure:"max_message_size"`
}

// NodeSettings represents node-specific settings
type NodeSettings struct {
	ID           string   `mapstructure:"id"`
	Tags         []string `mapstructure:"tags"`
	Capabilities []string `mapstructure:"capabilities"`
	MaxWorkers   int      `mapstructure:"max_workers"`
	QueueSize    int      `mapstructure:"queue_size"`
}

// HubSettings represents hub connection settings
type HubSettings struct {
	Addr                  string        `mapstructure:"addr"`
	EnableGRPC            bool          `mapstructure:"enable_grpc"`
	ReconnectInterval     time.Duration `mapstructure:"reconnect_interval"`
	MaxReconnectAttempts  int           `mapstructure:"max_reconnect_attempts"`
}

// CrawlerSettings represents crawler settings
type CrawlerSettings struct {
	UserAgent           string        `mapstructure:"user_agent"`
	Timeout             time.Duration `mapstructure:"timeout"`
	MaxRedirects        int           `mapstructure:"max_redirects"`
	RequestsPerSecond   int           `mapstructure:"requests_per_second"`
	ConcurrentRequests  int           `mapstructure:"concurrent_requests"`
}

// FetcherSettings represents fetcher settings
type FetcherSettings struct {
	MaxRetries  int               `mapstructure:"max_retries"`
	RetryDelay  time.Duration     `mapstructure:"retry_delay"`
	ProxyURL    string            `mapstructure:"proxy_url"`
	Headers     map[string]string `mapstructure:"headers"`
}

// ExtractorSettings represents extractor settings
type ExtractorSettings struct {
	DefaultParser      string        `mapstructure:"default_parser"`
	EnableJavaScript   bool          `mapstructure:"enable_javascript"`
	JavaScriptTimeout  time.Duration `mapstructure:"javascript_timeout"`
}

// StorageSettings represents storage settings
type StorageSettings struct {
	Type  string                 `mapstructure:"type"`
	Redis RedisStorageSettings   `mapstructure:"redis"`
}

// RedisStorageSettings represents Redis storage settings
type RedisStorageSettings struct {
	TTL      time.Duration `mapstructure:"ttl"`
	MaxItems int           `mapstructure:"max_items"`
}

// MonitoringConfig represents monitoring configuration
type MonitoringConfig struct {
	MetricsEnabled       bool          `mapstructure:"metrics_enabled"`
	MetricsPort          string        `mapstructure:"metrics_port"`
	ReportInterval       time.Duration `mapstructure:"report_interval"`
	CollectSystemMetrics bool          `mapstructure:"collect_system_metrics"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// DispatcherConfig represents dispatcher configuration (reuse existing)
type DispatcherConfig struct {
	QueueSize         int           `mapstructure:"queue_size"`
	DispatchInterval  time.Duration `mapstructure:"dispatch_interval"`
	MaxTasksPerNode   int           `mapstructure:"max_tasks_per_node"`
}

// LoadHubConfig loads hub configuration from file
func LoadHubConfig(configPath string) (*HubConfig, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	// Set defaults
	v.SetDefault("server.grpc_addr", ":50051")
	v.SetDefault("server.http_addr", ":8080")
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.prefix", "crawler")
	v.SetDefault("cluster.max_nodes", 100)
	v.SetDefault("communication.heartbeat_interval", "5s")
	v.SetDefault("communication.heartbeat_timeout", "30s")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Parse into struct
	var config HubConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// LoadNodeConfig loads node configuration from file
func LoadNodeConfig(configPath string) (*NodeConfigFile, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	// Set defaults
	v.SetDefault("node.max_workers", 10)
	v.SetDefault("node.queue_size", 100)
	v.SetDefault("hub.addr", "localhost:50051")
	v.SetDefault("hub.enable_grpc", true)
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.prefix", "crawler")
	v.SetDefault("crawler.timeout", "30s")
	v.SetDefault("crawler.requests_per_second", 10)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Parse into struct
	var config NodeConfigFile
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// ConvertToNodeConfig converts file config to NodeConfig for backward compatibility
func (c *NodeConfigFile) ConvertToNodeConfig() *NodeConfig {
	return &NodeConfig{
		ID:           c.Node.ID,
		MaxWorkers:   c.Node.MaxWorkers,
		QueueSize:    c.Node.QueueSize,
		Tags:         c.Node.Tags,
		Labels:       make(map[string]string),
		Capabilities: c.Node.Capabilities,
		RedisAddr:    c.Redis.Addr,
		RedisPrefix:  c.Redis.Prefix,
		LogLevel:     c.Logging.Level,
		HubAddr:      c.Hub.Addr,
		EnableGRPC:   c.Hub.EnableGRPC,
	}
}