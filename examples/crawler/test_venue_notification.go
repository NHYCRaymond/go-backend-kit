package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/monitor"
	"github.com/NHYCRaymond/go-backend-kit/crawler/notification"
	"github.com/NHYCRaymond/go-backend-kit/crawler/parser"
	"github.com/NHYCRaymond/go-backend-kit/crawler/types"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// 您的钉钉机器人 webhook
	DINGTALK_WEBHOOK = "https://oapi.dingtalk.com/robot/send?access_token=348d61c66dc7ba986ebd59696a2850f1697ed1b397995737a2213c6cc025217f"
)

func main() {
	// 设置日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("Starting venue notification test")

	// 初始化 Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	defer redisClient.Close()

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	logger.Info("Connected to Redis")

	// 初始化 MongoDB
	mongoURI := "mongodb://localhost:27017"
	clientOptions := options.Client().ApplyURI(mongoURI)
	mongoClient, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer mongoClient.Disconnect(ctx)

	// Ping MongoDB
	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatal("Failed to ping MongoDB:", err)
	}
	
	mongoDatabase := mongoClient.Database("crawler")
	logger.Info("Connected to MongoDB")

	// 创建变更检测器配置
	changeDetectorConfig := &types.ChangeDetectorConfig{
		Enabled:                true,
		WatchFields:            []string{"status", "available", "price"},
		IgnoreFields:           []string{"updated_at", "crawled_at"},
		SubscriptionAPIURL:     "", // 留空使用模拟数据
		SubscriptionAPITimeout: 10,
		CacheTTL:               60,
		Workers:                5,
		QueueSize:              1000,
	}

	// 创建变更检测器
	changeDetector := monitor.NewChangeDetector(
		mongoDatabase,
		redisClient,
		changeDetectorConfig,
		logger,
	)

	// 注册网球场解析器
	changeDetector.RegisterParser("tennis_court", parser.NewTennisVenueParser())
	
	// 启动变更检测器
	if err := changeDetector.Start(); err != nil {
		log.Fatal("Failed to start change detector:", err)
	}
	defer changeDetector.Stop()

	logger.Info("Change detector started")

	// 创建一个测试的订阅客户端，返回我们的钉钉 webhook
	subscriptionClient := monitor.NewSubscriptionClient("", 10)
	subscriptionClient.SetBaseURL("") // 使用模拟模式

	// 模拟第一次爬虫结果（初始状态）
	logger.Info("=== Simulating initial venue data ===")
	initialResult := map[string]interface{}{
		"source": "tennis_court",
		"venues": []interface{}{
			map[string]interface{}{
				"venue_id":   "court_center_1",
				"venue_name": "中央球场1号",
				"time_slot":  fmt.Sprintf("%s 19:00-20:00", time.Now().Format("2006-01-02")),
				"status":     "available",
				"price":      120.0,
				"capacity":   1,
				"available":  1,
			},
			map[string]interface{}{
				"venue_id":   "court_center_2",
				"venue_name": "中央球场2号",
				"time_slot":  fmt.Sprintf("%s 19:00-20:00", time.Now().Format("2006-01-02")),
				"status":     "available",
				"price":      100.0,
				"capacity":   1,
				"available":  1,
			},
			map[string]interface{}{
				"venue_id":   "court_vip",
				"venue_name": "VIP专属球场",
				"time_slot":  fmt.Sprintf("%s 20:00-21:00", time.Now().Format("2006-01-02")),
				"status":     "full",
				"price":      200.0,
				"capacity":   1,
				"available":  0,
			},
		},
	}

	// 处理初始结果
	taskID1 := fmt.Sprintf("test_task_%d", time.Now().Unix())
	if err := changeDetector.ProcessTaskResult(taskID1, initialResult); err != nil {
		logger.Error("Failed to process initial result", "error", err)
	} else {
		logger.Info("Processed initial venue data", "task_id", taskID1)
	}

	// 等待一下
	logger.Info("Waiting 5 seconds before simulating changes...")
	time.Sleep(5 * time.Second)

	// 模拟变更 - 中央球场1号被预订，VIP球场变为可用
	logger.Info("=== Simulating venue changes ===")
	changedResult := map[string]interface{}{
		"source": "tennis_court",
		"venues": []interface{}{
			map[string]interface{}{
				"venue_id":   "court_center_1",
				"venue_name": "中央球场1号",
				"time_slot":  fmt.Sprintf("%s 19:00-20:00", time.Now().Format("2006-01-02")),
				"status":     "full", // 变更：从 available 变为 full
				"price":      120.0,
				"capacity":   1,
				"available":  0, // 变更：从 1 变为 0
			},
			map[string]interface{}{
				"venue_id":   "court_center_2",
				"venue_name": "中央球场2号",
				"time_slot":  fmt.Sprintf("%s 19:00-20:00", time.Now().Format("2006-01-02")),
				"status":     "available", // 无变化
				"price":      100.0,
				"capacity":   1,
				"available":  1,
			},
			map[string]interface{}{
				"venue_id":   "court_vip",
				"venue_name": "VIP专属球场",
				"time_slot":  fmt.Sprintf("%s 20:00-21:00", time.Now().Format("2006-01-02")),
				"status":     "available", // 变更：从 full 变为 available
				"price":      180.0,       // 变更：价格从 200 降到 180
				"capacity":   1,
				"available":  1, // 变更：从 0 变为 1
			},
		},
	}

	// 创建测试推送器
	pusherConfig := &notification.PusherConfig{
		Workers:    2,
		QueueSize:  100,
		RetryTimes: 3,
		RetryDelay: 5,
	}
	pusher := notification.NewNotificationPusher(pusherConfig, logger)
	pusher.Start()
	defer pusher.Stop()

	// 手动创建订阅者并推送（模拟有用户订阅了这些场地）
	subscribers := []types.Subscriber{
		{
			UserID:     "test_user_1",
			WebhookURL: DINGTALK_WEBHOOK,
			UserName:   "测试用户",
		},
	}

	// 处理变更结果
	taskID2 := fmt.Sprintf("test_task_%d", time.Now().Unix())
	
	// 在处理之前，我们先手动检测变更并推送
	logger.Info("Processing changed venue data and sending notifications...")
	
	// 模拟检测到的变更事件
	changeEvents := []types.ChangeEvent{
		{
			ID:           fmt.Sprintf("event_%d_1", time.Now().Unix()),
			Source:       "tennis_court",
			TimeSlot:     fmt.Sprintf("%s 19:00-20:00", time.Now().Format("2006-01-02")),
			VenueID:      "court_center_1",
			VenueName:    "中央球场1号",
			OldStatus:    "available",
			NewStatus:    "full",
			OldAvailable: 1,
			NewAvailable: 0,
			OldPrice:     120.0,
			NewPrice:     120.0,
			Timestamp:    time.Now(),
			Details:      changedResult["venues"].([]interface{})[0].(map[string]interface{}),
		},
		{
			ID:           fmt.Sprintf("event_%d_2", time.Now().Unix()),
			Source:       "tennis_court",
			TimeSlot:     fmt.Sprintf("%s 20:00-21:00", time.Now().Format("2006-01-02")),
			VenueID:      "court_vip",
			VenueName:    "VIP专属球场",
			OldStatus:    "full",
			NewStatus:    "available",
			OldAvailable: 0,
			NewAvailable: 1,
			OldPrice:     200.0,
			NewPrice:     180.0,
			Timestamp:    time.Now(),
			Details:      changedResult["venues"].([]interface{})[2].(map[string]interface{}),
		},
	}

	// 推送通知
	for _, event := range changeEvents {
		logger.Info("Sending notification for venue change",
			"venue", event.VenueName,
			"old_status", event.OldStatus,
			"new_status", event.NewStatus)
		
		pusher.Push(subscribers, event)
	}

	// 也处理到数据库中
	if err := changeDetector.ProcessTaskResult(taskID2, changedResult); err != nil {
		logger.Error("Failed to process changed result", "error", err)
	} else {
		logger.Info("Processed changed venue data", "task_id", taskID2)
	}

	// 等待推送完成
	logger.Info("Waiting for notifications to be sent...")
	time.Sleep(10 * time.Second)

	// 获取统计信息
	stats, err := changeDetector.GetStats(ctx)
	if err == nil {
		logger.Info("Change detector statistics",
			"total_snapshots", stats["total_snapshots"],
			"today_snapshots", stats["today_snapshots"])
	}

	logger.Info("Test completed! Check your DingTalk for notifications.")
}