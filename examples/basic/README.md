# Go Backend Kit - 监控示例

这个示例展示了如何使用 Go Backend Kit 的 Prometheus 指标集成功能。

## 功能特性

- 完整的 API 端点示例（认证、用户管理、管理员功能）
- Prometheus 指标集成
- 模拟各种监控场景的演示端点
- 支持 Grafana 监控面板导入

## 运行要求

- Go 1.23+
- MySQL (本地运行在 3306 端口)
- Redis (本地运行在 6379 端口)
- Prometheus (本地运行)
- Grafana (本地运行)

## 快速开始

### 1. 运行应用

```bash
go run main.go
```

应用将在以下端口启动：
- API: http://localhost:8080
- Metrics: http://localhost:9090/metrics

### 2. 配置 Prometheus

在你的本地 Prometheus 配置中添加：

```yaml
scrape_configs:
  - job_name: 'go-backend-kit'
    static_configs:
      - targets: ['localhost:9090']
    metrics_path: '/metrics'
```

### 3. 导入 Grafana 面板

1. 登录本地 Grafana
2. 进入 Dashboards → Import
3. 导入以下监控面板：
   - `monitoring/grafana/dashboards/application-overview.json`
   - `monitoring/grafana/dashboards/business-metrics.json`
   - `monitoring/grafana/dashboards/middleware-metrics.json`
   - `monitoring/grafana/dashboards/system-resources.json`

## API 端点

### 公开端点
- `GET /api/v1/ping` - 健康检查
- `POST /api/v1/auth/login` - 用户登录（模拟 90% 成功率）
- `POST /api/v1/auth/register` - 用户注册

### 受保护端点（需要认证）
- `GET /api/v1/user/profile` - 获取用户信息（模拟缓存命中）
- `PUT /api/v1/user/profile` - 更新用户信息
- `GET /api/v1/data/analytics` - 模拟慢查询（可能触发 SLA 违规）

### 管理员端点
- `GET /api/v1/admin/users` - 获取用户列表（模拟外部服务调用）
- `GET /api/v1/admin/stats` - 获取统计信息

### 演示端点（用于测试监控）
- `GET /api/v1/demo/error?type=<error_type>` - 生成错误
- `POST /api/v1/demo/message?queue=<queue_name>` - 模拟消息队列操作
- `GET /api/v1/demo/cache/<key>` - 模拟缓存操作

### 监控端点
- `GET /health` - 健康检查
- `GET /metrics` - Prometheus 指标

## 测试监控功能

### 1. 生成流量

使用以下脚本生成测试流量：

```bash
# 测试登录端点
for i in {1..100}; do
  curl -X POST http://localhost:8080/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"test","password":"test"}'
  sleep 0.1
done

# 测试缓存命中率
for i in {1..50}; do
  curl http://localhost:8080/api/v1/demo/cache/key$((i % 10))
  sleep 0.2
done

# 生成一些错误
for i in {1..10}; do
  curl "http://localhost:8080/api/v1/demo/error?type=database_error"
  curl "http://localhost:8080/api/v1/demo/error?type=validation_error"
  sleep 1
done
```

### 2. 观察指标变化

在 Grafana 中观察：
- 请求速率和响应时间
- 错误率变化
- 缓存命中率
- 系统资源使用情况

### 3. 触发告警

```bash
# 生成高错误率触发告警
for i in {1..50}; do
  curl "http://localhost:8080/api/v1/demo/error?type=critical"
done
```

## 查看指标

访问 http://localhost:9090/metrics 查看所有 Prometheus 指标。

主要指标类型：
- **HTTP 指标**: 请求数、延迟、状态码分布
- **数据库指标**: 连接池、查询性能
- **缓存指标**: 命中率、Redis 操作
- **业务指标**: 用户活动、错误统计
- **系统指标**: CPU、内存、GC

## 故障排查

### 应用无法启动
```bash
# 检查端口占用
lsof -i :8080
lsof -i :9090

# 查看日志
go run main.go
```

### Prometheus 无法抓取指标
```bash
# 检查指标端点
curl http://localhost:9090/metrics

# 在 Prometheus 中检查 targets
# 访问 http://localhost:9090/targets
```

### Grafana 面板无数据
1. 确认 Prometheus 数据源配置正确
2. 检查面板中的 job 标签是否为 "go-backend-kit"
3. 确认应用正在运行并产生指标