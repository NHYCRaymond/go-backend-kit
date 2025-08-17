package task

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TaskDocument MongoDB中的任务文档结构 - 完全通用的设计
type TaskDocument struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name        string             `bson:"name" json:"name"`                 // 任务名称
	Type        string             `bson:"type" json:"type"`                 // 任务类型 (http, api, websocket, custom)
	Category    string             `bson:"category" json:"category"`         // 任务分类 (sports, news, e-commerce, etc)
	Description string             `bson:"description" json:"description"`   // 任务描述
	Version     string             `bson:"version" json:"version"`           // 任务版本
	Status      TaskStatus         `bson:"status" json:"status"`             // 任务状态
	Config      TaskConfiguration  `bson:"config" json:"config"`             // 通用配置
	Schedule    ScheduleConfig     `bson:"schedule" json:"schedule"`         // 调度配置
	Execution   ExecutionConfig    `bson:"execution" json:"execution"`       // 执行配置
	Request     RequestTemplate    `bson:"request" json:"request"`           // 请求模板
	Extraction  ExtractionConfig   `bson:"extraction" json:"extraction"`     // 数据提取配置
	Transform   TransformConfig    `bson:"transform" json:"transform"`       // 数据转换配置
	Validation  ValidationConfig   `bson:"validation" json:"validation"`     // 数据验证配置
	Storage     StorageConfiguration `bson:"storage" json:"storage"`           // 存储配置
	Monitoring  MonitoringConfig   `bson:"monitoring" json:"monitoring"`     // 监控配置
	Metadata    map[string]any     `bson:"metadata" json:"metadata"`         // 元数据
	Tags        []string           `bson:"tags" json:"tags"`                 // 标签
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
	CreatedBy   string             `bson:"created_by" json:"created_by"`
	UpdatedBy   string             `bson:"updated_by" json:"updated_by"`
}

// TaskStatus 任务状态
type TaskStatus struct {
	Enabled       bool      `bson:"enabled" json:"enabled"`               // 是否启用
	LastRun       time.Time `bson:"last_run" json:"last_run"`             // 最后运行时间
	NextRun       time.Time `bson:"next_run" json:"next_run"`             // 下次运行时间
	RunCount      int64     `bson:"run_count" json:"run_count"`           // 运行次数
	SuccessCount  int64     `bson:"success_count" json:"success_count"`   // 成功次数
	FailureCount  int64     `bson:"failure_count" json:"failure_count"`   // 失败次数
	LastSuccess   time.Time `bson:"last_success" json:"last_success"`     // 最后成功时间
	LastFailure   time.Time `bson:"last_failure" json:"last_failure"`     // 最后失败时间
	ErrorMessage  string    `bson:"error_message" json:"error_message"`   // 错误信息
}

// TaskConfiguration 通用任务配置
type TaskConfiguration struct {
	Priority    int            `bson:"priority" json:"priority"`         // 优先级 (1-10)
	Timeout     int            `bson:"timeout" json:"timeout"`           // 超时时间（秒）
	MaxRetries  int            `bson:"max_retries" json:"max_retries"`   // 最大重试次数
	RetryDelay  int            `bson:"retry_delay" json:"retry_delay"`   // 重试延迟（秒）
	Concurrency int            `bson:"concurrency" json:"concurrency"`   // 并发数
	RateLimit   RateLimitConfig `bson:"rate_limit" json:"rate_limit"`    // 速率限制
	Parameters  map[string]any  `bson:"parameters" json:"parameters"`    // 自定义参数
}

// RateLimitConfig 速率限制配置
type RateLimitConfig struct {
	Enabled         bool `bson:"enabled" json:"enabled"`
	RequestsPerMin  int  `bson:"requests_per_min" json:"requests_per_min"`
	RequestsPerHour int  `bson:"requests_per_hour" json:"requests_per_hour"`
	BurstSize       int  `bson:"burst_size" json:"burst_size"`
}

// ScheduleConfig 调度配置
type ScheduleConfig struct {
	Type       string         `bson:"type" json:"type"`               // 调度类型 (cron, interval, once, manual)
	Expression string         `bson:"expression" json:"expression"`   // Cron表达式或间隔
	Timezone   string         `bson:"timezone" json:"timezone"`       // 时区
	StartTime  *time.Time     `bson:"start_time" json:"start_time"`   // 开始时间
	EndTime    *time.Time     `bson:"end_time" json:"end_time"`       // 结束时间
	Conditions []ScheduleCond `bson:"conditions" json:"conditions"`   // 执行条件
}

// ScheduleCond 调度条件
type ScheduleCond struct {
	Type       string `bson:"type" json:"type"`             // 条件类型
	Expression string `bson:"expression" json:"expression"` // 条件表达式
}

// ExecutionConfig 执行配置
type ExecutionConfig struct {
	Mode          string         `bson:"mode" json:"mode"`                     // 执行模式 (single, batch, stream)
	BatchSize     int            `bson:"batch_size" json:"batch_size"`         // 批量大小
	Parallelism   int            `bson:"parallelism" json:"parallelism"`       // 并行度
	Dependencies  []string       `bson:"dependencies" json:"dependencies"`     // 依赖任务
	PreHooks      []HookConfig   `bson:"pre_hooks" json:"pre_hooks"`           // 前置钩子
	PostHooks     []HookConfig   `bson:"post_hooks" json:"post_hooks"`         // 后置钩子
	ErrorHandling ErrorHandling  `bson:"error_handling" json:"error_handling"` // 错误处理
}

// HookConfig 钩子配置
type HookConfig struct {
	Type   string         `bson:"type" json:"type"`     // 钩子类型 (http, script, function)
	Action string         `bson:"action" json:"action"` // 动作
	Config map[string]any `bson:"config" json:"config"` // 配置
}

// ErrorHandling 错误处理配置
type ErrorHandling struct {
	Strategy      string   `bson:"strategy" json:"strategy"`           // 策略 (retry, skip, fail, notify)
	RetryStrategy string   `bson:"retry_strategy" json:"retry_strategy"` // 重试策略 (exponential, linear, fixed)
	MaxRetries    int      `bson:"max_retries" json:"max_retries"`     // 最大重试次数
	NotifyOn      []string `bson:"notify_on" json:"notify_on"`         // 通知条件
}

// RequestTemplate 请求模板 - 通用HTTP/WebSocket/自定义协议
type RequestTemplate struct {
	Method      string            `bson:"method" json:"method"`           // 请求方法
	URL         string            `bson:"url" json:"url"`                 // URL模板（支持变量）
	Headers     map[string]string `bson:"headers" json:"headers"`         // 请求头
	Cookies     map[string]string `bson:"cookies" json:"cookies"`         // Cookies
	QueryParams map[string]any    `bson:"query_params" json:"query_params"` // 查询参数
	Body        any               `bson:"body" json:"body"`               // 请求体（支持模板）
	Auth        AuthConfig        `bson:"auth" json:"auth"`               // 认证配置
	Proxy       ProxyConfig       `bson:"proxy" json:"proxy"`             // 代理配置
	Variables   []Variable        `bson:"variables" json:"variables"`     // 变量定义
	Pagination  PaginationConfig  `bson:"pagination" json:"pagination"`   // 分页配置
}

// Variable 变量定义
type Variable struct {
	Name       string `bson:"name" json:"name"`               // 变量名
	Type       string `bson:"type" json:"type"`               // 类型 (string, number, date, array)
	Source     string `bson:"source" json:"source"`           // 来源 (env, context, function, static)
	Expression string `bson:"expression" json:"expression"`   // 表达式
	Default    any    `bson:"default" json:"default"`         // 默认值
}

// AuthConfig 认证配置
type AuthConfig struct {
	Type   string         `bson:"type" json:"type"`     // 认证类型 (none, basic, bearer, oauth2, custom)
	Config map[string]any `bson:"config" json:"config"` // 认证配置
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	Enabled  bool     `bson:"enabled" json:"enabled"`
	Type     string   `bson:"type" json:"type"`     // 代理类型 (http, socks5)
	Rotation bool     `bson:"rotation" json:"rotation"`
	Servers  []string `bson:"servers" json:"servers"`
}

// PaginationConfig 分页配置
type PaginationConfig struct {
	Enabled     bool   `bson:"enabled" json:"enabled"`
	Type        string `bson:"type" json:"type"`                 // 分页类型 (page, offset, cursor, token)
	StartParam  string `bson:"start_param" json:"start_param"`   // 起始参数名
	LimitParam  string `bson:"limit_param" json:"limit_param"`   // 限制参数名
	StartValue  any    `bson:"start_value" json:"start_value"`   // 起始值
	PageSize    int    `bson:"page_size" json:"page_size"`       // 页大小
	MaxPages    int    `bson:"max_pages" json:"max_pages"`       // 最大页数
	StopCondition string `bson:"stop_condition" json:"stop_condition"` // 停止条件
}

// ExtractionConfig 数据提取配置 - 支持任何数据格式
type ExtractionConfig struct {
	Type      string           `bson:"type" json:"type"`           // 提取类型 (json, html, xml, regex, custom)
	Rules     []ExtractionRule `bson:"rules" json:"rules"`         // 提取规则
	Scripts   []Script         `bson:"scripts" json:"scripts"`     // 自定义脚本
	Templates []Template       `bson:"templates" json:"templates"` // 模板
}

// ExtractionRule 提取规则
type ExtractionRule struct {
	Field      string         `bson:"field" json:"field"`           // 字段名
	Path       string         `bson:"path" json:"path"`             // 路径表达式
	Type       string         `bson:"type" json:"type"`             // 数据类型
	Required   bool           `bson:"required" json:"required"`     // 是否必需
	Default    any            `bson:"default" json:"default"`       // 默认值
	Transform  []Transform    `bson:"transform" json:"transform"`   // 转换规则
	Validation []Validation   `bson:"validation" json:"validation"` // 验证规则
	Children   []ExtractionRule `bson:"children" json:"children"`   // 子规则（嵌套结构）
}

// TransformConfig 数据转换配置
type TransformConfig struct {
	Enabled    bool        `bson:"enabled" json:"enabled"`
	Processors []Processor `bson:"processors" json:"processors"` // 处理器链
}

// Processor 处理器
type Processor struct {
	Name   string         `bson:"name" json:"name"`     // 处理器名称
	Type   string         `bson:"type" json:"type"`     // 处理器类型
	Config map[string]any `bson:"config" json:"config"` // 处理器配置
}

// Transform 转换规则
type Transform struct {
	Type   string         `bson:"type" json:"type"`     // 转换类型
	Config map[string]any `bson:"config" json:"config"` // 转换配置
}

// ValidationConfig 数据验证配置
type ValidationConfig struct {
	Enabled bool         `bson:"enabled" json:"enabled"`
	Rules   []Validation `bson:"rules" json:"rules"` // 验证规则
}

// Validation 验证规则
type Validation struct {
	Type    string `bson:"type" json:"type"`       // 验证类型
	Field   string `bson:"field" json:"field"`     // 字段
	Rule    string `bson:"rule" json:"rule"`       // 规则表达式
	Message string `bson:"message" json:"message"` // 错误消息
}

// StorageConfiguration 存储配置 - 支持多种存储后端
type StorageConfiguration struct {
	Targets []StorageTarget `bson:"targets" json:"targets"` // 存储目标
}

// StorageTarget 存储目标
type StorageTarget struct {
	Name       string         `bson:"name" json:"name"`             // 目标名称
	Type       string         `bson:"type" json:"type"`             // 存储类型 (mongodb, mysql, redis, elasticsearch, file, s3)
	Purpose    string         `bson:"purpose" json:"purpose"`       // 用途 (primary, backup, cache, archive)
	Config     map[string]any `bson:"config" json:"config"`         // 存储配置
	Conditions []string       `bson:"conditions" json:"conditions"` // 存储条件
}

// MonitoringConfig 监控配置
type MonitoringConfig struct {
	Metrics    MetricsConfig    `bson:"metrics" json:"metrics"`       // 指标配置
	Logging    LoggingConfig    `bson:"logging" json:"logging"`       // 日志配置
	Alerting   AlertingConfig   `bson:"alerting" json:"alerting"`     // 告警配置
	Tracing    TracingConfig    `bson:"tracing" json:"tracing"`       // 追踪配置
}

// MetricsConfig 指标配置
type MetricsConfig struct {
	Enabled    bool     `bson:"enabled" json:"enabled"`
	Collectors []string `bson:"collectors" json:"collectors"` // 收集器
	Labels     map[string]string `bson:"labels" json:"labels"`   // 标签
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level      string  `bson:"level" json:"level"`           // 日志级别
	SampleRate float64 `bson:"sample_rate" json:"sample_rate"` // 采样率
}

// AlertingConfig 告警配置
type AlertingConfig struct {
	Enabled bool    `bson:"enabled" json:"enabled"`
	Rules   []Alert `bson:"rules" json:"rules"` // 告警规则
}

// Alert 告警规则
type Alert struct {
	Name      string   `bson:"name" json:"name"`           // 告警名称
	Condition string   `bson:"condition" json:"condition"` // 触发条件
	Channels  []string `bson:"channels" json:"channels"`   // 通知渠道
}

// TracingConfig 追踪配置
type TracingConfig struct {
	Enabled     bool    `bson:"enabled" json:"enabled"`
	SampleRate  float64 `bson:"sample_rate" json:"sample_rate"`
	ServiceName string  `bson:"service_name" json:"service_name"`
}

// Script 脚本
type Script struct {
	Language string `bson:"language" json:"language"` // 脚本语言 (javascript, python, lua)
	Code     string `bson:"code" json:"code"`         // 脚本代码
}

// Template 模板
type Template struct {
	Name    string `bson:"name" json:"name"`       // 模板名称
	Content string `bson:"content" json:"content"` // 模板内容
}