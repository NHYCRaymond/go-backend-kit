# 爬虫数据处理流程

## 概述

爬虫采集的数据经过以下几个阶段处理：
1. **Fetching** - 获取网页内容
2. **Extraction** - 提取结构化数据
3. **Pipeline Processing** - 数据清洗和转换
4. **Storage** - 持久化存储到 MongoDB/MySQL/Redis

## 详细流程

### 1. 任务执行 (Executor)

```go
// executor/executor.go
func (e *Executor) Execute(ctx context.Context, t *task.Task) (*task.TaskResult, error)
```

执行流程：
1. **Fetch** - 通过 Fetcher 获取网页内容
2. **Extract** - 使用 ExtractRules 提取数据
3. **Pipeline** - 通过 Pipeline 处理数据
4. **Storage** - 保存到配置的存储后端

### 2. 数据提取 (Extraction)

提取器支持多种方式：
- **CSS Selector** - 使用 CSS 选择器提取
- **XPath** - 使用 XPath 表达式
- **JSON Path** - 处理 JSON 响应
- **Regex** - 正则表达式匹配

```go
// 提取规则示例
ExtractRules: []ExtractRule{
    {
        Field:    "title",
        Selector: "h1.title",
        Type:     "text",
    },
    {
        Field:    "price",
        Selector: ".price span",
        Type:     "number",
    },
}
```

### 3. Pipeline 处理

Pipeline 是一系列数据处理器的链式调用：

#### 3.1 清洗器 (CleanerProcessor)
```go
// 清洗配置
config := CleanerConfig{
    TrimSpace:      true,  // 去除空格
    RemoveEmpty:    true,  // 移除空值
    NormalizeSpace: true,  // 规范化空格
    ToLower:        false, // 转小写
}
```

功能：
- 去除前后空格
- 规范化空白字符
- 移除空字段
- 大小写转换

#### 3.2 验证器 (ValidatorProcessor)
```go
// 验证规则
rules := []ValidationRule{
    {
        Field:    "price",
        Required: true,
        Type:     "number",
        Min:      0,
        Max:      999999,
    },
}
```

功能：
- 必填字段检查
- 数据类型验证
- 范围验证
- 格式验证（email、URL等）

#### 3.3 转换器 (TransformerProcessor)
```go
// 数据转换
transformer := TransformerProcessor{
    Transformations: []Transformation{
        {
            Field: "price",
            Type:  "multiply",
            Value: 100, // 转换为分
        },
    },
}
```

功能：
- 字段重命名
- 数据类型转换
- 值映射
- 计算字段

#### 3.4 去重器 (DeduplicatorProcessor)
```go
// 去重配置
dedup := DeduplicatorProcessor{
    Fields: []string{"url", "title"},
    Strategy: "hash", // 或 "exact"
}
```

功能：
- 基于字段的去重
- Hash 去重
- 精确匹配去重

### 4. 存储层 (Storage)

#### 4.1 MongoDB 存储

```go
// 存储到 MongoDB
storage := NewMongoStorage(db, StorageConfig{
    Database: "crawler",
    Table:    "products",
    TTL:      24 * time.Hour,
})

// 保存的数据结构
dataWithMeta := map[string]interface{}{
    "task_id":   "task-123",
    "parent_id": "parent-456",
    "url":       "https://example.com/product",
    "timestamp": time.Now(),
    "data": {
        "title": "Product Name",
        "price": 99.99,
        "description": "...",
    },
}
```

MongoDB 特性：
- 自动创建 TTL 索引
- 支持复杂嵌套结构
- 原子性更新
- 灵活的查询

#### 4.2 MySQL 存储

```go
// 存储到 MySQL
storage := NewMySQLStorage(db, StorageConfig{
    Database: "crawler",
    Table:    "products",
})
```

MySQL 特性：
- 结构化存储
- 事务支持
- 关系型查询
- JSON 字段支持

#### 4.3 Redis 存储

```go
// 存储到 Redis
storage := NewRedisStorage(redis, StorageConfig{
    Prefix: "crawler:data",
    TTL:    1 * time.Hour,
})
```

Redis 特性：
- 高速缓存
- 自动过期
- 发布订阅
- 去重支持

### 5. 数据流示例

```
1. 爬取商品页面
   ↓
2. 提取数据
   - title: "iPhone 15 Pro"
   - price: "$999"
   - description: "..."
   ↓
3. Pipeline 处理
   a. 清洗：去除 $ 符号，trim 空格
   b. 验证：确保 price 是数字
   c. 转换：price 转为数值类型
   d. 去重：基于 URL 去重
   ↓
4. 存储到 MongoDB
   {
     "_id": "65f3a2b1...",
     "task_id": "task-123",
     "url": "https://...",
     "timestamp": "2024-03-15T10:30:00Z",
     "data": {
       "title": "iPhone 15 Pro",
       "price": 999,
       "description": "..."
     }
   }
```

### 6. 分布式处理

在分布式模式下，数据处理流程增加了以下步骤：

1. **任务结果发布**
   - Node 完成任务后，结果发布到 Redis PubSub
   - Coordinator 订阅结果 channel

2. **结果聚合**
   - Coordinator 收集多个 Node 的结果
   - 合并和去重处理
   - 统一存储到主数据库

3. **增量任务生成**
   - 从提取的 links 生成新任务
   - 任务去重和优先级分配
   - 分发到任务队列

### 7. 数据质量保证

#### 7.1 数据验证
- Schema 验证
- 必填字段检查
- 数据类型检查
- 范围和格式验证

#### 7.2 错误处理
- 部分失败重试
- 错误数据隔离
- 异常告警

#### 7.3 数据监控
- 提取成功率
- 数据完整性
- 异常值检测
- 重复率统计

### 8. 配置示例

```yaml
# 完整的数据处理配置
task:
  type: "detail"
  url: "https://example.com/product/123"
  
  # 提取规则
  extract_rules:
    - field: "title"
      selector: "h1.product-title"
      type: "text"
    - field: "price"
      selector: ".price-now"
      type: "number"
    - field: "images"
      selector: ".product-images img"
      type: "attr"
      attr: "src"
      multiple: true
  
  # Pipeline 配置
  pipeline_id: "product_pipeline"
  
  # 存储配置
  storage:
    type: "mongodb"
    database: "ecommerce"
    collection: "products"
    ttl: "168h"  # 7 days

# Pipeline 定义
pipeline:
  name: "product_pipeline"
  processors:
    - type: "cleaner"
      config:
        trim_space: true
        remove_empty: true
    - type: "validator"
      rules:
        - field: "price"
          required: true
          type: "number"
          min: 0
    - type: "transformer"
      transformations:
        - field: "price"
          type: "multiply"
          value: 100  # 转为分
    - type: "deduplicator"
      fields: ["url"]
```

### 9. 性能优化

1. **批量处理**
   - 批量写入数据库
   - 批量验证和转换
   - 减少网络开销

2. **并发控制**
   - Pipeline 并发处理
   - 存储连接池
   - 限流和背压

3. **缓存策略**
   - 去重缓存
   - 结果缓存
   - 规则缓存

### 10. 扩展点

系统提供了多个扩展接口：

1. **自定义 Processor**
```go
type CustomProcessor struct {
    *BaseProcessor
}

func (p *CustomProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
    // 自定义处理逻辑
    return data, nil
}
```

2. **自定义 Storage**
```go
type CustomStorage struct {
    // 自定义存储实现
}

func (s *CustomStorage) Save(ctx context.Context, data interface{}) error {
    // 自定义存储逻辑
    return nil
}
```

3. **自定义 Extractor**
```go
type CustomExtractor struct {
    // 自定义提取器
}

func (e *CustomExtractor) Extract(ctx context.Context, response *Response, rules []Rule) (map[string]interface{}, error) {
    // 自定义提取逻辑
    return data, nil
}
```

这个架构设计保证了数据处理的灵活性、可扩展性和高性能。