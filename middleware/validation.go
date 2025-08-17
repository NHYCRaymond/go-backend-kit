package middleware

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// ValidationConfig holds configuration for input validation
type ValidationConfig struct {
	// MaxRequestSize is the maximum allowed request body size
	MaxRequestSize int64
	// SkipPaths are paths that should bypass validation
	SkipPaths []string
	// CustomPatterns are additional malicious patterns to check
	CustomPatterns []*regexp.Regexp
	// AllowedFileTypes for file uploads (e.g., "image/jpeg", "image/png")
	AllowedFileTypes []string
	// MaxFileSize for file uploads
	MaxFileSize int64
}

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MaxRequestSize: 10 * 1024 * 1024, // 10MB
		MaxFileSize:    5 * 1024 * 1024,  // 5MB
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/ready",
			"/ping",
		},
		AllowedFileTypes: []string{
			"image/jpeg",
			"image/png",
			"image/gif",
			"image/webp",
			"application/pdf",
		},
	}
}

// Common malicious patterns
var defaultMaliciousPatterns = []*regexp.Regexp{
	// XSS patterns
	regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`),
	regexp.MustCompile(`(?i)javascript:`),
	regexp.MustCompile(`(?i)on\w+\s*=`),
	regexp.MustCompile(`(?i)<iframe[^>]*>`),
	regexp.MustCompile(`(?i)<object[^>]*>`),
	regexp.MustCompile(`(?i)<embed[^>]*>`),
	regexp.MustCompile(`(?i)<link[^>]*>`),
	regexp.MustCompile(`(?i)<meta[^>]*>`),

	// SQL Injection patterns
	regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter)\s+`),
	regexp.MustCompile(`(?i)(or|and)\s+\d+\s*=\s*\d+`),
	regexp.MustCompile(`(?i)['";].*?(or|and).*?['";]`),
	regexp.MustCompile(`(?i)--\s*$`),
	regexp.MustCompile(`(?i)/\*.*?\*/`),

	// Path traversal patterns
	regexp.MustCompile(`\.\.[\\/]`),
	regexp.MustCompile(`[\\/]\.\.`),

	// Command injection patterns
	regexp.MustCompile(`[;&|` + "`" + `$]`),
	regexp.MustCompile(`\$\([^)]*\)`),
	regexp.MustCompile(`\$\{[^}]*\}`),

	// LDAP injection patterns
	regexp.MustCompile(`[()&|!<>=~*]`),
}

// InputValidation creates an input validation middleware
func InputValidation(config *ValidationConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultValidationConfig()
	}

	// Compile all patterns
	patterns := append(defaultMaliciousPatterns, config.CustomPatterns...)

	return func(c *gin.Context) {
		// Skip validation for configured paths
		for _, path := range config.SkipPaths {
			if c.Request.URL.Path == path {
				c.Next()
				return
			}
		}

		// Validate URL parameters
		if !validateURLParams(c, patterns) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid URL parameters detected",
			})
			c.Abort()
			return
		}

		// Validate query parameters
		if !validateQueryParams(c, patterns) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid query parameters detected",
			})
			c.Abort()
			return
		}

		// Validate headers
		if !validateHeaders(c, patterns) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid headers detected",
			})
			c.Abort()
			return
		}

		// Check request size
		if c.Request.ContentLength > config.MaxRequestSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("Request body too large, max allowed: %d bytes", config.MaxRequestSize),
			})
			c.Abort()
			return
		}

		// Validate request body for POST/PUT/PATCH
		if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "PATCH" {
			if !validateRequestBody(c, patterns, config) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Invalid request body detected",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// validateURLParams validates URL parameters
func validateURLParams(c *gin.Context, patterns []*regexp.Regexp) bool {
	for _, param := range c.Params {
		if containsMaliciousContent(param.Value, patterns) {
			return false
		}
		// Additional check for path traversal in URL params
		if strings.Contains(param.Value, "..") {
			return false
		}
	}
	return true
}

// validateQueryParams validates query parameters
func validateQueryParams(c *gin.Context, patterns []*regexp.Regexp) bool {
	for key, values := range c.Request.URL.Query() {
		if containsMaliciousContent(key, patterns) {
			return false
		}
		for _, value := range values {
			if containsMaliciousContent(value, patterns) {
				return false
			}
		}
	}
	return true
}

// validateHeaders validates request headers
func validateHeaders(c *gin.Context, patterns []*regexp.Regexp) bool {
	// Check specific headers that are commonly attacked
	suspiciousHeaders := []string{
		"User-Agent",
		"Referer",
		"X-Forwarded-For",
		"X-Real-IP",
		"X-Forwarded-Host",
		"X-Original-URL",
		"X-Rewrite-URL",
	}

	for _, header := range suspiciousHeaders {
		value := c.GetHeader(header)
		if value != "" && containsMaliciousContent(value, patterns) {
			return false
		}
	}

	// Check all custom headers (X- prefix)
	for key, values := range c.Request.Header {
		if strings.HasPrefix(key, "X-") {
			for _, value := range values {
				if containsMaliciousContent(value, patterns) {
					return false
				}
			}
		}
	}

	return true
}

// validateRequestBody validates the request body
func validateRequestBody(c *gin.Context, patterns []*regexp.Regexp, config *ValidationConfig) bool {
	contentType := c.GetHeader("Content-Type")

	// Handle multipart form data (file uploads)
	if strings.Contains(contentType, "multipart/form-data") {
		return validateMultipartForm(c, patterns, config)
	}

	// Only validate text-based content types
	if !isTextContentType(contentType) {
		return true
	}

	// Read and validate body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false
	}

	// Restore body for later use
	c.Request.Body = io.NopCloser(strings.NewReader(string(body)))

	// For form data, use a less strict validation
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return !containsMaliciousContentInFormData(string(body), patterns)
	}

	return !containsMaliciousContent(string(body), patterns)
}

// validateMultipartForm validates multipart form data
func validateMultipartForm(c *gin.Context, patterns []*regexp.Regexp, config *ValidationConfig) bool {
	// Parse multipart form
	err := c.Request.ParseMultipartForm(config.MaxFileSize)
	if err != nil {
		return false
	}

	if c.Request.MultipartForm == nil {
		return true
	}

	// Validate form fields
	for _, values := range c.Request.MultipartForm.Value {
		for _, value := range values {
			if containsMaliciousContent(value, patterns) {
				return false
			}
		}
	}

	// Validate files
	for _, files := range c.Request.MultipartForm.File {
		for _, file := range files {
			// Check file size
			if file.Size > config.MaxFileSize {
				return false
			}

			// Check filename
			if containsMaliciousContent(file.Filename, patterns) {
				return false
			}

			// Check content type
			if !isAllowedFileType(file.Header.Get("Content-Type"), config.AllowedFileTypes) {
				return false
			}
		}
	}

	return true
}

// containsMaliciousContent checks if content matches any malicious pattern
func containsMaliciousContent(content string, patterns []*regexp.Regexp) bool {
	// Convert to lowercase for case-insensitive comparison
	lowerContent := strings.ToLower(content)

	// Check against all patterns
	for _, pattern := range patterns {
		if pattern.MatchString(content) || pattern.MatchString(lowerContent) {
			return true
		}
	}

	// Additional checks
	// Check for null bytes
	if strings.Contains(content, "\x00") {
		return true
	}

	// Check for excessive length (potential buffer overflow attempt)
	if len(content) > 10000 {
		return true
	}

	return false
}

// containsMaliciousContentInFormData checks form data with less strict patterns
func containsMaliciousContentInFormData(content string, patterns []*regexp.Regexp) bool {
	// Convert to lowercase for case-insensitive comparison
	lowerContent := strings.ToLower(content)

	// Skip patterns that would match normal form data characters
	formDataSafePatterns := []*regexp.Regexp{
		// Skip LDAP injection pattern as it contains = which is normal in form data
		regexp.MustCompile(`[()&|!<>=~*]`),
		// Skip command injection patterns that might match normal form data
		regexp.MustCompile(`[;&|` + "`" + `$]`),
	}

	// Check against patterns, skipping form-data-safe ones
	for _, pattern := range patterns {
		// Skip patterns that are too strict for form data
		skipPattern := false
		for _, safePattern := range formDataSafePatterns {
			if pattern.String() == safePattern.String() {
				skipPattern = true
				break
			}
		}
		if skipPattern {
			continue
		}

		if pattern.MatchString(content) || pattern.MatchString(lowerContent) {
			return true
		}
	}

	// Additional checks
	// Check for null bytes
	if strings.Contains(content, "\x00") {
		return true
	}

	// Check for excessive length (potential buffer overflow attempt)
	if len(content) > 10000 {
		return true
	}

	return false
}

// isTextContentType checks if the content type is text-based
func isTextContentType(contentType string) bool {
	textTypes := []string{
		"application/json",
		"application/xml",
		"application/x-www-form-urlencoded",
		"text/",
	}

	for _, textType := range textTypes {
		if strings.Contains(contentType, textType) {
			return true
		}
	}
	return false
}

// isAllowedFileType checks if the file type is allowed
func isAllowedFileType(contentType string, allowedTypes []string) bool {
	if len(allowedTypes) == 0 {
		return true // No restrictions
	}

	for _, allowed := range allowedTypes {
		if strings.Contains(contentType, allowed) {
			return true
		}
	}
	return false
}

// RequestSizeLimit creates a middleware to limit request body size
func RequestSizeLimit(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("Request body too large, max allowed: %d bytes", maxSize),
			})
			c.Abort()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		c.Next()
	}
}

// SanitizeOutput sanitizes strings before sending to client
func SanitizeOutput(input string) string {
	// HTML escape
	replacer := strings.NewReplacer(
		"<", "&lt;",
		">", "&gt;",
		"&", "&amp;",
		"'", "&#39;",
		`"`, "&quot;",
	)
	return replacer.Replace(input)
}
