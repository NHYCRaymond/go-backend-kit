package common

import "time"

// Default network addresses
const (
	DefaultRedisAddr = "localhost:6379"
	DefaultMongoURI  = "mongodb://localhost:27017"
	DefaultHTTPPort  = ":8080"
	DefaultGRPCPort  = ":50051"
	DefaultBindIP    = "127.0.0.1"
)

// Database and collection names
const (
	DefaultTasksCollection = "crawler_tasks"
	DefaultStorageTable    = "crawler_storage"
	DefaultDatabase        = "crawler"
)

// Redis key prefixes and patterns
const (
	DefaultRedisPrefix      = "crawler"
	RedisKeyLogsStream      = "logs:stream"
	RedisKeyLogsPubSub      = "logs:pubsub"
	RedisKeyResults         = "result"
	RedisKeyStats           = "stats"
	RedisKeyQueue           = "queue"
	RedisKeyTasksCompleted  = "tasks:completed"
	RedisKeyStatsCompleted  = "stats:completed_tasks"
	RedisKeyStatsFailed     = "stats:failed_tasks"
	RedisKeyQueuePending    = "queue:pending"
	RedisKeyNodes           = "nodes"
	RedisKeyTaskAssignments = "tasks:assignments"
)

// Timeout values
const (
	DefaultTimeout          = 30 * time.Second
	DefaultTaskTimeout      = 30 * time.Minute
	DefaultHeartbeatTimeout = 5 * time.Second
	DefaultGRPCTimeout      = 10 * time.Second
	DefaultFetchTimeout     = 30 * time.Second
	DefaultShutdownTimeout  = 30 * time.Second
	DefaultReassignInterval = 30 * time.Second
	DefaultWatchInterval    = 30 * time.Second
)

// TTL and retention values
const (
	DefaultCacheTTL        = 24 * time.Hour
	DefaultResultTTL       = 30 * time.Minute
	DefaultRetentionDays   = 7 * 24 * time.Hour
	DefaultCleanupInterval = 10 * time.Minute
)

// Limits and sizes
const (
	DefaultMaxRetries      = 3
	DefaultBatchSize       = 10
	DefaultMaxWorkers      = 10
	DefaultQueueSize       = 100
	DefaultMaxLogEntries   = 1000
	DefaultLogBufferSize   = 100
	DefaultMaxConnections  = 100
	DefaultMaxIdleConns    = 10
	DefaultMaxOpenConns    = 100
	DefaultConnMaxLifetime = 3600
)

// Helper functions to build Redis keys with prefix
func BuildRedisKey(prefix, key string) string {
	if prefix == "" {
		prefix = DefaultRedisPrefix
	}
	return prefix + ":" + key
}

func BuildRedisPattern(prefix, pattern string) string {
	if prefix == "" {
		prefix = DefaultRedisPrefix
	}
	return prefix + ":" + pattern + ":*"
}