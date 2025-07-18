package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/response"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// RateLimitMiddleware handles rate limiting
type RateLimitMiddleware struct {
	config config.RateLimitConfig
	redis  *redis.Client
}

// NewRateLimitMiddleware creates a new rate limit middleware
func NewRateLimitMiddleware(cfg config.RateLimitConfig, redis *redis.Client) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		config: cfg,
		redis:  redis,
	}
}

// Handler returns the middleware handler function
func (r *RateLimitMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if rate limiting is disabled
		if !r.config.Enable {
			c.Next()
			return
		}

		// Skip rate limiting for configured paths
		for _, path := range r.config.SkipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		// Generate rate limit key
		key := r.generateKey(c)

		// Check rate limit
		allowed, remaining, resetTime, err := r.checkRateLimit(c, key)
		if err != nil {
			// Log error but don't block request
			c.Next()
			return
		}

		// Add rate limit headers if enabled
		if r.config.Headers {
			c.Header("X-RateLimit-Limit", strconv.FormatInt(r.config.Limit, 10))
			c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
			c.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
		}

		// Block request if rate limit exceeded
		if !allowed {
			response.Error(c, http.StatusTooManyRequests, "rate_limit_exceeded", "Rate limit exceeded")
			c.Abort()
			return
		}

		c.Next()
	}
}

// generateKey generates rate limit key based on configuration
func (r *RateLimitMiddleware) generateKey(c *gin.Context) string {
	var keyParts []string

	switch r.config.KeyGenerator {
	case "ip":
		keyParts = []string{"ratelimit", "ip", c.ClientIP()}
	case "user":
		if userID, exists := c.Get("user_id"); exists {
			keyParts = []string{"ratelimit", "user", userID.(string)}
		} else {
			keyParts = []string{"ratelimit", "ip", c.ClientIP()}
		}
	case "ip_path":
		keyParts = []string{"ratelimit", "ip", c.ClientIP(), "path", c.Request.URL.Path}
	case "user_path":
		if userID, exists := c.Get("user_id"); exists {
			keyParts = []string{"ratelimit", "user", userID.(string), "path", c.Request.URL.Path}
		} else {
			keyParts = []string{"ratelimit", "ip", c.ClientIP(), "path", c.Request.URL.Path}
		}
	default:
		keyParts = []string{"ratelimit", "ip", c.ClientIP()}
	}

	return strings.Join(keyParts, ":")
}

// checkRateLimit checks if request is within rate limit
func (r *RateLimitMiddleware) checkRateLimit(c *gin.Context, key string) (bool, int64, time.Time, error) {
	ctx := context.Background()

	// Parse window duration
	window, err := time.ParseDuration(r.config.Window)
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("invalid window duration: %w", err)
	}

	// Use sliding window log algorithm
	now := time.Now()
	windowStart := now.Add(-window)

	// Redis pipeline for atomic operations
	pipe := r.redis.Pipeline()

	// Remove expired entries
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart.UnixNano(), 10))

	// Count current requests
	countCmd := pipe.ZCard(ctx, key)

	// Add current request
	pipe.ZAdd(ctx, key, &redis.Z{
		Score:  float64(now.UnixNano()),
		Member: now.UnixNano(),
	})

	// Set expiry
	pipe.Expire(ctx, key, window)

	// Execute pipeline
	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("redis pipeline error: %w", err)
	}

	// Get current count
	currentCount := countCmd.Val()

	// Check if limit exceeded
	allowed := currentCount < r.config.Limit
	remaining := r.config.Limit - currentCount
	if remaining < 0 {
		remaining = 0
	}

	// Calculate reset time
	resetTime := now.Add(window)

	return allowed, remaining, resetTime, nil
}

// FixedWindowRateLimit implements fixed window rate limiting
func (r *RateLimitMiddleware) FixedWindowRateLimit(c *gin.Context, key string) (bool, int64, time.Time, error) {
	ctx := context.Background()

	// Parse window duration
	window, err := time.ParseDuration(r.config.Window)
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("invalid window duration: %w", err)
	}

	// Calculate window key
	now := time.Now()
	windowStart := now.Truncate(window)
	windowKey := fmt.Sprintf("%s:%d", key, windowStart.Unix())

	// Increment counter
	count, err := r.redis.Incr(ctx, windowKey).Result()
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("redis incr error: %w", err)
	}

	// Set expiry on first request
	if count == 1 {
		r.redis.Expire(ctx, windowKey, window)
	}

	// Check if limit exceeded
	allowed := count <= r.config.Limit
	remaining := r.config.Limit - count
	if remaining < 0 {
		remaining = 0
	}

	// Calculate reset time
	resetTime := windowStart.Add(window)

	return allowed, remaining, resetTime, nil
}

// ClearRateLimit clears rate limit for a specific key
func (r *RateLimitMiddleware) ClearRateLimit(ctx context.Context, key string) error {
	return r.redis.Del(ctx, key).Err()
}

// GetRateLimitStatus gets current rate limit status
func (r *RateLimitMiddleware) GetRateLimitStatus(ctx context.Context, key string) (int64, time.Time, error) {
	// Parse window duration
	window, err := time.ParseDuration(r.config.Window)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("invalid window duration: %w", err)
	}

	now := time.Now()
	windowStart := now.Add(-window)

	// Remove expired entries and count
	pipe := r.redis.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart.UnixNano(), 10))
	countCmd := pipe.ZCard(ctx, key)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("redis pipeline error: %w", err)
	}

	currentCount := countCmd.Val()
	resetTime := now.Add(window)

	return currentCount, resetTime, nil
}