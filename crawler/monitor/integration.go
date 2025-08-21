package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/NHYCRaymond/go-backend-kit/crawler/types"
	"github.com/go-redis/redis/v8"
)

// CoordinatorIntegration 集成到Coordinator的监控服务
type CoordinatorIntegration struct {
	detector      *ChangeDetector
	redisClient   *redis.Client
	logger        *slog.Logger
	subscriptions map[string][]types.Subscriber // source:timeSlot -> subscribers
	webhookURL    string
}

// NewCoordinatorIntegration 创建Coordinator集成
func NewCoordinatorIntegration(detector *ChangeDetector, redisClient *redis.Client, webhookURL string, logger *slog.Logger) *CoordinatorIntegration {
	ci := &CoordinatorIntegration{
		detector:      detector,
		redisClient:   redisClient,
		logger:        logger,
		webhookURL:    webhookURL,
		subscriptions: make(map[string][]types.Subscriber),
	}

	// 初始化19:00-22:00时段的订阅
	ci.setupEveningSubscriptions()

	return ci
}

// setupEveningSubscriptions 设置晚间时段订阅
func (ci *CoordinatorIntegration) setupEveningSubscriptions() {
	// 4个爬虫源
	sources := []string{
		"drip_ground_board",
		"venue_massage_site",
		"pospal_venue_5662377",
		"pospal_venue_classroom",
	}

	// 19:00-22:00的时段
	timeSlots := []string{
		"19:00", "19:30",
		"20:00", "20:30",
		"21:00", "21:30",
		"22:00",
	}

	subscriber := types.Subscriber{
		UserID:     "venue_monitor_system",
		WebhookURL: ci.webhookURL,
		UserName:   "场地监控系统",
	}

	// 为每个源和时段组合创建订阅
	for _, source := range sources {
		for _, slot := range timeSlots {
			// 创建多种时段格式的key，以匹配不同的时段表示方式
			keys := []string{
				fmt.Sprintf("%s:%s", source, slot),
				fmt.Sprintf("%s:*%s*", source, slot),
			}
			
			for _, key := range keys {
				ci.subscriptions[key] = append(ci.subscriptions[key], subscriber)
			}
		}
	}

	ci.logger.Info("Evening subscriptions configured",
		"sources", sources,
		"time_range", "19:00-22:00",
		"total_subscriptions", len(ci.subscriptions))
}

// ListenForResults 监听爬虫结果并处理
func (ci *CoordinatorIntegration) ListenForResults(ctx context.Context) {
	// 订阅Redis中的爬虫结果
	pubsub := ci.redisClient.Subscribe(ctx, "crawler:results")
	defer pubsub.Close()

	ch := pubsub.Channel()
	
	ci.logger.Info("Listening for crawler results on Redis channel", "channel", "crawler:results")

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			ci.processResultMessage(msg.Payload)
		}
	}
}

// processResultMessage 处理结果消息
func (ci *CoordinatorIntegration) processResultMessage(payload string) {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		ci.logger.Error("Failed to unmarshal result", "error", err)
		return
	}

	// 提取任务ID和源
	taskID, _ := result["task_id"].(string)
	source, _ := result["source"].(string)

	// 检查是否是我们监控的4个源之一
	if !ci.isMonitoredSource(source) {
		return
	}

	ci.logger.Info("Processing monitored source result",
		"task_id", taskID,
		"source", source)

	// 处理结果
	if err := ci.detector.ProcessTaskResult(taskID, result); err != nil {
		ci.logger.Error("Failed to process task result",
			"task_id", taskID,
			"source", source,
			"error", err)
	}
}

// isMonitoredSource 检查是否是监控的源
func (ci *CoordinatorIntegration) isMonitoredSource(source string) bool {
	monitoredSources := []string{
		"drip_ground_board",
		"venue_massage_site",
		"pospal_venue_5662377",
		"pospal_venue_classroom",
	}

	for _, ms := range monitoredSources {
		if source == ms {
			return true
		}
	}
	return false
}

// GetSubscribersForSlot 根据源和时段获取订阅者
func (ci *CoordinatorIntegration) GetSubscribersForSlot(source, timeSlot string) []types.Subscriber {
	// 检查时段是否包含19:00-22:00
	if !ci.isEveningSlot(timeSlot) {
		return nil
	}

	// 直接返回订阅者
	return []types.Subscriber{
		{
			UserID:     "venue_monitor_system",
			WebhookURL: ci.webhookURL,
			UserName:   "场地监控系统",
		},
	}
}

// isEveningSlot 检查是否是晚间时段
func (ci *CoordinatorIntegration) isEveningSlot(timeSlot string) bool {
	// 检查时段字符串中是否包含19:00-22:00之间的时间
	eveningHours := []string{"19:", "20:", "21:"}
	
	for _, hour := range eveningHours {
		if strings.Contains(timeSlot, hour) {
			return true
		}
	}
	
	// 也检查是否包含22:00作为结束时间
	if strings.Contains(timeSlot, "-22:00") {
		return true
	}
	
	return false
}

// EnhancedSubscriptionClient 增强的订阅客户端，集成晚间时段订阅
type EnhancedSubscriptionClient struct {
	*SubscriptionClient
	integration *CoordinatorIntegration
}

// NewEnhancedSubscriptionClient 创建增强的订阅客户端
func NewEnhancedSubscriptionClient(baseURL string, timeout int, integration *CoordinatorIntegration) *EnhancedSubscriptionClient {
	return &EnhancedSubscriptionClient{
		SubscriptionClient: NewSubscriptionClient(baseURL, timeout),
		integration:        integration,
	}
}

// GetSubscribers 获取订阅者，包括晚间时段的自动订阅
func (esc *EnhancedSubscriptionClient) GetSubscribers(ctx context.Context, source, timeSlot string) []types.Subscriber {
	// 首先检查是否是监控的源和时段
	if esc.integration != nil {
		if subscribers := esc.integration.GetSubscribersForSlot(source, timeSlot); len(subscribers) > 0 {
			return subscribers
		}
	}
	
	// 否则使用基础实现
	return esc.SubscriptionClient.GetSubscribers(ctx, source, timeSlot)
}