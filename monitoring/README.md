# Go Backend Kit - 监控系统

本项目提供了完整的监控解决方案，包含Prometheus指标收集、Grafana可视化和SLA监控。

## 核心问题解决方案

### 1. API响应耗时监控

**问题**: 如何准确监控每个API的响应耗时？

**解决方案**: 使用专门的Gin中间件 `GinMetricsMiddleware`

```go
// 配置监控中间件
metricsConfig := &monitoring.MetricsConfig{
    SLAThresholds: map[string]float64{
        "/api/v1/users":         0.5,  // 500ms SLA
        "/api/v1/user/profile":  0.3,  // 300ms SLA
        "/api/v1/orders":        1.0,  // 1s SLA
    },
    SkipPaths: []string{
        "/health", "/metrics", "/ready", "/live",
    },
    PathGrouping: map[string]string{
        `/api/v1/users/\d+`: "/api/v1/users/:id",  // 路径参数归组
    },
}

// 在Gin中使用
router.Use(monitoring.GinMetricsMiddleware(metricsConfig))
```

### 2. 关键监控指标

#### HTTP请求指标
- `gin_requests_total` - 总请求数 (按method, route, status_code分组)
- `gin_request_duration_seconds` - 请求耗时直方图
- `gin_request_size_bytes` - 请求体大小
- `gin_response_size_bytes` - 响应体大小
- `gin_active_requests` - 当前活跃请求数

#### API端点指标
- `api_endpoint_requests_total` - API端点请求数
- `api_endpoint_duration_seconds` - API端点响应时间
- `sla_violations_total` - SLA违规次数

#### 系统指标
- `cpu_usage_percent` - CPU使用率
- `memory_usage_bytes` - 内存使用量
- `goroutines_total` - Goroutine数量
- `gc_duration_seconds` - GC耗时

#### 中间件指标
- `authentication_attempts_total` - 认证尝试次数
- `rate_limit_hits_total` - 限流命中次数
- `cache_hits_total` / `cache_misses_total` - 缓存命中/未命中

#### 业务指标
- `user_registrations_total` - 用户注册数
- `user_logins_total` - 用户登录数
- `active_users` - 活跃用户数
- `errors_total` - 错误总数

## 使用方法

### 1. 基本集成

```go
import (
    "github.com/NHYCRaymond/go-backend-kit/monitoring"
    "github.com/gin-gonic/gin"
)

func main() {
    router := gin.New()
    
    // 1. 首先添加监控中间件
    router.Use(monitoring.GinMetricsMiddleware(nil)) // 使用默认配置
    
    // 2. 添加其他中间件
    router.Use(gin.Logger())
    router.Use(gin.Recovery())
    
    // 3. 启动监控服务器
    monitoring.StartServer(&config.MonitoringConfig{
        Port: 9090,
        Path: "/metrics",
    }, logger)
}
```

### 2. 自定义配置

```go
config := &monitoring.MetricsConfig{
    SLAThresholds: map[string]float64{
        "/api/v1/users":    0.5,  // 500ms
        "/api/v1/orders":   1.0,  // 1s
    },
    SkipPaths: []string{"/health", "/metrics"},
    PathGrouping: map[string]string{
        `/api/v1/users/\d+`: "/api/v1/users/:id",
    },
}

router.Use(monitoring.GinMetricsMiddleware(config))
```

### 3. 记录业务指标

```go
// 在业务逻辑中记录指标
func loginHandler(c *gin.Context) {
    // 业务逻辑
    success := performLogin(c)
    
    if success {
        monitoring.RecordUserLogin("success")
    } else {
        monitoring.RecordUserLogin("failed")
    }
}

func registerHandler(c *gin.Context) {
    // 业务逻辑
    performRegistration(c)
    
    monitoring.RecordUserRegistration()
}
```

## Prometheus查询示例

### 1. API响应时间监控

```promql
# 95th百分位响应时间
histogram_quantile(0.95, rate(gin_request_duration_seconds_bucket[5m]))

# 按路由分组的平均响应时间
rate(gin_request_duration_seconds_sum[5m]) / rate(gin_request_duration_seconds_count[5m])

# 慢请求率（>1秒）
rate(gin_request_duration_seconds_bucket{le="1"}[5m])
```

### 2. 错误率监控

```promql
# 总错误率
rate(gin_requests_total{status_code=~"5.."}[5m]) / rate(gin_requests_total[5m])

# 按端点分组的错误率
rate(gin_requests_total{status_code=~"5..", route="/api/v1/users"}[5m])
```

### 3. SLA监控

```promql
# SLA违规率
rate(sla_violations_total[5m])

# 按端点分组的SLA违规
rate(sla_violations_total{endpoint="/api/v1/users"}[5m])
```

### 4. 系统资源监控

```promql
# 内存使用率
memory_usage_bytes / memory_total_bytes * 100

# Goroutine数量趋势
goroutines_total

# 活跃请求数
gin_active_requests
```

## Grafana集成

### 1. 导入仪表板

使用提供的 `monitoring/grafana/dashboard.json` 文件导入预配置的仪表板。

### 2. 关键面板

1. **HTTP请求率** - 实时请求量
2. **响应时间分布** - 95th/50th百分位
3. **错误率趋势** - 4xx/5xx错误
4. **SLA违规监控** - 超时请求追踪
5. **系统资源使用** - CPU/内存/Goroutine
6. **业务指标** - 用户注册/登录/活跃用户

## 告警规则

### 1. 高响应时间告警

```yaml
- alert: HighResponseTime
  expr: histogram_quantile(0.95, rate(gin_request_duration_seconds_bucket[5m])) > 1
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "High response time detected"
```

### 2. 高错误率告警

```yaml
- alert: HighErrorRate
  expr: rate(gin_requests_total{status_code=~"5.."}[5m]) / rate(gin_requests_total[5m]) > 0.05
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "High error rate detected"
```

### 3. SLA违规告警

```yaml
- alert: SLAViolation
  expr: rate(sla_violations_total[5m]) > 0.01
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "SLA violation detected"
```

## 最佳实践

### 1. 标签基数控制

- 使用 `PathGrouping` 将相似路径归组
- 避免使用高基数标签（如用户ID）
- 限制标签值的数量

### 2. SLA设置

- 根据业务需求设置合理的SLA阈值
- 区分不同类型API的SLA要求
- 定期评估和调整SLA阈值

### 3. 监控覆盖

- 监控所有关键业务流程
- 包含基础设施和应用层指标
- 设置合适的告警阈值

### 4. 性能优化

- 定期清理过期指标
- 使用合适的抓取间隔
- 监控Prometheus自身性能

## 故障排查

### 1. 指标缺失

- 检查中间件是否正确注册
- 确认路由配置正确
- 验证Prometheus抓取配置

### 2. 高基数问题

- 检查路径参数是否正确归组
- 限制动态标签值
- 使用聚合规则减少存储

### 3. 性能问题

- 监控指标收集开销
- 优化标签使用
- 考虑采样策略

## 扩展功能

### 1. 分布式追踪

可以集成Jaeger或OpenTelemetry进行分布式追踪。

### 2. 日志聚合

可以集成Loki进行日志聚合和关联。

### 3. 自定义指标

可以根据业务需求添加自定义指标收集。