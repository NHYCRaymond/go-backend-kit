package adapter

import (
	"context"
	"log/slog"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/logging"
)

// SlogHandler adapts slog to our logging system
type SlogHandler struct {
	writer logging.Writer
	source string
	attrs  []slog.Attr
	groups []string
}

// NewSlogHandler creates a new slog handler
func NewSlogHandler(writer logging.Writer, source string) *SlogHandler {
	return &SlogHandler{
		writer: writer,
		source: source,
		attrs:  make([]slog.Attr, 0),
		groups: make([]string, 0),
	}
}

// Enabled implements slog.Handler
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return true // Let pipeline filters handle this
}

// Handle implements slog.Handler
func (h *SlogHandler) Handle(ctx context.Context, r slog.Record) error {
	// Convert slog level
	level := h.convertLevel(r.Level)
	
	// Build fields
	fields := make(map[string]interface{})
	
	// Add handler attributes
	for _, attr := range h.attrs {
		fields[attr.Key] = attr.Value.Any()
	}
	
	// Add record attributes
	r.Attrs(func(attr slog.Attr) bool {
		key := attr.Key
		if len(h.groups) > 0 {
			for _, g := range h.groups {
				key = g + "." + key
			}
		}
		fields[key] = attr.Value.Any()
		return true
	})

	// Create log entry
	entry := &logging.LogEntry{
		Timestamp: r.Time,
		Level:     level,
		Source:    h.source,
		Message:   r.Message,
		Fields:    fields,
	}

	// Write through the logging system
	return h.writer.Write(ctx, entry)
}

// WithAttrs implements slog.Handler
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := *h
	newHandler.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &newHandler
}

// WithGroup implements slog.Handler
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	newHandler := *h
	newHandler.groups = append(append([]string{}, h.groups...), name)
	return &newHandler
}

// convertLevel converts slog.Level to logging.Level
func (h *SlogHandler) convertLevel(level slog.Level) logging.Level {
	switch {
	case level < slog.LevelInfo:
		return logging.LevelDebug
	case level < slog.LevelWarn:
		return logging.LevelInfo
	case level < slog.LevelError:
		return logging.LevelWarn
	default:
		return logging.LevelError
	}
}

// LoggerFactory creates configured loggers
type LoggerFactory struct {
	pipeline *logging.Pipeline
	sources  map[string]*slog.Logger
}

// NewLoggerFactory creates a new logger factory
func NewLoggerFactory(pipeline *logging.Pipeline) *LoggerFactory {
	return &LoggerFactory{
		pipeline: pipeline,
		sources:  make(map[string]*slog.Logger),
	}
}

// GetLogger gets or creates a logger for a source
func (f *LoggerFactory) GetLogger(source string) *slog.Logger {
	if logger, exists := f.sources[source]; exists {
		return logger
	}

	// Create writer that goes through pipeline
	writer := &pipelineWriter{
		pipeline: f.pipeline,
		source:   source,
	}

	handler := NewSlogHandler(writer, source)
	logger := slog.New(handler)
	
	f.sources[source] = logger
	return logger
}

// pipelineWriter wraps Pipeline as a Writer
type pipelineWriter struct {
	pipeline *logging.Pipeline
	source   string
}

func (w *pipelineWriter) Write(ctx context.Context, entry *logging.LogEntry) error {
	if entry.Source == "" {
		entry.Source = w.source
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	return w.pipeline.Process(ctx, entry)
}

func (w *pipelineWriter) Close() error {
	return w.pipeline.Close()
}