package lock

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*redis.Client, func()) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cleanup := func() {
		client.Close()
		mr.Close()
	}

	return client, cleanup
}

func TestDistributedLock_AcquireAndRelease(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	// Test acquiring a lock
	lock1, err := dl.AcquireLock(ctx, "test-key", "lock1", 5*time.Second)
	assert.NoError(t, err)
	assert.True(t, lock1.Acquired)

	// Test acquiring the same lock should fail
	lock2, err := dl.AcquireLock(ctx, "test-key", "lock2", 5*time.Second)
	assert.NoError(t, err)
	assert.False(t, lock2.Acquired)

	// Test releasing the lock
	err = dl.ReleaseLock(ctx, lock1)
	assert.NoError(t, err)

	// Now acquiring should succeed
	lock3, err := dl.AcquireLock(ctx, "test-key", "lock3", 5*time.Second)
	assert.NoError(t, err)
	assert.True(t, lock3.Acquired)
}

func TestDistributedLock_ExtendLock(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	// Acquire a lock with short expiration
	lock, err := dl.AcquireLock(ctx, "test-key", "lock1", 2*time.Second)
	require.NoError(t, err)
	require.True(t, lock.Acquired)

	// Extend the lock
	err = dl.ExtendLock(ctx, lock, 5*time.Second)
	assert.NoError(t, err)

	// Wait for original expiration
	time.Sleep(3 * time.Second)

	// Lock should still be held
	locked, err := dl.IsLocked(ctx, "test-key")
	assert.NoError(t, err)
	assert.True(t, locked)
}

func TestDistributedLock_WithLock(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	executed := false
	err := dl.WithLock(ctx, "test-key", 5*time.Second, func(ctx context.Context) error {
		executed = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, executed)

	// Lock should be released after function execution
	locked, err := dl.IsLocked(ctx, "test-key")
	assert.NoError(t, err)
	assert.False(t, locked)
}

func TestDistributedLock_WithLockT(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	result, err := WithLockT(dl, ctx, "test-key", 5*time.Second, func(ctx context.Context) (string, error) {
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestDistributedLock_Concurrent(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	counter := 0
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			err := dl.WithLock(ctx, "counter-key", 5*time.Second, func(ctx context.Context) error {
				// Critical section
				mu.Lock()
				counter++
				mu.Unlock()
				time.Sleep(10 * time.Millisecond) // Simulate work
				return nil
			})

			if err != nil {
				t.Logf("Goroutine %d failed to acquire lock: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// Not all goroutines may have acquired the lock due to timeout
	assert.LessOrEqual(t, counter, numGoroutines)
	assert.Greater(t, counter, 0)
}

func TestDistributedLock_TryLock(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	// First lock should succeed immediately
	lock1, err := dl.AcquireLock(ctx, "test-key", "lock1", 5*time.Second)
	require.NoError(t, err)
	require.True(t, lock1.Acquired)

	// Try to acquire with retries
	start := time.Now()
	lock2, err := dl.TryLock(ctx, "test-key", "lock2", 5*time.Second, 3, 100*time.Millisecond)
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Nil(t, lock2)
	assert.Greater(t, elapsed, 300*time.Millisecond)
}

func TestDistributedLock_WithAutoRefresh(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	opts := &LockOptions{
		Expiration:      1 * time.Second,
		MaxRetries:      1,
		RetryDelay:      100 * time.Millisecond,
		RefreshInterval: 300 * time.Millisecond,
	}

	err := dl.WithAutoRefresh(ctx, "test-key", opts, func(ctx context.Context) error {
		// Simulate long-running task
		time.Sleep(2 * time.Second)
		return nil
	})

	assert.NoError(t, err)

	// Lock should be released after function completion
	locked, err := dl.IsLocked(ctx, "test-key")
	assert.NoError(t, err)
	assert.False(t, locked)
}

func TestDistributedLock_GetLockInfo(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	// Get info for non-existent lock
	info, err := dl.GetLockInfo(ctx, "test-key")
	assert.NoError(t, err)
	assert.False(t, info["exists"].(bool))

	// Acquire lock and get info
	lock, err := dl.AcquireLock(ctx, "test-key", "lock1", 5*time.Second)
	require.NoError(t, err)
	require.True(t, lock.Acquired)

	info, err = dl.GetLockInfo(ctx, "test-key")
	assert.NoError(t, err)
	assert.True(t, info["exists"].(bool))
	assert.Equal(t, "lock1", info["value"])
	assert.Greater(t, info["ttl"].(float64), float64(0))
}

func TestDistributedLock_ErrorHandling(t *testing.T) {
	// Test with closed Redis connection
	client, cleanup := setupTestRedis(t)
	cleanup() // Close immediately

	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	// Should return error
	_, err := dl.AcquireLock(ctx, "test-key", "lock1", 5*time.Second)
	assert.Error(t, err)

	// WithLock should also return error
	err = dl.WithLock(ctx, "test-key", 5*time.Second, func(ctx context.Context) error {
		return nil
	})
	assert.Error(t, err)
}

func ExampleDistributedLock_WithLock() {
	// Setup Redis client
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	// Create distributed lock
	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	// Execute critical section with lock
	err := dl.WithLock(ctx, "resource-key", 30*time.Second, func(ctx context.Context) error {
		// Critical section code here
		fmt.Println("Executing critical section")
		return nil
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func ExampleDistributedLock_WithAutoRefresh() {
	// Setup Redis client
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	// Create distributed lock
	dl := NewDistributedLock(client, nil)
	ctx := context.Background()

	// Configure lock with auto-refresh
	opts := &LockOptions{
		Expiration:      30 * time.Second,
		RefreshInterval: 10 * time.Second,
		MaxRetries:      3,
		RetryDelay:      100 * time.Millisecond,
	}

	// Execute long-running task with auto-refresh
	err := dl.WithAutoRefresh(ctx, "long-task", opts, func(ctx context.Context) error {
		// Long-running task
		fmt.Println("Starting long-running task")
		time.Sleep(1 * time.Minute)
		return nil
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
