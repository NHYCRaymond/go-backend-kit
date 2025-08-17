package extractor

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/PuerkitoBio/goquery"
)

// CSSExtractor implements Extractor using CSS selectors
type CSSExtractor struct{}

// NewCSSExtractor creates a new CSS extractor
func NewCSSExtractor() *CSSExtractor {
	return &CSSExtractor{}
}

// Extract extracts data using CSS selectors
func (e *CSSExtractor) Extract(ctx context.Context, response *task.Response, rules []task.ExtractRule) (map[string]interface{}, error) {
	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(response.HTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}
	
	result := make(map[string]interface{})
	
	for _, rule := range rules {
		if rule.Type != "css" && rule.Type != "" {
			continue // Skip non-CSS rules
		}
		
		if rule.Multiple {
			// Extract multiple values
			var values []string
			doc.Find(rule.Selector).Each(func(i int, s *goquery.Selection) {
				value := e.extractValue(s, rule.Attribute)
				if value != "" {
					values = append(values, value)
				}
			})
			
			if len(values) > 0 || rule.Default != nil {
				if len(values) == 0 && rule.Default != nil {
					result[rule.Field] = rule.Default
				} else {
					result[rule.Field] = values
				}
			} else if rule.Required {
				return nil, fmt.Errorf("required field %s not found", rule.Field)
			}
		} else {
			// Extract single value
			selection := doc.Find(rule.Selector).First()
			value := e.extractValue(selection, rule.Attribute)
			
			if value != "" || rule.Default != nil {
				if value == "" && rule.Default != nil {
					result[rule.Field] = rule.Default
				} else {
					result[rule.Field] = e.applyTransforms(value, rule.Transform)
				}
			} else if rule.Required {
				return nil, fmt.Errorf("required field %s not found", rule.Field)
			}
		}
		
		// Handle nested extraction
		if len(rule.Children) > 0 {
			doc.Find(rule.Selector).Each(func(i int, s *goquery.Selection) {
				childHtml, _ := s.Html()
				childDoc, _ := goquery.NewDocumentFromReader(strings.NewReader(childHtml))
				childResult := make(map[string]interface{})
				
				for _, childRule := range rule.Children {
					childSel := childDoc.Find(childRule.Selector).First()
					childValue := e.extractValue(childSel, childRule.Attribute)
					if childValue != "" {
						childResult[childRule.Field] = childValue
					}
				}
				
				if len(childResult) > 0 {
					if existingChildren, ok := result[rule.Field].([]map[string]interface{}); ok {
						result[rule.Field] = append(existingChildren, childResult)
					} else {
						result[rule.Field] = []map[string]interface{}{childResult}
					}
				}
			})
		}
	}
	
	return result, nil
}

// extractValue extracts value from selection
func (e *CSSExtractor) extractValue(s *goquery.Selection, attribute string) string {
	if attribute != "" {
		value, _ := s.Attr(attribute)
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(s.Text())
}

// applyTransforms applies transformation rules
func (e *CSSExtractor) applyTransforms(value string, transforms []task.TransformRule) string {
	result := value
	
	for _, transform := range transforms {
		switch transform.Type {
		case "trim":
			result = strings.TrimSpace(result)
		case "replace":
			if old, ok := transform.Params["old"].(string); ok {
				if new, ok := transform.Params["new"].(string); ok {
					result = strings.ReplaceAll(result, old, new)
				}
			}
		case "regex":
			if pattern, ok := transform.Params["pattern"].(string); ok {
				if re, err := regexp.Compile(pattern); err == nil {
					matches := re.FindStringSubmatch(result)
					if len(matches) > 1 {
						result = matches[1]
					}
				}
			}
		case "split":
			if sep, ok := transform.Params["separator"].(string); ok {
				if index, ok := transform.Params["index"].(int); ok {
					parts := strings.Split(result, sep)
					if index < len(parts) {
						result = parts[index]
					}
				}
			}
		}
	}
	
	return result
}

// ExtractLinks extracts all links from the page
func (e *CSSExtractor) ExtractLinks(ctx context.Context, response *task.Response) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(response.HTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}
	
	var links []string
	seen := make(map[string]bool)
	
	// Extract all href attributes
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			href = strings.TrimSpace(href)
			
			// Skip empty, javascript, and anchor links
			if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
				return
			}
			
			// Convert relative URLs to absolute
			if !strings.HasPrefix(href, "http") {
				// Simple relative URL handling (should use proper URL parsing in production)
				if strings.HasPrefix(href, "/") {
					// Get base URL from response
					baseURL := response.URL
					if idx := strings.Index(baseURL[8:], "/"); idx > 0 {
						baseURL = baseURL[:idx+8]
					}
					href = baseURL + href
				}
			}
			
			// Deduplicate
			if !seen[href] {
				seen[href] = true
				links = append(links, href)
			}
		}
	})
	
	return links, nil
}

// GetType returns the extractor type
func (e *CSSExtractor) GetType() string {
	return "css"
}