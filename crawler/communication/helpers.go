package communication

import (
	"log/slog"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/NHYCRaymond/go-backend-kit/crawler/proto"
)

// MetricsAggregator 指标聚合器
type MetricsAggregator struct {
	// 节点指标
	nodeMetrics map[string]*pb.MetricsUpdate
	metricsMu   sync.RWMutex

	// 聚合指标
	totalRequests   atomic.Int64
	totalSuccess    atomic.Int64
	totalFailed     atomic.Int64
	totalBytes      atomic.Int64
	totalItems      atomic.Int64

	// 运行状态
	running bool
	stopCh  chan struct{}
	logger  *slog.Logger
}

// NewMetricsAggregator 创建指标聚合器
func NewMetricsAggregator(logger *slog.Logger) *MetricsAggregator {
	return &MetricsAggregator{
		nodeMetrics: make(map[string]*pb.MetricsUpdate),
		stopCh:      make(chan struct{}),
		logger:      logger,
	}
}

// Start 启动指标聚合
func (ma *MetricsAggregator) Start(ctx context.Context) {
	ma.running = true
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ma.stopCh:
			return
		case <-ticker.C:
			ma.aggregate()
		}
	}
}

// Stop 停止指标聚合
func (ma *MetricsAggregator) Stop() {
	if ma.running {
		close(ma.stopCh)
		ma.running = false
	}
}

// UpdateNodeMetrics 更新节点指标
func (ma *MetricsAggregator) UpdateNodeMetrics(nodeID string, metrics *pb.MetricsUpdate) {
	ma.metricsMu.Lock()
	defer ma.metricsMu.Unlock()

	// 计算增量
	if oldMetrics, exists := ma.nodeMetrics[nodeID]; exists {
		ma.totalRequests.Add(metrics.RequestsTotal - oldMetrics.RequestsTotal)
		ma.totalSuccess.Add(metrics.RequestsSuccess - oldMetrics.RequestsSuccess)
		ma.totalFailed.Add(metrics.RequestsFailed - oldMetrics.RequestsFailed)
		ma.totalBytes.Add(metrics.BytesDownloaded - oldMetrics.BytesDownloaded)
		ma.totalItems.Add(metrics.ItemsExtracted - oldMetrics.ItemsExtracted)
	} else {
		ma.totalRequests.Add(metrics.RequestsTotal)
		ma.totalSuccess.Add(metrics.RequestsSuccess)
		ma.totalFailed.Add(metrics.RequestsFailed)
		ma.totalBytes.Add(metrics.BytesDownloaded)
		ma.totalItems.Add(metrics.ItemsExtracted)
	}

	ma.nodeMetrics[nodeID] = metrics
}

// aggregate 聚合指标
func (ma *MetricsAggregator) aggregate() {
	ma.metricsMu.RLock()
	defer ma.metricsMu.RUnlock()

	totalNodes := len(ma.nodeMetrics)
	if totalNodes == 0 {
		return
	}

	var avgResponseTime float64
	var totalSuccessRate float64

	for _, metrics := range ma.nodeMetrics {
		avgResponseTime += metrics.AvgResponseTime
		totalSuccessRate += metrics.SuccessRate
	}

	avgResponseTime /= float64(totalNodes)
	totalSuccessRate /= float64(totalNodes)

	ma.logger.Info("Aggregated metrics",
		"total_requests", ma.totalRequests.Load(),
		"total_success", ma.totalSuccess.Load(),
		"total_failed", ma.totalFailed.Load(),
		"total_bytes", ma.totalBytes.Load(),
		"total_items", ma.totalItems.Load(),
		"avg_response_time", avgResponseTime,
		"avg_success_rate", totalSuccessRate,
		"total_nodes", totalNodes)
}

// GetAggregatedMetrics 获取聚合指标
func (ma *MetricsAggregator) GetAggregatedMetrics() map[string]interface{} {
	ma.metricsMu.RLock()
	defer ma.metricsMu.RUnlock()

	return map[string]interface{}{
		"total_requests":  ma.totalRequests.Load(),
		"total_success":   ma.totalSuccess.Load(),
		"total_failed":    ma.totalFailed.Load(),
		"total_bytes":     ma.totalBytes.Load(),
		"total_items":     ma.totalItems.Load(),
		"total_nodes":     len(ma.nodeMetrics),
	}
}

// MessageRouter 消息路由器
type MessageRouter struct {
	hub    *CommunicationHub
	routes map[string]MessageHandler
	mu     sync.RWMutex
	logger *slog.Logger
	stopCh chan struct{}
}

// MessageHandler 消息处理器
type MessageHandler func(ctx context.Context, msg *pb.NodeMessage) error

// NewMessageRouter 创建消息路由器
func NewMessageRouter(hub *CommunicationHub, logger *slog.Logger) *MessageRouter {
	return &MessageRouter{
		hub:    hub,
		routes: make(map[string]MessageHandler),
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Start 启动路由器
func (mr *MessageRouter) Start(ctx context.Context) {
	// 这里可以添加路由逻辑
	<-ctx.Done()
}

// Stop 停止路由器
func (mr *MessageRouter) Stop() {
	close(mr.stopCh)
}

// RegisterHandler 注册处理器
func (mr *MessageRouter) RegisterHandler(messageType string, handler MessageHandler) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.routes[messageType] = handler
}

// Route 路由消息
func (mr *MessageRouter) Route(ctx context.Context, msg *pb.NodeMessage) error {
	messageType := fmt.Sprintf("%T", msg.Message)
	
	mr.mu.RLock()
	handler, exists := mr.routes[messageType]
	mr.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no handler for message type: %s", messageType)
	}

	return handler(ctx, msg)
}

// TUIClient TUI客户端
type TUIClient struct {
	ID        string
	hub       *CommunicationHub
	eventCh   chan *Event
	updateCh  chan interface{}
	active    atomic.Bool
	stopCh    chan struct{}
	logger    *slog.Logger
}

// NewTUIClient 创建TUI客户端
func NewTUIClient(id string, hub *CommunicationHub, logger *slog.Logger) *TUIClient {
	client := &TUIClient{
		ID:       id,
		hub:      hub,
		eventCh:  make(chan *Event, 100),
		updateCh: make(chan interface{}, 100),
		stopCh:   make(chan struct{}),
		logger:   logger,
	}
	client.active.Store(true)
	return client
}

// Start 启动客户端
func (tc *TUIClient) Start(ctx context.Context) error {
	// 订阅所有事件
	tc.hub.Subscribe(EventType("*"), tc.handleEvent)

	// 处理事件循环
	go tc.processEvents(ctx)

	tc.logger.Info("TUI client started", "client_id", tc.ID)
	return nil
}

// handleEvent 处理事件
func (tc *TUIClient) handleEvent(event *Event) {
	select {
	case tc.eventCh <- event:
	default:
		tc.logger.Warn("TUI client event channel full", "client_id", tc.ID)
	}
}

// processEvents 处理事件
func (tc *TUIClient) processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tc.stopCh:
			return
		case event := <-tc.eventCh:
			// 转换为UI更新
			update := tc.convertEventToUpdate(event)
			select {
			case tc.updateCh <- update:
			default:
				tc.logger.Warn("TUI client update channel full", "client_id", tc.ID)
			}
		}
	}
}

// convertEventToUpdate 转换事件为UI更新
func (tc *TUIClient) convertEventToUpdate(event *Event) interface{} {
	// 根据事件类型创建不同的更新
	switch event.Type {
	case EventNodeOnline, EventNodeOffline:
		return &NodeUpdate{
			NodeID: event.Source,
			Status: string(event.Type),
			Time:   event.Timestamp,
		}
	case EventTaskStart, EventTaskComplete, EventTaskFailed:
		return &TaskUpdate{
			TaskID: event.ID,
			Status: string(event.Type),
			Time:   event.Timestamp,
		}
	case EventMetricsUpdate:
		return &MetricsUpdate{
			Source: event.Source,
			Data:   event.Data,
			Time:   event.Timestamp,
		}
	default:
		return &GenericUpdate{
			Type: string(event.Type),
			Data: event.Data,
			Time: event.Timestamp,
		}
	}
}

// SendCommand 发送命令到节点
func (tc *TUIClient) SendCommand(nodeID string, cmd *pb.Command) error {
	msg := &pb.ControlMessage{
		MessageId: generateMessageID(),
		Timestamp: nil, // Will be set by hub
		Message: &pb.ControlMessage_Command{
			Command: cmd,
		},
	}
	return tc.hub.SendToNode(nodeID, msg)
}

// UpdateRateLimit 更新速率限制
func (tc *TUIClient) UpdateRateLimit(nodeID string, rps int32) error {
	msg := &pb.ControlMessage{
		MessageId: generateMessageID(),
		Message: &pb.ControlMessage_RateLimit{
			RateLimit: &pb.RateLimitUpdate{
				RequestsPerSecond: rps,
			},
		},
	}
	return tc.hub.SendToNode(nodeID, msg)
}

// GetNodes 获取所有节点
func (tc *TUIClient) GetNodes() map[string]*NodeInfo {
	return tc.hub.GetNodes()
}

// GetNodeMetrics 获取节点指标
func (tc *TUIClient) GetNodeMetrics(nodeID string) (*pb.MetricsUpdate, error) {
	return tc.hub.GetNodeMetrics(nodeID)
}

// GetUpdates 获取更新通道
func (tc *TUIClient) GetUpdates() <-chan interface{} {
	return tc.updateCh
}

// IsActive 检查是否活跃
func (tc *TUIClient) IsActive() bool {
	return tc.active.Load()
}

// Close 关闭客户端
func (tc *TUIClient) Close() {
	if tc.active.CompareAndSwap(true, false) {
		close(tc.stopCh)
		tc.hub.UnregisterTUIClient(tc.ID)
		tc.logger.Info("TUI client closed", "client_id", tc.ID)
	}
}

// UI更新类型定义

type NodeUpdate struct {
	NodeID string    `json:"node_id"`
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

type TaskUpdate struct {
	TaskID string    `json:"task_id"`
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

type MetricsUpdate struct {
	Source string      `json:"source"`
	Data   interface{} `json:"data"`
	Time   time.Time   `json:"time"`
}

type GenericUpdate struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
	Time time.Time   `json:"time"`
}