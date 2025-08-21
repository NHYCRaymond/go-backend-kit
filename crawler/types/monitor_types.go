package types

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// VenueSnapshot represents a snapshot of venue state at a specific time
type VenueSnapshot struct {
	ID          primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	TaskID      string                 `bson:"task_id" json:"task_id"`           // 爬虫任务ID
	Source      string                 `bson:"source" json:"source"`             // 场地来源
	TimeSlot    string                 `bson:"time_slot" json:"time_slot"`       // 时段
	VenueID     string                 `bson:"venue_id" json:"venue_id"`         // 场地标识
	VenueName   string                 `bson:"venue_name" json:"venue_name"`     // 场地名称
	Status      string                 `bson:"status" json:"status"`             // 状态(available/full/closed)
	Price       float64                `bson:"price" json:"price"`               // 价格
	Capacity    int                    `bson:"capacity" json:"capacity"`         // 容量
	Available   int                    `bson:"available" json:"available"`       // 可用数量
	StateMD5    string                 `bson:"state_md5" json:"state_md5"`       // 状态字段的MD5
	RawData     map[string]interface{} `bson:"raw_data" json:"raw_data"`         // 原始数据
	CrawledAt   time.Time              `bson:"crawled_at" json:"crawled_at"`     // 爬取时间
	CreatedAt   time.Time              `bson:"created_at" json:"created_at"`
}

// VenueState represents the key state fields for comparison
type VenueState struct {
	Status    string  `json:"status"`
	Available int     `json:"available"`
	Price     float64 `json:"price"`
}

// ChangeEvent represents a venue state change event
type ChangeEvent struct {
	ID           string                 `json:"id"`
	Source       string                 `json:"source"`        // 场地来源
	TimeSlot     string                 `json:"time_slot"`     // 时段
	VenueID      string                 `json:"venue_id"`      // 场地ID
	VenueName    string                 `json:"venue_name"`    // 场地名称
	OldStatus    string                 `json:"old_status"`    // 旧状态
	NewStatus    string                 `json:"new_status"`    // 新状态
	OldAvailable int                    `json:"old_available"` // 旧可用数量
	NewAvailable int                    `json:"new_available"` // 新可用数量
	OldPrice     float64                `json:"old_price"`     // 旧价格
	NewPrice     float64                `json:"new_price"`     // 新价格
	Timestamp    time.Time              `json:"timestamp"`     // 变更时间
	Details      map[string]interface{} `json:"details"`       // 详细信息
	IsInitial    bool                   `json:"is_initial"`    // 是否为初始状态推送
}

// Subscriber represents a subscription user
type Subscriber struct {
	UserID     string `json:"user_id"`
	WebhookURL string `json:"webhook_url"`
	UserName   string `json:"user_name,omitempty"`
}

// PushTask represents a push notification task
type PushTask struct {
	ID        string       `json:"id"`
	Webhook   string       `json:"webhook"`
	Users     []Subscriber `json:"users"`
	Event     ChangeEvent  `json:"event"`
	CreatedAt time.Time    `json:"created_at"`
	Retries   int          `json:"retries"`
}

// PushLog represents a push notification log
type PushLog struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TaskID    string             `bson:"task_id" json:"task_id"`
	EventID   string             `bson:"event_id" json:"event_id"`
	Webhook   string             `bson:"webhook" json:"webhook"`
	Users     []string           `bson:"users" json:"users"`
	Source    string             `bson:"source" json:"source"`
	TimeSlot  string             `bson:"time_slot" json:"time_slot"`
	Success   bool               `bson:"success" json:"success"`
	Error     string             `bson:"error,omitempty" json:"error,omitempty"`
	PushedAt  time.Time          `bson:"pushed_at" json:"pushed_at"`
	Retries   int                `bson:"retries" json:"retries"`
}

// SubscriptionQuery represents a query to external subscription service
type SubscriptionQuery struct {
	Source   string `json:"source"`
	TimeSlot string `json:"time_slot"`
}

// SubscriptionResponse represents response from subscription service
type SubscriptionResponse struct {
	Success     bool         `json:"success"`
	Subscribers []Subscriber `json:"subscribers"`
	Count       int          `json:"count"`
}

// VenueParseResult represents parsed venue data from crawler result
type VenueParseResult struct {
	Source    string          `json:"source"`
	Venues    []VenueSnapshot `json:"venues"`
	ParsedAt  time.Time       `json:"parsed_at"`
}

// ChangeDetectorConfig represents configuration for change detector
type ChangeDetectorConfig struct {
	Enabled                bool     `mapstructure:"enabled"`
	WatchFields            []string `mapstructure:"watch_fields"`
	IgnoreFields           []string `mapstructure:"ignore_fields"`
	SubscriptionAPIURL     string   `mapstructure:"subscription_api_url"`
	SubscriptionAPITimeout int      `mapstructure:"subscription_api_timeout"` // seconds
	CacheTTL               int      `mapstructure:"cache_ttl"`                // seconds
	Workers                int      `mapstructure:"workers"`
	QueueSize              int      `mapstructure:"queue_size"`
}

// NotificationConfig represents notification configuration
type NotificationConfig struct {
	Type       string `mapstructure:"type"`        // dingtalk, webhook, etc
	Workers    int    `mapstructure:"workers"`
	QueueSize  int    `mapstructure:"queue_size"`
	RetryTimes int    `mapstructure:"retry_times"`
	RetryDelay int    `mapstructure:"retry_delay"` // seconds
}