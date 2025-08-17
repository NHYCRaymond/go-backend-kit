package task

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Service 任务服务 - 提供任务管理的完整功能
type Service struct {
	db         *mongo.Database
	collection *mongo.Collection
	ctx        context.Context
}

// NewService 创建任务服务
func NewService(db *mongo.Database) *Service {
	collection := db.Collection("crawler_tasks")
	
	// 创建索引
	ctx := context.Background()
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "name", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "category", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "status.enabled", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "schedule.type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "tags", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}
	
	collection.Indexes().CreateMany(ctx, indexes)
	
	return &Service{
		db:         db,
		collection: collection,
		ctx:        ctx,
	}
}

// CreateTask 创建任务
func (s *Service) CreateTask(task *TaskDocument) (*TaskDocument, error) {
	task.ID = primitive.NewObjectID()
	task.CreatedAt = time.Now()
	task.UpdatedAt = time.Now()
	
	// 初始化状态
	if task.Status.Enabled {
		task.Status.NextRun = s.calculateNextRun(task.Schedule)
	}
	
	_, err := s.collection.InsertOne(s.ctx, task)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	
	return task, nil
}

// GetTask 获取单个任务
func (s *Service) GetTask(id string) (*TaskDocument, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid task id: %w", err)
	}
	
	var task TaskDocument
	err = s.collection.FindOne(s.ctx, bson.M{"_id": objectID}).Decode(&task)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("task not found")
		}
		return nil, fmt.Errorf("find task: %w", err)
	}
	
	return &task, nil
}

// ListTasks 列出任务
func (s *Service) ListTasks(filter TaskFilter) ([]*TaskDocument, error) {
	query := bson.M{}
	
	// 构建查询条件
	if filter.Type != "" {
		query["type"] = filter.Type
	}
	if filter.Category != "" {
		query["category"] = filter.Category
	}
	if filter.Enabled != nil {
		query["status.enabled"] = *filter.Enabled
	}
	if len(filter.Tags) > 0 {
		query["tags"] = bson.M{"$in": filter.Tags}
	}
	
	// 查询选项
	opts := options.Find()
	if filter.SortBy != "" {
		order := 1
		if filter.SortDesc {
			order = -1
		}
		opts.SetSort(bson.D{{Key: filter.SortBy, Value: order}})
	}
	if filter.Limit > 0 {
		opts.SetLimit(int64(filter.Limit))
	}
	if filter.Offset > 0 {
		opts.SetSkip(int64(filter.Offset))
	}
	
	cursor, err := s.collection.Find(s.ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("find tasks: %w", err)
	}
	defer cursor.Close(s.ctx)
	
	var tasks []*TaskDocument
	if err := cursor.All(s.ctx, &tasks); err != nil {
		return nil, fmt.Errorf("decode tasks: %w", err)
	}
	
	return tasks, nil
}

// UpdateTask 更新任务
func (s *Service) UpdateTask(id string, update bson.M) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid task id: %w", err)
	}
	
	update["updated_at"] = time.Now()
	
	_, err = s.collection.UpdateByID(s.ctx, objectID, bson.M{"$set": update})
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	
	return nil
}

// DeleteTask 删除任务
func (s *Service) DeleteTask(id string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid task id: %w", err)
	}
	
	result, err := s.collection.DeleteOne(s.ctx, bson.M{"_id": objectID})
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	
	if result.DeletedCount == 0 {
		return fmt.Errorf("task not found")
	}
	
	return nil
}

// EnableTask 启用任务
func (s *Service) EnableTask(id string) error {
	return s.UpdateTask(id, bson.M{
		"status.enabled": true,
		"status.next_run": time.Now(),
	})
}

// DisableTask 禁用任务
func (s *Service) DisableTask(id string) error {
	return s.UpdateTask(id, bson.M{
		"status.enabled": false,
	})
}

// UpdateTaskRaw 使用原始更新文档更新任务
func (s *Service) UpdateTaskRaw(id primitive.ObjectID, update interface{}) (*mongo.UpdateResult, error) {
	return s.collection.UpdateByID(s.ctx, id, update)
}

// GetReadyTasks 获取待执行的任务
func (s *Service) GetReadyTasks() ([]*TaskDocument, error) {
	now := time.Now()
	query := bson.M{
		"status.enabled": true,
		"status.next_run": bson.M{"$lte": now},
	}
	
	cursor, err := s.collection.Find(s.ctx, query)
	if err != nil {
		return nil, fmt.Errorf("find ready tasks: %w", err)
	}
	defer cursor.Close(s.ctx)
	
	var tasks []*TaskDocument
	if err := cursor.All(s.ctx, &tasks); err != nil {
		return nil, fmt.Errorf("decode tasks: %w", err)
	}
	
	return tasks, nil
}

// calculateNextRun 计算下次运行时间
func (s *Service) calculateNextRun(schedule ScheduleConfig) time.Time {
	switch schedule.Type {
	case "interval":
		return time.Now().Add(time.Duration(30) * time.Second)
	case "cron":
		return time.Now().Add(time.Hour)
	case "once":
		if schedule.StartTime != nil {
			return *schedule.StartTime
		}
		return time.Now()
	default:
		return time.Time{}
	}
}

// TaskFilter 任务过滤器
type TaskFilter struct {
	Type     string
	Category string
	Enabled  *bool
	Tags     []string
	SortBy   string
	SortDesc bool
	Limit    int
	Offset   int
}