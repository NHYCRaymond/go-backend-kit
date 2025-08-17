package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/response"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents JWT claims
type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// AuthMiddleware handles JWT authentication
type AuthMiddleware struct {
	config config.AuthConfig
	jwt    *JWTService
	redis  *redis.Client
}

// JWTService handles JWT operations
type JWTService struct {
	secretKey             []byte
	accessExpirationMins  int
	refreshExpirationMins int
}

// NewJWTService creates a new JWT service
func NewJWTService(cfg config.JWTConfig) *JWTService {
	return &JWTService{
		secretKey:             []byte(cfg.SecretKey),
		accessExpirationMins:  cfg.AccessExpirationMins,
		refreshExpirationMins: cfg.RefreshExpirationMins,
	}
}

// GenerateTokens generates access and refresh tokens
func (j *JWTService) GenerateTokens(userID, role string) (string, string, error) {
	now := time.Now()

	// Access token
	accessClaims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(j.accessExpirationMins) * time.Minute)),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(j.secretKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	// Refresh token
	refreshClaims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(j.refreshExpirationMins) * time.Minute)),
		},
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(j.secretKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return accessTokenString, refreshTokenString, nil
}

// ValidateToken validates a JWT token
func (j *JWTService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(cfg config.AuthConfig, jwtService *JWTService, redis *redis.Client) *AuthMiddleware {
	return &AuthMiddleware{
		config: cfg,
		jwt:    jwtService,
		redis:  redis,
	}
}

// Handler returns the middleware handler function
func (a *AuthMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip authentication for configured paths
		for _, path := range a.config.SkipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		// Get token from header or query parameter
		token := a.extractToken(c)
		if token == "" {
			response.Error(c, http.StatusUnauthorized, "missing_token", "Authentication token is required")
			c.Abort()
			return
		}

		// Check if token is blacklisted
		if a.config.EnableBlacklist && a.isTokenBlacklisted(c, token) {
			response.Error(c, http.StatusUnauthorized, "token_blacklisted", "Token has been revoked")
			c.Abort()
			return
		}

		// Validate token
		claims, err := a.jwt.ValidateToken(token)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "invalid_token", "Invalid authentication token")
			c.Abort()
			return
		}

		// Set user information in context
		c.Set("user_id", claims.UserID)
		c.Set("user_role", claims.Role)
		c.Set("token", token)

		c.Next()
	}
}

// extractToken extracts token from request
func (a *AuthMiddleware) extractToken(c *gin.Context) string {
	// Try header first
	authHeader := c.GetHeader(a.config.TokenHeader)
	if authHeader != "" {
		if a.config.RequireBearer {
			if strings.HasPrefix(authHeader, "Bearer ") {
				return strings.TrimPrefix(authHeader, "Bearer ")
			}
		} else {
			return authHeader
		}
	}

	// Try query parameter
	if a.config.TokenQueryParam != "" {
		return c.Query(a.config.TokenQueryParam)
	}

	return ""
}

// isTokenBlacklisted checks if token is in blacklist
func (a *AuthMiddleware) isTokenBlacklisted(c *gin.Context, token string) bool {
	if a.redis == nil {
		return false
	}

	key := fmt.Sprintf("blacklist:token:%s", token)
	exists, err := a.redis.Exists(context.Background(), key).Result()
	if err != nil {
		return false
	}

	return exists > 0
}

// BlacklistToken adds token to blacklist
func (a *AuthMiddleware) BlacklistToken(ctx context.Context, token string, expiry time.Duration) error {
	if a.redis == nil {
		return fmt.Errorf("redis client not available")
	}

	key := fmt.Sprintf("blacklist:token:%s", token)
	return a.redis.SetEX(ctx, key, "1", expiry).Err()
}

// RequireRole creates a middleware that requires specific role
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("user_role")
		if !exists {
			response.Error(c, http.StatusUnauthorized, "unauthorized", "User role not found")
			c.Abort()
			return
		}

		if userRole != role {
			response.Error(c, http.StatusForbidden, "forbidden", "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAnyRole creates a middleware that requires any of the specified roles
func RequireAnyRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("user_role")
		if !exists {
			response.Error(c, http.StatusUnauthorized, "unauthorized", "User role not found")
			c.Abort()
			return
		}

		for _, role := range roles {
			if userRole == role {
				c.Next()
				return
			}
		}

		response.Error(c, http.StatusForbidden, "forbidden", "Insufficient permissions")
		c.Abort()
	}
}

// GetUserID gets user ID from context
func GetUserID(c *gin.Context) (string, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return "", false
	}

	if id, ok := userID.(string); ok {
		return id, true
	}

	return "", false
}

// GetUserRole gets user role from context
func GetUserRole(c *gin.Context) (string, bool) {
	userRole, exists := c.Get("user_role")
	if !exists {
		return "", false
	}

	if role, ok := userRole.(string); ok {
		return role, true
	}

	return "", false
}
