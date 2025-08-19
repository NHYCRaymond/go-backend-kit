# Lua 脚本数据解析使用指南

## 概述

分布式爬虫系统现在支持使用 Lua 脚本进行数据解析。每个任务可以绑定一个 Lua 脚本，实现灵活的数据处理、存储和任务分发。

## 重要：脚本路径配置

Lua 脚本路径不存储在 MongoDB 中，而是通过运行时参数传递。这样避免了路径硬编码问题。

### 配置优先级（从高到低）

1. **命令行参数**: `--scripts=/app/scripts`
2. **环境变量**: `CRAWLER_SCRIPT_BASE=/app/scripts`
3. **配置文件**: `crawler.script_base` in node.yaml
4. **默认值**: `crawler/scripts`

## 系统架构

```
任务(Task) → Node执行 → Lua脚本解析 → 双层存储
                                    ↓
                            原始数据 + 统一格式数据
```

## Node 使用 Lua 脚本的流程

### 1. 任务配置

任务需要配置两个关键字段：
- `project_id`: 项目标识（如 `tennis_booking`）
- `lua_script`: Lua脚本文件名（如 `drip_ground_board.lua`）

### 2. Node 执行过程

当 Node 的 Worker 接收到任务时：

1. **检测 Lua 配置**
   ```go
   if t.ProjectID != "" && t.LuaScript != "" {
       // 使用 Lua 脚本解析
   }
   ```

2. **执行器调用**
   - Node 的 executor 已配置 MongoDB 和 Redis
   - 执行器会创建 Lua 引擎并执行脚本
   - Lua 脚本直接处理数据存储

3. **自动流程**
   - Fetch: 获取 HTTP 响应
   - Parse: Lua 脚本解析数据
   - Store: 双层存储（原始 + 统一格式）
   - Queue: 生成下游任务（如需要）

### 3. Lua 脚本功能

Lua 脚本可以：
- 解析 JSON 响应
- 转换数据为统一格式
- 直接保存到 MongoDB
- 操作 Redis 缓存
- 创建新任务

## 配置示例

### 任务配置（MongoDB）

```javascript
{
  name: "drip_ground_board",
  type: "api",
  url: "https://api.example.com/courts",
  method: "GET",
  
  // Lua 脚本配置
  project_id: "tennis_booking",
  lua_script: "drip_ground_board.lua",
  
  // 其他配置...
}
```

### Lua 脚本结构

脚本基础路径（`script_base`）下的目录结构：

```
${script_base}/projects/
└── tennis_booking/                    # 项目目录
    ├── config.lua                     # 统一数据结构定义
    ├── common.lua                     # 公共函数
    └── parsers/                       # 解析脚本
        ├── drip_ground_board.lua
        ├── venue_massage_site.lua
        ├── pospal_venue.lua
        └── pospal_store_5662377.lua
```

**注意**: 
- 任务中只存储 `project_id` 和 `lua_script` 名称
- 实际路径在运行时拼接：`${script_base}/projects/${project_id}/parsers/${lua_script}`
- 这样可以在不同环境使用不同的脚本路径

## 双层存储

### 1. 原始数据层
保存完整的 HTTP 响应，用于调试和回溯：
- `court_booking_boards` - drip_ground_board 原始数据
- `venue_tennis_courts` - venue_massage_site 原始数据
- `pospal_venue_slots` - pospal_venue 原始数据
- `pospal_venue_slots_5662377` - pospal_store_5662377 原始数据

### 2. 统一格式层
标准化的数据结构，便于查询和分析：
- `unified_venues` - 所有数据源的统一格式

## 日志输出

Node 执行带 Lua 脚本的任务时会输出详细日志：

```
INFO Executing task with enhanced executor {
  "task_id": "xxx",
  "has_lua_script": true,
  "project_id": "tennis_booking",
  "lua_script": "drip_ground_board.lua"
}

INFO Using Lua script for parsing {
  "project_id": "tennis_booking",
  "lua_script": "drip_ground_board.lua"
}

INFO Lua script executed successfully
```

## 数据查询

### 查询统一格式数据

```javascript
// 查询所有可用场地
db.unified_venues.find({
  project_id: "tennis_booking",
  "statistics.available_slots": { $gt: 0 }
})

// 查询特定数据源
db.unified_venues.find({
  project_id: "tennis_booking",
  source: "drip_ground_board"
})
```

### 查询原始数据

```javascript
// 查询原始响应
db.court_booking_boards.findOne({ 
  task_id: "xxx" 
})
```

## 优势

1. **灵活性**: 修改 Lua 脚本无需重新编译
2. **统一管理**: 同一项目的数据统一格式
3. **可追溯**: 保留原始数据便于调试
4. **高性能**: Lua 脚本执行效率高
5. **易扩展**: 新数据源只需添加脚本

## 添加新数据源

1. 在项目的 `parsers/` 目录创建新的 Lua 脚本
2. 使用 `create_unified_record()` 创建统一结构
3. 解析数据并填充字段
4. 调用 `mongo_save()` 保存数据
5. 更新任务配置，指定新脚本

## 注意事项

1. **脚本路径**: 
   - 脚本路径通过运行时参数传递，不存储在数据库
   - 生产环境建议使用绝对路径（如 `/app/scripts`）
   - 开发环境可使用相对路径（如 `./crawler/scripts`）
2. **错误处理**: Lua 脚本应包含错误处理逻辑
3. **性能**: 避免在 Lua 脚本中进行复杂计算
4. **安全**: 不要在脚本中硬编码敏感信息
5. **可移植性**: 使用相对于 `script_base` 的路径，不要使用绝对路径

## 测试方法

### 1. 单任务测试

```javascript
// 创建测试任务
db.crawler_tasks.insertOne({
  name: "test_lua_task",
  type: "api",
  url: "https://api.example.com/test",
  project_id: "tennis_booking",
  lua_script: "test_parser.lua"
})
```

### 2. 查看执行结果

```javascript
// 查看统一数据
db.unified_venues.find({
  project_id: "tennis_booking"
}).sort({ "metadata.crawled_at": -1 }).limit(1)

// 查看原始数据
db.court_booking_boards.find().sort({ crawled_at: -1 }).limit(1)
```

## 故障排查

1. **检查任务配置**
   ```javascript
   db.crawler_tasks.findOne({ name: "task_name" })
   ```

2. **查看 Node 日志**
   - 确认 "has_lua_script": true
   - 检查 Lua 执行错误

3. **验证脚本路径**
   - 确保脚本文件存在
   - 检查项目目录结构

4. **测试 Lua 脚本**
   - 使用样本数据独立测试脚本
   - 检查数据格式转换逻辑