# Grafana 监控面板设置指南

由于 Grafana 版本差异，直接导入 JSON 可能会失败。请按照以下步骤手动创建监控面板。

## 前置条件

1. 确保 Prometheus 已配置并抓取了 `go-backend-kit` 的指标
2. 在 Grafana 中已添加 Prometheus 数据源

## 创建业务指标监控面板

### 1. 创建新 Dashboard

1. 登录 Grafana
2. 点击左侧菜单 "Dashboards" → "New" → "New Dashboard"
3. 点击 "Add visualization"

### 2. 添加请求速率面板

**Panel 1: API 请求速率**
- 查询: `sum(rate(gin_requests_total[5m])) by (method)`
- 可视化类型: Time series
- 标题: API 请求速率（按方法）
- 单位: reqps (requests per second)
- Legend: {{method}}

### 3. 添加成功率面板

**Panel 2: API 成功率**
- 查询: `sum(rate(gin_requests_total{status_code!~"5.."}[5m])) / sum(rate(gin_requests_total[5m]))`
- 可视化类型: Gauge
- 标题: API 成功率
- 单位: Percent (0-1)
- 阈值:
  - 0-0.95: 红色
  - 0.95-0.99: 黄色
  - 0.99-1: 绿色

### 4. 添加响应时间面板

**Panel 3: API 响应时间**
- 查询 A: `histogram_quantile(0.95, sum(rate(gin_request_duration_seconds_bucket[5m])) by (route, le))`
- 查询 B: `histogram_quantile(0.99, sum(rate(gin_request_duration_seconds_bucket[5m])) by (route, le))`
- 可视化类型: Time series
- 标题: API 响应时间分位数
- 单位: seconds
- Legend:
  - A: P95 - {{route}}
  - B: P99 - {{route}}

### 5. 添加状态码分布面板

**Panel 4: HTTP 状态码分布**
- 查询: `sum(rate(gin_requests_total[5m])) by (status_code)`
- 可视化类型: Time series
- 标题: HTTP 状态码分布
- Legend: {{status_code}}

### 6. 添加错误率面板

**Panel 5: 错误率**
- 查询: `sum(rate(gin_requests_total{status_code=~"5.."}[5m])) by (route)`
- 可视化类型: Time series
- 标题: 错误率（按路由）
- 单位: errors/sec
- Legend: {{route}}

### 7. 添加活跃请求面板

**Panel 6: 活跃请求数**
- 查询: `gin_active_requests`
- 可视化类型: Time series
- 标题: 活跃请求数
- Legend: {{method}} {{route}}

## 创建中间件监控面板

### 数据库监控

**Panel 1: 数据库连接池**
- 查询 A: `db_connections_active{database="mysql"}`
- 查询 B: `db_connections_idle{database="mysql"}`
- 标题: MySQL 连接池状态
- Legend:
  - A: 活跃连接
  - B: 空闲连接

**Panel 2: 数据库查询性能**
- 查询: `histogram_quantile(0.95, sum(rate(db_query_duration_seconds_bucket[5m])) by (operation, le))`
- 标题: 数据库查询延迟 P95
- 单位: seconds
- Legend: {{operation}}

### Redis 监控

**Panel 3: Redis 操作速率**
- 查询: `sum(rate(redis_commands_total{status="success"}[5m])) by (command)`
- 标题: Redis 命令执行速率
- 单位: ops
- Legend: {{command}}

**Panel 4: 缓存命中率**
- 查询: `sum(rate(cache_hits_total[5m])) by (cache_type) / (sum(rate(cache_hits_total[5m])) by (cache_type) + sum(rate(cache_misses_total[5m])) by (cache_type))`
- 标题: 缓存命中率
- 单位: Percent (0-1)
- Legend: {{cache_type}}

## 创建系统资源监控面板

**Panel 1: CPU 使用率**
- 查询: `cpu_usage_percent`
- 可视化类型: Gauge
- 标题: CPU 使用率
- 单位: Percent (0-100)

**Panel 2: 内存使用**
- 查询 A: `memory_usage_bytes`
- 查询 B: `memory_total_bytes`
- 标题: 内存使用情况
- 单位: bytes

**Panel 3: Goroutines**
- 查询: `goroutines_total`
- 标题: Goroutine 数量

**Panel 4: GC 性能**
- 查询: `histogram_quantile(0.99, rate(gc_duration_seconds_bucket[5m]))`
- 标题: GC 暂停时间 P99
- 单位: seconds

## 变量设置

在 Dashboard Settings → Variables 中添加：

1. **数据源变量**
   - Name: datasource
   - Type: Data source
   - Query: prometheus

2. **Job 变量**
   - Name: job
   - Type: Query
   - Data source: ${datasource}
   - Query: `label_values(gin_requests_total, job)`
   - Default: go-backend-kit

## 常见问题

### 面板无数据
1. 检查 Prometheus targets: http://localhost:9090/targets
2. 确认应用正在运行: `curl http://localhost:9090/metrics`
3. 检查 job 标签是否正确

### 查询语法错误
1. 在 Prometheus 中测试查询: http://localhost:9090/graph
2. 检查指标名称是否正确
3. 验证标签过滤条件

### 性能问题
1. 调整时间范围
2. 增加查询的 step 参数
3. 考虑使用记录规则