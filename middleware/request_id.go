package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// RequestIDKey is the key used to store request ID in context
	RequestIDKey = "request_id"

	// RequestIDHeader is the header name for request ID
	RequestIDHeader = "X-Request-ID"

	// TraceIDHeader is the header name for trace ID (for distributed tracing)
	TraceIDHeader = "X-Trace-ID"
)

// RequestIDConfig holds configuration for request ID middleware
type RequestIDConfig struct {
	// Generator is the function to generate request ID
	Generator func() string

	// HeaderName is the header name for request ID
	HeaderName string

	// ContextKey is the key used to store request ID in context
	ContextKey string

	// SkipPaths are paths that should not generate request ID
	SkipPaths []string

	// EnableTraceID enables distributed tracing ID
	EnableTraceID bool

	// TraceIDGenerator is the function to generate trace ID
	TraceIDGenerator func() string
}

// DefaultRequestIDConfig returns default configuration
func DefaultRequestIDConfig() *RequestIDConfig {
	return &RequestIDConfig{
		Generator:        generateUUID,
		HeaderName:       RequestIDHeader,
		ContextKey:       RequestIDKey,
		SkipPaths:        []string{"/health", "/metrics", "/ready", "/live"},
		EnableTraceID:    true,
		TraceIDGenerator: generateTraceID,
	}
}

// RequestIDMiddleware returns a middleware that generates and tracks request IDs
func RequestIDMiddleware(config *RequestIDConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultRequestIDConfig()
	}

	return func(c *gin.Context) {
		// Skip for certain paths
		for _, path := range config.SkipPaths {
			if c.Request.URL.Path == path {
				c.Next()
				return
			}
		}

		// Get request ID from header or generate new one
		requestID := c.GetHeader(config.HeaderName)
		if requestID == "" {
			requestID = config.Generator()
		}

		// Generate trace ID if enabled
		var traceID string
		if config.EnableTraceID {
			traceID = c.GetHeader(TraceIDHeader)
			if traceID == "" {
				traceID = config.TraceIDGenerator()
			}
		}

		// Store in context
		c.Set(config.ContextKey, requestID)
		if config.EnableTraceID {
			c.Set("trace_id", traceID)
		}

		// Set response headers
		c.Header(config.HeaderName, requestID)
		if config.EnableTraceID {
			c.Header(TraceIDHeader, traceID)
		}

		// Create context with request ID for downstream services
		ctx := context.WithValue(c.Request.Context(), config.ContextKey, requestID)
		if config.EnableTraceID {
			ctx = context.WithValue(ctx, "trace_id", traceID)
		}
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// generateUUID generates a UUID v4 request ID
func generateUUID() string {
	return uuid.New().String()
}

// generateTraceID generates a trace ID for distributed tracing
func generateTraceID() string {
	// Generate 16 bytes for trace ID (compatible with OpenTelemetry)
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// generateShortID generates a shorter request ID (8 characters)
func generateShortID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%08x", time.Now().Unix())
	}
	return hex.EncodeToString(bytes)
}

// generateTimestampID generates a timestamp-based request ID
func generateTimestampID() string {
	return fmt.Sprintf("%d-%s", time.Now().Unix(), generateShortID())
}

// GetRequestIDFromGin extracts request ID from Gin context
func GetRequestIDFromGin(c *gin.Context) string {
	if requestID, exists := c.Get(RequestIDKey); exists {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return ""
}

// GetTraceIDFromGin extracts trace ID from Gin context
func GetTraceIDFromGin(c *gin.Context) string {
	if traceID, exists := c.Get("trace_id"); exists {
		if id, ok := traceID.(string); ok {
			return id
		}
	}
	return ""
}

// GetRequestIDFromContext extracts request ID from standard context
func GetRequestIDFromContext(ctx context.Context) string {
	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return ""
}

// GetTraceIDFromContext extracts trace ID from standard context
func GetTraceIDFromContext(ctx context.Context) string {
	if traceID := ctx.Value("trace_id"); traceID != nil {
		if id, ok := traceID.(string); ok {
			return id
		}
	}
	return ""
}

// WithRequestID creates a new context with request ID
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// WithTraceID creates a new context with trace ID
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, "trace_id", traceID)
}

// RequestInfo holds request information for logging
type RequestInfo struct {
	RequestID string    `json:"request_id"`
	TraceID   string    `json:"trace_id,omitempty"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`
	Duration  int64     `json:"duration_ms,omitempty"`
	Status    int       `json:"status,omitempty"`
}

// GetRequestInfo extracts complete request information from context
func GetRequestInfo(c *gin.Context) RequestInfo {
	info := RequestInfo{
		RequestID: GetRequestIDFromGin(c),
		TraceID:   GetTraceIDFromGin(c),
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
		UserAgent: c.Request.UserAgent(),
		IP:        c.ClientIP(),
	}

	// Get start time from context if available
	if startTime, exists := c.Get("start_time"); exists {
		if t, ok := startTime.(time.Time); ok {
			info.StartTime = t
		}
	}

	return info
}

// RequestTracker tracks request lifecycle
type RequestTracker struct {
	requests map[string]*RequestInfo
	config   *RequestIDConfig
}

// NewRequestTracker creates a new request tracker
func NewRequestTracker(config *RequestIDConfig) *RequestTracker {
	if config == nil {
		config = DefaultRequestIDConfig()
	}

	return &RequestTracker{
		requests: make(map[string]*RequestInfo),
		config:   config,
	}
}

// TrackRequest tracks a request by its ID
func (rt *RequestTracker) TrackRequest(c *gin.Context) {
	requestID := GetRequestIDFromGin(c)
	if requestID == "" {
		return
	}

	info := GetRequestInfo(c)
	info.StartTime = time.Now()

	rt.requests[requestID] = &info

	// Store start time in context for later use
	c.Set("start_time", info.StartTime)
}

// FinishRequest marks a request as finished
func (rt *RequestTracker) FinishRequest(c *gin.Context) {
	requestID := GetRequestIDFromGin(c)
	if requestID == "" {
		return
	}

	if info, exists := rt.requests[requestID]; exists {
		info.EndTime = time.Now()
		info.Duration = info.EndTime.Sub(info.StartTime).Milliseconds()
		info.Status = c.Writer.Status()

		// Clean up after some time (optional)
		go func() {
			time.Sleep(5 * time.Minute)
			delete(rt.requests, requestID)
		}()
	}
}

// GetRequestByID retrieves request information by ID
func (rt *RequestTracker) GetRequestByID(requestID string) (*RequestInfo, bool) {
	info, exists := rt.requests[requestID]
	return info, exists
}

// GetAllActiveRequests returns all currently active requests
func (rt *RequestTracker) GetAllActiveRequests() map[string]*RequestInfo {
	result := make(map[string]*RequestInfo)
	for k, v := range rt.requests {
		if v.EndTime.IsZero() { // Still active
			result[k] = v
		}
	}
	return result
}

// RequestLifecycleMiddleware combines request ID generation with lifecycle tracking
func RequestLifecycleMiddleware(config *RequestIDConfig) gin.HandlerFunc {
	tracker := NewRequestTracker(config)

	return func(c *gin.Context) {
		// Generate request ID first
		RequestIDMiddleware(config)(c)

		// Track request start
		tracker.TrackRequest(c)

		// Process request
		c.Next()

		// Track request finish
		tracker.FinishRequest(c)
	}
}
