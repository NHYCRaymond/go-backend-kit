package task

import (
	"context"
	"io"
)

// Pipeline defines the interface for data processing pipeline
type Pipeline interface {
	// Process processes the extracted data through the pipeline
	Process(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)
	
	// GetID returns the pipeline identifier
	GetID() string
	
	// Validate validates the data against pipeline rules
	Validate(data map[string]interface{}) error
}

// Storage defines the interface for data storage
type Storage interface {
	// Save saves the processed data
	Save(ctx context.Context, data interface{}) error
	
	// SaveBatch saves multiple items
	SaveBatch(ctx context.Context, items []interface{}) error
	
	// GetType returns the storage type (mongodb, mysql, redis, file, etc.)
	GetType() string
	
	// Close closes the storage connection
	Close() error
}

// StorageConfig defines storage configuration
type StorageConfig struct {
	Type       string `json:"type"`       // mongodb, mysql, redis, file, s3
	Database   string `json:"database,omitempty"`
	Collection string `json:"collection,omitempty"` // For MongoDB
	Table      string `json:"table,omitempty"`      // For SQL databases
	Bucket     string `json:"bucket,omitempty"`     // For S3
	Path       string `json:"path,omitempty"`       // For file storage
	Format     string `json:"format,omitempty"`     // json, csv, parquet
	
	// Additional configuration
	Options map[string]interface{} `json:"options,omitempty"`
}

// Fetcher defines the interface for fetching content
type Fetcher interface {
	// Fetch fetches content from the given URL
	Fetch(ctx context.Context, task *Task) (*Response, error)
	
	// GetType returns the fetcher type (http, browser, api)
	GetType() string
}

// Response represents the fetch response
type Response struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
	URL        string // Final URL after redirects
	
	// For browser-based fetching
	HTML       string
	Screenshot []byte
	
	// Metadata
	Duration int64 // Response time in milliseconds
	Size     int64 // Content size in bytes
}

// Extractor defines the interface for data extraction
type Extractor interface {
	// Extract extracts data from the response
	Extract(ctx context.Context, response *Response, rules []ExtractRule) (map[string]interface{}, error)
	
	// ExtractLinks extracts links for further crawling
	ExtractLinks(ctx context.Context, response *Response) ([]string, error)
	
	// GetType returns the extractor type (css, xpath, json, regex)
	GetType() string
}

// Executor defines the interface for task execution
type Executor interface {
	// Execute executes the task
	Execute(ctx context.Context, task *Task) (*TaskResult, error)
	
	// SetFetcher sets the fetcher
	SetFetcher(fetcher Fetcher)
	
	// SetExtractor sets the extractor
	SetExtractor(extractor Extractor)
	
	// SetPipeline sets the pipeline
	SetPipeline(pipeline Pipeline)
	
	// SetStorage sets the storage
	SetStorage(storage Storage)
}

// TaskResult represents task execution result
type TaskResult struct {
	TaskID    string                 `json:"task_id"`
	Status    string                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Links     []string               `json:"links,omitempty"`
	Error     string                 `json:"error,omitempty"`
	
	// Metrics
	StartTime   int64 `json:"start_time"`
	EndTime     int64 `json:"end_time"`
	Duration    int64 `json:"duration"` // in milliseconds
	BytesFetched int64 `json:"bytes_fetched"`
	ItemsExtracted int `json:"items_extracted"`
}

// TaskScheduler defines the interface for task scheduling
type TaskScheduler interface {
	// AddTask adds a task to the scheduler
	AddTask(task *Task) error
	
	// GetNextTask gets the next task to execute
	GetNextTask() (*Task, error)
	
	// UpdateTask updates task status
	UpdateTask(task *Task) error
	
	// GetStats returns scheduler statistics
	GetStats() map[string]interface{}
}

// Writer defines the interface for writing output
type Writer interface {
	io.Writer
	io.Closer
	
	// WriteRecord writes a single record
	WriteRecord(record interface{}) error
	
	// WriteBatch writes multiple records
	WriteBatch(records []interface{}) error
	
	// Flush flushes any buffered data
	Flush() error
}