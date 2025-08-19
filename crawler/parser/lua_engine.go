package parser

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	lua "github.com/yuin/gopher-lua"
	luajson "github.com/layeh/gopher-json"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// LuaEngine Lua脚本引擎
type LuaEngine struct {
	vm         *lua.LState
	mongodb    *mongo.Database
	redis      *redis.Client
	taskQueue  chan *task.Task
	logger     *slog.Logger
	scriptBase string // 脚本基础路径（运行时传入）
}

// LuaEngineConfig Lua引擎配置
type LuaEngineConfig struct {
	MongoDB    *mongo.Database
	Redis      *redis.Client
	Logger     *slog.Logger
	ScriptBase string // 脚本基础路径，如 "/app/scripts" 或 "./crawler/scripts"
}

// NewLuaEngine 创建Lua引擎（保留向后兼容）
func NewLuaEngine(mongodb *mongo.Database, redis *redis.Client, logger *slog.Logger) *LuaEngine {
	return NewLuaEngineWithConfig(&LuaEngineConfig{
		MongoDB:    mongodb,
		Redis:      redis,
		Logger:     logger,
		ScriptBase: "crawler/scripts", // 默认路径
	})
}

// NewLuaEngineWithConfig 使用配置创建Lua引擎
func NewLuaEngineWithConfig(config *LuaEngineConfig) *LuaEngine {
	L := lua.NewState()

	// 加载JSON库
	luajson.Preload(L)

	// 默认脚本路径
	scriptBase := config.ScriptBase
	if scriptBase == "" {
		scriptBase = "crawler/scripts"
	}

	engine := &LuaEngine{
		vm:         L,
		mongodb:    config.MongoDB,
		redis:      config.Redis,
		taskQueue:  make(chan *task.Task, 1000),
		logger:     config.Logger,
		scriptBase: scriptBase,
	}

	// 注册全局函数
	engine.registerFunctions()

	return engine
}

// Execute 执行Lua脚本
func (e *LuaEngine) Execute(projectID, scriptName string, responseData []byte) error {
	return e.ExecuteWithContext(projectID, scriptName, responseData, nil)
}

// SetScriptBase 设置脚本基础路径（用于运行时配置）
func (e *LuaEngine) SetScriptBase(base string) {
	e.scriptBase = base
}

// GetScriptBase 获取当前脚本基础路径
func (e *LuaEngine) GetScriptBase() string {
	return e.scriptBase
}

// ExecuteWithContext 执行Lua脚本（带上下文信息）
func (e *LuaEngine) ExecuteWithContext(projectID, scriptName string, responseData []byte, taskInfo map[string]string) error {
	// 构建脚本路径（使用运行时传入的基础路径）
	scriptPath := filepath.Join(e.scriptBase, "projects", projectID, "parsers", scriptName)

	// 设置全局变量
	e.vm.SetGlobal("PROJECT_ID", lua.LString(projectID))
	e.vm.SetGlobal("SCRIPT_NAME", lua.LString(scriptName))
	e.vm.SetGlobal("response", lua.LString(responseData))
	
	// 设置任务相关信息
	if taskInfo != nil {
		if taskID, ok := taskInfo["task_id"]; ok {
			e.vm.SetGlobal("TASK_ID", lua.LString(taskID))
		}
		if url, ok := taskInfo["url"]; ok {
			e.vm.SetGlobal("URL", lua.LString(url))
		}
		if parentID, ok := taskInfo["parent_id"]; ok {
			e.vm.SetGlobal("PARENT_ID", lua.LString(parentID))
		}
	}

	// 加载项目配置（可选）
	configPath := filepath.Join(e.scriptBase, "projects", projectID, "config.lua")
	if err := e.vm.DoFile(configPath); err != nil {
		e.logger.Debug("No config file or error loading config",
			"project", projectID,
			"path", configPath,
			"error", err)
	}

	// 加载公共函数（可选）
	commonPath := filepath.Join(e.scriptBase, "projects", projectID, "common.lua")
	if err := e.vm.DoFile(commonPath); err != nil {
		e.logger.Debug("No common file or error loading common",
			"project", projectID,
			"path", commonPath,
			"error", err)
	}

	// 执行解析脚本
	e.logger.Info("Executing Lua script",
		"project", projectID,
		"script", scriptName,
		"path", scriptPath)
	
	if err := e.vm.DoFile(scriptPath); err != nil {
		return fmt.Errorf("failed to execute script %s: %w", scriptPath, err)
	}

	return nil
}

// GetTaskQueue 获取任务队列
func (e *LuaEngine) GetTaskQueue() <-chan *task.Task {
	return e.taskQueue
}

// Close 关闭引擎
func (e *LuaEngine) Close() {
	if e.vm != nil {
		e.vm.Close()
	}
	close(e.taskQueue)
}

// registerFunctions 注册Lua函数
func (e *LuaEngine) registerFunctions() {
	// MongoDB操作
	e.vm.SetGlobal("mongo_save", e.vm.NewFunction(e.mongoSave))
	e.vm.SetGlobal("mongo_save_batch", e.vm.NewFunction(e.mongoSaveBatch))
	e.vm.SetGlobal("mongo_upsert", e.vm.NewFunction(e.mongoUpsert))

	// Redis操作
	e.vm.SetGlobal("redis_set", e.vm.NewFunction(e.redisSet))
	e.vm.SetGlobal("redis_get", e.vm.NewFunction(e.redisGet))

	// 任务队列
	e.vm.SetGlobal("create_task", e.vm.NewFunction(e.createTask))
	e.vm.SetGlobal("create_tasks", e.vm.NewFunction(e.createTasks))

	// 日志
	e.vm.SetGlobal("log_info", e.vm.NewFunction(e.logInfo))
	e.vm.SetGlobal("log_error", e.vm.NewFunction(e.logError))
}

// mongoSave 保存到MongoDB
func (e *LuaEngine) mongoSave(L *lua.LState) int {
	collection := L.ToString(1)
	dataTable := L.ToTable(2)

	if collection == "" || dataTable == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("invalid parameters"))
		return 2
	}

	// 转换Lua table为Go map
	data := e.luaTableToMap(dataTable)

	// 添加时间戳
	if _, ok := data["created_at"]; !ok {
		data["created_at"] = time.Now()
	}

	// 保存到MongoDB
	_, err := e.mongodb.Collection(collection).InsertOne(context.Background(), data)
	if err != nil {
		e.logger.Error("Failed to save to MongoDB",
			"collection", collection,
			"error", err)
		L.Push(lua.LBool(false))
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LBool(true))
	return 1
}

// mongoSaveBatch 批量保存到MongoDB
func (e *LuaEngine) mongoSaveBatch(L *lua.LState) int {
	collection := L.ToString(1)
	dataArray := L.ToTable(2)

	if collection == "" || dataArray == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("invalid parameters"))
		return 2
	}

	// 转换为文档数组
	var docs []interface{}
	dataArray.ForEach(func(k, v lua.LValue) {
		if tbl, ok := v.(*lua.LTable); ok {
			doc := e.luaTableToMap(tbl)
			if _, ok := doc["created_at"]; !ok {
				doc["created_at"] = time.Now()
			}
			docs = append(docs, doc)
		}
	})

	if len(docs) == 0 {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("no documents to save"))
		return 2
	}

	// 批量插入
	_, err := e.mongodb.Collection(collection).InsertMany(context.Background(), docs)
	if err != nil {
		e.logger.Error("Failed to batch save to MongoDB",
			"collection", collection,
			"count", len(docs),
			"error", err)
		L.Push(lua.LBool(false))
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LBool(true))
	L.Push(lua.LNumber(len(docs)))
	return 2
}

// mongoUpsert MongoDB upsert操作（存在则更新，不存在则插入）
func (e *LuaEngine) mongoUpsert(L *lua.LState) int {
	collection := L.ToString(1)
	filterTable := L.ToTable(2)
	dataTable := L.ToTable(3)

	if collection == "" || filterTable == nil || dataTable == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("invalid parameters"))
		return 2
	}

	// 转换filter和data
	filter := e.luaTableToMap(filterTable)
	data := e.luaTableToMap(dataTable)

	// 添加/更新时间戳
	if _, ok := data["created_at"]; !ok {
		data["created_at"] = time.Now()
	}
	data["updated_at"] = time.Now()

	// 执行upsert操作
	opts := options.Update().SetUpsert(true)
	
	_, err := e.mongodb.Collection(collection).UpdateOne(
		context.Background(),
		filter,
		map[string]interface{}{"$set": data},
		opts,
	)
	
	if err != nil {
		e.logger.Error("Failed to upsert to MongoDB",
			"collection", collection,
			"filter", filter,
			"error", err)
		L.Push(lua.LBool(false))
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LBool(true))
	return 1
}

// redisSet Redis设置值
func (e *LuaEngine) redisSet(L *lua.LState) int {
	key := L.ToString(1)
	value := L.ToString(2)
	ttl := L.ToInt(3) // 秒

	if key == "" {
		L.Push(lua.LBool(false))
		return 1
	}

	var err error
	if ttl > 0 {
		err = e.redis.Set(context.Background(), key, value, time.Duration(ttl)*time.Second).Err()
	} else {
		err = e.redis.Set(context.Background(), key, value, 0).Err()
	}

	if err != nil {
		e.logger.Error("Failed to set Redis key",
			"key", key,
			"error", err)
		L.Push(lua.LBool(false))
		return 1
	}

	L.Push(lua.LBool(true))
	return 1
}

// redisGet Redis获取值
func (e *LuaEngine) redisGet(L *lua.LState) int {
	key := L.ToString(1)

	if key == "" {
		L.Push(lua.LNil)
		return 1
	}

	val, err := e.redis.Get(context.Background(), key).Result()
	if err != nil {
		if err != redis.Nil {
			e.logger.Error("Failed to get Redis key",
				"key", key,
				"error", err)
		}
		L.Push(lua.LNil)
		return 1
	}

	L.Push(lua.LString(val))
	return 1
}

// createTask 创建单个任务
func (e *LuaEngine) createTask(L *lua.LState) int {
	taskData := L.ToTable(1)

	if taskData == nil {
		L.Push(lua.LString(""))
		return 1
	}

	// 使用提供的ID或生成新的
	taskID := e.tableGetString(taskData, "id")
	if taskID == "" {
		taskID = uuid.New().String()
	}

	newTask := &task.Task{
		ID:        taskID,
		Type:      e.tableGetString(taskData, "type"),
		URL:       e.tableGetString(taskData, "url"),
		Method:    e.tableGetString(taskData, "method"),
		ProjectID: e.tableGetString(taskData, "project_id"),
		LuaScript: e.tableGetString(taskData, "lua_script"),
	}

	// 设置默认方法
	if newTask.Method == "" {
		newTask.Method = "GET"
	}

	// 获取其他字段
	if priority := taskData.RawGetString("priority"); priority != lua.LNil {
		newTask.Priority = int(lua.LVAsNumber(priority))
	}

	if parentID := taskData.RawGetString("parent_id"); parentID != lua.LNil {
		newTask.ParentID = lua.LVAsString(parentID)
	}

	// 处理请求body (转换string为[]byte)
	if body := taskData.RawGetString("body"); body != lua.LNil {
		newTask.Body = []byte(lua.LVAsString(body))
	}

	// 处理headers
	if headers := taskData.RawGetString("headers"); headers != lua.LNil {
		if headersTable, ok := headers.(*lua.LTable); ok {
			newTask.Headers = make(map[string]string)
			headersTable.ForEach(func(k, v lua.LValue) {
				newTask.Headers[lua.LVAsString(k)] = lua.LVAsString(v)
			})
		}
	}

	// 处理metadata
	if metadata := taskData.RawGetString("metadata"); metadata != lua.LNil {
		if metaTable, ok := metadata.(*lua.LTable); ok {
			newTask.Metadata = e.luaTableToMap(metaTable)
		}
	}

	// 处理其他可选字段
	if name := taskData.RawGetString("name"); name != lua.LNil {
		newTask.Name = lua.LVAsString(name)
	}

	if maxRetries := taskData.RawGetString("max_retries"); maxRetries != lua.LNil {
		newTask.MaxRetries = int(lua.LVAsNumber(maxRetries))
	}

	if timeout := taskData.RawGetString("timeout"); timeout != lua.LNil {
		// 假设Lua传递的是秒数
		newTask.Timeout = time.Duration(lua.LVAsNumber(timeout)) * time.Second
	}

	// 处理存储配置
	if storageConf := taskData.RawGetString("storage_config"); storageConf != lua.LNil {
		if storageTable, ok := storageConf.(*lua.LTable); ok {
			newTask.StorageConf = task.StorageConfig{
				Type:       e.tableGetString(storageTable, "type"),
				Database:   e.tableGetString(storageTable, "database"),
				Collection: e.tableGetString(storageTable, "collection"),
			}
		}
	}

	// 发送到任务队列
	select {
	case e.taskQueue <- newTask:
		e.logger.Debug("Task created from Lua",
			"task_id", newTask.ID,
			"type", newTask.Type,
			"url", newTask.URL)
		L.Push(lua.LString(newTask.ID))
	default:
		e.logger.Warn("Task queue is full")
		L.Push(lua.LString(""))
	}

	return 1
}

// createTasks 批量创建任务
func (e *LuaEngine) createTasks(L *lua.LState) int {
	tasksArray := L.ToTable(1)

	if tasksArray == nil {
		L.Push(lua.LNumber(0))
		return 1
	}

	var taskCount int
	tasksArray.ForEach(func(k, v lua.LValue) {
		if tbl, ok := v.(*lua.LTable); ok {
			newTask := &task.Task{
				ID:     uuid.New().String(),
				Type:   e.tableGetString(tbl, "type"),
				URL:    e.tableGetString(tbl, "url"),
				Method: e.tableGetString(tbl, "method"),
			}

			if newTask.Method == "" {
				newTask.Method = "GET"
			}

			select {
			case e.taskQueue <- newTask:
				taskCount++
			default:
				e.logger.Warn("Task queue is full, skipping task")
			}
		}
	})

	L.Push(lua.LNumber(taskCount))
	return 1
}

// logInfo 记录信息日志
func (e *LuaEngine) logInfo(L *lua.LState) int {
	message := L.ToString(1)
	e.logger.Info(message, "source", "lua_script")
	return 0
}

// logError 记录错误日志
func (e *LuaEngine) logError(L *lua.LState) int {
	message := L.ToString(1)
	e.logger.Error(message, "source", "lua_script")
	return 0
}

// luaTableToMap 转换Lua table为Go map
func (e *LuaEngine) luaTableToMap(table *lua.LTable) map[string]interface{} {
	result := make(map[string]interface{})

	table.ForEach(func(k, v lua.LValue) {
		key := lua.LVAsString(k)
		result[key] = e.luaValueToGo(v)
	})

	return result
}

// luaValueToGo 转换Lua值为Go值
func (e *LuaEngine) luaValueToGo(lv lua.LValue) interface{} {
	switch v := lv.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LString:
		return string(v)
	case lua.LNumber:
		return float64(v)
	case *lua.LTable:
		// 检查是否为数组
		if e.isArray(v) {
			return e.luaArrayToSlice(v)
		}
		return e.luaTableToMap(v)
	default:
		return lua.LVAsString(lv)
	}
}

// isArray 检查Lua table是否为数组
func (e *LuaEngine) isArray(table *lua.LTable) bool {
	expectedIndex := 1
	isArr := true

	table.ForEach(func(k, v lua.LValue) {
		if _, ok := k.(lua.LNumber); !ok {
			isArr = false
			return
		}
		if int(k.(lua.LNumber)) != expectedIndex {
			isArr = false
			return
		}
		expectedIndex++
	})

	return isArr
}

// luaArrayToSlice 转换Lua数组为Go切片
func (e *LuaEngine) luaArrayToSlice(table *lua.LTable) []interface{} {
	var result []interface{}

	table.ForEach(func(k, v lua.LValue) {
		if _, ok := k.(lua.LNumber); ok {
			result = append(result, e.luaValueToGo(v))
		}
	})

	return result
}

// tableGetString 从Lua table获取字符串
func (e *LuaEngine) tableGetString(table *lua.LTable, key string) string {
	val := table.RawGetString(key)
	if val == lua.LNil {
		return ""
	}
	return lua.LVAsString(val)
}

// ExecuteString 直接执行Lua代码字符串（用于测试）
func (e *LuaEngine) ExecuteString(code string, responseData []byte) error {
	e.vm.SetGlobal("response", lua.LString(responseData))
	return e.vm.DoString(code)
}