package extractor

// ExtractRule 提取规则
type ExtractRule struct {
	// 字段名
	Field string `json:"field" yaml:"field"`
	
	// 选择器
	Selector string `json:"selector" yaml:"selector"`
	
	// 选择器类型
	Type SelectorType `json:"type" yaml:"type"`
	
	// 属性（用于CSS选择器）
	Attribute string `json:"attribute,omitempty" yaml:"attribute,omitempty"`
	
	// 是否提取多个
	Multiple bool `json:"multiple" yaml:"multiple"`
	
	// 默认值
	DefaultValue string `json:"default_value,omitempty" yaml:"default_value,omitempty"`
	
	// 转换规则
	Transforms []TransformRule `json:"transforms,omitempty" yaml:"transforms,omitempty"`
	
	// 子规则（用于嵌套提取）
	Children []ExtractRule `json:"children,omitempty" yaml:"children,omitempty"`
}

// SelectorType 选择器类型
type SelectorType string

const (
	SelectorTypeCSS      SelectorType = "css"
	SelectorTypeXPath    SelectorType = "xpath"
	SelectorTypeJSONPath SelectorType = "jsonpath"
	SelectorTypeRegex    SelectorType = "regex"
)

// TransformRule 转换规则
type TransformRule struct {
	// 转换类型
	Type TransformType `json:"type" yaml:"type"`
	
	// 参数
	Params map[string]interface{} `json:"params,omitempty" yaml:"params,omitempty"`
}

// TransformType 转换类型
type TransformType string

const (
	TransformTrim       TransformType = "trim"
	TransformReplace    TransformType = "replace"
	TransformRegex      TransformType = "regex"
	TransformFormat     TransformType = "format"
	TransformSplit      TransformType = "split"
	TransformJoin       TransformType = "join"
	TransformUpperCase  TransformType = "uppercase"
	TransformLowerCase  TransformType = "lowercase"
	TransformParseInt   TransformType = "parse_int"
	TransformParseFloat TransformType = "parse_float"
	TransformParseDate  TransformType = "parse_date"
	TransformParseJSON  TransformType = "parse_json"
)

// Extractor 提取器接口
type Extractor interface {
	// Extract 根据规则提取数据
	Extract(content []byte, rules []ExtractRule) ([]map[string]interface{}, error)
	
	// ExtractLinks 提取链接
	ExtractLinks(content []byte, baseURL string) ([]string, error)
	
	// ExtractMeta 提取元数据
	ExtractMeta(content []byte) (map[string]string, error)
}

// Transform 数据转换接口
type Transform interface {
	// Apply 应用转换
	Apply(value interface{}) (interface{}, error)
}

// Result 提取结果
type Result struct {
	// 提取的数据
	Data []map[string]interface{} `json:"data"`
	
	// 提取的链接
	Links []string `json:"links,omitempty"`
	
	// 元数据
	Meta map[string]string `json:"meta,omitempty"`
	
	// 错误信息
	Errors []string `json:"errors,omitempty"`
}

// RuleSet 规则集
type RuleSet struct {
	// 名称
	Name string `json:"name" yaml:"name"`
	
	// 版本
	Version string `json:"version" yaml:"version"`
	
	// 描述
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	
	// 内容类型
	ContentType string `json:"content_type" yaml:"content_type"` // html, json, xml, text
	
	// 提取规则
	Rules []ExtractRule `json:"rules" yaml:"rules"`
	
	// 分页规则
	Pagination *PaginationRule `json:"pagination,omitempty" yaml:"pagination,omitempty"`
	
	// 链接提取规则
	LinkRules []LinkRule `json:"link_rules,omitempty" yaml:"link_rules,omitempty"`
}

// PaginationRule 分页规则
type PaginationRule struct {
	// 下一页选择器
	NextSelector string `json:"next_selector" yaml:"next_selector"`
	
	// 下一页URL模板
	NextURLTemplate string `json:"next_url_template,omitempty" yaml:"next_url_template,omitempty"`
	
	// 最大页数
	MaxPages int `json:"max_pages,omitempty" yaml:"max_pages,omitempty"`
	
	// 页码参数名
	PageParam string `json:"page_param,omitempty" yaml:"page_param,omitempty"`
	
	// 起始页码
	StartPage int `json:"start_page,omitempty" yaml:"start_page,omitempty"`
}

// LinkRule 链接提取规则
type LinkRule struct {
	// 选择器
	Selector string `json:"selector" yaml:"selector"`
	
	// URL模式（正则）
	Pattern string `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	
	// 排除模式
	Exclude string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
	
	// 链接类型
	Type string `json:"type,omitempty" yaml:"type,omitempty"` // detail, list, next
	
	// 优先级
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`
}