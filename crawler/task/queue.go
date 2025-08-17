package task

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// QueueType defines queue types
type QueueType string

const (
	QueueTypeMemory QueueType = "memory"
	QueueTypeRedis  QueueType = "redis"
)

// Queue interface for task queue
type Queue interface {
	// Push adds a task to the queue
	Push(ctx context.Context, task *Task) error
	// PushBatch adds multiple tasks
	PushBatch(ctx context.Context, tasks []*Task) error
	// Pop retrieves and removes a task from the queue
	Pop(ctx context.Context) (*Task, error)
	// PopN retrieves and removes N tasks from the queue
	PopN(ctx context.Context, n int) ([]*Task, error)
	// Peek retrieves but doesn't remove a task
	Peek(ctx context.Context) (*Task, error)
	// Size returns queue size
	Size(ctx context.Context) (int64, error)
	// Clear removes all tasks
	Clear(ctx context.Context) error
	// Stats returns queue statistics
	Stats(ctx context.Context) (*QueueStats, error)
}

// QueueStats contains queue statistics
type QueueStats struct {
	Total      int64              `json:"total"`
	ByStatus   map[Status]int64   `json:"by_status"`
	ByPriority map[Priority]int64 `json:"by_priority"`
	ByType     map[Type]int64     `json:"by_type"`
	OldestTask time.Time          `json:"oldest_task"`
	NewestTask time.Time          `json:"newest_task"`
}

// PriorityQueue manages tasks with priority
type PriorityQueue struct {
	mu      sync.RWMutex
	queues  map[Priority]*MemoryQueue
	counter int64
}

// NewPriorityQueue creates a new priority queue
func NewPriorityQueue() *PriorityQueue {
	return &PriorityQueue{
		queues: map[Priority]*MemoryQueue{
			PriorityUrgent: NewMemoryQueue(1000),
			PriorityHigh:   NewMemoryQueue(1000),
			PriorityNormal: NewMemoryQueue(1000),
			PriorityLow:    NewMemoryQueue(1000),
		},
	}
}

// Push adds a task to the appropriate priority queue
func (pq *PriorityQueue) Push(ctx context.Context, task *Task) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	queue, ok := pq.queues[Priority(task.Priority)]
	if !ok {
		queue = NewMemoryQueue(1000)
		pq.queues[Priority(task.Priority)] = queue
	}

	task.Queue()
	pq.counter++
	return queue.Push(ctx, task)
}

// PushBatch adds multiple tasks
func (pq *PriorityQueue) PushBatch(ctx context.Context, tasks []*Task) error {
	for _, task := range tasks {
		if err := pq.Push(ctx, task); err != nil {
			return err
		}
	}
	return nil
}

// Pop retrieves highest priority task
func (pq *PriorityQueue) Pop(ctx context.Context) (*Task, error) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Check queues in priority order
	priorities := []Priority{PriorityUrgent, PriorityHigh, PriorityNormal, PriorityLow}

	for _, priority := range priorities {
		queue := pq.queues[priority]
		if queue != nil {
			task, err := queue.Pop(ctx)
			if err == nil && task != nil {
				pq.counter--
				return task, nil
			}
		}
	}

	return nil, fmt.Errorf("no tasks available")
}

// PopN retrieves N highest priority tasks
func (pq *PriorityQueue) PopN(ctx context.Context, n int) ([]*Task, error) {
	var tasks []*Task
	for i := 0; i < n; i++ {
		task, err := pq.Pop(ctx)
		if err != nil {
			break
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// Peek retrieves but doesn't remove the highest priority task
func (pq *PriorityQueue) Peek(ctx context.Context) (*Task, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	priorities := []Priority{PriorityUrgent, PriorityHigh, PriorityNormal, PriorityLow}

	for _, priority := range priorities {
		queue := pq.queues[priority]
		if queue != nil {
			task, err := queue.Peek(ctx)
			if err == nil && task != nil {
				return task, nil
			}
		}
	}

	return nil, fmt.Errorf("no tasks available")
}

// Size returns total queue size
func (pq *PriorityQueue) Size(ctx context.Context) (int64, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return pq.counter, nil
}

// Clear removes all tasks
func (pq *PriorityQueue) Clear(ctx context.Context) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for _, queue := range pq.queues {
		if err := queue.Clear(ctx); err != nil {
			return err
		}
	}
	pq.counter = 0
	return nil
}

// Stats returns queue statistics
func (pq *PriorityQueue) Stats(ctx context.Context) (*QueueStats, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	stats := &QueueStats{
		Total:      pq.counter,
		ByPriority: make(map[Priority]int64),
		ByStatus:   make(map[Status]int64),
		ByType:     make(map[Type]int64),
	}

	for priority, queue := range pq.queues {
		size, _ := queue.Size(ctx)
		stats.ByPriority[priority] = size
	}

	return stats, nil
}

// MemoryQueue is an in-memory task queue
type MemoryQueue struct {
	mu       sync.RWMutex
	tasks    []*Task
	capacity int
}

// NewMemoryQueue creates a new memory queue
func NewMemoryQueue(capacity int) *MemoryQueue {
	return &MemoryQueue{
		tasks:    make([]*Task, 0),
		capacity: capacity,
	}
}

// Push adds a task to the queue
func (mq *MemoryQueue) Push(ctx context.Context, task *Task) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.tasks) >= mq.capacity {
		return fmt.Errorf("queue is full")
	}

	task.Queue()
	mq.tasks = append(mq.tasks, task)
	return nil
}

// PushBatch adds multiple tasks
func (mq *MemoryQueue) PushBatch(ctx context.Context, tasks []*Task) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.tasks)+len(tasks) > mq.capacity {
		return fmt.Errorf("not enough capacity")
	}

	for _, task := range tasks {
		task.Queue()
		mq.tasks = append(mq.tasks, task)
	}
	return nil
}

// Pop retrieves and removes a task
func (mq *MemoryQueue) Pop(ctx context.Context) (*Task, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.tasks) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	task := mq.tasks[0]
	mq.tasks = mq.tasks[1:]
	return task, nil
}

// PopN retrieves and removes N tasks
func (mq *MemoryQueue) PopN(ctx context.Context, n int) ([]*Task, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.tasks) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	if n > len(mq.tasks) {
		n = len(mq.tasks)
	}

	tasks := mq.tasks[:n]
	mq.tasks = mq.tasks[n:]
	return tasks, nil
}

// Peek retrieves but doesn't remove a task
func (mq *MemoryQueue) Peek(ctx context.Context) (*Task, error) {
	mq.mu.RLock()
	defer mq.mu.RUnlock()

	if len(mq.tasks) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	return mq.tasks[0], nil
}

// Size returns queue size
func (mq *MemoryQueue) Size(ctx context.Context) (int64, error) {
	mq.mu.RLock()
	defer mq.mu.RUnlock()
	return int64(len(mq.tasks)), nil
}

// Clear removes all tasks
func (mq *MemoryQueue) Clear(ctx context.Context) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	mq.tasks = make([]*Task, 0)
	return nil
}

// Stats returns queue statistics
func (mq *MemoryQueue) Stats(ctx context.Context) (*QueueStats, error) {
	mq.mu.RLock()
	defer mq.mu.RUnlock()

	stats := &QueueStats{
		Total:      int64(len(mq.tasks)),
		ByStatus:   make(map[Status]int64),
		ByPriority: make(map[Priority]int64),
		ByType:     make(map[Type]int64),
	}

	for _, task := range mq.tasks {
		stats.ByStatus[task.Status]++
		stats.ByPriority[Priority(task.Priority)]++
		stats.ByType[Type(task.Type)]++

		if stats.OldestTask.IsZero() || task.CreatedAt.Before(stats.OldestTask) {
			stats.OldestTask = task.CreatedAt
		}
		if task.CreatedAt.After(stats.NewestTask) {
			stats.NewestTask = task.CreatedAt
		}
	}

	return stats, nil
}

// RedisQueue is a Redis-based distributed queue
type RedisQueue struct {
	client    *redis.Client
	keyPrefix string
	mu        sync.RWMutex
}

// NewRedisQueue creates a new Redis queue
func NewRedisQueue(client *redis.Client, keyPrefix string) *RedisQueue {
	return &RedisQueue{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

// Push adds a task to the Redis queue
func (rq *RedisQueue) Push(ctx context.Context, task *Task) error {
	task.Queue()

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	// Use sorted set with priority as score
	score := float64(time.Now().Unix()*1000) - float64(task.Priority*1000000)
	key := rq.getQueueKey(Priority(task.Priority))

	if err := rq.client.ZAdd(ctx, key, &redis.Z{
		Score:  score,
		Member: string(data),
	}).Err(); err != nil {
		return fmt.Errorf("failed to push task: %w", err)
	}

	// Update metadata
	rq.updateMetadata(ctx, task)

	return nil
}

// PushBatch adds multiple tasks
func (rq *RedisQueue) PushBatch(ctx context.Context, tasks []*Task) error {
	pipe := rq.client.Pipeline()

	for _, task := range tasks {
		task.Queue()

		data, err := json.Marshal(task)
		if err != nil {
			return fmt.Errorf("failed to marshal task: %w", err)
		}

		score := float64(time.Now().Unix()*1000) - float64(task.Priority*1000000)
		key := rq.getQueueKey(Priority(task.Priority))

		pipe.ZAdd(ctx, key, &redis.Z{
			Score:  score,
			Member: string(data),
		})

		rq.updateMetadataPipe(ctx, pipe, task)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// Pop retrieves and removes highest priority task
func (rq *RedisQueue) Pop(ctx context.Context) (*Task, error) {
	priorities := []Priority{PriorityUrgent, PriorityHigh, PriorityNormal, PriorityLow}

	for _, priority := range priorities {
		key := rq.getQueueKey(priority)

		// Pop from sorted set
		result, err := rq.client.ZPopMin(ctx, key, 1).Result()
		if err == nil && len(result) > 0 {
			var task Task
			if err := json.Unmarshal([]byte(result[0].Member.(string)), &task); err != nil {
				return nil, fmt.Errorf("failed to unmarshal task: %w", err)
			}

			// Update task count
			rq.client.HIncrBy(ctx, rq.keyPrefix+":stats", "processed", 1)

			return &task, nil
		}
	}

	return nil, fmt.Errorf("no tasks available")
}

// PopN retrieves and removes N tasks
func (rq *RedisQueue) PopN(ctx context.Context, n int) ([]*Task, error) {
	var tasks []*Task

	for i := 0; i < n; i++ {
		task, err := rq.Pop(ctx)
		if err != nil {
			break
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// Peek retrieves but doesn't remove highest priority task
func (rq *RedisQueue) Peek(ctx context.Context) (*Task, error) {
	priorities := []Priority{PriorityUrgent, PriorityHigh, PriorityNormal, PriorityLow}

	for _, priority := range priorities {
		key := rq.getQueueKey(priority)

		// Peek from sorted set
		result, err := rq.client.ZRange(ctx, key, 0, 0).Result()
		if err == nil && len(result) > 0 {
			var task Task
			if err := json.Unmarshal([]byte(result[0]), &task); err != nil {
				return nil, fmt.Errorf("failed to unmarshal task: %w", err)
			}
			return &task, nil
		}
	}

	return nil, fmt.Errorf("no tasks available")
}

// Size returns total queue size
func (rq *RedisQueue) Size(ctx context.Context) (int64, error) {
	var total int64

	priorities := []Priority{PriorityUrgent, PriorityHigh, PriorityNormal, PriorityLow}
	for _, priority := range priorities {
		key := rq.getQueueKey(priority)
		count, err := rq.client.ZCard(ctx, key).Result()
		if err != nil {
			return 0, err
		}
		total += count
	}

	return total, nil
}

// Clear removes all tasks
func (rq *RedisQueue) Clear(ctx context.Context) error {
	pattern := rq.keyPrefix + ":queue:*"

	iter := rq.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := rq.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}

	return iter.Err()
}

// Stats returns queue statistics
func (rq *RedisQueue) Stats(ctx context.Context) (*QueueStats, error) {
	stats := &QueueStats{
		ByPriority: make(map[Priority]int64),
		ByStatus:   make(map[Status]int64),
		ByType:     make(map[Type]int64),
	}

	priorities := []Priority{PriorityUrgent, PriorityHigh, PriorityNormal, PriorityLow}
	for _, priority := range priorities {
		key := rq.getQueueKey(priority)
		count, _ := rq.client.ZCard(ctx, key).Result()
		stats.ByPriority[priority] = count
		stats.Total += count
	}

	// Get additional stats from hash
	result, _ := rq.client.HGetAll(ctx, rq.keyPrefix+":stats").Result()
	for k, _ := range result {
		if k == "processed" {
			// Track processed count
		}
	}

	return stats, nil
}

// getQueueKey returns the Redis key for a priority queue
func (rq *RedisQueue) getQueueKey(priority Priority) string {
	return fmt.Sprintf("%s:queue:%d", rq.keyPrefix, priority)
}

// updateMetadata updates task metadata in Redis
func (rq *RedisQueue) updateMetadata(ctx context.Context, task *Task) {
	pipe := rq.client.Pipeline()
	rq.updateMetadataPipe(ctx, pipe, task)
	pipe.Exec(ctx)
}

// updateMetadataPipe updates task metadata in pipeline
func (rq *RedisQueue) updateMetadataPipe(ctx context.Context, pipe redis.Pipeliner, task *Task) {
	// Update counters
	pipe.HIncrBy(ctx, rq.keyPrefix+":stats", "total", 1)
	pipe.HIncrBy(ctx, rq.keyPrefix+":stats", fmt.Sprintf("type:%s", task.Type), 1)
	pipe.HIncrBy(ctx, rq.keyPrefix+":stats", fmt.Sprintf("status:%s", task.Status), 1)

	// Store task details for tracking
	taskKey := fmt.Sprintf("%s:task:%s", rq.keyPrefix, task.ID)
	taskData, _ := json.Marshal(task)
	pipe.Set(ctx, taskKey, taskData, 24*time.Hour)
}

// DelayedQueue manages delayed task execution
type DelayedQueue struct {
	queue  Queue
	client *redis.Client
	prefix string
	mu     sync.RWMutex
}

// NewDelayedQueue creates a new delayed queue
func NewDelayedQueue(queue Queue, client *redis.Client, prefix string) *DelayedQueue {
	return &DelayedQueue{
		queue:  queue,
		client: client,
		prefix: prefix,
	}
}

// PushDelayed adds a task with delay
func (dq *DelayedQueue) PushDelayed(ctx context.Context, task *Task, delay time.Duration) error {
	executeAt := time.Now().Add(delay)

	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	// Store in Redis sorted set with execution time as score
	key := dq.prefix + ":delayed"
	score := float64(executeAt.Unix())

	return dq.client.ZAdd(ctx, key, &redis.Z{
		Score:  score,
		Member: string(data),
	}).Err()
}

// ProcessDelayed moves ready tasks to main queue
func (dq *DelayedQueue) ProcessDelayed(ctx context.Context) error {
	key := dq.prefix + ":delayed"
	now := float64(time.Now().Unix())

	// Get all tasks ready for execution
	results, err := dq.client.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%f", now),
	}).Result()

	if err != nil {
		return err
	}

	// Move tasks to main queue
	for _, result := range results {
		var task Task
		if err := json.Unmarshal([]byte(result.Member.(string)), &task); err != nil {
			continue
		}

		// Push to main queue
		if err := dq.queue.Push(ctx, &task); err != nil {
			continue
		}

		// Remove from delayed queue
		dq.client.ZRem(ctx, key, result.Member)
	}

	return nil
}

// StartProcessor starts the delayed task processor
func (dq *DelayedQueue) StartProcessor(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dq.ProcessDelayed(ctx)
		}
	}
}
