package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

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
	db, err := database.NewMySQL(cfg.Database.MySQL)
	if err != nil {
		logger.Error("Failed to connect to MySQL", "error", err)
		os.Exit(1)
	}

	redis, err := database.NewRedis(context.Background(), cfg.Redis)
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
			// Login logic here
			response.Success(c, gin.H{
				"token": "example-token",
			})
		})
	}

	// Protected routes
	protected := router.Group("/api/v1")
	protected.Use(middlewareManager.GetAuthMiddleware())
	{
		protected.GET("/user/profile", func(c *gin.Context) {
			userID, _ := middleware.GetUserID(c)
			response.Success(c, gin.H{
				"user_id": userID,
				"profile": "user profile data",
			})
		})

		protected.PUT("/user/profile", func(c *gin.Context) {
			userID, _ := middleware.GetUserID(c)
			response.Success(c, gin.H{
				"user_id": userID,
				"message": "profile updated",
			})
		})
	}

	// Admin routes
	admin := router.Group("/api/v1/admin")
	admin.Use(middlewareManager.GetAuthMiddleware())
	admin.Use(middleware.RequireRole("admin"))
	{
		admin.GET("/users", func(c *gin.Context) {
			response.Success(c, gin.H{
				"users": []string{"user1", "user2"},
			})
		})

		admin.GET("/stats", func(c *gin.Context) {
			response.Success(c, gin.H{
				"total_users": 100,
				"active_users": 50,
			})
		})
	}

	// Health check routes
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"database": "connected",
			"redis": "connected",
		})
	})
}