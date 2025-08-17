package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize logger
	if err := initLogger(); err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}

	// Create Gin router
	router := gin.New()

	// Add structured logging middleware
	router.Use(logging.StructuredLoggerMiddleware(&logging.StructuredLoggerConfig{
		EnableRequestBody:  true,
		EnableResponseBody: false,
		MaxBodySize:        1024,
		SkipPaths:          []string{"/health"},
		SensitiveFields:    []string{"password", "token"},
	}))

	// Setup routes
	setupRoutes(router)

	// Start server
	logging.Info(context.Background(), "Starting server", "port", 8080)
	if err := router.Run(":8080"); err != nil {
		logging.Fatal(context.Background(), "Failed to start server", "error", err)
	}
}

func initLogger() error {
	cfg := config.LoggerConfig{
		Level:          "debug",
		Format:         "json",
		Output:         "both",
		FilePath:       "logs/app.log",
		MaxSize:        100,
		MaxAge:         7,
		MaxBackups:     3,
		Compress:       true,
		EnableRotation: true,
		AddSource:      true,
	}

	return logging.InitLogger(cfg)
}

func setupRoutes(router *gin.Engine) {
	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Demo routes
	router.GET("/demo/basic", demoBasicLogging)
	router.POST("/demo/business", demoBusinessLogging)
	router.GET("/demo/operation", demoOperationTracking)
	router.GET("/demo/error", demoErrorLogging)
}

func demoBasicLogging(c *gin.Context) {
	ctx := c.Request.Context()

	// Basic logging with context
	logging.Debug(ctx, "Debug message", "detail", "some debug info")
	logging.Info(ctx, "Processing request", "endpoint", "/demo/basic")
	logging.Warn(ctx, "Warning message", "reason", "demonstration")

	// Using logger from context
	logger := logging.L(ctx)
	logger.Info("Using logger from context",
		"user_agent", c.Request.UserAgent(),
		"ip", c.ClientIP(),
	)

	c.JSON(200, gin.H{
		"message": "Basic logging demo completed",
		"tip":     "Check logs for output",
	})
}

func demoBusinessLogging(c *gin.Context) {
	ctx := c.Request.Context()

	// Simulate user login
	userID := "user-123"
	email := "demo@example.com"

	// Log business operation
	logging.Biz(ctx).Operation("user.login",
		"user_id", userID,
		"email", email,
		"ip", c.ClientIP(),
	)

	// Simulate some business logic
	time.Sleep(50 * time.Millisecond)

	// Log success
	logging.Biz(ctx).Success("user.authenticated",
		"user_id", userID,
		"session_id", "sess-456",
	)

	// Log business metric
	logging.Biz(ctx).Metric("active_users", 150, "type", "concurrent")

	// Log business event
	logging.Biz(ctx).Event("feature.used",
		"feature", "logging_demo",
		"user_id", userID,
	)

	c.JSON(200, gin.H{
		"message": "Business logging demo completed",
		"user_id": userID,
	})
}

func demoOperationTracking(c *gin.Context) {
	ctx := c.Request.Context()

	// Start operation tracking
	op := logging.StartOperation(ctx, "demo.complex_operation")
	op.WithField("request_id", c.GetHeader("X-Request-ID"))
	op.WithFields(map[string]interface{}{
		"user_id": "user-789",
		"action":  "data_processing",
	})

	// Simulate complex operation
	result, err := performComplexOperation()

	if err != nil {
		op.Failed(err, "Complex operation failed")
		c.JSON(500, gin.H{"error": "Operation failed"})
		return
	}

	op.WithField("result", result).Success("Complex operation completed")

	c.JSON(200, gin.H{
		"message": "Operation tracking demo completed",
		"result":  result,
	})
}

func demoErrorLogging(c *gin.Context) {
	ctx := c.Request.Context()

	// Simulate an error
	err := fmt.Errorf("simulated error for demonstration")

	// Log error with context
	logging.Error(ctx, "An error occurred",
		"error", err,
		"error_code", "DEMO_ERROR",
		"severity", "low",
		"action", "demonstration",
	)

	// Log with structured business context
	logging.LogBusiness(ctx, "demo_service", "error_simulation", "error",
		"Simulated error for logging demonstration",
		map[string]interface{}{
			"error_type": "simulated",
			"handled":    true,
			"user_id":    "demo-user",
		},
	)

	c.JSON(400, gin.H{
		"error":   "Simulated error",
		"message": "Check logs for error output",
	})
}

func performComplexOperation() (string, error) {
	// Simulate some work
	time.Sleep(100 * time.Millisecond)

	// Randomly fail for demonstration
	if time.Now().Unix()%2 == 0 {
		return "", fmt.Errorf("random failure for demonstration")
	}

	return "operation_result_" + fmt.Sprint(time.Now().Unix()), nil
}
