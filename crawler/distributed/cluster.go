package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// ClusterManager manages a cluster of crawler nodes
type ClusterManager struct {
	// Identity
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`

	// Components
	registry *Registry
	logger   *slog.Logger
	redis    *redis.Client

	// State
	nodes      map[string]*NodeInfo
	nodesMu    sync.RWMutex
	leader     string
	leaderLock sync.RWMutex

	// Configuration
	config *ClusterConfig

	// Control
	stopChan chan struct{}
	status   ClusterStatus
	mu       sync.RWMutex
}

// ClusterStatus represents cluster status
type ClusterStatus string

const (
	ClusterStatusInitializing ClusterStatus = "initializing"
	ClusterStatusRunning      ClusterStatus = "running"
	ClusterStatusDegraded     ClusterStatus = "degraded"
	ClusterStatusStopping     ClusterStatus = "stopping"
	ClusterStatusStopped      ClusterStatus = "stopped"
)

// ClusterConfig contains cluster configuration
type ClusterConfig struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	LeaderElection    bool          `json:"leader_election"`
	HeartbeatTimeout  time.Duration `json:"heartbeat_timeout"`
	RebalanceEnabled  bool          `json:"rebalance_enabled"`
	RebalanceInterval time.Duration `json:"rebalance_interval"`
	MinNodes          int           `json:"min_nodes"`
	MaxNodes          int           `json:"max_nodes"`
}

// NewClusterManager creates a new cluster manager
func NewClusterManager(registry *Registry, logger *slog.Logger) *ClusterManager {
	return &ClusterManager{
		ID:       fmt.Sprintf("cluster-%d", time.Now().Unix()),
		Name:     "default-cluster",
		Version:  "1.0.0",
		registry: registry,
		logger:   logger,
		nodes:    make(map[string]*NodeInfo),
		stopChan: make(chan struct{}),
		status:   ClusterStatusStopped,
		config: &ClusterConfig{
			HeartbeatTimeout:  30 * time.Second,
			RebalanceInterval: 60 * time.Second,
			MinNodes:          1,
			MaxNodes:          100,
		},
	}
}

// Start starts the cluster manager
func (cm *ClusterManager) Start(ctx context.Context) error {
	cm.mu.Lock()
	if cm.status != ClusterStatusStopped {
		cm.mu.Unlock()
		return fmt.Errorf("cluster manager already running")
	}
	cm.status = ClusterStatusInitializing
	cm.mu.Unlock()

	cm.logger.Info("Starting cluster manager", "id", cm.ID, "name", cm.Name)

	// Start monitoring nodes
	go cm.monitorNodes(ctx)

	// Start leader election if enabled
	if cm.config.LeaderElection {
		go cm.runLeaderElection(ctx)
	}

	// Start rebalancing if enabled
	if cm.config.RebalanceEnabled {
		go cm.runRebalancing(ctx)
	}

	// Start metrics collection
	go cm.collectMetrics(ctx)

	cm.mu.Lock()
	cm.status = ClusterStatusRunning
	cm.mu.Unlock()

	cm.logger.Info("Cluster manager started", "id", cm.ID)
	return nil
}

// Stop stops the cluster manager
func (cm *ClusterManager) Stop(ctx context.Context) error {
	cm.mu.Lock()
	if cm.status != ClusterStatusRunning {
		cm.mu.Unlock()
		return fmt.Errorf("cluster manager not running")
	}
	cm.status = ClusterStatusStopping
	cm.mu.Unlock()

	cm.logger.Info("Stopping cluster manager", "id", cm.ID)

	// Signal stop
	close(cm.stopChan)

	// Wait for goroutines to finish
	time.Sleep(2 * time.Second)

	cm.mu.Lock()
	cm.status = ClusterStatusStopped
	cm.mu.Unlock()

	cm.logger.Info("Cluster manager stopped", "id", cm.ID)
	return nil
}

// monitorNodes monitors cluster nodes
func (cm *ClusterManager) monitorNodes(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopChan:
			return
		case <-ticker.C:
			cm.updateNodeList(ctx)
			cm.checkNodeHealth(ctx)
			cm.updateClusterStatus()
		}
	}
}

// updateNodeList updates the list of nodes
func (cm *ClusterManager) updateNodeList(ctx context.Context) {
	nodes, err := cm.registry.ListNodes(ctx)
	if err != nil {
		cm.logger.Error("Failed to list nodes", "error", err)
		return
	}

	cm.nodesMu.Lock()
	defer cm.nodesMu.Unlock()

	// Clear existing nodes
	cm.nodes = make(map[string]*NodeInfo)

	// Add current nodes
	for _, node := range nodes {
		cm.nodes[node.ID] = node
	}

	cm.logger.Debug("Updated node list", "count", len(cm.nodes))
}

// checkNodeHealth checks health of all nodes
func (cm *ClusterManager) checkNodeHealth(ctx context.Context) {
	cm.nodesMu.RLock()
	nodes := make([]*NodeInfo, 0, len(cm.nodes))
	for _, node := range cm.nodes {
		nodes = append(nodes, node)
	}
	cm.nodesMu.RUnlock()

	now := time.Now()
	for _, node := range nodes {
		// Check heartbeat timeout
		if now.Sub(node.UpdatedAt) > cm.config.HeartbeatTimeout {
			cm.logger.Warn("Node heartbeat timeout", "node_id", node.ID)
			cm.handleNodeFailure(ctx, node.ID)
		}
	}
}

// handleNodeFailure handles node failure
func (cm *ClusterManager) handleNodeFailure(ctx context.Context, nodeID string) {
	cm.logger.Error("Node failed", "node_id", nodeID)

	// Remove from registry
	if err := cm.registry.UnregisterNode(ctx, nodeID); err != nil {
		cm.logger.Error("Failed to unregister node", "node_id", nodeID, "error", err)
	}

	// Remove from local list
	cm.nodesMu.Lock()
	delete(cm.nodes, nodeID)
	cm.nodesMu.Unlock()

	// Trigger rebalancing if needed
	if cm.config.RebalanceEnabled {
		cm.triggerRebalance(ctx)
	}
}

// updateClusterStatus updates cluster status based on node health
func (cm *ClusterManager) updateClusterStatus() {
	cm.nodesMu.RLock()
	nodeCount := len(cm.nodes)
	healthyCount := 0
	for _, node := range cm.nodes {
		if node.Status == "active" {
			healthyCount++
		}
	}
	cm.nodesMu.RUnlock()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if healthyCount == 0 {
		cm.status = ClusterStatusStopped
	} else if healthyCount < cm.config.MinNodes {
		cm.status = ClusterStatusDegraded
	} else {
		cm.status = ClusterStatusRunning
	}

	cm.logger.Debug("Cluster status updated",
		"status", cm.status,
		"total_nodes", nodeCount,
		"healthy_nodes", healthyCount)
}

// runLeaderElection runs leader election
func (cm *ClusterManager) runLeaderElection(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopChan:
			return
		case <-ticker.C:
			cm.electLeader(ctx)
		}
	}
}

// electLeader elects a cluster leader
func (cm *ClusterManager) electLeader(ctx context.Context) {
	cm.nodesMu.RLock()
	nodes := make([]*NodeInfo, 0, len(cm.nodes))
	for _, node := range cm.nodes {
		if node.Status == "active" {
			nodes = append(nodes, node)
		}
	}
	cm.nodesMu.RUnlock()

	if len(nodes) == 0 {
		return
	}

	// Simple leader election: choose node with lowest ID
	leader := nodes[0]
	for _, node := range nodes[1:] {
		if node.ID < leader.ID {
			leader = node
		}
	}

	cm.leaderLock.Lock()
	oldLeader := cm.leader
	cm.leader = leader.ID
	cm.leaderLock.Unlock()

	if oldLeader != leader.ID {
		cm.logger.Info("New leader elected", "leader_id", leader.ID)
	}
}

// runRebalancing runs task rebalancing
func (cm *ClusterManager) runRebalancing(ctx context.Context) {
	ticker := time.NewTicker(cm.config.RebalanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopChan:
			return
		case <-ticker.C:
			cm.rebalanceTasks(ctx)
		}
	}
}

// triggerRebalance triggers immediate rebalancing
func (cm *ClusterManager) triggerRebalance(ctx context.Context) {
	go cm.rebalanceTasks(ctx)
}

// rebalanceTasks rebalances tasks across nodes
func (cm *ClusterManager) rebalanceTasks(ctx context.Context) {
	cm.logger.Debug("Starting task rebalancing")

	cm.nodesMu.RLock()
	nodes := make([]*NodeInfo, 0, len(cm.nodes))
	for _, node := range cm.nodes {
		if node.Status == "active" {
			nodes = append(nodes, node)
		}
	}
	cm.nodesMu.RUnlock()

	if len(nodes) < 2 {
		return // No need to rebalance with single node
	}

	// Get task distribution
	taskCounts := make(map[string]int)
	totalTasks := 0
	for _, node := range nodes {
		// Get task count from metrics
		count := node.Metrics.TasksInQueue
		taskCounts[node.ID] = count
		totalTasks += count
	}

	// Calculate ideal distribution
	idealPerNode := totalTasks / len(nodes)

	// Identify overloaded and underloaded nodes
	overloaded := []string{}
	underloaded := []string{}

	for nodeID, count := range taskCounts {
		if count > idealPerNode+10 { // Threshold of 10 tasks
			overloaded = append(overloaded, nodeID)
		} else if count < idealPerNode-10 {
			underloaded = append(underloaded, nodeID)
		}
	}

	// Perform rebalancing
	if len(overloaded) > 0 && len(underloaded) > 0 {
		cm.logger.Info("Rebalancing tasks",
			"overloaded", len(overloaded),
			"underloaded", len(underloaded))

		// Implement actual task migration
		cm.migrateTasksBetweenNodes(ctx, overloaded, underloaded, idealPerNode)
	}
}

// migrateTasksBetweenNodes migrates tasks from overloaded to underloaded nodes
func (cm *ClusterManager) migrateTasksBetweenNodes(ctx context.Context, overloaded, underloaded []string, idealPerNode int) {
	cm.logger.Debug("Starting task migration",
		"from_nodes", overloaded,
		"to_nodes", underloaded)

	// For each overloaded node
	for _, sourceNodeID := range overloaded {
		// Get tasks from the overloaded node's queue
		queueKey := fmt.Sprintf("%s:queue:tasks:%s", "crawler", sourceNodeID)
		
		// Calculate how many tasks to migrate
		sourceNode := cm.nodes[sourceNodeID]
		if sourceNode == nil || sourceNode.Metrics == nil {
			continue
		}
		
		excessTasks := sourceNode.Metrics.TasksInQueue - idealPerNode
		if excessTasks <= 0 {
			continue
		}

		// Migrate tasks to underloaded nodes
		targetIndex := 0
		tasksToMigrate := int(excessTasks)
		
		for i := 0; i < tasksToMigrate && targetIndex < len(underloaded); i++ {
			targetNodeID := underloaded[targetIndex]
			targetNode := cm.nodes[targetNodeID]
			
			if targetNode == nil || targetNode.Metrics == nil {
				targetIndex++
				continue
			}
			
			// Check target node capacity
			targetCapacity := idealPerNode - targetNode.Metrics.TasksInQueue
			if targetCapacity <= 0 {
				targetIndex++
				continue
			}
			
			// Pop task from source queue
			taskID, err := cm.redis.RPop(ctx, queueKey).Result()
			if err != nil {
				cm.logger.Debug("No more tasks to migrate from node", "node_id", sourceNodeID)
				break
			}
			
			// Push task to target queue
			targetQueueKey := fmt.Sprintf("%s:queue:tasks:%s", "crawler", targetNodeID)
			if err := cm.redis.LPush(ctx, targetQueueKey, taskID).Err(); err != nil {
				cm.logger.Error("Failed to migrate task",
					"task_id", taskID,
					"from", sourceNodeID,
					"to", targetNodeID,
					"error", err)
				
				// Return task to source queue
				cm.redis.LPush(ctx, queueKey, taskID)
				continue
			}
			
			// Update task assignment in Redis
			assignmentKey := fmt.Sprintf("%s:assignment:%s", "crawler", taskID)
			assignment := map[string]interface{}{
				"node_id":     targetNodeID,
				"migrated_at": time.Now().Unix(),
				"from_node":   sourceNodeID,
			}
			
			data, _ := json.Marshal(assignment)
			// Assignment should complete within 30 minutes
			cm.redis.Set(ctx, assignmentKey, data, 30*time.Minute)
			
			cm.logger.Info("Migrated task",
				"task_id", taskID,
				"from", sourceNodeID,
				"to", targetNodeID)
			
			// Update metrics
			if sourceNode.Metrics != nil {
				sourceNode.Metrics.TasksInQueue--
			}
			if targetNode.Metrics != nil {
				targetNode.Metrics.TasksInQueue++
			}
			
			// Move to next target if current is full
			if targetNode.Metrics.TasksInQueue >= idealPerNode {
				targetIndex++
			}
		}
	}
	
	cm.logger.Info("Task migration completed")
}

// collectMetrics collects cluster metrics
func (cm *ClusterManager) collectMetrics(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopChan:
			return
		case <-ticker.C:
			metrics := cm.getClusterMetrics()
			cm.storeMetrics(ctx, metrics)
		}
	}
}

// getClusterMetrics gets cluster metrics
func (cm *ClusterManager) getClusterMetrics() *ClusterMetrics {
	cm.nodesMu.RLock()
	defer cm.nodesMu.RUnlock()

	metrics := &ClusterMetrics{
		ClusterID:    cm.ID,
		Timestamp:    time.Now(),
		TotalNodes:   len(cm.nodes),
		ActiveNodes:  0,
		TotalTasks:   0,
		TotalWorkers: 0,
	}

	for _, node := range cm.nodes {
		if node.Status == "active" {
			metrics.ActiveNodes++
		}
		metrics.TotalWorkers += node.MaxWorkers
		if node.Metrics != nil {
			metrics.TotalTasks += int(node.Metrics.TasksProcessed)
			metrics.TotalTasksFailed += node.Metrics.TasksFailed
			metrics.TotalBytesDownloaded += node.Metrics.BytesDownloaded
			metrics.TotalItemsExtracted += node.Metrics.ItemsExtracted
		}
	}

	return metrics
}

// storeMetrics stores cluster metrics
func (cm *ClusterManager) storeMetrics(ctx context.Context, metrics *ClusterMetrics) {
	if cm.redis == nil {
		return
	}

	key := fmt.Sprintf("cluster:metrics:%s", cm.ID)
	data, _ := json.Marshal(metrics)
	cm.redis.Set(ctx, key, data, time.Hour)
}

// GetStatus returns cluster status
func (cm *ClusterManager) GetStatus() ClusterStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.status
}

// GetLeader returns current leader node ID
func (cm *ClusterManager) GetLeader() string {
	cm.leaderLock.RLock()
	defer cm.leaderLock.RUnlock()
	return cm.leader
}

// GetNodes returns all nodes in the cluster
func (cm *ClusterManager) GetNodes() map[string]*NodeInfo {
	cm.nodesMu.RLock()
	defer cm.nodesMu.RUnlock()

	nodes := make(map[string]*NodeInfo)
	for k, v := range cm.nodes {
		nodes[k] = v
	}
	return nodes
}

// IsHealthy checks if cluster is healthy
func (cm *ClusterManager) IsHealthy() bool {
	cm.mu.RLock()
	status := cm.status
	cm.mu.RUnlock()

	return status == ClusterStatusRunning
}

// ClusterMetrics contains cluster-wide metrics
type ClusterMetrics struct {
	ClusterID            string        `json:"cluster_id"`
	Timestamp            time.Time     `json:"timestamp"`
	TotalNodes           int           `json:"total_nodes"`
	ActiveNodes          int           `json:"active_nodes"`
	TotalWorkers         int           `json:"total_workers"`
	TotalTasks           int           `json:"total_tasks"`
	TotalTasksFailed     int64         `json:"total_tasks_failed"`
	TotalBytesDownloaded int64         `json:"total_bytes_downloaded"`
	TotalItemsExtracted  int64         `json:"total_items_extracted"`
	AvgResponseTime      time.Duration `json:"avg_response_time"`
	ErrorRate            float64       `json:"error_rate"`
}
