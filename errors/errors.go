package errors

import (
	"fmt"
	"net/http"
)

// AppError represents an application error
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Cause      error  `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Cause
}

// WithCause adds a cause to the error
func (e *AppError) WithCause(cause error) *AppError {
	return &AppError{
		Code:       e.Code,
		Message:    e.Message,
		HTTPStatus: e.HTTPStatus,
		Cause:      cause,
	}
}

// New creates a new application error
func New(code, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: http.StatusInternalServerError,
	}
}

// NewWithStatus creates a new application error with HTTP status
func NewWithStatus(code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// Wrap wraps an existing error with application error
func Wrap(err error, code, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: http.StatusInternalServerError,
		Cause:      err,
	}
}

// WrapWithStatus wraps an existing error with application error and HTTP status
func WrapWithStatus(err error, code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Cause:      err,
	}
}

// Predefined errors
var (
	// Generic errors
	ErrInternal           = NewWithStatus("INTERNAL_ERROR", "Internal server error", http.StatusInternalServerError)
	ErrNotFound           = NewWithStatus("NOT_FOUND", "Resource not found", http.StatusNotFound)
	ErrBadRequest         = NewWithStatus("BAD_REQUEST", "Bad request", http.StatusBadRequest)
	ErrUnauthorized       = NewWithStatus("UNAUTHORIZED", "Unauthorized", http.StatusUnauthorized)
	ErrForbidden          = NewWithStatus("FORBIDDEN", "Forbidden", http.StatusForbidden)
	ErrConflict           = NewWithStatus("CONFLICT", "Resource conflict", http.StatusConflict)
	ErrTooManyRequests    = NewWithStatus("TOO_MANY_REQUESTS", "Too many requests", http.StatusTooManyRequests)
	ErrServiceUnavailable = NewWithStatus("SERVICE_UNAVAILABLE", "Service unavailable", http.StatusServiceUnavailable)

	// Authentication errors
	ErrInvalidToken       = NewWithStatus("INVALID_TOKEN", "Invalid authentication token", http.StatusUnauthorized)
	ErrExpiredToken       = NewWithStatus("EXPIRED_TOKEN", "Authentication token has expired", http.StatusUnauthorized)
	ErrInvalidCredentials = NewWithStatus("INVALID_CREDENTIALS", "Invalid username or password", http.StatusUnauthorized)
	ErrUserNotFound       = NewWithStatus("USER_NOT_FOUND", "User not found", http.StatusNotFound)
	ErrEmailExists        = NewWithStatus("EMAIL_EXISTS", "Email already exists", http.StatusConflict)
	ErrUsernameExists     = NewWithStatus("USERNAME_EXISTS", "Username already exists", http.StatusConflict)

	// Validation errors
	ErrValidationFailed = NewWithStatus("VALIDATION_FAILED", "Validation failed", http.StatusBadRequest)
	ErrMissingField     = NewWithStatus("MISSING_FIELD", "Required field is missing", http.StatusBadRequest)
	ErrInvalidFormat    = NewWithStatus("INVALID_FORMAT", "Invalid format", http.StatusBadRequest)
	ErrInvalidValue     = NewWithStatus("INVALID_VALUE", "Invalid value", http.StatusBadRequest)

	// Database errors
	ErrDatabaseConnection  = NewWithStatus("DATABASE_CONNECTION", "Database connection failed", http.StatusServiceUnavailable)
	ErrDatabaseQuery       = NewWithStatus("DATABASE_QUERY", "Database query failed", http.StatusInternalServerError)
	ErrDatabaseTransaction = NewWithStatus("DATABASE_TRANSACTION", "Database transaction failed", http.StatusInternalServerError)
	ErrRecordNotFound      = NewWithStatus("RECORD_NOT_FOUND", "Record not found", http.StatusNotFound)
	ErrDuplicateKey        = NewWithStatus("DUPLICATE_KEY", "Duplicate key violation", http.StatusConflict)

	// Permission errors
	ErrInsufficientPermissions = NewWithStatus("INSUFFICIENT_PERMISSIONS", "Insufficient permissions", http.StatusForbidden)
	ErrResourceOwnership       = NewWithStatus("RESOURCE_OWNERSHIP", "You don't own this resource", http.StatusForbidden)
	ErrAdminRequired           = NewWithStatus("ADMIN_REQUIRED", "Admin privileges required", http.StatusForbidden)

	// Rate limiting errors
	ErrRateLimitExceeded = NewWithStatus("RATE_LIMIT_EXCEEDED", "Rate limit exceeded", http.StatusTooManyRequests)

	// File upload errors
	ErrFileUploadFailed = NewWithStatus("FILE_UPLOAD_FAILED", "File upload failed", http.StatusInternalServerError)
	ErrInvalidFileType  = NewWithStatus("INVALID_FILE_TYPE", "Invalid file type", http.StatusBadRequest)
	ErrFileTooLarge     = NewWithStatus("FILE_TOO_LARGE", "File too large", http.StatusBadRequest)

	// External service errors
	ErrExternalService = NewWithStatus("EXTERNAL_SERVICE", "External service error", http.StatusServiceUnavailable)
	ErrThirdPartyAPI   = NewWithStatus("THIRD_PARTY_API", "Third party API error", http.StatusServiceUnavailable)

	// Business logic errors
	ErrBusinessRule    = NewWithStatus("BUSINESS_RULE", "Business rule violation", http.StatusBadRequest)
	ErrInvalidState    = NewWithStatus("INVALID_STATE", "Invalid state", http.StatusBadRequest)
	ErrOperationFailed = NewWithStatus("OPERATION_FAILED", "Operation failed", http.StatusInternalServerError)
)

// IsAppError checks if an error is an application error
func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

// AsAppError converts an error to an application error
func AsAppError(err error) (*AppError, bool) {
	if appErr, ok := err.(*AppError); ok {
		return appErr, true
	}
	return nil, false
}

// GetHTTPStatus gets the HTTP status code from an error
func GetHTTPStatus(err error) int {
	if appErr, ok := AsAppError(err); ok {
		return appErr.HTTPStatus
	}
	return http.StatusInternalServerError
}

// GetErrorCode gets the error code from an error
func GetErrorCode(err error) string {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Code
	}
	return "UNKNOWN_ERROR"
}

// GetErrorMessage gets the error message from an error
func GetErrorMessage(err error) string {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Message
	}
	return err.Error()
}

// ValidationError represents a validation error with field details
type ValidationError struct {
	Field   string      `json:"field"`
	Message string      `json:"message"`
	Value   interface{} `json:"value,omitempty"`
}

// ValidationErrors represents multiple validation errors
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

// Error implements the error interface
func (ve *ValidationErrors) Error() string {
	if len(ve.Errors) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed: %s", ve.Errors[0].Message)
}

// Add adds a validation error
func (ve *ValidationErrors) Add(field, message string, value interface{}) {
	ve.Errors = append(ve.Errors, ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	})
}

// HasErrors checks if there are any validation errors
func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

// NewValidationErrors creates a new validation errors container
func NewValidationErrors() *ValidationErrors {
	return &ValidationErrors{
		Errors: make([]ValidationError, 0),
	}
}

// NewValidationError creates a new validation error
func NewValidationError(field, message string, value interface{}) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	}
}

// Error codes for crawler system
const (
	ValidationErrorCode       = "VALIDATION_ERROR"
	NotFoundErrorCode         = "NOT_FOUND"
	AlreadyExistsErrorCode    = "ALREADY_EXISTS"
	NotImplementedErrorCode   = "NOT_IMPLEMENTED"
	ResourceExhaustedErrorCode = "RESOURCE_EXHAUSTED"
)

// IsRecoverable checks if an error is recoverable
func IsRecoverable(err error) bool {
	if err == nil {
		return true
	}
	
	// Check if it's an AppError
	if appErr, ok := err.(*AppError); ok {
		// Consider 5xx errors as recoverable
		return appErr.HTTPStatus >= 500 && appErr.HTTPStatus < 600
	}
	
	// By default, consider unknown errors as recoverable
	return true
}
