package main

import (
	"context"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/database"
	"github.com/NHYCRaymond/go-backend-kit/middleware"
	"github.com/NHYCRaymond/go-backend-kit/monitoring"
	"github.com/NHYCRaymond/go-backend-kit/response"
	"github.com/NHYCRaymond/go-backend-kit/server"
	"github.com/gin-gonic/gin"
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

	// Setup middleware
	jwtService := middleware.NewJWTService(cfg.JWT)
	middlewareManager := middleware.NewManager(&cfg.Middleware, redis, logger, jwtService)
	middlewareManager.SetupBasic(router)

	// Add Prometheus metrics middleware
	router.Use(monitoring.GinMetricsMiddleware(nil))

	// Setup routes
	setupRoutes(router, db, redis, middlewareManager, logger)

	// Create and start server
	srv := server.New(cfg.Server, router, logger)
	srv.SetupHealthCheck()

	if err := srv.StartAndWait(); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}

func setupRoutes(router *gin.Engine, db interface{}, redis interface{}, middlewareManager *middleware.Manager, logger *slog.Logger) {
	// Public routes
	public := router.Group("/api/v1")
	{
		public.GET("/ping", func(c *gin.Context) {
			response.Success(c, gin.H{
				"message": "pong",
			})
		})

		public.POST("/auth/login", func(c *gin.Context) {
			// Simulate some processing time
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))

			// Record login attempt
			success := rand.Float32() > 0.1 // 90% success rate
			if success {
				monitoring.UserLogins.WithLabelValues("success").Inc()
				response.Success(c, gin.H{
					"token":   "example-token",
					"user_id": "user123",
				})
			} else {
				monitoring.UserLogins.WithLabelValues("failed").Inc()
				monitoring.AuthenticationAttempts.WithLabelValues("failed", "password").Inc()
				response.Unauthorized(c, "Invalid credentials")
			}
		})

		public.POST("/auth/register", func(c *gin.Context) {
			// Simulate user registration
			monitoring.UserRegistrations.Inc()
			monitoring.ActiveUsers.Inc()

			response.Success(c, gin.H{
				"user_id": "new-user-" + time.Now().Format("20060102150405"),
				"message": "Registration successful",
			})
		})
	}

	// Protected routes
	protected := router.Group("/api/v1")
	protected.Use(middlewareManager.GetAuthMiddleware())
	{
		protected.GET("/user/profile", func(c *gin.Context) {
			startTime := time.Now()
			userID, _ := middleware.GetUserID(c)

			// Simulate database query
			monitoring.DBQueryDuration.WithLabelValues("select").Observe(time.Since(startTime).Seconds())
			monitoring.DBQueryTotal.WithLabelValues("select", "success").Inc()

			// Simulate cache hit/miss
			if rand.Float32() > 0.3 { // 70% cache hit rate
				monitoring.CacheHits.WithLabelValues("user_profile", "profile:"+userID).Inc()
			} else {
				monitoring.CacheMisses.WithLabelValues("user_profile", "profile:"+userID).Inc()
			}

			response.Success(c, gin.H{
				"user_id": userID,
				"profile": "user profile data",
			})
		})

		protected.PUT("/user/profile", func(c *gin.Context) {
			startTime := time.Now()
			userID, _ := middleware.GetUserID(c)

			// Simulate database update
			monitoring.DBQueryDuration.WithLabelValues("update").Observe(time.Since(startTime).Seconds())
			monitoring.DBQueryTotal.WithLabelValues("update", "success").Inc()

			response.Success(c, gin.H{
				"user_id": userID,
				"message": "profile updated",
			})
		})

		protected.GET("/data/analytics", func(c *gin.Context) {
			// Simulate a slow endpoint
			time.Sleep(time.Millisecond * time.Duration(500+rand.Intn(1500)))

			// Record SLA violation if response time > 1s
			if rand.Float32() > 0.7 {
				monitoring.SLAViolations.WithLabelValues("/data/analytics", "1s").Inc()
			}

			response.Success(c, gin.H{
				"data": "analytics data",
			})
		})
	}

	// Admin routes
	admin := router.Group("/api/v1/admin")
	admin.Use(middlewareManager.GetAuthMiddleware())
	admin.Use(middleware.RequireRole("admin"))
	{
		admin.GET("/users", func(c *gin.Context) {
			// Simulate external service call
			startTime := time.Now()

			// Simulate calling user service
			if rand.Float32() > 0.95 { // 5% failure rate
				monitoring.ExternalServiceCalls.WithLabelValues("user-service", "/users", "error").Inc()
				monitoring.ExternalServiceDuration.WithLabelValues("user-service", "/users").Observe(time.Since(startTime).Seconds())
				response.ServiceUnavailable(c, "User service unavailable")
				return
			}

			monitoring.ExternalServiceCalls.WithLabelValues("user-service", "/users", "success").Inc()
			monitoring.ExternalServiceDuration.WithLabelValues("user-service", "/users").Observe(time.Since(startTime).Seconds())
			response.Success(c, gin.H{
				"users": []string{"user1", "user2", "user3"},
			})
		})

		admin.GET("/stats", func(c *gin.Context) {
			// Simulate complex stats calculation
			startTime := time.Now()

			// Simulate multiple DB queries
			monitoring.DBQueryDuration.WithLabelValues("select").Observe(0.05)
			monitoring.DBQueryTotal.WithLabelValues("select", "success").Inc()
			monitoring.DBQueryDuration.WithLabelValues("select").Observe(0.03)
			monitoring.DBQueryTotal.WithLabelValues("select", "success").Inc()
			monitoring.DBQueryDuration.WithLabelValues("aggregate").Observe(0.1)
			monitoring.DBQueryTotal.WithLabelValues("aggregate", "success").Inc()

			response.Success(c, gin.H{
				"total_users":     100,
				"active_users":    50,
				"processing_time": time.Since(startTime).Seconds(),
			})
		})
	}

	// Demo routes for testing monitoring
	demo := router.Group("/api/v1/demo")
	{
		// Endpoint to simulate errors
		demo.GET("/error", func(c *gin.Context) {
			errorType := c.Query("type")
			monitoring.ErrorsTotal.WithLabelValues(errorType, "demo", "high").Inc()
			response.InternalError(c, "Simulated error: "+errorType)
		})

		// Endpoint to simulate message queue operations
		demo.POST("/message", func(c *gin.Context) {
			queue := c.DefaultQuery("queue", "default")

			// Simulate message publishing
			if rand.Float32() > 0.05 { // 95% success rate
				monitoring.MessageQueuePublishedTotal.WithLabelValues(queue, "success").Inc()
				monitoring.MessageQueueProcessingDuration.WithLabelValues(queue).Observe(rand.Float64() * 0.5)
				response.Success(c, gin.H{
					"message_id": time.Now().UnixNano(),
					"queue":      queue,
				})
			} else {
				monitoring.MessageQueuePublishedTotal.WithLabelValues(queue, "error").Inc()
				response.ServiceUnavailable(c, "Failed to publish message")
			}
		})

		// Endpoint to simulate Redis operations
		demo.GET("/cache/:key", func(c *gin.Context) {
			key := c.Param("key")
			startTime := time.Now()

			// Simulate Redis GET
			monitoring.RedisCommandsDuration.WithLabelValues("GET").Observe(time.Since(startTime).Seconds())
			monitoring.RedisCommandsTotal.WithLabelValues("GET", "success").Inc()

			if rand.Float32() > 0.2 { // 80% cache hit
				monitoring.CacheHits.WithLabelValues("demo", key).Inc()
				response.Success(c, gin.H{
					"key":   key,
					"value": "cached_value_" + key,
					"hit":   true,
				})
			} else {
				monitoring.CacheMisses.WithLabelValues("demo", key).Inc()
				response.Success(c, gin.H{
					"key":   key,
					"value": nil,
					"hit":   false,
				})
			}
		})
	}

}
