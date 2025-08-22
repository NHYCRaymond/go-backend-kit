package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/common"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-redis/redis/v8"
	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Scheduler manages task scheduling from MongoDB to Redis queue
type Scheduler struct {
	mongodb *mongo.Database
	redis   *redis.Client
	cron    *cron.Cron
	logger  *slog.Logger

	// Task tracking
	scheduledTasks map[string]cron.EntryID
	mu             sync.RWMutex

	// Configuration
	queuePrefix string
	stopChan    chan struct{}
}

// Config holds scheduler configuration
type Config struct {
	MongoDB     *mongo.Database
	Redis       *redis.Client
	QueuePrefix string
	Logger      *slog.Logger
}

// NewScheduler creates a new task scheduler
func NewScheduler(config *Config) *Scheduler {
	if config.QueuePrefix == "" {
		config.QueuePrefix = "crawler"
	}

	return &Scheduler{
		mongodb:        config.MongoDB,
		redis:          config.Redis,
		cron:           cron.New(cron.WithSeconds()),
		logger:         config.Logger,
		queuePrefix:    config.QueuePrefix,
		scheduledTasks: make(map[string]cron.EntryID),
		stopChan:       make(chan struct{}),
	}
}

// Start begins the scheduler
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("Starting task scheduler")

	// Load all enabled tasks from MongoDB
	if err := s.loadTasks(ctx); err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Start cron scheduler
	s.cron.Start()

	// Start task watcher for changes
	go s.watchTasks(ctx)

	s.logger.Info("Task scheduler started", "scheduled_tasks", len(s.scheduledTasks))
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.logger.Info("Stopping task scheduler")
	close(s.stopChan)

	ctx := s.cron.Stop()
	<-ctx.Done()

	s.logger.Info("Task scheduler stopped")
}

// loadTasks loads all enabled tasks from MongoDB
func (s *Scheduler) loadTasks(ctx context.Context) error {
	collection := s.mongodb.Collection(common.DefaultTasksCollection)

	// Find all enabled tasks
	filter := bson.M{"status.enabled": true}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var doc task.TaskDocument
		if err := cursor.Decode(&doc); err != nil {
			s.logger.Error("Failed to decode task", "error", err)
			continue
		}

		if err := s.scheduleTask(&doc); err != nil {
			s.logger.Error("Failed to schedule task",
				"task_id", doc.ID.Hex(),
				"task_name", doc.Name,
				"error", err)
		}
	}

	return cursor.Err()
}

// scheduleTask schedules a single task
func (s *Scheduler) scheduleTask(doc *task.TaskDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID := doc.ID.Hex()

	// Remove existing schedule if any
	if entryID, exists := s.scheduledTasks[taskID]; exists {
		s.cron.Remove(entryID)
		delete(s.scheduledTasks, taskID)
	}

	// Schedule based on type
	switch doc.Schedule.Type {
	case "manual":
		// Manual tasks are not scheduled
		s.logger.Debug("Skipping manual task", "task_name", doc.Name)
		return nil

	case "once":
		// Schedule one-time execution
		if !doc.Status.NextRun.IsZero() && doc.Status.NextRun.After(time.Now()) {
			delay := time.Until(doc.Status.NextRun)
			time.AfterFunc(delay, func() {
				s.submitTask(doc)
			})
			s.logger.Info("Scheduled one-time task",
				"task_name", doc.Name,
				"run_at", doc.Status.NextRun)
		}

	case "interval":
		// Parse interval (e.g., "5m", "1h", "30s")
		duration, err := time.ParseDuration(doc.Schedule.Expression)
		if err != nil {
			return fmt.Errorf("invalid interval: %w", err)
		}

		// Schedule periodic execution that checks if task should run
		entryID, err := s.cron.AddFunc(fmt.Sprintf("@every %s", duration), func() {
			// Reload the task document to get latest next_run
			ctx := context.Background()
			collection := s.mongodb.Collection(common.DefaultTasksCollection)
			
			var currentDoc task.TaskDocument
			if err := collection.FindOne(ctx, bson.M{"_id": doc.ID}).Decode(&currentDoc); err != nil {
				s.logger.Error("Failed to reload task document", "task_id", doc.ID.Hex(), "error", err)
				return
			}
			
			// Check if it's time to run based on next_run
			now := time.Now()
			if currentDoc.Status.NextRun.IsZero() || currentDoc.Status.NextRun.After(now) {
				s.logger.Debug("Skipping task, not due yet",
					"task_name", currentDoc.Name,
					"next_run", currentDoc.Status.NextRun,
					"now", now)
				return
			}
			
			s.submitTask(&currentDoc)
		})
		if err != nil {
			return err
		}
		s.scheduledTasks[taskID] = entryID

		s.logger.Info("Scheduled interval task",
			"task_name", doc.Name,
			"interval", duration)

	case "cron":
		// Schedule with cron expression
		entryID, err := s.cron.AddFunc(doc.Schedule.Expression, func() {
			s.submitTask(doc)
		})
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		s.scheduledTasks[taskID] = entryID

		s.logger.Info("Scheduled cron task",
			"task_name", doc.Name,
			"cron", doc.Schedule.Expression)

	default:
		return fmt.Errorf("unknown schedule type: %s", doc.Schedule.Type)
	}

	return nil
}

// submitTask converts TaskDocument to executable task and submits to Redis queue
func (s *Scheduler) submitTask(doc *task.TaskDocument) {
	ctx := context.Background()
	
	s.logger.Info("submitTask called", 
		"task_name", doc.Name,
		"task_id", doc.ID.Hex())

	// Check for date variables and their strategies
	hasDateVariable := false
	var dateStrategy *task.DateStrategy
	
	// Check variables for date type with strategy
	for _, v := range doc.Request.Variables {
		s.logger.Debug("Checking variable",
			"name", v.Name,
			"type", v.Type,
			"has_strategy", v.Strategy != nil)
		if v.Type == "date" && v.Name == "DATE" {
			hasDateVariable = true
			dateStrategy = v.Strategy
			s.logger.Info("Found DATE variable with strategy",
				"task_name", doc.Name,
				"strategy_mode", dateStrategy.Mode)
			break
		}
	}
	
	// If no explicit date variable, check body for ${DATE} (backward compatibility)
	if !hasDateVariable {
		bodyStr := ""
		if doc.Request.Body != nil {
			if bodyBytes, err := json.Marshal(doc.Request.Body); err == nil {
				bodyStr = string(bodyBytes)
			}
		}
		hasDateVariable = strings.Contains(bodyStr, "${DATE}")
	}
	
	if hasDateVariable {
		s.logger.Info("Submitting tasks with date strategy",
			"task_name", doc.Name,
			"has_strategy", dateStrategy != nil)
		s.submitTasksWithDateStrategy(doc, dateStrategy)
	} else {
		// No date variable, submit as single task
		s.logger.Info("Submitting single task (no date variable)",
			"task_name", doc.Name)
		execTask := s.convertToTask(doc)
		s.submitSingleTask(ctx, execTask, doc)
	}
}

// submitTasksWithDateStrategy submits tasks based on date strategy
func (s *Scheduler) submitTasksWithDateStrategy(doc *task.TaskDocument, strategy *task.DateStrategy) {
	var dates []time.Time
	now := time.Now()
	
	if strategy == nil {
		// Default strategy: next 7 days
		s.logger.Info("Using default date strategy (7 days)",
			"task_name", doc.Name)
		for i := 0; i < 7; i++ {
			dates = append(dates, now.AddDate(0, 0, i))
		}
	} else {
		switch strategy.Mode {
		case "single":
			// Single date based on StartDays offset
			dates = append(dates, now.AddDate(0, 0, strategy.StartDays))
			s.logger.Info("Using single date strategy",
				"task_name", doc.Name,
				"offset_days", strategy.StartDays)
			
		case "range":
			// Date range from StartDays to EndDays with DayStep
			step := strategy.DayStep
			if step <= 0 {
				step = 1
			}
			for i := strategy.StartDays; i <= strategy.EndDays; i += step {
				dates = append(dates, now.AddDate(0, 0, i))
			}
			s.logger.Info("Using range date strategy",
				"task_name", doc.Name,
				"start_days", strategy.StartDays,
				"end_days", strategy.EndDays,
				"step", step)
			
		case "custom":
			// Custom list of day offsets
			for _, dayOffset := range strategy.CustomDays {
				dates = append(dates, now.AddDate(0, 0, dayOffset))
			}
			s.logger.Info("Using custom date strategy",
				"task_name", doc.Name,
				"custom_days", strategy.CustomDays)
			
		default:
			// Default to single current day
			dates = append(dates, now)
			s.logger.Warn("Unknown date strategy mode, using today",
				"task_name", doc.Name,
				"mode", strategy.Mode)
		}
	}
	
	// Create batch for multiple dates
	if len(dates) > 1 {
		// Generate batch ID for this group of tasks
		batchID := fmt.Sprintf("%s_batch_%d", doc.ID.Hex(), time.Now().UnixNano())
		batchType := "scheduled_batch"
		if doc.Name != "" {
			// Use task name as batch type for better identification
			batchType = strings.ReplaceAll(strings.ToLower(doc.Name), " ", "_")
		}
		
		s.logger.Info("Creating batch for date strategy tasks",
			"batch_id", batchID,
			"batch_size", len(dates),
			"batch_type", batchType,
			"task_name", doc.Name)
		
		// Submit tasks as a batch
		for i, date := range dates {
			s.submitTaskForDateWithBatch(doc, date, batchID, batchType, len(dates), i)
		}
	} else if len(dates) == 1 {
		// Single date, no batch needed
		s.submitTaskForDate(doc, dates[0])
	}
}

// submitTaskForDate creates and submits a task for a specific date
func (s *Scheduler) submitTaskForDate(doc *task.TaskDocument, targetDate time.Time) {
	ctx := context.Background()
	
	// Convert TaskDocument to executable Task with specific date
	execTask := s.convertToTask(doc, targetDate)
	
	s.logger.Info("Task converted for date",
		"task_id", execTask.ID,
		"date", targetDate.Format("2006-01-02"),
		"url", execTask.URL,
		"body_preview", string(execTask.Body))
	
	s.submitSingleTask(ctx, execTask, doc)
}

// submitTaskForDateWithBatch creates and submits a task as part of a batch
func (s *Scheduler) submitTaskForDateWithBatch(doc *task.TaskDocument, targetDate time.Time, batchID string, batchType string, batchSize int, batchIndex int) {
	ctx := context.Background()
	
	// Convert TaskDocument to executable Task with specific date
	execTask := s.convertToTask(doc, targetDate)
	
	// Add batch information
	execTask.BatchID = batchID
	execTask.BatchType = batchType
	execTask.BatchSize = batchSize
	execTask.BatchIndex = batchIndex
	
	s.logger.Info("Task converted for date with batch",
		"task_id", execTask.ID,
		"date", targetDate.Format("2006-01-02"),
		"batch_id", batchID,
		"batch_type", batchType,
		"batch_index", batchIndex,
		"batch_size", batchSize,
		"url", execTask.URL)
	
	s.submitSingleTask(ctx, execTask, doc)
}

// submitSingleTask submits a single task to the queue
func (s *Scheduler) submitSingleTask(ctx context.Context, execTask *task.Task, doc *task.TaskDocument) {
	// Serialize task
	data, err := json.Marshal(execTask)
	if err != nil {
		s.logger.Error("Failed to marshal task", "error", err)
		return
	}

	// Store task data in Redis for Coordinator to retrieve
	taskKey := fmt.Sprintf("%s:task:%s", s.queuePrefix, execTask.ID)
	if err := s.redis.Set(ctx, taskKey, data, 24*time.Hour).Err(); err != nil {
		s.logger.Error("Failed to store task data",
			"task_id", execTask.ID,
			"error", err)
		return
	}
	
	// Push full task data to Redis queue - must match Coordinator's expected queue name
	queueKey := fmt.Sprintf("%s:queue:tasks:pending", s.queuePrefix)
	if err := s.redis.LPush(ctx, queueKey, data).Err(); err != nil {
		s.logger.Error("Failed to submit task to queue",
			"task_id", execTask.ID,
			"queue", queueKey,
			"error", err)
		return
	}

	// Update task status in MongoDB (only for the first task)
	if !strings.Contains(execTask.ID, "_date_") || strings.Contains(execTask.ID, "_date_0") {
		s.updateTaskStatus(ctx, doc.ID, "submitted")
	}

	s.logger.Info("Task submitted to queue",
		"task_id", execTask.ID,
		"task_name", doc.Name,
		"url", doc.Request.URL)
}

// convertToTask converts TaskDocument to executable Task with optional date
func (s *Scheduler) convertToTask(doc *task.TaskDocument, options ...interface{}) *task.Task {
	// Parse options - for now just support a target date
	var targetDate time.Time
	var hasDate bool
	
	for _, opt := range options {
		if date, ok := opt.(time.Time); ok {
			targetDate = date
			hasDate = true
			break
		}
	}
	
	// Default to current time if no date provided
	if !hasDate {
		targetDate = time.Now()
	}
	
	// Generate unique task instance ID
	var instanceID string
	if hasDate {
		// Include date in ID for date-specific tasks
		instanceID = fmt.Sprintf("%s_date_%s_%d", doc.ID.Hex(), targetDate.Format("20060102"), time.Now().UnixNano())
	} else {
		// Simple ID for non-date tasks
		instanceID = fmt.Sprintf("%s_%d", doc.ID.Hex(), time.Now().UnixNano())
	}
	
	// Build URL with query parameters if present
	url := doc.Request.URL
	if doc.Request.QueryParams != nil && len(doc.Request.QueryParams) > 0 {
		params := make([]string, 0, len(doc.Request.QueryParams))
		for key, value := range doc.Request.QueryParams {
			params = append(params, fmt.Sprintf("%s=%v", key, value))
		}
		if len(params) > 0 {
			separator := "?"
			if strings.Contains(url, "?") {
				separator = "&"
			}
			url = url + separator + strings.Join(params, "&")
		}
	}
	
	// Process request body with date replacement if needed
	var body []byte
	if doc.Request.Body != nil {
		body = s.processRequestBody(doc.Request.Body, targetDate)
	}
	
	// Generate task name
	taskName := doc.Name
	if hasDate {
		taskName = fmt.Sprintf("%s_%s", doc.Name, targetDate.Format("20060102"))
	}
	
	// Create the task
	t := &task.Task{
		ID:       instanceID,
		ParentID: doc.ID.Hex(),
		Name:     taskName,
		Type:     doc.Type,
		Priority: doc.Config.Priority,

		// Request details
		URL:     url,
		Method:  doc.Request.Method,
		Headers: doc.Request.Headers,
		Cookies: doc.Request.Cookies,
		Body:    body,

		// Configuration
		Timeout:    time.Duration(doc.Config.Timeout) * time.Second,
		MaxRetries: doc.Config.MaxRetries,
		RetryDelay: time.Duration(doc.Config.RetryDelay) * time.Second,

		// Metadata
		CreatedAt: time.Now(),
		Status:    task.StatusPending,
		Source:    doc.Category, // Set source from task category

		// Storage configuration
		StorageConf: s.buildStorageConfig(doc.Storage),
		
		// Lua script configuration
		ProjectID: doc.ProjectID,
		LuaScript: doc.LuaScript,
	}

	// Add extract rules from extraction config
	if doc.Extraction.Rules != nil && len(doc.Extraction.Rules) > 0 {
		t.ExtractRules = make([]task.ExtractRule, len(doc.Extraction.Rules))
		for i, ext := range doc.Extraction.Rules {
			// Use extraction type from parent, not rule type (which is data type)
			selectorType := doc.Extraction.Type
			if selectorType == "" {
				selectorType = "css" // Default
			}
			
			t.ExtractRules[i] = task.ExtractRule{
				Field:     ext.Field,
				Selector:  ext.Path,
				Type:      selectorType,
				Required:  ext.Required,
				Default:   ext.Default,
			}
		}
	}

	return t
}

// calculateNextRun calculates the next run time based on schedule configuration
func (s *Scheduler) calculateNextRun(schedule task.ScheduleConfig) time.Time {
	now := time.Now()
	
	switch schedule.Type {
	case "interval":
		// Parse interval duration (e.g., "30s", "5m", "1h")
		duration, err := time.ParseDuration(schedule.Expression)
		if err != nil {
			s.logger.Error("Failed to parse interval duration", 
				"expression", schedule.Expression, 
				"error", err)
			// Default to 5 minutes if parsing fails
			return now.Add(5 * time.Minute)
		}
		return now.Add(duration)
		
	case "cron":
		// For cron expressions, we would need a cron parser to calculate next run
		// For now, just add 1 hour as a placeholder
		// TODO: Use robfig/cron to properly calculate next execution time
		return now.Add(1 * time.Hour)
		
	case "once":
		// One-time tasks don't have a next run
		return time.Time{}
		
	default:
		// Unknown schedule type, no next run
		return time.Time{}
	}
}

// updateTaskStatus updates task status in MongoDB
func (s *Scheduler) updateTaskStatus(ctx context.Context, taskID primitive.ObjectID, status string) {
	collection := s.mongodb.Collection(common.DefaultTasksCollection)

	// Get task document to retrieve schedule information
	var doc task.TaskDocument
	if err := collection.FindOne(ctx, bson.M{"_id": taskID}).Decode(&doc); err != nil {
		s.logger.Error("Failed to find task", "task_id", taskID.Hex(), "error", err)
		return
	}

	now := time.Now()
	// Calculate next_run based on schedule type
	nextRun := s.calculateNextRun(doc.Schedule)

	update := bson.M{
		"$set": bson.M{
			"status.last_run": now,
			"status.next_run": nextRun,
			"updated_at":      now,
		},
		"$inc": bson.M{
			"status.run_count": 1,
		},
	}

	if _, err := collection.UpdateByID(ctx, taskID, update); err != nil {
		s.logger.Error("Failed to update task status",
			"task_id", taskID.Hex(),
			"error", err)
	} else {
		s.logger.Info("Updated task status",
			"task_id", taskID.Hex(),
			"status", status,
			"last_run", now.Format(time.RFC3339),
			"next_run", nextRun.Format(time.RFC3339))
	}
}

// UpdateTaskResult updates task execution result in MongoDB
func (s *Scheduler) UpdateTaskResult(ctx context.Context, taskInstanceID string, success bool, errorMsg string) {
	// Extract parent task ID from instance ID (format: parentID_timestamp)
	parts := strings.Split(taskInstanceID, "_")
	if len(parts) < 2 {
		s.logger.Error("Invalid task instance ID", "instance_id", taskInstanceID)
		return
	}
	
	parentID := parts[0]
	objID, err := primitive.ObjectIDFromHex(parentID)
	if err != nil {
		s.logger.Error("Invalid parent task ID", "parent_id", parentID, "error", err)
		return
	}
	
	collection := s.mongodb.Collection(common.DefaultTasksCollection)
	
	// Get task document to retrieve schedule information for next_run calculation
	var doc task.TaskDocument
	if err := collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&doc); err != nil {
		s.logger.Error("Failed to find task", "task_id", objID.Hex(), "error", err)
		return
	}
	
	// Calculate next_run based on schedule type
	nextRun := s.calculateNextRun(doc.Schedule)
	
	update := bson.M{
		"$set": bson.M{
			"status.last_run": time.Now(),
			"status.next_run": nextRun,
			"updated_at":      time.Now(),
		},
	}
	
	if success {
		update["$inc"] = bson.M{
			"status.success_count": 1,
		}
		update["$set"].(bson.M)["status.last_success"] = time.Now()
	} else {
		update["$inc"] = bson.M{
			"status.failure_count": 1,
		}
		update["$set"].(bson.M)["status.last_failure"] = time.Now()
		if errorMsg != "" {
			update["$set"].(bson.M)["status.error_message"] = errorMsg
		}
	}
	
	if _, err := collection.UpdateByID(ctx, objID, update); err != nil {
		s.logger.Error("Failed to update task result",
			"task_id", objID.Hex(),
			"success", success,
			"error", err)
	} else {
		s.logger.Info("Updated task result",
			"task_id", objID.Hex(),
			"success", success)
	}
}

// processRequestBody converts interface{} body to []byte with date variable replacement
func (s *Scheduler) processRequestBody(body interface{}, targetDate time.Time) []byte {
	if body == nil {
		return nil
	}
	
	// Log the raw body type for debugging
	s.logger.Debug("processRequestBody called",
		"body_type", fmt.Sprintf("%T", body),
		"target_date", targetDate.Format("2006-01-02"))
	
	// First, handle the case where body is already serialized as JSON array of Key-Value pairs
	// This happens when MongoDB stores it as primitive.D
	if bodyBytes, err := json.Marshal(body); err == nil {
		bodyStr := string(bodyBytes)
		s.logger.Debug("Marshaled body to JSON", "json", bodyStr)
		
		// Check if it's in the [{"Key":"...","Value":"..."}] format
		if len(bodyStr) > 0 && bodyStr[0] == '[' {
			var kvPairs []struct {
				Key   string      `json:"Key"`
				Value interface{} `json:"Value"`
			}
			if err := json.Unmarshal(bodyBytes, &kvPairs); err == nil && len(kvPairs) > 0 {
				s.logger.Debug("Detected primitive.D format", "pairs", kvPairs)
				// Convert to normal map
				bodyMap := make(map[string]interface{})
				for _, kv := range kvPairs {
					// Replace ${DATE} variable with target date
					if str, ok := kv.Value.(string); ok && str == "${DATE}" {
						bodyMap[kv.Key] = targetDate.Format("2006-01-02")
						s.logger.Debug("Replaced ${DATE} variable", "key", kv.Key, "value", targetDate.Format("2006-01-02"))
					} else {
						bodyMap[kv.Key] = kv.Value
					}
				}
				// Return as JSON
				result, _ := json.Marshal(bodyMap)
				s.logger.Debug("Converted primitive.D to map with date", "date", targetDate.Format("2006-01-02"))
				return result
			}
		}
	}
	
	// Handle other types
	var bodyMap map[string]interface{}
	
	switch v := body.(type) {
	case primitive.D:
		// This shouldn't happen now, but keep it as fallback
		bodyMap = make(map[string]interface{})
		for _, elem := range v {
			bodyMap[elem.Key] = elem.Value
		}
	case primitive.M:
		bodyMap = make(map[string]interface{})
		for key, val := range v {
			bodyMap[key] = val
		}
	case map[string]interface{}:
		bodyMap = v
	case []byte:
		// Try to unmarshal and process
		var temp interface{}
		if err := json.Unmarshal(v, &temp); err == nil {
			if m, ok := temp.(map[string]interface{}); ok {
				bodyMap = m
			} else {
				return v // Return as is if not a map
			}
		} else {
			return v // Return as is if not JSON
		}
	case string:
		// Try to unmarshal and process
		var temp interface{}
		if err := json.Unmarshal([]byte(v), &temp); err == nil {
			if m, ok := temp.(map[string]interface{}); ok {
				bodyMap = m
			} else {
				return []byte(v) // Return as is if not a map
			}
		} else {
			return []byte(v) // Return as is if not JSON
		}
	default:
		// Try to convert to JSON first to see what we have
		data, _ := json.Marshal(body)
		var temp interface{}
		if err := json.Unmarshal(data, &temp); err == nil {
			if m, ok := temp.(map[string]interface{}); ok {
				bodyMap = m
			} else {
				return data // Return as is if not a map
			}
		} else {
			return data // Return as is if not JSON
		}
	}
	
	// Process variables in the map with target date
	if bodyMap != nil {
		// Replace ${DATE} with target date
		for key, val := range bodyMap {
			if str, ok := val.(string); ok && str == "${DATE}" {
				bodyMap[key] = targetDate.Format("2006-01-02")
			}
		}
		// Marshal the processed map to JSON
		data, _ := json.Marshal(bodyMap)
		return data
	}
	
	return nil
}


// buildStorageConfig builds storage config from StorageConfiguration
func (s *Scheduler) buildStorageConfig(storage task.StorageConfiguration) task.StorageConfig {
	config := task.StorageConfig{
		Options: make(map[string]interface{}),
	}
	
	// Use first target if available
	if len(storage.Targets) > 0 {
		target := storage.Targets[0]
		config.Type = target.Type
		
		// Extract common config fields
		if target.Config != nil {
			if db, ok := target.Config["database"].(string); ok {
				config.Database = db
			}
			if coll, ok := target.Config["collection"].(string); ok {
				config.Collection = coll
			}
			if table, ok := target.Config["table"].(string); ok {
				config.Table = table
			}
			
			// Copy all config as options
			for k, v := range target.Config {
				config.Options[k] = v
			}
		}
	}
	
	return config
}

// watchTasks watches for task changes in MongoDB
func (s *Scheduler) watchTasks(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second) // Check less frequently
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			// Check for tasks that should run now but might have been missed
			s.checkOverdueTasks(ctx)
		}
	}
}

// checkOverdueTasks checks for tasks that are overdue and should run
func (s *Scheduler) checkOverdueTasks(ctx context.Context) {
	collection := s.mongodb.Collection(common.DefaultTasksCollection)
	
	// Find enabled tasks where next_run is in the past
	now := time.Now()
	filter := bson.M{
		"status.enabled": true,
		"status.next_run": bson.M{
			"$lte": now,
			"$ne": time.Time{}, // Not zero time
		},
	}
	
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		s.logger.Error("Failed to find overdue tasks", "error", err)
		return
	}
	defer cursor.Close(ctx)
	
	for cursor.Next(ctx) {
		var doc task.TaskDocument
		if err := cursor.Decode(&doc); err != nil {
			s.logger.Error("Failed to decode overdue task", "error", err)
			continue
		}
		
		// Submit the overdue task
		s.logger.Info("Submitting overdue task",
			"task_name", doc.Name,
			"next_run", doc.Status.NextRun,
			"now", now)
		s.submitTask(&doc)
	}
}

// RunTaskNow immediately runs a task (called from TUI)
func (s *Scheduler) RunTaskNow(ctx context.Context, taskID string) error {
	collection := s.mongodb.Collection(common.DefaultTasksCollection)

	// Find the task
	objID, err := primitive.ObjectIDFromHex(taskID)
	if err != nil {
		return fmt.Errorf("invalid task ID: %w", err)
	}

	var doc task.TaskDocument
	if err := collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&doc); err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// Submit task immediately
	s.submitTask(&doc)

	return nil
}
