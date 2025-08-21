package notification

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/types"
	"github.com/NHYCRaymond/go-backend-kit/dingtalk"
)

// NotificationPusher handles push notifications for venue changes
type NotificationPusher struct {
	config   *PusherConfig
	logger   *slog.Logger
	workers  int
	queue    chan *PushTask
	wg       sync.WaitGroup
	stopChan chan struct{}
	clients  map[string]*dingtalk.Client // webhook URL -> client
	mu       sync.RWMutex
}

// PusherConfig contains pusher configuration
type PusherConfig struct {
	Workers    int `json:"workers"`
	QueueSize  int `json:"queue_size"`
	RetryTimes int `json:"retry_times"`
	RetryDelay int `json:"retry_delay"` // seconds
}

// PushTask represents a notification task
type PushTask struct {
	Subscribers []types.Subscriber     `json:"subscribers"`
	Event       types.ChangeEvent      `json:"event"`
	RetryCount  int                    `json:"retry_count"`
	CreatedAt   time.Time              `json:"created_at"`
}

// NewNotificationPusher creates a new notification pusher
func NewNotificationPusher(config *PusherConfig, logger *slog.Logger) *NotificationPusher {
	if config.Workers <= 0 {
		config.Workers = 5
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 1000
	}
	if config.RetryTimes <= 0 {
		config.RetryTimes = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 5
	}

	return &NotificationPusher{
		config:   config,
		logger:   logger,
		workers:  config.Workers,
		queue:    make(chan *PushTask, config.QueueSize),
		stopChan: make(chan struct{}),
		clients:  make(map[string]*dingtalk.Client),
	}
}

// Start starts the notification pusher
func (np *NotificationPusher) Start() error {
	np.logger.Info("Starting notification pusher", "workers", np.workers)

	// Start worker goroutines
	for i := 0; i < np.workers; i++ {
		np.wg.Add(1)
		go np.worker(i)
	}

	np.logger.Info("Notification pusher started")
	return nil
}

// Stop stops the notification pusher
func (np *NotificationPusher) Stop() error {
	np.logger.Info("Stopping notification pusher")

	close(np.stopChan)
	
	// Wait for workers to finish current tasks
	done := make(chan struct{})
	go func() {
		np.wg.Wait()
		close(done)
	}()

	// Wait with timeout
	select {
	case <-done:
		np.logger.Info("All workers stopped gracefully")
	case <-time.After(30 * time.Second):
		np.logger.Warn("Timeout waiting for workers to stop")
	}

	np.logger.Info("Notification pusher stopped")
	return nil
}

// Push adds a notification task to the queue
func (np *NotificationPusher) Push(subscribers []types.Subscriber, event types.ChangeEvent) {
	task := &PushTask{
		Subscribers: subscribers,
		Event:       event,
		CreatedAt:   time.Now(),
	}

	select {
	case np.queue <- task:
		np.logger.Debug("Notification task queued",
			"event_id", event.ID,
			"subscribers", len(subscribers))
	default:
		np.logger.Warn("Notification queue full, dropping task",
			"event_id", event.ID)
	}
}

// worker processes notification tasks
func (np *NotificationPusher) worker(id int) {
	defer np.wg.Done()
	
	np.logger.Info("Notification worker started", "worker_id", id)

	for {
		select {
		case <-np.stopChan:
			np.logger.Info("Notification worker stopping", "worker_id", id)
			return
		case task := <-np.queue:
			if task != nil {
				np.processTask(task)
			}
		}
	}
}

// processTask processes a single notification task
func (np *NotificationPusher) processTask(task *PushTask) {
	// Group subscribers by webhook URL
	webhookGroups := np.groupByWebhook(task.Subscribers)
	
	for webhook, users := range webhookGroups {
		// Get or create DingTalk client for this webhook
		client := np.getOrCreateClient(webhook)
		
		// Build notification message
		message := np.buildMessage(task.Event, users)
		
		// Send notification
		if err := np.sendNotification(client, message, task); err != nil {
			np.logger.Error("Failed to send notification",
				"webhook", webhook,
				"error", err,
				"retry_count", task.RetryCount)
			
			// Retry if needed
			if task.RetryCount < np.config.RetryTimes {
				task.RetryCount++
				time.Sleep(time.Duration(np.config.RetryDelay) * time.Second)
				np.queue <- task
			}
		} else {
			np.logger.Info("Notification sent successfully",
				"webhook", webhook,
				"event_id", task.Event.ID,
				"users", len(users))
		}
	}
}

// groupByWebhook groups subscribers by their webhook URL
func (np *NotificationPusher) groupByWebhook(subscribers []types.Subscriber) map[string][]types.Subscriber {
	groups := make(map[string][]types.Subscriber)
	
	for _, sub := range subscribers {
		groups[sub.WebhookURL] = append(groups[sub.WebhookURL], sub)
	}
	
	return groups
}

// getOrCreateClient gets or creates a DingTalk client for the webhook
func (np *NotificationPusher) getOrCreateClient(webhook string) *dingtalk.Client {
	np.mu.RLock()
	client, exists := np.clients[webhook]
	np.mu.RUnlock()
	
	if exists {
		return client
	}
	
	np.mu.Lock()
	defer np.mu.Unlock()
	
	// Double-check after acquiring write lock
	if client, exists = np.clients[webhook]; exists {
		return client
	}
	
	// Extract access token from webhook URL
	accessToken := extractAccessToken(webhook)
	
	// Create new client
	client = dingtalk.NewClient(dingtalk.ClientOption{
		AccessToken: accessToken,
		Timeout:     10 * time.Second,
		Keywords:    []string{}, // 不自动添加关键词，由消息内容控制
	})
	
	np.clients[webhook] = client
	return client
}

// buildMessage builds a DingTalk message for the change event
func (np *NotificationPusher) buildMessage(event types.ChangeEvent, users []types.Subscriber) string {
	// Build markdown message
	var message string
	
	// Check if this is an initial state notification or a change notification
	if event.IsInitial {
		// 初始状态通知
		message = fmt.Sprintf("## 🎾 球球雷达 - 场地状态通知\n\n")
		message += fmt.Sprintf("**场地来源:** %s\n", event.Source)
		message += fmt.Sprintf("**时段:** %s\n", event.TimeSlot)
		message += fmt.Sprintf("**场地:** %s\n", event.VenueName)
		message += fmt.Sprintf("**时间:** %s\n\n", event.Timestamp.Format("2006-01-02 15:04:05"))
		
		// Current state
		statusEmoji := "🔄"
		if event.NewStatus == "available" {
			statusEmoji = "✅"
		} else if event.NewStatus == "full" {
			statusEmoji = "❌"
		} else if event.NewStatus == "closed" {
			statusEmoji = "🚫"
		}
		
		message += fmt.Sprintf("%s **当前状态:** %s\n", statusEmoji, event.NewStatus)
		message += fmt.Sprintf("📊 **可用数量:** %d\n", event.NewAvailable)
		if event.NewPrice > 0 {
			message += fmt.Sprintf("💰 **当前价格:** ¥%.2f\n", event.NewPrice)
		}
		
		message += "\n📌 *这是该场地在该时段的初始状态通知*\n"
	} else {
		// 变更通知
		message = fmt.Sprintf("## 🎾 球球雷达 - 场地状态变更通知\n\n")
		message += fmt.Sprintf("**场地来源:** %s\n", event.Source)
		message += fmt.Sprintf("**时段:** %s\n", event.TimeSlot)
		message += fmt.Sprintf("**场地:** %s\n", event.VenueName)
		message += fmt.Sprintf("**变更时间:** %s\n\n", event.Timestamp.Format("2006-01-02 15:04:05"))
		
		// Status change
		if event.OldStatus != event.NewStatus {
			statusEmoji := "🔄"
			if event.NewStatus == "available" {
				statusEmoji = "✅"
			} else if event.NewStatus == "full" {
				statusEmoji = "❌"
			}
			message += fmt.Sprintf("%s **状态变更:** %s → %s\n", statusEmoji, event.OldStatus, event.NewStatus)
		}
		
		// Available count change
		if event.OldAvailable != event.NewAvailable {
			emoji := "📉"
			if event.NewAvailable > event.OldAvailable {
				emoji = "📈"
			}
			message += fmt.Sprintf("%s **可用数量:** %d → %d\n", emoji, event.OldAvailable, event.NewAvailable)
		}
		
		// Price change
		if event.OldPrice != event.NewPrice {
			emoji := "💰"
			if event.NewPrice < event.OldPrice {
				emoji = "💸"
			}
			message += fmt.Sprintf("%s **价格变化:** ¥%.2f → ¥%.2f\n", emoji, event.OldPrice, event.NewPrice)
		}
	}
	
	// Add user mentions if any
	if len(users) > 0 {
		message += "\n---\n"
		message += "**订阅用户:** "
		for i, user := range users {
			if i > 0 {
				message += ", "
			}
			message += user.UserName
		}
		message += "\n"
	}
	
	// Add timestamp
	message += fmt.Sprintf("\n*推送时间: %s*", time.Now().Format("15:04:05"))
	
	return message
}

// sendNotification sends the notification via DingTalk
func (np *NotificationPusher) sendNotification(client *dingtalk.Client, message string, task *PushTask) error {
	// Create markdown message
	err := client.SendMarkdown(
		"场地变更通知",
		message,
		nil, // No @ mentions for now
	)
	
	if err != nil {
		return fmt.Errorf("failed to send DingTalk message: %w", err)
	}
	
	// Log successful push
	np.logPushSuccess(task)
	
	return nil
}

// logPushSuccess logs successful push notification
func (np *NotificationPusher) logPushSuccess(task *PushTask) {
	pushLog := map[string]interface{}{
		"event_id":   task.Event.ID,
		"source":     task.Event.Source,
		"time_slot":  task.Event.TimeSlot,
		"venue_name": task.Event.VenueName,
		"users":      len(task.Subscribers),
		"pushed_at":  time.Now(),
		"latency":    time.Since(task.CreatedAt).Milliseconds(),
	}
	
	logData, _ := json.Marshal(pushLog)
	np.logger.Info("Push notification sent", "push_log", string(logData))
}

// extractAccessToken extracts access token from webhook URL
func extractAccessToken(webhook string) string {
	// Parse URL to extract access_token parameter
	// Example: https://oapi.dingtalk.com/robot/send?access_token=xxx
	const prefix = "access_token="
	
	for i := len(webhook) - 1; i >= 0; i-- {
		if webhook[i] == '=' && i >= len(prefix)-1 {
			if webhook[i-len(prefix)+1:i+1] == prefix {
				return webhook[i+1:]
			}
		}
	}
	
	// Fallback: assume the token is after the last '='
	for i := len(webhook) - 1; i >= 0; i-- {
		if webhook[i] == '=' {
			return webhook[i+1:]
		}
	}
	
	return ""
}

// GetQueueSize returns the current queue size
func (np *NotificationPusher) GetQueueSize() int {
	return len(np.queue)
}

// GetStats returns pusher statistics
func (np *NotificationPusher) GetStats() map[string]interface{} {
	np.mu.RLock()
	defer np.mu.RUnlock()
	
	return map[string]interface{}{
		"queue_size":    len(np.queue),
		"max_queue":     np.config.QueueSize,
		"workers":       np.workers,
		"clients":       len(np.clients),
		"retry_times":   np.config.RetryTimes,
		"retry_delay":   np.config.RetryDelay,
	}
}