package extractor

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

// HTMLExtractor HTML提取器
type HTMLExtractor struct {
	// 配置
	config *HTMLExtractorConfig
	
	// 预编译的正则表达式
	regexCache map[string]*regexp.Regexp
}

// HTMLExtractorConfig HTML提取器配置
type HTMLExtractorConfig struct {
	// 默认编码
	DefaultEncoding string `json:"default_encoding"`
	
	// 是否移除脚本和样式
	RemoveScripts bool `json:"remove_scripts"`
	RemoveStyles  bool `json:"remove_styles"`
	
	// 是否规范化空白字符
	NormalizeWhitespace bool `json:"normalize_whitespace"`
	
	// 最大文档大小（字节）
	MaxDocumentSize int `json:"max_document_size"`
}

// DefaultHTMLExtractorConfig 默认配置
func DefaultHTMLExtractorConfig() *HTMLExtractorConfig {
	return &HTMLExtractorConfig{
		DefaultEncoding:     "utf-8",
		RemoveScripts:       true,
		RemoveStyles:        true,
		NormalizeWhitespace: true,
		MaxDocumentSize:     10 * 1024 * 1024, // 10MB
	}
}

// NewHTMLExtractor 创建HTML提取器
func NewHTMLExtractor(config *HTMLExtractorConfig) *HTMLExtractor {
	if config == nil {
		config = DefaultHTMLExtractorConfig()
	}
	
	return &HTMLExtractor{
		config:     config,
		regexCache: make(map[string]*regexp.Regexp),
	}
}

// Extract 提取数据
func (e *HTMLExtractor) Extract(content []byte, rules []ExtractRule) ([]map[string]interface{}, error) {
	// 检查文档大小
	if len(content) > e.config.MaxDocumentSize {
		return nil, fmt.Errorf("document size exceeds limit: %d > %d", len(content), e.config.MaxDocumentSize)
	}
	
	// 解析HTML
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}
	
	// 预处理文档
	e.preprocessDocument(doc)
	
	// 提取数据
	results := make([]map[string]interface{}, 0)
	
	// 如果没有规则，返回整个文档的文本
	if len(rules) == 0 {
		result := map[string]interface{}{
			"text":  doc.Text(),
			"title": doc.Find("title").Text(),
			"html":  string(content),
		}
		results = append(results, result)
		return results, nil
	}
	
	// 根据规则提取
	for _, rule := range rules {
		extracted := e.extractByRule(doc, rule)
		if rule.Multiple {
			// 多个结果
			for _, item := range extracted {
				result := map[string]interface{}{
					rule.Field: item,
				}
				results = append(results, result)
			}
		} else {
			// 单个结果
			if len(extracted) > 0 {
				result := map[string]interface{}{
					rule.Field: extracted[0],
				}
				results = append(results, result)
			} else if rule.DefaultValue != "" {
				result := map[string]interface{}{
					rule.Field: rule.DefaultValue,
				}
				results = append(results, result)
			}
		}
	}
	
	return results, nil
}

// ExtractWithCSS 使用CSS选择器提取
func (e *HTMLExtractor) ExtractWithCSS(content []byte, selector string, attr string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	
	results := make([]string, 0)
	
	doc.Find(selector).Each(func(i int, s *goquery.Selection) {
		var value string
		if attr != "" {
			value, _ = s.Attr(attr)
		} else {
			value = s.Text()
		}
		
		if e.config.NormalizeWhitespace {
			value = normalizeWhitespace(value)
		}
		
		if value != "" {
			results = append(results, value)
		}
	})
	
	return results, nil
}

// ExtractWithXPath 使用XPath提取
func (e *HTMLExtractor) ExtractWithXPath(content []byte, xpath string) ([]string, error) {
	// 解析HTML
	doc, err := htmlquery.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML for XPath: %w", err)
	}
	
	// 执行XPath查询
	nodes, err := htmlquery.QueryAll(doc, xpath)
	if err != nil {
		return nil, fmt.Errorf("XPath query failed: %w", err)
	}
	
	results := make([]string, 0, len(nodes))
	for _, node := range nodes {
		text := htmlquery.InnerText(node)
		if e.config.NormalizeWhitespace {
			text = normalizeWhitespace(text)
		}
		if text != "" {
			results = append(results, text)
		}
	}
	
	return results, nil
}

// ExtractWithRegex 使用正则表达式提取
func (e *HTMLExtractor) ExtractWithRegex(content []byte, pattern string) ([]string, error) {
	// 获取或编译正则表达式
	re, exists := e.regexCache[pattern]
	if !exists {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %w", err)
		}
		e.regexCache[pattern] = re
	}
	
	// 查找所有匹配
	matches := re.FindAllStringSubmatch(string(content), -1)
	
	results := make([]string, 0)
	for _, match := range matches {
		if len(match) > 1 {
			// 如果有捕获组，返回第一个捕获组
			results = append(results, match[1])
		} else if len(match) > 0 {
			// 否则返回整个匹配
			results = append(results, match[0])
		}
	}
	
	return results, nil
}

// ExtractLinks 提取所有链接
func (e *HTMLExtractor) ExtractLinks(content []byte, baseURL string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	
	links := make([]string, 0)
	seen := make(map[string]bool)
	
	// 提取所有链接
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}
		
		// 处理相对链接
		if baseURL != "" {
			href = resolveURL(baseURL, href)
		}
		
		// 去重
		if !seen[href] {
			seen[href] = true
			links = append(links, href)
		}
	})
	
	return links, nil
}

// ExtractImages 提取所有图片
func (e *HTMLExtractor) ExtractImages(content []byte, baseURL string) ([]map[string]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	
	images := make([]map[string]string, 0)
	
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		img := make(map[string]string)
		
		if src, exists := s.Attr("src"); exists && src != "" {
			if baseURL != "" {
				src = resolveURL(baseURL, src)
			}
			img["src"] = src
		}
		
		if alt, exists := s.Attr("alt"); exists {
			img["alt"] = alt
		}
		
		if title, exists := s.Attr("title"); exists {
			img["title"] = title
		}
		
		if len(img) > 0 {
			images = append(images, img)
		}
	})
	
	return images, nil
}

// ExtractMeta 提取元数据
func (e *HTMLExtractor) ExtractMeta(content []byte) (map[string]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	
	meta := make(map[string]string)
	
	// 提取标题
	meta["title"] = doc.Find("title").Text()
	
	// 提取meta标签
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		// 处理 name="xxx" content="yyy" 格式
		if name, exists := s.Attr("name"); exists && name != "" {
			if content, exists := s.Attr("content"); exists {
				meta[name] = content
			}
		}
		
		// 处理 property="xxx" content="yyy" 格式（Open Graph等）
		if property, exists := s.Attr("property"); exists && property != "" {
			if content, exists := s.Attr("content"); exists {
				meta[property] = content
			}
		}
		
		// 处理 http-equiv="xxx" content="yyy" 格式
		if httpEquiv, exists := s.Attr("http-equiv"); exists && httpEquiv != "" {
			if content, exists := s.Attr("content"); exists {
				meta["http-equiv:"+httpEquiv] = content
			}
		}
	})
	
	return meta, nil
}

// ExtractTable 提取表格数据
func (e *HTMLExtractor) ExtractTable(content []byte, selector string) ([][]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	
	tables := make([][]string, 0)
	
	// 选择表格
	tableSelector := selector
	if tableSelector == "" {
		tableSelector = "table"
	}
	
	doc.Find(tableSelector).Each(func(i int, table *goquery.Selection) {
		// 提取表格行
		table.Find("tr").Each(func(j int, row *goquery.Selection) {
			rowData := make([]string, 0)
			
			// 提取单元格
			row.Find("td, th").Each(func(k int, cell *goquery.Selection) {
				text := cell.Text()
				if e.config.NormalizeWhitespace {
					text = normalizeWhitespace(text)
				}
				rowData = append(rowData, text)
			})
			
			if len(rowData) > 0 {
				tables = append(tables, rowData)
			}
		})
	})
	
	return tables, nil
}

// extractByRule 根据规则提取数据
func (e *HTMLExtractor) extractByRule(doc *goquery.Document, rule ExtractRule) []string {
	results := make([]string, 0)
	
	switch rule.Type {
	case SelectorTypeCSS:
		doc.Find(rule.Selector).Each(func(i int, s *goquery.Selection) {
			var value string
			if rule.Attribute != "" {
				value, _ = s.Attr(rule.Attribute)
			} else {
				value = s.Text()
			}
			
			if e.config.NormalizeWhitespace {
				value = normalizeWhitespace(value)
			}
			
			if value != "" {
				results = append(results, value)
			}
			
			if !rule.Multiple && len(results) > 0 {
				return
			}
		})
		
	case SelectorTypeXPath:
		// 将goquery文档转换为html.Node
		htmlStr, _ := doc.Html()
		if xpathDoc, err := htmlquery.Parse(strings.NewReader(htmlStr)); err == nil {
			if nodes, err := htmlquery.QueryAll(xpathDoc, rule.Selector); err == nil {
				for _, node := range nodes {
					text := htmlquery.InnerText(node)
					if e.config.NormalizeWhitespace {
						text = normalizeWhitespace(text)
					}
					if text != "" {
						results = append(results, text)
						if !rule.Multiple {
							break
						}
					}
				}
			}
		}
		
	case SelectorTypeRegex:
		htmlStr, _ := doc.Html()
		if matches, err := e.ExtractWithRegex([]byte(htmlStr), rule.Selector); err == nil {
			results = append(results, matches...)
		}
	}
	
	return results
}

// preprocessDocument 预处理文档
func (e *HTMLExtractor) preprocessDocument(doc *goquery.Document) {
	// 移除脚本
	if e.config.RemoveScripts {
		doc.Find("script").Remove()
	}
	
	// 移除样式
	if e.config.RemoveStyles {
		doc.Find("style").Remove()
	}
	
	// 移除注释
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		for _, node := range s.Nodes {
			removeComments(node)
		}
	})
}

// removeComments 递归移除HTML注释
func removeComments(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.CommentNode {
			n.RemoveChild(c)
		} else {
			removeComments(c)
		}
		c = next
	}
}

// normalizeWhitespace 规范化空白字符
func normalizeWhitespace(s string) string {
	// 替换所有空白字符为空格
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	// 去除首尾空白
	return strings.TrimSpace(s)
}

// resolveURL 解析相对URL
func resolveURL(baseURL, relativeURL string) string {
	// 简单的URL解析逻辑
	// 在生产环境中应使用 net/url 包进行更准确的解析
	
	if strings.HasPrefix(relativeURL, "http://") || strings.HasPrefix(relativeURL, "https://") {
		return relativeURL
	}
	
	if strings.HasPrefix(relativeURL, "//") {
		// 协议相对URL
		if strings.HasPrefix(baseURL, "https:") {
			return "https:" + relativeURL
		}
		return "http:" + relativeURL
	}
	
	if strings.HasPrefix(relativeURL, "/") {
		// 绝对路径
		parts := strings.SplitN(baseURL, "/", 4)
		if len(parts) >= 3 {
			return parts[0] + "//" + parts[2] + relativeURL
		}
	}
	
	// 相对路径
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return baseURL + relativeURL
}