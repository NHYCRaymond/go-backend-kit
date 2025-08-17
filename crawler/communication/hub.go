package communication

import (
	"log/slog"
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/NHYCRaymond/go-backend-kit/crawler/proto"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CommunicationHub 通信中心，管理所有节点连接和消息路由
type CommunicationHub struct {
	pb.UnimplementedCrawlerNodeServer

	// 节点连接管理
	nodes   map[string]*NodeConnection
	nodesMu sync.RWMutex

	// 事件总线
	eventBus *EventBus

	// TUI客户端管理
	tuiClients   map[string]*TUIClient
	clientsMu    sync.RWMutex

	// 指标聚合
	metricsAgg *MetricsAggregator

	// 消息路由
	router *MessageRouter

	// 配置
	config *HubConfig

	// 运行状态
	running atomic.Bool
	stopCh  chan struct{}

	// 依赖
	redis  *redis.Client
	logger *slog.Logger
}

// HubConfig 通信中心配置
type HubConfig struct {
	// 服务地址
	GRPCAddr string `json:"grpc_addr"`
	
	// Redis配置
	RedisAddr   string `json:"redis_addr"`
	RedisPrefix string `json:"redis_prefix"`
	
	// 心跳配置
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
	HeartbeatTimeout  time.Duration `json:"heartbeat_timeout"`
	
	// 连接配置
	MaxConnections    int           `json:"max_connections"`
	ConnectionTimeout time.Duration `json:"connection_timeout"`
	
	// 缓冲配置
	MessageBufferSize int `json:"message_buffer_size"`
	EventBufferSize   int `json:"event_buffer_size"`
}

// DefaultHubConfig 默认配置
func DefaultHubConfig() *HubConfig {
	return &HubConfig{
		GRPCAddr:          ":50051",
		RedisAddr:         "localhost:6379",
		RedisPrefix:       "crawler",
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
		MaxConnections:    1000,
		ConnectionTimeout: 10 * time.Second,
		MessageBufferSize: 1000,
		EventBufferSize:   10000,
	}
}

// NewCommunicationHub 创建通信中心
func NewCommunicationHub(config *HubConfig, logger *slog.Logger) (*CommunicationHub, error) {
	if config == nil {
		config = DefaultHubConfig()
	}

	if logger == nil {
		logger = logging.GetLogger()
	}

	// 创建Redis客户端
	redisClient := redis.NewClient(&redis.Options{
		Addr: config.RedisAddr,
	})

	// 测试Redis连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	hub := &CommunicationHub{
		nodes:      make(map[string]*NodeConnection),
		tuiClients: make(map[string]*TUIClient),
		config:     config,
		stopCh:     make(chan struct{}),
		redis:      redisClient,
		logger:     logger,
	}

	// 初始化组件
	hub.eventBus = NewEventBus(redisClient, config.RedisPrefix, logger)
	hub.metricsAgg = NewMetricsAggregator(logger)
	hub.router = NewMessageRouter(hub, logger)

	return hub, nil
}

// Start 启动通信中心
func (h *CommunicationHub) Start(ctx context.Context) error {
	if !h.running.CompareAndSwap(false, true) {
		return fmt.Errorf("hub already running")
	}

	h.logger.Info("Starting communication hub", "addr", h.config.GRPCAddr)

	// 启动事件总线
	if err := h.eventBus.Start(ctx); err != nil {
		return fmt.Errorf("failed to start event bus: %w", err)
	}

	// 启动指标聚合
	go h.metricsAgg.Start(ctx)

	// 启动消息路由
	go h.router.Start(ctx)

	// 启动心跳检查
	go h.heartbeatChecker(ctx)

	// 启动gRPC服务器
	go h.startGRPCServer(ctx)

	// 启动清理任务
	go h.cleanupTask(ctx)

	h.logger.Info("Communication hub started successfully")
	return nil
}

// Stop 停止通信中心
func (h *CommunicationHub) Stop(ctx context.Context) error {
	if !h.running.CompareAndSwap(true, false) {
		return fmt.Errorf("hub not running")
	}

	h.logger.Info("Stopping communication hub")

	// 发送停止信号
	close(h.stopCh)

	// 断开所有节点连接
	h.disconnectAllNodes(ctx)

	// 停止事件总线
	if err := h.eventBus.Stop(ctx); err != nil {
		h.logger.Error("Failed to stop event bus", "error", err)
	}

	// 停止指标聚合
	h.metricsAgg.Stop()

	// 停止消息路由
	h.router.Stop()

	h.logger.Info("Communication hub stopped")
	return nil
}

// Connect 实现gRPC双向流连接
func (h *CommunicationHub) Connect(stream pb.CrawlerNode_ConnectServer) error {
	ctx := stream.Context()
	
	// 等待节点注册消息
	msg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive registration: %w", err)
	}

	// 验证是注册消息
	register, ok := msg.Message.(*pb.NodeMessage_Register)
	if !ok {
		return fmt.Errorf("first message must be registration")
	}

	nodeID := register.Register.NodeId
	h.logger.Info("Node connecting", "node_id", nodeID)

	// 创建节点连接
	conn := NewNodeConnection(nodeID, stream, h.logger)
	
	// 注册节点
	if err := h.registerNode(nodeID, conn, register.Register); err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	// 发送确认消息
	ack := &pb.ControlMessage{
		MessageId: generateMessageID(),
		Timestamp: timestamppb.Now(),
		Message: &pb.ControlMessage_Ack{
			Ack: &pb.Acknowledgment{
				MessageId: msg.MessageId,
				Success:   true,
			},
		},
	}
	if err := stream.Send(ack); err != nil {
		h.unregisterNode(nodeID)
		return fmt.Errorf("failed to send acknowledgment: %w", err)
	}

	// 发布节点上线事件
	h.publishNodeEvent(nodeID, pb.EventType_EVENT_NODE_ONLINE, nil)

	// 启动连接处理
	defer func() {
		h.unregisterNode(nodeID)
		h.publishNodeEvent(nodeID, pb.EventType_EVENT_NODE_OFFLINE, nil)
	}()

	// 处理消息循环
	return conn.HandleMessages(ctx)
}

// GetMetrics 获取节点指标
func (h *CommunicationHub) GetMetrics(ctx context.Context, req *pb.GetMetricsRequest) (*pb.MetricsResponse, error) {
	h.nodesMu.RLock()
	conn, exists := h.nodes[req.NodeId]
	h.nodesMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node not found: %s", req.NodeId)
	}

	return conn.GetMetrics(ctx, req)
}

// UpdateConfig 更新节点配置
func (h *CommunicationHub) UpdateConfig(ctx context.Context, req *pb.ConfigUpdateRequest) (*pb.ConfigUpdateResponse, error) {
	h.nodesMu.RLock()
	conn, exists := h.nodes[req.NodeId]
	h.nodesMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node not found: %s", req.NodeId)
	}

	// 发送配置更新消息
	msg := &pb.ControlMessage{
		MessageId: generateMessageID(),
		Timestamp: timestamppb.Now(),
		Message: &pb.ControlMessage_ConfigUpdate{
			ConfigUpdate: &pb.ConfigUpdate{
				Config:           req.Config,
				RestartRequired:  false,
			},
		},
	}

	if err := conn.Send(msg); err != nil {
		return nil, err
	}

	return &pb.ConfigUpdateResponse{
		Success: true,
	}, nil
}

// ExecuteCommand 执行节点命令
func (h *CommunicationHub) ExecuteCommand(ctx context.Context, req *pb.CommandRequest) (*pb.CommandResponse, error) {
	h.nodesMu.RLock()
	conn, exists := h.nodes[req.NodeId]
	h.nodesMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node not found: %s", req.NodeId)
	}

	// 发送命令消息
	msg := &pb.ControlMessage{
		MessageId: generateMessageID(),
		Timestamp: timestamppb.Now(),
		Message: &pb.ControlMessage_Command{
			Command: req.Command,
		},
	}

	response, err := conn.SendWithResponse(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &pb.CommandResponse{
		Success: response.Success,
		ErrorMessage: response.ErrorMessage,
		Result: response.Data,
	}, nil
}

// registerNode 注册节点
func (h *CommunicationHub) registerNode(nodeID string, conn *NodeConnection, info *pb.RegisterNode) error {
	h.nodesMu.Lock()
	defer h.nodesMu.Unlock()

	// 检查是否已存在
	if oldConn, exists := h.nodes[nodeID]; exists {
		oldConn.Close()
	}

	// 保存连接
	h.nodes[nodeID] = conn

	// 更新节点信息
	conn.UpdateInfo(info)

	h.logger.Info("Node registered",
		"node_id", nodeID,
		"hostname", info.Hostname,
		"capabilities", info.Capabilities)

	return nil
}

// unregisterNode 注销节点
func (h *CommunicationHub) unregisterNode(nodeID string) {
	h.nodesMu.Lock()
	defer h.nodesMu.Unlock()

	if conn, exists := h.nodes[nodeID]; exists {
		conn.Close()
		delete(h.nodes, nodeID)
		h.logger.Info("Node unregistered", "node_id", nodeID)
	}
}

// disconnectAllNodes 断开所有节点连接
func (h *CommunicationHub) disconnectAllNodes(ctx context.Context) {
	h.nodesMu.Lock()
	defer h.nodesMu.Unlock()

	for nodeID, conn := range h.nodes {
		conn.Close()
		h.logger.Info("Disconnecting node", "node_id", nodeID)
	}

	h.nodes = make(map[string]*NodeConnection)
}

// heartbeatChecker 心跳检查
func (h *CommunicationHub) heartbeatChecker(ctx context.Context) {
	ticker := time.NewTicker(h.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.checkHeartbeats()
		}
	}
}

// checkHeartbeats 检查所有节点心跳
func (h *CommunicationHub) checkHeartbeats() {
	now := time.Now()
	timeout := h.config.HeartbeatTimeout

	h.nodesMu.RLock()
	nodes := make([]*NodeConnection, 0, len(h.nodes))
	for _, conn := range h.nodes {
		nodes = append(nodes, conn)
	}
	h.nodesMu.RUnlock()

	for _, conn := range nodes {
		if now.Sub(conn.LastHeartbeat()) > timeout {
			h.logger.Warn("Node heartbeat timeout",
				"node_id", conn.NodeID(),
				"last_heartbeat", conn.LastHeartbeat())
			
			// 发布节点离线事件
			h.publishNodeEvent(conn.NodeID(), pb.EventType_EVENT_NODE_OFFLINE, nil)
			
			// 断开连接
			h.unregisterNode(conn.NodeID())
		}
	}
}

// cleanupTask 清理任务
func (h *CommunicationHub) cleanupTask(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.cleanup()
		}
	}
}

// cleanup 执行清理
func (h *CommunicationHub) cleanup() {
	// 清理断开的连接
	h.nodesMu.Lock()
	for nodeID, conn := range h.nodes {
		if !conn.IsConnected() {
			delete(h.nodes, nodeID)
			h.logger.Debug("Cleaned up disconnected node", "node_id", nodeID)
		}
	}
	h.nodesMu.Unlock()

	// 清理过期的TUI客户端
	h.clientsMu.Lock()
	for clientID, client := range h.tuiClients {
		if !client.IsActive() {
			delete(h.tuiClients, clientID)
			h.logger.Debug("Cleaned up inactive TUI client", "client_id", clientID)
		}
	}
	h.clientsMu.Unlock()
}

// startGRPCServer 启动gRPC服务器
func (h *CommunicationHub) startGRPCServer(ctx context.Context) error {
	lis, err := net.Listen("tcp", h.config.GRPCAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// 创建gRPC服务器
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(10 * 1024 * 1024), // 10MB
		grpc.MaxSendMsgSize(10 * 1024 * 1024), // 10MB
	}
	
	server := grpc.NewServer(opts...)
	pb.RegisterCrawlerNodeServer(server, h)

	h.logger.Info("gRPC server listening", "addr", h.config.GRPCAddr)

	// 启动服务器
	go func() {
		if err := server.Serve(lis); err != nil {
			h.logger.Error("gRPC server error", "error", err)
		}
	}()

	// 等待停止信号
	<-h.stopCh
	
	// 优雅关闭
	server.GracefulStop()
	return nil
}

// publishNodeEvent 发布节点事件
func (h *CommunicationHub) publishNodeEvent(nodeID string, eventType pb.EventType, data interface{}) {
	event := &Event{
		ID:        generateEventID(),
		Type:      EventType(eventType),
		Source:    nodeID,
		Target:    "*",
		Timestamp: time.Now(),
		Data:      data,
	}

	h.eventBus.Publish(event)
}

// SendToNode 发送消息到指定节点
func (h *CommunicationHub) SendToNode(nodeID string, msg *pb.ControlMessage) error {
	h.nodesMu.RLock()
	conn, exists := h.nodes[nodeID]
	h.nodesMu.RUnlock()

	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	return conn.Send(msg)
}

// BroadcastToNodes 广播消息到所有节点
func (h *CommunicationHub) BroadcastToNodes(msg *pb.ControlMessage) {
	h.nodesMu.RLock()
	nodes := make([]*NodeConnection, 0, len(h.nodes))
	for _, conn := range h.nodes {
		nodes = append(nodes, conn)
	}
	h.nodesMu.RUnlock()

	for _, conn := range nodes {
		if err := conn.Send(msg); err != nil {
			h.logger.Error("Failed to send to node",
				"node_id", conn.NodeID(),
				"error", err)
		}
	}
}

// RegisterTUIClient 注册TUI客户端
func (h *CommunicationHub) RegisterTUIClient(client *TUIClient) error {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	h.tuiClients[client.ID] = client
	h.logger.Info("TUI client registered", "client_id", client.ID)
	return nil
}

// UnregisterTUIClient 注销TUI客户端
func (h *CommunicationHub) UnregisterTUIClient(clientID string) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	if client, exists := h.tuiClients[clientID]; exists {
		client.Close()
		delete(h.tuiClients, clientID)
		h.logger.Info("TUI client unregistered", "client_id", clientID)
	}
}

// GetNodes 获取所有节点信息
func (h *CommunicationHub) GetNodes() map[string]*NodeInfo {
	h.nodesMu.RLock()
	defer h.nodesMu.RUnlock()

	nodes := make(map[string]*NodeInfo)
	for nodeID, conn := range h.nodes {
		nodes[nodeID] = conn.GetInfo()
	}
	return nodes
}

// GetNodeMetrics 获取节点指标
func (h *CommunicationHub) GetNodeMetrics(nodeID string) (*pb.MetricsUpdate, error) {
	h.nodesMu.RLock()
	conn, exists := h.nodes[nodeID]
	h.nodesMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	return conn.GetLatestMetrics(), nil
}

// Subscribe 订阅事件
func (h *CommunicationHub) Subscribe(eventType EventType, handler EventHandler) {
	h.eventBus.Subscribe(eventType, handler)
}

// Unsubscribe 取消订阅
func (h *CommunicationHub) Unsubscribe(eventType EventType, handler EventHandler) {
	h.eventBus.Unsubscribe(eventType, handler)
}