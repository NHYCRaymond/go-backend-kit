package pipeline

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/validation"
)

// CleanerProcessor cleans data
type CleanerProcessor struct {
	*BaseProcessor
	config CleanerConfig
}

// CleanerConfig contains cleaner configuration
type CleanerConfig struct {
	TrimSpace      bool
	RemoveEmpty    bool
	NormalizeSpace bool
	ToLower        bool
	ToUpper        bool
}

// NewCleanerProcessor creates a cleaner processor
func NewCleanerProcessor(config CleanerConfig) *CleanerProcessor {
	return &CleanerProcessor{
		BaseProcessor: NewBaseProcessor("cleaner"),
		config:        config,
	}
}

// Process cleans the data
func (cp *CleanerProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	switch v := data.(type) {
	case string:
		return cp.cleanString(v), nil
	case map[string]interface{}:
		return cp.cleanMap(v), nil
	case []interface{}:
		return cp.cleanSlice(v), nil
	default:
		return data, nil
	}
}

func (cp *CleanerProcessor) cleanString(s string) string {
	if cp.config.TrimSpace {
		s = strings.TrimSpace(s)
	}
	if cp.config.NormalizeSpace {
		s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	}
	if cp.config.ToLower {
		s = strings.ToLower(s)
	}
	if cp.config.ToUpper {
		s = strings.ToUpper(s)
	}
	return s
}

func (cp *CleanerProcessor) cleanMap(m map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{})

	for k, v := range m {
		switch val := v.(type) {
		case string:
			cleanedVal := cp.cleanString(val)
			if !cp.config.RemoveEmpty || cleanedVal != "" {
				cleaned[k] = cleanedVal
			}
		case map[string]interface{}:
			cleaned[k] = cp.cleanMap(val)
		case []interface{}:
			cleaned[k] = cp.cleanSlice(val)
		default:
			cleaned[k] = v
		}
	}

	return cleaned
}

func (cp *CleanerProcessor) cleanSlice(s []interface{}) []interface{} {
	cleaned := make([]interface{}, 0, len(s))

	for _, v := range s {
		switch val := v.(type) {
		case string:
			cleanedVal := cp.cleanString(val)
			if !cp.config.RemoveEmpty || cleanedVal != "" {
				cleaned = append(cleaned, cleanedVal)
			}
		case map[string]interface{}:
			cleaned = append(cleaned, cp.cleanMap(val))
		case []interface{}:
			cleaned = append(cleaned, cp.cleanSlice(val))
		default:
			cleaned = append(cleaned, v)
		}
	}

	return cleaned
}

// ValidatorProcessor validates data
type ValidatorProcessor struct {
	*BaseProcessor
	rules []ValidationRule
}

// ValidationRule defines a validation rule
type ValidationRule struct {
	Field    string
	Required bool
	Type     string // string, number, email, url, etc.
	Min      interface{}
	Max      interface{}
	Pattern  string
	Custom   func(interface{}) bool
}

// NewValidatorProcessor creates a validator processor
func NewValidatorProcessor(rules []ValidationRule) *ValidatorProcessor {
	return &ValidatorProcessor{
		BaseProcessor: NewBaseProcessor("validator"),
		rules:         rules,
	}
}

// Process validates the data
func (vp *ValidatorProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	m, ok := data.(map[string]interface{})
	if !ok {
		return data, fmt.Errorf("validator expects map[string]interface{}")
	}

	for _, rule := range vp.rules {
		value, exists := m[rule.Field]

		// Check required
		if rule.Required && !exists {
			return nil, fmt.Errorf("required field missing: %s", rule.Field)
		}

		if !exists {
			continue
		}

		// Type validation
		if rule.Type != "" {
			if err := vp.validateType(value, rule.Type); err != nil {
				return nil, fmt.Errorf("field %s: %w", rule.Field, err)
			}
		}

		// Pattern validation
		if rule.Pattern != "" {
			if s, ok := value.(string); ok {
				if matched, _ := regexp.MatchString(rule.Pattern, s); !matched {
					return nil, fmt.Errorf("field %s does not match pattern", rule.Field)
				}
			}
		}

		// Custom validation
		if rule.Custom != nil && !rule.Custom(value) {
			return nil, fmt.Errorf("field %s failed custom validation", rule.Field)
		}
	}

	return data, nil
}

func (vp *ValidatorProcessor) validateType(value interface{}, dataType string) error {
	switch dataType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string")
		}
	case "number":
		switch value.(type) {
		case int, int64, float64:
		default:
			return fmt.Errorf("expected number")
		}
	case "email":
		if s, ok := value.(string); ok {
			if !validation.IsEmail(s) {
				return fmt.Errorf("invalid email")
			}
		}
	case "url":
		if s, ok := value.(string); ok {
			if !validation.IsURL(s) {
				return fmt.Errorf("invalid URL")
			}
		}
	}
	return nil
}

// DeduplicatorProcessor removes duplicates
type DeduplicatorProcessor struct {
	*BaseProcessor
	keyField string
	seen     map[string]bool
}

// NewDeduplicatorProcessor creates a deduplicator processor
func NewDeduplicatorProcessor(keyField string) *DeduplicatorProcessor {
	return &DeduplicatorProcessor{
		BaseProcessor: NewBaseProcessor("deduplicator"),
		keyField:      keyField,
		seen:          make(map[string]bool),
	}
}

// Process removes duplicates
func (dp *DeduplicatorProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	switch v := data.(type) {
	case map[string]interface{}:
		key := dp.getKey(v)
		if dp.seen[key] {
			return nil, nil // Skip duplicate
		}
		dp.seen[key] = true
		return v, nil

	case []interface{}:
		unique := make([]interface{}, 0)
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				key := dp.getKey(m)
				if !dp.seen[key] {
					dp.seen[key] = true
					unique = append(unique, item)
				}
			} else {
				unique = append(unique, item)
			}
		}
		return unique, nil

	default:
		return data, nil
	}
}

func (dp *DeduplicatorProcessor) getKey(m map[string]interface{}) string {
	if dp.keyField != "" {
		if v, ok := m[dp.keyField]; ok {
			return fmt.Sprintf("%v", v)
		}
	}

	// Use hash of entire map
	h := md5.New()
	h.Write([]byte(fmt.Sprintf("%v", m)))
	return hex.EncodeToString(h.Sum(nil))
}

// TransformProcessor transforms data
type TransformProcessor struct {
	*BaseProcessor
	transforms []Transform
}

// Transform defines a data transformation
type Transform struct {
	Field  string
	Type   string // add_field, remove_field, rename_field, format, calculate
	Params map[string]interface{}
}

// NewTransformProcessor creates a transform processor
func NewTransformProcessor(transforms []Transform) *TransformProcessor {
	return &TransformProcessor{
		BaseProcessor: NewBaseProcessor("transformer"),
		transforms:    transforms,
	}
}

// Process transforms the data
func (tp *TransformProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	m, ok := data.(map[string]interface{})
	if !ok {
		return data, nil
	}

	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}

	for _, transform := range tp.transforms {
		switch transform.Type {
		case "add_field":
			if value, ok := transform.Params["value"]; ok {
				result[transform.Field] = value
			}

		case "remove_field":
			delete(result, transform.Field)

		case "rename_field":
			if newName, ok := transform.Params["new_name"].(string); ok {
				if value, exists := result[transform.Field]; exists {
					result[newName] = value
					delete(result, transform.Field)
				}
			}

		case "format":
			if value, exists := result[transform.Field]; exists {
				result[transform.Field] = tp.formatValue(value, transform.Params)
			}

		case "timestamp":
			result[transform.Field] = time.Now().Unix()

		case "merge":
			if fields, ok := transform.Params["fields"].([]string); ok {
				merged := ""
				for _, f := range fields {
					if v, exists := result[f]; exists {
						merged += fmt.Sprintf("%v ", v)
					}
				}
				result[transform.Field] = strings.TrimSpace(merged)
			}
		}
	}

	return result, nil
}

func (tp *TransformProcessor) formatValue(value interface{}, params map[string]interface{}) interface{} {
	if format, ok := params["format"].(string); ok {
		switch format {
		case "uppercase":
			if s, ok := value.(string); ok {
				return strings.ToUpper(s)
			}
		case "lowercase":
			if s, ok := value.(string); ok {
				return strings.ToLower(s)
			}
		case "trim":
			if s, ok := value.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return value
}

// FilterProcessor filters data
type FilterProcessor struct {
	*BaseProcessor
	condition func(interface{}) bool
}

// NewFilterProcessor creates a filter processor
func NewFilterProcessor(condition func(interface{}) bool) *FilterProcessor {
	return &FilterProcessor{
		BaseProcessor: NewBaseProcessor("filter"),
		condition:     condition,
	}
}

// Process filters the data
func (fp *FilterProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	if fp.condition(data) {
		return data, nil
	}
	return nil, nil // Filter out
}

// EnrichProcessor enriches data with additional information
type EnrichProcessor struct {
	*BaseProcessor
	enrichFunc func(context.Context, interface{}) (interface{}, error)
}

// NewEnrichProcessor creates an enrich processor
func NewEnrichProcessor(enrichFunc func(context.Context, interface{}) (interface{}, error)) *EnrichProcessor {
	return &EnrichProcessor{
		BaseProcessor: NewBaseProcessor("enricher"),
		enrichFunc:    enrichFunc,
	}
}

// Process enriches the data
func (ep *EnrichProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	return ep.enrichFunc(ctx, data)
}

// BatchProcessor processes data in batches
type BatchProcessor struct {
	*BaseProcessor
	batchSize int
	batch     []interface{}
	processor func([]interface{}) ([]interface{}, error)
}

// NewBatchProcessor creates a batch processor
func NewBatchProcessor(batchSize int, processor func([]interface{}) ([]interface{}, error)) *BatchProcessor {
	return &BatchProcessor{
		BaseProcessor: NewBaseProcessor("batch"),
		batchSize:     batchSize,
		batch:         make([]interface{}, 0, batchSize),
		processor:     processor,
	}
}

// Process adds to batch and processes when full
func (bp *BatchProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	bp.batch = append(bp.batch, data)

	if len(bp.batch) >= bp.batchSize {
		processed, err := bp.processor(bp.batch)
		bp.batch = make([]interface{}, 0, bp.batchSize)
		return processed, err
	}

	return nil, nil // Not ready yet
}

// Flush processes remaining items in batch
func (bp *BatchProcessor) Flush() ([]interface{}, error) {
	if len(bp.batch) > 0 {
		processed, err := bp.processor(bp.batch)
		bp.batch = make([]interface{}, 0, bp.batchSize)
		return processed, err
	}
	return nil, nil
}
