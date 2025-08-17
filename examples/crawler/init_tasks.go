package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	var (
		mongoURI = flag.String("mongo", "mongodb://localhost:27017", "MongoDB URI")
		action   = flag.String("action", "list", "Action: list, init, clear")
	)
	flag.Parse()

	// 连接MongoDB
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatalf("连接MongoDB失败: %v", err)
	}
	defer client.Disconnect(ctx)

	// 测试连接
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("MongoDB ping失败: %v", err)
	}

	db := client.Database("crawler")
	service := task.NewService(db)

	switch *action {
	case "list":
		listTasks(service)
	case "init":
		initSampleTasks(service)
	case "clear":
		clearTasks(service)
	default:
		log.Fatalf("未知操作: %s", *action)
	}
}

// listTasks 列出所有任务
func listTasks(service *task.Service) {
	tasks, err := service.ListTasks(task.TaskFilter{})
	if err != nil {
		log.Fatalf("列出任务失败: %v", err)
	}

	fmt.Printf("找到 %d 个任务:\n", len(tasks))
	for _, t := range tasks {
		status := "禁用"
		if t.Status.Enabled {
			status = "启用"
		}
		fmt.Printf("- %s (%s) - %s - %s\n", t.Name, t.Type, t.Category, status)
	}
}

// initSampleTasks 初始化示例任务
func initSampleTasks(service *task.Service) {
	fmt.Println("初始化示例任务...")

	// 滴球场地任务
	dripTask := &task.TaskDocument{
		Name:        "drip_ground_board",
		Type:        "api",
		Category:    "sports",
		Description: "滴球场地预订数据爬取",
		Version:     "1.0.0",
		Status: task.TaskStatus{
			Enabled: true,
		},
		Config: task.TaskConfiguration{
			Priority:    5,
			Timeout:     30,
			MaxRetries:  3,
			Concurrency: 1,
			RateLimit: task.RateLimitConfig{
				Enabled:        true,
				RequestsPerMin: 12,
			},
		},
		Schedule: task.ScheduleConfig{
			Type:       "interval",
			Expression: "30s",
			Timezone:   "Asia/Shanghai",
		},
		Request: task.RequestTemplate{
			Method: "POST",
			URL:    "https://zd.drip.im/api-zd-wap/booking/order/getGroundBoard",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"User-Agent":   "Mozilla/5.0",
			},
			QueryParams: map[string]interface{}{
				"special": "13249191",
			},
			Body: map[string]interface{}{
				"groupId": 1330,
				"date":    "${DATE}",
			},
		},
		Extraction: task.ExtractionConfig{
			Type: "json",
			Rules: []task.ExtractionRule{
				{
					Field:    "group_id",
					Path:     "result.groupId",
					Type:     "integer",
					Required: true,
				},
				{
					Field:    "date",
					Path:     "result.date",
					Type:     "string",
					Required: true,
				},
				{
					Field:    "grounds",
					Path:     "result.groundDatas",
					Type:     "array",
					Required: true,
				},
			},
		},
		Storage: task.StorageConfiguration{
			Targets: []task.StorageTarget{
				{
					Name:    "primary",
					Type:    "mongodb",
					Purpose: "primary",
					Config: map[string]interface{}{
						"database":   "crawler",
						"collection": "court_booking_boards",
						"upsert":     true,
					},
				},
			},
		},
		Tags:      []string{"production", "venue", "booking"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		CreatedBy: "system",
		UpdatedBy: "system",
	}

	// 网球教练任务
	coachTask := &task.TaskDocument{
		Name:        "tennis_coach",
		Type:        "http",
		Category:    "sports",
		Description: "网球教练信息爬取",
		Version:     "1.0.0",
		Status: task.TaskStatus{
			Enabled: false,
		},
		Config: task.TaskConfiguration{
			Priority:    3,
			Timeout:     30,
			MaxRetries:  3,
			Concurrency: 5,
			RateLimit: task.RateLimitConfig{
				Enabled:        true,
				RequestsPerMin: 60,
			},
		},
		Schedule: task.ScheduleConfig{
			Type:       "cron",
			Expression: "0 2 * * *",
			Timezone:   "Asia/Shanghai",
		},
		Request: task.RequestTemplate{
			Method: "GET",
			URL:    "https://example.com/api/coaches",
			Headers: map[string]string{
				"Accept":     "application/json",
				"User-Agent": "QiuQiuRadar/1.0",
			},
		},
		Extraction: task.ExtractionConfig{
			Type: "json",
			Rules: []task.ExtractionRule{
				{
					Field:    "coach_id",
					Path:     "$.data[*].id",
					Type:     "string",
					Required: true,
				},
				{
					Field:    "name",
					Path:     "$.data[*].name",
					Type:     "string",
					Required: true,
				},
			},
		},
		Storage: task.StorageConfiguration{
			Targets: []task.StorageTarget{
				{
					Name:    "primary",
					Type:    "mongodb",
					Purpose: "primary",
					Config: map[string]interface{}{
						"database":   "crawler",
						"collection": "coaches",
						"upsert":     true,
					},
				},
			},
		},
		Tags:      []string{"coach", "tennis"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		CreatedBy: "system",
		UpdatedBy: "system",
	}

	// GitHub Trending任务
	githubTask := &task.TaskDocument{
		Name:        "github_trending",
		Type:        "http",
		Category:    "tech",
		Description: "GitHub热门项目爬取",
		Version:     "1.0.0",
		Status: task.TaskStatus{
			Enabled: true,
		},
		Config: task.TaskConfiguration{
			Priority:    2,
			Timeout:     20,
			MaxRetries:  2,
			Concurrency: 3,
		},
		Schedule: task.ScheduleConfig{
			Type:       "cron",
			Expression: "0 */6 * * *",
			Timezone:   "UTC",
		},
		Request: task.RequestTemplate{
			Method: "GET",
			URL:    "https://api.github.com/search/repositories",
			Headers: map[string]string{
				"Accept": "application/vnd.github.v3+json",
			},
			QueryParams: map[string]interface{}{
				"q":        "stars:>1000",
				"sort":     "stars",
				"order":    "desc",
				"per_page": 30,
			},
		},
		Storage: task.StorageConfiguration{
			Targets: []task.StorageTarget{
				{
					Name:    "primary",
					Type:    "mongodb",
					Purpose: "primary",
					Config: map[string]interface{}{
						"database":   "crawler",
						"collection": "github_trending",
					},
				},
			},
		},
		Tags:      []string{"github", "trending", "opensource"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		CreatedBy: "system",
		UpdatedBy: "system",
	}

	// 创建任务
	tasks := []*task.TaskDocument{dripTask, coachTask, githubTask}
	
	for _, t := range tasks {
		created, err := service.CreateTask(t)
		if err != nil {
			log.Printf("创建任务 %s 失败: %v", t.Name, err)
		} else {
			fmt.Printf("✓ 创建任务: %s (ID: %s)\n", created.Name, created.ID.Hex())
		}
	}

	fmt.Println("\n初始化完成！")
}

// clearTasks 清空所有任务
func clearTasks(service *task.Service) {
	tasks, err := service.ListTasks(task.TaskFilter{})
	if err != nil {
		log.Fatalf("列出任务失败: %v", err)
	}

	fmt.Printf("准备删除 %d 个任务...\n", len(tasks))
	
	for _, t := range tasks {
		if err := service.DeleteTask(t.ID.Hex()); err != nil {
			log.Printf("删除任务 %s 失败: %v", t.Name, err)
		} else {
			fmt.Printf("✓ 删除任务: %s\n", t.Name)
		}
	}

	fmt.Println("清空完成！")
}