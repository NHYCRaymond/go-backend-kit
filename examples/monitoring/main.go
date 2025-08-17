package main

import (
	"context"
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

	// Setup monitoring middleware FIRST
	metricsConfig := &monitoring.MetricsConfig{
		SLAThresholds: map[string]float64{
			"/api/v1/users":        0.5, // 500ms
			"/api/v1/user/profile": 0.3, // 300ms
			"/api/v1/orders":       1.0, // 1s
			"/api/v1/admin/users":  0.8, // 800ms
			"/api/v1/admin/stats":  1.5, // 1.5s
		},
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/ready",
			"/live",
			"/ping",
		},
		PathGrouping: map[string]string{
			`/api/v1/users/\d+`:    "/api/v1/users/:id",
			`/api/v1/orders/\d+`:   "/api/v1/orders/:id",
			`/api/v1/products/\d+`: "/api/v1/products/:id",
		},
	}

	// Apply monitoring middleware
	router.Use(monitoring.GinMetricsMiddleware(metricsConfig))

	// Setup other middleware
	jwtService := middleware.NewJWTService(cfg.JWT)
	middlewareManager := middleware.NewManager(&cfg.Middleware, redis, logger, jwtService)
	middlewareManager.SetupBasic(router)

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
			time.Sleep(100 * time.Millisecond)

			// Record business metric
			monitoring.RecordUserLogin("success")

			response.Success(c, gin.H{
				"token": "example-token",
			})
		})

		public.POST("/auth/register", func(c *gin.Context) {
			// Simulate processing time
			time.Sleep(200 * time.Millisecond)

			// Record business metric
			monitoring.RecordUserRegistration()

			response.Success(c, gin.H{
				"message": "user registered",
			})
		})

		// Route that simulates slow response
		public.GET("/slow", func(c *gin.Context) {
			time.Sleep(2 * time.Second) // This will trigger SLA violation
			response.Success(c, gin.H{
				"message": "slow response",
			})
		})

		// Route that simulates error
		public.GET("/error", func(c *gin.Context) {
			response.Error(c, http.StatusInternalServerError, http.StatusInternalServerError, "simulated error")
		})
	}

	// Protected routes
	protected := router.Group("/api/v1")
	protected.Use(middlewareManager.GetAuthMiddleware())
	{
		protected.GET("/user/profile", func(c *gin.Context) {
			userID, _ := middleware.GetUserID(c)

			// Simulate database query
			time.Sleep(50 * time.Millisecond)

			response.Success(c, gin.H{
				"user_id": userID,
				"profile": "user profile data",
			})
		})

		protected.PUT("/user/profile", func(c *gin.Context) {
			userID, _ := middleware.GetUserID(c)

			// Simulate database update
			time.Sleep(150 * time.Millisecond)

			response.Success(c, gin.H{
				"user_id": userID,
				"message": "profile updated",
			})
		})

		protected.GET("/orders", func(c *gin.Context) {
			// Simulate complex query
			time.Sleep(800 * time.Millisecond)

			response.Success(c, gin.H{
				"orders": []string{"order1", "order2"},
			})
		})
	}

	// Admin routes
	admin := router.Group("/api/v1/admin")
	admin.Use(middlewareManager.GetAuthMiddleware())
	admin.Use(middleware.RequireRole("admin"))
	{
		admin.GET("/users", func(c *gin.Context) {
			// Simulate admin query
			time.Sleep(600 * time.Millisecond)

			response.Success(c, gin.H{
				"users": []string{"user1", "user2"},
			})
		})

		admin.GET("/stats", func(c *gin.Context) {
			// Simulate complex statistics calculation
			time.Sleep(1200 * time.Millisecond)

			// Update business metrics
			monitoring.SetActiveUsers(42)

			response.Success(c, gin.H{
				"total_users":  100,
				"active_users": 42,
			})
		})
	}

	// Health check routes (these are skipped by monitoring)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":   "healthy",
			"database": "connected",
			"redis":    "connected",
		})
	})

	router.GET("/metrics", func(c *gin.Context) {
		// This endpoint is handled by Prometheus
		c.String(http.StatusOK, "Use /metrics endpoint from monitoring server")
	})
}
