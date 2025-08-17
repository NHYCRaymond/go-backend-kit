package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/go-redis/redis/v8"
)

// Note: Using logging.LogEntry from the logging package for consistency

// LogForwarder forwards logs to Redis Stream
type LogForwarder struct {
	redis      *redis.Client
	nodeID     string
	streamKey  string
	pubsubKey  string
	maxLen     int64
	buffer     chan *logging.LogEntry
	stopCh     chan struct{}
}

// LogForwarderConfig holds configuration for log forwarder
type LogForwarderConfig struct {
	Redis     *redis.Client
	NodeID    string
	Prefix    string
	MaxLen    int64 // Maximum entries in stream (default: 1000)
	BufferSize int  // Buffer size for async forwarding
}

// NewLogForwarder creates a new log forwarder
func NewLogForwarder(config *LogForwarderConfig) *LogForwarder {
	if config.MaxLen == 0 {
		config.MaxLen = 1000
	}
	if config.BufferSize == 0 {
		config.BufferSize = 100
	}
	if config.Prefix == "" {
		config.Prefix = "crawler"
	}

	return &LogForwarder{
		redis:     config.Redis,
		nodeID:    config.NodeID,
		streamKey: fmt.Sprintf("%s:logs:stream", config.Prefix),
		pubsubKey: fmt.Sprintf("%s:logs:pubsub", config.Prefix),
		maxLen:    config.MaxLen,
		buffer:    make(chan *logging.LogEntry, config.BufferSize),
		stopCh:    make(chan struct{}),
	}
}

// Start starts the log forwarder
func (f *LogForwarder) Start(ctx context.Context) {
	go f.processLoop(ctx)
}

// Stop stops the log forwarder
func (f *LogForwarder) Stop() {
	close(f.stopCh)
}

// processLoop processes buffered log entries
func (f *LogForwarder) processLoop(ctx context.Context) {
	for {
		select {
		case entry := <-f.buffer:
			f.forward(ctx, entry)
		case <-f.stopCh:
			// Flush remaining entries
			for len(f.buffer) > 0 {
				select {
				case entry := <-f.buffer:
					f.forward(ctx, entry)
				default:
					return
				}
			}
			return
		case <-ctx.Done():
			return
		}
	}
}

// forward sends log entry to Redis
func (f *LogForwarder) forward(ctx context.Context, entry *logging.LogEntry) {
	// Set entry ID if not provided
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("%s-%d", f.nodeID, time.Now().UnixNano())
	}

	// Prepare fields for Redis Stream
	fields := map[string]interface{}{
		"timestamp": entry.Timestamp.UnixMilli(),  // Use UnixMilli for consistency
		"level":     entry.Level.String(),
		"source":    entry.Source,  // Use Source field instead of node_id
		"message":   entry.Message,
	}

	// Store task_id in fields if present
	if taskID, ok := entry.Fields["task_id"]; ok {
		fields["task_id"] = taskID
	}

	if len(entry.Fields) > 0 {
		fieldsJSON, _ := json.Marshal(entry.Fields)
		fields["fields"] = string(fieldsJSON)
	}

	// Add to Redis Stream with automatic trimming
	_, err := f.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: f.streamKey,
		MaxLen: f.maxLen,
		Approx: true, // Use approximate trimming for better performance
		Values: fields,
	}).Result()

	if err != nil {
		// Log error but don't block
		return
	}

	// Publish to PubSub for real-time subscribers
	data, _ := json.Marshal(entry)
	f.redis.Publish(ctx, f.pubsubKey, data)
}

// Log sends a log entry (async)
func (f *LogForwarder) Log(level, message string, fields map[string]interface{}) {
	entry := &logging.LogEntry{
		Timestamp: time.Now(),
		Level:     logging.ParseLevel(level),
		Source:    f.nodeID,  // Map NodeID to Source field
		Message:   message,
		Fields:    fields,
	}

	select {
	case f.buffer <- entry:
	default:
		// Buffer full, drop log
	}
}

// LogWithTask sends a log entry with task ID
func (f *LogForwarder) LogWithTask(taskID, level, message string, fields map[string]interface{}) {
	// Ensure fields map exists
	if fields == nil {
		fields = make(map[string]interface{})
	}
	// Add task_id to fields
	fields["task_id"] = taskID
	
	entry := &logging.LogEntry{
		Timestamp: time.Now(),
		Level:     logging.ParseLevel(level),
		Source:    f.nodeID,  // Map NodeID to Source field
		Message:   message,
		Fields:    fields,
	}

	select {
	case f.buffer <- entry:
	default:
		// Buffer full, drop log
	}
}

// SlogHandler implements slog.Handler interface for integration
type SlogHandler struct {
	forwarder *LogForwarder
	attrs     []slog.Attr
	groups    []string
}

// NewSlogHandler creates a new slog handler
func NewSlogHandler(forwarder *LogForwarder) *SlogHandler {
	return &SlogHandler{
		forwarder: forwarder,
	}
}

// Enabled implements slog.Handler
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return true // Forward all levels
}

// Handle implements slog.Handler
func (h *SlogHandler) Handle(_ context.Context, r slog.Record) error {
	// Convert slog level to logging.Level
	var level logging.Level
	switch r.Level {
	case slog.LevelDebug:
		level = logging.LevelDebug
	case slog.LevelInfo:
		level = logging.LevelInfo
	case slog.LevelWarn:
		level = logging.LevelWarn
	case slog.LevelError:
		level = logging.LevelError
	default:
		level = logging.LevelInfo
	}

	// Collect attributes
	fields := make(map[string]interface{})
	
	// Add handler attributes
	for _, attr := range h.attrs {
		fields[attr.Key] = attr.Value.Any()
	}

	// Add record attributes
	r.Attrs(func(attr slog.Attr) bool {
		fields[attr.Key] = attr.Value.Any()
		return true
	})

	// Create and send log entry directly
	entry := &logging.LogEntry{
		Timestamp: r.Time,
		Level:     level,
		Source:    h.forwarder.nodeID,
		Message:   r.Message,
		Fields:    fields,
	}

	select {
	case h.forwarder.buffer <- entry:
	default:
		// Buffer full, drop log
	}
	
	return nil
}

// WithAttrs implements slog.Handler
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := *h
	newHandler.attrs = append(newHandler.attrs, attrs...)
	return &newHandler
}

// WithGroup implements slog.Handler
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	newHandler := *h
	newHandler.groups = append(newHandler.groups, name)
	return &newHandler
}