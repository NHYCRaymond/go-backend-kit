package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/factory"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

// NodeStatus represents node status
type NodeStatus string

const (
	NodeStatusStarting     NodeStatus = "starting"
	NodeStatusActive       NodeStatus = "active"
	NodeStatusPaused       NodeStatus = "paused"
	NodeStatusStopping     NodeStatus = "stopping"
	NodeStatusStopped      NodeStatus = "stopped"
	NodeStatusError        NodeStatus = "error"
	NodeStatusDisconnected NodeStatus = "disconnected"
)

// Node represents a worker node in the cluster
type Node struct {
	// Identity
	ID       string            `json:"id"`
	Hostname string            `json:"hostname"`
	IP       string            `json:"ip"`
	Port     int               `json:"port"`
	Tags     []string          `json:"tags"`
	Labels   map[string]string `json:"labels"`

	// Status
	Status    NodeStatus `json:"status"`
	StartedAt time.Time  `json:"started_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Capabilities
	Capabilities []string `json:"capabilities"` // browser, api, custom
	MaxWorkers   int      `json:"max_workers"`

	// Resources
	CPUCores    int     `json:"cpu_cores"`
	MemoryGB    float64 `json:"memory_gb"`
	DiskGB      float64 `json:"disk_gb"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`

	// Metrics
	TasksProcessed  int64         `json:"tasks_processed"`
	TasksFailed     int64         `json:"tasks_failed"`
	BytesDownloaded int64         `json:"bytes_downloaded"`
	ItemsExtracted  int64         `json:"items_extracted"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	ErrorRate       float64       `json:"error_rate"`

	// Internal
	mu          sync.RWMutex
	workers     []*Worker
	taskQueue   chan *task.Task
	resultQueue chan *TaskResult
	stopChan    chan struct{}

	// Dependencies
	redis      *redis.Client
	registry   *Registry
	executor   task.Executor  // New executor interface
	extractors map[string]task.Extractor // Map of extractors by type
	factory    *factory.Factory  // Factory for creating pipelines and storage
	logger     *slog.Logger
	logForwarder *LogForwarder // Log forwarding to Redis

	// gRPC client
	grpcClient *NodeClient

	// Configuration
	config *NodeConfig
}

// NodeConfig contains node configuration
type NodeConfig struct {
	ID           string            `json:"id"`
	MaxWorkers   int               `json:"max_workers"`
	QueueSize    int               `json:"queue_size"`
	Tags         []string          `json:"tags"`
	Labels       map[string]string `json:"labels"`
	Capabilities []string          `json:"capabilities"`
	RedisAddr    string            `json:"redis_addr"`
	RedisPrefix  string            `json:"redis_prefix"`
	LogLevel     string            `json:"log_level"`
	HubAddr      string            `json:"hub_addr"`    // gRPC hub address
	EnableGRPC   bool              `json:"enable_grpc"` // Enable gRPC connection
}

// Worker represents a task worker
type Worker struct {
	ID      string
	Node    *Node
	Status  string
	Current *task.Task
	mu      sync.Mutex
}

// TaskResult represents task execution result
type TaskResult struct {
	TaskID    string                   `json:"task_id"`
	NodeID    string                   `json:"node_id"`
	WorkerID  string                   `json:"worker_id"`
	Status    string                   `json:"status"`
	Data      []map[string]interface{} `json:"data,omitempty"`
	Error     string                   `json:"error,omitempty"`
	StartTime time.Time                `json:"start_time"`
	EndTime   time.Time                `json:"end_time"`
	Duration  time.Duration            `json:"duration"`
	BytesRead int64                    `json:"bytes_read"`
}

// NewNode creates a new node
func NewNode(config *NodeConfig) (*Node, error) {
	// Get logger first for debugging
	logger := logging.GetLogger()
	if logger != nil {
		logger.Info("NewNode called", "config", config)
	}

	hostname, _ := os.Hostname()

	if config.ID == "" {
		config.ID = fmt.Sprintf("node-%s", uuid.New().String()[:8])
	}

	// Create Redis client using our database package
	if logger != nil {
		logger.Info("Creating Redis client", "addr", config.RedisAddr)
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr: config.RedisAddr,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		if logger != nil {
			logger.Error("Failed to connect to Redis", "error", err)
		}
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	if logger != nil {
		logger.Info("Connected to Redis in NewNode")
	}

	// Logger is already initialized in main, just use it
	if logger == nil {
		logger = logging.GetLogger()
	}
	if logger != nil {
		logger.Info("Logger obtained successfully")
	}

	// Get local IP address
	if logger != nil {
		logger.Info("Getting local IP address")
	}
	localIP := "127.0.0.1"
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					localIP = ipnet.IP.String()
					break
				}
			}
		}
	}
	if logger != nil {
		logger.Info("Local IP obtained", "ip", localIP)
	}

	if logger != nil {
		logger.Info("Creating Node struct")
	}

	node := &Node{
		ID:           config.ID,
		Hostname:     hostname,
		IP:           localIP,
		Port:         8080, // Default port, can be configured
		Tags:         config.Tags,
		Labels:       config.Labels,
		Capabilities: config.Capabilities,
		MaxWorkers:   config.MaxWorkers,
		Status:       NodeStatusStopped,
		CPUCores:     runtime.NumCPU(),
		taskQueue:    make(chan *task.Task, config.QueueSize),
		resultQueue:  make(chan *TaskResult, 100),
		stopChan:     make(chan struct{}),
		redis:        redisClient,
		logger:       logger,
		config:       config,
	}

	// Create log forwarder for centralized logging
	if redisClient != nil {
		node.logForwarder = NewLogForwarder(&LogForwarderConfig{
			Redis:      redisClient,
			NodeID:     node.ID,
			Prefix:     config.RedisPrefix,
			MaxLen:     1000, // Keep last 1000 logs
			BufferSize: 100,
		})
		
		// Create a multi-handler that outputs to both console and Redis
		consoleHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
			AddSource: true,
		})
		redisHandler := NewSlogHandler(node.logForwarder)
		
		// Use a multi-handler that writes to both
		multiHandler := &MultiHandler{
			handlers: []slog.Handler{consoleHandler, redisHandler},
		}
		node.logger = slog.New(multiHandler)
		logger = node.logger
	}

	if logger != nil {
		logger.Info("Node struct created", "node_id", node.ID)
	}

	// Get system resources
	if logger != nil {
		logger.Info("Updating resource info")
	}
	node.updateResourceInfo()

	if logger != nil {
		logger.Info("Node initialization complete", "node_id", node.ID)
	}

	return node, nil
}

// Start starts the node
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.Status != NodeStatusStopped {
		return fmt.Errorf("node is not stopped, current status: %s", n.Status)
	}

	n.Status = NodeStatusStarting
	n.StartedAt = time.Now()
	n.UpdatedAt = time.Now()

	// Start log forwarder
	if n.logForwarder != nil {
		n.logForwarder.Start(ctx)
	}

	n.logger.Info("Starting node",
		"node_id", n.ID,
		"hostname", n.Hostname,
		"ip", n.IP,
		"port", n.Port,
		"max_workers", n.MaxWorkers)

	// Create registry
	n.logger.Debug("Creating registry with prefix", "prefix", n.config.RedisPrefix)
	n.registry = NewRegistry(n.redis, n.config.RedisPrefix)

	// Register node
	n.logger.Info("Registering node to Redis registry")
	if err := n.register(ctx); err != nil {
		n.Status = NodeStatusError
		n.logger.Error("Failed to register node", "error", err)
		return fmt.Errorf("failed to register node: %w", err)
	}
	n.logger.Info("Node registered successfully", "node_id", n.ID, "redis_prefix", n.config.RedisPrefix)

	// Connect to gRPC hub if enabled
	if n.config.EnableGRPC && n.config.HubAddr != "" {
		n.grpcClient = NewNodeClient(&NodeClientConfig{
			NodeID:  n.ID,
			HubAddr: n.config.HubAddr,
			Node:    n,
			Logger:  n.logger,
		})

		if err := n.grpcClient.Connect(); err != nil {
			n.logger.Error("Failed to connect to gRPC hub", "error", err)
			// Continue without gRPC connection
		} else {
			n.logger.Info("Connected to gRPC hub", "addr", n.config.HubAddr)
		}
	}

	// Start workers
	n.workers = make([]*Worker, n.MaxWorkers)
	for i := 0; i < n.MaxWorkers; i++ {
		worker := &Worker{
			ID:     fmt.Sprintf("%s-worker-%d", n.ID, i),
			Node:   n,
			Status: "idle",
		}
		n.workers[i] = worker
		go n.runWorker(ctx, worker)
	}

	// Start task fetcher
	go n.fetchTasks(ctx)

	// Start result processor
	go n.processResults(ctx)

	// Start heartbeat - always needed for Redis registry
	go n.heartbeat(ctx)

	// Start metrics reporter
	go n.reportMetrics(ctx)

	// Start resource monitor
	go n.monitorResources(ctx)

	n.Status = NodeStatusActive
	n.UpdatedAt = time.Now()

	// Update status in Redis after successful start
	if n.registry != nil {
		// Re-register with active status
		info := &NodeInfo{
			ID:            n.ID,
			Hostname:      n.Hostname,
			IP:            n.IP,
			Port:          n.Port,
			Status:        string(n.Status),
			Tags:          n.Tags,
			Labels:        n.Labels,
			Capabilities:  n.Capabilities,
			MaxWorkers:    n.MaxWorkers,
			CPUCores:      n.CPUCores,
			MemoryGB:      n.MemoryGB,
			StartedAt:     n.StartedAt,
			UpdatedAt:     n.UpdatedAt,
			LastHeartbeat: time.Now(), // Set initial heartbeat time
		}
		// Simply re-register to update the status
		if err := n.registry.RegisterNode(ctx, info); err != nil {
			n.logger.Error("Failed to update node status to active", "error", err)
		}
	}

	n.logger.Info("Node started successfully", "node_id", n.ID)

	return nil
}

// Stop stops the node
func (n *Node) Stop(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.Status == NodeStatusStopped {
		return nil
	}

	n.logger.Info("Stopping node", "node_id", n.ID)

	n.Status = NodeStatusStopping

	// Signal stop
	close(n.stopChan)

	// Wait for workers to finish current tasks
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			n.logger.Warn("Timeout waiting for workers to stop")
			goto FORCE_STOP
		case <-ticker.C:
			allIdle := true
			for _, worker := range n.workers {
				if worker.Status != "idle" {
					allIdle = false
					break
				}
			}
			if allIdle {
				goto FORCE_STOP
			}
		}
	}

FORCE_STOP:
	// Disconnect from gRPC hub
	if n.grpcClient != nil {
		n.grpcClient.Disconnect()
	}

	// Unregister from registry
	if n.registry != nil {
		n.unregister(ctx)
	}

	n.Status = NodeStatusStopped
	n.logger.Info("Node stopped", "node_id", n.ID)

	return nil
}

// Pause pauses the node
func (n *Node) Pause() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.Status == NodeStatusActive {
		n.Status = NodeStatusPaused
		n.logger.Info("Node paused", "node_id", n.ID)
	}
}

// Resume resumes the node
func (n *Node) Resume() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.Status == NodeStatusPaused {
		n.Status = NodeStatusActive
		n.logger.Info("Node resumed", "node_id", n.ID)
	}
}

// register registers the node in the registry
func (n *Node) register(ctx context.Context) error {
	info := &NodeInfo{
		ID:            n.ID,
		Hostname:      n.Hostname,
		IP:            n.IP,
		Port:          n.Port,
		Status:        string(n.Status),
		Tags:          n.Tags,
		Labels:        n.Labels,
		Capabilities:  n.Capabilities,
		MaxWorkers:    n.MaxWorkers,
		CPUCores:      n.CPUCores,
		MemoryGB:      n.MemoryGB,
		StartedAt:     n.StartedAt,
		UpdatedAt:     n.UpdatedAt,
		LastHeartbeat: time.Now(), // Set initial heartbeat time
	}

	return n.registry.RegisterNode(ctx, info)
}

// unregister unregisters the node from the registry
func (n *Node) unregister(ctx context.Context) error {
	return n.registry.UnregisterNode(ctx, n.ID)
}

// heartbeat sends periodic heartbeat
func (n *Node) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-n.stopChan:
			return
		case <-ticker.C:
			n.sendHeartbeat(ctx)
		}
	}
}

// sendHeartbeat sends a heartbeat to registry
func (n *Node) sendHeartbeat(ctx context.Context) {
	n.mu.RLock()
	activeWorkers := 0
	for _, worker := range n.workers {
		if worker.Status == "busy" {
			activeWorkers++
		}
	}
	n.mu.RUnlock()

	heartbeat := &Heartbeat{
		NodeID:         n.ID,
		Status:         string(n.Status),
		ActiveWorkers:  activeWorkers,
		MaxWorkers:     n.MaxWorkers,
		TasksProcessed: atomic.LoadInt64(&n.TasksProcessed),
		TasksFailed:    atomic.LoadInt64(&n.TasksFailed),
		CPUUsage:       n.CPUUsage,
		MemoryUsage:    n.MemoryUsage,
		Timestamp:      time.Now(),
	}

	// Also send metrics via gRPC if connected
	if n.grpcClient != nil && n.grpcClient.IsConnected() {
		metrics := n.getNodeMetrics()
		n.grpcClient.SendMetrics(metrics)
	}

	if err := n.registry.UpdateHeartbeat(ctx, heartbeat); err != nil {
		n.logger.Error("Failed to send heartbeat", "error", err)
	} else {
		n.logger.Debug("Heartbeat sent successfully", "node_id", n.ID, "status", n.Status)
	}
}

// fetchTasks fetches tasks from the queue
func (n *Node) fetchTasks(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-n.stopChan:
			return
		case <-ticker.C:
			if n.Status != NodeStatusActive {
				continue
			}

			// Check queue capacity
			if len(n.taskQueue) < cap(n.taskQueue)/2 {
				n.fetchBatch(ctx)
			}
		}
	}
}

// fetchBatch fetches a batch of tasks
func (n *Node) fetchBatch(ctx context.Context) {
	// Calculate batch size
	batchSize := cap(n.taskQueue) - len(n.taskQueue)
	if batchSize > 10 {
		batchSize = 10
	}

	// Try to fetch from multiple queues
	queues := []string{
		n.getTaskQueueKey(),                                   // Node-specific queue
		fmt.Sprintf("%s:queue:pending", n.config.RedisPrefix), // General pending queue
	}

	// Fetch tasks from Redis queues
	for i := 0; i < batchSize; i++ {
		var taskData string
		var err error
		
		// Try each queue in order
		for _, queueKey := range queues {
			taskData, err = n.redis.RPop(ctx, queueKey).Result()
			if err == nil {
				n.logger.Debug("Fetched task from queue", "queue", queueKey)
				break
			}
			if err != redis.Nil {
				n.logger.Error("Failed to fetch task", "queue", queueKey, "error", err)
			}
		}
		
		if err == redis.Nil || taskData == "" {
			break // No more tasks in any queue
		}
		if err != nil {
			break
		}

		// Parse task
		var t task.Task
		if err := json.Unmarshal([]byte(taskData), &t); err != nil {
			n.logger.Error("Failed to unmarshal task", "error", err)
			continue
		}
		
		// Debug: Log what we received
		n.logger.Debug("Unmarshaled task from Redis",
			"task_id", t.ID,
			"has_cookies", len(t.Cookies) > 0,
			"body_len", len(t.Body),
			"body_preview", string(t.Body))

		// Add to local queue
		select {
		case n.taskQueue <- &t:
			n.logger.Info("Task queued for processing", 
				"task_id", t.ID,
				"url", t.URL,
				"type", t.Type)
		default:
			// Queue full, return task to Redis (to the general queue)
			n.redis.LPush(ctx, fmt.Sprintf("%s:queue:pending", n.config.RedisPrefix), taskData)
			return
		}
	}
}

// runWorker runs a worker goroutine
func (n *Node) runWorker(ctx context.Context, worker *Worker) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-n.stopChan:
			return
		case t := <-n.taskQueue:
			if n.Status != NodeStatusActive {
				// Return task to queue
				n.taskQueue <- t
				time.Sleep(time.Second)
				continue
			}

			worker.processTask(ctx, t)
		}
	}
}

// processTask processes a single task
func (w *Worker) processTask(ctx context.Context, t *task.Task) {
	w.mu.Lock()
	w.Status = "busy"
	w.Current = t
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.Status = "idle"
		w.Current = nil
		w.mu.Unlock()
	}()

	startTime := time.Now()

	// Log task start with structured data
	w.Node.logger.Info("📋 Starting task execution",
		"task_id", t.ID,
		"worker_id", w.ID, 
		"task_type", t.Type,
		"url", t.URL,
		"method", t.Method,
		"depth", t.Depth,
		"retry_count", t.RetryCount)

	// Create result with task metadata
	result := &TaskResult{
		TaskID:    t.ID,
		NodeID:    w.Node.ID,
		WorkerID:  w.ID,
		StartTime: startTime,
		// Store task URL and other metadata in Data for display
		Data: []map[string]interface{}{
			{
				"url":    t.URL,
				"method": t.Method,
				"type":   t.Type,
			},
		},
	}

	// Execute task
	if err := w.executeTask(ctx, t, result); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		atomic.AddInt64(&w.Node.TasksFailed, 1)

		// Log failure with details
		w.Node.logger.Error("❌ Task execution failed",
			"task_id", t.ID,
			"error", err.Error(),
			"url", t.URL,
			"duration_ms", time.Since(startTime).Milliseconds())

		// Handle retry
		if t.CanRetry() {
			t.IncrementRetry()
			w.Node.logger.Info("🔄 Retrying task",
				"task_id", t.ID,
				"retry_count", t.RetryCount+1,
				"max_retries", t.MaxRetries)
			w.Node.requeueTask(ctx, t)
		}
	} else {
		result.Status = "success"
		atomic.AddInt64(&w.Node.TasksProcessed, 1)
		
		// Log success with metrics
		w.Node.logger.Info("✅ Task completed successfully",
			"task_id", t.ID,
			"url", t.URL,
			"duration_ms", time.Since(startTime).Milliseconds(),
			"items_extracted", len(result.Data),
			"bytes_read", result.BytesRead)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Send result
	select {
	case w.Node.resultQueue <- result:
	default:
		w.Node.logger.Warn("Result queue full, dropping result", "task_id", t.ID)
	}
}

// executeTask executes the actual task
func (w *Worker) executeTask(ctx context.Context, t *task.Task, result *TaskResult) error {
	// Check if we have the enhanced executor
	if w.Node.executor != nil {
		// Use enhanced executor
		return w.executeTaskEnhanced(ctx, t, result)
	}
	
	// Fallback: basic implementation for testing
	w.Node.logger.Info("Executing task (basic mode)",
		"task_id", t.ID,
		"worker_id", w.ID,
		"url", t.URL)

	// Simulate basic HTTP fetch
	time.Sleep(2 * time.Second)
	
	// Simulate successful execution
	result.Status = "success"
	result.Data = []map[string]interface{}{
		{
			"url":       t.URL,
			"timestamp": time.Now(),
			"message":   "Task executed in basic mode (no executor configured)",
		},
	}
	
	// Update metrics
	atomic.AddInt64(&w.Node.BytesDownloaded, 1024)
	atomic.AddInt64(&w.Node.ItemsExtracted, 1)
	
	w.Node.logger.Info("Task completed (basic mode)",
		"task_id", t.ID,
		"worker_id", w.ID,
		"status", result.Status)

	return nil
}

// requeueTask returns a task to the queue
func (n *Node) requeueTask(ctx context.Context, t *task.Task) {
	taskData, err := json.Marshal(t)
	if err != nil {
		n.logger.Error("Failed to marshal task for requeue", "error", err)
		return
	}

	// Add back to Redis queue with higher priority
	if err := n.redis.LPush(ctx, n.getTaskQueueKey(), taskData).Err(); err != nil {
		n.logger.Error("Failed to requeue task", "error", err)
	}
}

// processResults processes task results
func (n *Node) processResults(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-n.stopChan:
			return
		case result := <-n.resultQueue:
			n.handleResult(ctx, result)

			// Send result via gRPC if connected
			if n.grpcClient != nil && n.grpcClient.IsConnected() {
				data, err := json.Marshal(result.Data)
				if err != nil {
					n.logger.Error("Failed to marshal result data for gRPC",
						"task_id", result.TaskID,
						"error", err)
				} else {
					n.grpcClient.SendTaskResult(
						result.TaskID,
						result.Error == "",
						data,
						result.Error,
					)
				}
			}
		}
	}
}

// handleResult handles a task result
func (n *Node) handleResult(ctx context.Context, result *TaskResult) {
	// Store result in Redis
	resultKey := fmt.Sprintf("%s:result:%s", n.config.RedisPrefix, result.TaskID)
	resultData, err := json.Marshal(result)
	if err != nil {
		n.logger.Error("Failed to marshal task result",
			"task_id", result.TaskID,
			"error", err)
		return
	}

	if err := n.redis.Set(ctx, resultKey, resultData, 24*time.Hour).Err(); err != nil {
		n.logger.Error("Failed to store task result in Redis",
			"task_id", result.TaskID,
			"error", err)
		return
	}
	
	// Publish result to PubSub channel for Coordinator
	resultChannel := fmt.Sprintf("%s:results", n.config.RedisPrefix)
	if err := n.redis.Publish(ctx, resultChannel, resultData).Err(); err != nil {
		n.logger.Error("Failed to publish result", "error", err)
	} else {
		n.logger.Debug("Published result to channel", 
			"channel", resultChannel,
			"task_id", result.TaskID)
	}

	// Update task status
	taskKey := fmt.Sprintf("%s:task:%s", n.config.RedisPrefix, result.TaskID)
	n.redis.HSet(ctx, taskKey, "status", result.Status)
	n.redis.HSet(ctx, taskKey, "completed_at", time.Now().Format(time.RFC3339))

	// Notify completion
	if result.Status == "success" {
		n.redis.Publish(ctx, n.getEventChannel(), fmt.Sprintf("task:completed:%s", result.TaskID))
	} else {
		n.redis.Publish(ctx, n.getEventChannel(), fmt.Sprintf("task:failed:%s", result.TaskID))
	}

	n.logger.Debug("Result processed",
		"task_id", result.TaskID,
		"status", result.Status,
		"duration", result.Duration)
}

// reportMetrics reports node metrics
func (n *Node) reportMetrics(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-n.stopChan:
			return
		case <-ticker.C:
			n.collectAndReportMetrics(ctx)
		}
	}
}

// collectAndReportMetrics collects and reports metrics
func (n *Node) collectAndReportMetrics(ctx context.Context) {
	metrics := map[string]interface{}{
		"tasks_processed":  atomic.LoadInt64(&n.TasksProcessed),
		"tasks_failed":     atomic.LoadInt64(&n.TasksFailed),
		"bytes_downloaded": atomic.LoadInt64(&n.BytesDownloaded),
		"items_extracted":  atomic.LoadInt64(&n.ItemsExtracted),
		"cpu_usage":        n.CPUUsage,
		"memory_usage":     n.MemoryUsage,
		"queue_size":       len(n.taskQueue),
		"active_workers":   n.getActiveWorkerCount(),
	}

	// Store in Redis
	metricsKey := fmt.Sprintf("%s:metrics:%s", n.config.RedisPrefix, n.ID)
	for k, v := range metrics {
		n.redis.HSet(ctx, metricsKey, k, v)
	}

	// Set expiry
	n.redis.Expire(ctx, metricsKey, time.Hour)
}

// monitorResources monitors system resources
func (n *Node) monitorResources(ctx context.Context) {
	// Use 3 seconds to avoid conflict with 5-second heartbeat
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-n.stopChan:
			return
		case <-ticker.C:
			n.updateResourceInfo()
		}
	}
}

// updateResourceInfo updates resource information
func (n *Node) updateResourceInfo() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Count active workers directly without calling getActiveWorkerCount to avoid deadlock
	activeCount := 0
	for _, worker := range n.workers {
		if worker != nil && worker.Status == "busy" {
			activeCount++
		}
	}

	// Get real system memory info
	if vmStat, err := mem.VirtualMemory(); err == nil {
		n.MemoryUsage = vmStat.UsedPercent
		n.MemoryGB = float64(vmStat.Used) / 1024 / 1024 / 1024
	}

	// Get real CPU usage for this process
	if p, err := process.NewProcess(int32(os.Getpid())); err == nil {
		if cpuPercent, err := p.CPUPercent(); err == nil {
			n.CPUUsage = cpuPercent
		}
	}

	// If CPU usage is still 0, try system-wide CPU usage
	if n.CPUUsage == 0 {
		if cpuPercent, err := cpu.Percent(time.Second, false); err == nil && len(cpuPercent) > 0 {
			n.CPUUsage = cpuPercent[0]
		}
	}
}

// getActiveWorkerCount returns the number of active workers
func (n *Node) getActiveWorkerCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()

	count := 0
	for _, worker := range n.workers {
		if worker.Status == "busy" {
			count++
		}
	}
	return count
}

// getTaskQueueKey returns the Redis key for task queue
func (n *Node) getTaskQueueKey() string {
	return fmt.Sprintf("%s:queue:tasks:%s", n.config.RedisPrefix, n.ID)
}

// getEventChannel returns the Redis channel for events
func (n *Node) getEventChannel() string {
	return fmt.Sprintf("%s:events", n.config.RedisPrefix)
}

// GetStatus returns node status
func (n *Node) GetStatus() NodeStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Status
}

// GetMetrics returns node metrics
func (n *Node) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"id":               n.ID,
		"status":           n.Status,
		"tasks_processed":  atomic.LoadInt64(&n.TasksProcessed),
		"tasks_failed":     atomic.LoadInt64(&n.TasksFailed),
		"bytes_downloaded": atomic.LoadInt64(&n.BytesDownloaded),
		"items_extracted":  atomic.LoadInt64(&n.ItemsExtracted),
		"active_workers":   n.getActiveWorkerCount(),
		"max_workers":      n.MaxWorkers,
		"cpu_usage":        n.CPUUsage,
		"memory_usage":     n.MemoryUsage,
	}
}

// These methods are deprecated - use SetExecutor and SetFactory instead

// updateStatus updates node status
func (n *Node) updateStatus(status NodeStatus) {
	n.mu.Lock()
	
	// Count active workers while we have the lock
	activeWorkers := 0
	for _, worker := range n.workers {
		if worker != nil && worker.Status == "busy" {
			activeWorkers++
		}
	}
	
	n.Status = status
	n.UpdatedAt = time.Now()
	n.mu.Unlock()
	
	// Immediately update status in Redis (without lock)
	if n.registry != nil {
		ctx := context.Background()
		heartbeat := &Heartbeat{
			NodeID:         n.ID,
			Status:         string(status),
			ActiveWorkers:  activeWorkers,
			MaxWorkers:     n.MaxWorkers,
			TasksProcessed: atomic.LoadInt64(&n.TasksProcessed),
			TasksFailed:    atomic.LoadInt64(&n.TasksFailed),
			CPUUsage:       n.CPUUsage,
			MemoryUsage:    n.MemoryUsage,
			Timestamp:      time.Now(),
		}
		if err := n.registry.UpdateHeartbeat(ctx, heartbeat); err != nil {
			n.logger.Error("Failed to update status in Redis", "error", err, "status", status)
		} else {
			n.logger.Debug("Status updated in Redis", "node_id", n.ID, "status", status)
		}
	}
}

// getNodeMetrics returns node metrics for gRPC
func (n *Node) getNodeMetrics() *NodeMetrics {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return &NodeMetrics{
		TasksProcessed:  atomic.LoadInt64(&n.TasksProcessed),
		TasksInQueue:    len(n.taskQueue),
		TasksFailed:     atomic.LoadInt64(&n.TasksFailed),
		BytesDownloaded: atomic.LoadInt64(&n.BytesDownloaded),
		ItemsExtracted:  atomic.LoadInt64(&n.ItemsExtracted),
		AverageLatency:  n.AvgResponseTime,
		ErrorRate:       n.ErrorRate,
	}
}
