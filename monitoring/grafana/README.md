# Grafana 仪表板使用说明

本目录包含了 Go Backend Kit 项目的 Grafana 监控仪表板模板。

## 仪表板列表

1. **go-backend-kit-overview.json** - 业务监控总览
   - 核心业务指标（请求速率、成功率、P95延迟、错误率）
   - 业务统计（用户活动、缓存命中率）
   - 数据库性能监控

2. **middleware-metrics.json** - 中间件监控
   - MySQL 数据库监控（连接池、查询延迟）
   - Redis 缓存监控（命令速率、命中率）
   - RabbitMQ 消息队列监控（吞吐量、处理成功率）
   - 认证与速率限制监控

3. **system-performance.json** - 系统性能监控
   - 系统资源概览（CPU、内存、Goroutines）
   - Go 运行时统计（GC性能、内存分配）
   - 系统资源历史趋势

4. **api-endpoints.json** - API 端点监控
   - 端点性能排行榜
   - 端点详细分析（请求速率、响应时间、状态码分布）
   - 端点对比分析

5. **business-metrics.json** - 业务指标监控（旧版本，建议使用 go-backend-kit-overview.json）

6. **application-overview.json** - 应用总览（旧版本，建议使用 go-backend-kit-overview.json）

## 导入仪表板

### 前提条件

1. 确保 Prometheus 已正确配置并能采集 go-backend-kit 的指标
2. 确保 Grafana 已安装并能访问 Prometheus 数据源

### 导入步骤

1. 登录 Grafana（默认地址：http://localhost:3000）

2. 配置 Prometheus 数据源：
   - 进入 Configuration → Data Sources
   - 点击 "Add data source"
   - 选择 Prometheus
   - 设置 URL 为 `http://localhost:9090`
   - 点击 "Save & Test"

3. 导入仪表板：
   - 进入 Dashboards → Import
   - 点击 "Upload JSON file"
   - 选择需要导入的仪表板文件
   - 选择刚才配置的 Prometheus 数据源
   - 点击 "Import"

## 使用建议

1. **开始使用**：建议先导入 `go-backend-kit-overview.json`，这个仪表板提供了最全面的业务监控视图。

2. **深入分析**：
   - 如果需要分析特定中间件的性能，使用 `middleware-metrics.json`
   - 如果需要分析系统资源使用情况，使用 `system-performance.json`
   - 如果需要分析特定 API 端点的性能，使用 `api-endpoints.json`

3. **自定义调整**：
   - 所有仪表板都可以根据实际需求进行编辑和调整
   - 可以修改查询、添加新的面板或调整布局
   - 记得保存修改后的仪表板

## 指标说明

主要监控指标包括：

- **gin_requests_total**: HTTP 请求总数
- **gin_request_duration_seconds**: HTTP 请求延迟
- **db_connections_active/idle**: 数据库连接池状态
- **db_query_duration_seconds**: 数据库查询延迟
- **redis_commands_total**: Redis 命令执行次数
- **cache_hits/misses_total**: 缓存命中/未命中次数
- **message_queue_published/consumed_total**: 消息队列发布/消费数量
- **user_registrations/logins_total**: 用户注册/登录次数
- **go_memstats_***: Go 运行时内存统计
- **process_***: 进程级别的资源使用统计

## 故障排查

如果仪表板没有显示数据：

1. 检查 Prometheus 是否正在运行（http://localhost:9090）
2. 检查 Prometheus targets 页面，确认 go-backend-kit 的状态为 UP
3. 检查应用是否正确暴露了 /metrics 端点（http://localhost:9091/metrics）
4. 检查 Grafana 数据源配置是否正确
5. 检查仪表板的时间范围设置是否合适

## 最佳实践

1. 定期检查仪表板，特别是错误率和延迟指标
2. 为关键指标设置告警规则
3. 根据业务需求自定义仪表板
4. 定期备份重要的仪表板配置
5. 使用变量功能来创建可交互的仪表板