package communication

import (
	"log/slog"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/NHYCRaymond/go-backend-kit/crawler/proto"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NodeConnection 节点连接
type NodeConnection struct {
	// 基本信息
	id     string
	nodeID string
	stream pb.CrawlerNode_ConnectServer

	// 节点信息
	info   *pb.RegisterNode
	infoMu sync.RWMutex

	// 连接状态
	connected     atomic.Bool
	lastHeartbeat atomic.Value // time.Time

	// 发送通道
	sendCh chan *pb.ControlMessage

	// 响应等待
	pendingRequests map[string]chan *pb.Response
	requestsMu      sync.RWMutex

	// 指标缓存
	latestMetrics *pb.MetricsUpdate
	metricsMu     sync.RWMutex

	// 日志缓冲
	logBuffer   []*pb.LogEntry
	logBufferMu sync.RWMutex

	// 控制
	stopCh chan struct{}
	logger *slog.Logger
}

// NodeInfo 节点信息
type NodeInfo struct {
	ID           string            `json:"id"`
	Hostname     string            `json:"hostname"`
	IP           string            `json:"ip"`
	Port         int               `json:"port"`
	Capabilities []string          `json:"capabilities"`
	MaxWorkers   int               `json:"max_workers"`
	Labels       map[string]string `json:"labels"`
	Version      string            `json:"version"`
	Status       string            `json:"status"`
	LastSeen     time.Time         `json:"last_seen"`
}

// NewNodeConnection 创建节点连接
func NewNodeConnection(nodeID string, stream pb.CrawlerNode_ConnectServer, logger *slog.Logger) *NodeConnection {
	conn := &NodeConnection{
		id:              uuid.New().String(),
		nodeID:          nodeID,
		stream:          stream,
		sendCh:          make(chan *pb.ControlMessage, 100),
		pendingRequests: make(map[string]chan *pb.Response),
		logBuffer:       make([]*pb.LogEntry, 0, 1000),
		stopCh:          make(chan struct{}),
		logger:          logger,
	}

	conn.connected.Store(true)
	conn.lastHeartbeat.Store(time.Now())

	// 启动发送协程
	go conn.sendLoop()

	return conn
}

// HandleMessages 处理来自节点的消息
func (c *NodeConnection) HandleMessages(ctx context.Context) error {
	defer c.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.stopCh:
			return nil
		default:
			// 接收消息
			msg, err := c.stream.Recv()
			if err != nil {
				c.logger.Error("Failed to receive message",
					"node_id", c.nodeID,
					"error", err)
				return err
			}

			// 处理消息
			if err := c.handleMessage(msg); err != nil {
				c.logger.Error("Failed to handle message",
					"node_id", c.nodeID,
					"message_id", msg.MessageId,
					"error", err)
			}
		}
	}
}

// handleMessage 处理单个消息
func (c *NodeConnection) handleMessage(msg *pb.NodeMessage) error {
	c.logger.Debug("Received message",
		"node_id", c.nodeID,
		"message_id", msg.MessageId,
		"type", fmt.Sprintf("%T", msg.Message))

	switch m := msg.Message.(type) {
	case *pb.NodeMessage_Heartbeat:
		return c.handleHeartbeat(m.Heartbeat)

	case *pb.NodeMessage_MetricsUpdate:
		return c.handleMetricsUpdate(m.MetricsUpdate)

	case *pb.NodeMessage_TaskStatus:
		return c.handleTaskStatus(m.TaskStatus)

	case *pb.NodeMessage_LogEntry:
		return c.handleLogEntry(m.LogEntry)

	case *pb.NodeMessage_Event:
		return c.handleEvent(m.Event)

	case *pb.NodeMessage_Response:
		return c.handleResponse(m.Response)

	default:
		return fmt.Errorf("unknown message type: %T", msg.Message)
	}
}

// handleHeartbeat 处理心跳
func (c *NodeConnection) handleHeartbeat(hb *pb.Heartbeat) error {
	c.lastHeartbeat.Store(time.Now())

	c.logger.Debug("Heartbeat received",
		"node_id", c.nodeID,
		"status", hb.Status,
		"active_workers", hb.ActiveWorkers,
		"queue_size", hb.QueueSize)

	// 更新节点状态
	c.infoMu.Lock()
	if c.info != nil {
		// 可以在这里更新一些动态信息
	}
	c.infoMu.Unlock()

	return nil
}

// handleMetricsUpdate 处理指标更新
func (c *NodeConnection) handleMetricsUpdate(metrics *pb.MetricsUpdate) error {
	c.metricsMu.Lock()
	c.latestMetrics = metrics
	c.metricsMu.Unlock()

	c.logger.Debug("Metrics updated",
		"node_id", c.nodeID,
		"requests_total", metrics.RequestsTotal,
		"success_rate", metrics.SuccessRate)

	return nil
}

// handleTaskStatus 处理任务状态更新
func (c *NodeConnection) handleTaskStatus(status *pb.TaskStatusUpdate) error {
	c.logger.Info("Task status update",
		"node_id", c.nodeID,
		"task_id", status.TaskId,
		"status", status.Status,
		"url", status.Url)

	// TODO: 转发到任务管理器

	return nil
}

// handleLogEntry 处理日志条目
func (c *NodeConnection) handleLogEntry(log *pb.LogEntry) error {
	// 添加到缓冲区
	c.logBufferMu.Lock()
	c.logBuffer = append(c.logBuffer, log)
	// 保持缓冲区大小
	if len(c.logBuffer) > 1000 {
		c.logBuffer = c.logBuffer[100:]
	}
	c.logBufferMu.Unlock()

	// 根据日志级别记录
	switch log.Level {
	case pb.LogLevel_LOG_LEVEL_ERROR:
		c.logger.Error("Node log",
			"node_id", c.nodeID,
			"message", log.Message,
			"fields", log.Fields)
	case pb.LogLevel_LOG_LEVEL_WARN:
		c.logger.Warn("Node log",
			"node_id", c.nodeID,
			"message", log.Message,
			"fields", log.Fields)
	default:
		c.logger.Debug("Node log",
			"node_id", c.nodeID,
			"level", log.Level,
			"message", log.Message)
	}

	return nil
}

// handleEvent 处理事件
func (c *NodeConnection) handleEvent(event *pb.Event) error {
	c.logger.Info("Node event",
		"node_id", c.nodeID,
		"event_id", event.EventId,
		"type", event.Type,
		"source", event.Source)

	// TODO: 转发到事件总线

	return nil
}

// handleResponse 处理响应
func (c *NodeConnection) handleResponse(resp *pb.Response) error {
	c.requestsMu.RLock()
	ch, exists := c.pendingRequests[resp.RequestId]
	c.requestsMu.RUnlock()

	if !exists {
		c.logger.Warn("Received response for unknown request",
			"node_id", c.nodeID,
			"request_id", resp.RequestId)
		return nil
	}

	// 发送响应
	select {
	case ch <- resp:
	case <-time.After(time.Second):
		c.logger.Warn("Timeout sending response to channel",
			"node_id", c.nodeID,
			"request_id", resp.RequestId)
	}

	// 清理
	c.requestsMu.Lock()
	delete(c.pendingRequests, resp.RequestId)
	c.requestsMu.Unlock()

	return nil
}

// sendLoop 发送循环
func (c *NodeConnection) sendLoop() {
	for {
		select {
		case <-c.stopCh:
			return
		case msg := <-c.sendCh:
			if err := c.stream.Send(msg); err != nil {
				c.logger.Error("Failed to send message",
					"node_id", c.nodeID,
					"message_id", msg.MessageId,
					"error", err)
				c.connected.Store(false)
				return
			}
		}
	}
}

// Send 发送消息
func (c *NodeConnection) Send(msg *pb.ControlMessage) error {
	if !c.IsConnected() {
		return fmt.Errorf("node not connected")
	}

	select {
	case c.sendCh <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("send timeout")
	}
}

// SendWithResponse 发送消息并等待响应
func (c *NodeConnection) SendWithResponse(ctx context.Context, msg *pb.ControlMessage) (*pb.Response, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("node not connected")
	}

	// 创建请求
	requestID := generateMessageID()
	request := &pb.ControlMessage{
		MessageId: requestID,
		Timestamp: msg.Timestamp,
		Message: &pb.ControlMessage_Request{
			Request: &pb.Request{
				RequestId: requestID,
				Type:      pb.RequestType_REQUEST_GET_STATUS,
				Data:      nil,
			},
		},
	}

	// 创建响应通道
	respCh := make(chan *pb.Response, 1)

	c.requestsMu.Lock()
	c.pendingRequests[requestID] = respCh
	c.requestsMu.Unlock()

	// 发送请求
	if err := c.Send(request); err != nil {
		c.requestsMu.Lock()
		delete(c.pendingRequests, requestID)
		c.requestsMu.Unlock()
		return nil, err
	}

	// 等待响应
	select {
	case <-ctx.Done():
		c.requestsMu.Lock()
		delete(c.pendingRequests, requestID)
		c.requestsMu.Unlock()
		return nil, ctx.Err()

	case resp := <-respCh:
		return resp, nil

	case <-time.After(10 * time.Second):
		c.requestsMu.Lock()
		delete(c.pendingRequests, requestID)
		c.requestsMu.Unlock()
		return nil, fmt.Errorf("response timeout")
	}
}

// UpdateInfo 更新节点信息
func (c *NodeConnection) UpdateInfo(info *pb.RegisterNode) {
	c.infoMu.Lock()
	c.info = info
	c.infoMu.Unlock()
}

// GetInfo 获取节点信息
func (c *NodeConnection) GetInfo() *NodeInfo {
	c.infoMu.RLock()
	defer c.infoMu.RUnlock()

	if c.info == nil {
		return nil
	}

	return &NodeInfo{
		ID:           c.nodeID,
		Hostname:     c.info.Hostname,
		IP:           c.info.Ip,
		Port:         int(c.info.Port),
		Capabilities: c.info.Capabilities,
		MaxWorkers:   int(c.info.MaxWorkers),
		Labels:       c.info.Labels,
		Version:      c.info.Version,
		Status:       "active",
		LastSeen:     c.LastHeartbeat(),
	}
}

// GetMetrics 获取节点指标
func (c *NodeConnection) GetMetrics(ctx context.Context, req *pb.GetMetricsRequest) (*pb.MetricsResponse, error) {
	// 这里可以实现更复杂的指标获取逻辑
	// 比如从缓存获取或向节点请求

	c.metricsMu.RLock()
	metrics := c.latestMetrics
	c.metricsMu.RUnlock()

	if metrics == nil {
		return nil, fmt.Errorf("no metrics available")
	}

	// 转换为响应格式
	resp := &pb.MetricsResponse{
		Metrics: make(map[string]*pb.MetricValue),
	}

	resp.Metrics["requests_total"] = &pb.MetricValue{
		Value:     &pb.MetricValue_IntValue{IntValue: metrics.RequestsTotal},
		Timestamp: timestamppb.Now(),
	}

	resp.Metrics["success_rate"] = &pb.MetricValue{
		Value:     &pb.MetricValue_DoubleValue{DoubleValue: metrics.SuccessRate},
		Timestamp: timestamppb.Now(),
	}

	return resp, nil
}

// GetLatestMetrics 获取最新指标
func (c *NodeConnection) GetLatestMetrics() *pb.MetricsUpdate {
	c.metricsMu.RLock()
	defer c.metricsMu.RUnlock()
	return c.latestMetrics
}

// GetLogs 获取日志缓冲
func (c *NodeConnection) GetLogs(limit int) []*pb.LogEntry {
	c.logBufferMu.RLock()
	defer c.logBufferMu.RUnlock()

	if limit <= 0 || limit > len(c.logBuffer) {
		limit = len(c.logBuffer)
	}

	// 返回最新的日志
	start := len(c.logBuffer) - limit
	if start < 0 {
		start = 0
	}

	result := make([]*pb.LogEntry, limit)
	copy(result, c.logBuffer[start:])
	return result
}

// IsConnected 检查是否连接
func (c *NodeConnection) IsConnected() bool {
	return c.connected.Load()
}

// LastHeartbeat 获取最后心跳时间
func (c *NodeConnection) LastHeartbeat() time.Time {
	if v := c.lastHeartbeat.Load(); v != nil {
		return v.(time.Time)
	}
	return time.Time{}
}

// NodeID 获取节点ID
func (c *NodeConnection) NodeID() string {
	return c.nodeID
}

// Close 关闭连接
func (c *NodeConnection) Close() {
	if !c.connected.CompareAndSwap(true, false) {
		return
	}

	c.logger.Debug("Closing node connection", "node_id", c.nodeID)

	// 发送停止信号
	close(c.stopCh)

	// 清理待处理请求
	c.requestsMu.Lock()
	for _, ch := range c.pendingRequests {
		close(ch)
	}
	c.pendingRequests = make(map[string]chan *pb.Response)
	c.requestsMu.Unlock()
}

// 辅助函数

func generateMessageID() string {
	return uuid.New().String()
}

func generateEventID() string {
	return fmt.Sprintf("evt_%s", uuid.New().String())
}
