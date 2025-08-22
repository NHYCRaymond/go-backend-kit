package distributed

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"

	pb "github.com/NHYCRaymond/go-backend-kit/crawler/proto"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NodeClient represents a gRPC client for crawler nodes
type NodeClient struct {
	// Connection info
	nodeID  string
	hubAddr string
	conn    *grpc.ClientConn
	client  pb.CrawlerNodeClient
	stream  pb.CrawlerNode_ConnectClient

	// Node reference
	node *Node

	// Logger
	logger *slog.Logger

	// Control
	ctx       context.Context
	cancel    context.CancelFunc
	stopChan  chan struct{}
	connected bool
	connMu    sync.RWMutex

	// Message handlers
	handlers map[string]MessageHandler

	// Metrics
	messagesSent     int64
	messagesReceived int64
	lastMessageTime  time.Time
}

// MessageHandler handles incoming messages
type MessageHandler func(context.Context, *pb.ControlMessage) error

// NodeClientConfig contains client configuration
type NodeClientConfig struct {
	NodeID  string
	HubAddr string
	Node    *Node
	Logger  *slog.Logger
}

// NewNodeClient creates a new node client
func NewNodeClient(config *NodeClientConfig) *NodeClient {
	ctx, cancel := context.WithCancel(context.Background())

	client := &NodeClient{
		nodeID:   config.NodeID,
		hubAddr:  config.HubAddr,
		node:     config.Node,
		logger:   config.Logger,
		ctx:      ctx,
		cancel:   cancel,
		stopChan: make(chan struct{}),
		handlers: make(map[string]MessageHandler),
	}

	// Register default handlers
	client.registerHandlers()

	return client
}

// Connect establishes connection to the hub
func (nc *NodeClient) Connect() error {
	nc.logger.Info("Connecting to hub", "addr", nc.hubAddr)

	// Create gRPC connection
	conn, err := grpc.Dial(nc.hubAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}

	nc.conn = conn
	nc.client = pb.NewCrawlerNodeClient(conn)

	// Establish bidirectional stream
	stream, err := nc.client.Connect(nc.ctx)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create stream: %w", err)
	}

	nc.stream = stream

	// Send registration message
	if err := nc.register(); err != nil {
		stream.CloseSend()
		conn.Close()
		return fmt.Errorf("failed to register: %w", err)
	}

	nc.connMu.Lock()
	nc.connected = true
	nc.connMu.Unlock()

	// Start message handlers
	go nc.receiveLoop()
	go nc.heartbeatLoop()

	nc.logger.Info("Connected to hub successfully")
	return nil
}

// Disconnect closes the connection
func (nc *NodeClient) Disconnect() error {
	nc.logger.Info("Disconnecting from hub")

	nc.connMu.Lock()
	nc.connected = false
	nc.connMu.Unlock()

	// Cancel context
	nc.cancel()

	// Close stream
	if nc.stream != nil {
		nc.stream.CloseSend()
	}

	// Close connection
	if nc.conn != nil {
		nc.conn.Close()
	}

	close(nc.stopChan)

	nc.logger.Info("Disconnected from hub")
	return nil
}

// register sends registration message
func (nc *NodeClient) register() error {
	regMsg := &pb.NodeMessage{
		MessageId: generateMessageID(),
		Timestamp: timestamppb.Now(),
		Message: &pb.NodeMessage_Register{
			Register: &pb.RegisterNode{
				NodeId:       nc.nodeID,
				Hostname:     nc.node.Hostname,
				Ip:           nc.node.IP,
				Port:         int32(nc.node.Port),
				Capabilities: []string{"http", "https", "browser"},
				MaxWorkers:   int32(nc.node.MaxWorkers),
				Labels:       nc.node.Labels,
			},
		},
	}

	if err := nc.stream.Send(regMsg); err != nil {
		return fmt.Errorf("failed to send registration: %w", err)
	}

	nc.logger.Info("Registration sent", "node_id", nc.nodeID)
	return nil
}

// receiveLoop receives messages from hub
func (nc *NodeClient) receiveLoop() {
	for {
		select {
		case <-nc.ctx.Done():
			return
		default:
			msg, err := nc.stream.Recv()
			if err == io.EOF {
				nc.logger.Info("Stream closed by hub")
				nc.handleDisconnect()
				return
			}
			if err != nil {
				nc.logger.Error("Failed to receive message", "error", err)
				nc.handleDisconnect()
				return
			}

			nc.messagesReceived++
			nc.lastMessageTime = time.Now()

			// Handle message
			go nc.handleMessage(msg)
		}
	}
}

// handleMessage processes incoming messages
func (nc *NodeClient) handleMessage(msg *pb.ControlMessage) {
	nc.logger.Debug("Received message",
		"message_id", msg.MessageId,
		"type", msg.Message)

	// Determine message type and handle
	switch m := msg.Message.(type) {
	case *pb.ControlMessage_Ack:
		nc.handleAck(m.Ack)

	case *pb.ControlMessage_TaskAssignment:
		nc.handleTaskAssignment(m.TaskAssignment)

	case *pb.ControlMessage_Command:
		nc.handleCommand(msg, m.Command)

	case *pb.ControlMessage_ConfigUpdate:
		nc.handleConfigUpdate(m.ConfigUpdate)

	case *pb.ControlMessage_RateLimit:
		nc.handleRateLimitUpdate(m.RateLimit)

	default:
		nc.logger.Warn("Unknown message type", "type", msg.Message)
	}
}

// handleAck handles acknowledgment messages
func (nc *NodeClient) handleAck(ack *pb.Acknowledgment) {
	if ack.Success {
		nc.logger.Debug("Received ACK", "message_id", ack.MessageId)
	} else {
		nc.logger.Error("Received NACK",
			"message_id", ack.MessageId,
			"error", ack.ErrorMessage)
	}
}

// handleTaskAssignment handles task assignments
func (nc *NodeClient) handleTaskAssignment(assignment *pb.TaskAssignment) {
	nc.logger.Info("Received task assignment",
		"task_id", assignment.TaskId,
		"url", assignment.Url,
		"method", assignment.Method,
		"task_type", assignment.TaskType,
		"project_id", assignment.ProjectId,
		"lua_script", assignment.LuaScript,
		"has_body", len(assignment.Body) > 0,
		"body_size", len(assignment.Body),
		"headers_count", len(assignment.Headers),
		"cookies_count", len(assignment.Cookies),
		"extract_rules_count", len(assignment.ExtractRules))
	
	if len(assignment.Body) > 0 {
		nc.logger.Debug("Task body content",
			"task_id", assignment.TaskId,
			"body", string(assignment.Body))
	}

	// Convert to internal task
	t := &task.Task{
		ID:        assignment.TaskId,
		ParentID:  assignment.ParentId,  // Include parent task ID
		URL:       assignment.Url,
		Type:      assignment.TaskType,  // Use task type from assignment
		Priority:  int(assignment.Priority),
		Method:    assignment.Method,
		Headers:   assignment.Headers,
		Body:      assignment.Body,
		Cookies:   assignment.Cookies,
		ProjectID: assignment.ProjectId,  // Include project ID for Lua scripts
		LuaScript: assignment.LuaScript,  // Include Lua script name
		Metadata:  make(map[string]interface{}),
	}
	
	// Default task type if not specified
	if t.Type == "" {
		t.Type = "detail"  // Default type
		nc.logger.Debug("Task type not specified, defaulting to detail",
			"task_id", t.ID)
	}
	
	// Convert extract rules if present
	if len(assignment.ExtractRules) > 0 {
		t.ExtractRules = make([]task.ExtractRule, len(assignment.ExtractRules))
		for i, rule := range assignment.ExtractRules {
			// Convert selector type from protobuf enum to string
			selectorType := "css" // default
			switch rule.Type {
			case pb.SelectorType_SELECTOR_CSS:
				selectorType = "css"
			case pb.SelectorType_SELECTOR_XPATH:
				selectorType = "xpath"
			case pb.SelectorType_SELECTOR_JSONPATH:
				selectorType = "json"
			case pb.SelectorType_SELECTOR_REGEX:
				selectorType = "regex"
			}
			
			t.ExtractRules[i] = task.ExtractRule{
				Field:     rule.Field,
				Selector:  rule.Selector,
				Type:      selectorType,
				Attribute: rule.Attribute,
				Multiple:  rule.Multiple,
				Default:   rule.DefaultValue,
			}
		}
	}
	
	// Set timeout if specified
	if assignment.TimeoutSeconds > 0 {
		t.Timeout = time.Duration(assignment.TimeoutSeconds) * time.Second
	}
	
	// Set max retries
	if assignment.MaxRetries > 0 {
		t.MaxRetries = int(assignment.MaxRetries)
	}
	
	// Default to GET if method not specified
	if t.Method == "" {
		t.Method = "GET"
		nc.logger.Debug("Method not specified, defaulting to GET",
			"task_id", t.ID)
	}
	
	// Convert storage config if present
	if assignment.StorageConfig != nil {
		t.StorageConf = task.StorageConfig{
			Type:       assignment.StorageConfig.Type,
			Database:   assignment.StorageConfig.Database,
			Collection: assignment.StorageConfig.Collection,
			Table:      assignment.StorageConfig.Table,
			Bucket:     assignment.StorageConfig.Bucket,
			Path:       assignment.StorageConfig.Path,
			Format:     assignment.StorageConfig.Format,
			Options:    make(map[string]interface{}),
		}
		
		// Convert options
		for k, v := range assignment.StorageConfig.Options {
			t.StorageConf.Options[k] = v
		}
		
		nc.logger.Info("Task has storage config",
			"task_id", t.ID,
			"storage_type", t.StorageConf.Type,
			"database", t.StorageConf.Database,
			"collection", t.StorageConf.Collection)
	}
	
	// Extract batch information from metadata
	if assignment.Metadata != nil {
		if batchID, ok := assignment.Metadata["batch_id"]; ok && batchID != "" {
			t.BatchID = batchID
			nc.logger.Info("Task has batch ID", "task_id", t.ID, "batch_id", batchID)
		}
		if batchSize, ok := assignment.Metadata["batch_size"]; ok && batchSize != "" {
			if size, err := strconv.Atoi(batchSize); err == nil {
				t.BatchSize = size
			}
		}
		if batchIndex, ok := assignment.Metadata["batch_index"]; ok && batchIndex != "" {
			if index, err := strconv.Atoi(batchIndex); err == nil {
				t.BatchIndex = index
			}
		}
		if batchType, ok := assignment.Metadata["batch_type"]; ok && batchType != "" {
			t.BatchType = batchType
		}
		// Extract source field
		if source, ok := assignment.Metadata["source"]; ok && source != "" {
			t.Source = source
		}
		
		// Log batch information
		if t.BatchID != "" {
			nc.logger.Info("Task batch information extracted",
				"task_id", t.ID,
				"batch_id", t.BatchID,
				"batch_size", t.BatchSize,
				"batch_index", t.BatchIndex,
				"batch_type", t.BatchType,
				"source", t.Source)
		}
	}

	// Add to node's task queue
	if nc.node != nil {
		select {
		case nc.node.taskQueue <- t:
			// Send acknowledgment
			nc.sendTaskAck(assignment.TaskId, true, "")
		default:
			// Queue full
			nc.sendTaskAck(assignment.TaskId, false, "Task queue full")
		}
	}
}

// handleCommand handles command messages
func (nc *NodeClient) handleCommand(msg *pb.ControlMessage, cmd *pb.Command) {
	nc.logger.Info("Received command", "type", cmd.Type)

	var success bool
	var result string
	var errorMsg string

	switch cmd.Type {
	case pb.CommandType_CMD_PAUSE:
		nc.node.Pause()
		success = true
		result = "Node paused"

	case pb.CommandType_CMD_RESUME:
		nc.node.Resume()
		success = true
		result = "Node resumed"

	case pb.CommandType_CMD_STOP:
		// Stop command
		success = false
		errorMsg = "Stop not implemented"

	default:
		success = false
		errorMsg = fmt.Sprintf("Unknown command type: %v", cmd.Type)
	}

	// Send response
	nc.sendCommandResponse(msg.MessageId, success, result, errorMsg)
}

// handleConfigUpdate handles configuration updates
func (nc *NodeClient) handleConfigUpdate(update *pb.ConfigUpdate) {
	nc.logger.Info("Received config update")

	// Apply configuration updates
	if update.Config != nil {
		for key, value := range update.Config {
			nc.logger.Debug("Config update", "key", key, "value", value)
			// Apply config to node
			// This would update node configuration
		}
	}

	if update.RestartRequired {
		nc.logger.Info("Restart required for config update")
		// Handle restart if needed
	}
}

// handleRateLimitUpdate handles rate limit updates
func (nc *NodeClient) handleRateLimitUpdate(update *pb.RateLimitUpdate) {
	nc.logger.Info("Rate limit update",
		"requests_per_second", update.RequestsPerSecond,
		"burst_size", update.BurstSize)

	// Update rate limiter in node
	// This would update the fetcher's rate limiting
}

// heartbeatLoop sends periodic heartbeats
func (nc *NodeClient) heartbeatLoop() {
	// Send initial heartbeat immediately
	nc.sendHeartbeat()

	ticker := time.NewTicker(5 * time.Second) // Send heartbeats every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-nc.ctx.Done():
			return
		case <-ticker.C:
			nc.sendHeartbeat()
		}
	}
}

// sendHeartbeat sends heartbeat message
func (nc *NodeClient) sendHeartbeat() {
	nc.connMu.RLock()
	connected := nc.connected
	nc.connMu.RUnlock()

	if !connected {
		return
	}

	// Collect node metrics
	var workerCount int32
	var queueSize int32
	var memoryUsage float32
	var cpuUsage float32

	if nc.node != nil {
		workerCount = int32(nc.node.getActiveWorkerCount())
		queueSize = int32(len(nc.node.taskQueue))
		// TODO: Get actual resource usage
		memoryUsage = 50.0
		cpuUsage = 30.0
	}

	msg := &pb.NodeMessage{
		MessageId: generateMessageID(),
		Timestamp: timestamppb.Now(),
		Message: &pb.NodeMessage_Heartbeat{
			Heartbeat: &pb.Heartbeat{
				NodeId:        nc.nodeID,
				Status:        pb.NodeStatus_NODE_STATUS_ACTIVE,
				ActiveWorkers: workerCount,
				QueueSize:     queueSize,
				MemoryUsage:   float64(memoryUsage),
				CpuUsage:      float64(cpuUsage),
			},
		},
	}

	if err := nc.stream.Send(msg); err != nil {
		nc.logger.Error("Failed to send heartbeat", "error", err)
		nc.handleDisconnect()
	}
}

// SendTaskResult sends task result to hub
func (nc *NodeClient) SendTaskResult(taskID string, success bool, data []byte, errorMsg string) error {
	// Task results are sent via TaskStatusUpdate
	msg := &pb.NodeMessage{
		MessageId: generateMessageID(),
		Timestamp: timestamppb.Now(),
		Message: &pb.NodeMessage_TaskStatus{
			TaskStatus: &pb.TaskStatusUpdate{
				TaskId:       taskID,
				NodeId:       nc.nodeID,
				Status:       pb.TaskStatus_TASK_STATUS_SUCCESS,
				ErrorMessage: errorMsg,
			},
		},
	}

	if err := nc.stream.Send(msg); err != nil {
		return fmt.Errorf("failed to send task result: %w", err)
	}

	nc.messagesSent++
	return nil
}

// SendMetrics sends metrics update to hub
func (nc *NodeClient) SendMetrics(metrics *NodeMetrics) error {
	msg := &pb.NodeMessage{
		MessageId: generateMessageID(),
		Timestamp: timestamppb.Now(),
		Message: &pb.NodeMessage_MetricsUpdate{
			MetricsUpdate: &pb.MetricsUpdate{
				NodeId:          nc.nodeID,
				TasksCompleted:  metrics.TasksProcessed,
				TasksFailed:     metrics.TasksFailed,
				BytesDownloaded: metrics.BytesDownloaded,
				ItemsExtracted:  metrics.ItemsExtracted,
				AvgResponseTime: float64(metrics.AverageLatency.Milliseconds()),
			},
		},
	}

	if err := nc.stream.Send(msg); err != nil {
		return fmt.Errorf("failed to send metrics: %w", err)
	}

	nc.messagesSent++
	return nil
}

// sendTaskAck sends task acknowledgment
func (nc *NodeClient) sendTaskAck(taskID string, success bool, errorMsg string) {
	// Task acknowledgment is sent via TaskStatusUpdate
	msg := &pb.NodeMessage{
		MessageId: generateMessageID(),
		Timestamp: timestamppb.Now(),
		Message: &pb.NodeMessage_TaskStatus{
			TaskStatus: &pb.TaskStatusUpdate{
				TaskId:       taskID,
				NodeId:       nc.nodeID,
				Status:       pb.TaskStatus_TASK_STATUS_PENDING,
				ErrorMessage: errorMsg,
			},
		},
	}

	if err := nc.stream.Send(msg); err != nil {
		nc.logger.Error("Failed to send task ack", "error", err)
	}
}

// sendCommandResponse sends command response
func (nc *NodeClient) sendCommandResponse(messageID string, success bool, result string, errorMsg string) {
	// For now, we log the response
	nc.logger.Info("Command response",
		"message_id", messageID,
		"success", success,
		"result", result,
		"error", errorMsg)

	// TODO: Implement sending response back to hub via Response message type
}

// handleDisconnect handles disconnection
func (nc *NodeClient) handleDisconnect() {
	nc.connMu.Lock()
	wasConnected := nc.connected
	nc.connected = false
	nc.connMu.Unlock()

	if wasConnected {
		nc.logger.Warn("Disconnected from hub")

		// Notify node of disconnection
		if nc.node != nil {
			nc.node.updateStatus(NodeStatusDisconnected)
		}

		// Attempt reconnection
		go nc.reconnect()
	}
}

// reconnect attempts to reconnect to hub
func (nc *NodeClient) reconnect() {
	backoff := time.Second
	maxBackoff := time.Minute

	for {
		select {
		case <-nc.stopChan:
			return
		default:
			nc.logger.Info("Attempting to reconnect", "backoff", backoff)

			if err := nc.Connect(); err != nil {
				nc.logger.Error("Reconnection failed", "error", err)

				time.Sleep(backoff)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			} else {
				nc.logger.Info("Reconnected successfully")
				// Update node status back to active after successful reconnection
				if nc.node != nil {
					nc.node.updateStatus(NodeStatusActive)
				}
				return
			}
		}
	}
}

// registerHandlers registers message handlers
func (nc *NodeClient) registerHandlers() {
	// Default handlers are registered in handleMessage
	// Custom handlers can be added here
}

// IsConnected returns connection status
func (nc *NodeClient) IsConnected() bool {
	nc.connMu.RLock()
	defer nc.connMu.RUnlock()
	return nc.connected
}

// GetStats returns client statistics
func (nc *NodeClient) GetStats() map[string]interface{} {
	nc.connMu.RLock()
	connected := nc.connected
	nc.connMu.RUnlock()

	return map[string]interface{}{
		"connected":         connected,
		"messages_sent":     nc.messagesSent,
		"messages_received": nc.messagesReceived,
		"last_message":      nc.lastMessageTime,
		"hub_address":       nc.hubAddr,
	}
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("msg-%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
}
