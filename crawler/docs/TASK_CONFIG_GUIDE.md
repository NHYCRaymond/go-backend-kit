# 爬虫任务配置指南

## 概述

爬虫系统支持灵活的任务配置，每个任务可以有独立的解析规则、数据处理流程和存储方案。通过配置文件、数据库或 Lua 脚本，可以定义不同类型的爬取任务。

## 1. 任务配置架构

```
任务定义 (TaskDocument)
├── 基本信息 (name, type, category)
├── 请求配置 (RequestTemplate)
├── 提取配置 (ExtractionConfig)
├── 转换配置 (TransformConfig)
├── 验证配置 (ValidationConfig)
└── 存储配置 (StorageConfiguration)
```

## 2. 配置方式

### 2.1 YAML 配置文件

```yaml
# config/tasks/product_crawler.yaml
type: definition
definition:
  name: "电商商品爬虫"
  type: "http"
  category: "e-commerce"
  description: "爬取电商网站商品信息"
  version: "1.0.0"
  
  # 请求配置
  request:
    method: "GET"
    url: "https://example.com/product/{{product_id}}"
    headers:
      User-Agent: "Mozilla/5.0..."
      Accept: "text/html,application/json"
    variables:
      - name: product_id
        type: string
        source: context
      # 日期变量示例（新增）
      - name: DATE
        type: date
        source: function
        expression: "${DATE}"
        strategy:
          mode: single    # single/range/custom
          format: "2006-01-02"
          start_days: 0   # 今天
    pagination:
      enabled: true
      type: page
      start_param: page
      limit_param: size
      page_size: 20
      max_pages: 100
  
  # 数据提取配置
  extraction:
    type: html
    rules:
      # 商品标题
      - field: title
        path: "h1.product-title"
        type: text
        required: true
        
      # 商品价格
      - field: price
        path: ".price-now span"
        type: number
        required: true
        transform:
          - type: replace
            config:
              pattern: "[￥,]"
              replacement: ""
          - type: convert
            config:
              to: float
        validation:
          - type: range
            min: 0
            max: 999999
            
      # 商品图片
      - field: images
        path: ".product-images img"
        type: array
        transform:
          - type: extract_attr
            config:
              attr: src
          - type: absolute_url
            config:
              base: "https://example.com"
              
      # 商品描述
      - field: description
        path: ".product-desc"
        type: html
        transform:
          - type: clean_html
            config:
              remove_tags: ["script", "style"]
              keep_tags: ["p", "ul", "li"]
              
      # 商品规格（嵌套结构）
      - field: specifications
        path: ".spec-list li"
        type: object
        children:
          - field: name
            path: ".spec-name"
            type: text
          - field: value
            path: ".spec-value"
            type: text
            
      # 商品评价
      - field: reviews
        path: ".review-item"
        type: array
        children:
          - field: rating
            path: ".rating"
            type: number
          - field: content
            path: ".review-content"
            type: text
          - field: date
            path: ".review-date"
            type: date
            transform:
              - type: parse_date
                config:
                  format: "2006-01-02"
  
  # 数据转换配置
  transform:
    enabled: true
    processors:
      # 清洗处理器
      - name: cleaner
        type: clean
        config:
          trim_space: true
          remove_empty: true
          normalize_space: true
          
      # 数据增强
      - name: enricher
        type: enrich
        config:
          add_fields:
            crawled_at: "{{now}}"
            source: "example.com"
            
      # 去重处理
      - name: deduplicator
        type: dedupe
        config:
          fields: ["url", "title"]
          strategy: hash
  
  # 数据验证配置
  validation:
    enabled: true
    rules:
      - field: title
        required: true
        min_length: 5
        max_length: 200
      - field: price
        required: true
        type: number
        min: 0
      - field: images
        min_items: 1
        max_items: 10
  
  # 存储配置
  storage:
    type: mongodb
    database: e_commerce
    collection: products
    indexes:
      - fields: ["product_id"]
        unique: true
      - fields: ["category", "price"]
    ttl: 604800  # 7 days
```

### 2.2 JSON 配置（API 爬虫示例）

```json
{
  "type": "definition",
  "definition": {
    "name": "GitHub API 爬虫",
    "type": "api",
    "category": "developer",
    
    "request": {
      "method": "GET",
      "url": "https://api.github.com/repos/{{owner}}/{{repo}}/issues",
      "headers": {
        "Accept": "application/vnd.github.v3+json",
        "Authorization": "Bearer {{github_token}}"
      },
      "variables": [
        {
          "name": "github_token",
          "source": "env",
          "expression": "GITHUB_TOKEN"
        }
      ],
      "pagination": {
        "enabled": true,
        "type": "page",
        "start_param": "page",
        "limit_param": "per_page",
        "page_size": 100
      }
    },
    
    "extraction": {
      "type": "json",
      "rules": [
        {
          "field": "issues",
          "path": "$",
          "type": "array",
          "children": [
            {
              "field": "id",
              "path": "$.id",
              "type": "number"
            },
            {
              "field": "title",
              "path": "$.title",
              "type": "text"
            },
            {
              "field": "state",
              "path": "$.state",
              "type": "text"
            },
            {
              "field": "labels",
              "path": "$.labels[*].name",
              "type": "array"
            },
            {
              "field": "created_at",
              "path": "$.created_at",
              "type": "date"
            }
          ]
        }
      ]
    },
    
    "storage": {
      "type": "mysql",
      "database": "github_data",
      "table": "issues",
      "upsert": true,
      "key_fields": ["id"]
    }
  }
}
```

### 2.3 Lua 脚本配置（动态爬虫）

```lua
-- scripts/dynamic_crawler.lua
local task = {
    name = "动态内容爬虫",
    type = "browser",  -- 使用浏览器引擎
    category = "dynamic",
    
    -- 初始化函数
    init = function(context)
        -- 设置浏览器选项
        context:setBrowserOptions({
            headless = true,
            javascript = true,
            wait_time = 3000
        })
    end,
    
    -- 请求生成函数
    request = function(context)
        local url = context:getVariable("url")
        return {
            method = "GET",
            url = url,
            wait_for = ".content-loaded",  -- 等待元素加载
            scroll = true,  -- 自动滚动加载更多
            actions = {
                -- 点击"加载更多"按钮
                {type = "click", selector = ".load-more", times = 3},
                -- 等待内容加载
                {type = "wait", duration = 2000}
            }
        }
    end,
    
    -- 数据提取函数
    extract = function(response, context)
        local data = {}
        
        -- 使用 CSS 选择器提取
        data.title = response:querySelector("h1"):text()
        
        -- 提取列表数据
        data.items = {}
        local items = response:querySelectorAll(".item")
        for _, item in ipairs(items) do
            table.insert(data.items, {
                name = item:querySelector(".name"):text(),
                price = item:querySelector(".price"):number(),
                image = item:querySelector("img"):attr("src")
            })
        end
        
        -- 提取动态加载的数据
        data.comments = response:evaluateJS([[
            return Array.from(document.querySelectorAll('.comment')).map(c => ({
                author: c.querySelector('.author').innerText,
                content: c.querySelector('.content').innerText,
                date: c.querySelector('.date').innerText
            }))
        ]])
        
        return data
    end,
    
    -- 数据处理函数
    transform = function(data, context)
        -- 价格转换
        if data.items then
            for _, item in ipairs(data.items) do
                item.price = item.price * 100  -- 转为分
            end
        end
        
        -- 添加元数据
        data.crawled_at = os.time()
        data.source = context:getVariable("source")
        
        return data
    end,
    
    -- 存储函数
    store = function(data, context)
        -- 自定义存储逻辑
        local storage = context:getStorage("mongodb")
        storage:upsert("products", {url = data.url}, data)
        
        -- 发送到消息队列
        local mq = context:getMessageQueue("rabbitmq")
        mq:publish("products.new", data)
        
        return true
    end,
    
    -- 错误处理
    onError = function(error, context)
        context:log("error", "Task failed: " .. error.message)
        
        -- 重试逻辑
        if error.type == "network" and context:getRetryCount() < 3 then
            context:retry(5000)  -- 5秒后重试
        else
            context:fail(error.message)
        end
    end
}

return task
```

### 2.4 数据库配置（MongoDB 存储）

```javascript
// 在 MongoDB 中存储任务配置
db.task_definitions.insertOne({
  name: "新闻爬虫",
  type: "http",
  category: "news",
  
  // 多个新闻源配置
  sources: [
    {
      name: "新浪新闻",
      url: "https://news.sina.com.cn/",
      extraction: {
        rules: [
          {field: "title", path: "h2.news-title"},
          {field: "content", path: ".news-content"},
          {field: "time", path: ".time", type: "date"}
        ]
      }
    },
    {
      name: "腾讯新闻",
      url: "https://news.qq.com/",
      extraction: {
        rules: [
          {field: "title", path: "h1.title"},
          {field: "content", path: ".content-article"},
          {field: "time", path: ".pubtime", type: "date"}
        ]
      }
    }
  ],
  
  // 通用处理配置
  transform: {
    processors: [
      {type: "clean", config: {remove_ads: true}},
      {type: "extract_keywords", config: {count: 10}},
      {type: "sentiment_analysis", config: {model: "chinese"}}
    ]
  },
  
  storage: {
    type: "elasticsearch",
    index: "news",
    pipeline: "news_processing"
  }
});
```

## 3. 任务模板系统

### 3.1 定义模板

```yaml
# templates/e-commerce-template.yaml
template:
  id: "e-commerce-basic"
  name: "电商基础模板"
  description: "适用于大多数电商网站的通用模板"
  
  # 模板变量
  variables:
    - name: base_url
      type: string
      required: true
      description: "网站基础URL"
    - name: product_list_selector
      type: string
      default: ".product-list .item"
    - name: title_selector
      type: string
      default: ".title"
    - name: price_selector
      type: string
      default: ".price"
  
  # 可覆盖的提取规则
  extraction:
    type: html
    rules:
      - field: title
        path: "{{title_selector}}"
        type: text
      - field: price
        path: "{{price_selector}}"
        type: number
```

### 3.2 使用模板

```yaml
# 基于模板创建具体任务
type: instance
template: "e-commerce-basic"
variables:
  base_url: "https://shop.example.com"
  product_list_selector: "div.products article"
  title_selector: "h3.product-name"
  price_selector: "span.current-price"

# 覆盖或扩展规则
extraction_overrides:
  rules:
    - field: discount
      path: ".discount-badge"
      type: text
    - field: rating
      path: ".star-rating"
      type: number
```

## 4. 规则继承和组合

### 4.1 规则继承

```yaml
# 基础规则
base_rules:
  id: "base-product"
  rules:
    - field: url
      type: current_url
    - field: timestamp
      type: current_time

# 继承并扩展
extraction:
  inherit: "base-product"
  rules:
    - field: title
      path: "h1"
      type: text
    - field: price
      path: ".price"
      type: number
```

### 4.2 规则组合

```yaml
# 组合多个规则集
extraction:
  combine:
    - ref: "common/metadata"     # 通用元数据
    - ref: "product/basic"        # 商品基础信息
    - ref: "product/images"       # 图片提取规则
    - ref: "product/reviews"      # 评论提取规则
```

## 5. 条件化配置

```yaml
# 根据条件选择不同的提取规则
extraction:
  conditions:
    - when: "response.url contains '/mobile/'"
      rules:
        - field: title
          path: ".m-title"
          type: text
    - when: "response.status == 200"
      rules:
        - field: title
          path: "h1.title"
          type: text
    - default:
      rules:
        - field: error
          value: "Failed to extract"
```

## 6. 自定义处理器

```go
// 注册自定义处理器
crawler.RegisterProcessor("custom-price", func(data interface{}) (interface{}, error) {
    // 自定义价格处理逻辑
    price := data.(string)
    // 处理各种价格格式
    // ￥99.99 -> 9999
    // $99.99 -> 9999
    // 99.99元 -> 9999
    return parsePrice(price), nil
})
```

## 7. 日期变量策略（新增）

### 7.1 日期变量配置

日期变量支持灵活的策略配置，适用于需要按日期批量爬取的场景：

```yaml
variables:
  - name: DATE
    type: date
    source: function
    expression: "${DATE}"  # 在请求中使用 ${DATE} 占位符
    strategy:
      mode: single       # 模式选择
      format: "2006-01-02"  # Go的日期格式
      start_days: 0      # 相对今天的偏移天数
```

### 7.2 策略模式说明

#### Single 模式（单个日期）
```yaml
strategy:
  mode: single
  format: "2006-01-02"
  start_days: 0  # 0表示今天，-1表示昨天，1表示明天
```

#### Range 模式（日期范围）
```yaml
strategy:
  mode: range
  format: "2006-01-02"
  start_days: -7   # 从7天前开始
  end_days: 0      # 到今天结束
  day_step: 1      # 每天一个任务
```

#### Custom 模式（自定义日期列表）
```yaml
strategy:
  mode: custom
  format: "2006-01-02"
  custom_days: [-7, -3, -1, 0, 1, 3, 7]  # 特定的日期偏移
```

### 7.3 使用示例

#### 体育赛事数据爬取（当天）
```yaml
variables:
  - name: DATE
    type: date
    source: function
    expression: "${DATE}"
    strategy:
      mode: single
      format: "20060102"  # YYYYMMDD格式
      start_days: 0       # 只爬取当天
```

#### 历史数据补充（最近一周）
```yaml
variables:
  - name: DATE
    type: date
    source: function
    expression: "${DATE}"
    strategy:
      mode: range
      format: "2006-01-02"
      start_days: -6    # 6天前
      end_days: 0       # 到今天
      day_step: 1       # 每天
```

#### 周报数据采集（每周一次）
```yaml
variables:
  - name: DATE
    type: date
    source: function
    expression: "${DATE}"
    strategy:
      mode: range
      format: "2006-01-02"
      start_days: -28   # 4周前
      end_days: 0       # 到今天
      day_step: 7       # 每7天（每周）
```

### 7.4 向后兼容

如果未指定 strategy，系统会使用默认策略（向后兼容）：
- 自动检测请求体中的 ${DATE} 变量
- 生成未来7天的任务

## 8. 配置热更新

```go
// 监听配置变化
loader := task.NewConfigLoader(&task.LoaderConfig{
    ConfigPaths: []string{"./configs/tasks"},
    WatchFiles: true,  // 启用文件监听
})

// 配置变化时自动重载
loader.OnChange(func(taskID string, config *task.TaskConfig) {
    logger.Info("Task configuration updated", "task_id", taskID)
    // 重新加载任务配置
    crawler.ReloadTask(taskID, config)
})
```

## 8. 最佳实践

### 8.1 配置组织结构

```
configs/
├── tasks/                    # 任务定义
│   ├── e-commerce/
│   │   ├── amazon.yaml
│   │   ├── ebay.yaml
│   │   └── taobao.yaml
│   ├── news/
│   │   ├── sina.yaml
│   │   └── qq.yaml
│   └── social/
│       ├── twitter.yaml
│       └── weibo.yaml
├── templates/                # 通用模板
│   ├── e-commerce.yaml
│   ├── news.yaml
│   └── forum.yaml
├── rules/                    # 可复用规则
│   ├── common/
│   │   ├── metadata.yaml
│   │   └── pagination.yaml
│   └── extractors/
│       ├── product.yaml
│       └── article.yaml
└── scripts/                  # Lua脚本
    ├── processors/
    └── validators/
```

### 8.2 配置版本管理

```yaml
# 配置版本控制
version: "2.0.0"
compatible_versions: ["1.8", "1.9", "2.0"]
deprecated_fields:
  - field: "old_field"
    replacement: "new_field"
    removed_in: "3.0.0"
```

### 8.3 配置测试

```yaml
# 测试用例
tests:
  - name: "商品价格提取测试"
    input:
      html: '<div class="price">￥99.99</div>'
    expected:
      price: 99.99
  - name: "分页测试"
    input:
      url: "https://example.com/list?page=1"
    expected:
      next_page: "https://example.com/list?page=2"
```

## 9. 配置示例库

### 9.1 电商商品爬虫
- Amazon 商品爬虫
- 淘宝商品爬虫
- 京东商品爬虫

### 9.2 新闻内容爬虫
- 新闻列表爬虫
- 新闻详情爬虫
- 新闻评论爬虫

### 9.3 社交媒体爬虫
- Twitter 爬虫
- 微博爬虫
- Instagram 爬虫

### 9.4 API 数据爬虫
- RESTful API 爬虫
- GraphQL 爬虫
- WebSocket 爬虫

通过这套灵活的配置系统，可以轻松应对各种不同的爬取需求，无需修改代码即可适配新的网站和数据结构。