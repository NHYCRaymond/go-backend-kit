package logging

import (
	"context"
	"time"
)

// LogEntry represents a generic log entry
type LogEntry struct {
	ID        string                 `json:"id,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Level     Level                  `json:"level"`
	Source    string                 `json:"source,omitempty"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// Level represents log severity level
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns string representation of log level
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses string to Level
func ParseLevel(s string) Level {
	switch s {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// Writer interface for log output
type Writer interface {
	Write(ctx context.Context, entry *LogEntry) error
	Close() error
}

// Reader interface for log retrieval
type Reader interface {
	Read(ctx context.Context, opts ReadOptions) ([]*LogEntry, error)
	Close() error
}

// Subscriber interface for real-time log subscription
type Subscriber interface {
	Subscribe(ctx context.Context, handler Handler) error
	Unsubscribe() error
}

// Store combines Writer, Reader and Subscriber
type Store interface {
	Writer
	Reader
	Subscriber
}

// Handler processes log entries
type Handler func(entry *LogEntry)

// ReadOptions configures log reading
type ReadOptions struct {
	Limit     int           // Maximum entries to return
	Since     time.Time     // Start time filter
	Until     time.Time     // End time filter
	Level     Level         // Minimum level filter
	Source    string        // Source filter
	Search    string        // Text search
	Ascending bool          // Sort order
}

// Filter processes log entries before writing/reading
type Filter interface {
	Filter(entry *LogEntry) bool
}

// Transformer modifies log entries
type Transformer interface {
	Transform(entry *LogEntry) *LogEntry
}

// Pipeline chains multiple operations
type Pipeline struct {
	filters      []Filter
	transformers []Transformer
	writers      []Writer
}

// NewPipeline creates a new processing pipeline
func NewPipeline() *Pipeline {
	return &Pipeline{
		filters:      make([]Filter, 0),
		transformers: make([]Transformer, 0),
		writers:      make([]Writer, 0),
	}
}

// AddFilter adds a filter to the pipeline
func (p *Pipeline) AddFilter(f Filter) *Pipeline {
	p.filters = append(p.filters, f)
	return p
}

// AddTransformer adds a transformer to the pipeline
func (p *Pipeline) AddTransformer(t Transformer) *Pipeline {
	p.transformers = append(p.transformers, t)
	return p
}

// AddWriter adds a writer to the pipeline
func (p *Pipeline) AddWriter(w Writer) *Pipeline {
	p.writers = append(p.writers, w)
	return p
}

// Process runs the entry through the pipeline
func (p *Pipeline) Process(ctx context.Context, entry *LogEntry) error {
	// Apply filters
	for _, f := range p.filters {
		if !f.Filter(entry) {
			return nil // Entry filtered out
		}
	}

	// Apply transformers
	for _, t := range p.transformers {
		entry = t.Transform(entry)
		if entry == nil {
			return nil // Entry transformed to nil
		}
	}

	// Write to all writers
	for _, w := range p.writers {
		if err := w.Write(ctx, entry); err != nil {
			// Continue writing to other writers even if one fails
			continue
		}
	}

	return nil
}

// Close closes all writers in the pipeline
func (p *Pipeline) Close() error {
	for _, w := range p.writers {
		w.Close()
	}
	return nil
}