package task

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
)

// DispatchStrategy defines how tasks are distributed
type DispatchStrategy string

const (
	StrategyRoundRobin DispatchStrategy = "round_robin"
	StrategyLeastLoad  DispatchStrategy = "least_load"
	StrategyRandom     DispatchStrategy = "random"
	StrategyHash       DispatchStrategy = "hash"
	StrategySticky     DispatchStrategy = "sticky"
)

// Dispatcher distributes tasks to worker nodes
type Dispatcher struct {
	mu       sync.RWMutex
	queue    Queue
	nodes    map[string]*NodeInfo
	strategy DispatchStrategy

	// Redis for coordination
	redis  *redis.Client
	prefix string

	// Metrics
	dispatched int64
	failed     int64

	// Configuration
	batchSize  int
	maxRetries int
	timeout    time.Duration

	// Channels
	taskChan chan *Task
	stopChan chan struct{}

	// Logger
	logger *slog.Logger

	// Seed expansion
	seedExpander SeedExpander
}

// NodeInfo contains worker node information
type NodeInfo struct {
	ID            string    `json:"id"`
	Address       string    `json:"address"`
	Capacity      int       `json:"capacity"`
	CurrentLoad   int       `json:"current_load"`
	Status        string    `json:"status"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	Tags          []string  `json:"tags"`
	Capabilities  []string  `json:"capabilities"`
}

// DispatcherConfig contains dispatcher configuration
type DispatcherConfig struct {
	QueueType   QueueType
	Strategy    DispatchStrategy
	BatchSize   int
	MaxRetries  int
	Timeout     time.Duration
	RedisClient *redis.Client
	RedisPrefix string
	Logger      *slog.Logger
}

// NewDispatcher creates a new task dispatcher
func NewDispatcher(config *DispatcherConfig) *Dispatcher {
	var queue Queue
	switch config.QueueType {
	case QueueTypeRedis:
		queue = NewRedisQueue(config.RedisClient, config.RedisPrefix)
	default:
		queue = NewPriorityQueue()
	}

	return &Dispatcher{
		queue:        queue,
		nodes:        make(map[string]*NodeInfo),
		strategy:     config.Strategy,
		redis:        config.RedisClient,
		prefix:       config.RedisPrefix,
		batchSize:    config.BatchSize,
		maxRetries:   config.MaxRetries,
		timeout:      config.Timeout,
		taskChan:     make(chan *Task, 1000),
		stopChan:     make(chan struct{}),
		logger:       config.Logger,
		seedExpander: NewDefaultSeedExpander(),
	}
}

// Start starts the dispatcher
func (d *Dispatcher) Start(ctx context.Context) error {
	d.logger.Info("Starting dispatcher")

	// Start worker pool
	workerCount := 10
	for i := 0; i < workerCount; i++ {
		go d.worker(ctx)
	}

	// Start dispatch loop
	go d.dispatchLoop(ctx)

	// Start node monitor
	go d.monitorNodes(ctx)

	// Start seed processor
	go d.processSeedTasks(ctx)

	return nil
}

// Stop stops the dispatcher
func (d *Dispatcher) Stop() error {
	d.logger.Info("Stopping dispatcher")
	close(d.stopChan)
	return nil
}

// Submit submits a task for dispatch
func (d *Dispatcher) Submit(ctx context.Context, task *Task) error {
	// Validate task
	if err := task.Validate(); err != nil {
		return fmt.Errorf("invalid task: %w", err)
	}

	// Check for seed task
	if task.Type == string(TypeSeed) {
		// Process seed task for expansion
		return d.submitSeedTask(ctx, task)
	}

	// Add to queue
	if err := d.queue.Push(ctx, task); err != nil {
		return fmt.Errorf("failed to queue task: %w", err)
	}

	// Update metrics
	atomic.AddInt64(&d.dispatched, 1)

	// Send to task channel for immediate processing
	select {
	case d.taskChan <- task:
	default:
		// Channel full, task will be processed from queue
	}

	return nil
}

// SubmitBatch submits multiple tasks
func (d *Dispatcher) SubmitBatch(ctx context.Context, tasks []*Task) error {
	seedTasks := []*Task{}
	regularTasks := []*Task{}

	// Separate seed tasks
	for _, task := range tasks {
		if task.Type == string(TypeSeed) {
			seedTasks = append(seedTasks, task)
		} else {
			regularTasks = append(regularTasks, task)
		}
	}

	// Process seed tasks
	for _, task := range seedTasks {
		if err := d.submitSeedTask(ctx, task); err != nil {
			d.logger.Error("Failed to submit seed task", "error", err)
		}
	}

	// Queue regular tasks
	if len(regularTasks) > 0 {
		if err := d.queue.PushBatch(ctx, regularTasks); err != nil {
			return fmt.Errorf("failed to queue batch: %w", err)
		}
		atomic.AddInt64(&d.dispatched, int64(len(regularTasks)))
	}

	return nil
}

// submitSeedTask processes a seed task
func (d *Dispatcher) submitSeedTask(ctx context.Context, task *Task) error {
	// Mark as seed task for tracking
	task.SetMetadata("is_seed", true)
	task.SetMetadata("seed_id", task.ID)

	// Add to high priority queue
	task.Priority = int(PriorityHigh)

	// Queue the seed task itself
	if err := d.queue.Push(ctx, task); err != nil {
		return err
	}

	d.logger.Info("Seed task submitted",
		"task_id", task.ID,
		"url", task.URL)

	return nil
}

// processSeedTasks processes completed seed tasks
func (d *Dispatcher) processSeedTasks(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		case <-ticker.C:
			d.expandSeedTasks(ctx)
		}
	}
}

// expandSeedTasks expands seed tasks into new tasks
func (d *Dispatcher) expandSeedTasks(ctx context.Context) {
	// Get completed seed tasks from Redis
	key := fmt.Sprintf("%s:seed:completed", d.prefix)

	members, err := d.redis.SMembers(ctx, key).Result()
	if err != nil || len(members) == 0 {
		return
	}

	for _, member := range members {
		// Get seed result
		resultKey := fmt.Sprintf("%s:seed:result:%s", d.prefix, member)
		data, err := d.redis.Get(ctx, resultKey).Result()
		if err != nil {
			continue
		}

		// Parse and expand
		expandedTasks := d.seedExpander.Expand([]byte(data))
		if len(expandedTasks) > 0 {
			d.SubmitBatch(ctx, expandedTasks)

			d.logger.Info("Expanded seed task",
				"seed_id", member,
				"new_tasks", len(expandedTasks))
		}

		// Remove processed seed
		d.redis.SRem(ctx, key, member)
		d.redis.Del(ctx, resultKey)
	}
}

// dispatchLoop continuously dispatches tasks
func (d *Dispatcher) dispatchLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		case <-ticker.C:
			d.dispatchBatch(ctx)
		}
	}
}

// dispatchBatch dispatches a batch of tasks
func (d *Dispatcher) dispatchBatch(ctx context.Context) {
	// Get available nodes
	nodes := d.getAvailableNodes()
	if len(nodes) == 0 {
		return
	}

	// Get tasks from queue
	tasks, err := d.queue.PopN(ctx, d.batchSize)
	if err != nil || len(tasks) == 0 {
		return
	}

	// Dispatch tasks to nodes
	for _, task := range tasks {
		node := d.selectNode(nodes, task)
		if node == nil {
			// No suitable node, return to queue
			d.queue.Push(ctx, task)
			continue
		}

		// Assign task to node
		if err := d.assignTask(ctx, task, node); err != nil {
			d.logger.Error("Failed to assign task",
				"task_id", task.ID,
				"node_id", node.ID,
				"error", err)

			// Return to queue
			d.queue.Push(ctx, task)
			atomic.AddInt64(&d.failed, 1)
		}
	}
}

// selectNode selects a node based on strategy
func (d *Dispatcher) selectNode(nodes []*NodeInfo, task *Task) *NodeInfo {
	if len(nodes) == 0 {
		return nil
	}

	switch d.strategy {
	case StrategyLeastLoad:
		return d.selectLeastLoadNode(nodes)
	case StrategyHash:
		return d.selectHashNode(nodes, task)
	case StrategySticky:
		return d.selectStickyNode(nodes, task)
	default: // StrategyRoundRobin
		return d.selectRoundRobinNode(nodes)
	}
}

// selectLeastLoadNode selects node with least load
func (d *Dispatcher) selectLeastLoadNode(nodes []*NodeInfo) *NodeInfo {
	var selected *NodeInfo
	minLoad := int(^uint(0) >> 1) // Max int

	for _, node := range nodes {
		load := node.CurrentLoad
		if load < minLoad && load < node.Capacity {
			selected = node
			minLoad = load
		}
	}

	return selected
}

// selectHashNode selects node based on task hash
func (d *Dispatcher) selectHashNode(nodes []*NodeInfo, task *Task) *NodeInfo {
	if len(nodes) == 0 {
		return nil
	}

	// Simple hash based on task URL
	hash := 0
	for _, b := range []byte(task.URL) {
		hash = hash*31 + int(b)
	}

	index := hash % len(nodes)
	return nodes[index]
}

// selectStickyNode selects same node for same source
func (d *Dispatcher) selectStickyNode(nodes []*NodeInfo, task *Task) *NodeInfo {
	// Check if task has preferred node
	if nodeID, ok := task.Metadata["preferred_node"].(string); ok {
		for _, node := range nodes {
			if node.ID == nodeID {
				return node
			}
		}
	}

	// Fall back to hash selection
	return d.selectHashNode(nodes, task)
}

// selectRoundRobinNode selects node in round-robin fashion
func (d *Dispatcher) selectRoundRobinNode(nodes []*NodeInfo) *NodeInfo {
	// Simple round-robin using timestamp
	index := time.Now().UnixNano() % int64(len(nodes))
	return nodes[index]
}

// assignTask assigns a task to a node
func (d *Dispatcher) assignTask(ctx context.Context, task *Task, node *NodeInfo) error {
	// Update task
	task.NodeID = node.ID
	task.Start()

	// Store assignment in Redis
	assignmentKey := fmt.Sprintf("%s:assignment:%s", d.prefix, task.ID)
	taskData, _ := task.ToJSON()

	pipe := d.redis.Pipeline()
	pipe.Set(ctx, assignmentKey, taskData, 24*time.Hour)
	pipe.LPush(ctx, fmt.Sprintf("%s:node:%s:tasks", d.prefix, node.ID), task.ID)
	pipe.HIncrBy(ctx, fmt.Sprintf("%s:node:%s:stats", d.prefix, node.ID), "assigned", 1)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to store assignment: %w", err)
	}

	// Update node load
	d.mu.Lock()
	if n, ok := d.nodes[node.ID]; ok {
		n.CurrentLoad++
	}
	d.mu.Unlock()

	d.logger.Debug("Task assigned",
		"task_id", task.ID,
		"node_id", node.ID)

	return nil
}

// getAvailableNodes returns available worker nodes
func (d *Dispatcher) getAvailableNodes() []*NodeInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var available []*NodeInfo
	now := time.Now()

	for _, node := range d.nodes {
		// Check if node is alive (heartbeat within 30 seconds)
		if now.Sub(node.LastHeartbeat) > 30*time.Second {
			continue
		}

		// Check if node has capacity
		if node.CurrentLoad >= node.Capacity {
			continue
		}

		// Check node status
		if node.Status != "active" {
			continue
		}

		available = append(available, node)
	}

	return available
}

// RegisterNode registers a worker node
func (d *Dispatcher) RegisterNode(node *NodeInfo) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	node.LastHeartbeat = time.Now()
	d.nodes[node.ID] = node

	d.logger.Info("Node registered",
		"node_id", node.ID,
		"address", node.Address,
		"capacity", node.Capacity)

	return nil
}

// UnregisterNode unregisters a worker node
func (d *Dispatcher) UnregisterNode(nodeID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.nodes, nodeID)

	d.logger.Info("Node unregistered", "node_id", nodeID)

	// TODO: Reassign tasks from this node

	return nil
}

// UpdateNodeHeartbeat updates node heartbeat
func (d *Dispatcher) UpdateNodeHeartbeat(nodeID string, load int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if node, ok := d.nodes[nodeID]; ok {
		node.LastHeartbeat = time.Now()
		node.CurrentLoad = load
	}

	return nil
}

// monitorNodes monitors node health
func (d *Dispatcher) monitorNodes(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		case <-ticker.C:
			d.checkNodeHealth()
		}
	}
}

// checkNodeHealth checks node health
func (d *Dispatcher) checkNodeHealth() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for nodeID, node := range d.nodes {
		if now.Sub(node.LastHeartbeat) > 60*time.Second {
			// Node is dead
			node.Status = "dead"
			d.logger.Warn("Node is dead", "node_id", nodeID)

			// TODO: Reassign tasks from dead node
		}
	}
}

// worker processes tasks
func (d *Dispatcher) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		case task := <-d.taskChan:
			d.processTask(ctx, task)
		}
	}
}

// processTask processes a single task
func (d *Dispatcher) processTask(ctx context.Context, task *Task) {
	// This is a placeholder for actual task processing
	// In real implementation, this would send task to node
	d.logger.Debug("Processing task", "task_id", task.ID)
}

// GetStats returns dispatcher statistics
func (d *Dispatcher) GetStats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return map[string]interface{}{
		"nodes":      len(d.nodes),
		"dispatched": atomic.LoadInt64(&d.dispatched),
		"failed":     atomic.LoadInt64(&d.failed),
		"strategy":   d.strategy,
	}
}

// SeedExpander expands seed tasks into new tasks
type SeedExpander interface {
	Expand(data []byte) []*Task
}

// DefaultSeedExpander is the default seed expander
type DefaultSeedExpander struct{}

// NewDefaultSeedExpander creates a default seed expander
func NewDefaultSeedExpander() *DefaultSeedExpander {
	return &DefaultSeedExpander{}
}

// Expand expands seed data into tasks
func (e *DefaultSeedExpander) Expand(data []byte) []*Task {
	// Parse extracted URLs from seed task result
	// This is a simplified implementation
	// In real use, this would parse the extracted data
	// and create new tasks based on extraction rules

	var tasks []*Task

	// Example: Create detail tasks from extracted URLs
	// urls := parseExtractedURLs(data)
	// for _, url := range urls {
	//     task := NewTask(url, TypeDetail, PriorityNormal)
	//     tasks = append(tasks, task)
	// }

	return tasks
}

// TaskRouter routes tasks based on rules
type TaskRouter struct {
	rules []RoutingRule
}

// RoutingRule defines task routing rules
type RoutingRule struct {
	Name      string
	Condition func(*Task) bool
	Target    string // Node ID or tag
	Priority  int
}

// Route determines where to route a task
func (tr *TaskRouter) Route(task *Task) string {
	for _, rule := range tr.rules {
		if rule.Condition(task) {
			return rule.Target
		}
	}
	return ""
}

// AddRule adds a routing rule
func (tr *TaskRouter) AddRule(rule RoutingRule) {
	tr.rules = append(tr.rules, rule)
}
