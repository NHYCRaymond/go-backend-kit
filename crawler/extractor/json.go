package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/tidwall/gjson"
)

// JSONExtractor JSON提取器
type JSONExtractor struct {
	// 配置
	config *JSONExtractorConfig

	// 预编译的正则表达式缓存
	regexCache map[string]*regexp.Regexp
}

// JSONExtractorConfig JSON提取器配置
type JSONExtractorConfig struct {
	// 是否严格模式（不允许注释等非标准JSON）
	StrictMode bool `json:"strict_mode"`

	// 是否展开数组
	FlattenArrays bool `json:"flatten_arrays"`

	// 最大嵌套深度
	MaxDepth int `json:"max_depth"`

	// 最大文档大小（字节）
	MaxDocumentSize int `json:"max_document_size"`
}

// DefaultJSONExtractorConfig 默认配置
func DefaultJSONExtractorConfig() *JSONExtractorConfig {
	return &JSONExtractorConfig{
		StrictMode:      false,
		FlattenArrays:   false,
		MaxDepth:        10,
		MaxDocumentSize: 10 * 1024 * 1024, // 10MB
	}
}

// NewJSONExtractor 创建JSON提取器
func NewJSONExtractor(config *JSONExtractorConfig) *JSONExtractor {
	if config == nil {
		config = DefaultJSONExtractorConfig()
	}

	return &JSONExtractor{
		config:     config,
		regexCache: make(map[string]*regexp.Regexp),
	}
}


// ExtractWithJSONPath 使用JSONPath提取
func (e *JSONExtractor) ExtractWithJSONPath(content []byte, jsonPath string) ([]interface{}, error) {
	// 使用gjson进行JSONPath查询
	result := gjson.Get(string(content), jsonPath)

	if !result.Exists() {
		return nil, nil
	}

	// 根据结果类型处理
	if result.IsArray() {
		arr := result.Array()
		results := make([]interface{}, 0, len(arr))
		for _, item := range arr {
			results = append(results, item.Value())
		}
		return results, nil
	}

	return []interface{}{result.Value()}, nil
}

// ExtractFlat 展平提取（将嵌套JSON展平为单层）
func (e *JSONExtractor) ExtractFlat(content []byte) (map[string]interface{}, error) {
	var data interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	flattened := make(map[string]interface{})
	e.flatten("", data, flattened, 0)

	return flattened, nil
}

// ExtractSchema 提取JSON模式
func (e *JSONExtractor) ExtractSchema(content []byte) (map[string]string, error) {
	var data interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	schema := make(map[string]string)
	e.extractSchema("", data, schema, 0)

	return schema, nil
}

// ExtractPaths 提取所有JSONPath路径
func (e *JSONExtractor) ExtractPaths(content []byte) ([]string, error) {
	var data interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	paths := make([]string, 0)
	e.extractPaths("$", data, &paths, 0)

	return paths, nil
}

// extractByRule 根据规则提取数据
func (e *JSONExtractor) extractByRule(root gjson.Result, rule ExtractRule) []interface{} {
	results := make([]interface{}, 0)

	switch rule.Type {
	case SelectorTypeJSONPath:
		result := root.Get(rule.Selector)
		if result.Exists() {
			if result.IsArray() && rule.Multiple {
				for _, item := range result.Array() {
					value := e.processValue(item.Value(), rule.Transforms)
					results = append(results, value)
				}
			} else {
				value := e.processValue(result.Value(), rule.Transforms)
				results = append(results, value)
			}
		}

	case SelectorTypeRegex:
		// 对JSON字符串应用正则表达式
		jsonStr := root.String()
		if matches := e.extractWithRegex(jsonStr, rule.Selector); len(matches) > 0 {
			for _, match := range matches {
				value := e.processValue(match, rule.Transforms)
				results = append(results, value)
				if !rule.Multiple {
					break
				}
			}
		}
	}

	return results
}

// extractChildren 提取子规则数据
func (e *JSONExtractor) extractChildren(data interface{}, rules []ExtractRule) map[string]interface{} {
	result := make(map[string]interface{})

	// 将数据转换为gjson.Result
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return result
	}

	root := gjson.Parse(string(jsonBytes))

	for _, rule := range rules {
		extracted := e.extractByRule(root, rule)
		if len(extracted) > 0 {
			if rule.Multiple {
				result[rule.Field] = extracted
			} else {
				result[rule.Field] = extracted[0]
			}
		} else if rule.DefaultValue != "" {
			result[rule.Field] = rule.DefaultValue
		}
	}

	return result
}

// extractWithRegex 使用正则表达式提取
func (e *JSONExtractor) extractWithRegex(text, pattern string) []string {
	re, exists := e.regexCache[pattern]
	if !exists {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil
		}
		e.regexCache[pattern] = re
	}

	matches := re.FindAllStringSubmatch(text, -1)
	results := make([]string, 0)

	for _, match := range matches {
		if len(match) > 1 {
			results = append(results, match[1])
		} else if len(match) > 0 {
			results = append(results, match[0])
		}
	}

	return results
}

// processValue 处理值（应用转换）
func (e *JSONExtractor) processValue(value interface{}, transforms []TransformRule) interface{} {
	if len(transforms) == 0 {
		return value
	}

	result := value
	for _, transform := range transforms {
		result = e.applyTransform(result, transform)
	}

	return result
}

// applyTransform 应用转换
func (e *JSONExtractor) applyTransform(value interface{}, transform TransformRule) interface{} {
	switch transform.Type {
	case TransformTrim:
		if s, ok := value.(string); ok {
			return strings.TrimSpace(s)
		}

	case TransformReplace:
		if s, ok := value.(string); ok {
			if old, ok := transform.Params["old"].(string); ok {
				if new, ok := transform.Params["new"].(string); ok {
					return strings.ReplaceAll(s, old, new)
				}
			}
		}

	case TransformUpperCase:
		if s, ok := value.(string); ok {
			return strings.ToUpper(s)
		}

	case TransformLowerCase:
		if s, ok := value.(string); ok {
			return strings.ToLower(s)
		}

	case TransformParseInt:
		switch v := value.(type) {
		case string:
			if i, err := strconv.ParseInt(v, 10, 64); err == nil {
				return i
			}
		case float64:
			return int64(v)
		}

	case TransformParseFloat:
		switch v := value.(type) {
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		case int64:
			return float64(v)
		}

	case TransformSplit:
		if s, ok := value.(string); ok {
			if sep, ok := transform.Params["separator"].(string); ok {
				return strings.Split(s, sep)
			}
		}

	case TransformJoin:
		if arr, ok := value.([]interface{}); ok {
			if sep, ok := transform.Params["separator"].(string); ok {
				strs := make([]string, len(arr))
				for i, v := range arr {
					strs[i] = fmt.Sprintf("%v", v)
				}
				return strings.Join(strs, sep)
			}
		}
	}

	return value
}

// flatten 展平JSON
func (e *JSONExtractor) flatten(prefix string, data interface{}, result map[string]interface{}, depth int) {
	if depth >= e.config.MaxDepth {
		return
	}

	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			newKey := key
			if prefix != "" {
				newKey = prefix + "." + key
			}
			e.flatten(newKey, value, result, depth+1)
		}

	case []interface{}:
		if e.config.FlattenArrays {
			for i, value := range v {
				newKey := fmt.Sprintf("%s[%d]", prefix, i)
				e.flatten(newKey, value, result, depth+1)
			}
		} else {
			if prefix != "" {
				result[prefix] = v
			}
		}

	default:
		if prefix != "" {
			result[prefix] = v
		}
	}
}

// extractSchema 提取JSON模式
func (e *JSONExtractor) extractSchema(prefix string, data interface{}, schema map[string]string, depth int) {
	if depth >= e.config.MaxDepth {
		return
	}

	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			newKey := key
			if prefix != "" {
				newKey = prefix + "." + key
			}
			e.extractSchema(newKey, value, schema, depth+1)
		}

	case []interface{}:
		if prefix != "" {
			schema[prefix] = "array"
		}
		if len(v) > 0 {
			e.extractSchema(prefix+"[0]", v[0], schema, depth+1)
		}

	case string:
		if prefix != "" {
			schema[prefix] = "string"
		}

	case float64:
		if prefix != "" {
			schema[prefix] = "number"
		}

	case bool:
		if prefix != "" {
			schema[prefix] = "boolean"
		}

	case nil:
		if prefix != "" {
			schema[prefix] = "null"
		}

	default:
		if prefix != "" {
			schema[prefix] = fmt.Sprintf("%T", v)
		}
	}
}

// extractPaths 提取所有路径
func (e *JSONExtractor) extractPaths(prefix string, data interface{}, paths *[]string, depth int) {
	if depth >= e.config.MaxDepth {
		return
	}

	*paths = append(*paths, prefix)

	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			newPath := fmt.Sprintf("%s.%s", prefix, key)
			e.extractPaths(newPath, value, paths, depth+1)
		}

	case []interface{}:
		for i, value := range v {
			newPath := fmt.Sprintf("%s[%d]", prefix, i)
			e.extractPaths(newPath, value, paths, depth+1)
		}
	}
}


// ExtractMeta 提取元数据
func (e *JSONExtractor) ExtractMeta(content []byte) (map[string]string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	meta := make(map[string]string)

	// 提取常见的元数据字段
	metaFields := []string{
		"title", "description", "author", "date", "version",
		"type", "category", "tags", "status", "source",
	}

	for _, field := range metaFields {
		if value, exists := data[field]; exists {
			meta[field] = fmt.Sprintf("%v", value)
		}
	}

	// 提取统计信息
	meta["_size"] = fmt.Sprintf("%d", len(content))
	meta["_type"] = "json"

	return meta, nil
}

// Extract implements task.Extractor interface - extracts data from JSON response
func (e *JSONExtractor) Extract(ctx context.Context, response *task.Response, rules []task.ExtractRule) (map[string]interface{}, error) {
	// Validate JSON
	if !gjson.Valid(string(response.Body)) {
		// Provide more context in error message
		preview := string(response.Body)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		// Check if it might be HTML
		if strings.Contains(preview, "<html") || strings.Contains(preview, "<!DOCTYPE") {
			return nil, fmt.Errorf("invalid JSON content: received HTML response (status=%d)", response.StatusCode)
		}
		return nil, fmt.Errorf("invalid JSON content (status=%d, preview: %s)", response.StatusCode, preview)
	}

	// Parse JSON
	root := gjson.Parse(string(response.Body))
	result := make(map[string]interface{})

	// If no rules, return entire JSON
	if len(rules) == 0 {
		var data map[string]interface{}
		if err := json.Unmarshal(response.Body, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
		return data, nil
	}

	// Extract based on rules
	for _, rule := range rules {
		// Use selector as JSONPath
		value := root.Get(rule.Selector)
		
		if !value.Exists() {
			if rule.Required {
				return nil, fmt.Errorf("required field %s not found", rule.Field)
			}
			if rule.Default != nil {
				result[rule.Field] = rule.Default
			}
			continue
		}

		// Handle multiple values
		if rule.Multiple && value.IsArray() {
			var values []interface{}
			for _, item := range value.Array() {
				values = append(values, item.Value())
			}
			result[rule.Field] = values
		} else {
			// Single value
			result[rule.Field] = value.Value()
		}
	}

	return result, nil
}

// ExtractLinks implements task.Extractor interface - extracts links from JSON
func (e *JSONExtractor) ExtractLinks(ctx context.Context, response *task.Response) ([]string, error) {
	// For JSON, we typically don't extract links unless they're in specific fields
	var links []string
	
	root := gjson.Parse(string(response.Body))
	
	// Look for common link fields
	linkFields := []string{"url", "link", "href", "links", "urls"}
	for _, field := range linkFields {
		value := root.Get(field)
		if value.Exists() {
			if value.IsArray() {
				for _, item := range value.Array() {
					if item.Type == gjson.String {
						links = append(links, item.String())
					}
				}
			} else if value.Type == gjson.String {
				links = append(links, value.String())
			}
		}
	}
	
	return links, nil
}

// GetType implements task.Extractor interface
func (e *JSONExtractor) GetType() string {
	return "json"
}
