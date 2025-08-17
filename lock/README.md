# Distributed Lock Package

This package provides a Redis-based distributed locking mechanism for Go applications.

## Features

- **Atomic Operations**: Uses Redis Lua scripts for atomic lock acquire/release
- **Lock Extension**: Extend lock expiration time while holding it
- **Auto-Refresh**: Automatically refresh locks for long-running operations
- **Generic Support**: Type-safe wrapper for functions returning values
- **Retry Logic**: Built-in retry mechanism with configurable delays
- **Lock Information**: Query lock status and metadata

## Installation

```bash
go get github.com/yourusername/go-backend-kit
```

## Usage

### Basic Lock/Unlock

```go
import (
    "github.com/yourusername/go-backend-kit/lock"
    "github.com/go-redis/redis/v8"
)

// Create Redis client
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

// Create distributed lock
dl := lock.NewDistributedLock(redisClient, logger)

// Acquire lock
lockResult, err := dl.AcquireLock(ctx, "my-resource", "unique-id", 30*time.Second)
if err != nil {
    // Handle error
}

if !lockResult.Acquired {
    // Lock is held by another process
    return
}

// Do work...

// Release lock
err = dl.ReleaseLock(ctx, lockResult)
```

### Using WithLock Helper

```go
err := dl.WithLock(ctx, "my-resource", 30*time.Second, func(ctx context.Context) error {
    // Critical section code here
    // Lock is automatically released when function returns
    return nil
})
```

### Using Generic WithLockT

```go
result, err := lock.WithLockT(dl, ctx, "my-resource", 30*time.Second, 
    func(ctx context.Context) (string, error) {
        // Critical section that returns a value
        return "result", nil
    })
```

### Auto-Refresh for Long Operations

```go
opts := &lock.LockOptions{
    Expiration:      30 * time.Second,
    RefreshInterval: 10 * time.Second,
    MaxRetries:      3,
    RetryDelay:      100 * time.Millisecond,
}

err := dl.WithAutoRefresh(ctx, "long-task", opts, func(ctx context.Context) error {
    // Long-running operation
    // Lock is automatically refreshed every 10 seconds
    return performLongTask()
})
```

### Try Lock with Retries

```go
lockResult, err := dl.TryLock(ctx, "my-resource", "unique-id", 
    30*time.Second,     // expiration
    5,                  // max retries
    200*time.Millisecond) // retry delay
```

### Check Lock Status

```go
// Check if locked
locked, err := dl.IsLocked(ctx, "my-resource")

// Get lock information
info, err := dl.GetLockInfo(ctx, "my-resource")
// info["exists"] - bool
// info["value"] - string (lock holder)
// info["ttl"] - float64 (seconds remaining)
```

## Lock Key Format

All lock keys are prefixed with `lock:` to avoid conflicts with other Redis keys. For example, if you acquire a lock with key `my-resource`, the actual Redis key will be `lock:my-resource`.

## Best Practices

1. **Always Release Locks**: Use `defer` or the `WithLock` helper to ensure locks are released
2. **Set Appropriate Timeouts**: Choose expiration times based on expected operation duration
3. **Handle Lock Failures**: Always check if lock was acquired before proceeding
4. **Use Unique Values**: Generate unique lock values (e.g., using process ID, timestamp, UUID)
5. **Consider Auto-Refresh**: For long operations, use auto-refresh to prevent premature expiration

## Error Handling

The package returns wrapped errors that can be inspected:

```go
lockResult, err := dl.AcquireLock(ctx, key, value, expiration)
if err != nil {
    // Redis connection error or other issues
    return fmt.Errorf("failed to acquire lock: %w", err)
}

if !lockResult.Acquired {
    // Lock is held by another process (not an error)
    return ErrResourceBusy
}
```

## Testing

The package includes comprehensive tests using miniredis for unit testing:

```bash
go test ./lock/...
```

## Performance Considerations

- Lock operations require network round trips to Redis
- Lua scripts ensure atomicity but add slight overhead
- Auto-refresh creates a background goroutine per lock
- Consider using connection pooling for high-throughput scenarios

## Thread Safety

The `DistributedLock` struct is thread-safe and can be shared across goroutines.