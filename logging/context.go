package logging

import (
	"context"
	"log/slog"
)

type contextKey string

const (
	contextKeyLogger    contextKey = "logger"
	contextKeyRequestID contextKey = "request_id"
	contextKeyUserID    contextKey = "user_id"
	contextKeyTraceID   contextKey = "trace_id"
)

// ContextWithRequestID returns a new context with request ID
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, contextKeyRequestID, requestID)
}

// RequestIDFromContext returns request ID from context
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(contextKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// ContextWithUserID returns a new context with user ID
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, contextKeyUserID, userID)
}

// UserIDFromContext returns user ID from context
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(contextKeyUserID).(string); ok {
		return v
	}
	return ""
}

// ContextWithTraceID returns a new context with trace ID
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, contextKeyTraceID, traceID)
}

// TraceIDFromContext returns trace ID from context
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(contextKeyTraceID).(string); ok {
		return v
	}
	return ""
}

// EnrichLogger enriches logger with context values
func EnrichLogger(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = GetLogger()
	}

	// Add request ID
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		logger = logger.With("request_id", requestID)
	}

	// Add user ID
	if userID := UserIDFromContext(ctx); userID != "" {
		logger = logger.With("user_id", userID)
	}

	// Add trace ID
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		logger = logger.With("trace_id", traceID)
	}

	return logger
}

// ContextWithLogger returns a new context with enriched logger
func ContextWithLogger(ctx context.Context) context.Context {
	logger := EnrichLogger(ctx, GetLogger())
	return WithContext(ctx, logger)
}
