package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// DistributedLock provides distributed locking for change detection
type DistributedLock struct {
	redis  *redis.Client
	prefix string
}

// NewDistributedLock creates a new distributed lock
func NewDistributedLock(redis *redis.Client, prefix string) *DistributedLock {
	return &DistributedLock{
		redis:  redis,
		prefix: prefix,
	}
}

// AcquireVenueLock acquires a lock for processing a specific venue
func (dl *DistributedLock) AcquireVenueLock(ctx context.Context, source, timeSlot, venueID string, ttl time.Duration) (string, bool) {
	lockKey := dl.buildVenueLockKey(source, timeSlot, venueID)
	lockValue := uuid.New().String()
	
	// Use SET NX EX for atomic lock acquisition
	ok, err := dl.redis.SetNX(ctx, lockKey, lockValue, ttl).Result()
	if err != nil || !ok {
		return "", false
	}
	
	return lockValue, true
}

// ReleaseVenueLock releases a venue lock
func (dl *DistributedLock) ReleaseVenueLock(ctx context.Context, source, timeSlot, venueID, lockValue string) error {
	lockKey := dl.buildVenueLockKey(source, timeSlot, venueID)
	
	// Use Lua script for atomic release
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	
	_, err := dl.redis.Eval(ctx, script, []string{lockKey}, lockValue).Result()
	return err
}

// AcquireProcessingLock acquires a lock for processing a batch of venues
func (dl *DistributedLock) AcquireProcessingLock(ctx context.Context, taskID string, ttl time.Duration) (string, bool) {
	lockKey := fmt.Sprintf("%s:processing:%s", dl.prefix, taskID)
	lockValue := uuid.New().String()
	
	ok, err := dl.redis.SetNX(ctx, lockKey, lockValue, ttl).Result()
	if err != nil || !ok {
		return "", false
	}
	
	return lockValue, true
}

// ReleaseProcessingLock releases a processing lock
func (dl *DistributedLock) ReleaseProcessingLock(ctx context.Context, taskID, lockValue string) error {
	lockKey := fmt.Sprintf("%s:processing:%s", dl.prefix, taskID)
	
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	
	_, err := dl.redis.Eval(ctx, script, []string{lockKey}, lockValue).Result()
	return err
}

// CheckAndSetLastNotification checks and sets the last notification time to prevent duplicate notifications
func (dl *DistributedLock) CheckAndSetLastNotification(ctx context.Context, source, timeSlot, venueID, stateMD5 string, interval time.Duration) bool {
	notifyKey := dl.buildNotificationKey(source, timeSlot, venueID)
	
	// Atomic check and set with expiration
	script := `
		local key = KEYS[1]
		local new_md5 = ARGV[1]
		local ttl = ARGV[2]
		
		local last_md5 = redis.call("get", key)
		if last_md5 == new_md5 then
			-- Same state already notified recently
			return 0
		else
			-- New state or expired, set and notify
			redis.call("setex", key, ttl, new_md5)
			return 1
		end
	`
	
	result, err := dl.redis.Eval(ctx, script, 
		[]string{notifyKey}, 
		stateMD5, 
		int(interval.Seconds()),
	).Result()
	
	if err != nil {
		return false
	}
	
	return result.(int64) == 1
}

// buildVenueLockKey builds the lock key for a venue
func (dl *DistributedLock) buildVenueLockKey(source, timeSlot, venueID string) string {
	return fmt.Sprintf("%s:lock:venue:%s:%s:%s", dl.prefix, source, timeSlot, venueID)
}

// buildNotificationKey builds the notification tracking key
func (dl *DistributedLock) buildNotificationKey(source, timeSlot, venueID string) string {
	return fmt.Sprintf("%s:notify:%s:%s:%s", dl.prefix, source, timeSlot, venueID)
}

// DeduplicationChecker provides deduplication for change events
type DeduplicationChecker struct {
	redis  *redis.Client
	prefix string
	ttl    time.Duration
}

// NewDeduplicationChecker creates a new deduplication checker
func NewDeduplicationChecker(redis *redis.Client, prefix string, ttl time.Duration) *DeduplicationChecker {
	return &DeduplicationChecker{
		redis:  redis,
		prefix: prefix,
		ttl:    ttl,
	}
}

// IsEventProcessed checks if an event has already been processed
func (dc *DeduplicationChecker) IsEventProcessed(ctx context.Context, eventID string) bool {
	key := fmt.Sprintf("%s:event:processed:%s", dc.prefix, eventID)
	
	exists, err := dc.redis.Exists(ctx, key).Result()
	if err != nil {
		return false
	}
	
	return exists > 0
}

// MarkEventProcessed marks an event as processed
func (dc *DeduplicationChecker) MarkEventProcessed(ctx context.Context, eventID string) error {
	key := fmt.Sprintf("%s:event:processed:%s", dc.prefix, eventID)
	return dc.redis.Set(ctx, key, time.Now().Unix(), dc.ttl).Err()
}

// GenerateEventID generates a unique ID for a change event
func (dc *DeduplicationChecker) GenerateEventID(source, timeSlot, venueID, oldMD5, newMD5 string, timestamp time.Time) string {
	// Include timestamp with minute precision to allow re-notification after some time
	timeKey := timestamp.Truncate(time.Minute).Unix()
	return fmt.Sprintf("%s:%s:%s:%s:%s:%d", source, timeSlot, venueID, oldMD5, newMD5, timeKey)
}