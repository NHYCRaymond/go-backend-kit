package task

import (
	"log/slog"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/errors"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// TaskManager manages task instances and their lifecycle
type TaskManager struct {
	mu       sync.RWMutex
	tasks    map[string]*TaskInstance
	registry *TaskRegistry
	redis    *redis.Client
	prefix   string
	logger   *slog.Logger

	// Task allocation
	allocator    TaskAllocator
	nodeManager  NodeManager

	// Metrics
	totalTasks     int64
	activeTasks    int64
	completedTasks int64
	failedTasks    int64

	// Configuration
	maxActiveTasks   int
	taskTimeout      time.Duration
	cleanupInterval  time.Duration
	stateCheckInterval time.Duration

	// Channels
	taskChan     chan *TaskInstance
	completeChan chan string
	stopChan     chan struct{}
}

// TaskInstance represents a running task instance
type TaskInstance struct {
	ID           string                 `json:"id"`
	DefinitionID string                 `json:"definition_id"`
	Task         *Task                  `json:"task"`
	State        TaskState              `json:"state"`
	NodeID       string                 `json:"node_id"`
	WorkerID     string                 `json:"worker_id"`
	
	// Context
	Context      map[string]interface{} `json:"context"`
	Variables    map[string]interface{} `json:"variables"`
	
	// Timing
	CreatedAt    time.Time              `json:"created_at"`
	StartedAt    *time.Time             `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at"`
	
	// Results
	Result       interface{}            `json:"result"`
	Error        string                 `json:"error"`
	RetryCount   int                    `json:"retry_count"`
	
	// Dependencies
	Dependencies []string               `json:"dependencies"`
	DependsOn    []string               `json:"depends_on"`
	
	// Monitoring
	LastHeartbeat time.Time             `json:"last_heartbeat"`
	Progress      float64               `json:"progress"`
	Metrics       map[string]interface{} `json:"metrics"`
}

// TaskState represents task execution state
type TaskState string

const (
	TaskStateCreated   TaskState = "created"
	TaskStateAllocated TaskState = "allocated"
	TaskStateRunning   TaskState = "running"
	TaskStatePaused    TaskState = "paused"
	TaskStateCompleted TaskState = "completed"
	TaskStateFailed    TaskState = "failed"
	TaskStateCancelled TaskState = "cancelled"
	TaskStateTimeout   TaskState = "timeout"
)

// TaskAllocator allocates tasks to nodes
type TaskAllocator interface {
	Allocate(ctx context.Context, instance *TaskInstance, nodes []NodeInfo) (*NodeInfo, error)
	Release(ctx context.Context, instance *TaskInstance) error
}

// NodeManager manages node information
type NodeManager interface {
	GetAvailableNodes(ctx context.Context) ([]NodeInfo, error)
	GetNode(ctx context.Context, nodeID string) (*NodeInfo, error)
	UpdateNodeLoad(ctx context.Context, nodeID string, delta int) error
}

// ManagerConfig contains task manager configuration
type ManagerConfig struct {
	Registry           *TaskRegistry
	Redis              *redis.Client
	Prefix             string
	Logger             *slog.Logger
	Allocator          TaskAllocator
	NodeManager        NodeManager
	MaxActiveTasks     int
	TaskTimeout        time.Duration
	CleanupInterval    time.Duration
	StateCheckInterval time.Duration
}

// NewTaskManager creates a new task manager
func NewTaskManager(config *ManagerConfig) *TaskManager {
	if config.MaxActiveTasks <= 0 {
		config.MaxActiveTasks = 1000
	}
	if config.TaskTimeout <= 0 {
		config.TaskTimeout = 30 * time.Minute
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}
	if config.StateCheckInterval <= 0 {
		config.StateCheckInterval = 30 * time.Second
	}

	return &TaskManager{
		tasks:              make(map[string]*TaskInstance),
		registry:           config.Registry,
		redis:              config.Redis,
		prefix:             config.Prefix,
		logger:             config.Logger,
		allocator:          config.Allocator,
		nodeManager:        config.NodeManager,
		maxActiveTasks:     config.MaxActiveTasks,
		taskTimeout:        config.TaskTimeout,
		cleanupInterval:    config.CleanupInterval,
		stateCheckInterval: config.StateCheckInterval,
		taskChan:           make(chan *TaskInstance, 100),
		completeChan:       make(chan string, 100),
		stopChan:           make(chan struct{}),
	}
}

// CreateInstance creates a new task instance
func (tm *TaskManager) CreateInstance(definitionID string, params map[string]interface{}) (*TaskInstance, error) {
	// Get definition
	def, err := tm.registry.GetDefinition(definitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get definition: %w", err)
	}

	// Generate URL from params
	url := ""
	if u, ok := params["url"].(string); ok {
		url = u
	} else {
		return nil, errors.New(errors.ValidationErrorCode, "URL is required")
	}

	// Create task from definition
	task, err := def.CreateTask(url, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Create instance
	instance := &TaskInstance{
		ID:           uuid.New().String(),
		DefinitionID: definitionID,
		Task:         task,
		State:        TaskStateCreated,
		Context:      make(map[string]interface{}),
		Variables:    params,
		CreatedAt:    time.Now(),
		LastHeartbeat: time.Now(),
		Metrics:      make(map[string]interface{}),
	}

	// Store instance
	tm.mu.Lock()
	tm.tasks[instance.ID] = instance
	atomic.AddInt64(&tm.totalTasks, 1)
	tm.mu.Unlock()

	// Persist to Redis
	if err := tm.saveInstance(instance); err != nil {
		tm.logger.Error("Failed to save instance", "error", err)
	}

	tm.logger.Info("Task instance created",
		"instance_id", instance.ID,
		"definition_id", definitionID,
		"url", url)

	// Queue for allocation
	select {
	case tm.taskChan <- instance:
	default:
		tm.logger.Warn("Task queue full", "instance_id", instance.ID)
	}

	return instance, nil
}

// CreateInstanceFromTemplate creates instance from template
func (tm *TaskManager) CreateInstanceFromTemplate(templateID string, params map[string]interface{}) (*TaskInstance, error) {
	// Get template
	tmpl, err := tm.registry.GetTemplate(templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	// Create task from template
	task, err := tm.registry.CreateTaskFromTemplate(templateID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create task from template: %w", err)
	}

	// Create instance
	instance := &TaskInstance{
		ID:           uuid.New().String(),
		DefinitionID: tmpl.Definition,
		Task:         task,
		State:        TaskStateCreated,
		Context:      make(map[string]interface{}),
		Variables:    params,
		CreatedAt:    time.Now(),
		LastHeartbeat: time.Now(),
		Metrics:      make(map[string]interface{}),
	}

	instance.Context["template_id"] = templateID
	instance.Context["template_name"] = tmpl.Name

	// Store instance
	tm.mu.Lock()
	tm.tasks[instance.ID] = instance
	atomic.AddInt64(&tm.totalTasks, 1)
	tm.mu.Unlock()

	// Persist to Redis
	if err := tm.saveInstance(instance); err != nil {
		tm.logger.Error("Failed to save instance", "error", err)
	}

	// Queue for allocation
	select {
	case tm.taskChan <- instance:
	default:
		tm.logger.Warn("Task queue full", "instance_id", instance.ID)
	}

	return instance, nil
}

// GetInstance retrieves a task instance
func (tm *TaskManager) GetInstance(instanceID string) (*TaskInstance, error) {
	tm.mu.RLock()
	instance, exists := tm.tasks[instanceID]
	tm.mu.RUnlock()

	if exists {
		return instance, nil
	}

	// Try to load from Redis
	instance, err := tm.loadInstance(instanceID)
	if err != nil {
		return nil, errors.New(errors.NotFoundErrorCode, fmt.Sprintf("instance %s not found", instanceID))
	}

	// Cache in memory
	tm.mu.Lock()
	tm.tasks[instanceID] = instance
	tm.mu.Unlock()

	return instance, nil
}

// UpdateState updates task instance state
func (tm *TaskManager) UpdateState(instanceID string, state TaskState, metadata map[string]interface{}) error {
	tm.mu.Lock()
	instance, exists := tm.tasks[instanceID]
	if !exists {
		tm.mu.Unlock()
		return errors.New(errors.NotFoundErrorCode, fmt.Sprintf("instance %s not found", instanceID))
	}

	oldState := instance.State
	instance.State = state
	now := time.Now()

	switch state {
	case TaskStateRunning:
		if instance.StartedAt == nil {
			instance.StartedAt = &now
			atomic.AddInt64(&tm.activeTasks, 1)
		}
	case TaskStateCompleted:
		if instance.CompletedAt == nil {
			instance.CompletedAt = &now
			atomic.AddInt64(&tm.completedTasks, 1)
			atomic.AddInt64(&tm.activeTasks, -1)
		}
	case TaskStateFailed:
		if instance.CompletedAt == nil {
			instance.CompletedAt = &now
			atomic.AddInt64(&tm.failedTasks, 1)
			atomic.AddInt64(&tm.activeTasks, -1)
		}
	}

	// Update metadata
	for k, v := range metadata {
		instance.Context[k] = v
	}

	instance.LastHeartbeat = now
	tm.mu.Unlock()

	// Persist to Redis
	if err := tm.saveInstance(instance); err != nil {
		tm.logger.Error("Failed to save instance", "error", err)
	}

	// Publish state change event
	tm.publishStateChange(instance, oldState, state)

	tm.logger.Debug("Task state updated",
		"instance_id", instanceID,
		"old_state", oldState,
		"new_state", state)

	// Handle completion
	if state == TaskStateCompleted || state == TaskStateFailed {
		select {
		case tm.completeChan <- instanceID:
		default:
		}
	}

	return nil
}

// UpdateProgress updates task progress
func (tm *TaskManager) UpdateProgress(instanceID string, progress float64, metrics map[string]interface{}) error {
	tm.mu.Lock()
	instance, exists := tm.tasks[instanceID]
	if !exists {
		tm.mu.Unlock()
		return errors.New(errors.NotFoundErrorCode, fmt.Sprintf("instance %s not found", instanceID))
	}

	instance.Progress = progress
	instance.LastHeartbeat = time.Now()

	// Update metrics
	for k, v := range metrics {
		instance.Metrics[k] = v
	}
	tm.mu.Unlock()

	// Persist to Redis (throttled)
	key := fmt.Sprintf("%s:progress:%s", tm.prefix, instanceID)
	ctx := context.Background()
	tm.redis.Set(ctx, key, progress, time.Minute)

	return nil
}

// AllocateTask allocates a task to a node
func (tm *TaskManager) AllocateTask(ctx context.Context, instanceID string) error {
	instance, err := tm.GetInstance(instanceID)
	if err != nil {
		return err
	}

	// Get available nodes
	nodes, err := tm.nodeManager.GetAvailableNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get available nodes: %w", err)
	}

	if len(nodes) == 0 {
		return errors.New(errors.ResourceExhaustedErrorCode, "no available nodes")
	}

	// Allocate to node
	node, err := tm.allocator.Allocate(ctx, instance, nodes)
	if err != nil {
		return fmt.Errorf("failed to allocate task: %w", err)
	}

	// Update instance
	tm.mu.Lock()
	instance.NodeID = node.ID
	instance.State = TaskStateAllocated
	tm.mu.Unlock()

	// Update node load
	if err := tm.nodeManager.UpdateNodeLoad(ctx, node.ID, 1); err != nil {
		tm.logger.Error("Failed to update node load", "error", err)
	}

	// Persist allocation
	if err := tm.saveAllocation(instance, node); err != nil {
		tm.logger.Error("Failed to save allocation", "error", err)
	}

	tm.logger.Info("Task allocated",
		"instance_id", instanceID,
		"node_id", node.ID)

	return nil
}

// Start starts the task manager
func (tm *TaskManager) Start(ctx context.Context) error {
	tm.logger.Info("Starting task manager")

	// Load existing instances from Redis
	if err := tm.loadInstances(ctx); err != nil {
		tm.logger.Error("Failed to load instances", "error", err)
	}

	// Start allocation worker
	go tm.allocationWorker(ctx)

	// Start state monitor
	go tm.stateMonitor(ctx)

	// Start cleanup worker
	go tm.cleanupWorker(ctx)

	return nil
}

// Stop stops the task manager
func (tm *TaskManager) Stop() error {
	tm.logger.Info("Stopping task manager")
	close(tm.stopChan)
	return nil
}

// allocationWorker handles task allocation
func (tm *TaskManager) allocationWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.stopChan:
			return
		case instance := <-tm.taskChan:
			if err := tm.AllocateTask(ctx, instance.ID); err != nil {
				tm.logger.Error("Failed to allocate task",
					"instance_id", instance.ID,
					"error", err)
				
				// Retry later
				time.Sleep(5 * time.Second)
				select {
				case tm.taskChan <- instance:
				default:
				}
			}
		}
	}
}

// stateMonitor monitors task states
func (tm *TaskManager) stateMonitor(ctx context.Context) {
	ticker := time.NewTicker(tm.stateCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.stopChan:
			return
		case <-ticker.C:
			tm.checkTaskStates(ctx)
		}
	}
}

// checkTaskStates checks and updates task states
func (tm *TaskManager) checkTaskStates(ctx context.Context) {
	tm.mu.RLock()
	instances := make([]*TaskInstance, 0, len(tm.tasks))
	for _, instance := range tm.tasks {
		instances = append(instances, instance)
	}
	tm.mu.RUnlock()

	now := time.Now()
	for _, instance := range instances {
		// Check for timeout
		if instance.State == TaskStateRunning {
			if instance.StartedAt != nil && now.Sub(*instance.StartedAt) > tm.taskTimeout {
				tm.UpdateState(instance.ID, TaskStateTimeout, map[string]interface{}{
					"reason": "task timeout",
				})
				continue
			}

			// Check heartbeat
			if now.Sub(instance.LastHeartbeat) > 2*tm.stateCheckInterval {
				tm.logger.Warn("Task heartbeat missed",
					"instance_id", instance.ID,
					"last_heartbeat", instance.LastHeartbeat)
			}
		}

		// Check dependencies
		if instance.State == TaskStateCreated && len(instance.DependsOn) > 0 {
			if tm.checkDependencies(instance) {
				// Dependencies satisfied, queue for allocation
				select {
				case tm.taskChan <- instance:
				default:
				}
			}
		}
	}
}

// checkDependencies checks if task dependencies are satisfied
func (tm *TaskManager) checkDependencies(instance *TaskInstance) bool {
	for _, depID := range instance.DependsOn {
		dep, err := tm.GetInstance(depID)
		if err != nil || dep.State != TaskStateCompleted {
			return false
		}
	}
	return true
}

// cleanupWorker cleans up old task instances
func (tm *TaskManager) cleanupWorker(ctx context.Context) {
	ticker := time.NewTicker(tm.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.stopChan:
			return
		case <-ticker.C:
			tm.cleanupOldInstances(ctx)
		}
	}
}

// cleanupOldInstances removes old completed instances
func (tm *TaskManager) cleanupOldInstances(ctx context.Context) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	for id, instance := range tm.tasks {
		if instance.CompletedAt != nil && now.Sub(*instance.CompletedAt) > 24*time.Hour {
			delete(tm.tasks, id)
			tm.logger.Debug("Cleaned up old instance", "instance_id", id)
		}
	}
}

// saveInstance saves instance to Redis
func (tm *TaskManager) saveInstance(instance *TaskInstance) error {
	ctx := context.Background()
	key := fmt.Sprintf("%s:instance:%s", tm.prefix, instance.ID)
	data, err := json.Marshal(instance)
	if err != nil {
		return err
	}

	return tm.redis.Set(ctx, key, data, 24*time.Hour).Err()
}

// loadInstance loads instance from Redis
func (tm *TaskManager) loadInstance(instanceID string) (*TaskInstance, error) {
	ctx := context.Background()
	key := fmt.Sprintf("%s:instance:%s", tm.prefix, instanceID)
	data, err := tm.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var instance TaskInstance
	if err := json.Unmarshal([]byte(data), &instance); err != nil {
		return nil, err
	}

	return &instance, nil
}

// loadInstances loads all instances from Redis
func (tm *TaskManager) loadInstances(ctx context.Context) error {
	pattern := fmt.Sprintf("%s:instance:*", tm.prefix)
	keys, err := tm.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	for _, key := range keys {
		data, err := tm.redis.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var instance TaskInstance
		if err := json.Unmarshal([]byte(data), &instance); err != nil {
			continue
		}

		// Only load active instances
		if instance.State == TaskStateRunning || instance.State == TaskStateAllocated {
			tm.mu.Lock()
			tm.tasks[instance.ID] = &instance
			tm.mu.Unlock()
		}
	}

	tm.logger.Info("Loaded instances from Redis", "count", len(tm.tasks))
	return nil
}

// saveAllocation saves task allocation to Redis
func (tm *TaskManager) saveAllocation(instance *TaskInstance, node *NodeInfo) error {
	ctx := context.Background()
	key := fmt.Sprintf("%s:allocation:%s", tm.prefix, instance.ID)
	
	allocation := map[string]interface{}{
		"instance_id": instance.ID,
		"node_id":     node.ID,
		"allocated_at": time.Now(),
	}
	
	data, _ := json.Marshal(allocation)
	return tm.redis.Set(ctx, key, data, 24*time.Hour).Err()
}

// publishStateChange publishes task state change event
func (tm *TaskManager) publishStateChange(instance *TaskInstance, oldState, newState TaskState) {
	ctx := context.Background()
	channel := fmt.Sprintf("%s:events:state", tm.prefix)
	
	event := map[string]interface{}{
		"instance_id": instance.ID,
		"task_id":     instance.Task.ID,
		"old_state":   oldState,
		"new_state":   newState,
		"timestamp":   time.Now(),
	}
	
	data, _ := json.Marshal(event)
	tm.redis.Publish(ctx, channel, data)
}

// GetMetrics returns task manager metrics
func (tm *TaskManager) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"total_tasks":     atomic.LoadInt64(&tm.totalTasks),
		"active_tasks":    atomic.LoadInt64(&tm.activeTasks),
		"completed_tasks": atomic.LoadInt64(&tm.completedTasks),
		"failed_tasks":    atomic.LoadInt64(&tm.failedTasks),
		"cached_tasks":    len(tm.tasks),
	}
}

// DefaultTaskAllocator is the default task allocator
type DefaultTaskAllocator struct {
	strategy string // round-robin, least-load, hash
}

// NewDefaultTaskAllocator creates a default task allocator
func NewDefaultTaskAllocator(strategy string) *DefaultTaskAllocator {
	return &DefaultTaskAllocator{
		strategy: strategy,
	}
}

// Allocate allocates a task to a node
func (a *DefaultTaskAllocator) Allocate(ctx context.Context, instance *TaskInstance, nodes []NodeInfo) (*NodeInfo, error) {
	if len(nodes) == 0 {
		return nil, errors.New(errors.ResourceExhaustedErrorCode, "no available nodes")
	}

	switch a.strategy {
	case "least-load":
		return a.allocateLeastLoad(nodes), nil
	case "hash":
		return a.allocateHash(instance, nodes), nil
	default: // round-robin
		return a.allocateRoundRobin(nodes), nil
	}
}

// Release releases a task allocation
func (a *DefaultTaskAllocator) Release(ctx context.Context, instance *TaskInstance) error {
	// Nothing to do for default allocator
	return nil
}

func (a *DefaultTaskAllocator) allocateRoundRobin(nodes []NodeInfo) *NodeInfo {
	// Simple round-robin using timestamp
	index := time.Now().UnixNano() % int64(len(nodes))
	return &nodes[index]
}

func (a *DefaultTaskAllocator) allocateLeastLoad(nodes []NodeInfo) *NodeInfo {
	var selected *NodeInfo
	minLoad := int(^uint(0) >> 1) // Max int

	for i := range nodes {
		if nodes[i].CurrentLoad < minLoad {
			selected = &nodes[i]
			minLoad = nodes[i].CurrentLoad
		}
	}

	return selected
}

func (a *DefaultTaskAllocator) allocateHash(instance *TaskInstance, nodes []NodeInfo) *NodeInfo {
	// Hash based on task URL
	hash := 0
	for _, b := range []byte(instance.Task.URL) {
		hash = hash*31 + int(b)
	}

	index := hash % len(nodes)
	return &nodes[index]
}