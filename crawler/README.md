# Go Backend Kit - Distributed Crawler System

A highly scalable, distributed web crawler system built with Go, featuring task management, distributed coordination, and flexible data processing pipelines.

## Quick Example

```go
// Create a crawler with minimal configuration
crawler, _ := crawler.New(&crawler.Config{
    ID:        "my-crawler",
    Mode:      crawler.ModeStandalone,
    Workers:   10,
    RedisAddr: "localhost:6379",
})

// Define what to crawl
taskDef := &task.TaskDefinition{
    ID:       "product-crawler",
    Type:     task.TypeDetail,     // Use constants, not strings
    Priority: task.PriorityNormal,  // Type-safe constants
    ExtractRules: []task.ExtractRule{
        {Field: "title", Selector: "h1", Type: "css"},
        {Field: "price", Selector: ".price", Type: "css"},
    },
}

// Start crawling
ctx := context.Background()
crawler.Start(ctx)
crawler.AddSeed("https://example.com/products", nil)
```

## Features

### Core Features
- **Multiple Crawler Types**: Support for seed collection, bulk data crawling, and flexible data processing
- **Distributed Architecture**: Scalable distributed crawler cluster with Redis coordination
- **Task Management**: Comprehensive task definition, registration, and lifecycle management
- **Flexible Configuration**: YAML, JSON, and Lua script configuration support
- **Data Pipeline**: Pluggable data preprocessing, cleaning, and storage components
- **High Performance**: Built with resty HTTP client and optimized for commercial-grade applications

### Architecture Components
- **Task System**: Definition-based task management with templates and validation
- **Fetcher**: High-performance HTTP client with middleware support (rate limiting, caching, retry)
- **Extractor**: HTML, JSON, XPath, and regex data extraction
- **Pipeline**: Multi-stage data processing with custom processors
- **Dispatcher**: Intelligent task distribution with multiple strategies
- **Communication**: gRPC-based node communication with event bus
- **Storage**: Pluggable storage backends (Redis, MongoDB, MySQL)

## Installation

```bash
go get github.com/NHYCRaymond/go-backend-kit/crawler
```

## Quick Start

### Standalone Mode

```go
package main

import (
    "context"
    "github.com/NHYCRaymond/go-backend-kit/crawler"
    "github.com/NHYCRaymond/go-backend-kit/logging"
)

func main() {
    // Initialize logging
    logging.InitLogger(logging.DefaultLoggerConfig())
    
    // Create crawler configuration
    config := &crawler.Config{
        ID:          "my-crawler",
        Name:        "My Web Crawler",
        Mode:        crawler.ModeStandalone,
        Workers:     10,
        RedisAddr:   "localhost:6379",
    }
    
    // Create and start crawler
    c, _ := crawler.New(config)
    ctx := context.Background()
    c.Start(ctx)
    
    // Add seed URLs
    c.AddSeed("https://example.com", nil)
    
    // ... crawling happens ...
    
    c.Stop(ctx)
}
```

### Distributed Mode

```go
// Configure distributed crawler
config := &crawler.Config{
    Mode: crawler.ModeDistributed,
    Node: &distributed.NodeConfig{
        ID:         "node-001",
        MaxWorkers: 20,
    },
    EnableDistributed: true,
}
```

## Task Definition

### Task Constants

The crawler system uses constants for type safety. Always use these constants instead of strings:

```go
// Task Types
task.TypeSeed      // "seed" - Generates more tasks
task.TypeDetail    // "detail" - Detail page crawling
task.TypeList      // "list" - List page crawling  
task.TypeAPI       // "api" - API endpoint crawling
task.TypeBrowser   // "browser" - JavaScript rendering required
task.TypeAggregate // "aggregate" - Aggregate multiple sources

// Task Priorities
task.PriorityLow    // 0
task.PriorityNormal // 1
task.PriorityHigh   // 2
task.PriorityUrgent // 3

// Task Status
task.StatusPending   // "pending"
task.StatusQueued    // "queued"
task.StatusRunning   // "running"
task.StatusCompleted // "completed"
task.StatusFailed    // "failed"
task.StatusRetrying  // "retrying"
task.StatusCancelled // "cancelled"
```

### Creating Task Definitions

```go
taskDef := &task.TaskDefinition{
    ID:       "product-crawler",
    Name:     "Product Crawler",
    Type:     task.TypeDetail,     // Use constant
    Priority: task.PriorityNormal,  // Use constant
    
    // Extraction rules
    ExtractRules: []task.ExtractRule{
        {
            Field:    "title",
            Selector: "h1.product-title",
            Type:     "css",
        },
        {
            Field:    "price",
            Selector: ".price",
            Type:     "css",
        },
    },
    
    // Link generation rules
    LinkRules: []task.LinkRule{
        {
            Name:     "related",
            Selector: ".related a",
            Type:     "css",
            TaskType: task.TypeDetail,  // Use constant
        },
    },
}

// Register definition
registry.RegisterDefinition(taskDef)
```

### Using Lua Scripts

Lua scripts have access to all task constants automatically. The ConfigLoader registers these constants in the Lua environment:

```lua
-- tasks.lua
-- Constants are automatically available (no need to define them)
-- TYPE_SEED, TYPE_DETAIL, TYPE_LIST, TYPE_API, TYPE_BROWSER, TYPE_AGGREGATE
-- PRIORITY_LOW, PRIORITY_NORMAL, PRIORITY_HIGH, PRIORITY_URGENT
-- STATUS_PENDING, STATUS_QUEUED, STATUS_RUNNING, STATUS_COMPLETED, etc.

tasks.product_crawler = {
    id = "product-crawler",
    name = "Product Crawler",
    type = TYPE_DETAIL,      -- Use constant (automatically available)
    priority = PRIORITY_NORMAL,  -- Use constant (automatically available)
    extract_rules = {
        {field = "title", selector = "h1", type = "css"},
        {field = "price", selector = ".price", type = "css"}
    },
    link_rules = {
        {
            name = "related",
            selector = ".related a",
            type = "css",
            task_type = TYPE_DETAIL,  -- Use constant
            priority = PRIORITY_LOW    -- Use constant
        }
    }
}

-- Register with crawler
crawler.create_definition(tasks.product_crawler)

-- You can also use constants in control flow
if task.status == STATUS_COMPLETED then
    log("info", "Task completed successfully")
elseif task.status == STATUS_FAILED then
    log("error", "Task failed")
end
```

## Data Pipeline

### Creating Custom Processors

```go
type PriceProcessor struct{}

func (p *PriceProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
    if m, ok := data.(map[string]interface{}); ok {
        // Extract and normalize price
        if price, ok := m["price"].(string); ok {
            m["price_normalized"] = normalizePrice(price)
        }
    }
    return data, nil
}

// Add to pipeline
pipeline.AddProcessor(&PriceProcessor{})
```

## Middleware System

### Request Middleware

```go
// Rate limiting
rateLimiter := &fetcher.RateLimiterMiddleware{
    RequestsPerSecond: 10,
}

// Caching
cache := &fetcher.CacheMiddleware{
    TTL:      time.Hour,
    MaxSize:  100 * 1024 * 1024, // 100MB
}

// Add to fetcher
fetcher.WithMiddleware(rateLimiter, cache)
```

## Configuration

### YAML Configuration

```yaml
# config.yaml
id: "production-crawler"
mode: "distributed"
workers: 20

fetcher:
  timeout: "30s"
  rate_limit: 10
  
pipeline:
  concurrency: 10
  processors:
    - name: "cleaner"
      enabled: true
    - name: "validator"
      enabled: true
```

### Environment Variables

```bash
export CRAWLER_REDIS_ADDR=localhost:6379
export CRAWLER_LOG_LEVEL=debug
export CRAWLER_MAX_WORKERS=50
```

## Monitoring

### Metrics

```go
metrics := crawler.GetMetrics()
fmt.Printf("Requests: %d, Success: %d, Failed: %d\n",
    metrics.RequestsTotal,
    metrics.RequestsSuccess,
    metrics.RequestsFailed)
```

### Events

```go
// Subscribe to events
eventBus.Subscribe(EventTypeTaskCompleted, func(event *Event) {
    fmt.Printf("Task completed: %s\n", event.Data)
})
```

## Advanced Features

### Proxy Pool

```go
proxyPool := &fetcher.ProxyPoolConfig{
    Proxies: []fetcher.ProxyConfig{
        {URL: "http://proxy1:8080"},
        {URL: "http://proxy2:8080"},
    },
    Strategy: fetcher.ProxyStrategyRoundRobin,
}
```

### Browser Mode (JavaScript Rendering)

```go
task := &task.Task{
    Type: task.TypeBrowser,
    // ... other fields
}
```

### Deduplication

```go
// Bloom filter deduplication
dedup := &pipeline.DeduplicatorProcessor{
    Strategy:  "bloom_filter",
    Capacity:  1000000,
    ErrorRate: 0.001,
}
```

## Constants Reference

### Task Types
| Constant | Value | Description |
|----------|-------|-------------|
| `task.TypeSeed` | "seed" | Seed URLs that generate more tasks |
| `task.TypeDetail` | "detail" | Detail page crawling |
| `task.TypeList` | "list" | List/index page crawling |
| `task.TypeAPI` | "api" | API endpoint crawling |
| `task.TypeBrowser` | "browser" | Browser-based crawling with JS rendering |
| `task.TypeAggregate` | "aggregate" | Aggregate data from multiple sources |

### Task Priorities
| Constant | Value | Description |
|----------|-------|-------------|
| `task.PriorityLow` | 0 | Low priority tasks |
| `task.PriorityNormal` | 1 | Normal priority (default) |
| `task.PriorityHigh` | 2 | High priority tasks |
| `task.PriorityUrgent` | 3 | Urgent tasks (processed first) |

### Task Status
| Constant | Value | Description |
|----------|-------|-------------|
| `task.StatusPending` | "pending" | Task created but not queued |
| `task.StatusQueued` | "queued" | Task in queue waiting for execution |
| `task.StatusRunning` | "running" | Task currently being executed |
| `task.StatusCompleted` | "completed" | Task successfully completed |
| `task.StatusFailed` | "failed" | Task failed after all retries |
| `task.StatusRetrying` | "retrying" | Task failed, will retry |
| `task.StatusCancelled` | "cancelled" | Task cancelled by user |

### Dispatch Strategies
| Constant | Value | Description |
|----------|-------|-------------|
| `task.StrategyRoundRobin` | "round_robin" | Distribute tasks evenly |
| `task.StrategyLeastLoad` | "least_load" | Send to least loaded node |
| `task.StrategyRandom` | "random" | Random distribution |
| `task.StrategyHash` | "hash" | Hash-based distribution |
| `task.StrategySticky` | "sticky" | Stick to same node |

## Best Practices

1. **Use Constants**: Always use predefined constants instead of string literals for type safety
2. **Task Definition**: Use task definitions and templates for reusable crawling patterns
3. **Rate Limiting**: Always configure appropriate rate limits to avoid being blocked
4. **Error Handling**: Implement proper retry logic with exponential backoff
5. **Data Validation**: Use pipeline processors to validate and clean extracted data
6. **Monitoring**: Enable metrics and logging for production deployments
7. **Resource Management**: Configure appropriate worker counts and memory limits

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                     TUI Management                       │
└─────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│              Communication Hub (gRPC)                    │
├──────────────────┬──────────────────┬──────────────────┤
│   Event Bus      │  Node Manager    │  Task Registry   │
└──────────────────┴──────────────────┴──────────────────┘
                              │
            ┌─────────────────┼─────────────────┐
            │                 │                 │
            ▼                 ▼                 ▼
┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐
│  Crawler Node 1  │ │  Crawler Node 2  │ │  Crawler Node N  │
├──────────────────┤ ├──────────────────┤ ├──────────────────┤
│  Task Executor   │ │  Task Executor   │ │  Task Executor   │
│  Fetcher         │ │  Fetcher         │ │  Fetcher         │
│  Extractor       │ │  Extractor       │ │  Extractor       │
│  Pipeline        │ │  Pipeline        │ │  Pipeline        │
└──────────────────┘ └──────────────────┘ └──────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│                    Storage Layer                         │
├──────────────────┬──────────────────┬──────────────────┤
│      Redis       │     MongoDB      │      MySQL       │
└──────────────────┴──────────────────┴──────────────────┘
```

## Examples

See the [examples](../examples/crawler) directory for complete examples:
- `main.go` - Basic crawler usage
- `tasks.lua` - Lua script configuration
- `config.yaml` - YAML configuration

## Performance Tuning

### Connection Pool
```go
config.MaxIdleConns = 100
config.MaxIdleConnsPerHost = 10
config.IdleConnTimeout = 90 * time.Second
```

### Memory Management
```go
config.MaxMemoryMB = 2048
config.GCInterval = time.Minute
```

### Concurrency
```go
config.Workers = 50
config.MaxConcurrent = 200
config.ChannelBufferSize = 1000
```

## Troubleshooting

### Common Issues

1. **High Memory Usage**: Reduce worker count or enable memory limits
2. **Slow Performance**: Check rate limits and network latency
3. **Task Failures**: Review retry configuration and error logs
4. **Data Loss**: Ensure proper storage configuration and persistence

### Debug Mode

```go
config.LogLevel = "debug"
config.EnableProfiling = true
```

## Contributing

Please see the main [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.

## License

This project is part of the Go Backend Kit and follows the same license.