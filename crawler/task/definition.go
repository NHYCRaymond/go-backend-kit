package task

import (
	"log/slog"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/errors"
	"github.com/go-redis/redis/v8"
)

// TaskDef defines a type of crawler task (legacy)
type TaskDef struct {
	// Basic info
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Category    string `json:"category"` // seed, page, api, browser

	// Task type
	Type     Type     `json:"type"`
	Priority Priority `json:"priority"`

	// Request configuration
	Method         string            `json:"method"`
	Headers        map[string]string `json:"headers"`
	DefaultTimeout time.Duration     `json:"default_timeout"`
	MaxRetries     int               `json:"max_retries"`
	RetryInterval  time.Duration     `json:"retry_interval"`

	// Extraction rules
	ExtractRules []ExtractRule `json:"extract_rules"`
	LinkRules    []LinkRule    `json:"link_rules"` // For generating child tasks

	// Processing
	Pipeline      string                 `json:"pipeline"`       // Pipeline configuration name
	Processors    []string               `json:"processors"`     // Processor names
	Storage       string                 `json:"storage"`        // Storage configuration name
	Deduplication DeduplicationConfig    `json:"deduplication"`

	// Validation
	Validator TaskValidator `json:"-"`
	Schema    interface{}   `json:"schema"` // JSON schema for validation

	// Hooks
	Hooks TaskHooks `json:"-"`

	// Tags and metadata
	Tags     []string               `json:"tags"`
	Labels   map[string]string      `json:"labels"`
	Metadata map[string]interface{} `json:"metadata"`

	// Status
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LinkRule defines rules for extracting and generating child tasks
type LinkRule struct {
	Name      string        `json:"name"`
	Selector  string        `json:"selector"`
	Type      string        `json:"type"` // css, xpath, regex, json
	Attribute string        `json:"attribute"`
	Pattern   string        `json:"pattern"`   // URL pattern to match
	TaskType  Type          `json:"task_type"` // Type for generated tasks
	Priority  Priority      `json:"priority"`
	Depth     int           `json:"depth"`      // Max depth for this rule
	Metadata  interface{}   `json:"metadata"`   // Metadata to add to generated tasks
}

// DeduplicationConfig defines deduplication settings
type DeduplicationConfig struct {
	Enabled    bool     `json:"enabled"`
	Strategy   string   `json:"strategy"` // url, content, custom
	Fields     []string `json:"fields"`   // Fields to use for deduplication
	Expiration time.Duration `json:"expiration"`
}

// TaskValidator validates task data
type TaskValidator interface {
	Validate(task *Task) error
	ValidateData(data map[string]interface{}) error
}

// TaskHooks defines lifecycle hooks
type TaskHooks struct {
	BeforeExecute  func(ctx context.Context, task *Task) error
	AfterExecute   func(ctx context.Context, task *Task, result interface{}) error
	OnSuccess      func(ctx context.Context, task *Task, data interface{}) error
	OnFailure      func(ctx context.Context, task *Task, err error) error
	OnRetry        func(ctx context.Context, task *Task, attempt int) error
}

// TaskTemplate is a template for creating tasks
type TaskTemplate struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Definition  string                 `json:"definition"` // Definition ID
	
	// URL generation
	URLPattern  string                 `json:"url_pattern"`
	URLParams   map[string]interface{} `json:"url_params"`
	
	// Overrides
	Headers     map[string]string      `json:"headers"`
	Metadata    map[string]interface{} `json:"metadata"`
	Priority    *Priority              `json:"priority"`
	MaxRetries  *int                   `json:"max_retries"`
	
	// Schedule
	Schedule    string                 `json:"schedule"` // Cron expression
	Enabled     bool                   `json:"enabled"`
	
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// CreateTask creates a task from this definition
func (td *TaskDef) CreateTask(url string, metadata map[string]interface{}) (*Task, error) {
	task := NewTask(url, string(td.Type), int(td.Priority))
	
	// Apply definition settings
	task.Method = td.Method
	task.Headers = make(map[string]string)
	for k, v := range td.Headers {
		task.Headers[k] = v
	}
	task.Timeout = td.DefaultTimeout
	task.MaxRetries = td.MaxRetries
	// task.RetryInterval = td.RetryInterval // Field doesn't exist in Task
	task.ExtractRules = td.ExtractRules
	task.PipelineID = td.Pipeline // Pipeline is a string ID
	// Storage config should be set separately
	
	// Merge metadata
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	task.Metadata["definition_id"] = td.ID
	task.Metadata["definition_name"] = td.Name
	for k, v := range td.Metadata {
		task.Metadata[k] = v
	}
	for k, v := range metadata {
		task.Metadata[k] = v
	}
	
	// Copy tags
	task.Tags = append(task.Tags, td.Tags...)
	
	return task, nil
}

// Validate validates the task definition
func (td *TaskDef) Validate() error {
	if td.ID == "" {
		return errors.New(errors.ValidationErrorCode, "definition ID is required")
	}
	if td.Name == "" {
		return errors.New(errors.ValidationErrorCode, "definition name is required")
	}
	if td.Type == "" {
		return errors.New(errors.ValidationErrorCode, "task type is required")
	}
	if td.Method == "" {
		td.Method = "GET"
	}
	if td.DefaultTimeout <= 0 {
		td.DefaultTimeout = 30 * time.Second
	}
	if td.MaxRetries < 0 {
		return errors.New(errors.ValidationErrorCode, "max retries cannot be negative")
	}
	return nil
}

// GenerateChildTasks generates child tasks based on link rules
func (td *TaskDef) GenerateChildTasks(parentTask *Task, extractedData []map[string]interface{}) ([]*Task, error) {
	var childTasks []*Task
	
	for _, rule := range td.LinkRules {
		// Check depth limit
		if parentTask.Depth >= rule.Depth && rule.Depth > 0 {
			continue
		}
		
		// Extract URLs based on rule
		urls := extractURLsFromData(extractedData, rule)
		
		for _, url := range urls {
			// Create child task
			childTask := NewTask(url, string(rule.TaskType), int(rule.Priority))
			childTask.ParentID = parentTask.ID
			childTask.Depth = parentTask.Depth + 1
			
			// Copy parent metadata
			childTask.Metadata = make(map[string]interface{})
			for k, v := range parentTask.Metadata {
				childTask.Metadata[k] = v
			}
			
			// Add rule metadata
			if rule.Metadata != nil {
				if m, ok := rule.Metadata.(map[string]interface{}); ok {
					for k, v := range m {
						childTask.Metadata[k] = v
					}
				}
			}
			
			childTask.Metadata["link_rule"] = rule.Name
			childTask.Metadata["parent_task"] = parentTask.ID
			
			childTasks = append(childTasks, childTask)
		}
	}
	
	return childTasks, nil
}

// extractURLsFromData extracts URLs from data based on link rule
func extractURLsFromData(data []map[string]interface{}, rule LinkRule) []string {
	var urls []string
	
	// This is a simplified implementation
	// In production, implement proper extraction based on rule type
	for _, item := range data {
		if url, ok := item[rule.Name].(string); ok {
			urls = append(urls, url)
		}
	}
	
	return urls
}

// TaskRegistry manages task definitions and templates
type TaskRegistry struct {
	mu          sync.RWMutex
	definitions map[string]*TaskDef
	templates   map[string]*TaskTemplate
	validators  map[string]TaskValidator
	redis       *redis.Client
	prefix      string
	logger      *slog.Logger
}

// NewTaskRegistry creates a new task registry
func NewTaskRegistry(redisClient *redis.Client, prefix string, logger *slog.Logger) *TaskRegistry {
	return &TaskRegistry{
		definitions: make(map[string]*TaskDef),
		templates:   make(map[string]*TaskTemplate),
		validators:  make(map[string]TaskValidator),
		redis:       redisClient,
		prefix:      prefix,
		logger:      logger,
	}
}

// RegisterDefinition registers a task definition
func (tr *TaskRegistry) RegisterDefinition(def *TaskDef) error {
	if err := def.Validate(); err != nil {
		return fmt.Errorf("invalid definition: %w", err)
	}
	
	tr.mu.Lock()
	defer tr.mu.Unlock()
	
	// Check for duplicate
	if _, exists := tr.definitions[def.ID]; exists {
		return errors.New(errors.AlreadyExistsErrorCode, fmt.Sprintf("definition %s already exists", def.ID))
	}
	
	// Store in memory
	tr.definitions[def.ID] = def
	
	// Store in Redis for persistence
	ctx := context.Background()
	key := fmt.Sprintf("%s:definition:%s", tr.prefix, def.ID)
	data, _ := json.Marshal(def)
	
	if err := tr.redis.Set(ctx, key, data, 0).Err(); err != nil {
		tr.logger.Error("Failed to store definition in Redis", "error", err)
	}
	
	// Register in index
	indexKey := fmt.Sprintf("%s:definitions", tr.prefix)
	tr.redis.SAdd(ctx, indexKey, def.ID)
	
	tr.logger.Info("Task definition registered",
		"id", def.ID,
		"name", def.Name,
		"type", def.Type)
	
	return nil
}

// GetDefinition retrieves a task definition
func (tr *TaskRegistry) GetDefinition(id string) (*TaskDef, error) {
	tr.mu.RLock()
	def, exists := tr.definitions[id]
	tr.mu.RUnlock()
	
	if exists {
		return def, nil
	}
	
	// Try to load from Redis
	ctx := context.Background()
	key := fmt.Sprintf("%s:definition:%s", tr.prefix, id)
	data, err := tr.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, errors.New(errors.NotFoundErrorCode, fmt.Sprintf("definition %s not found", id))
	}
	
	def = &TaskDef{}
	if err := json.Unmarshal([]byte(data), def); err != nil {
		return nil, err
	}
	
	// Cache in memory
	tr.mu.Lock()
	tr.definitions[id] = def
	tr.mu.Unlock()
	
	return def, nil
}

// ListDefinitions lists all task definitions
func (tr *TaskRegistry) ListDefinitions() ([]*TaskDef, error) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	
	defs := make([]*TaskDef, 0, len(tr.definitions))
	for _, def := range tr.definitions {
		defs = append(defs, def)
	}
	
	return defs, nil
}

// RegisterTemplate registers a task template
func (tr *TaskRegistry) RegisterTemplate(tmpl *TaskTemplate) error {
	if tmpl.ID == "" {
		return errors.New(errors.ValidationErrorCode, "template ID is required")
	}
	
	// Verify definition exists
	if _, err := tr.GetDefinition(tmpl.Definition); err != nil {
		return fmt.Errorf("definition not found: %w", err)
	}
	
	tr.mu.Lock()
	defer tr.mu.Unlock()
	
	// Store in memory
	tr.templates[tmpl.ID] = tmpl
	
	// Store in Redis
	ctx := context.Background()
	key := fmt.Sprintf("%s:template:%s", tr.prefix, tmpl.ID)
	data, _ := json.Marshal(tmpl)
	
	if err := tr.redis.Set(ctx, key, data, 0).Err(); err != nil {
		tr.logger.Error("Failed to store template in Redis", "error", err)
	}
	
	// Register in index
	indexKey := fmt.Sprintf("%s:templates", tr.prefix)
	tr.redis.SAdd(ctx, indexKey, tmpl.ID)
	
	tr.logger.Info("Task template registered",
		"id", tmpl.ID,
		"name", tmpl.Name,
		"definition", tmpl.Definition)
	
	return nil
}

// GetTemplate retrieves a task template
func (tr *TaskRegistry) GetTemplate(id string) (*TaskTemplate, error) {
	tr.mu.RLock()
	tmpl, exists := tr.templates[id]
	tr.mu.RUnlock()
	
	if exists {
		return tmpl, nil
	}
	
	// Try to load from Redis
	ctx := context.Background()
	key := fmt.Sprintf("%s:template:%s", tr.prefix, id)
	data, err := tr.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, errors.New(errors.NotFoundErrorCode, fmt.Sprintf("template %s not found", id))
	}
	
	tmpl = &TaskTemplate{}
	if err := json.Unmarshal([]byte(data), tmpl); err != nil {
		return nil, err
	}
	
	// Cache in memory
	tr.mu.Lock()
	tr.templates[id] = tmpl
	tr.mu.Unlock()
	
	return tmpl, nil
}

// CreateTaskFromTemplate creates a task from a template
func (tr *TaskRegistry) CreateTaskFromTemplate(templateID string, params map[string]interface{}) (*Task, error) {
	tmpl, err := tr.GetTemplate(templateID)
	if err != nil {
		return nil, err
	}
	
	def, err := tr.GetDefinition(tmpl.Definition)
	if err != nil {
		return nil, err
	}
	
	// Generate URL from pattern and params
	url := tmpl.URLPattern
	for k, v := range tmpl.URLParams {
		url = strings.ReplaceAll(url, fmt.Sprintf("{%s}", k), fmt.Sprint(v))
	}
	for k, v := range params {
		url = strings.ReplaceAll(url, fmt.Sprintf("{%s}", k), fmt.Sprint(v))
	}
	
	// Create task from definition
	task, err := def.CreateTask(url, tmpl.Metadata)
	if err != nil {
		return nil, err
	}
	
	// Apply template overrides
	if tmpl.Headers != nil {
		for k, v := range tmpl.Headers {
			task.Headers[k] = v
		}
	}
	if tmpl.Priority != nil {
		task.Priority = int(*tmpl.Priority)
	}
	if tmpl.MaxRetries != nil {
		task.MaxRetries = *tmpl.MaxRetries
	}
	
	task.Metadata["template_id"] = tmpl.ID
	task.Metadata["template_name"] = tmpl.Name
	
	return task, nil
}

// RegisterValidator registers a task validator
func (tr *TaskRegistry) RegisterValidator(name string, validator TaskValidator) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.validators[name] = validator
}

// GetValidator gets a task validator
func (tr *TaskRegistry) GetValidator(name string) (TaskValidator, error) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	
	validator, exists := tr.validators[name]
	if !exists {
		return nil, errors.New(errors.NotFoundErrorCode, fmt.Sprintf("validator %s not found", name))
	}
	
	return validator, nil
}

// LoadDefinitionsFromRedis loads all definitions from Redis
func (tr *TaskRegistry) LoadDefinitionsFromRedis(ctx context.Context) error {
	indexKey := fmt.Sprintf("%s:definitions", tr.prefix)
	ids, err := tr.redis.SMembers(ctx, indexKey).Result()
	if err != nil {
		return err
	}
	
	for _, id := range ids {
		if _, err := tr.GetDefinition(id); err != nil {
			tr.logger.Error("Failed to load definition", "id", id, "error", err)
		}
	}
	
	tr.logger.Info("Loaded definitions from Redis", "count", len(ids))
	return nil
}

// LoadTemplatesFromRedis loads all templates from Redis
func (tr *TaskRegistry) LoadTemplatesFromRedis(ctx context.Context) error {
	indexKey := fmt.Sprintf("%s:templates", tr.prefix)
	ids, err := tr.redis.SMembers(ctx, indexKey).Result()
	if err != nil {
		return err
	}
	
	for _, id := range ids {
		if _, err := tr.GetTemplate(id); err != nil {
			tr.logger.Error("Failed to load template", "id", id, "error", err)
		}
	}
	
	tr.logger.Info("Loaded templates from Redis", "count", len(ids))
	return nil
}