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
	"github.com/NHYCRaymond/go-backend-kit/monitoring"
	"github.com/NHYCRaymond/go-backend-kit/response"
	"github.com/NHYCRaymond/go-backend-kit/server"
	"github.com/gin-gonic/gin"
	goredis "github.com/go-redis/redis/v8"
)

func main() {
	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Set up logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Initialize database connections
	db, err := database.NewMySQLLegacy(cfg.Database.MySQL)
	if err != nil {
		logger.Error("Failed to connect to MySQL", "error", err)
		os.Exit(1)
	}

	redis, err := database.NewRedisLegacy(context.Background(), cfg.Redis)
	if err != nil {
		logger.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}

	// Start monitoring server
	monitoring.StartServer(&cfg.Monitoring, logger)

	// Create Gin router
	router := gin.New()

	// Setup middleware stack with proper order
	setupMiddleware(router, &cfg.Middleware, redis, logger)

	// Setup routes
	setupRoutes(router, db, redis, logger)

	// Create and start server
	srv := server.New(cfg.Server, router, logger)
	srv.SetupHealthCheck()

	if err := srv.StartAndWait(); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}

func setupMiddleware(router *gin.Engine, cfg *config.MiddlewareConfig, redis interface{}, logger *slog.Logger) {
	// 1. Recovery middleware (should be first)
	router.Use(gin.Recovery())

	// 2. Request ID middleware (must be early to ensure all subsequent middleware have access)
	requestIDConfig := &middleware.RequestIDConfig{
		Generator:        middleware.DefaultRequestIDConfig().Generator,
		HeaderName:       "X-Request-ID",
		ContextKey:       "request_id",
		EnableTraceID:    true,
		TraceIDGenerator: middleware.DefaultRequestIDConfig().TraceIDGenerator,
		SkipPaths:        []string{"/health", "/metrics", "/ready", "/live"},
	}
	router.Use(middleware.RequestIDMiddleware(requestIDConfig))

	// 3. Monitoring middleware (after RequestID so it can include request ID in metrics)
	metricsConfig := &monitoring.MetricsConfig{
		SLAThresholds: map[string]float64{
			"/api/v1/users":  0.5,
			"/api/v1/orders": 1.0,
			"/api/v1/slow":   0.3, // This will trigger SLA violations
		},
		SkipPaths: []string{"/health", "/metrics", "/ready", "/live"},
		PathGrouping: map[string]string{
			`/api/v1/users/\d+`: "/api/v1/users/:id",
		},
	}
	router.Use(monitoring.GinMetricsMiddleware(metricsConfig))

	// 4. Logger middleware (after RequestID so it can log request IDs)
	loggerMiddleware := middleware.NewLoggerMiddleware(cfg.Logger, logger)
	router.Use(loggerMiddleware.Handler())

	// 5. Other middleware
	// Security middleware
	router.Use(middleware.SecureHeaders())

	// Rate limiting middleware
	redisDB := redis.(*database.RedisDatabase)
	rateLimitMiddleware := middleware.NewRateLimitMiddleware(cfg.RateLimit, redisDB.GetClient().(*goredis.Client))
	router.Use(rateLimitMiddleware.Handler())
}

func setupRoutes(router *gin.Engine, db interface{}, redis interface{}, logger *slog.Logger) {
	// Demo routes to show REQUEST_ID lifecycle
	api := router.Group("/api/v1")
	{
		// Simple route
		api.GET("/ping", func(c *gin.Context) {
			requestID := middleware.GetRequestID(c)
			traceID := middleware.GetTraceIDFromLogger(c)

			// Log with request context
			contextLogger := middleware.LogWithRequestID(c, logger)
			contextLogger.Info("Processing ping request")

			response.Success(c, gin.H{
				"message":    "pong",
				"request_id": requestID,
				"trace_id":   traceID,
			})
		})

		// Route that demonstrates request tracking
		api.GET("/track", func(c *gin.Context) {
			requestID := middleware.GetRequestID(c)

			// Log start of processing
			contextLogger := middleware.LogWithRequestID(c, logger)
			contextLogger.Info("Starting request processing", "operation", "track")

			// Simulate some work
			time.Sleep(100 * time.Millisecond)

			// Simulate database call with request context
			ctx := c.Request.Context()
			simulateDBCall(ctx, contextLogger)

			// Simulate external API call
			simulateExternalCall(ctx, contextLogger)

			contextLogger.Info("Request processing completed", "operation", "track")

			response.Success(c, gin.H{
				"message":    "request tracked successfully",
				"request_id": requestID,
				"operations": []string{"db_query", "external_api"},
			})
		})

		// Route that demonstrates slow request
		api.GET("/slow", func(c *gin.Context) {
			requestID := middleware.GetRequestID(c)

			contextLogger := middleware.LogWithRequestID(c, logger)
			contextLogger.Info("Processing slow request")

			// Simulate slow processing (will trigger SLA violation)
			time.Sleep(1 * time.Second)

			contextLogger.Warn("Slow request completed", "duration", "1s")

			response.Success(c, gin.H{
				"message":    "slow response",
				"request_id": requestID,
				"duration":   "1s",
			})
		})

		// Route that demonstrates error handling with request ID
		api.GET("/error", func(c *gin.Context) {
			requestID := middleware.GetRequestID(c)

			contextLogger := middleware.LogWithRequestID(c, logger)
			contextLogger.Info("Processing error request")

			// Simulate error
			err := fmt.Errorf("simulated error for request %s", requestID)
			contextLogger.Error("Request failed", "error", err)

			response.Error(c, http.StatusInternalServerError, 500, "Internal server error")
		})

		// Route that demonstrates nested service calls
		api.GET("/nested", func(c *gin.Context) {
			requestID := middleware.GetRequestID(c)

			contextLogger := middleware.LogWithRequestID(c, logger)
			contextLogger.Info("Processing nested request")

			// Simulate nested service calls
			ctx := c.Request.Context()
			result1 := simulateService1(ctx, contextLogger)
			result2 := simulateService2(ctx, contextLogger)

			contextLogger.Info("Nested request completed")

			response.Success(c, gin.H{
				"message":    "nested services called",
				"request_id": requestID,
				"results":    []interface{}{result1, result2},
			})
		})

		// Route to demonstrate user context
		api.GET("/user/:id", func(c *gin.Context) {
			userID := c.Param("id")
			requestID := middleware.GetRequestID(c)

			contextLogger := middleware.LogWithRequestID(c, logger)
			contextLogger.Info("Processing user request", "user_id", userID)

			// Simulate user data fetch
			ctx := c.Request.Context()
			userData := simulateUserFetch(ctx, userID, contextLogger)

			contextLogger.Info("User request completed", "user_id", userID)

			response.Success(c, gin.H{
				"message":    "user data fetched",
				"request_id": requestID,
				"user_data":  userData,
			})
		})
	}

	// Health check routes (these skip REQUEST_ID generation)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
		})
	})

	// Debug route to show current request info
	router.GET("/debug/request", func(c *gin.Context) {
		requestInfo := middleware.GetRequestInfo(c)
		structuredFields := middleware.GetStructuredLogFields(c)

		c.JSON(http.StatusOK, gin.H{
			"request_info":      requestInfo,
			"structured_fields": structuredFields,
		})
	})
}

// Simulate database call with request context
func simulateDBCall(ctx context.Context, logger *slog.Logger) {
	requestID := middleware.GetRequestIDFromContext(ctx)

	logger.Info("Executing database query", "request_id", requestID, "query", "SELECT * FROM users")

	// Simulate query time
	time.Sleep(50 * time.Millisecond)

	logger.Info("Database query completed", "request_id", requestID, "rows", 5)
}

// Simulate external API call with request context
func simulateExternalCall(ctx context.Context, logger *slog.Logger) {
	requestID := middleware.GetRequestIDFromContext(ctx)
	traceID := middleware.GetTraceIDFromContext(ctx)

	logger.Info("Calling external API",
		"request_id", requestID,
		"trace_id", traceID,
		"endpoint", "https://api.example.com/data")

	// Simulate API call time
	time.Sleep(200 * time.Millisecond)

	logger.Info("External API call completed",
		"request_id", requestID,
		"trace_id", traceID,
		"status", "200")
}

// Simulate service 1 call
func simulateService1(ctx context.Context, logger *slog.Logger) map[string]interface{} {
	requestID := middleware.GetRequestIDFromContext(ctx)

	logger.Info("Service 1 processing", "request_id", requestID)
	time.Sleep(30 * time.Millisecond)

	result := map[string]interface{}{
		"service":    "service1",
		"data":       "service1 data",
		"request_id": requestID,
	}

	logger.Info("Service 1 completed", "request_id", requestID)
	return result
}

// Simulate service 2 call
func simulateService2(ctx context.Context, logger *slog.Logger) map[string]interface{} {
	requestID := middleware.GetRequestIDFromContext(ctx)

	logger.Info("Service 2 processing", "request_id", requestID)
	time.Sleep(80 * time.Millisecond)

	result := map[string]interface{}{
		"service":    "service2",
		"data":       "service2 data",
		"request_id": requestID,
	}

	logger.Info("Service 2 completed", "request_id", requestID)
	return result
}

// Simulate user data fetch
func simulateUserFetch(ctx context.Context, userID string, logger *slog.Logger) map[string]interface{} {
	requestID := middleware.GetRequestIDFromContext(ctx)

	logger.Info("Fetching user data", "request_id", requestID, "user_id", userID)
	time.Sleep(60 * time.Millisecond)

	result := map[string]interface{}{
		"user_id":    userID,
		"name":       fmt.Sprintf("User %s", userID),
		"email":      fmt.Sprintf("user%s@example.com", userID),
		"request_id": requestID,
	}

	logger.Info("User data fetched", "request_id", requestID, "user_id", userID)
	return result
}
