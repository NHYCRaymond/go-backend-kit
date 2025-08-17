package database

import (
	"sync"
	"sync/atomic"
	"time"
)

// BaseDatabase provides common fields and methods for all database implementations
type BaseDatabase struct {
	connected      bool
	connectionTime time.Duration
	lastError      string
	queryCount     int64
	errorCount     int64
	mutex          sync.RWMutex
}

// IsConnected returns whether the database is connected
func (b *BaseDatabase) IsConnected() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.connected
}

// SetConnected sets the connection status
func (b *BaseDatabase) SetConnected(connected bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.connected = connected
}

// GetConnectionTime returns the connection time
func (b *BaseDatabase) GetConnectionTime() time.Duration {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.connectionTime
}

// SetConnectionTime sets the connection time
func (b *BaseDatabase) SetConnectionTime(duration time.Duration) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.connectionTime = duration
}

// GetLastError returns the last error message
func (b *BaseDatabase) GetLastError() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.lastError
}

// SetLastError sets the last error message
func (b *BaseDatabase) SetLastError(err string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.lastError = err
}

// IncrementQueryCount increments the query count
func (b *BaseDatabase) IncrementQueryCount() {
	atomic.AddInt64(&b.queryCount, 1)
}

// GetQueryCount returns the total query count
func (b *BaseDatabase) GetQueryCount() int64 {
	return atomic.LoadInt64(&b.queryCount)
}

// IncrementErrorCount increments the error count
func (b *BaseDatabase) IncrementErrorCount() {
	atomic.AddInt64(&b.errorCount, 1)
}

// GetErrorCount returns the total error count
func (b *BaseDatabase) GetErrorCount() int64 {
	return atomic.LoadInt64(&b.errorCount)
}

// GetStats returns common statistics
func (b *BaseDatabase) GetStats() map[string]interface{} {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return map[string]interface{}{
		"connected":       b.connected,
		"connection_time": b.connectionTime.String(),
		"query_count":     atomic.LoadInt64(&b.queryCount),
		"error_count":     atomic.LoadInt64(&b.errorCount),
		"last_error":      b.lastError,
	}
}
