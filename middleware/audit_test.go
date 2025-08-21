package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestAuditLogger(t *testing.T) (*AuditLogger, *redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config := AuditConfig{
		Enabled:       true,
		LogToRedis:    true,
		LogToFile:     false,
		RetentionDays: 30,
	}

	auditLogger := NewAuditLogger(logger, client, config)

	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})

	return auditLogger, client, mr
}

func TestNewAuditLogger(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client, _ := setupTestRedis(t)

	tests := []struct {
		name   string
		config AuditConfig
	}{
		{
			name: "enabled with redis",
			config: AuditConfig{
				Enabled:       true,
				LogToRedis:    true,
				LogToFile:     false,
				RetentionDays: 30,
			},
		},
		{
			name: "disabled",
			config: AuditConfig{
				Enabled: false,
			},
		},
		{
			name: "with custom retention",
			config: AuditConfig{
				Enabled:       true,
				LogToRedis:    true,
				RetentionDays: 60,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditLogger := NewAuditLogger(logger, client, tt.config)
			assert.NotNil(t, auditLogger)
			assert.Equal(t, tt.config.Enabled, auditLogger.enabled)
			assert.Equal(t, tt.config.LogToRedis, auditLogger.logToRedis)
			if tt.config.RetentionDays > 0 {
				assert.Equal(t, tt.config.RetentionDays, auditLogger.retentionDays)
			} else {
				assert.Equal(t, 90, auditLogger.retentionDays) // Default
			}
		})
	}
}

func TestLogAuth(t *testing.T) {
	auditLogger, client, _ := setupTestAuditLogger(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		action   AuditAction
		userID   string
		userType string
		success  bool
		details  map[string]interface{}
	}{
		{
			name:     "successful login",
			action:   AuditActionLogin,
			userID:   "user123",
			userType: "REGULAR",
			success:  true,
			details: map[string]interface{}{
				"device_id": "device-001",
				"ip":        "192.168.1.1",
			},
		},
		{
			name:     "failed login",
			action:   AuditActionLogin,
			userID:   "user456",
			userType: "REGULAR",
			success:  false,
			details: map[string]interface{}{
				"reason": "invalid_password",
			},
		},
		{
			name:     "token refresh",
			action:   AuditActionTokenRefresh,
			userID:   "user789",
			userType: "ADMIN",
			success:  true,
			details:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditLogger.LogAuth(ctx, tt.action, tt.userID, tt.userType, tt.success, tt.details)

			// Give Redis time to process
			time.Sleep(10 * time.Millisecond)

			// Check if entry was stored in Redis
			key := fmt.Sprintf("audit:log:%s", time.Now().Format("2006-01-02"))
			entries, err := client.ZRange(ctx, key, 0, -1).Result()
			assert.NoError(t, err)
			assert.NotEmpty(t, entries)

			// Parse and verify the last entry
			if len(entries) > 0 {
				var entry AuditEntry
				err := json.Unmarshal([]byte(entries[len(entries)-1]), &entry)
				assert.NoError(t, err)
				assert.Equal(t, tt.action, entry.Action)
				assert.Equal(t, tt.userID, entry.UserID)
				assert.Equal(t, tt.userType, entry.UserType)
				assert.Equal(t, tt.success, entry.Success)
			}
		})
	}
}

func TestLogDataAccess(t *testing.T) {
	auditLogger, client, _ := setupTestAuditLogger(t)
	ctx := context.Background()

	userID := "user123"
	resource := "users"
	action := "read"
	details := map[string]interface{}{
		"record_id": "456",
		"fields":    []string{"name", "email"},
	}

	auditLogger.LogDataAccess(ctx, userID, resource, action, details)

	// Give Redis time to process
	time.Sleep(10 * time.Millisecond)

	// Check if entry was stored
	key := fmt.Sprintf("audit:log:%s", time.Now().Format("2006-01-02"))
	entries, err := client.ZRange(ctx, key, 0, -1).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, entries)

	// Parse and verify
	var entry AuditEntry
	err = json.Unmarshal([]byte(entries[0]), &entry)
	assert.NoError(t, err)
	assert.Equal(t, AuditActionDataAccess, entry.Action)
	assert.Equal(t, userID, entry.UserID)
	assert.Equal(t, resource, entry.Details["resource"])
	assert.Equal(t, action, entry.Details["action"])
}

func TestAuditMiddleware(t *testing.T) {
	auditLogger, client, _ := setupTestAuditLogger(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(auditLogger.AuditMiddleware())

	// Add test routes
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	router.POST("/auth/login", func(c *gin.Context) {
		// Set user context as if authentication succeeded
		c.Set("user_claims", &EnhancedClaims{
			UserID:   "user123",
			UserType: "REGULAR",
		})
		c.JSON(200, gin.H{"token": "test-token"})
	})

	router.GET("/protected", func(c *gin.Context) {
		c.Set("user_claims", &EnhancedClaims{
			UserID:    "user456",
			UserType:  "ADMIN",
			SessionID: "session-001",
		})
		c.JSON(200, gin.H{"data": "sensitive"})
	})

	tests := []struct {
		name       string
		method     string
		path       string
		wantAction AuditAction
		wantStatus int
	}{
		{
			name:       "GET request",
			method:     "GET",
			path:       "/test",
			wantAction: AuditActionDataAccess,
			wantStatus: 200,
		},
		{
			name:       "login endpoint",
			method:     "POST",
			path:       "/auth/login",
			wantAction: AuditActionLogin,
			wantStatus: 200,
		},
		{
			name:       "protected endpoint",
			method:     "GET",
			path:       "/protected",
			wantAction: AuditActionDataAccess,
			wantStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear Redis
			client.FlushAll(context.Background())

			// Make request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("User-Agent", "test-agent")
			req.Header.Set("X-Real-IP", "192.168.1.100")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			// Give Redis time to process
			time.Sleep(10 * time.Millisecond)

			// Check audit log
			key := fmt.Sprintf("audit:log:%s", time.Now().Format("2006-01-02"))
			entries, err := client.ZRange(context.Background(), key, 0, -1).Result()
			assert.NoError(t, err)

			if len(entries) > 0 {
				var entry AuditEntry
				err = json.Unmarshal([]byte(entries[0]), &entry)
				assert.NoError(t, err)
				assert.Equal(t, tt.wantAction, entry.Action)
				assert.Equal(t, tt.method, entry.Method)
				assert.Equal(t, tt.path, entry.Path)
				assert.Equal(t, tt.wantStatus, entry.StatusCode)
				assert.True(t, entry.Success)
			}
		})
	}
}

func TestSkipPaths(t *testing.T) {
	auditLogger, client, _ := setupTestAuditLogger(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(auditLogger.AuditMiddleware())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	router.GET("/api/users", func(c *gin.Context) {
		c.JSON(200, gin.H{"users": []string{"user1", "user2"}})
	})

	// Request to skipped path
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Request to normal path
	req2 := httptest.NewRequest("GET", "/api/users", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	time.Sleep(10 * time.Millisecond)

	// Check audit logs
	key := fmt.Sprintf("audit:log:%s", time.Now().Format("2006-01-02"))
	entries, err := client.ZRange(context.Background(), key, 0, -1).Result()
	assert.NoError(t, err)

	// Should only have one entry (the /api/users request)
	assert.Len(t, entries, 1)

	var entry AuditEntry
	err = json.Unmarshal([]byte(entries[0]), &entry)
	assert.NoError(t, err)
	assert.Equal(t, "/api/users", entry.Path)
}

func TestQueryAuditLogs(t *testing.T) {
	auditLogger, _, _ := setupTestAuditLogger(t)
	ctx := context.Background()

	// Store some test audit entries
	now := time.Now()
	for i := 0; i < 5; i++ {
		auditLogger.LogAuth(ctx, AuditActionLogin, fmt.Sprintf("user%d", i), "REGULAR", true, nil)
		time.Sleep(5 * time.Millisecond)
	}

	// Query logs
	filter := AuditFilter{
		Date:      now,
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
		Limit:     10,
	}

	entries, err := auditLogger.QueryAuditLogs(ctx, filter)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(entries), 5)
}

func TestAuditRetention(t *testing.T) {
	auditLogger, client, mr := setupTestAuditLogger(t)
	ctx := context.Background()

	// Set short retention for testing
	auditLogger.retentionDays = 1

	// Log an entry
	auditLogger.LogAuth(ctx, AuditActionLogin, "user123", "REGULAR", true, nil)

	// Check key exists
	key := fmt.Sprintf("audit:log:%s", time.Now().Format("2006-01-02"))
	exists, err := client.Exists(ctx, key).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), exists)

	// Check TTL is set
	ttl, err := client.TTL(ctx, key).Result()
	assert.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 24*time.Hour)

	// Fast-forward time
	mr.FastForward(25 * time.Hour)

	// Key should be expired
	exists, err = client.Exists(ctx, key).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

func TestAuditIndexing(t *testing.T) {
	auditLogger, client, _ := setupTestAuditLogger(t)
	ctx := context.Background()

	userID := "user123"
	action := AuditActionLogin

	// Log an entry
	auditLogger.LogAuth(ctx, action, userID, "REGULAR", true, map[string]interface{}{
		"test": "data",
	})

	time.Sleep(10 * time.Millisecond)

	// Check user index
	userKey := fmt.Sprintf("audit:user:%s:%s", userID, time.Now().Format("2006-01-02"))
	userEntries, err := client.ZRange(ctx, userKey, 0, -1).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, userEntries)

	// Check action index
	actionKey := fmt.Sprintf("audit:action:%s:%s", action, time.Now().Format("2006-01-02"))
	actionEntries, err := client.ZRange(ctx, actionKey, 0, -1).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, actionEntries)
}

func TestDisabledAuditLogger(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client, _ := setupTestRedis(t)
	
	config := AuditConfig{
		Enabled: false, // Disabled
	}
	
	auditLogger := NewAuditLogger(logger, client, config)
	ctx := context.Background()

	// Try to log (should do nothing)
	auditLogger.LogAuth(ctx, AuditActionLogin, "user123", "REGULAR", true, nil)

	// Check nothing was stored
	key := fmt.Sprintf("audit:log:%s", time.Now().Format("2006-01-02"))
	entries, err := client.ZRange(ctx, key, 0, -1).Result()
	assert.NoError(t, err)
	assert.Empty(t, entries)
}

func BenchmarkLogAuth(b *testing.B) {
	auditLogger, _, _ := setupTestAuditLogger(&testing.T{})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auditLogger.LogAuth(ctx, AuditActionLogin, "user123", "REGULAR", true, map[string]interface{}{
			"device_id": "device-001",
			"ip":        "192.168.1.1",
		})
	}
}

func BenchmarkAuditMiddleware(b *testing.B) {
	auditLogger, _, _ := setupTestAuditLogger(&testing.T{})
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(auditLogger.AuditMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}