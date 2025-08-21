package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// AuditAction represents different types of audit actions
type AuditAction string

const (
	AuditActionLogin         AuditAction = "LOGIN"
	AuditActionLogout        AuditAction = "LOGOUT"
	AuditActionTokenRefresh  AuditAction = "TOKEN_REFRESH"
	AuditActionTokenRevoke   AuditAction = "TOKEN_REVOKE"
	AuditActionPasswordReset AuditAction = "PASSWORD_RESET"
	AuditActionPermissionChange AuditAction = "PERMISSION_CHANGE"
	AuditActionDataAccess    AuditAction = "DATA_ACCESS"
	AuditActionDataModify    AuditAction = "DATA_MODIFY"
	AuditActionAdminAction   AuditAction = "ADMIN_ACTION"
)

// AuditEntry represents an audit log entry
type AuditEntry struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Action      AuditAction            `json:"action"`
	UserID      string                 `json:"user_id"`
	UserType    string                 `json:"user_type,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
	SessionID   string                 `json:"session_id,omitempty"`
	IP          string                 `json:"ip"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Query       string                 `json:"query,omitempty"`
	StatusCode  int                    `json:"status_code,omitempty"`
	Duration    int64                  `json:"duration_ms,omitempty"`
	Success     bool                   `json:"success"`
	ErrorMsg    string                 `json:"error_msg,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Metadata    map[string]string      `json:"metadata,omitempty"`
}

// AuditLogger handles audit logging
type AuditLogger struct {
	logger      *slog.Logger
	redis       *redis.Client
	enabled     bool
	logToRedis  bool
	logToFile   bool
	retentionDays int
}

// AuditConfig holds audit logging configuration
type AuditConfig struct {
	Enabled       bool
	LogToRedis    bool
	LogToFile     bool
	RetentionDays int
	SensitiveFields []string // Fields to redact from logs
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logger *slog.Logger, redis *redis.Client, config AuditConfig) *AuditLogger {
	if config.RetentionDays == 0 {
		config.RetentionDays = 90 // Default 90 days retention
	}
	
	return &AuditLogger{
		logger:        logger,
		redis:         redis,
		enabled:       config.Enabled,
		logToRedis:    config.LogToRedis,
		logToFile:     config.LogToFile,
		retentionDays: config.RetentionDays,
	}
}

// LogAuth logs authentication-related events
func (a *AuditLogger) LogAuth(ctx context.Context, action AuditAction, userID, userType string, success bool, details map[string]interface{}) {
	if !a.enabled {
		return
	}
	
	entry := AuditEntry{
		ID:        generateAuditID(),
		Timestamp: time.Now(),
		Action:    action,
		UserID:    userID,
		UserType:  userType,
		Success:   success,
		Details:   details,
	}
	
	// Add context information if available
	if ginCtx, ok := ctx.(*gin.Context); ok {
		a.enrichFromGinContext(&entry, ginCtx)
	}
	
	a.writeEntry(ctx, entry)
}

// LogDataAccess logs data access events
func (a *AuditLogger) LogDataAccess(ctx context.Context, userID string, resource string, action string, details map[string]interface{}) {
	if !a.enabled {
		return
	}
	
	entry := AuditEntry{
		ID:        generateAuditID(),
		Timestamp: time.Now(),
		Action:    AuditActionDataAccess,
		UserID:    userID,
		Success:   true,
		Details: map[string]interface{}{
			"resource": resource,
			"action":   action,
		},
	}
	
	// Merge additional details
	for k, v := range details {
		entry.Details[k] = v
	}
	
	// Add context information if available
	if ginCtx, ok := ctx.(*gin.Context); ok {
		a.enrichFromGinContext(&entry, ginCtx)
	}
	
	a.writeEntry(ctx, entry)
}

// AuditMiddleware returns a Gin middleware for audit logging
func (a *AuditLogger) AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.enabled {
			c.Next()
			return
		}
		
		// Skip audit for certain paths
		skipPaths := []string{"/health", "/metrics", "/ping"}
		for _, path := range skipPaths {
			if c.Request.URL.Path == path {
				c.Next()
				return
			}
		}
		
		start := time.Now()
		
		// Process request
		c.Next()
		
		// Create audit entry
		entry := AuditEntry{
			ID:         generateAuditID(),
			Timestamp:  start,
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
			Query:      c.Request.URL.RawQuery,
			StatusCode: c.Writer.Status(),
			Duration:   time.Since(start).Milliseconds(),
			Success:    c.Writer.Status() < 400,
		}
		
		// Get user information from context
		if claims, exists := c.Get("user_claims"); exists {
			if enhancedClaims, ok := claims.(*EnhancedClaims); ok {
				entry.UserID = enhancedClaims.UserID
				entry.UserType = enhancedClaims.UserType
				entry.SessionID = enhancedClaims.SessionID
			}
		} else if claims, exists := c.Get("user_claims_old"); exists {
			if oldClaims, ok := claims.(*Claims); ok {
				entry.UserID = oldClaims.UserID
				entry.UserType = oldClaims.Role
			}
		}
		
		// Add request context
		a.enrichFromGinContext(&entry, c)
		
		// Determine action based on path and method
		entry.Action = a.determineAction(c.Request.URL.Path, c.Request.Method)
		
		// Add any errors
		if len(c.Errors) > 0 {
			entry.Success = false
			entry.ErrorMsg = c.Errors.String()
		}
		
		a.writeEntry(c.Request.Context(), entry)
	}
}

// enrichFromGinContext adds context information from Gin context
func (a *AuditLogger) enrichFromGinContext(entry *AuditEntry, c *gin.Context) {
	entry.IP = c.ClientIP()
	entry.UserAgent = c.GetHeader("User-Agent")
	entry.RequestID = c.GetString("request_id")
	
	// Add custom metadata
	entry.Metadata = map[string]string{
		"xff":        c.GetHeader("X-Forwarded-For"),
		"real_ip":    c.GetHeader("X-Real-IP"),
		"referer":    c.GetHeader("Referer"),
		"origin":     c.GetHeader("Origin"),
	}
}

// determineAction determines the audit action based on path and method
func (a *AuditLogger) determineAction(path, method string) AuditAction {
	// Authentication endpoints
	if path == "/auth/login" || path == "/api/auth/login" {
		return AuditActionLogin
	}
	if path == "/auth/logout" || path == "/api/auth/logout" {
		return AuditActionLogout
	}
	if path == "/auth/refresh" || path == "/api/auth/refresh" {
		return AuditActionTokenRefresh
	}
	if path == "/auth/revoke" || path == "/api/auth/revoke" {
		return AuditActionTokenRevoke
	}
	
	// Data operations
	switch method {
	case "GET":
		return AuditActionDataAccess
	case "POST", "PUT", "PATCH", "DELETE":
		return AuditActionDataModify
	default:
		return AuditActionDataAccess
	}
}

// writeEntry writes the audit entry to configured destinations
func (a *AuditLogger) writeEntry(ctx context.Context, entry AuditEntry) {
	// Log to structured logger
	if a.logger != nil {
		attrs := []slog.Attr{
			slog.String("audit_id", entry.ID),
			slog.String("action", string(entry.Action)),
			slog.String("user_id", entry.UserID),
			slog.String("ip", entry.IP),
			slog.String("path", entry.Path),
			slog.Bool("success", entry.Success),
		}
		
		if entry.Duration > 0 {
			attrs = append(attrs, slog.Int64("duration_ms", entry.Duration))
		}
		
		if entry.ErrorMsg != "" {
			attrs = append(attrs, slog.String("error", entry.ErrorMsg))
		}
		
		if entry.Success {
			a.logger.LogAttrs(ctx, slog.LevelInfo, "audit", attrs...)
		} else {
			a.logger.LogAttrs(ctx, slog.LevelWarn, "audit_failure", attrs...)
		}
	}
	
	// Store in Redis for querying
	if a.logToRedis && a.redis != nil {
		a.storeInRedis(ctx, entry)
	}
}

// storeInRedis stores the audit entry in Redis
func (a *AuditLogger) storeInRedis(ctx context.Context, entry AuditEntry) {
	// Convert entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		a.logger.Error("failed to marshal audit entry", "error", err)
		return
	}
	
	// Store in Redis sorted set by timestamp
	key := fmt.Sprintf("audit:log:%s", entry.Timestamp.Format("2006-01-02"))
	score := float64(entry.Timestamp.Unix())
	
	if err := a.redis.ZAdd(ctx, key, &redis.Z{
		Score:  score,
		Member: string(data),
	}).Err(); err != nil {
		a.logger.Error("failed to store audit entry in Redis", "error", err)
		return
	}
	
	// Set expiration
	expiration := time.Duration(a.retentionDays) * 24 * time.Hour
	a.redis.Expire(ctx, key, expiration)
	
	// Also index by user for user-specific queries
	if entry.UserID != "" {
		userKey := fmt.Sprintf("audit:user:%s:%s", entry.UserID, entry.Timestamp.Format("2006-01-02"))
		a.redis.ZAdd(ctx, userKey, &redis.Z{
			Score:  score,
			Member: entry.ID,
		})
		a.redis.Expire(ctx, userKey, expiration)
	}
	
	// Index by action for action-specific queries
	actionKey := fmt.Sprintf("audit:action:%s:%s", entry.Action, entry.Timestamp.Format("2006-01-02"))
	a.redis.ZAdd(ctx, actionKey, &redis.Z{
		Score:  score,
		Member: entry.ID,
	})
	a.redis.Expire(ctx, actionKey, expiration)
}

// QueryAuditLogs queries audit logs based on filters
func (a *AuditLogger) QueryAuditLogs(ctx context.Context, filters AuditFilter) ([]AuditEntry, error) {
	if !a.logToRedis || a.redis == nil {
		return nil, fmt.Errorf("audit log querying requires Redis storage")
	}
	
	var entries []AuditEntry
	
	// Determine the Redis key based on filters
	var key string
	if filters.UserID != "" {
		key = fmt.Sprintf("audit:user:%s:%s", filters.UserID, filters.Date.Format("2006-01-02"))
	} else if filters.Action != "" {
		key = fmt.Sprintf("audit:action:%s:%s", filters.Action, filters.Date.Format("2006-01-02"))
	} else {
		key = fmt.Sprintf("audit:log:%s", filters.Date.Format("2006-01-02"))
	}
	
	// Query Redis
	start := filters.StartTime.Unix()
	end := filters.EndTime.Unix()
	
	results, err := a.redis.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:    fmt.Sprintf("%d", start),
		Max:    fmt.Sprintf("%d", end),
		Offset: int64(filters.Offset),
		Count:  int64(filters.Limit),
	}).Result()
	
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	
	// Parse results
	for _, result := range results {
		var entry AuditEntry
		if err := json.Unmarshal([]byte(result), &entry); err != nil {
			a.logger.Error("failed to unmarshal audit entry", "error", err)
			continue
		}
		entries = append(entries, entry)
	}
	
	return entries, nil
}

// AuditFilter defines filters for querying audit logs
type AuditFilter struct {
	UserID    string
	Action    string
	Date      time.Time
	StartTime time.Time
	EndTime   time.Time
	Offset    int
	Limit     int
}

// generateAuditID generates a unique audit entry ID
func generateAuditID() string {
	return fmt.Sprintf("audit_%d_%s", time.Now().UnixNano(), generateRandomString(8))
}

// generateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}