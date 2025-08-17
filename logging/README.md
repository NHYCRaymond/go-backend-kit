# Logging Package

A comprehensive structured logging package for Go applications using the standard library's `slog` package (Go 1.21+).

## Features

- **Structured Logging**: Built on Go's `slog` package for structured, performant logging
- **Multiple Outputs**: Support for stdout, file, or both
- **Log Rotation**: Automatic log rotation based on size or daily rotation
- **Context Integration**: Pass logger through context with automatic field enrichment
- **Business Logging**: Specialized methods for business operations and metrics
- **Request Tracking**: Automatic request ID, user ID, and trace ID propagation
- **Sensitive Data Protection**: Automatic redaction of sensitive fields
- **Performance**: Minimal overhead with lazy evaluation

## Installation

```go
import "github.com/yourusername/go-backend-kit/logging"
```

## Configuration

```yaml
logging:
  level: "info"              # debug, info, warn, error
  format: "json"             # json, text
  output: "both"             # stdout, file, both
  file_path: "logs/app.log"
  max_size: 100              # MB
  max_age: 7                 # days
  max_backups: 30            # number of backup files
  compress: true             # compress rotated files
  enable_rotation: true      # enable log rotation
  add_source: false          # add source code location
```

## Usage

### Basic Setup

```go
// Initialize logger
cfg := config.LoggerConfig{
    Level:  "info",
    Format: "json",
    Output: "both",
    FilePath: "logs/app.log",
    EnableRotation: true,
}

if err := logging.InitLogger(cfg); err != nil {
    log.Fatal("Failed to initialize logger:", err)
}

// Use the logger
logger := logging.GetLogger()
logger.Info("Application started")
```

### Context-based Logging

```go
// Add logger to context
ctx := logging.WithContext(context.Background(), logger)

// Use logger from context
logging.Info(ctx, "Processing request", "user_id", userID)

// With request ID
ctx = logging.ContextWithRequestID(ctx, "req-123")
logging.L(ctx).Info("Request processed") // Automatically includes request_id
```

### Structured Logging

```go
// Log with structured fields
logger.Info("user action",
    "action", "login",
    "user_id", 123,
    "ip", "192.168.1.1",
    "duration_ms", 150,
)

// With error
logger.Error("operation failed",
    "operation", "database_query",
    "error", err,
    "query", "SELECT * FROM users",
)
```

### Business Logging

```go
// Business operations
logging.Biz(ctx).Operation("user.registration", "email", email)
logging.Biz(ctx).Success("payment.process", "amount", 99.99, "currency", "USD")
logging.Biz(ctx).Failed("order.create", err, "product_id", productID)

// Business metrics
logging.Biz(ctx).Metric("revenue", 1250.50, "currency", "USD")
logging.Biz(ctx).Event("user.upgraded", "plan", "premium")

// Operation tracking
op := logging.StartOperation(ctx, "process_order")
op.WithField("order_id", orderID).
   WithField("items", len(items))

if err := processOrder(); err != nil {
    op.Failed(err)
} else {
    op.Success()
}
```

### Middleware Integration

```go
// Structured logging middleware for Gin
router := gin.New()
router.Use(logging.StructuredLoggerMiddleware(&logging.StructuredLoggerConfig{
    EnableRequestBody:  false,
    EnableResponseBody: false,
    MaxBodySize:        4096,
    SkipPaths:          []string{"/health", "/metrics"},
}))

// The middleware automatically logs:
// - Request start with method, path, headers
// - Request completion with status, duration
// - Errors if any occur
```

### Daily Log Rotation

```go
// Use daily rotation instead of size-based
rotator, err := logging.NewDailyRotator("logs/app.log", 7) // Keep 7 days
if err != nil {
    log.Fatal(err)
}

// Use with slog
logger := slog.New(slog.NewJSONHandler(rotator, nil))
```

## Log Levels

The package supports four log levels:

- **Debug**: Detailed information for debugging
- **Info**: General informational messages
- **Warn**: Warning messages for potentially harmful situations
- **Error**: Error messages for serious problems

```go
logging.Debug(ctx, "Debug message", "details", data)
logging.Info(ctx, "Info message", "user", userID)
logging.Warn(ctx, "Warning message", "retry", retryCount)
logging.Error(ctx, "Error message", "error", err)
```

## Sensitive Data Protection

The package automatically redacts sensitive fields in logs:

```go
// These fields are automatically redacted:
// password, token, secret, key, authorization, credit_card, phone, email, id_card

logger.Info("user login", 
    "username", "john@example.com",
    "password", "secret123", // Will be logged as "password": "[REDACTED]"
)
```

## Performance Considerations

1. **Lazy Evaluation**: Use `slog.Group` for expensive operations
2. **Sampling**: Consider sampling for high-frequency logs
3. **Async Writing**: File operations are buffered
4. **Context Fields**: Minimize fields stored in context

## Best Practices

1. **Use Structured Fields**: Always use key-value pairs for better searchability
2. **Consistent Keys**: Use consistent field names across your application
3. **Error Handling**: Always log errors with full context
4. **Request Tracking**: Use request IDs for distributed tracing
5. **Business Metrics**: Use business logging for important operations

## Integration Examples

### With HTTP Handler

```go
func MyHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    logger := logging.L(ctx)
    
    logger.Info("handling request",
        "method", r.Method,
        "path", r.URL.Path,
    )
    
    // Your handler logic...
}
```

### With Database Operations

```go
func GetUser(ctx context.Context, userID int) (*User, error) {
    op := logging.StartOperation(ctx, "get_user")
    op.WithField("user_id", userID)
    
    user, err := db.QueryUser(userID)
    if err != nil {
        op.Failed(err)
        return nil, err
    }
    
    op.Success()
    return user, nil
}
```

### With Background Jobs

```go
func ProcessJob(ctx context.Context, job *Job) error {
    logger := logging.L(ctx).With(
        "job_id", job.ID,
        "job_type", job.Type,
    )
    
    logger.Info("starting job")
    
    if err := job.Execute(); err != nil {
        logger.Error("job failed", "error", err)
        return err
    }
    
    logger.Info("job completed", "duration_ms", job.Duration())
    return nil
}
```

## Thread Safety

All logging operations are thread-safe and can be used concurrently from multiple goroutines.