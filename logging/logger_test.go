package logging

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitLogger(t *testing.T) {
	tests := []struct {
		name   string
		config config.LoggerConfig
		verify func(t *testing.T)
	}{
		{
			name: "JSON format to stdout",
			config: config.LoggerConfig{
				Level:  "info",
				Format: "json",
				Output: "stdout",
			},
			verify: func(t *testing.T) {
				logger := GetLogger()
				assert.NotNil(t, logger)
			},
		},
		{
			name: "Text format to file",
			config: config.LoggerConfig{
				Level:    "debug",
				Format:   "text",
				Output:   "file",
				FilePath: "test.log",
			},
			verify: func(t *testing.T) {
				logger := GetLogger()
				assert.NotNil(t, logger)
				// Clean up
				os.Remove("test.log")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InitLogger(tt.config)
			require.NoError(t, err)
			tt.verify(t)
		})
	}
}

func TestLogLevels(t *testing.T) {
	// Initialize logger
	err := InitLogger(config.LoggerConfig{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Test different log levels
	Debug(ctx, "debug message", "key", "value")
	Info(ctx, "info message", "key", "value")
	Warn(ctx, "warn message", "key", "value")
	Error(ctx, "error message", "key", "value")
}

func TestContextLogging(t *testing.T) {
	err := InitLogger(config.LoggerConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Add request ID
	ctx = ContextWithRequestID(ctx, "req-123")
	assert.Equal(t, "req-123", RequestIDFromContext(ctx))

	// Add user ID
	ctx = ContextWithUserID(ctx, "user-456")
	assert.Equal(t, "user-456", UserIDFromContext(ctx))

	// Add trace ID
	ctx = ContextWithTraceID(ctx, "trace-789")
	assert.Equal(t, "trace-789", TraceIDFromContext(ctx))

	// Enrich logger
	logger := EnrichLogger(ctx, nil)
	assert.NotNil(t, logger)
}

func TestBusinessLogging(t *testing.T) {
	err := InitLogger(config.LoggerConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})
	require.NoError(t, err)

	ctx := context.Background()
	biz := Biz(ctx)

	// Test business operations
	biz.Operation("user.login", "user_id", "123")
	biz.Success("payment.process", "amount", 100.50)
	biz.Failed("order.create", assert.AnError, "order_id", "456")
	biz.Metric("revenue", 1000.00, "currency", "USD")
	biz.Event("user.signup", "plan", "premium")
}

func TestOperationLogger(t *testing.T) {
	err := InitLogger(config.LoggerConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Test successful operation
	op := StartOperation(ctx, "test_operation")
	op.WithField("key", "value").
		WithFields(map[string]interface{}{
			"count": 10,
			"type":  "test",
		})

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	op.Success("Operation completed successfully")

	// Test failed operation
	op2 := StartOperation(ctx, "failing_operation")
	op2.Failed(assert.AnError, "Operation failed")
}

func TestLoggerWithFields(t *testing.T) {
	err := InitLogger(config.LoggerConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})
	require.NoError(t, err)

	logger := GetLogger()

	// Test with fields
	enriched := WithFields(logger, "service", "api", "version", "1.0")
	enriched = WithRequestID(enriched, "req-123")
	enriched = WithUserID(enriched, "user-456")
	enriched = WithError(enriched, assert.AnError)

	assert.NotNil(t, enriched)
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"debug", "DEBUG"},
		{"DEBUG", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"warning", "WARN"},
		{"error", "ERROR"},
		{"invalid", "INFO"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLogLevel(tt.input)
			assert.Equal(t, tt.expected, level.String())
		})
	}
}

func TestSensitiveDataRedaction(t *testing.T) {
	// This would require capturing log output
	// For now, just test the function exists
	assert.NotNil(t, sanitizeBody)
}

func BenchmarkLogging(b *testing.B) {
	err := InitLogger(config.LoggerConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})
	require.NoError(b, err)

	ctx := context.Background()
	logger := L(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark log message",
			"iteration", i,
			"timestamp", time.Now(),
			"data", map[string]interface{}{
				"key1": "value1",
				"key2": 12345,
			},
		)
	}
}

func ExampleLogger() {
	// Initialize logger
	InitLogger(config.LoggerConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})

	ctx := context.Background()

	// Basic logging
	Info(ctx, "Application started", "version", "1.0.0")

	// With context
	ctx = ContextWithRequestID(ctx, "req-123")
	L(ctx).Info("Processing request")

	// Business logging
	Biz(ctx).Operation("user.login", "user_id", "123")

	// Operation tracking
	op := StartOperation(ctx, "process_order")
	op.WithField("order_id", "456").Success()
}
