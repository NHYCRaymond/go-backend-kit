package response

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Response represents the standard API response format
type Response struct {
	Code      int         `json:"code"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	TraceID   string      `json:"trace_id,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// PaginatedResponse represents a paginated API response
type PaginatedResponse struct {
	Code       int         `json:"code"`
	Message    string      `json:"message,omitempty"`
	Data       interface{} `json:"data,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
	RequestID  string      `json:"request_id,omitempty"`
	TraceID    string      `json:"trace_id,omitempty"`
	Timestamp  int64       `json:"timestamp"`
}

// Pagination represents pagination information
type Pagination struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// Error codes
const (
	CodeSuccess           = 0
	CodeBadRequest        = 400
	CodeUnauthorized      = 401
	CodeForbidden         = 403
	CodeNotFound          = 404
	CodeConflict          = 409
	CodeTooManyRequests   = 429
	CodeInternalError     = 500
	CodeServiceUnavailable = 503
)

// Error messages
const (
	MsgSuccess           = "success"
	MsgBadRequest        = "bad request"
	MsgUnauthorized      = "unauthorized"
	MsgForbidden         = "forbidden"
	MsgNotFound          = "not found"
	MsgConflict          = "conflict"
	MsgTooManyRequests   = "too many requests"
	MsgInternalError     = "internal server error"
	MsgServiceUnavailable = "service unavailable"
)

// getRequestMeta extracts request metadata from context
func getRequestMeta(c *gin.Context) (string, string, int64) {
	var requestID, traceID string
	
	if id, exists := c.Get("request_id"); exists {
		if reqID, ok := id.(string); ok {
			requestID = reqID
		}
	}
	
	if id, exists := c.Get("trace_id"); exists {
		if trcID, ok := id.(string); ok {
			traceID = trcID
		}
	}
	
	return requestID, traceID, time.Now().Unix()
}

// Success sends a successful response
func Success(c *gin.Context, data interface{}) {
	requestID, traceID, timestamp := getRequestMeta(c)
	c.JSON(http.StatusOK, Response{
		Code:      CodeSuccess,
		Data:      data,
		RequestID: requestID,
		TraceID:   traceID,
		Timestamp: timestamp,
	})
}

// SuccessWithMessage sends a successful response with custom message
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	requestID, traceID, timestamp := getRequestMeta(c)
	c.JSON(http.StatusOK, Response{
		Code:      CodeSuccess,
		Message:   message,
		Data:      data,
		RequestID: requestID,
		TraceID:   traceID,
		Timestamp: timestamp,
	})
}

// Error sends an error response
func Error(c *gin.Context, httpStatus int, code interface{}, message string) {
	var errorCode int
	
	switch v := code.(type) {
	case int:
		errorCode = v
	case string:
		errorCode = httpStatus
	default:
		errorCode = httpStatus
	}

	requestID, traceID, timestamp := getRequestMeta(c)
	c.JSON(httpStatus, Response{
		Code:      errorCode,
		Message:   message,
		RequestID: requestID,
		TraceID:   traceID,
		Timestamp: timestamp,
	})
}

// BadRequest sends a bad request response
func BadRequest(c *gin.Context, message string) {
	if message == "" {
		message = MsgBadRequest
	}
	Error(c, http.StatusBadRequest, CodeBadRequest, message)
}

// Unauthorized sends an unauthorized response
func Unauthorized(c *gin.Context, message string) {
	if message == "" {
		message = MsgUnauthorized
	}
	Error(c, http.StatusUnauthorized, CodeUnauthorized, message)
}

// Forbidden sends a forbidden response
func Forbidden(c *gin.Context, message string) {
	if message == "" {
		message = MsgForbidden
	}
	Error(c, http.StatusForbidden, CodeForbidden, message)
}

// NotFound sends a not found response
func NotFound(c *gin.Context, message string) {
	if message == "" {
		message = MsgNotFound
	}
	Error(c, http.StatusNotFound, CodeNotFound, message)
}

// Conflict sends a conflict response
func Conflict(c *gin.Context, message string) {
	if message == "" {
		message = MsgConflict
	}
	Error(c, http.StatusConflict, CodeConflict, message)
}

// TooManyRequests sends a too many requests response
func TooManyRequests(c *gin.Context, message string) {
	if message == "" {
		message = MsgTooManyRequests
	}
	Error(c, http.StatusTooManyRequests, CodeTooManyRequests, message)
}

// InternalError sends an internal server error response
func InternalError(c *gin.Context, message string) {
	if message == "" {
		message = MsgInternalError
	}
	Error(c, http.StatusInternalServerError, CodeInternalError, message)
}

// ServiceUnavailable sends a service unavailable response
func ServiceUnavailable(c *gin.Context, message string) {
	if message == "" {
		message = MsgServiceUnavailable
	}
	Error(c, http.StatusServiceUnavailable, CodeServiceUnavailable, message)
}

// Paginated sends a paginated response
func Paginated(c *gin.Context, data interface{}, pagination *Pagination) {
	requestID, traceID, timestamp := getRequestMeta(c)
	c.JSON(http.StatusOK, PaginatedResponse{
		Code:       CodeSuccess,
		Data:       data,
		Pagination: pagination,
		RequestID:  requestID,
		TraceID:    traceID,
		Timestamp:  timestamp,
	})
}

// PaginatedWithMessage sends a paginated response with custom message
func PaginatedWithMessage(c *gin.Context, message string, data interface{}, pagination *Pagination) {
	requestID, traceID, timestamp := getRequestMeta(c)
	c.JSON(http.StatusOK, PaginatedResponse{
		Code:       CodeSuccess,
		Message:    message,
		Data:       data,
		Pagination: pagination,
		RequestID:  requestID,
		TraceID:    traceID,
		Timestamp:  timestamp,
	})
}

// Created sends a created response
func Created(c *gin.Context, data interface{}) {
	requestID, traceID, timestamp := getRequestMeta(c)
	c.JSON(http.StatusCreated, Response{
		Code:      CodeSuccess,
		Data:      data,
		RequestID: requestID,
		TraceID:   traceID,
		Timestamp: timestamp,
	})
}

// NoContent sends a no content response
func NoContent(c *gin.Context) {
	c.JSON(http.StatusNoContent, nil)
}

// JSON sends a custom JSON response
func JSON(c *gin.Context, httpStatus int, code int, message string, data interface{}) {
	requestID, traceID, timestamp := getRequestMeta(c)
	c.JSON(httpStatus, Response{
		Code:      code,
		Message:   message,
		Data:      data,
		RequestID: requestID,
		TraceID:   traceID,
		Timestamp: timestamp,
	})
}

// NewPagination creates a new pagination object
func NewPagination(page, perPage int, total int64) *Pagination {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 10
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if totalPages < 1 {
		totalPages = 1
	}

	return &Pagination{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}
}

// CalculateOffset calculates the offset for database queries
func CalculateOffset(page, perPage int) int {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 10
	}
	return (page - 1) * perPage
}