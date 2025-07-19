package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// LoggerMiddleware handles request logging
type LoggerMiddleware struct {
	config config.LoggerConfig
	logger *slog.Logger
}

// NewLoggerMiddleware creates a new logger middleware
func NewLoggerMiddleware(cfg config.LoggerConfig, logger *slog.Logger) *LoggerMiddleware {
	return &LoggerMiddleware{
		config: cfg,
		logger: logger,
	}
}

// Handler returns the middleware handler function
func (l *LoggerMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip logging for configured paths
		for _, path := range l.config.SkipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		// Skip logging for configured methods
		for _, method := range l.config.SkipMethods {
			if c.Request.Method == method {
				c.Next()
				return
			}
		}

		// Start timer
		start := time.Now()

		// Get request ID from context (should be set by RequestID middleware)
		requestID := GetRequestIDFromLogger(c)
		if requestID == "" {
			// Fallback if RequestID middleware not used
			requestID = uuid.New().String()
			c.Set("request_id", requestID)
			c.Header("X-Request-ID", requestID)
		}
		
		// Get trace ID if available
		traceID := GetTraceIDFromLogger(c)

		// Read request body if enabled
		var requestBody []byte
		if l.config.EnableBody && c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

			// Truncate if too large
			if len(requestBody) > l.config.MaxBodySize {
				requestBody = requestBody[:l.config.MaxBodySize]
			}
		}

		// Capture response
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Prepare log fields
		fields := []any{
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", duration.String(),
			"duration_ms", duration.Milliseconds(),
			"ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		}
		
		// Add trace ID if available
		if traceID != "" {
			fields = append(fields, "trace_id", traceID)
		}

		// Add query parameters if any
		if c.Request.URL.RawQuery != "" {
			fields = append(fields, "query", c.Request.URL.RawQuery)
		}

		// Add request body if enabled
		if l.config.EnableBody && len(requestBody) > 0 {
			fields = append(fields, "request_body", string(requestBody))
		}

		// Add response body if enabled
		if l.config.EnableBody && blw.body.Len() > 0 {
			responseBody := blw.body.String()
			if len(responseBody) > l.config.MaxBodySize {
				responseBody = responseBody[:l.config.MaxBodySize]
			}
			fields = append(fields, "response_body", responseBody)
		}

		// Add user ID if available
		if userID, exists := c.Get("user_id"); exists {
			fields = append(fields, "user_id", userID)
		}

		// Sanitize sensitive headers
		headers := make(map[string]string)
		for name, values := range c.Request.Header {
			if l.isSensitiveHeader(name) {
				headers[name] = "[REDACTED]"
			} else {
				headers[name] = strings.Join(values, ",")
			}
		}
		fields = append(fields, "headers", headers)

		// Log based on status code
		statusCode := c.Writer.Status()
		if statusCode >= 500 {
			l.logger.Error("HTTP request completed with server error", fields...)
		} else if statusCode >= 400 {
			l.logger.Warn("HTTP request completed with client error", fields...)
		} else {
			l.logger.Info("HTTP request completed", fields...)
		}
	}
}

// isSensitiveHeader checks if header contains sensitive information
func (l *LoggerMiddleware) isSensitiveHeader(name string) bool {
	name = strings.ToLower(name)
	
	// Default sensitive headers
	sensitiveHeaders := []string{
		"authorization",
		"cookie",
		"x-api-key",
		"x-auth-token",
		"x-access-token",
	}

	// Add configured sensitive headers
	for _, header := range l.config.SensitiveHeaders {
		sensitiveHeaders = append(sensitiveHeaders, strings.ToLower(header))
	}

	for _, sensitive := range sensitiveHeaders {
		if name == sensitive {
			return true
		}
	}

	return false
}

// bodyLogWriter captures response body
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// LogWithRequestID creates a logger with request ID context
func LogWithRequestID(c *gin.Context, logger *slog.Logger) *slog.Logger {
	requestID := GetRequestIDFromLogger(c)
	traceID := GetTraceIDFromLogger(c)
	
	logFields := []any{"request_id", requestID}
	if traceID != "" {
		logFields = append(logFields, "trace_id", traceID)
	}
	
	return logger.With(logFields...)
}

// GetStructuredLogFields returns structured log fields for the request
func GetStructuredLogFields(c *gin.Context) map[string]interface{} {
	fields := map[string]interface{}{
		"request_id": GetRequestIDFromLogger(c),
		"method":     c.Request.Method,
		"path":       c.Request.URL.Path,
		"ip":         c.ClientIP(),
		"user_agent": c.Request.UserAgent(),
	}
	
	if traceID := GetTraceIDFromLogger(c); traceID != "" {
		fields["trace_id"] = traceID
	}
	
	if userID, exists := c.Get("user_id"); exists {
		fields["user_id"] = userID
	}
	
	return fields
}

// GetRequestIDFromLogger gets request ID from context (logger middleware compatible)
func GetRequestIDFromLogger(c *gin.Context) string {
	if requestID, exists := c.Get("request_id"); exists {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return ""
}

// GetTraceIDFromLogger gets trace ID from context (logger middleware compatible)
func GetTraceIDFromLogger(c *gin.Context) string {
	if traceID, exists := c.Get("trace_id"); exists {
		if id, ok := traceID.(string); ok {
			return id
		}
	}
	return ""
}

// GetRequestID is an alias for GetRequestIDFromLogger for backward compatibility
func GetRequestID(c *gin.Context) string {
	return GetRequestIDFromLogger(c)
}

