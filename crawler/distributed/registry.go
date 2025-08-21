package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// Registry manages node registration and discovery
type Registry struct {
	redis  *redis.Client
	prefix string
	mu     sync.RWMutex

	// Cache
	nodes map[string]*NodeInfo

	// Watchers
	watchers []NodeWatcher
}

// NodeMetrics contains node performance metrics
type NodeMetrics struct {
	TasksProcessed  int64         `json:"tasks_processed"`
	TasksInQueue    int           `json:"tasks_in_queue"`
	TasksFailed     int64         `json:"tasks_failed"`
	BytesDownloaded int64         `json:"bytes_downloaded"`
	BytesUploaded   int64         `json:"bytes_uploaded"`
	ItemsExtracted  int64         `json:"items_extracted"`
	AverageLatency  time.Duration `json:"average_latency"`
	ErrorRate       float64       `json:"error_rate"`
	
	// Real-time metrics
	PagesPerSecond   float64       `json:"pages_per_second"`
	BytesPerSecond   float64       `json:"bytes_per_second"`
	ActiveConnections int          `json:"active_connections"`
	QueueDepth       int           `json:"queue_depth"`
	
	// URL metrics
	URLsDiscovered   int64         `json:"urls_discovered"`
	URLsDuplicated   int64         `json:"urls_duplicated"`
	DedupeRate       float64       `json:"dedupe_rate"`
	
	// Task execution metrics
	TasksPending     int           `json:"tasks_pending"`
	TasksRunning     int           `json:"tasks_running"`
	TasksRetrying    int           `json:"tasks_retrying"`
	AverageExecTime  time.Duration `json:"average_exec_time"`
	
	// Error breakdown
	NetworkErrors    int64         `json:"network_errors"`
	ParseErrors      int64         `json:"parse_errors"`
	TimeoutErrors    int64         `json:"timeout_errors"`
	OtherErrors      int64         `json:"other_errors"`
}

// NodeInfo contains node information for registry
type NodeInfo struct {
	ID            string            `json:"id"`
	Hostname      string            `json:"hostname"`
	IP            string            `json:"ip"`
	Port          int               `json:"port"`
	Status        string            `json:"status"`
	Tags          []string          `json:"tags"`
	Labels        map[string]string `json:"labels"`
	Capabilities  []string          `json:"capabilities"`
	MaxWorkers    int               `json:"max_workers"`
	CPUCores      int               `json:"cpu_cores"`
	MemoryGB      float64           `json:"memory_gb"`
	StartedAt     time.Time         `json:"started_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	LastHeartbeat time.Time         `json:"last_heartbeat"`

	// Metrics
	TasksProcessed int64        `json:"tasks_processed"`
	TasksFailed    int64        `json:"tasks_failed"`
	CPUUsage       float64      `json:"cpu_usage"`
	MemoryUsage    float64      `json:"memory_usage"`
	ActiveWorkers  int          `json:"active_workers"`
	Metrics        *NodeMetrics `json:"metrics,omitempty"`
}

// Heartbeat represents a node heartbeat
type Heartbeat struct {
	NodeID         string    `json:"node_id"`
	Status         string    `json:"status"`
	ActiveWorkers  int       `json:"active_workers"`
	MaxWorkers     int       `json:"max_workers"`
	TasksProcessed int64     `json:"tasks_processed"`
	TasksFailed    int64     `json:"tasks_failed"`
	CPUUsage       float64   `json:"cpu_usage"`
	MemoryUsage    float64   `json:"memory_usage"`
	Timestamp      time.Time `json:"timestamp"`
}

// NodeWatcher watches for node changes
type NodeWatcher interface {
	OnNodeJoin(node *NodeInfo)
	OnNodeLeave(nodeID string)
	OnNodeUpdate(node *NodeInfo)
}

// NewRegistry creates a new registry
func NewRegistry(redisClient *redis.Client, prefix string) *Registry {
	return &Registry{
		redis:    redisClient,
		prefix:   prefix,
		nodes:    make(map[string]*NodeInfo),
		watchers: []NodeWatcher{},
	}
}

// RegisterNode registers a new node
func (r *Registry) RegisterNode(ctx context.Context, node *NodeInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Log registration for debugging
	// fmt.Printf("[Registry] RegisterNode called for %s at %s\n", node.ID, time.Now().Format("15:04:05"))

	// Update timestamp
	node.UpdatedAt = time.Now()
	node.LastHeartbeat = time.Now()

	// Serialize node info
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal node info: %w", err)
	}

	// Store in Redis
	key := r.getNodeKey(node.ID)
	// Set with 1 hour TTL (nodes should send heartbeat every 30 seconds)
	if err := r.redis.Set(ctx, key, data, 1*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to store node info: %w", err)
	}

	// Add to active nodes sorted set with timestamp as score
	activeKey := r.getActiveNodesKey()
	score := float64(time.Now().Unix())
	if err := r.redis.ZAdd(ctx, activeKey, &redis.Z{
		Score:  score,
		Member: node.ID,
	}).Err(); err != nil {
		return fmt.Errorf("failed to add to active nodes: %w", err)
	}

	// Store node capabilities
	if len(node.Capabilities) > 0 {
		for _, capability := range node.Capabilities {
			capKey := r.getCapabilityKey(capability)
			r.redis.SAdd(ctx, capKey, node.ID)
			// Set TTL for capability set (1 hour, refreshed on heartbeat)
			r.redis.Expire(ctx, capKey, 1*time.Hour)
		}
	}

	// Store node tags
	if len(node.Tags) > 0 {
		for _, tag := range node.Tags {
			tagKey := r.getTagKey(tag)
			r.redis.SAdd(ctx, tagKey, node.ID)
			// Set TTL for tag set (1 hour, refreshed on heartbeat)
			r.redis.Expire(ctx, tagKey, 1*time.Hour)
		}
	}

	// Update cache
	r.nodes[node.ID] = node

	// Notify watchers
	for _, watcher := range r.watchers {
		go watcher.OnNodeJoin(node)
	}

	// Publish event
	event := map[string]interface{}{
		"type":    "node_join",
		"node_id": node.ID,
		"time":    time.Now(),
	}
	eventData, _ := json.Marshal(event)
	r.redis.Publish(ctx, r.getEventChannel(), eventData)

	return nil
}

// UnregisterNode unregisters a node
func (r *Registry) UnregisterNode(ctx context.Context, nodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get node info
	node, exists := r.nodes[nodeID]
	if !exists {
		// Try to load from Redis
		key := r.getNodeKey(nodeID)
		data, err := r.redis.Get(ctx, key).Result()
		if err == nil {
			json.Unmarshal([]byte(data), &node)
		}
	}

	// Remove from Redis
	key := r.getNodeKey(nodeID)
	r.redis.Del(ctx, key)

	// Remove from active nodes sorted set
	activeKey := r.getActiveNodesKey()
	r.redis.ZRem(ctx, activeKey, nodeID)

	// Remove from capability sets
	if node != nil && len(node.Capabilities) > 0 {
		for _, capability := range node.Capabilities {
			capKey := r.getCapabilityKey(capability)
			r.redis.SRem(ctx, capKey, nodeID)
		}
	}

	// Remove from tag sets
	if node != nil && len(node.Tags) > 0 {
		for _, tag := range node.Tags {
			tagKey := r.getTagKey(tag)
			r.redis.SRem(ctx, tagKey, nodeID)
		}
	}

	// Remove heartbeat
	heartbeatKey := r.getHeartbeatKey(nodeID)
	r.redis.Del(ctx, heartbeatKey)

	// Remove from cache
	delete(r.nodes, nodeID)

	// Notify watchers
	for _, watcher := range r.watchers {
		go watcher.OnNodeLeave(nodeID)
	}

	// Publish event
	event := map[string]interface{}{
		"type":    "node_leave",
		"node_id": nodeID,
		"time":    time.Now(),
	}
	eventData, _ := json.Marshal(event)
	r.redis.Publish(ctx, r.getEventChannel(), eventData)

	return nil
}

// UpdateHeartbeat updates node heartbeat
func (r *Registry) UpdateHeartbeat(ctx context.Context, heartbeat *Heartbeat) error {
	// Use a pipeline to ensure atomic updates
	pipe := r.redis.Pipeline()
	
	// Store heartbeat with expiry
	heartbeatKey := r.getHeartbeatKey(heartbeat.NodeID)
	heartbeatData, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}
	pipe.Set(ctx, heartbeatKey, heartbeatData, 60*time.Second)

	// Update the timestamp in the sorted set to keep node active
	activeKey := r.getActiveNodesKey()
	score := float64(time.Now().Unix())
	pipe.ZAdd(ctx, activeKey, &redis.Z{
		Score:  score,
		Member: heartbeat.NodeID,
	})

	// Execute the pipeline first to ensure heartbeat and sorted set are updated
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to execute heartbeat pipeline: %w", err)
	}
	
	// Now update node info separately
	nodeKey := r.getNodeKey(heartbeat.NodeID)
	var nodeInfo *NodeInfo
	
	// Try to get existing node info
	if data, err := r.redis.Get(ctx, nodeKey).Result(); err == nil {
		nodeInfo = &NodeInfo{}
		if err := json.Unmarshal([]byte(data), nodeInfo); err != nil {
			// If unmarshal fails, treat as not found
			nodeInfo = nil
		}
	}
	
	// If no node info exists, create from heartbeat
	if nodeInfo == nil {
		nodeInfo = &NodeInfo{
			ID:             heartbeat.NodeID,
			Status:         heartbeat.Status,
			ActiveWorkers:  heartbeat.ActiveWorkers,
			MaxWorkers:     heartbeat.MaxWorkers,
			TasksProcessed: heartbeat.TasksProcessed,
			TasksFailed:    heartbeat.TasksFailed,
			CPUUsage:       heartbeat.CPUUsage,
			MemoryUsage:    heartbeat.MemoryUsage,
			LastHeartbeat:  heartbeat.Timestamp,
			UpdatedAt:      time.Now(),
		}
	} else {
		// Update existing node info with heartbeat data
		nodeInfo.Status = heartbeat.Status
		nodeInfo.ActiveWorkers = heartbeat.ActiveWorkers
		nodeInfo.TasksProcessed = heartbeat.TasksProcessed
		nodeInfo.TasksFailed = heartbeat.TasksFailed
		nodeInfo.CPUUsage = heartbeat.CPUUsage
		nodeInfo.MemoryUsage = heartbeat.MemoryUsage
		nodeInfo.LastHeartbeat = heartbeat.Timestamp
		nodeInfo.UpdatedAt = time.Now()
	}
	
	// Save updated node info back to Redis
	nodeData, err := json.Marshal(nodeInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal node info: %w", err)
	}
	// Update with 1 hour TTL (nodes should send heartbeat every 30 seconds)
	if err := r.redis.Set(ctx, nodeKey, nodeData, 1*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to update node info: %w", err)
	}

	return nil
}

// GetNode returns node information
func (r *Registry) GetNode(ctx context.Context, nodeID string) (*NodeInfo, error) {
	// Always load from Redis to get the latest data
	// Cache is only used for UpdateHeartbeat optimization
	key := r.getNodeKey(nodeID)
	data, err := r.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	node := &NodeInfo{}
	if err := json.Unmarshal([]byte(data), node); err != nil {
		return nil, err
	}

	// Update cache for future use
	r.mu.Lock()
	r.nodes[nodeID] = node
	r.mu.Unlock()

	return node, nil
}

// ListNodes returns all nodes
func (r *Registry) ListNodes(ctx context.Context) ([]*NodeInfo, error) {
	// Get active node IDs from sorted set
	activeKey := r.getActiveNodesKey()
	nodeIDs, err := r.redis.ZRange(ctx, activeKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	nodes := make([]*NodeInfo, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, err := r.GetNode(ctx, nodeID)
		if err == nil && node != nil {
			nodes = append(nodes, node)
		}
		// Don't silently skip errors - at least log them
		// This way we can see if nodes are failing to load
	}

	return nodes, nil
}

// ListNodesByCapability returns nodes with specific capability
func (r *Registry) ListNodesByCapability(ctx context.Context, capability string) ([]*NodeInfo, error) {
	capKey := r.getCapabilityKey(capability)
	nodeIDs, err := r.redis.SMembers(ctx, capKey).Result()
	if err != nil {
		return nil, err
	}

	nodes := make([]*NodeInfo, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, err := r.GetNode(ctx, nodeID)
		if err == nil && node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// ListNodesByTag returns nodes with specific tag
func (r *Registry) ListNodesByTag(ctx context.Context, tag string) ([]*NodeInfo, error) {
	tagKey := r.getTagKey(tag)
	nodeIDs, err := r.redis.SMembers(ctx, tagKey).Result()
	if err != nil {
		return nil, err
	}

	nodes := make([]*NodeInfo, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, err := r.GetNode(ctx, nodeID)
		if err == nil && node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// GetHealthyNodes returns healthy nodes
func (r *Registry) GetHealthyNodes(ctx context.Context) ([]*NodeInfo, error) {
	// Instead of filtering by time, use the sorted set scores
	// Nodes that have been updated recently will have higher scores
	activeKey := r.getActiveNodesKey()
	cutoffTime := time.Now().Add(-30 * time.Second).Unix()
	
	// Get node IDs with scores >= cutoff time
	nodeIDs, err := r.redis.ZRangeByScore(ctx, activeKey, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", cutoffTime),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, err
	}

	nodes := make([]*NodeInfo, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, err := r.GetNode(ctx, nodeID)
		if err == nil && node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// GetNodeMetrics returns node metrics
func (r *Registry) GetNodeMetrics(ctx context.Context, nodeID string) (map[string]interface{}, error) {
	metricsKey := fmt.Sprintf("%s:metrics:%s", r.prefix, nodeID)

	metrics, err := r.redis.HGetAll(ctx, metricsKey).Result()
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for k, v := range metrics {
		result[k] = v
	}

	return result, nil
}

// GetClusterStats returns cluster statistics
func (r *Registry) GetClusterStats(ctx context.Context) (*ClusterStats, error) {
	nodes, err := r.GetHealthyNodes(ctx)
	if err != nil {
		return nil, err
	}

	stats := &ClusterStats{
		TotalNodes:     len(nodes),
		ActiveNodes:    0,
		TotalWorkers:   0,
		ActiveWorkers:  0,
		TasksProcessed: 0,
		TasksFailed:    0,
		AvgCPUUsage:    0,
		AvgMemoryUsage: 0,
		// Initialize new fields
		TotalPagesPerSecond: 0,
		TotalBytesPerSecond: 0,
		TasksPending:   0,
		TasksRunning:   0,
		TasksQueued:    0,
		TasksRetrying:  0,
		URLsTotal:      0,
		URLsDuplicated: 0,
		NetworkErrors:  0,
		ParseErrors:    0,
		TimeoutErrors:  0,
	}

	for _, node := range nodes {
		if node.Status == "active" {
			stats.ActiveNodes++
		}
		stats.TotalWorkers += node.MaxWorkers
		stats.ActiveWorkers += node.ActiveWorkers
		stats.TasksProcessed += node.TasksProcessed
		stats.TasksFailed += node.TasksFailed
		stats.AvgCPUUsage += node.CPUUsage
		stats.AvgMemoryUsage += node.MemoryUsage
		
		// Aggregate metrics from NodeMetrics if available
		if node.Metrics != nil {
			stats.TotalPagesPerSecond += node.Metrics.PagesPerSecond
			stats.TotalBytesPerSecond += node.Metrics.BytesPerSecond
			stats.TasksPending += int64(node.Metrics.TasksPending)
			stats.TasksRunning += int64(node.Metrics.TasksRunning)
			stats.TasksQueued += int64(node.Metrics.TasksInQueue)
			stats.TasksRetrying += int64(node.Metrics.TasksRetrying)
			stats.URLsTotal += node.Metrics.URLsDiscovered
			stats.URLsDuplicated += node.Metrics.URLsDuplicated
			stats.NetworkErrors += node.Metrics.NetworkErrors
			stats.ParseErrors += node.Metrics.ParseErrors
			stats.TimeoutErrors += node.Metrics.TimeoutErrors
		}
	}

	if len(nodes) > 0 {
		stats.AvgCPUUsage /= float64(len(nodes))
		stats.AvgMemoryUsage /= float64(len(nodes))
	}
	
	// Calculate derived metrics
	if stats.TasksProcessed > 0 {
		stats.SuccessRate = float64(stats.TasksProcessed-stats.TasksFailed) / float64(stats.TasksProcessed) * 100
		stats.ErrorRate = float64(stats.TasksFailed) / float64(stats.TasksProcessed) * 100
	}
	
	if stats.URLsTotal > 0 {
		stats.DedupeRate = float64(stats.URLsDuplicated) / float64(stats.URLsTotal) * 100
	}
	
	// Convert bytes/sec to Mbps
	stats.TotalBandwidthMbps = (stats.TotalBytesPerSecond * 8) / (1024 * 1024)

	return stats, nil
}

// AddWatcher adds a node watcher
func (r *Registry) AddWatcher(watcher NodeWatcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.watchers = append(r.watchers, watcher)
}

// RemoveWatcher removes a node watcher
func (r *Registry) RemoveWatcher(watcher NodeWatcher) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, w := range r.watchers {
		if w == watcher {
			r.watchers = append(r.watchers[:i], r.watchers[i+1:]...)
			break
		}
	}
}

// CheckAndCleanDeadNodes checks and removes dead nodes
func (r *Registry) CheckAndCleanDeadNodes(ctx context.Context) error {
	nodes, err := r.ListNodes(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	deadTimeout := 60 * time.Second

	for _, node := range nodes {
		if now.Sub(node.LastHeartbeat) > deadTimeout {
			// Node is dead, unregister it
			r.UnregisterNode(ctx, node.ID)
		}
	}

	return nil
}

// StartCleanup starts periodic cleanup of dead nodes
func (r *Registry) StartCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.CheckAndCleanDeadNodes(ctx)
		}
	}
}

// Helper methods for Redis keys
func (r *Registry) getNodeKey(nodeID string) string {
	return fmt.Sprintf("%s:node:%s", r.prefix, nodeID)
}

func (r *Registry) getActiveNodesKey() string {
	return fmt.Sprintf("%s:nodes:active", r.prefix)
}

func (r *Registry) getHeartbeatKey(nodeID string) string {
	return fmt.Sprintf("%s:heartbeat:%s", r.prefix, nodeID)
}

func (r *Registry) getCapabilityKey(capability string) string {
	return fmt.Sprintf("%s:capability:%s", r.prefix, capability)
}

func (r *Registry) getTagKey(tag string) string {
	return fmt.Sprintf("%s:tag:%s", r.prefix, tag)
}

func (r *Registry) getEventChannel() string {
	return fmt.Sprintf("%s:events", r.prefix)
}

// ClusterStats represents cluster statistics
type ClusterStats struct {
	TotalNodes     int     `json:"total_nodes"`
	ActiveNodes    int     `json:"active_nodes"`
	TotalWorkers   int     `json:"total_workers"`
	ActiveWorkers  int     `json:"active_workers"`
	TasksProcessed int64   `json:"tasks_processed"`
	TasksFailed    int64   `json:"tasks_failed"`
	AvgCPUUsage    float64 `json:"avg_cpu_usage"`
	AvgMemoryUsage float64 `json:"avg_memory_usage"`
	
	// Real-time performance
	TotalPagesPerSecond float64 `json:"total_pages_per_second"`
	TotalBytesPerSecond float64 `json:"total_bytes_per_second"`
	TotalBandwidthMbps  float64 `json:"total_bandwidth_mbps"`
	
	// Task statistics
	TasksPending   int64   `json:"tasks_pending"`
	TasksRunning   int64   `json:"tasks_running"`
	TasksQueued    int64   `json:"tasks_queued"`
	TasksRetrying  int64   `json:"tasks_retrying"`
	SuccessRate    float64 `json:"success_rate"`
	
	// URL statistics
	URLsTotal      int64   `json:"urls_total"`
	URLsDuplicated int64   `json:"urls_duplicated"`
	DedupeRate     float64 `json:"dedupe_rate"`
	
	// Error statistics
	ErrorRate      float64 `json:"error_rate"`
	NetworkErrors  int64   `json:"network_errors"`
	ParseErrors    int64   `json:"parse_errors"`
	TimeoutErrors  int64   `json:"timeout_errors"`
}
