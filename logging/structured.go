package logging

import (
	"bytes"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// StructuredLoggerConfig holds configuration for structured logging
type StructuredLoggerConfig struct {
	EnableRequestBody  bool
	EnableResponseBody bool
	MaxBodySize        int
	SensitiveFields    []string
	SkipPaths          []string
}

// DefaultStructuredLoggerConfig returns default configuration
func DefaultStructuredLoggerConfig() *StructuredLoggerConfig {
	return &StructuredLoggerConfig{
		EnableRequestBody:  false,
		EnableResponseBody: false,
		MaxBodySize:        4096,
		SensitiveFields: []string{
			"password", "token", "secret", "key", "authorization",
			"credit_card", "phone", "email", "id_card",
		},
		SkipPaths: []string{"/health", "/metrics", "/ready"},
	}
}

// StructuredLoggerMiddleware returns a gin middleware for structured logging
func StructuredLoggerMiddleware(config *StructuredLoggerConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultStructuredLoggerConfig()
	}

	return func(c *gin.Context) {
		// Skip logging for configured paths
		for _, path := range config.SkipPaths {
			if c.Request.URL.Path == path {
				c.Next()
				return
			}
		}

		// Start timer
		start := time.Now()

		// Get request ID from context or header
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = RequestIDFromContext(c.Request.Context())
		}

		// Create logger with request context
		logger := GetLogger().With(
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		)

		// Add logger to context
		c.Request = c.Request.WithContext(WithContext(c.Request.Context(), logger))

		// Log request start
		requestLog := map[string]interface{}{
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"query":      c.Request.URL.RawQuery,
			"ip":         c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
			"request_id": requestID,
		}

		// Log request body if enabled
		if config.EnableRequestBody && shouldLogBody(c.Request.Header.Get("Content-Type")) {
			if body := readBody(c.Request.Body, config.MaxBodySize); body != "" {
				requestLog["body"] = sanitizeBody(body, config.SensitiveFields)
			}
		}

		logger.Info("request started", "request", requestLog)

		// Create response writer wrapper
		writer := &responseWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
			config:         config,
		}
		c.Writer = writer

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Get response status
		status := writer.Status()

		// Determine log level based on status code
		level := getLogLevel(status)

		// Build response log
		responseLog := map[string]interface{}{
			"status":      status,
			"duration_ms": duration.Milliseconds(),
			"size":        writer.Size(),
		}

		// Log response body if enabled
		if config.EnableResponseBody && shouldLogBody(writer.Header().Get("Content-Type")) {
			if body := writer.body.String(); body != "" && len(body) <= config.MaxBodySize {
				responseLog["body"] = sanitizeBody(body, config.SensitiveFields)
			}
		}

		// Add error if exists
		if len(c.Errors) > 0 {
			errors := make([]string, len(c.Errors))
			for i, err := range c.Errors {
				errors[i] = err.Error()
			}
			responseLog["errors"] = errors
		}

		// Log with appropriate level
		message := "request completed"
		args := []any{
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"response", responseLog,
		}

		switch level {
		case "error":
			logger.Error(message, args...)
		case "warn":
			logger.Warn(message, args...)
		default:
			logger.Info(message, args...)
		}
	}
}

// responseWriter wraps gin.ResponseWriter to capture response body
type responseWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	config *StructuredLoggerConfig
}

func (w *responseWriter) Write(data []byte) (int, error) {
	// Write to buffer if we need to log response body
	if w.config.EnableResponseBody && w.body.Len() < w.config.MaxBodySize {
		w.body.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

// readBody reads and returns the request body
func readBody(body io.ReadCloser, maxSize int) string {
	if body == nil {
		return ""
	}

	// Read body
	bodyBytes := make([]byte, maxSize)
	n, _ := io.ReadFull(body, bodyBytes)
	body.Close()

	// Restore body for further processing
	body = io.NopCloser(bytes.NewReader(bodyBytes[:n]))

	return string(bodyBytes[:n])
}

// shouldLogBody determines if body should be logged based on content type
func shouldLogBody(contentType string) bool {
	// Log JSON and form data
	return strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "form") ||
		strings.Contains(contentType, "text")
}

// sanitizeBody removes sensitive information from body
func sanitizeBody(body string, sensitiveFields []string) string {
	// This is a simple implementation
	// In production, use a more sophisticated approach
	result := body
	for _, field := range sensitiveFields {
		// Simple pattern matching - can be improved with regex
		if strings.Contains(strings.ToLower(body), field) {
			// Replace the value after the field
			// This is simplified - a real implementation would be more robust
			result = strings.ReplaceAll(result, field, field+":[REDACTED]")
		}
	}
	return result
}

// getLogLevel returns the appropriate log level based on status code
func getLogLevel(status int) string {
	switch {
	case status >= 500:
		return "error"
	case status >= 400:
		return "warn"
	default:
		return "info"
	}
}
