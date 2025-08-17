package task

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Priority defines task priority levels
type Priority int

const (
	PriorityLow    Priority = 0
	PriorityNormal Priority = 1
	PriorityHigh   Priority = 2
	PriorityUrgent Priority = 3
)

// Status defines task status
type Status string

const (
	StatusPending   Status = "pending"
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusRetrying  Status = "retrying"
	StatusCancelled Status = "cancelled"
)

// Type defines task types
type Type string

const (
	TypeSeed      Type = "seed"      // Seed URL that generates more tasks
	TypeDetail    Type = "detail"    // Detail page crawling
	TypeList      Type = "list"      // List page crawling
	TypeAPI       Type = "api"       // API endpoint crawling
	TypeBrowser   Type = "browser"   // Browser-based crawling (JS rendering)
	TypeAggregate Type = "aggregate" // Aggregate multiple sources
)

// Task represents a crawler task
type Task struct {
	// Identity
	ID       string   `json:"id"`
	ParentID string   `json:"parent_id,omitempty"` // MongoDB task ID
	Name     string   `json:"name,omitempty"`      // Task name from MongoDB
	URL      string   `json:"url"`
	Hash     string   `json:"hash"` // URL hash for deduplication
	Type     string   `json:"type"` // Changed to string for flexibility
	Priority int      `json:"priority"`
	Status   Status   `json:"status"`
	NodeID   string   `json:"node_id,omitempty"` // Assigned node

	// Retry management
	RetryCount int           `json:"retry_count"`
	MaxRetries int           `json:"max_retries"`
	RetryDelay time.Duration `json:"retry_delay"`
	LastError  string        `json:"last_error,omitempty"`

	// Timing
	CreatedAt   time.Time  `json:"created_at"`
	QueuedAt    *time.Time `json:"queued_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`

	// Configuration
	Method         string            `json:"method"` // GET, POST, etc.
	Headers        map[string]string `json:"headers,omitempty"`
	Body           []byte            `json:"body,omitempty"`
	Cookies        map[string]string `json:"cookies,omitempty"`
	Proxy          string            `json:"proxy,omitempty"`
	UserAgent      string            `json:"user_agent,omitempty"`
	Timeout        time.Duration     `json:"timeout"`
	FollowRedirect bool              `json:"follow_redirect"`

	// Data extraction
	ExtractRules []ExtractRule `json:"extract_rules,omitempty"`
	Pipeline     Pipeline      `json:"-"`                   // Pipeline interface (not serialized)
	PipelineID   string        `json:"pipeline_id,omitempty"` // Pipeline identifier for reconstruction
	Storage      Storage       `json:"-"`                   // Storage interface (not serialized)
	StorageConf  StorageConfig `json:"storage_config,omitempty"` // Storage configuration

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Tags     []string               `json:"tags,omitempty"`
	Depth    int                    `json:"depth"`            // Crawl depth from seed
	Source   string                 `json:"source,omitempty"` // Source identifier

	// Callbacks
	OnSuccess  string `json:"on_success,omitempty"`  // Webhook or queue for success
	OnFailure  string `json:"on_failure,omitempty"`  // Webhook or queue for failure
	OnComplete string `json:"on_complete,omitempty"` // Webhook or queue for completion
}

// ExtractRule defines data extraction rules
type ExtractRule struct {
	Field     string          `json:"field"`
	Selector  string          `json:"selector"`
	Type      string          `json:"type"` // css, xpath, json, regex
	Attribute string          `json:"attribute,omitempty"`
	Multiple  bool            `json:"multiple"`
	Required  bool            `json:"required"`
	Default   interface{}     `json:"default,omitempty"`
	Transform []TransformRule `json:"transform,omitempty"`
	Children  []ExtractRule   `json:"children,omitempty"` // For nested extraction
}

// TransformRule defines data transformation
type TransformRule struct {
	Type   string                 `json:"type"` // trim, replace, regex, format, split, join
	Params map[string]interface{} `json:"params,omitempty"`
}

// NewTask creates a new task
func NewTask(url string, taskType string, priority int) *Task {
	now := time.Now()
	task := &Task{
		ID:             uuid.New().String(),
		URL:            url,
		Hash:           hashURL(url),
		Type:           taskType,
		Priority:       priority,
		Status:         StatusPending,
		CreatedAt:      now,
		Method:         "GET",
		MaxRetries:     3,
		RetryDelay:     5 * time.Second,
		Timeout:        30 * time.Second,
		FollowRedirect: true,
		Metadata:       make(map[string]interface{}),
		Tags:           []string{},
	}
	return task
}

// NewSeedTask creates a seed task that generates more tasks
func NewSeedTask(url string, extractRules []ExtractRule) *Task {
	task := NewTask(url, string(TypeSeed), int(PriorityHigh))
	task.ExtractRules = extractRules
	return task
}

// GetID returns task ID
func (t *Task) GetID() string {
	return t.ID
}

// GetURL returns task URL
func (t *Task) GetURL() string {
	return t.URL
}

// GetPriority returns task priority
func (t *Task) GetPriority() int {
	return int(t.Priority)
}

// GetRetryCount returns retry count
func (t *Task) GetRetryCount() int {
	return t.RetryCount
}

// IncrementRetry increments retry count
func (t *Task) IncrementRetry() {
	t.RetryCount++
	t.Status = StatusRetrying
}

// GetCreatedAt returns creation time
func (t *Task) GetCreatedAt() time.Time {
	return t.CreatedAt
}

// GetMetadata returns all metadata
func (t *Task) GetMetadata() map[string]interface{} {
	return t.Metadata
}

// SetMetadata sets a metadata value
func (t *Task) SetMetadata(key string, value interface{}) {
	if t.Metadata == nil {
		t.Metadata = make(map[string]interface{})
	}
	t.Metadata[key] = value
}

// Clone creates a copy of the task
func (t *Task) Clone() *Task {
	cloned := &Task{}
	data, _ := json.Marshal(t)
	json.Unmarshal(data, cloned)
	cloned.ID = uuid.New().String()
	cloned.Status = StatusPending
	cloned.RetryCount = 0
	cloned.NodeID = ""
	cloned.QueuedAt = nil
	cloned.StartedAt = nil
	cloned.CompletedAt = nil
	return cloned
}

// IsExpired checks if task has expired
func (t *Task) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*t.ExpiresAt)
}

// CanRetry checks if task can be retried
func (t *Task) CanRetry() bool {
	return t.RetryCount < t.MaxRetries && !t.IsExpired()
}

// SetError sets error message and increments retry if possible
func (t *Task) SetError(err error) {
	t.LastError = err.Error()
	if t.CanRetry() {
		t.IncrementRetry()
	} else {
		t.Status = StatusFailed
	}
}

// Start marks task as started
func (t *Task) Start() {
	now := time.Now()
	t.StartedAt = &now
	t.Status = StatusRunning
}

// Complete marks task as completed
func (t *Task) Complete() {
	now := time.Now()
	t.CompletedAt = &now
	t.Status = StatusCompleted
}

// Fail marks task as failed
func (t *Task) Fail(err error) {
	now := time.Now()
	t.CompletedAt = &now
	t.SetError(err)
}

// Queue marks task as queued
func (t *Task) Queue() {
	now := time.Now()
	t.QueuedAt = &now
	t.Status = StatusQueued
}

// Cancel marks task as cancelled
func (t *Task) Cancel() {
	t.Status = StatusCancelled
}

// Duration returns task execution duration
func (t *Task) Duration() time.Duration {
	if t.StartedAt == nil {
		return 0
	}
	if t.CompletedAt != nil {
		return t.CompletedAt.Sub(*t.StartedAt)
	}
	return time.Since(*t.StartedAt)
}

// WaitTime returns time spent waiting in queue
func (t *Task) WaitTime() time.Duration {
	if t.QueuedAt == nil {
		return 0
	}
	if t.StartedAt != nil {
		return t.StartedAt.Sub(*t.QueuedAt)
	}
	return time.Since(*t.QueuedAt)
}

// AddTag adds a tag to the task
func (t *Task) AddTag(tag string) {
	for _, existing := range t.Tags {
		if existing == tag {
			return
		}
	}
	t.Tags = append(t.Tags, tag)
}

// HasTag checks if task has a specific tag
func (t *Task) HasTag(tag string) bool {
	for _, existing := range t.Tags {
		if existing == tag {
			return true
		}
	}
	return false
}

// SetHeaders sets request headers
func (t *Task) SetHeaders(headers map[string]string) {
	t.Headers = headers
}

// SetHeader sets a single header
func (t *Task) SetHeader(key, value string) {
	if t.Headers == nil {
		t.Headers = make(map[string]string)
	}
	t.Headers[key] = value
}

// SetCookies sets request cookies
func (t *Task) SetCookies(cookies map[string]string) {
	t.Cookies = cookies
}

// SetCookie sets a single cookie
func (t *Task) SetCookie(key, value string) {
	if t.Cookies == nil {
		t.Cookies = make(map[string]string)
	}
	t.Cookies[key] = value
}

// ToJSON converts task to JSON
func (t *Task) ToJSON() ([]byte, error) {
	return json.Marshal(t)
}

// FromJSON creates task from JSON
func FromJSON(data []byte) (*Task, error) {
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// Validate validates task configuration
func (t *Task) Validate() error {
	if t.URL == "" {
		return fmt.Errorf("task URL is required")
	}
	if t.Type == "" {
		return fmt.Errorf("task type is required")
	}
	if t.Method == "" {
		t.Method = "GET"
	}
	if t.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}
	if t.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	return nil
}

// hashURL generates a hash for URL deduplication
func hashURL(url string) string {
	h := md5.New()
	h.Write([]byte(url))
	return hex.EncodeToString(h.Sum(nil))
}

// Batch represents a batch of tasks
type Batch struct {
	ID        string    `json:"id"`
	Tasks     []*Task   `json:"tasks"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
}

// NewTaskBatch creates a new task batch
func NewTaskBatch(tasks []*Task, source string) *Batch {
	return &Batch{
		ID:        uuid.New().String(),
		Tasks:     tasks,
		CreatedAt: time.Now(),
		Source:    source,
	}
}

// Size returns batch size
func (tb *Batch) Size() int {
	return len(tb.Tasks)
}

// Filter filters tasks by predicate
func (tb *Batch) Filter(predicate func(*Task) bool) []*Task {
	var filtered []*Task
	for _, task := range tb.Tasks {
		if predicate(task) {
			filtered = append(filtered, task)
		}
	}
	return filtered
}
