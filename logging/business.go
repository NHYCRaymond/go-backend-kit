package logging

import (
	"context"
	"log/slog"
	"time"
)

// BusinessLogger provides business-oriented logging methods
type BusinessLogger struct {
	logger *slog.Logger
}

// Biz returns a business logger for the given context
func Biz(ctx context.Context) *BusinessLogger {
	return &BusinessLogger{
		logger: L(ctx),
	}
}

// Operation logs a business operation
func (b *BusinessLogger) Operation(operation string, args ...any) {
	allArgs := append([]any{"operation", operation, "type", "business"}, args...)
	b.logger.Info("business operation", allArgs...)
}

// Success logs a successful business operation
func (b *BusinessLogger) Success(operation string, args ...any) {
	allArgs := append([]any{"operation", operation, "status", "success", "type", "business"}, args...)
	b.logger.Info("business operation succeeded", allArgs...)
}

// Failed logs a failed business operation
func (b *BusinessLogger) Failed(operation string, err error, args ...any) {
	allArgs := append([]any{"operation", operation, "status", "failed", "type", "business", "error", err.Error()}, args...)
	b.logger.Error("business operation failed", allArgs...)
}

// Metric logs a business metric
func (b *BusinessLogger) Metric(metric string, value interface{}, args ...any) {
	allArgs := append([]any{"metric", metric, "value", value, "type", "metric"}, args...)
	b.logger.Info("business metric", allArgs...)
}

// Event logs a business event
func (b *BusinessLogger) Event(event string, args ...any) {
	allArgs := append([]any{"event", event, "type", "event"}, args...)
	b.logger.Info("business event", allArgs...)
}

// LogBusiness logs a structured business log entry
func LogBusiness(ctx context.Context, service, operation, level, message string, data map[string]interface{}) {
	logger := L(ctx)

	args := make([]any, 0, len(data)*2+6)
	args = append(args, "service", service, "operation", operation, "type", "business")

	for k, v := range data {
		args = append(args, k, v)
	}

	switch level {
	case "error":
		logger.Error(message, args...)
	case "warn":
		logger.Warn(message, args...)
	case "debug":
		logger.Debug(message, args...)
	default:
		logger.Info(message, args...)
	}
}

// LogBusinessWithDuration logs a business operation with duration
func LogBusinessWithDuration(ctx context.Context, service, operation string, duration time.Duration, data map[string]interface{}, err error) {
	logger := L(ctx)

	args := make([]any, 0, len(data)*2+10)
	args = append(args,
		"service", service,
		"operation", operation,
		"duration_ms", duration.Milliseconds(),
		"type", "business",
	)

	for k, v := range data {
		args = append(args, k, v)
	}

	if err != nil {
		args = append(args, "error", err.Error(), "status", "failed")
		logger.Error("business operation failed", args...)
	} else {
		args = append(args, "status", "success")
		logger.Info("business operation completed", args...)
	}
}

// OperationLogger provides a fluent interface for logging operations
type OperationLogger struct {
	ctx       context.Context
	operation string
	startTime time.Time
	fields    map[string]interface{}
}

// StartOperation starts logging an operation
func StartOperation(ctx context.Context, operation string) *OperationLogger {
	return &OperationLogger{
		ctx:       ctx,
		operation: operation,
		startTime: time.Now(),
		fields:    make(map[string]interface{}),
	}
}

// WithField adds a field to the operation log
func (ol *OperationLogger) WithField(key string, value interface{}) *OperationLogger {
	ol.fields[key] = value
	return ol
}

// WithFields adds multiple fields to the operation log
func (ol *OperationLogger) WithFields(fields map[string]interface{}) *OperationLogger {
	for k, v := range fields {
		ol.fields[k] = v
	}
	return ol
}

// Success logs the operation as successful
func (ol *OperationLogger) Success(message ...string) {
	duration := time.Since(ol.startTime)

	msg := "operation completed"
	if len(message) > 0 {
		msg = message[0]
	}

	LogBusinessWithDuration(ol.ctx, "", ol.operation, duration, ol.fields, nil)
	L(ol.ctx).Info(msg, "operation", ol.operation, "duration_ms", duration.Milliseconds(), "status", "success")
}

// Failed logs the operation as failed
func (ol *OperationLogger) Failed(err error, message ...string) {
	duration := time.Since(ol.startTime)

	msg := "operation failed"
	if len(message) > 0 {
		msg = message[0]
	}

	LogBusinessWithDuration(ol.ctx, "", ol.operation, duration, ol.fields, err)
	L(ol.ctx).Error(msg, "operation", ol.operation, "duration_ms", duration.Milliseconds(), "status", "failed", "error", err.Error())
}
