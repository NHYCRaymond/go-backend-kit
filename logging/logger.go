package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	defaultLogger *slog.Logger
	loggerMutex   sync.RWMutex
)

// InitLogger initializes the global logger with the given configuration
func InitLogger(cfg config.LoggerConfig) error {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()

	// Parse log level
	level := parseLogLevel(cfg.Level)

	// Create handler options
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Customize log attributes if needed
			if a.Key == slog.SourceKey {
				source, _ := a.Value.Any().(*slog.Source)
				if source != nil {
					source.File = filepath.Base(source.File)
				}
			}
			return a
		},
	}

	// Create output writer
	writer, err := createWriter(cfg)
	if err != nil {
		return fmt.Errorf("failed to create log writer: %w", err)
	}

	// Create handler based on format
	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(writer, opts)
	default:
		handler = slog.NewTextHandler(writer, opts)
	}

	// Create logger
	defaultLogger = slog.New(handler)

	// Set as default logger
	slog.SetDefault(defaultLogger)

	return nil
}

// GetLogger returns the default logger
func GetLogger() *slog.Logger {
	loggerMutex.RLock()
	defer loggerMutex.RUnlock()

	if defaultLogger == nil {
		// Return a basic logger if not initialized
		return slog.Default()
	}
	return defaultLogger
}

// L returns a logger from context or the default logger
func L(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(contextKeyLogger).(*slog.Logger); ok {
			return logger
		}
	}
	return GetLogger()
}

// WithContext returns a new context with the logger
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKeyLogger, logger)
}

// FromContext returns the logger from context
func FromContext(ctx context.Context) *slog.Logger {
	return L(ctx)
}

// WithFields returns a logger with additional fields
func WithFields(logger *slog.Logger, fields ...any) *slog.Logger {
	return logger.With(fields...)
}

// WithRequestID returns a logger with request ID
func WithRequestID(logger *slog.Logger, requestID string) *slog.Logger {
	return logger.With("request_id", requestID)
}

// WithUserID returns a logger with user ID
func WithUserID(logger *slog.Logger, userID string) *slog.Logger {
	return logger.With("user_id", userID)
}

// WithError returns a logger with error
func WithError(logger *slog.Logger, err error) *slog.Logger {
	return logger.With("error", err.Error())
}

// parseLogLevel parses string log level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// createWriter creates the output writer based on configuration
func createWriter(cfg config.LoggerConfig) (io.Writer, error) {
	writers := make([]io.Writer, 0)

	output := strings.ToLower(cfg.Output)

	// Add stdout writer
	if output == "stdout" || output == "both" {
		writers = append(writers, os.Stdout)
	}

	// Add file writer
	if output == "file" || output == "both" {
		if cfg.FilePath == "" {
			cfg.FilePath = "logs/app.log"
		}

		// Create log directory
		dir := filepath.Dir(cfg.FilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		// Create file writer with rotation
		if cfg.EnableRotation {
			fileWriter := &lumberjack.Logger{
				Filename:   cfg.FilePath,
				MaxSize:    cfg.MaxSize,    // MB
				MaxAge:     cfg.MaxAge,     // days
				MaxBackups: cfg.MaxBackups, // files
				LocalTime:  true,
				Compress:   cfg.Compress,
			}
			writers = append(writers, fileWriter)
		} else {
			// Simple file writer without rotation
			file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return nil, fmt.Errorf("failed to open log file: %w", err)
			}
			writers = append(writers, file)
		}
	}

	if len(writers) == 0 {
		return os.Stdout, nil
	}

	if len(writers) == 1 {
		return writers[0], nil
	}

	return io.MultiWriter(writers...), nil
}

// Convenience functions for logging

// Debug logs a debug message
func Debug(ctx context.Context, msg string, args ...any) {
	L(ctx).Debug(msg, args...)
}

// Info logs an info message
func Info(ctx context.Context, msg string, args ...any) {
	L(ctx).Info(msg, args...)
}

// Warn logs a warning message
func Warn(ctx context.Context, msg string, args ...any) {
	L(ctx).Warn(msg, args...)
}

// Error logs an error message
func Error(ctx context.Context, msg string, args ...any) {
	L(ctx).Error(msg, args...)
}

// Fatal logs a fatal message and exits
func Fatal(ctx context.Context, msg string, args ...any) {
	L(ctx).Error(msg, args...)
	os.Exit(1)
}

// LogWithCaller logs a message with caller information
func LogWithCaller(ctx context.Context, level slog.Level, msg string, args ...any) {
	logger := L(ctx)

	// Get caller information
	_, file, line, ok := runtime.Caller(2)
	if ok {
		file = filepath.Base(file)
		logger = logger.With("caller", fmt.Sprintf("%s:%d", file, line))
	}

	switch level {
	case slog.LevelDebug:
		logger.Debug(msg, args...)
	case slog.LevelInfo:
		logger.Info(msg, args...)
	case slog.LevelWarn:
		logger.Warn(msg, args...)
	case slog.LevelError:
		logger.Error(msg, args...)
	}
}
