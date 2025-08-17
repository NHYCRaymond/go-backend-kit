# REQUEST_ID 生命周期追踪系统

本系统实现了完整的REQUEST_ID生命周期追踪，从HTTP请求到响应的整个过程中都可以通过REQUEST_ID查询到相关信息。

## 核心功能

### 1. REQUEST_ID 自动生成和传递
- 自动为每个HTTP请求生成唯一的REQUEST_ID
- 支持从请求头获取现有的REQUEST_ID
- 在响应头中返回REQUEST_ID
- 在响应体中包含REQUEST_ID

### 2. 分布式追踪支持
- 支持生成TRACE_ID用于分布式追踪
- 兼容OpenTelemetry规范
- 可以在微服务间传递追踪信息

### 3. 完整的生命周期追踪
- 从请求开始到响应结束全程追踪
- 支持在日志中记录REQUEST_ID
- 支持在监控指标中包含REQUEST_ID
- 支持在错误报告中包含REQUEST_ID

## 使用方法

### 1. 基本集成

```go
import (
    "github.com/NHYCRaymond/go-backend-kit/middleware"
    "github.com/gin-gonic/gin"
)

func main() {
    router := gin.New()
    
    // 添加REQUEST_ID中间件（必须在其他中间件之前）
    router.Use(middleware.RequestIDMiddleware(nil)) // 使用默认配置
    
    // 其他中间件...
    router.Use(loggerMiddleware.Handler())
    
    // 路由...
    router.GET("/api/test", func(c *gin.Context) {
        requestID := middleware.GetRequestID(c)
        // 使用requestID...
    })
}
```

### 2. 自定义配置

```go
config := &middleware.RequestIDConfig{
    Generator:        middleware.DefaultRequestIDConfig().Generator,
    HeaderName:       "X-Request-ID",
    ContextKey:       "request_id",
    EnableTraceID:    true,
    TraceIDGenerator: middleware.DefaultRequestIDConfig().TraceIDGenerator,
    SkipPaths:        []string{"/health", "/metrics"},
}

router.Use(middleware.RequestIDMiddleware(config))
```

### 3. 在业务逻辑中使用

```go
func userHandler(c *gin.Context) {
    // 获取REQUEST_ID
    requestID := middleware.GetRequestID(c)
    traceID := middleware.GetTraceIDFromLogger(c)
    
    // 创建带有REQUEST_ID的日志器
    contextLogger := middleware.LogWithRequestID(c, logger)
    contextLogger.Info("Processing user request", "user_id", userID)
    
    // 传递context到下层服务
    ctx := c.Request.Context()
    result := userService.GetUser(ctx, userID)
    
    // 响应自动包含REQUEST_ID
    response.Success(c, result)
}
```

### 4. 在服务层使用

```go
func (s *UserService) GetUser(ctx context.Context, userID string) (*User, error) {
    // 从context获取REQUEST_ID
    requestID := middleware.GetRequestIDFromContext(ctx)
    traceID := middleware.GetTraceIDFromContext(ctx)
    
    // 在日志中记录REQUEST_ID
    s.logger.Info("Getting user from database", 
        "request_id", requestID,
        "trace_id", traceID,
        "user_id", userID)
    
    // 数据库查询...
    user, err := s.db.GetUser(userID)
    if err != nil {
        s.logger.Error("Database query failed", 
            "request_id", requestID,
            "error", err)
        return nil, err
    }
    
    return user, nil
}
```

## REQUEST_ID 格式

### 1. 支持的生成器

```go
// UUID v4 (默认)
func generateUUID() string {
    return uuid.New().String()
}

// 短ID (8字符)
func generateShortID() string {
    // 返回8字符的十六进制ID
}

// 时间戳ID
func generateTimestampID() string {
    // 返回 timestamp-shortid 格式
}

// 自定义生成器
config := &middleware.RequestIDConfig{
    Generator: func() string {
        return "custom-" + generateShortID()
    },
}
```

### 2. TRACE_ID 生成

```go
// 16字节随机ID (兼容OpenTelemetry)
func generateTraceID() string {
    bytes := make([]byte, 16)
    rand.Read(bytes)
    return hex.EncodeToString(bytes)
}
```

## 响应格式

### 1. 成功响应

```json
{
    "code": 0,
    "message": "success",
    "data": {
        "user_id": "123",
        "name": "John Doe"
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "trace_id": "5b8aa5a2d2c872e8321374a59b056f47",
    "timestamp": 1640995200
}
```

### 2. 错误响应

```json
{
    "code": 500,
    "message": "Internal server error",
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "trace_id": "5b8aa5a2d2c872e8321374a59b056f47",
    "timestamp": 1640995200
}
```

### 3. 分页响应

```json
{
    "code": 0,
    "data": [...],
    "pagination": {
        "page": 1,
        "per_page": 10,
        "total": 100,
        "total_pages": 10
    },
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "trace_id": "5b8aa5a2d2c872e8321374a59b056f47",
    "timestamp": 1640995200
}
```

## 日志格式

### 1. 结构化日志

```json
{
    "timestamp": "2024-01-01T00:00:00Z",
    "level": "INFO",
    "message": "HTTP request completed",
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "trace_id": "5b8aa5a2d2c872e8321374a59b056f47",
    "method": "GET",
    "path": "/api/v1/users/123",
    "status": 200,
    "duration": "150ms",
    "duration_ms": 150,
    "ip": "192.168.1.1",
    "user_agent": "Mozilla/5.0...",
    "user_id": "123"
}
```

### 2. 业务日志

```json
{
    "timestamp": "2024-01-01T00:00:00Z",
    "level": "INFO",
    "message": "User data fetched from database",
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "trace_id": "5b8aa5a2d2c872e8321374a59b056f47",
    "user_id": "123",
    "operation": "user_fetch",
    "duration_ms": 45
}
```

## 监控集成

### 1. Prometheus指标

REQUEST_ID会被自动包含在监控指标的标签中（如果配置了的话）：

```promql
# 按REQUEST_ID查询慢请求
gin_request_duration_seconds{request_id="550e8400-e29b-41d4-a716-446655440000"}

# 按TRACE_ID查询分布式请求
gin_request_duration_seconds{trace_id="5b8aa5a2d2c872e8321374a59b056f47"}
```

### 2. SLA违规追踪

```go
// 在监控中间件中自动记录SLA违规
if durationSeconds > threshold {
    monitoring.RecordSLAViolation(route, threshold)
    // 可以在这里记录REQUEST_ID用于后续分析
}
```

## 错误处理

### 1. 错误响应中的REQUEST_ID

```go
func errorHandler(c *gin.Context) {
    requestID := middleware.GetRequestID(c)
    
    // 记录错误日志
    contextLogger := middleware.LogWithRequestID(c, logger)
    contextLogger.Error("Business logic error", 
        "error", err,
        "operation", "user_create")
    
    // 返回错误响应（自动包含REQUEST_ID）
    response.Error(c, http.StatusBadRequest, 400, "Invalid input")
}
```

### 2. 错误日志关联

```bash
# 使用REQUEST_ID查询所有相关日志
grep "550e8400-e29b-41d4-a716-446655440000" /var/log/app.log

# 使用TRACE_ID查询分布式追踪
grep "5b8aa5a2d2c872e8321374a59b056f47" /var/log/app.log
```

## 生产环境配置

### 1. 性能优化

```go
// 使用更短的ID减少内存使用
config := &middleware.RequestIDConfig{
    Generator: middleware.generateShortID, // 8字符ID
    EnableTraceID: false, // 如果不需要分布式追踪
}
```

### 2. 安全考虑

```go
// 不在公共API中暴露内部TRACE_ID
config := &middleware.RequestIDConfig{
    EnableTraceID: true,
    SkipPaths: []string{"/public/*"}, // 公共API不生成TRACE_ID
}
```

### 3. 存储和查询

```go
// 可以选择将REQUEST_ID存储到数据库中用于审计
type RequestLog struct {
    RequestID string    `json:"request_id"`
    TraceID   string    `json:"trace_id"`
    UserID    string    `json:"user_id"`
    Method    string    `json:"method"`
    Path      string    `json:"path"`
    Status    int       `json:"status"`
    Duration  int64     `json:"duration_ms"`
    CreatedAt time.Time `json:"created_at"`
}
```

## 故障排查

### 1. 通过REQUEST_ID查询完整请求链路

```bash
# 查询REQUEST_ID相关的所有日志
grep -r "550e8400-e29b-41d4-a716-446655440000" /var/log/

# 查询特定时间范围内的请求
grep "2024-01-01T10:" /var/log/app.log | grep "550e8400-e29b-41d4-a716-446655440000"
```

### 2. 性能分析

```bash
# 查询慢请求
grep "duration_ms.*[5-9][0-9][0-9]" /var/log/app.log

# 查询错误请求
grep "level.*ERROR" /var/log/app.log | grep "request_id"
```

### 3. 分布式追踪

```bash
# 查询整个分布式请求链路
grep "5b8aa5a2d2c872e8321374a59b056f47" /var/log/service1.log /var/log/service2.log
```

## 最佳实践

1. **中间件顺序**：REQUEST_ID中间件应该在最前面（除了Recovery中间件）
2. **日志记录**：所有业务日志都应该包含REQUEST_ID
3. **错误处理**：所有错误响应都应该包含REQUEST_ID
4. **监控集成**：重要的监控指标应该包含REQUEST_ID用于问题定位
5. **性能考虑**：在高并发场景下考虑使用更短的ID格式
6. **安全性**：不要在REQUEST_ID中包含敏感信息
7. **一致性**：确保在整个请求链路中使用相同的REQUEST_ID