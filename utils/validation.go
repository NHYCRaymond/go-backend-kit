package utils

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
)

// ValidateEmail validates email format
func ValidateEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// ValidatePassword validates password strength
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}

	return nil
}

// ValidatePasswordSimple validates password with simple rules
func ValidatePasswordSimple(password string, minLength int) error {
	if len(password) < minLength {
		return fmt.Errorf("password must be at least %d characters long", minLength)
	}
	return nil
}

// ValidateUsername validates username format
func ValidateUsername(username string) error {
	if len(username) < 3 {
		return fmt.Errorf("username must be at least 3 characters long")
	}
	if len(username) > 20 {
		return fmt.Errorf("username must be no more than 20 characters long")
	}

	// Allow alphanumeric characters and underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]+$`, username)
	if !matched {
		return fmt.Errorf("username can only contain letters, numbers, and underscores")
	}

	return nil
}

// ValidatePhoneNumber validates phone number format
func ValidatePhoneNumber(phone string) error {
	// Remove all non-digit characters
	phone = regexp.MustCompile(`\D`).ReplaceAllString(phone, "")
	
	if len(phone) < 10 {
		return fmt.Errorf("phone number must be at least 10 digits long")
	}
	if len(phone) > 15 {
		return fmt.Errorf("phone number must be no more than 15 digits long")
	}

	return nil
}

// ValidateURL validates URL format
func ValidateURL(url string) error {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("URL must start with http:// or https://")
	}

	// Basic URL validation
	matched, _ := regexp.MatchString(`^https?://[^\s/$.?#].[^\s]*$`, url)
	if !matched {
		return fmt.Errorf("invalid URL format")
	}

	return nil
}

// ValidateRequired checks if a field is required and not empty
func ValidateRequired(value interface{}, fieldName string) error {
	if value == nil {
		return fmt.Errorf("%s is required", fieldName)
	}

	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required", fieldName)
		}
	case []string:
		if len(v) == 0 {
			return fmt.Errorf("%s is required", fieldName)
		}
	default:
		if v == nil {
			return fmt.Errorf("%s is required", fieldName)
		}
	}

	return nil
}

// ValidateMinLength validates minimum length
func ValidateMinLength(value string, minLength int, fieldName string) error {
	if len(value) < minLength {
		return fmt.Errorf("%s must be at least %d characters long", fieldName, minLength)
	}
	return nil
}

// ValidateMaxLength validates maximum length
func ValidateMaxLength(value string, maxLength int, fieldName string) error {
	if len(value) > maxLength {
		return fmt.Errorf("%s must be no more than %d characters long", fieldName, maxLength)
	}
	return nil
}

// ValidateRange validates numeric range
func ValidateRange(value, min, max int, fieldName string) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d", fieldName, min, max)
	}
	return nil
}

// ValidateFloatRange validates float range
func ValidateFloatRange(value, min, max float64, fieldName string) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %.2f and %.2f", fieldName, min, max)
	}
	return nil
}

// ValidateEnum validates enum values
func ValidateEnum(value string, allowedValues []string, fieldName string) error {
	for _, allowed := range allowedValues {
		if value == allowed {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of: %s", fieldName, strings.Join(allowedValues, ", "))
}

// ValidateRegex validates against a regex pattern
func ValidateRegex(value, pattern, fieldName string) error {
	matched, err := regexp.MatchString(pattern, value)
	if err != nil {
		return fmt.Errorf("invalid regex pattern for %s", fieldName)
	}
	if !matched {
		return fmt.Errorf("%s format is invalid", fieldName)
	}
	return nil
}

// ValidateUUID validates UUID format
func ValidateUUID(value string) error {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !uuidRegex.MatchString(value) {
		return fmt.Errorf("invalid UUID format")
	}
	return nil
}

// ValidateSlug validates slug format
func ValidateSlug(value string) error {
	slugRegex := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	if !slugRegex.MatchString(value) {
		return fmt.Errorf("invalid slug format (only lowercase letters, numbers, and hyphens allowed)")
	}
	return nil
}

// ValidateHexColor validates hex color format
func ValidateHexColor(value string) error {
	hexColorRegex := regexp.MustCompile(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`)
	if !hexColorRegex.MatchString(value) {
		return fmt.Errorf("invalid hex color format")
	}
	return nil
}

// ValidateIPAddress validates IP address format
func ValidateIPAddress(value string) error {
	ipRegex := regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)
	if !ipRegex.MatchString(value) {
		return fmt.Errorf("invalid IP address format")
	}
	return nil
}

// ValidateJSON validates JSON format
func ValidateJSON(value string) error {
	// This is a basic JSON validation - for production use, consider using json.Valid()
	if !strings.HasPrefix(value, "{") && !strings.HasPrefix(value, "[") {
		return fmt.Errorf("invalid JSON format")
	}
	if !strings.HasSuffix(value, "}") && !strings.HasSuffix(value, "]") {
		return fmt.Errorf("invalid JSON format")
	}
	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string      `json:"field"`
	Message string      `json:"message"`
	Value   interface{} `json:"value,omitempty"`
}

// ValidationResult represents validation result
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// Validator represents a field validator
type Validator struct {
	errors []ValidationError
}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{
		errors: make([]ValidationError, 0),
	}
}

// AddError adds a validation error
func (v *Validator) AddError(field, message string, value interface{}) {
	v.errors = append(v.errors, ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	})
}

// ValidateField validates a single field
func (v *Validator) ValidateField(field string, value interface{}, rules ...ValidationRule) {
	for _, rule := range rules {
		if err := rule(value); err != nil {
			v.AddError(field, err.Error(), value)
		}
	}
}

// IsValid returns true if validation passed
func (v *Validator) IsValid() bool {
	return len(v.errors) == 0
}

// GetErrors returns validation errors
func (v *Validator) GetErrors() []ValidationError {
	return v.errors
}

// GetResult returns validation result
func (v *Validator) GetResult() ValidationResult {
	return ValidationResult{
		Valid:  v.IsValid(),
		Errors: v.errors,
	}
}

// ValidationRule represents a validation rule function
type ValidationRule func(value interface{}) error

// Required creates a required validation rule
func Required() ValidationRule {
	return func(value interface{}) error {
		return ValidateRequired(value, "field")
	}
}

// MinLength creates a min length validation rule
func MinLength(minLength int) ValidationRule {
	return func(value interface{}) error {
		if str, ok := value.(string); ok {
			return ValidateMinLength(str, minLength, "field")
		}
		return fmt.Errorf("value must be a string")
	}
}

// MaxLength creates a max length validation rule
func MaxLength(maxLength int) ValidationRule {
	return func(value interface{}) error {
		if str, ok := value.(string); ok {
			return ValidateMaxLength(str, maxLength, "field")
		}
		return fmt.Errorf("value must be a string")
	}
}

// Email creates an email validation rule
func Email() ValidationRule {
	return func(value interface{}) error {
		if str, ok := value.(string); ok {
			if !ValidateEmail(str) {
				return fmt.Errorf("invalid email format")
			}
			return nil
		}
		return fmt.Errorf("value must be a string")
	}
}

// Password creates a password validation rule
func Password() ValidationRule {
	return func(value interface{}) error {
		if str, ok := value.(string); ok {
			return ValidatePassword(str)
		}
		return fmt.Errorf("value must be a string")
	}
}

// Enum creates an enum validation rule
func Enum(allowedValues []string) ValidationRule {
	return func(value interface{}) error {
		if str, ok := value.(string); ok {
			return ValidateEnum(str, allowedValues, "field")
		}
		return fmt.Errorf("value must be a string")
	}
}

// Regex creates a regex validation rule
func Regex(pattern string) ValidationRule {
	return func(value interface{}) error {
		if str, ok := value.(string); ok {
			return ValidateRegex(str, pattern, "field")
		}
		return fmt.Errorf("value must be a string")
	}
}