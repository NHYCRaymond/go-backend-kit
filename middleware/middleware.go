package middleware

import (
	"log/slog"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// Manager manages all middleware
type Manager struct {
	config   *config.MiddlewareConfig
	redis    *redis.Client
	logger   *slog.Logger
	jwt      *JWTService
	
	// Middleware instances
	auth     *AuthMiddleware
	logger   *LoggerMiddleware
	security *SecurityMiddleware
	rateLimit *RateLimitMiddleware
}

// NewManager creates a new middleware manager
func NewManager(cfg *config.MiddlewareConfig, redis *redis.Client, logger *slog.Logger, jwtService *JWTService) *Manager {
	return &Manager{
		config: cfg,
		redis:  redis,
		logger: logger,
		jwt:    jwtService,
	}
}

// Initialize initializes all middleware instances
func (m *Manager) Initialize() {
	// Initialize middleware instances
	m.auth = NewAuthMiddleware(m.config.Auth, m.jwt, m.redis)
	m.logger = NewLoggerMiddleware(m.config.Logger, m.logger)
	m.security = NewSecurityMiddleware(m.config.Security)
	m.rateLimit = NewRateLimitMiddleware(m.config.RateLimit, m.redis)
}

// SetupAll sets up all middleware in the correct order
func (m *Manager) SetupAll(router *gin.Engine) {
	// Initialize middleware if not already done
	if m.auth == nil {
		m.Initialize()
	}

	// Recovery middleware (should be first)
	router.Use(gin.Recovery())

	// Request ID middleware (should be early)
	router.Use(RequestID())

	// Security middleware
	if m.config.Security.EnableCORS {
		router.Use(m.security.CORSMiddleware())
	}
	router.Use(m.security.Handler())

	// Rate limiting middleware
	if m.config.RateLimit.Enable {
		router.Use(m.rateLimit.Handler())
	}

	// Logging middleware
	router.Use(m.logger.Handler())

	// Authentication middleware (applied to specific routes)
	// This should be applied per route group, not globally
}

// SetupBasic sets up basic middleware without authentication
func (m *Manager) SetupBasic(router *gin.Engine) {
	// Initialize middleware if not already done
	if m.security == nil {
		m.Initialize()
	}

	// Recovery middleware
	router.Use(gin.Recovery())

	// Request ID middleware
	router.Use(RequestID())

	// Security middleware
	if m.config.Security.EnableCORS {
		router.Use(m.security.CORSMiddleware())
	}
	router.Use(m.security.Handler())

	// Rate limiting middleware
	if m.config.RateLimit.Enable {
		router.Use(m.rateLimit.Handler())
	}

	// Logging middleware
	router.Use(m.logger.Handler())
}

// GetAuthMiddleware returns the auth middleware handler
func (m *Manager) GetAuthMiddleware() gin.HandlerFunc {
	if m.auth == nil {
		m.Initialize()
	}
	return m.auth.Handler()
}

// GetLoggerMiddleware returns the logger middleware handler
func (m *Manager) GetLoggerMiddleware() gin.HandlerFunc {
	if m.logger == nil {
		m.Initialize()
	}
	return m.logger.Handler()
}

// GetSecurityMiddleware returns the security middleware handler
func (m *Manager) GetSecurityMiddleware() gin.HandlerFunc {
	if m.security == nil {
		m.Initialize()
	}
	return m.security.Handler()
}

// GetRateLimitMiddleware returns the rate limit middleware handler
func (m *Manager) GetRateLimitMiddleware() gin.HandlerFunc {
	if m.rateLimit == nil {
		m.Initialize()
	}
	return m.rateLimit.Handler()
}

// GetCORSMiddleware returns the CORS middleware handler
func (m *Manager) GetCORSMiddleware() gin.HandlerFunc {
	if m.security == nil {
		m.Initialize()
	}
	return m.security.CORSMiddleware()
}

// Setup is a convenience function to set up all middleware
func Setup(router *gin.Engine, cfg *config.MiddlewareConfig, redis *redis.Client, logger *slog.Logger, jwtService *JWTService) *Manager {
	manager := NewManager(cfg, redis, logger, jwtService)
	manager.SetupAll(router)
	return manager
}

// SetupWithAuth sets up middleware with authentication on specific routes
func SetupWithAuth(router *gin.Engine, cfg *config.MiddlewareConfig, redis *redis.Client, logger *slog.Logger, jwtService *JWTService) *Manager {
	manager := NewManager(cfg, redis, logger, jwtService)
	manager.SetupBasic(router)
	return manager
}