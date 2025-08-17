package validation

import (
	"regexp"
	"strings"
)

// Common validation patterns that can be reused across the project
var (
	// Email validation pattern
	EmailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	// Phone validation patterns
	ChinaPhonePattern         = regexp.MustCompile(`^1[3-9]\d{9}$`)
	InternationalPhonePattern = regexp.MustCompile(`^\+?[1-9]\d{1,14}$`)

	// Security patterns
	XSSPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?i)javascript:`),
		regexp.MustCompile(`(?i)on\w+\s*=`),
		regexp.MustCompile(`(?i)<iframe[^>]*>`),
		regexp.MustCompile(`(?i)<object[^>]*>`),
		regexp.MustCompile(`(?i)<embed[^>]*>`),
	}

	SQLInjectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter)\s+`),
		regexp.MustCompile(`(?i)(or|and)\s+\d+\s*=\s*\d+`),
		regexp.MustCompile(`(?i)['"";].*?(or|and).*?['"";]`),
		regexp.MustCompile(`(?i)--\s*$`),
		regexp.MustCompile(`(?i)/\*.*?\*/`),
	}

	PathTraversalPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\.\.[\\/]`),
		regexp.MustCompile(`[\\/]\.\.`),
	}
)

// IsValidEmail checks if the email is valid
func IsValidEmail(email string) bool {
	return EmailPattern.MatchString(email)
}

// IsValidChinaPhone checks if the phone number is a valid China mobile number
func IsValidChinaPhone(phone string) bool {
	return ChinaPhonePattern.MatchString(phone)
}

// IsValidInternationalPhone checks if the phone number is valid international format
func IsValidInternationalPhone(phone string) bool {
	return InternationalPhonePattern.MatchString(phone)
}

// ContainsXSS checks if the content contains XSS patterns
func ContainsXSS(content string) bool {
	lowerContent := strings.ToLower(content)
	for _, pattern := range XSSPatterns {
		if pattern.MatchString(content) || pattern.MatchString(lowerContent) {
			return true
		}
	}
	return false
}

// ContainsSQLInjection checks if the content contains SQL injection patterns
func ContainsSQLInjection(content string) bool {
	lowerContent := strings.ToLower(content)
	for _, pattern := range SQLInjectionPatterns {
		if pattern.MatchString(content) || pattern.MatchString(lowerContent) {
			return true
		}
	}
	return false
}

// ContainsPathTraversal checks if the content contains path traversal patterns
func ContainsPathTraversal(content string) bool {
	for _, pattern := range PathTraversalPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return strings.Contains(content, "..")
}

// SanitizeHTML removes dangerous HTML tags and attributes
func SanitizeHTML(input string) string {
	// Basic HTML escape
	replacer := strings.NewReplacer(
		"<", "&lt;",
		">", "&gt;",
		"&", "&amp;",
		"'", "&#39;",
		`"`, "&quot;",
	)
	return replacer.Replace(input)
}

// NormalizePhone normalizes phone numbers for storage
func NormalizePhone(phone string) string {
	// Remove all non-digit characters
	normalized := regexp.MustCompile(`[^0-9+]`).ReplaceAllString(phone, "")

	// Handle China phone numbers
	if strings.HasPrefix(normalized, "86") && len(normalized) == 13 {
		normalized = normalized[2:]
	}
	if strings.HasPrefix(normalized, "+86") && len(normalized) == 14 {
		normalized = normalized[3:]
	}

	return normalized
}

// IsValidLength checks if the string length is within the specified range
func IsValidLength(str string, min, max int) bool {
	length := len(str)
	return length >= min && length <= max
}

// ContainsOnlyAlphanumeric checks if string contains only letters and numbers
func ContainsOnlyAlphanumeric(str string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString(str)
}

// ContainsOnlyDigits checks if string contains only digits
func ContainsOnlyDigits(str string) bool {
	return regexp.MustCompile(`^\d+$`).MatchString(str)
}

// IsValidURL checks if the string is a valid URL
func IsValidURL(str string) bool {
	urlPattern := regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
	return urlPattern.MatchString(str)
}

// RemoveNullBytes removes null bytes from string
func RemoveNullBytes(str string) string {
	return strings.ReplaceAll(str, "\x00", "")
}

// IsEmail is an alias for IsValidEmail for compatibility
func IsEmail(email string) bool {
	return IsValidEmail(email)
}

// IsURL is an alias for IsValidURL for compatibility
func IsURL(url string) bool {
	return IsValidURL(url)
}
