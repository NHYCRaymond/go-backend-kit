package communication

import (
	"log/slog"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// EventBus 事件总线
type EventBus struct {
	// Redis客户端
	redis       *redis.Client
	redisPrefix string

	// 本地订阅者
	subscribers map[EventType][]EventHandler
	subMu       sync.RWMutex

	// 事件流
	eventStream chan *Event
	
	// Redis订阅
	pubsub      *redis.PubSub
	
	// 运行状态
	running     bool
	stopCh      chan struct{}
	
	// 日志
	logger      *slog.Logger
}

// Event 事件
type Event struct {
	ID        string      `json:"id"`
	Type      EventType   `json:"type"`
	Source    string      `json:"source"`
	Target    string      `json:"target"` // 特定目标或 "*" 表示广播
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// EventType 事件类型
type EventType string

const (
	// 节点事件
	EventNodeOnline    EventType = "node.online"
	EventNodeOffline   EventType = "node.offline"
	EventNodeHeartbeat EventType = "node.heartbeat"
	EventNodeError     EventType = "node.error"
	
	// 任务事件
	EventTaskStart     EventType = "task.start"
	EventTaskComplete  EventType = "task.complete"
	EventTaskFailed    EventType = "task.failed"
	EventTaskRetry     EventType = "task.retry"
	EventTaskCancelled EventType = "task.cancelled"
	
	// 控制事件
	EventRateChange    EventType = "control.rate_change"
	EventConfigUpdate  EventType = "control.config_update"
	EventPause         EventType = "control.pause"
	EventResume        EventType = "control.resume"
	EventStop          EventType = "control.stop"
	
	// 监控事件
	EventMetricsUpdate EventType = "metrics.update"
	EventAlert         EventType = "metrics.alert"
	EventResourceHigh  EventType = "metrics.resource_high"
	
	// 系统事件
	EventSystemStart   EventType = "system.start"
	EventSystemStop    EventType = "system.stop"
	EventSystemError   EventType = "system.error"
)

// EventHandler 事件处理器
type EventHandler func(event *Event)

// NewEventBus 创建事件总线
func NewEventBus(redisClient *redis.Client, prefix string, logger *slog.Logger) *EventBus {
	return &EventBus{
		redis:       redisClient,
		redisPrefix: prefix,
		subscribers: make(map[EventType][]EventHandler),
		eventStream: make(chan *Event, 1000),
		stopCh:      make(chan struct{}),
		logger:      logger,
	}
}

// Start 启动事件总线
func (eb *EventBus) Start(ctx context.Context) error {
	if eb.running {
		return fmt.Errorf("event bus already running")
	}
	eb.running = true

	eb.logger.Info("Starting event bus")

	// 启动本地事件处理
	go eb.processLocalEvents(ctx)

	// 启动Redis订阅
	if eb.redis != nil {
		go eb.startRedisSubscription(ctx)
	}

	// 启动事件持久化
	go eb.persistEvents(ctx)

	// 启动定期清理任务
	if eb.redis != nil {
		go eb.cleanupOldEvents(ctx)
	}

	eb.logger.Info("Event bus started")
	return nil
}

// Stop 停止事件总线
func (eb *EventBus) Stop(ctx context.Context) error {
	if !eb.running {
		return fmt.Errorf("event bus not running")
	}

	eb.logger.Info("Stopping event bus")

	// 发送停止信号
	close(eb.stopCh)

	// 关闭Redis订阅
	if eb.pubsub != nil {
		if err := eb.pubsub.Close(); err != nil {
			eb.logger.Error("Failed to close Redis subscription", "error", err)
		}
	}

	eb.running = false
	eb.logger.Info("Event bus stopped")
	return nil
}

// Publish 发布事件
func (eb *EventBus) Publish(event *Event) {
	if !eb.running {
		eb.logger.Warn("Event bus not running, dropping event",
			"event_id", event.ID,
			"type", event.Type)
		return
	}

	// 设置时间戳
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// 发送到本地流
	select {
	case eb.eventStream <- event:
		eb.logger.Debug("Event published",
			"event_id", event.ID,
			"type", event.Type,
			"source", event.Source)
	default:
		eb.logger.Warn("Event stream full, dropping event",
			"event_id", event.ID,
			"type", event.Type)
	}

	// 发布到Redis（异步）
	if eb.redis != nil {
		go eb.publishToRedis(event)
	}
}

// Subscribe 订阅事件
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	eb.subMu.Lock()
	defer eb.subMu.Unlock()

	eb.subscribers[eventType] = append(eb.subscribers[eventType], handler)
	eb.logger.Debug("Event handler subscribed", "type", eventType)
}

// Unsubscribe 取消订阅
func (eb *EventBus) Unsubscribe(eventType EventType, handler EventHandler) {
	eb.subMu.Lock()
	defer eb.subMu.Unlock()

	handlers := eb.subscribers[eventType]
	for i, h := range handlers {
		// 比较函数指针
		if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
			eb.subscribers[eventType] = append(handlers[:i], handlers[i+1:]...)
			eb.logger.Debug("Event handler unsubscribed", "type", eventType)
			return
		}
	}
}

// processLocalEvents 处理本地事件
func (eb *EventBus) processLocalEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-eb.stopCh:
			return
		case event := <-eb.eventStream:
			eb.handleEvent(event)
		}
	}
}

// handleEvent 处理单个事件
func (eb *EventBus) handleEvent(event *Event) {
	eb.subMu.RLock()
	
	// 获取该事件类型的所有处理器
	handlers := eb.subscribers[event.Type]
	
	// 获取通配符处理器
	wildcardHandlers := eb.subscribers[EventType("*")]
	
	eb.subMu.RUnlock()

	// 调用处理器
	for _, handler := range handlers {
		go eb.safeInvokeHandler(handler, event)
	}

	// 调用通配符处理器
	for _, handler := range wildcardHandlers {
		go eb.safeInvokeHandler(handler, event)
	}
}

// safeInvokeHandler 安全调用处理器
func (eb *EventBus) safeInvokeHandler(handler EventHandler, event *Event) {
	defer func() {
		if r := recover(); r != nil {
			eb.logger.Error("Event handler panic",
				"event_id", event.ID,
				"type", event.Type,
				"error", r)
		}
	}()

	handler(event)
}

// publishToRedis 发布事件到Redis
func (eb *EventBus) publishToRedis(event *Event) {
	// 序列化事件
	data, err := json.Marshal(event)
	if err != nil {
		eb.logger.Error("Failed to marshal event",
			"event_id", event.ID,
			"error", err)
		return
	}

	// 发布到Redis频道
	channel := fmt.Sprintf("%s:events:%s", eb.redisPrefix, event.Type)
	if err := eb.redis.Publish(context.Background(), channel, data).Err(); err != nil {
		eb.logger.Error("Failed to publish event to Redis",
			"event_id", event.ID,
			"channel", channel,
			"error", err)
	}
}

// startRedisSubscription 启动Redis订阅
func (eb *EventBus) startRedisSubscription(ctx context.Context) {
	// 订阅所有事件频道
	pattern := fmt.Sprintf("%s:events:*", eb.redisPrefix)
	eb.pubsub = eb.redis.PSubscribe(ctx, pattern)

	// 等待订阅确认
	if _, err := eb.pubsub.Receive(ctx); err != nil {
		eb.logger.Error("Failed to subscribe to Redis", "error", err)
		return
	}

	eb.logger.Info("Redis subscription started", "pattern", pattern)

	// 接收消息
	ch := eb.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case <-eb.stopCh:
			return
		case msg := <-ch:
			// 解析事件
			var event Event
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				eb.logger.Error("Failed to unmarshal Redis event",
					"channel", msg.Channel,
					"error", err)
				continue
			}

			// 处理事件
			eb.handleEvent(&event)
		}
	}
}

// persistEvents 持久化事件
func (eb *EventBus) persistEvents(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	batch := make([]*Event, 0, 100)

	for {
		select {
		case <-ctx.Done():
			eb.flushEvents(batch)
			return
		case <-eb.stopCh:
			eb.flushEvents(batch)
			return
		case event := <-eb.eventStream:
			batch = append(batch, event)
			
			// 批量持久化
			if len(batch) >= 100 {
				eb.flushEvents(batch)
				batch = make([]*Event, 0, 100)
			}
		case <-ticker.C:
			if len(batch) > 0 {
				eb.flushEvents(batch)
				batch = make([]*Event, 0, 100)
			}
		}
	}
}

// flushEvents 批量持久化事件
func (eb *EventBus) flushEvents(events []*Event) {
	if len(events) == 0 || eb.redis == nil {
		return
	}

	ctx := context.Background()
	pipe := eb.redis.Pipeline()

	// Group events by type for efficient processing
	eventsByType := make(map[EventType][]*Event)
	for _, event := range events {
		eventsByType[event.Type] = append(eventsByType[event.Type], event)
	}

	for eventType, typeEvents := range eventsByType {
		key := fmt.Sprintf("%s:events:history:%s", eb.redisPrefix, eventType)
		
		// Add all events of this type
		for _, event := range typeEvents {
			// 序列化事件
			data, err := json.Marshal(event)
			if err != nil {
				eb.logger.Error("Failed to marshal event for persistence",
					"event_id", event.ID,
					"error", err)
				continue
			}

			// 存储到Redis（使用有序集合，按时间戳排序）
			pipe.ZAdd(ctx, key, &redis.Z{
				Score:  float64(event.Timestamp.Unix()),
				Member: data,
			})
		}

		// Remove events older than 7 days using ZREMRANGEBYSCORE
		// This ensures old events are cleaned up automatically
		sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour).Unix()
		pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", sevenDaysAgo))
	}

	if _, err := pipe.Exec(ctx); err != nil {
		eb.logger.Error("Failed to persist events",
			"count", len(events),
			"error", err)
	} else {
		eb.logger.Debug("Events persisted and old events cleaned", "count", len(events))
	}
}

// GetEventHistory 获取事件历史
func (eb *EventBus) GetEventHistory(eventType EventType, from, to time.Time, limit int) ([]*Event, error) {
	if eb.redis == nil {
		return nil, fmt.Errorf("Redis not available")
	}

	ctx := context.Background()
	key := fmt.Sprintf("%s:events:history:%s", eb.redisPrefix, eventType)

	// 使用时间戳作为分数范围查询
	opt := &redis.ZRangeBy{
		Min:    fmt.Sprintf("%d", from.Unix()),
		Max:    fmt.Sprintf("%d", to.Unix()),
		Offset: 0,
		Count:  int64(limit),
	}

	results, err := eb.redis.ZRangeByScore(ctx, key, opt).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get event history: %w", err)
	}

	events := make([]*Event, 0, len(results))
	for _, result := range results {
		var event Event
		if err := json.Unmarshal([]byte(result), &event); err != nil {
			eb.logger.Error("Failed to unmarshal historical event", "error", err)
			continue
		}
		events = append(events, &event)
	}

	return events, nil
}

// cleanupOldEvents 定期清理过期的历史事件
func (eb *EventBus) cleanupOldEvents(ctx context.Context) {
	ticker := time.NewTicker(time.Hour) // 每小时清理一次
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-eb.stopCh:
			return
		case <-ticker.C:
			eb.performCleanup(ctx)
		}
	}
}

// performCleanup 执行清理操作
func (eb *EventBus) performCleanup(ctx context.Context) {
	// 获取所有事件类型
	eventTypes := []EventType{
		EventNodeOnline, EventNodeOffline, EventNodeHeartbeat,
		EventTaskStart, EventTaskComplete, EventTaskFailed,
		EventMetricsUpdate, EventAlert,
	}

	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour).Unix()
	cleanedTotal := int64(0)

	for _, eventType := range eventTypes {
		key := fmt.Sprintf("%s:events:history:%s", eb.redisPrefix, eventType)
		
		// Remove events older than 7 days
		removed, err := eb.redis.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", sevenDaysAgo)).Result()
		if err != nil {
			eb.logger.Error("Failed to cleanup old events",
				"event_type", eventType,
				"error", err)
			continue
		}
		
		if removed > 0 {
			cleanedTotal += removed
			eb.logger.Debug("Cleaned old events",
				"event_type", eventType,
				"removed_count", removed)
		}
	}

	if cleanedTotal > 0 {
		eb.logger.Info("Event cleanup completed",
			"total_removed", cleanedTotal,
			"retention_days", 7)
	}
}

// GetEventStats 获取事件统计
func (eb *EventBus) GetEventStats() map[EventType]int64 {
	stats := make(map[EventType]int64)
	
	if eb.redis == nil {
		return stats
	}

	ctx := context.Background()
	
	// 获取所有事件类型
	eventTypes := []EventType{
		EventNodeOnline, EventNodeOffline, EventNodeHeartbeat,
		EventTaskStart, EventTaskComplete, EventTaskFailed,
		EventMetricsUpdate, EventAlert,
	}

	for _, eventType := range eventTypes {
		key := fmt.Sprintf("%s:events:history:%s", eb.redisPrefix, eventType)
		count, err := eb.redis.ZCard(ctx, key).Result()
		if err == nil {
			stats[eventType] = count
		}
	}

	return stats
}

// EmitNodeEvent 发送节点事件（辅助方法）
func (eb *EventBus) EmitNodeEvent(nodeID string, eventType EventType, data interface{}) {
	event := &Event{
		ID:        generateEventID(),
		Type:      eventType,
		Source:    nodeID,
		Target:    "*",
		Timestamp: time.Now(),
		Data:      data,
	}
	eb.Publish(event)
}

// EmitTaskEvent 发送任务事件（辅助方法）
func (eb *EventBus) EmitTaskEvent(taskID string, eventType EventType, data interface{}) {
	event := &Event{
		ID:        fmt.Sprintf("task_%s_%s", taskID, generateEventID()),
		Type:      eventType,
		Source:    "task_manager",
		Target:    "*",
		Timestamp: time.Now(),
		Data:      data,
	}
	eb.Publish(event)
}

// EmitSystemEvent 发送系统事件（辅助方法）
func (eb *EventBus) EmitSystemEvent(eventType EventType, data interface{}) {
	event := &Event{
		ID:        fmt.Sprintf("sys_%s", generateEventID()),
		Type:      eventType,
		Source:    "system",
		Target:    "*",
		Timestamp: time.Now(),
		Data:      data,
	}
	eb.Publish(event)
}