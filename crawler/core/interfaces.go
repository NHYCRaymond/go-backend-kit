package core

import (
	"context"
	"time"
)

// Task represents a crawler task
type Task interface {
	GetID() string
	GetURL() string
	GetPriority() int
	GetRetryCount() int
	IncrementRetry()
	GetCreatedAt() time.Time
	GetMetadata() map[string]interface{}
	SetMetadata(key string, value interface{})
	Clone() Task
}

// Response represents the response from a fetch operation
type Response struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
	URL        string
	Timestamp  time.Time
	Error      error
}

// Fetcher fetches data from URLs
type Fetcher interface {
	Fetch(ctx context.Context, task Task) (*Response, error)
	SetProxy(proxy string)
	SetUserAgent(userAgent string)
}

// Extractor extracts data from responses
type Extractor interface {
	Extract(response *Response, rules []ExtractRule) ([]map[string]interface{}, error)
	ExtractLinks(response *Response) ([]string, error)
}

// ExtractRule defines how to extract data
type ExtractRule struct {
	Field     string          `yaml:"field" json:"field"`
	Selector  string          `yaml:"selector" json:"selector"`
	Type      string          `yaml:"type" json:"type"` // css, xpath, json, regex
	Attr      string          `yaml:"attr" json:"attr"`
	Multiple  bool            `yaml:"multiple" json:"multiple"`
	Default   interface{}     `yaml:"default" json:"default"`
	Transform []TransformRule `yaml:"transform" json:"transform"`
}

// TransformRule defines data transformation
type TransformRule struct {
	Type   string                 `yaml:"type" json:"type"` // trim, replace, regex, format
	Params map[string]interface{} `yaml:"params" json:"params"`
}

// Storage stores extracted data
type Storage interface {
	Save(ctx context.Context, collection string, data []map[string]interface{}) error
	Query(ctx context.Context, collection string, filter map[string]interface{}) ([]map[string]interface{}, error)
	Close() error
}

// Pipeline processes data through multiple stages
type Pipeline interface {
	Process(ctx context.Context, data []map[string]interface{}) ([]map[string]interface{}, error)
	AddProcessor(processor Processor)
}

// Processor processes data in the pipeline
type Processor interface {
	Name() string
	Process(ctx context.Context, data interface{}) (interface{}, error)
}

// System is the main crawler system interface
type System interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	SubmitTask(task Task) error
	SubmitTasks(tasks []Task) error
	GetMetrics() *SystemMetrics
	GetStatus() SystemStatus
}

// SystemMetrics contains system metrics
type SystemMetrics struct {
	TasksSubmitted  int64     `json:"tasks_submitted"`
	TasksProcessed  int64     `json:"tasks_processed"`
	TasksFailed     int64     `json:"tasks_failed"`
	TasksRetried    int64     `json:"tasks_retried"`
	BytesDownloaded int64     `json:"bytes_downloaded"`
	ItemsExtracted  int64     `json:"items_extracted"`
	ItemsStored     int64     `json:"items_stored"`
	ActiveWorkers   int       `json:"active_workers"`
	QueueSize       int       `json:"queue_size"`
	ErrorRate       float64   `json:"error_rate"`
	AvgResponseTime float64   `json:"avg_response_time"`
	LastUpdated     time.Time `json:"last_updated"`
}

// SystemStatus represents the system status
type SystemStatus string

const (
	StatusIdle     SystemStatus = "idle"
	StatusRunning  SystemStatus = "running"
	StatusPaused   SystemStatus = "paused"
	StatusStopping SystemStatus = "stopping"
	StatusStopped  SystemStatus = "stopped"
	StatusError    SystemStatus = "error"
)

// RateLimiter controls request rate
type RateLimiter interface {
	Wait(ctx context.Context) error
	TryAcquire() bool
	SetRate(requestsPerSecond int)
}

// Deduplicator checks for duplicate URLs
type Deduplicator interface {
	IsDuplicate(url string) bool
	Add(url string) error
	Clear() error
	Size() int
}

// Scheduler schedules tasks
type Scheduler interface {
	Schedule(task Task) error
	GetNext() (Task, error)
	Pause()
	Resume()
	Clear()
}

// Middleware processes tasks before/after fetching
type Middleware interface {
	Name() string
	Process(ctx context.Context, task Task, next MiddlewareFunc) error
}

// MiddlewareFunc is the middleware function type
type MiddlewareFunc func(ctx context.Context, task Task) error
