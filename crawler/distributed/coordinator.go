package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/communication"
	pb "github.com/NHYCRaymond/go-backend-kit/crawler/proto"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-redis/redis/v8"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Coordinator coordinates task distribution between ClusterManager and Nodes
type Coordinator struct {
	// Components
	cluster        *ClusterManager
	dispatcher     *task.Dispatcher
	hub            *communication.CommunicationHub
	redis          *redis.Client
	logger         *slog.Logger
	scheduler      TaskResultUpdater // Interface for updating task results
	changeDetector ChangeDetector    // Interface for processing venue changes

	// Configuration
	config *CoordinatorConfig

	// State
	mu           sync.RWMutex
	taskAssignments map[string]string // taskID -> nodeID mapping
	pendingTasks    map[string]*task.Task
	
	// Control
	ctx      context.Context
	cancel   context.CancelFunc
	stopChan chan struct{}
}

// TaskResultUpdater interface for updating task results
type TaskResultUpdater interface {
	UpdateTaskResult(ctx context.Context, taskInstanceID string, success bool, errorMsg string)
}

// ChangeDetector interface for processing venue changes
type ChangeDetector interface {
	ProcessTaskResult(taskID string, result map[string]interface{}) error
}

// CoordinatorConfig contains coordinator configuration
type CoordinatorConfig struct {
	RedisPrefix      string        `json:"redis_prefix"`
	MaxRetries       int           `json:"max_retries"`
	TaskTimeout      time.Duration `json:"task_timeout"`
	ReassignInterval time.Duration `json:"reassign_interval"`
	BatchSize        int           `json:"batch_size"`
}

// NewCoordinator creates a new coordinator
func NewCoordinator(config *CoordinatorConfig, cluster *ClusterManager, dispatcher *task.Dispatcher, hub *communication.CommunicationHub, redis *redis.Client, logger *slog.Logger) *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Coordinator{
		cluster:         cluster,
		dispatcher:      dispatcher,
		hub:            hub,
		redis:          redis,
		logger:         logger,
		config:         config,
		taskAssignments: make(map[string]string),
		pendingTasks:   make(map[string]*task.Task),
		ctx:           ctx,
		cancel:        cancel,
		stopChan:      make(chan struct{}),
	}
}

// SetScheduler sets the task result updater (scheduler)
func (c *Coordinator) SetScheduler(scheduler TaskResultUpdater) {
	c.scheduler = scheduler
}

// SetChangeDetector sets the change detector for venue monitoring
func (c *Coordinator) SetChangeDetector(detector ChangeDetector) {
	c.changeDetector = detector
}

// Start starts the coordinator
func (c *Coordinator) Start() error {
	c.logger.Info("Starting coordinator")
	
	// Start node synchronization
	go c.syncNodes()
	
	// Start task distribution loop
	go c.distributeTasks()
	
	// Start task monitoring
	go c.monitorTasks()
	
	// Start failed task reassignment
	go c.reassignFailedTasks()
	
	// Listen for task results
	go c.handleTaskResults()
	
	c.logger.Info("Coordinator started")
	return nil
}

// Stop stops the coordinator
func (c *Coordinator) Stop() error {
	c.logger.Info("Stopping coordinator")
	
	c.cancel()
	close(c.stopChan)
	
	// Wait for goroutines to finish
	time.Sleep(2 * time.Second)
	
	c.logger.Info("Coordinator stopped")
	return nil
}

// distributeTasks distributes tasks to nodes
func (c *Coordinator) distributeTasks() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.distributeTaskBatch()
		}
	}
}

// distributeTaskBatch distributes a batch of tasks
func (c *Coordinator) distributeTaskBatch() {
	// Get available nodes
	nodes := c.cluster.GetNodes()
	if len(nodes) == 0 {
		return
	}
	
	// Get active nodes
	activeNodes := make([]*NodeInfo, 0)
	for _, node := range nodes {
		if node.Status == "active" {
			activeNodes = append(activeNodes, node)
		}
	}
	
	if len(activeNodes) == 0 {
		return
	}
	
	// Get tasks from dispatcher queue
	ctx := context.Background()
	tasksKey := fmt.Sprintf("%s:queue:tasks:pending", c.config.RedisPrefix)
	
	// Pop tasks from Redis queue
	taskIDs, err := c.redis.LRange(ctx, tasksKey, 0, int64(c.config.BatchSize-1)).Result()
	if err != nil || len(taskIDs) == 0 {
		return
	}
	
	// Remove fetched tasks from queue
	c.redis.LTrim(ctx, tasksKey, int64(len(taskIDs)), -1)
	
	// Distribute tasks to nodes
	for i, taskID := range taskIDs {
		// Select node (round-robin for simplicity)
		node := activeNodes[i%len(activeNodes)]
		
		// Get task details
		taskKey := fmt.Sprintf("%s:task:%s", c.config.RedisPrefix, taskID)
		taskData, err := c.redis.Get(ctx, taskKey).Result()
		if err != nil {
			c.logger.Error("Failed to get task", "task_id", taskID, "error", err)
			continue
		}
		
		var t task.Task
		if err := json.Unmarshal([]byte(taskData), &t); err != nil {
			c.logger.Error("Failed to unmarshal task", "task_id", taskID, "error", err)
			continue
		}
		
		// Assign task to node
		if err := c.assignTaskToNode(&t, node); err != nil {
			c.logger.Error("Failed to assign task", 
				"task_id", taskID,
				"node_id", node.ID,
				"error", err)
			
			// Return task to queue
			c.redis.RPush(ctx, tasksKey, taskID)
		}
	}
}

// assignTaskToNode assigns a task to a specific node
func (c *Coordinator) assignTaskToNode(t *task.Task, node *NodeInfo) error {
	// Create task assignment message
	assignment := &pb.TaskAssignment{
		TaskId:         t.ID,
		ParentId:       t.ParentID,  // Include parent task ID
		Url:            t.URL,
		Method:         t.Method,  // Use actual method from task
		Priority:       pb.TaskPriority(t.Priority),
		Headers:        t.Headers,
		Body:           t.Body,  // Include request body
		MaxRetries:     int32(t.MaxRetries),
		TimeoutSeconds: int32(30), // Default 30 seconds
		Metadata:       make(map[string]string),
		TaskType:       t.Type,  // Include task type
		Cookies:        t.Cookies,  // Include cookies
		ProjectId:      t.ProjectID,  // Include project ID for Lua scripts
		LuaScript:      t.LuaScript,  // Include Lua script name
	}
	
	// Default method to GET if not specified
	if assignment.Method == "" {
		assignment.Method = "GET"
	}
	
	// Convert metadata
	for k, v := range t.Metadata {
		if str, ok := v.(string); ok {
			assignment.Metadata[k] = str
		}
	}
	
	// Log Lua script configuration
	if t.ProjectID != "" || t.LuaScript != "" {
		c.logger.Info("Task has Lua script configuration",
			"task_id", t.ID,
			"project_id", t.ProjectID,
			"lua_script", t.LuaScript)
	}
	
	// Convert extract rules
	if len(t.ExtractRules) > 0 {
		assignment.ExtractRules = make([]*pb.ExtractRule, len(t.ExtractRules))
		for i, rule := range t.ExtractRules {
			// Convert type to protobuf enum
			var selectorType pb.SelectorType
			switch rule.Type {
			case "css":
				selectorType = pb.SelectorType_SELECTOR_CSS
			case "xpath":
				selectorType = pb.SelectorType_SELECTOR_XPATH
			case "json", "jsonpath":
				selectorType = pb.SelectorType_SELECTOR_JSONPATH
			case "regex":
				selectorType = pb.SelectorType_SELECTOR_REGEX
			default:
				selectorType = pb.SelectorType_SELECTOR_CSS
			}
			
			assignment.ExtractRules[i] = &pb.ExtractRule{
				Field:        rule.Field,
				Selector:     rule.Selector,
				Type:         selectorType,
				Attribute:    rule.Attribute,
				Multiple:     rule.Multiple,
				DefaultValue: fmt.Sprintf("%v", rule.Default),
			}
		}
		
		c.logger.Info("Task has extract rules",
			"task_id", t.ID,
			"rules_count", len(t.ExtractRules))
	}
	
	// Convert storage config
	if t.StorageConf.Type != "" {
		assignment.StorageConfig = &pb.StorageConfig{
			Type:       t.StorageConf.Type,
			Database:   t.StorageConf.Database,
			Collection: t.StorageConf.Collection,
			Table:      t.StorageConf.Table,
			Bucket:     t.StorageConf.Bucket,
			Path:       t.StorageConf.Path,
			Format:     t.StorageConf.Format,
			Options:    make(map[string]string),
		}
		
		// Convert options
		for k, v := range t.StorageConf.Options {
			if str, ok := v.(string); ok {
				assignment.StorageConfig.Options[k] = str
			}
		}
		
		c.logger.Info("Task has storage config",
			"task_id", t.ID,
			"storage_type", t.StorageConf.Type,
			"database", t.StorageConf.Database,
			"collection", t.StorageConf.Collection)
	}
	
	// Send via gRPC
	msg := &pb.ControlMessage{
		MessageId: generateMessageID(),
		Timestamp: timestamppb.Now(),
		Message: &pb.ControlMessage_TaskAssignment{
			TaskAssignment: assignment,
		},
	}
	
	if err := c.hub.SendToNode(node.ID, msg); err != nil {
		return fmt.Errorf("failed to send task to node: %w", err)
	}
	
	// Record assignment
	c.mu.Lock()
	c.taskAssignments[t.ID] = node.ID
	c.pendingTasks[t.ID] = t
	c.mu.Unlock()
	
	// Store assignment in Redis
	ctx := context.Background()
	assignmentKey := fmt.Sprintf("%s:assignment:%s", c.config.RedisPrefix, t.ID)
	assignmentData := map[string]interface{}{
		"task_id":     t.ID,
		"node_id":     node.ID,
		"assigned_at": time.Now().Unix(),
		"status":      "assigned",
	}
	
	data, _ := json.Marshal(assignmentData)
	// Assignment should complete within 30 minutes
	c.redis.Set(ctx, assignmentKey, data, 30*time.Minute)
	
	c.logger.Debug("Task assigned",
		"task_id", t.ID,
		"node_id", node.ID,
		"url", t.URL)
	
	return nil
}

// monitorTasks monitors task execution
func (c *Coordinator) monitorTasks() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.checkTaskTimeouts()
		}
	}
}

// checkTaskTimeouts checks for timed out tasks
func (c *Coordinator) checkTaskTimeouts() {
	c.mu.RLock()
	tasks := make(map[string]*task.Task)
	for id, t := range c.pendingTasks {
		tasks[id] = t
	}
	c.mu.RUnlock()
	
	now := time.Now()
	ctx := context.Background()
	
	for taskID, t := range tasks {
		// Check if task has timed out
		if t.StartedAt != nil && now.Sub(*t.StartedAt) > c.config.TaskTimeout {
			c.logger.Warn("Task timed out",
				"task_id", taskID,
				"elapsed", now.Sub(*t.StartedAt))
			
			// Mark task as failed
			c.handleTaskFailure(ctx, taskID, "timeout")
		}
	}
}

// handleTaskResults handles task completion results
func (c *Coordinator) handleTaskResults() {
	ctx := context.Background()
	resultChannel := fmt.Sprintf("%s:results", c.config.RedisPrefix)
	
	// Subscribe to result channel
	pubsub := c.redis.Subscribe(ctx, resultChannel)
	defer pubsub.Close()
	
	ch := pubsub.Channel()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case msg := <-ch:
			var result TaskResult
			if err := json.Unmarshal([]byte(msg.Payload), &result); err != nil {
				c.logger.Error("Failed to unmarshal result", "error", err)
				continue
			}
			
			c.handleTaskResult(ctx, &result)
		}
	}
}

// handleTaskResult handles a single task result
func (c *Coordinator) handleTaskResult(ctx context.Context, result *TaskResult) {
	c.mu.Lock()
	// Get task before deleting it (needed for change detector)
	t := c.pendingTasks[result.TaskID]
	delete(c.pendingTasks, result.TaskID)
	nodeID := c.taskAssignments[result.TaskID]
	delete(c.taskAssignments, result.TaskID)
	c.mu.Unlock()
	
	// Update task result in MongoDB if scheduler is available
	if c.scheduler != nil {
		success := result.Error == ""
		c.scheduler.UpdateTaskResult(ctx, result.TaskID, success, result.Error)
	}
	
	if result.Error != "" {
		c.logger.Error("Task failed",
			"task_id", result.TaskID,
			"node_id", nodeID,
			"error", result.Error)
		
		c.handleTaskFailure(ctx, result.TaskID, result.Error)
	} else {
		c.logger.Info("Task completed",
			"task_id", result.TaskID,
			"node_id", nodeID,
			"items", len(result.Data))
		
		// Store full result (not just data) for dashboard display
		resultKey := fmt.Sprintf("%s:result:%s", c.config.RedisPrefix, result.TaskID)
		data, err := json.Marshal(result)  // Store the full TaskResult, not just Data
		if err != nil {
			c.logger.Error("Failed to marshal task result", 
				"task_id", result.TaskID,
				"error", err)
			return
		}
		
		// Only keep results for 30 minutes due to large data volume
		if err := c.redis.Set(ctx, resultKey, data, 30*time.Minute).Err(); err != nil {
			c.logger.Error("Failed to store task result in Redis",
				"task_id", result.TaskID,
				"error", err)
		}
		
		// Process with change detector for venue monitoring
		if c.changeDetector != nil && t != nil {
				// Prepare result data with source information
				resultData := make(map[string]interface{})
				
				// Merge all data from result
				if len(result.Data) > 0 {
					// Combine all data maps
					for _, dataMap := range result.Data {
						for k, v := range dataMap {
							resultData[k] = v
						}
					}
				}
				
				// Add task metadata
				resultData["task_id"] = result.TaskID
				// Extract source from task metadata or project ID
				if t.ProjectID != "" {
					resultData["source"] = t.ProjectID
				} else if source, ok := t.Metadata["source"].(string); ok {
					resultData["source"] = source
				}
				resultData["timestamp"] = result.EndTime
				
				// Process with change detector
				go func() {
					if err := c.changeDetector.ProcessTaskResult(result.TaskID, resultData); err != nil {
						c.logger.Error("Failed to process task result with change detector",
							"task_id", result.TaskID,
							"error", err)
					}
				}()
		}
		
		// Process seed expansion if needed
		if t != nil && t.Type == string(task.TypeSeed) {
			c.processSeedResult(ctx, result)
		}
	}
}

// handleTaskFailure handles task failure
func (c *Coordinator) handleTaskFailure(ctx context.Context, taskID string, reason string) {
	// Get task details
	c.mu.RLock()
	t, exists := c.pendingTasks[taskID]
	c.mu.RUnlock()
	
	if !exists {
		return
	}
	
	// Check retry count
	if t.RetryCount < t.MaxRetries {
		// Retry task
		t.RetryCount++
		t.Status = task.StatusRetrying
		
		c.logger.Info("Retrying task",
			"task_id", taskID,
			"retry", t.RetryCount,
			"max_retries", t.MaxRetries)
		
		// Requeue task
		tasksKey := fmt.Sprintf("%s:queue:tasks:pending", c.config.RedisPrefix)
		c.redis.RPush(ctx, tasksKey, taskID)
		
		// Update task in Redis
		taskKey := fmt.Sprintf("%s:task:%s", c.config.RedisPrefix, taskID)
		data, _ := json.Marshal(t)
		// Task should be processed within 1 hour
		c.redis.Set(ctx, taskKey, data, 1*time.Hour)
	} else {
		// Mark as permanently failed
		t.Status = task.StatusFailed
		c.logger.Error("Task permanently failed",
			"task_id", taskID,
			"reason", reason)
		
		// Store failure
		failureKey := fmt.Sprintf("%s:failed:%s", c.config.RedisPrefix, taskID)
		failureData := map[string]interface{}{
			"task_id":    taskID,
			"reason":     reason,
			"failed_at":  time.Now().Unix(),
			"retry_count": t.RetryCount,
		}
		data, _ := json.Marshal(failureData)
		// Keep failure info for 1 hour for debugging
		c.redis.Set(ctx, failureKey, data, 1*time.Hour)
	}
}

// processSeedResult processes seed task results for expansion
func (c *Coordinator) processSeedResult(ctx context.Context, result *TaskResult) {
	// Extract URLs from seed result
	urls := extractURLsFromData(result.Data)
	
	c.logger.Info("Processing seed result",
		"task_id", result.TaskID,
		"new_urls", len(urls))
	
	// Create new tasks from URLs
	for _, url := range urls {
		newTask := task.NewTask(url, string(task.TypeDetail), int(task.PriorityNormal))
		newTask.ParentID = result.TaskID  // Set the actual ParentID field
		newTask.SetMetadata("parent_id", result.TaskID)
		newTask.SetMetadata("source", "seed_expansion")
		
		// Submit to dispatcher
		if err := c.dispatcher.Submit(ctx, newTask); err != nil {
			c.logger.Error("Failed to submit expanded task",
				"url", url,
				"error", err)
		}
	}
}

// reassignFailedTasks reassigns tasks from failed nodes
func (c *Coordinator) reassignFailedTasks() {
	ticker := time.NewTicker(c.config.ReassignInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.checkAndReassignTasks()
		}
	}
}

// checkAndReassignTasks checks for tasks on dead nodes and reassigns them
func (c *Coordinator) checkAndReassignTasks() {
	nodes := c.cluster.GetNodes()
	deadNodes := make([]string, 0)
	
	// Find dead nodes
	for nodeID, node := range nodes {
		if node.Status != "active" && time.Since(node.UpdatedAt) > 60*time.Second {
			deadNodes = append(deadNodes, nodeID)
		}
	}
	
	if len(deadNodes) == 0 {
		return
	}
	
	c.logger.Info("Found dead nodes, reassigning tasks",
		"count", len(deadNodes))
	
	// Reassign tasks from dead nodes
	c.mu.Lock()
	tasksToReassign := make([]*task.Task, 0)
	for taskID, nodeID := range c.taskAssignments {
		for _, deadNode := range deadNodes {
			if nodeID == deadNode {
				if t, ok := c.pendingTasks[taskID]; ok {
					tasksToReassign = append(tasksToReassign, t)
					delete(c.taskAssignments, taskID)
				}
				break
			}
		}
	}
	c.mu.Unlock()
	
	// Requeue tasks
	ctx := context.Background()
	tasksKey := fmt.Sprintf("%s:queue:tasks:pending", c.config.RedisPrefix)
	
	for _, t := range tasksToReassign {
		c.redis.RPush(ctx, tasksKey, t.ID)
		c.logger.Info("Reassigned task from dead node",
			"task_id", t.ID)
	}
}

// GetStats returns coordinator statistics
func (c *Coordinator) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return map[string]interface{}{
		"pending_tasks":    len(c.pendingTasks),
		"task_assignments": len(c.taskAssignments),
		"cluster_status":   c.cluster.GetStatus(),
		"active_nodes":     len(c.cluster.GetNodes()),
	}
}

// syncNodes synchronizes nodes from ClusterManager to Dispatcher
func (c *Coordinator) syncNodes() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.performNodeSync()
		}
	}
}

// performNodeSync performs actual node synchronization
func (c *Coordinator) performNodeSync() {
	// Get nodes from ClusterManager
	clusterNodes := c.cluster.GetNodes()
	
	for nodeID, nodeInfo := range clusterNodes {
		// Convert ClusterManager NodeInfo to Dispatcher NodeInfo
		dispatcherNode := &task.NodeInfo{
			ID:            nodeInfo.ID,
			Address:       fmt.Sprintf("%s:%d", nodeInfo.IP, nodeInfo.Port),
			Capacity:      nodeInfo.MaxWorkers,
			CurrentLoad:   0, // Will be updated by heartbeat
			Status:        nodeInfo.Status,
			LastHeartbeat: nodeInfo.LastHeartbeat,
			Tags:          nodeInfo.Tags,
			Capabilities:  nodeInfo.Capabilities,
		}
		
		// Register node with Dispatcher
		if err := c.dispatcher.RegisterNode(dispatcherNode); err != nil {
			c.logger.Error("Failed to register node with dispatcher",
				"node_id", nodeID,
				"error", err)
		} else {
			c.logger.Debug("Node synced to dispatcher",
				"node_id", nodeID,
				"status", nodeInfo.Status)
		}
		
		// Update heartbeat from ClusterManager info
		if nodeInfo.Status == "active" {
			// Estimate load based on ClusterManager metrics
			// This is a simplified approach - in production you'd get actual metrics
			c.dispatcher.UpdateNodeHeartbeat(nodeID, 0)
		}
	}
	
	c.logger.Debug("Node sync completed",
		"cluster_nodes", len(clusterNodes))
}

// extractURLsFromData extracts URLs from task result data
func extractURLsFromData(data interface{}) []string {
	urls := make([]string, 0)
	
	// This is a simplified implementation
	// In practice, this would parse the data structure
	// and extract URLs based on extraction rules
	
	if m, ok := data.(map[string]interface{}); ok {
		if links, ok := m["links"].([]interface{}); ok {
			for _, link := range links {
				if url, ok := link.(string); ok {
					urls = append(urls, url)
				}
			}
		}
	}
	
	return urls
}