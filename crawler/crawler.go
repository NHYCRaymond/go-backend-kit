package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/core"
	"github.com/NHYCRaymond/go-backend-kit/crawler/distributed"
	"github.com/NHYCRaymond/go-backend-kit/crawler/fetcher"
	"github.com/NHYCRaymond/go-backend-kit/crawler/pipeline"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/go-redis/redis/v8"
)

// Crawler represents the main crawler system
type Crawler struct {
	// Identity
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Tags    []string          `json:"tags"`
	Labels  map[string]string `json:"labels"`

	// Core components
	fetcher    fetcher.Fetcher
	pipeline   *pipeline.Pipeline
	taskQueue  task.Queue
	dispatcher *task.Dispatcher
	scheduler  *task.Scheduler

	// Storage
	storage core.Storage
	redis   *redis.Client

	// Distributed
	node     *distributed.Node
	registry *distributed.Registry
	cluster  *distributed.ClusterManager

	// Configuration
	config *Config
	logger *slog.Logger

	// Runtime
	status      Status
	startTime   time.Time
	stopChan    chan struct{}
	workerGroup sync.WaitGroup

	// Metrics
	metrics *Metrics
	mu      sync.RWMutex
}

// Status represents crawler status
type Status string

const (
	StatusIdle     Status = "idle"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusPausing  Status = "pausing"
	StatusPaused   Status = "paused"
	StatusStopping Status = "stopping"
	StatusStopped  Status = "stopped"
	StatusError    Status = "error"
)

// Config contains crawler configuration
type Config struct {
	// Basic
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Mode    Mode              `json:"mode"` // standalone, distributed, cluster
	Tags    []string          `json:"tags"`
	Labels  map[string]string `json:"labels"`

	// Components
	Fetcher    *fetcher.Config         `json:"fetcher"`
	Pipeline   *pipeline.Config        `json:"pipeline"`
	Storage    *StorageConfig          `json:"storage"`
	TaskQueue  string                  `json:"task_queue"`  // "redis" or "memory"
	Dispatcher *task.DispatcherConfig  `json:"dispatcher"`
	Scheduler  *task.SchedulerConfig   `json:"scheduler"`
	Node       *distributed.NodeConfig `json:"node"`

	// Runtime
	Workers       int           `json:"workers"`
	MaxConcurrent int           `json:"max_concurrent"`
	MaxDepth      int           `json:"max_depth"`
	MaxRetries    int           `json:"max_retries"`
	Timeout       time.Duration `json:"timeout"`

	// Redis
	RedisAddr   string `json:"redis_addr"`
	RedisDB     int    `json:"redis_db"`
	RedisPrefix string `json:"redis_prefix"`

	// Features
	EnableScheduler   bool `json:"enable_scheduler"`
	EnableDistributed bool `json:"enable_distributed"`
	EnableMetrics     bool `json:"enable_metrics"`
	EnableProfiling   bool `json:"enable_profiling"`

	// Logging
	LogLevel string `json:"log_level"`
	LogFile  string `json:"log_file"`
}

// Mode represents crawler mode
type Mode string

const (
	ModeStandalone  Mode = "standalone"
	ModeDistributed Mode = "distributed"
	ModeCluster     Mode = "cluster"
)

// StorageConfig contains storage configuration
type StorageConfig struct {
	Type     string                 `json:"type"` // memory, redis, mongodb, mysql, elasticsearch
	Endpoint string                 `json:"endpoint"`
	Database string                 `json:"database"`
	Options  map[string]interface{} `json:"options"`
}

// Metrics contains crawler metrics
type Metrics struct {
	// Counters
	RequestsTotal   int64 `json:"requests_total"`
	RequestsSuccess int64 `json:"requests_success"`
	RequestsFailed  int64 `json:"requests_failed"`
	BytesDownloaded int64 `json:"bytes_downloaded"`
	ItemsExtracted  int64 `json:"items_extracted"`
	ItemsProcessed  int64 `json:"items_processed"`
	ItemsStored     int64 `json:"items_stored"`
	TasksCreated    int64 `json:"tasks_created"`
	TasksCompleted  int64 `json:"tasks_completed"`
	TasksFailed     int64 `json:"tasks_failed"`

	// Gauges
	QueueSize       int64         `json:"queue_size"`
	ActiveWorkers   int64         `json:"active_workers"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	AvgProcessTime  time.Duration `json:"avg_process_time"`

	// Rates
	RequestRate float64 `json:"request_rate"`
	SuccessRate float64 `json:"success_rate"`
	ErrorRate   float64 `json:"error_rate"`

	// Timing
	StartTime       time.Time     `json:"start_time"`
	LastRequestTime time.Time     `json:"last_request_time"`
	TotalRuntime    time.Duration `json:"total_runtime"`
}

// New creates a new crawler
func New(config *Config) (*Crawler, error) {
	// Validate config
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create logger
	logger := logging.GetLogger().With("service", fmt.Sprintf("crawler-%s", config.ID))

	// Create Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: config.RedisAddr,
		DB:   config.RedisDB,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	crawler := &Crawler{
		ID:       config.ID,
		Name:     config.Name,
		Version:  config.Version,
		Tags:     config.Tags,
		Labels:   config.Labels,
		config:   config,
		logger:   logger,
		redis:    redisClient,
		status:   StatusIdle,
		stopChan: make(chan struct{}),
		metrics:  &Metrics{},
	}

	// Initialize components
	if err := crawler.initComponents(); err != nil {
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	logger.Info("Crawler created",
		"id", config.ID,
		"name", config.Name,
		"mode", config.Mode)

	return crawler, nil
}

// initComponents initializes crawler components
func (c *Crawler) initComponents() error {
	// Initialize fetcher
	if err := c.initFetcher(); err != nil {
		return fmt.Errorf("failed to init fetcher: %w", err)
	}

	// Initialize pipeline
	if err := c.initPipeline(); err != nil {
		return fmt.Errorf("failed to init pipeline: %w", err)
	}

	// Initialize storage
	if err := c.initStorage(); err != nil {
		return fmt.Errorf("failed to init storage: %w", err)
	}

	// Initialize task queue
	if err := c.initTaskQueue(); err != nil {
		return fmt.Errorf("failed to init task queue: %w", err)
	}

	// Initialize dispatcher
	if c.config.Mode != ModeStandalone {
		if err := c.initDispatcher(); err != nil {
			return fmt.Errorf("failed to init dispatcher: %w", err)
		}
	}

	// Initialize scheduler
	if c.config.EnableScheduler {
		if err := c.initScheduler(); err != nil {
			return fmt.Errorf("failed to init scheduler: %w", err)
		}
	}

	// Initialize distributed components
	if c.config.EnableDistributed {
		if err := c.initDistributed(); err != nil {
			return fmt.Errorf("failed to init distributed: %w", err)
		}
	}

	return nil
}

// initFetcher initializes the fetcher
func (c *Crawler) initFetcher() error {
	fetcherConfig := c.config.Fetcher
	if fetcherConfig == nil {
		fetcherConfig = fetcher.DefaultConfig()
	}

	// Create fetcher with options
	opts := []fetcher.Option{
		fetcher.WithLogger(c.logger),
		fetcher.WithConfig(fetcherConfig),
	}

	// Add user agent pool
	uaPool := fetcher.NewUserAgentPool(&fetcher.UserAgentPoolConfig{
		Strategy: fetcher.UserAgentStrategyRandom,
	})
	opts = append(opts, fetcher.WithUserAgentPool(uaPool))

	// Add default rate limiter (10 requests per second)
	rateLimiter := fetcher.NewDefaultRateLimiter(10)
	opts = append(opts, fetcher.WithRateLimiter(rateLimiter))

	// Add cache
	if fetcherConfig.EnableCache {
		cache := fetcher.NewMemoryCache()
		opts = append(opts, fetcher.WithCache(cache))
	}

	c.fetcher = fetcher.New(opts...)
	return nil
}

// initPipeline initializes the pipeline
func (c *Crawler) initPipeline() error {
	pipelineConfig := c.config.Pipeline
	if pipelineConfig == nil {
		pipelineConfig = &pipeline.Config{
			Concurrency: c.config.Workers,
		}
	}

	// Create pipeline builder
	builder := pipeline.NewBuilder("crawler", c.logger)

	// Add default processors
	if pipelineConfig.EnableCleaner {
		builder.Add(pipeline.NewCleanerProcessor(pipeline.CleanerConfig{
			TrimSpace:      true,
			RemoveEmpty:    true,
			NormalizeSpace: true,
		}))
	}

	if pipelineConfig.EnableValidator {
		// Add validation rules based on config
		builder.Add(pipeline.NewValidatorProcessor(pipelineConfig.ValidationRules))
	}

	if pipelineConfig.EnableDeduplicator {
		builder.Add(pipeline.NewDeduplicatorProcessor(pipelineConfig.DedupeKey))
	}

	c.pipeline = builder.Build()
	return nil
}

// initStorage initializes the storage
func (c *Crawler) initStorage() error {
	storageConfig := c.config.Storage
	if storageConfig == nil {
		// Default to Redis storage
		storageConfig = &StorageConfig{
			Type:     "redis",
			Endpoint: c.config.RedisAddr,
			Database: fmt.Sprintf("%s:storage", c.config.RedisPrefix),
		}
	}

	// Create storage based on type
	switch storageConfig.Type {
	case "memory":
		c.storage = NewMemoryStorage()
	case "redis":
		c.storage = NewRedisStorage(c.redis, storageConfig.Database)
	case "mongodb":
		// TODO: Implement MongoDB storage
		return fmt.Errorf("MongoDB storage not yet implemented")
	case "mysql":
		// TODO: Implement MySQL storage
		return fmt.Errorf("MySQL storage not yet implemented")
	default:
		return fmt.Errorf("unknown storage type: %s", storageConfig.Type)
	}

	return nil
}

// initTaskQueue initializes the task queue
func (c *Crawler) initTaskQueue() error {
	queueType := c.config.TaskQueue
	if queueType == "" {
		queueType = "redis"  // Default to Redis queue
	}

	// Create appropriate queue based on type
	var queue task.Queue
	switch queueType {
	case "redis":
		queue = task.NewRedisQueue(c.redis, c.config.RedisPrefix)
	case "memory":
		queue = task.NewPriorityQueue()
	default:
		return fmt.Errorf("unsupported queue type: %s", queueType)
	}

	c.taskQueue = queue
	return nil
}

// initDispatcher initializes the dispatcher
func (c *Crawler) initDispatcher() error {
	dispatcherConfig := c.config.Dispatcher
	if dispatcherConfig == nil {
		dispatcherConfig = &task.DispatcherConfig{
			Strategy:   task.StrategyRoundRobin,
			MaxRetries: c.config.MaxRetries,
			Timeout:    30 * time.Second,
		}
	}

	dispatcherConfig.Logger = c.logger
	c.dispatcher = task.NewDispatcher(dispatcherConfig)
	return nil
}

// initScheduler initializes the scheduler
func (c *Crawler) initScheduler() error {
	schedulerConfig := c.config.Scheduler
	if schedulerConfig == nil {
		schedulerConfig = &task.SchedulerConfig{}
	}

	c.scheduler = task.NewScheduler(schedulerConfig)
	return nil
}

// initDistributed initializes distributed components
func (c *Crawler) initDistributed() error {
	nodeConfig := c.config.Node
	if nodeConfig == nil {
		nodeConfig = &distributed.NodeConfig{
			ID:          c.ID,
			MaxWorkers:  c.config.Workers,
			QueueSize:   1000,
			RedisAddr:   c.config.RedisAddr,
			RedisPrefix: c.config.RedisPrefix,
			LogLevel:    c.config.LogLevel,
		}
	}

	node, err := distributed.NewNode(nodeConfig)
	if err != nil {
		return err
	}

	c.node = node
	c.registry = distributed.NewRegistry(c.redis, c.config.RedisPrefix)

	// Create cluster manager for cluster mode
	if c.config.Mode == ModeCluster {
		c.cluster = distributed.NewClusterManager(c.registry, c.logger)
	}

	return nil
}

// Start starts the crawler
func (c *Crawler) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.status != StatusIdle && c.status != StatusStopped {
		c.mu.Unlock()
		return fmt.Errorf("crawler is already running")
	}
	c.status = StatusStarting
	c.startTime = time.Now()
	c.mu.Unlock()

	c.logger.Info("Starting crawler",
		"id", c.ID,
		"mode", c.config.Mode,
		"workers", c.config.Workers)

	// Start components based on mode
	switch c.config.Mode {
	case ModeStandalone:
		if err := c.startStandalone(ctx); err != nil {
			return err
		}
	case ModeDistributed:
		if err := c.startDistributed(ctx); err != nil {
			return err
		}
	case ModeCluster:
		if err := c.startCluster(ctx); err != nil {
			return err
		}
	}

	// Start scheduler if enabled
	if c.config.EnableScheduler && c.scheduler != nil {
		if err := c.scheduler.Start(ctx); err != nil {
			return fmt.Errorf("failed to start scheduler: %w", err)
		}
	}

	// Start metrics collector
	if c.config.EnableMetrics {
		go c.collectMetrics(ctx)
	}

	c.mu.Lock()
	c.status = StatusRunning
	c.mu.Unlock()

	c.logger.Info("Crawler started successfully", "id", c.ID)
	return nil
}

// startStandalone starts standalone mode
func (c *Crawler) startStandalone(ctx context.Context) error {
	// Start workers
	for i := 0; i < c.config.Workers; i++ {
		c.workerGroup.Add(1)
		go c.runWorker(ctx, i)
	}

	// Start task processor
	go c.processTasks(ctx)

	return nil
}

// startDistributed starts distributed mode
func (c *Crawler) startDistributed(ctx context.Context) error {
	if c.node == nil {
		return fmt.Errorf("node not initialized")
	}

	// Start node
	if err := c.node.Start(ctx); err != nil {
		return fmt.Errorf("failed to start node: %w", err)
	}

	// Start dispatcher if configured
	if c.dispatcher != nil {
		go c.dispatcher.Start(ctx)
	}

	return nil
}

// startCluster starts cluster mode
func (c *Crawler) startCluster(ctx context.Context) error {
	// Start as distributed node
	if err := c.startDistributed(ctx); err != nil {
		return err
	}

	// Start cluster manager
	if c.cluster != nil {
		if err := c.cluster.Start(ctx); err != nil {
			return fmt.Errorf("failed to start cluster manager: %w", err)
		}
	}

	return nil
}

// Stop stops the crawler
func (c *Crawler) Stop(ctx context.Context) error {
	c.mu.Lock()
	if c.status != StatusRunning {
		c.mu.Unlock()
		return fmt.Errorf("crawler is not running")
	}
	c.status = StatusStopping
	c.mu.Unlock()

	c.logger.Info("Stopping crawler", "id", c.ID)

	// Signal stop
	close(c.stopChan)

	// Stop scheduler
	if c.scheduler != nil {
		c.scheduler.Stop()
	}

	// Stop dispatcher
	if c.dispatcher != nil {
		c.dispatcher.Stop()
	}

	// Stop node
	if c.node != nil {
		if err := c.node.Stop(ctx); err != nil {
			c.logger.Error("Failed to stop node", "error", err)
		}
	}

	// Stop cluster manager
	if c.cluster != nil {
		if err := c.cluster.Stop(ctx); err != nil {
			c.logger.Error("Failed to stop cluster manager", "error", err)
		}
	}

	// Wait for workers
	done := make(chan struct{})
	go func() {
		c.workerGroup.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Workers stopped gracefully
	case <-time.After(30 * time.Second):
		c.logger.Warn("Timeout waiting for workers to stop")
	}

	c.mu.Lock()
	c.status = StatusStopped
	c.mu.Unlock()

	c.logger.Info("Crawler stopped", "id", c.ID)
	return nil
}

// runWorker runs a worker goroutine
func (c *Crawler) runWorker(ctx context.Context, id int) {
	defer c.workerGroup.Done()

	workerID := fmt.Sprintf("worker-%d", id)
	c.logger.Debug("Worker started", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		default:
			// Get task from queue
			t, err := c.taskQueue.Pop(ctx)
			if err != nil {
				if err != context.Canceled {
					c.logger.Error("Failed to pop task", "error", err)
				}
				time.Sleep(time.Second)
				continue
			}

			if t == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Process task
			c.processTask(ctx, t, workerID)
		}
	}
}

// processTask processes a single task
func (c *Crawler) processTask(ctx context.Context, t *task.Task, workerID string) {
	startTime := time.Now()
	atomic.AddInt64(&c.metrics.ActiveWorkers, 1)
	defer atomic.AddInt64(&c.metrics.ActiveWorkers, -1)

	c.logger.Debug("Processing task",
		"task_id", t.ID,
		"url", t.URL,
		"worker_id", workerID)

	// Update task status
	t.Start()

	// Create request
	req := &fetcher.Request{
		ID:         t.ID,
		URL:        t.URL,
		Method:     "GET",
		Headers:    t.Headers,
		Timeout:    c.config.Timeout,
		RetryCount: c.config.MaxRetries,
		Metadata:   t.Metadata,
		Priority:   int(t.Priority),
	}

	// Execute request
	resp, err := c.fetcher.Execute(ctx, req)
	atomic.AddInt64(&c.metrics.RequestsTotal, 1)

	if err != nil {
		atomic.AddInt64(&c.metrics.RequestsFailed, 1)
		c.handleTaskError(ctx, t, err)
		return
	}

	atomic.AddInt64(&c.metrics.RequestsSuccess, 1)
	atomic.AddInt64(&c.metrics.BytesDownloaded, int64(len(resp.Body)))

	// Extract data
	data, err := c.extractData(ctx, t, resp)
	if err != nil {
		c.handleTaskError(ctx, t, err)
		return
	}

	atomic.AddInt64(&c.metrics.ItemsExtracted, int64(len(data)))

	// Process through pipeline
	processed, err := c.pipeline.Process(ctx, data)
	if err != nil {
		c.handleTaskError(ctx, t, err)
		return
	}

	// Count processed items based on type
	var processedCount int
	switch v := processed.(type) {
	case []interface{}:
		processedCount = len(v)
	case map[string]interface{}:
		processedCount = len(v)
	default:
		processedCount = 1
	}
	atomic.AddInt64(&c.metrics.ItemsProcessed, int64(processedCount))

	// Store data
	if err := c.storeData(ctx, t, processed); err != nil {
		c.handleTaskError(ctx, t, err)
		return
	}

	atomic.AddInt64(&c.metrics.ItemsStored, int64(processedCount))

	// Extract new URLs and create tasks
	if t.Type == string(task.TypeSeed) {
		newTasks := c.extractTasks(ctx, t, resp, data)
		for _, newTask := range newTasks {
			if err := c.taskQueue.Push(ctx, newTask); err != nil {
				c.logger.Error("Failed to push task", "error", err)
			} else {
				atomic.AddInt64(&c.metrics.TasksCreated, 1)
			}
		}
	}

	// Mark task complete
	t.Complete()
	atomic.AddInt64(&c.metrics.TasksCompleted, 1)

	// Update metrics
	duration := time.Since(startTime)
	c.updateAvgTimes(resp.Duration, duration)

	c.logger.Debug("Task completed",
		"task_id", t.ID,
		"duration", duration)
}

// handleTaskError handles task processing errors
func (c *Crawler) handleTaskError(ctx context.Context, t *task.Task, err error) {
	c.logger.Error("Task failed",
		"task_id", t.ID,
		"url", t.URL,
		"error", err)

	t.Fail(err)
	atomic.AddInt64(&c.metrics.TasksFailed, 1)

	// Retry if possible
	if t.CanRetry() {
		t.IncrementRetry()
		if err := c.taskQueue.Push(ctx, t); err != nil {
			c.logger.Error("Failed to requeue task", "error", err)
		}
	}
}

// extractData extracts data from response
func (c *Crawler) extractData(ctx context.Context, t *task.Task, resp *fetcher.Response) ([]map[string]interface{}, error) {
	// This is a simplified implementation
	// In production, use proper extractors based on task type

	data := []map[string]interface{}{
		{
			"url":         resp.URL,
			"status_code": resp.StatusCode,
			"headers":     resp.Headers,
			"body_size":   len(resp.Body),
			"timestamp":   time.Now(),
		},
	}

	return data, nil
}

// storeData stores processed data
func (c *Crawler) storeData(ctx context.Context, t *task.Task, data interface{}) error {
	// Convert data to slice if needed
	var items []interface{}
	switch v := data.(type) {
	case []interface{}:
		items = v
	case map[string]interface{}:
		items = []interface{}{v}
	default:
		items = []interface{}{data}
	}
	
	// Convert items to the format expected by storage
	dataToStore := make([]map[string]interface{}, len(items))
	for i, item := range items {
		dataMap, ok := item.(map[string]interface{})
		if !ok {
			// Wrap non-map items
			dataMap = map[string]interface{}{
				"data": item,
				"task_id": t.ID,
				"url": t.URL,
				"timestamp": time.Now(),
			}
		}
		dataToStore[i] = dataMap
	}
	
	// Store all items at once
	if err := c.storage.Save(ctx, "crawled_data", dataToStore); err != nil {
		return err
	}
	return nil
}

// extractTasks extracts new tasks from response
func (c *Crawler) extractTasks(ctx context.Context, parent *task.Task, resp *fetcher.Response, data []map[string]interface{}) []*task.Task {
	// This is a simplified implementation
	// In production, extract URLs from HTML/JSON response

	var tasks []*task.Task

	// Example: Create detail tasks from extracted URLs
	for i := 0; i < 5; i++ {
		newTask := &task.Task{
			ID:       fmt.Sprintf("%s-child-%d", parent.ID, i),
			URL:      fmt.Sprintf("%s/page/%d", parent.URL, i),
			Type:     string(task.TypeDetail),
			Priority: int(task.PriorityNormal),
			Depth:    parent.Depth + 1,
			ParentID: parent.ID,
			Metadata: parent.Metadata,
		}

		if newTask.Depth <= c.config.MaxDepth {
			tasks = append(tasks, newTask)
		}
	}

	return tasks
}

// processTasks processes tasks continuously
func (c *Crawler) processTasks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		default:
			// Check if we need more tasks
			queueSize := atomic.LoadInt64(&c.metrics.QueueSize)
			if queueSize < int64(c.config.Workers) {
				// Generate or fetch more tasks
				c.generateTasks(ctx)
			}
			time.Sleep(time.Second)
		}
	}
}

// generateTasks generates new tasks
func (c *Crawler) generateTasks(ctx context.Context) {
	// This is a placeholder for task generation logic
	// In production, this could fetch from database, API, or generate based on rules
}

// collectMetrics collects metrics periodically
func (c *Crawler) collectMetrics(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.updateMetrics()
			c.reportMetrics()
		}
	}
}

// updateMetrics updates metrics
func (c *Crawler) updateMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update rates
	runtime := time.Since(c.startTime).Seconds()
	if runtime > 0 {
		c.metrics.RequestRate = float64(c.metrics.RequestsTotal) / runtime
		c.metrics.SuccessRate = float64(c.metrics.RequestsSuccess) / float64(c.metrics.RequestsTotal) * 100
		c.metrics.ErrorRate = float64(c.metrics.RequestsFailed) / float64(c.metrics.RequestsTotal) * 100
	}

	c.metrics.TotalRuntime = time.Since(c.startTime)

	// Get queue size
	if c.taskQueue != nil {
		size, _ := c.taskQueue.Size(context.Background())
		c.metrics.QueueSize = size
	}
}

// reportMetrics reports metrics
func (c *Crawler) reportMetrics() {
	c.logger.Info("Crawler metrics",
		"requests_total", atomic.LoadInt64(&c.metrics.RequestsTotal),
		"requests_success", atomic.LoadInt64(&c.metrics.RequestsSuccess),
		"requests_failed", atomic.LoadInt64(&c.metrics.RequestsFailed),
		"items_extracted", atomic.LoadInt64(&c.metrics.ItemsExtracted),
		"items_stored", atomic.LoadInt64(&c.metrics.ItemsStored),
		"queue_size", atomic.LoadInt64(&c.metrics.QueueSize),
		"active_workers", atomic.LoadInt64(&c.metrics.ActiveWorkers))
}

// updateAvgTimes updates average response and process times
func (c *Crawler) updateAvgTimes(responseTime, processTime time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple moving average
	if c.metrics.AvgResponseTime == 0 {
		c.metrics.AvgResponseTime = responseTime
	} else {
		c.metrics.AvgResponseTime = (c.metrics.AvgResponseTime + responseTime) / 2
	}

	if c.metrics.AvgProcessTime == 0 {
		c.metrics.AvgProcessTime = processTime
	} else {
		c.metrics.AvgProcessTime = (c.metrics.AvgProcessTime + processTime) / 2
	}

	c.metrics.LastRequestTime = time.Now()
}

// GetStatus returns crawler status
func (c *Crawler) GetStatus() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// GetMetrics returns crawler metrics
func (c *Crawler) GetMetrics() *Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy
	return &Metrics{
		RequestsTotal:   atomic.LoadInt64(&c.metrics.RequestsTotal),
		RequestsSuccess: atomic.LoadInt64(&c.metrics.RequestsSuccess),
		RequestsFailed:  atomic.LoadInt64(&c.metrics.RequestsFailed),
		BytesDownloaded: atomic.LoadInt64(&c.metrics.BytesDownloaded),
		ItemsExtracted:  atomic.LoadInt64(&c.metrics.ItemsExtracted),
		ItemsProcessed:  atomic.LoadInt64(&c.metrics.ItemsProcessed),
		ItemsStored:     atomic.LoadInt64(&c.metrics.ItemsStored),
		TasksCreated:    atomic.LoadInt64(&c.metrics.TasksCreated),
		TasksCompleted:  atomic.LoadInt64(&c.metrics.TasksCompleted),
		TasksFailed:     atomic.LoadInt64(&c.metrics.TasksFailed),
		QueueSize:       atomic.LoadInt64(&c.metrics.QueueSize),
		ActiveWorkers:   atomic.LoadInt64(&c.metrics.ActiveWorkers),
		AvgResponseTime: c.metrics.AvgResponseTime,
		AvgProcessTime:  c.metrics.AvgProcessTime,
		RequestRate:     c.metrics.RequestRate,
		SuccessRate:     c.metrics.SuccessRate,
		ErrorRate:       c.metrics.ErrorRate,
		StartTime:       c.metrics.StartTime,
		LastRequestTime: c.metrics.LastRequestTime,
		TotalRuntime:    c.metrics.TotalRuntime,
	}
}

// AddSeed adds a seed URL to crawl
func (c *Crawler) AddSeed(url string, metadata map[string]interface{}) error {
	t := &task.Task{
		ID:       fmt.Sprintf("seed-%d", time.Now().UnixNano()),
		URL:      url,
		Type:     string(task.TypeSeed),
		Priority: int(task.PriorityHigh),
		Metadata: metadata,
		Status:   task.StatusPending,
		Depth:    0,
	}

	ctx := context.Background()
	if err := c.taskQueue.Push(ctx, t); err != nil {
		return err
	}

	atomic.AddInt64(&c.metrics.TasksCreated, 1)
	return nil
}

// validateConfig validates crawler configuration
func validateConfig(config *Config) error {
	if config.ID == "" {
		config.ID = fmt.Sprintf("crawler-%d", time.Now().Unix())
	}

	if config.Name == "" {
		config.Name = "Default Crawler"
	}

	if config.Version == "" {
		config.Version = "1.0.0"
	}

	if config.Mode == "" {
		config.Mode = ModeStandalone
	}

	if config.Workers <= 0 {
		config.Workers = 10
	}

	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 100
	}

	if config.MaxDepth <= 0 {
		config.MaxDepth = 3
	}

	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}

	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	if config.RedisAddr == "" {
		config.RedisAddr = "localhost:6379"
	}

	if config.RedisPrefix == "" {
		config.RedisPrefix = "crawler"
	}

	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	return nil
}
