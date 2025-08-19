# Venue Task Flow - 子任务分配机制

## 概述
Venue任务使用两阶段处理流程，通过动态生成子任务来获取多个日期的场地数据。

## 流程详解

### 第一阶段：初始请求
1. **主任务执行**
   - MongoDB中的venue任务被调度器取出
   - 请求体只包含基本参数：`vid=1557&pid=18&openid=xxx`
   - 不包含`date`和`week`参数

2. **API响应**
   - 返回包含`week`数组，显示可查询的日期
   - 返回今天的场地数据（`site_data`）
   ```json
   {
     "vn_id": "1557",
     "week": [
       {"week": "今天", "time": "2025-08-18", "date": "8.18"},
       {"week": "明天", "time": "2025-08-19", "date": "8.19"},
       {"week": "后天", "time": "2025-08-20", "date": "8.20"},
       // ... 更多日期
     ],
     "site_data": { /* 今天的场地数据 */ }
   }
   ```

### 第二阶段：子任务生成

#### 子任务创建流程
1. **Lua脚本处理** (`venue_massage_site.lua`)
   ```lua
   -- 检测是否为初始请求
   if not request_date then
       -- 缓存week数据到Redis
       redis_set("venue:week:1557", week_json, 86400)
       
       -- 为每个日期创建子任务（跳过今天）
       for i = 2, #data.week do
           local subtask = {
               id = "venue_1557_20250819",
               parent_id = TASK_ID,
               url = "https://wx.dududing.com/VenueMassige/sitedata",
               method = "POST",
               body = "vid=1557&pid=18&date=2025-08-19&week=[...]&openid=xxx",
               lua_script = "venue_massage_site.lua",
               -- ...
           }
           create_task(subtask)
       end
   end
   ```

2. **任务队列处理** (`executor.go`)
   ```go
   // Lua脚本创建的任务被发送到队列
   for newTask := range luaEngine.GetTaskQueue() {
       if e.scheduler != nil {
           e.scheduler.AddTask(newTask)
       }
   }
   ```

3. **调度器接管**
   - 子任务被添加到调度器的任务队列
   - 按照优先级和配置的并发度执行
   - 每个子任务独立请求特定日期的数据

## 子任务特点

### 任务ID生成规则
```
格式：{父任务ID}_{场地ID}_{日期}
示例：venue_task_001_1557_20250819
```

### 请求参数
每个子任务包含：
- `vid`: 场地ID（继承自主任务）
- `pid`: 产品ID（固定为18）
- `date`: 指定查询日期（如2025-08-19）
- `week`: 完整的week数组（从缓存获取）
- `openid`: 用户标识（继承自主任务）

### 执行特性
- **并行执行**：多个日期的子任务可并行处理
- **独立性**：每个子任务独立完成，互不影响
- **错误隔离**：单个子任务失败不影响其他任务
- **数据去重**：通过MongoDB的upsert操作避免重复数据

## 数据存储

### 缓存策略
- **Redis缓存**
  - Key: `venue:week:{venue_id}`
  - Value: week数组的JSON
  - TTL: 24小时
  - 作用：避免重复请求week信息

### 数据写入
- **venue_court_details**: 场地基本信息
  - ID格式：`{venue_id}_{court_id}_{date}`
  
- **venue_court_periods**: 时间段预订信息
  - ID格式：`{venue_id}_{date}_{court_id}_{time_slot}`

## 优势

1. **动态适应**：自动适应API返回的可用日期
2. **高效并行**：多个日期数据可并行获取
3. **容错性强**：任务失败可独立重试
4. **资源优化**：通过缓存减少重复请求
5. **扩展性好**：易于添加新的场地或调整日期范围

## 监控点

1. **任务创建数量**：每次主任务执行应创建`week.length - 1`个子任务
2. **缓存命中率**：24小时内重复执行应使用缓存的week数据
3. **子任务成功率**：监控每个日期的数据获取成功率
4. **数据完整性**：确保所有日期的数据都被正确存储

## 调试命令

```bash
# 查看Redis中缓存的week数据
redis-cli get venue:week:1557

# 查看MongoDB中的子任务
mongosh crawler --eval 'db.crawler_tasks.find({parent_id: /venue/})'

# 查看特定日期的场地数据
mongosh crawler --eval 'db.venue_court_periods.find({date: "2025-08-19"})'
```

## 流程图

```
主任务(不带date/week)
    ↓
API返回week数组 + 今天数据
    ↓
    ├─→ 缓存week到Redis (24h)
    ├─→ 保存今天的场地数据
    └─→ 生成子任务
         ├─→ 子任务1 (明天)
         ├─→ 子任务2 (后天)
         └─→ 子任务N (第N天)
              ↓
         各子任务并行执行
              ↓
         保存各日期数据到MongoDB
```