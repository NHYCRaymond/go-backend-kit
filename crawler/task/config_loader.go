package task

import (
	"log/slog"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/NHYCRaymond/go-backend-kit/errors"
	"github.com/go-redis/redis/v8"
	"github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"
)

// ConfigLoader loads task configurations from various sources
type ConfigLoader struct {
	registry    *TaskRegistry
	manager     *TaskManager
	redis       *redis.Client
	prefix      string
	logger      *slog.Logger
	luaState    *lua.LState
	
	// Configuration sources
	sources     []ConfigSource
	
	// Script cache
	scriptCache map[string]*lua.LFunction
}

// ConfigSource represents a configuration source
type ConfigSource interface {
	Name() string
	Load(ctx context.Context) ([]*TaskConfig, error)
}

// TaskConfig represents a task configuration
type TaskConfig struct {
	Type       string                 `json:"type" yaml:"type"`           // definition, template, script
	Definition *TaskDef        `json:"definition" yaml:"definition"`
	Template   *TaskTemplate          `json:"template" yaml:"template"`
	Script     *ScriptConfig          `json:"script" yaml:"script"`
	Metadata   map[string]interface{} `json:"metadata" yaml:"metadata"`
}

// ScriptConfig represents a Lua script configuration
type ScriptConfig struct {
	ID          string                 `json:"id" yaml:"id"`
	Name        string                 `json:"name" yaml:"name"`
	Description string                 `json:"description" yaml:"description"`
	Language    string                 `json:"language" yaml:"language"` // lua, javascript
	Source      string                 `json:"source" yaml:"source"`     // inline, file, url
	Content     string                 `json:"content" yaml:"content"`
	File        string                 `json:"file" yaml:"file"`
	URL         string                 `json:"url" yaml:"url"`
	Parameters  map[string]interface{} `json:"parameters" yaml:"parameters"`
	Schedule    string                 `json:"schedule" yaml:"schedule"` // Cron expression
}

// LoaderConfig contains configuration loader settings
type LoaderConfig struct {
	Registry    *TaskRegistry
	Manager     *TaskManager
	Redis       *redis.Client
	Prefix      string
	Logger      *slog.Logger
	ConfigPaths []string
	WatchFiles  bool
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(config *LoaderConfig) *ConfigLoader {
	loader := &ConfigLoader{
		registry:    config.Registry,
		manager:     config.Manager,
		redis:       config.Redis,
		prefix:      config.Prefix,
		logger:      config.Logger,
		sources:     []ConfigSource{},
		scriptCache: make(map[string]*lua.LFunction),
	}
	
	// Initialize Lua state
	loader.luaState = lua.NewState()
	loader.setupLuaEnvironment()
	
	// Add file sources for config paths
	for _, path := range config.ConfigPaths {
		loader.AddSource(NewFileSource(path, config.Logger))
	}
	
	// Add Redis source
	loader.AddSource(NewRedisSource(config.Redis, config.Prefix, config.Logger))
	
	return loader
}

// AddSource adds a configuration source
func (cl *ConfigLoader) AddSource(source ConfigSource) {
	cl.sources = append(cl.sources, source)
}

// LoadAll loads all configurations from all sources
func (cl *ConfigLoader) LoadAll(ctx context.Context) error {
	cl.logger.Info("Loading task configurations")
	
	totalConfigs := 0
	for _, source := range cl.sources {
		configs, err := source.Load(ctx)
		if err != nil {
			cl.logger.Error("Failed to load from source",
				"source", source.Name(),
				"error", err)
			continue
		}
		
		for _, config := range configs {
			if err := cl.processConfig(ctx, config); err != nil {
				cl.logger.Error("Failed to process config",
					"type", config.Type,
					"error", err)
				continue
			}
			totalConfigs++
		}
	}
	
	cl.logger.Info("Loaded task configurations", "count", totalConfigs)
	return nil
}

// processConfig processes a single configuration
func (cl *ConfigLoader) processConfig(ctx context.Context, config *TaskConfig) error {
	switch config.Type {
	case "definition":
		if config.Definition != nil {
			return cl.registry.RegisterDefinition(config.Definition)
		}
	case "template":
		if config.Template != nil {
			return cl.registry.RegisterTemplate(config.Template)
		}
	case "script":
		if config.Script != nil {
			return cl.loadScript(ctx, config.Script)
		}
	default:
		return errors.New(errors.ValidationErrorCode, fmt.Sprintf("unknown config type: %s", config.Type))
	}
	return nil
}

// loadScript loads and compiles a Lua script
func (cl *ConfigLoader) loadScript(ctx context.Context, script *ScriptConfig) error {
	cl.logger.Info("Loading script", "id", script.ID, "name", script.Name)
	
	var scriptContent string
	
	switch script.Source {
	case "inline":
		scriptContent = script.Content
	case "file":
		data, err := ioutil.ReadFile(script.File)
		if err != nil {
			return fmt.Errorf("failed to read script file: %w", err)
		}
		scriptContent = string(data)
	case "url":
		// TODO: Implement URL loading
		return errors.New(errors.NotImplementedErrorCode, "URL script loading not yet implemented")
	default:
		return errors.New(errors.ValidationErrorCode, fmt.Sprintf("unknown script source: %s", script.Source))
	}
	
	// Compile Lua script
	fn, err := cl.luaState.LoadString(scriptContent)
	if err != nil {
		return fmt.Errorf("failed to compile script: %w", err)
	}
	
	// Cache compiled function
	cl.scriptCache[script.ID] = fn
	
	// Register as task definition if it defines tasks
	if err := cl.registerScriptTasks(script, fn); err != nil {
		return fmt.Errorf("failed to register script tasks: %w", err)
	}
	
	return nil
}

// registerScriptTasks registers tasks defined by a Lua script
func (cl *ConfigLoader) registerScriptTasks(script *ScriptConfig, fn *lua.LFunction) error {
	// Execute script to get task definitions
	cl.luaState.Push(fn)
	if err := cl.luaState.PCall(0, lua.MultRet, nil); err != nil {
		return err
	}
	
	// Get registered tasks from Lua global
	tasksTable := cl.luaState.GetGlobal("tasks")
	if tasksTable == lua.LNil {
		return nil // No tasks defined
	}
	
	// Convert Lua table to task definitions
	tasks, ok := tasksTable.(*lua.LTable)
	if !ok {
		return errors.New(errors.ValidationErrorCode, "tasks must be a table")
	}
	
	tasks.ForEach(func(key, value lua.LValue) {
		taskDef := cl.luaTableToTaskDef(value.(*lua.LTable))
		if taskDef != nil {
			taskDef.Metadata["script_id"] = script.ID
			taskDef.Metadata["script_name"] = script.Name
			
			if err := cl.registry.RegisterDefinition(taskDef); err != nil {
				cl.logger.Error("Failed to register script task",
					"script", script.ID,
					"task", taskDef.ID,
					"error", err)
			}
		}
	})
	
	return nil
}

// setupLuaEnvironment sets up the Lua execution environment
func (cl *ConfigLoader) setupLuaEnvironment() {
	L := cl.luaState
	
	// Register task type constants
	L.SetGlobal("TYPE_SEED", lua.LString(TypeSeed))
	L.SetGlobal("TYPE_DETAIL", lua.LString(TypeDetail))
	L.SetGlobal("TYPE_LIST", lua.LString(TypeList))
	L.SetGlobal("TYPE_API", lua.LString(TypeAPI))
	L.SetGlobal("TYPE_BROWSER", lua.LString(TypeBrowser))
	L.SetGlobal("TYPE_AGGREGATE", lua.LString(TypeAggregate))
	
	// Register priority constants
	L.SetGlobal("PRIORITY_LOW", lua.LNumber(PriorityLow))
	L.SetGlobal("PRIORITY_NORMAL", lua.LNumber(PriorityNormal))
	L.SetGlobal("PRIORITY_HIGH", lua.LNumber(PriorityHigh))
	L.SetGlobal("PRIORITY_URGENT", lua.LNumber(PriorityUrgent))
	
	// Register status constants
	L.SetGlobal("STATUS_PENDING", lua.LString(StatusPending))
	L.SetGlobal("STATUS_QUEUED", lua.LString(StatusQueued))
	L.SetGlobal("STATUS_RUNNING", lua.LString(StatusRunning))
	L.SetGlobal("STATUS_COMPLETED", lua.LString(StatusCompleted))
	L.SetGlobal("STATUS_FAILED", lua.LString(StatusFailed))
	L.SetGlobal("STATUS_RETRYING", lua.LString(StatusRetrying))
	L.SetGlobal("STATUS_CANCELLED", lua.LString(StatusCancelled))
	
	// Register crawler API
	L.SetGlobal("crawler", cl.createCrawlerAPI())
	
	// Register utility functions
	L.SetGlobal("log", L.NewFunction(cl.luaLog))
	L.SetGlobal("http_get", L.NewFunction(cl.luaHTTPGet))
	L.SetGlobal("json_decode", L.NewFunction(cl.luaJSONDecode))
	L.SetGlobal("json_encode", L.NewFunction(cl.luaJSONEncode))
	L.SetGlobal("redis_get", L.NewFunction(cl.luaRedisGet))
	L.SetGlobal("redis_set", L.NewFunction(cl.luaRedisSet))
	
	// Register task creation functions
	L.SetGlobal("create_task", L.NewFunction(cl.luaCreateTask))
	L.SetGlobal("create_definition", L.NewFunction(cl.luaCreateDefinition))
	L.SetGlobal("create_template", L.NewFunction(cl.luaCreateTemplate))
}

// createCrawlerAPI creates the crawler API table for Lua
func (cl *ConfigLoader) createCrawlerAPI() *lua.LTable {
	L := cl.luaState
	api := L.NewTable()
	
	// Task management
	api.RawSetString("submit_task", L.NewFunction(cl.luaSubmitTask))
	api.RawSetString("get_task", L.NewFunction(cl.luaGetTask))
	api.RawSetString("update_task", L.NewFunction(cl.luaUpdateTask))
	
	// Data extraction
	api.RawSetString("extract_links", L.NewFunction(cl.luaExtractLinks))
	api.RawSetString("extract_data", L.NewFunction(cl.luaExtractData))
	
	// Storage
	api.RawSetString("save_data", L.NewFunction(cl.luaSaveData))
	api.RawSetString("query_data", L.NewFunction(cl.luaQueryData))
	
	return api
}

// Lua function implementations

func (cl *ConfigLoader) luaLog(L *lua.LState) int {
	level := L.ToString(1)
	message := L.ToString(2)
	
	switch level {
	case "debug":
		cl.logger.Debug(message)
	case "info":
		cl.logger.Info(message)
	case "warn":
		cl.logger.Warn(message)
	case "error":
		cl.logger.Error(message)
	default:
		cl.logger.Info(message)
	}
	
	return 0
}

func (cl *ConfigLoader) luaCreateTask(L *lua.LState) int {
	url := L.ToString(1)
	taskType := L.ToString(2)
	priority := L.ToInt(3)
	
	task := NewTask(url, taskType, priority)
	
	// Convert task to Lua table
	taskTable := cl.taskToLuaTable(task)
	L.Push(taskTable)
	
	return 1
}

func (cl *ConfigLoader) luaSubmitTask(L *lua.LState) int {
	taskTable := L.ToTable(1)
	task := cl.luaTableToTask(taskTable)
	
	if task != nil {
		// Submit task through manager or queue
		// This is simplified - actual implementation would use the task queue
		cl.logger.Info("Submitting task from Lua", "task_id", task.ID)
	}
	
	return 0
}

func (cl *ConfigLoader) luaHTTPGet(L *lua.LState) int {
	// Simplified HTTP GET implementation
	url := L.ToString(1)
	cl.logger.Debug("Lua HTTP GET", "url", url)
	
	// Return mock response for now
	L.Push(lua.LString("{\"status\": 200, \"body\": \"mock response\"}"))
	return 1
}

func (cl *ConfigLoader) luaJSONDecode(L *lua.LState) int {
	jsonStr := L.ToString(1)
	
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	
	L.Push(cl.goToLua(data))
	return 1
}

func (cl *ConfigLoader) luaJSONEncode(L *lua.LState) int {
	value := L.Get(1)
	goValue := cl.luaToGo(value)
	
	data, err := json.Marshal(goValue)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	
	L.Push(lua.LString(data))
	return 1
}

func (cl *ConfigLoader) luaRedisGet(L *lua.LState) int {
	key := L.ToString(1)
	
	ctx := context.Background()
	val, err := cl.redis.Get(ctx, key).Result()
	if err != nil {
		L.Push(lua.LNil)
		return 1
	}
	
	L.Push(lua.LString(val))
	return 1
}

func (cl *ConfigLoader) luaRedisSet(L *lua.LState) int {
	key := L.ToString(1)
	value := L.ToString(2)
	
	ctx := context.Background()
	err := cl.redis.Set(ctx, key, value, 0).Err()
	
	L.Push(lua.LBool(err == nil))
	return 1
}

// Stub functions for task operations
func (cl *ConfigLoader) luaGetTask(L *lua.LState) int {
	return 0
}

func (cl *ConfigLoader) luaUpdateTask(L *lua.LState) int {
	return 0
}

func (cl *ConfigLoader) luaExtractLinks(L *lua.LState) int {
	return 0
}

func (cl *ConfigLoader) luaExtractData(L *lua.LState) int {
	return 0
}

func (cl *ConfigLoader) luaSaveData(L *lua.LState) int {
	return 0
}

func (cl *ConfigLoader) luaQueryData(L *lua.LState) int {
	return 0
}

func (cl *ConfigLoader) luaCreateDefinition(L *lua.LState) int {
	return 0
}

func (cl *ConfigLoader) luaCreateTemplate(L *lua.LState) int {
	return 0
}

// Conversion helpers

func (cl *ConfigLoader) taskToLuaTable(task *Task) *lua.LTable {
	L := cl.luaState
	table := L.NewTable()
	
	table.RawSetString("id", lua.LString(task.ID))
	table.RawSetString("url", lua.LString(task.URL))
	table.RawSetString("type", lua.LString(task.Type))
	table.RawSetString("priority", lua.LNumber(task.Priority))
	table.RawSetString("status", lua.LString(task.Status))
	
	return table
}

func (cl *ConfigLoader) luaTableToTask(table *lua.LTable) *Task {
	task := &Task{}
	
	table.ForEach(func(key, value lua.LValue) {
		keyStr := key.String()
		switch keyStr {
		case "id":
			task.ID = value.String()
		case "url":
			task.URL = value.String()
		case "type":
			task.Type = value.String()
		case "priority":
			if num, ok := value.(lua.LNumber); ok {
				task.Priority = int(num)
			}
		case "status":
			task.Status = Status(value.String())
		}
	})
	
	return task
}

func (cl *ConfigLoader) luaTableToTaskDef(table *lua.LTable) *TaskDef {
	def := &TaskDef{
		Headers:  make(map[string]string),
		Tags:     []string{},
		Labels:   make(map[string]string),
		Metadata: make(map[string]interface{}),
	}
	
	table.ForEach(func(key, value lua.LValue) {
		keyStr := key.String()
		switch keyStr {
		case "id":
			def.ID = value.String()
		case "name":
			def.Name = value.String()
		case "type":
			def.Type = Type(value.String())
		case "priority":
			if num, ok := value.(lua.LNumber); ok {
				def.Priority = Priority(int(num))
			}
		}
	})
	
	return def
}

func (cl *ConfigLoader) goToLua(value interface{}) lua.LValue {
	L := cl.luaState
	
	switch v := value.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(v)
	case int:
		return lua.LNumber(v)
	case int64:
		return lua.LNumber(v)
	case float64:
		return lua.LNumber(v)
	case string:
		return lua.LString(v)
	case []interface{}:
		table := L.NewTable()
		for i, item := range v {
			table.RawSetInt(i+1, cl.goToLua(item))
		}
		return table
	case map[string]interface{}:
		table := L.NewTable()
		for key, val := range v {
			table.RawSetString(key, cl.goToLua(val))
		}
		return table
	default:
		return lua.LString(fmt.Sprint(v))
	}
}

func (cl *ConfigLoader) luaToGo(value lua.LValue) interface{} {
	switch v := value.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LTable:
		// Check if it's an array or map
		isArray := true
		maxIndex := 0
		
		v.ForEach(func(key, _ lua.LValue) {
			if _, ok := key.(lua.LNumber); !ok {
				isArray = false
			} else {
				if idx := int(key.(lua.LNumber)); idx > maxIndex {
					maxIndex = idx
				}
			}
		})
		
		if isArray && maxIndex > 0 {
			arr := make([]interface{}, maxIndex)
			for i := 1; i <= maxIndex; i++ {
				arr[i-1] = cl.luaToGo(v.RawGetInt(i))
			}
			return arr
		} else {
			m := make(map[string]interface{})
			v.ForEach(func(key, val lua.LValue) {
				m[key.String()] = cl.luaToGo(val)
			})
			return m
		}
	default:
		return value.String()
	}
}

// Close closes the configuration loader
func (cl *ConfigLoader) Close() {
	if cl.luaState != nil {
		cl.luaState.Close()
	}
}

// FileSource loads configurations from files
type FileSource struct {
	path   string
	logger *slog.Logger
}

// NewFileSource creates a new file configuration source
func NewFileSource(path string, logger *slog.Logger) *FileSource {
	return &FileSource{
		path:   path,
		logger: logger,
	}
}

// Name returns the source name
func (fs *FileSource) Name() string {
	return fmt.Sprintf("file:%s", fs.path)
}

// Load loads configurations from file
func (fs *FileSource) Load(ctx context.Context) ([]*TaskConfig, error) {
	ext := strings.ToLower(filepath.Ext(fs.path))
	
	data, err := ioutil.ReadFile(fs.path)
	if err != nil {
		return nil, err
	}
	
	var configs []*TaskConfig
	
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &configs); err != nil {
			return nil, err
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &configs); err != nil {
			return nil, err
		}
	case ".lua":
		// Lua script file
		config := &TaskConfig{
			Type: "script",
			Script: &ScriptConfig{
				ID:       filepath.Base(fs.path),
				Name:     filepath.Base(fs.path),
				Language: "lua",
				Source:   "file",
				File:     fs.path,
			},
		}
		configs = append(configs, config)
	default:
		return nil, errors.New(errors.ValidationErrorCode, fmt.Sprintf("unsupported file type: %s", ext))
	}
	
	return configs, nil
}

// RedisSource loads configurations from Redis
type RedisSource struct {
	redis  *redis.Client
	prefix string
	logger *slog.Logger
}

// NewRedisSource creates a new Redis configuration source
func NewRedisSource(redis *redis.Client, prefix string, logger *slog.Logger) *RedisSource {
	return &RedisSource{
		redis:  redis,
		prefix: prefix,
		logger: logger,
	}
}

// Name returns the source name
func (rs *RedisSource) Name() string {
	return "redis"
}

// Load loads configurations from Redis
func (rs *RedisSource) Load(ctx context.Context) ([]*TaskConfig, error) {
	pattern := fmt.Sprintf("%s:config:*", rs.prefix)
	keys, err := rs.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}
	
	var configs []*TaskConfig
	
	for _, key := range keys {
		data, err := rs.redis.Get(ctx, key).Result()
		if err != nil {
			rs.logger.Error("Failed to get config from Redis", "key", key, "error", err)
			continue
		}
		
		var config TaskConfig
		if err := json.Unmarshal([]byte(data), &config); err != nil {
			rs.logger.Error("Failed to parse config", "key", key, "error", err)
			continue
		}
		
		configs = append(configs, &config)
	}
	
	return configs, nil
}