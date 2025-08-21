package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/monitor"
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

// MockSubscriptionClient 模拟订阅客户端，返回固定的订阅配置
type MockSubscriptionClient struct {
	*monitor.SubscriptionClient
}

// NewMockSubscriptionClient 创建模拟订阅客户端
func NewMockSubscriptionClient() *MockSubscriptionClient {
	return &MockSubscriptionClient{
		SubscriptionClient: monitor.NewSubscriptionClient("", 10),
	}
}

// GetSubscribers 返回19:00-22:00时段的订阅者
func (msc *MockSubscriptionClient) GetSubscribers(ctx context.Context, source, timeSlot string) []types.Subscriber {
	// 检查时段是否在19:00-22:00范围内
	if isEveningSlot(timeSlot) {
		// 对所有4个爬虫源都返回订阅
		if source == "drip_ground_board" || 
		   source == "venue_massage_site" || 
		   source == "pospal_venue_5662377" || 
		   source == "pospal_venue_classroom" {
			return []types.Subscriber{
				{
					UserID:     "venue_monitor",
					WebhookURL: DINGTALK_WEBHOOK,
					UserName:   "场地监控系统",
				},
			}
		}
	}
	return []types.Subscriber{}
}

// isEveningSlot 检查时段是否在19:00-22:00范围内
func isEveningSlot(timeSlot string) bool {
	// 时段格式示例: "2024-01-20 19:00-20:00"
	// 提取小时部分
	for _, hour := range []string{"19:", "20:", "21:"} {
		if len(timeSlot) > 10 && 
		   (timeSlot[11:14] == hour || // 开始时间
		    (len(timeSlot) > 16 && timeSlot[17:20] == hour)) { // 结束时间
			return true
		}
	}
	return false
}

// VenueMonitor 场地监控服务
type VenueMonitor struct {
	detector      *monitor.ChangeDetector
	logger        *slog.Logger
	redisClient   *redis.Client
	mongoDatabase *mongo.Database
}

// NewVenueMonitor 创建场地监控服务
func NewVenueMonitor(logger *slog.Logger) (*VenueMonitor, error) {
	// 初始化 Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	logger.Info("Connected to Redis")

	// 初始化 MongoDB
	mongoURI := "mongodb://localhost:27017"
	clientOptions := options.Client().ApplyURI(mongoURI)
	mongoClient, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping MongoDB
	if err := mongoClient.Ping(ctx, nil); err != nil {
		mongoClient.Disconnect(ctx)
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	mongoDatabase := mongoClient.Database("crawler")
	logger.Info("Connected to MongoDB")

	// 创建变更检测器配置
	changeDetectorConfig := &types.ChangeDetectorConfig{
		Enabled:                true,
		WatchFields:            []string{"status", "available", "price"},
		IgnoreFields:           []string{"updated_at", "crawled_at"},
		SubscriptionAPIURL:     "", // 使用模拟订阅
		SubscriptionAPITimeout: 10,
		CacheTTL:               60,
		Workers:                5,
		QueueSize:              1000,
	}

	// 创建变更检测器
	detector := monitor.NewChangeDetector(
		mongoDatabase,
		redisClient,
		changeDetectorConfig,
		logger,
	)

	// 注册4个爬虫源的解析器
	// 1. Drip Ground Board
	detector.RegisterParser("drip_ground_board", &DripGroundBoardParser{})
	
	// 2. Venue Massage Site
	detector.RegisterParser("venue_massage_site", &VenueMassageSiteParser{})
	
	// 3. Pospal Venue Store 5662377
	detector.RegisterParser("pospal_venue_5662377", &PospalVenue5662377Parser{})
	
	// 4. Pospal Venue Classroom
	detector.RegisterParser("pospal_venue_classroom", &PospalVenueClassroomParser{})

	logger.Info("Registered parsers for all 4 venue sources")

	return &VenueMonitor{
		detector:      detector,
		logger:        logger,
		redisClient:   redisClient,
		mongoDatabase: mongoDatabase,
	}, nil
}

// Start 启动监控服务
func (vm *VenueMonitor) Start() error {
	// 启动变更检测器
	if err := vm.detector.Start(); err != nil {
		return fmt.Errorf("failed to start change detector: %w", err)
	}

	vm.logger.Info("Venue monitor started",
		"monitored_sources", []string{
			"drip_ground_board",
			"venue_massage_site", 
			"pospal_venue_5662377",
			"pospal_venue_classroom",
		},
		"monitored_time_slots", "19:00-22:00",
		"notification_webhook", "DingTalk")

	return nil
}

// Stop 停止监控服务
func (vm *VenueMonitor) Stop() error {
	vm.logger.Info("Stopping venue monitor")
	
	if err := vm.detector.Stop(); err != nil {
		return err
	}

	// 断开连接
	vm.redisClient.Close()
	vm.mongoDatabase.Client().Disconnect(context.Background())
	
	vm.logger.Info("Venue monitor stopped")
	return nil
}

// ProcessTaskResult 处理爬虫任务结果
func (vm *VenueMonitor) ProcessTaskResult(taskID string, source string, result map[string]interface{}) error {
	// 添加source到result中
	result["source"] = source
	
	vm.logger.Info("Processing crawler result",
		"task_id", taskID,
		"source", source)
	
	return vm.detector.ProcessTaskResult(taskID, result)
}

// 各个爬虫源的解析器实现

// DripGroundBoardParser Drip Ground Board解析器
type DripGroundBoardParser struct{}

func (p *DripGroundBoardParser) GetSource() string {
	return "drip_ground_board"
}

func (p *DripGroundBoardParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	
	// 解析Drip Ground Board数据格式
	venues, _ := rawData["venues"].([]interface{})
	now := time.Now()
	
	for _, v := range venues {
		venue, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		
		snapshot := types.VenueSnapshot{
			TaskID:     taskID,
			Source:     p.GetSource(),
			TimeSlot:   getStringField(venue, "time_slot"),
			VenueID:    getStringField(venue, "venue_id"),
			VenueName:  getStringField(venue, "venue_name"),
			Status:     getStringField(venue, "status"),
			Price:      getFloatField(venue, "price"),
			Available:  getIntField(venue, "available"),
			Capacity:   getIntField(venue, "capacity"),
			RawData:    venue,
			CrawledAt:  now,
		}
		
		snapshots = append(snapshots, snapshot)
	}
	
	return snapshots, nil
}

// VenueMassageSiteParser Venue Massage Site解析器
type VenueMassageSiteParser struct{}

func (p *VenueMassageSiteParser) GetSource() string {
	return "venue_massage_site"
}

func (p *VenueMassageSiteParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	
	venues, _ := rawData["venues"].([]interface{})
	now := time.Now()
	
	for _, v := range venues {
		venue, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		
		snapshot := types.VenueSnapshot{
			TaskID:     taskID,
			Source:     p.GetSource(),
			TimeSlot:   getStringField(venue, "time_slot"),
			VenueID:    getStringField(venue, "venue_id"),
			VenueName:  getStringField(venue, "venue_name"),
			Status:     getStringField(venue, "status"),
			Price:      getFloatField(venue, "price"),
			Available:  getIntField(venue, "available"),
			Capacity:   getIntField(venue, "capacity"),
			RawData:    venue,
			CrawledAt:  now,
		}
		
		snapshots = append(snapshots, snapshot)
	}
	
	return snapshots, nil
}

// PospalVenue5662377Parser Pospal Venue Store 5662377解析器
type PospalVenue5662377Parser struct{}

func (p *PospalVenue5662377Parser) GetSource() string {
	return "pospal_venue_5662377"
}

func (p *PospalVenue5662377Parser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	
	venues, _ := rawData["venues"].([]interface{})
	now := time.Now()
	
	for _, v := range venues {
		venue, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		
		snapshot := types.VenueSnapshot{
			TaskID:     taskID,
			Source:     p.GetSource(),
			TimeSlot:   getStringField(venue, "time_slot"),
			VenueID:    getStringField(venue, "venue_id"),
			VenueName:  getStringField(venue, "venue_name"),
			Status:     getStringField(venue, "status"),
			Price:      getFloatField(venue, "price"),
			Available:  getIntField(venue, "available"),
			Capacity:   getIntField(venue, "capacity"),
			RawData:    venue,
			CrawledAt:  now,
		}
		
		snapshots = append(snapshots, snapshot)
	}
	
	return snapshots, nil
}

// PospalVenueClassroomParser Pospal Venue Classroom解析器
type PospalVenueClassroomParser struct{}

func (p *PospalVenueClassroomParser) GetSource() string {
	return "pospal_venue_classroom"
}

func (p *PospalVenueClassroomParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	
	venues, _ := rawData["venues"].([]interface{})
	now := time.Now()
	
	for _, v := range venues {
		venue, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		
		snapshot := types.VenueSnapshot{
			TaskID:     taskID,
			Source:     p.GetSource(),
			TimeSlot:   getStringField(venue, "time_slot"),
			VenueID:    getStringField(venue, "venue_id"),
			VenueName:  getStringField(venue, "venue_name"),
			Status:     getStringField(venue, "status"),
			Price:      getFloatField(venue, "price"),
			Available:  getIntField(venue, "available"),
			Capacity:   getIntField(venue, "capacity"),
			RawData:    venue,
			CrawledAt:  now,
		}
		
		snapshots = append(snapshots, snapshot)
	}
	
	return snapshots, nil
}

// 辅助函数
func getStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloatField(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	if v, ok := m[key].(int); ok {
		return float64(v)
	}
	return 0
}

func getIntField(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	if v, ok := m[key].(int); ok {
		return v
	}
	return 0
}

func main() {
	// 设置日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("Starting venue monitor service",
		"monitored_hours", "19:00-22:00",
		"notification", "DingTalk")

	// 创建监控服务
	monitor, err := NewVenueMonitor(logger)
	if err != nil {
		log.Fatal("Failed to create venue monitor:", err)
	}

	// 启动服务
	if err := monitor.Start(); err != nil {
		log.Fatal("Failed to start venue monitor:", err)
	}

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Venue monitor is running. Press Ctrl+C to stop.")
	logger.Info("Monitoring configuration:",
		"sources", []string{
			"drip_ground_board - Drip Ground Board",
			"venue_massage_site - Venue Massage Site", 
			"pospal_venue_5662377 - Pospal Store 5662377",
			"pospal_venue_classroom - Pospal Classroom",
		},
		"time_slots", "19:00-20:00, 20:00-21:00, 21:00-22:00",
		"notification", "DingTalk webhook configured")

	// 等待退出信号
	<-sigChan

	logger.Info("Shutting down venue monitor...")
	
	// 停止服务
	if err := monitor.Stop(); err != nil {
		logger.Error("Failed to stop venue monitor", "error", err)
	}

	logger.Info("Venue monitor stopped")
}