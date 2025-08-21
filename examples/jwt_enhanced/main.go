package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/database"
	"github.com/NHYCRaymond/go-backend-kit/middleware"
	"github.com/NHYCRaymond/go-backend-kit/response"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize Redis for token storage
	redisClient, err := database.NewRedis(cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Configure enhanced JWT service
	jwtConfig := middleware.JWTServiceConfig{
		SecretKey:             cfg.JWT.SecretKey,
		AccessExpirationMins:  15,  // 15 minutes for access token
		RefreshExpirationMins: 10080, // 7 days for refresh token
		Issuer:                "go-backend-kit-demo",
		EnableJTI:             true,
		EnableDeviceTracking:  true,
		EnableSessionTracking: true,
		RefreshTokenRotation:  true,
		MaxDevices:            3,
	}

	// Create enhanced JWT service
	jwtService := middleware.NewEnhancedJWTService(jwtConfig)

	// Create refresh token manager
	refreshManager := middleware.NewRefreshTokenManager(
		redisClient,
		jwtConfig.RefreshExpirationMins,
		jwtConfig.RefreshTokenRotation,
	)

	// Create audit logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	auditConfig := middleware.AuditConfig{
		Enabled:       true,
		LogToRedis:    true,
		LogToFile:     false,
		RetentionDays: 30,
	}
	auditLogger := middleware.NewAuditLogger(logger, redisClient, auditConfig)

	// Setup Gin router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestIDMiddleware(nil))
	
	// Add audit middleware globally
	router.Use(auditLogger.AuditMiddleware())

	// Public endpoints (no auth required)
	public := router.Group("/api")
	{
		public.POST("/auth/login", handleLogin(jwtService, refreshManager, auditLogger))
		public.POST("/auth/refresh", handleRefresh(jwtService, refreshManager, auditLogger))
	}

	// Protected endpoints (auth required)
	protected := router.Group("/api")
	protected.Use(createAuthMiddleware(jwtService, redisClient))
	{
		protected.POST("/auth/logout", handleLogout(refreshManager, auditLogger))
		protected.GET("/auth/profile", handleProfile())
		protected.POST("/auth/revoke-all", handleRevokeAll(refreshManager, auditLogger))
		protected.GET("/auth/sessions", handleGetSessions(refreshManager))
	}

	// Admin endpoints (admin role required)
	admin := router.Group("/api/admin")
	admin.Use(createAuthMiddleware(jwtService, redisClient))
	admin.Use(requireRole("ADMIN"))
	{
		admin.GET("/audit/logs", handleGetAuditLogs(auditLogger))
		admin.POST("/users/:userId/revoke", handleRevokeUserTokens(refreshManager, auditLogger))
	}

	// Start server
	fmt.Println("Server starting on :8080...")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// Authentication handlers

func handleLogin(jwtService *middleware.EnhancedJWTService, refreshManager *middleware.RefreshTokenManager, auditLogger *middleware.AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
			DeviceID string `json:"device_id"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
			return
		}

		// TODO: Validate credentials against database
		// For demo, we'll use hardcoded credentials
		var userID, userType string
		if req.Username == "admin" && req.Password == "admin123" {
			userID = "user_admin_001"
			userType = "ADMIN"
		} else if req.Username == "user" && req.Password == "user123" {
			userID = "user_regular_001"
			userType = "REGULAR"
		} else {
			auditLogger.LogAuth(c.Request.Context(), middleware.AuditActionLogin, req.Username, "", false, map[string]interface{}{
				"reason": "invalid_credentials",
			})
			response.Error(c, http.StatusUnauthorized, "invalid_credentials", "Invalid username or password")
			return
		}

		// Generate token pair with options
		options := []middleware.TokenOption{
			middleware.WithIP(c.ClientIP()),
			middleware.WithUserAgent(c.GetHeader("User-Agent")),
		}
		
		if req.DeviceID != "" {
			options = append(options, middleware.WithDeviceID(req.DeviceID))
		}

		accessToken, refreshToken, err := jwtService.GenerateTokenPair(userID, userType, options...)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "token_generation_failed", "Failed to generate tokens")
			return
		}

		// Parse the refresh token to get claims
		refreshClaims, err := jwtService.ValidateRefreshToken(refreshToken)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "token_validation_failed", "Failed to validate refresh token")
			return
		}

		// Store refresh token in Redis
		if err := refreshManager.StoreRefreshToken(c.Request.Context(), userID, refreshToken, refreshClaims); err != nil {
			response.Error(c, http.StatusInternalServerError, "token_storage_failed", "Failed to store refresh token")
			return
		}

		// Log successful login
		auditLogger.LogAuth(c.Request.Context(), middleware.AuditActionLogin, userID, userType, true, map[string]interface{}{
			"device_id":  req.DeviceID,
			"session_id": refreshClaims.SessionID,
		})

		response.Success(c, gin.H{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"token_type":    "Bearer",
			"expires_in":    900, // 15 minutes in seconds
			"user": gin.H{
				"id":   userID,
				"type": userType,
			},
		})
	}
}

func handleRefresh(jwtService *middleware.EnhancedJWTService, refreshManager *middleware.RefreshTokenManager, auditLogger *middleware.AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			RefreshToken string `json:"refresh_token" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
			return
		}

		// Validate refresh token
		claims, err := jwtService.ValidateRefreshToken(req.RefreshToken)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "invalid_refresh_token", "Invalid refresh token")
			return
		}

		// Validate token in Redis
		valid, err := refreshManager.ValidateRefreshToken(c.Request.Context(), claims.UserID, req.RefreshToken)
		if err != nil || !valid {
			auditLogger.LogAuth(c.Request.Context(), middleware.AuditActionTokenRefresh, claims.UserID, claims.UserType, false, map[string]interface{}{
				"reason": "token_not_in_redis",
			})
			response.Error(c, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token has been revoked")
			return
		}

		// Generate new token pair
		options := []middleware.TokenOption{
			middleware.WithIP(c.ClientIP()),
			middleware.WithUserAgent(c.GetHeader("User-Agent")),
			middleware.WithSessionID(claims.SessionID), // Preserve session ID
		}
		
		if claims.DeviceID != "" {
			options = append(options, middleware.WithDeviceID(claims.DeviceID))
		}

		newAccessToken, newRefreshToken, err := jwtService.GenerateTokenPair(claims.UserID, claims.UserType, options...)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "token_generation_failed", "Failed to generate new tokens")
			return
		}

		// Parse new refresh token
		newRefreshClaims, _ := jwtService.ValidateRefreshToken(newRefreshToken)

		// Rotate refresh token if enabled
		if err := refreshManager.RotateRefreshToken(c.Request.Context(), claims.UserID, req.RefreshToken, newRefreshToken, newRefreshClaims); err != nil {
			response.Error(c, http.StatusInternalServerError, "token_rotation_failed", "Failed to rotate refresh token")
			return
		}

		// Log successful refresh
		auditLogger.LogAuth(c.Request.Context(), middleware.AuditActionTokenRefresh, claims.UserID, claims.UserType, true, map[string]interface{}{
			"session_id": claims.SessionID,
		})

		responseData := gin.H{
			"access_token": newAccessToken,
			"token_type":   "Bearer",
			"expires_in":   900,
		}

		// Include new refresh token if rotation is enabled
		if refreshManager.enableRotation {
			responseData["refresh_token"] = newRefreshToken
		}

		response.Success(c, responseData)
	}
}

func handleLogout(refreshManager *middleware.RefreshTokenManager, auditLogger *middleware.AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := c.MustGet("user_claims").(*middleware.EnhancedClaims)

		// Revoke refresh token
		if err := refreshManager.RevokeRefreshToken(c.Request.Context(), claims.UserID); err != nil {
			response.Error(c, http.StatusInternalServerError, "logout_failed", "Failed to revoke tokens")
			return
		}

		// Log logout
		auditLogger.LogAuth(c.Request.Context(), middleware.AuditActionLogout, claims.UserID, claims.UserType, true, nil)

		response.Success(c, gin.H{
			"message": "Logged out successfully",
		})
	}
}

func handleProfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := c.MustGet("user_claims").(*middleware.EnhancedClaims)

		response.Success(c, gin.H{
			"user": gin.H{
				"id":         claims.UserID,
				"type":       claims.UserType,
				"session_id": claims.SessionID,
				"device_id":  claims.DeviceID,
				"ip":         claims.IP,
			},
		})
	}
}

func handleRevokeAll(refreshManager *middleware.RefreshTokenManager, auditLogger *middleware.AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := c.MustGet("user_claims").(*middleware.EnhancedClaims)

		// Revoke all user tokens
		if err := refreshManager.RevokeAllUserTokens(c.Request.Context(), claims.UserID); err != nil {
			response.Error(c, http.StatusInternalServerError, "revoke_failed", "Failed to revoke all tokens")
			return
		}

		// Log revocation
		auditLogger.LogAuth(c.Request.Context(), middleware.AuditActionTokenRevoke, claims.UserID, claims.UserType, true, map[string]interface{}{
			"action": "revoke_all",
		})

		response.Success(c, gin.H{
			"message": "All tokens revoked successfully",
		})
	}
}

func handleGetSessions(refreshManager *middleware.RefreshTokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := c.MustGet("user_claims").(*middleware.EnhancedClaims)

		// Get active tokens
		tokens, err := refreshManager.GetActiveTokens(c.Request.Context(), claims.UserID)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "fetch_failed", "Failed to fetch active sessions")
			return
		}

		response.Success(c, gin.H{
			"sessions": tokens,
		})
	}
}

// Admin handlers

func handleGetAuditLogs(auditLogger *middleware.AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse query parameters
		filter := middleware.AuditFilter{
			Date:      time.Now(),
			StartTime: time.Now().Add(-24 * time.Hour),
			EndTime:   time.Now(),
			Limit:     100,
		}

		if userID := c.Query("user_id"); userID != "" {
			filter.UserID = userID
		}

		if action := c.Query("action"); action != "" {
			filter.Action = action
		}

		// Query audit logs
		logs, err := auditLogger.QueryAuditLogs(c.Request.Context(), filter)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "query_failed", "Failed to query audit logs")
			return
		}

		response.Success(c, gin.H{
			"logs":  logs,
			"count": len(logs),
		})
	}
}

func handleRevokeUserTokens(refreshManager *middleware.RefreshTokenManager, auditLogger *middleware.AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetUserID := c.Param("userId")
		adminClaims := c.MustGet("user_claims").(*middleware.EnhancedClaims)

		// Revoke all tokens for target user
		if err := refreshManager.RevokeAllUserTokens(c.Request.Context(), targetUserID); err != nil {
			response.Error(c, http.StatusInternalServerError, "revoke_failed", "Failed to revoke user tokens")
			return
		}

		// Log admin action
		auditLogger.LogAuth(c.Request.Context(), middleware.AuditActionAdminAction, adminClaims.UserID, adminClaims.UserType, true, map[string]interface{}{
			"action":      "revoke_user_tokens",
			"target_user": targetUserID,
		})

		response.Success(c, gin.H{
			"message": fmt.Sprintf("All tokens for user %s have been revoked", targetUserID),
		})
	}
}

// Middleware helpers

func createAuthMiddleware(jwtService *middleware.EnhancedJWTService, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Error(c, http.StatusUnauthorized, "missing_token", "Authorization header is required")
			c.Abort()
			return
		}

		// Check Bearer prefix
		const bearerPrefix = "Bearer "
		if len(authHeader) < len(bearerPrefix) || authHeader[:len(bearerPrefix)] != bearerPrefix {
			response.Error(c, http.StatusUnauthorized, "invalid_format", "Invalid authorization format")
			c.Abort()
			return
		}

		tokenString := authHeader[len(bearerPrefix):]

		// Validate access token
		claims, err := jwtService.ValidateAccessToken(tokenString)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "invalid_token", "Invalid or expired token")
			c.Abort()
			return
		}

		// Check if token is blacklisted (optional)
		blacklistKey := fmt.Sprintf("blacklist:token:%s", claims.JTI)
		if exists, _ := redisClient.Exists(context.Background(), blacklistKey).Result(); exists > 0 {
			response.Error(c, http.StatusUnauthorized, "token_revoked", "Token has been revoked")
			c.Abort()
			return
		}

		// Set claims in context
		c.Set("user_claims", claims)
		c.Set("user_id", claims.UserID)
		c.Set("user_type", claims.UserType)

		c.Next()
	}
}

func requireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("user_claims")
		if !exists {
			response.Error(c, http.StatusUnauthorized, "unauthorized", "User not authenticated")
			c.Abort()
			return
		}

		enhancedClaims := claims.(*middleware.EnhancedClaims)
		if enhancedClaims.UserType != role {
			response.Error(c, http.StatusForbidden, "forbidden", "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}