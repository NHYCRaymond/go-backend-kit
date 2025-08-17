package middleware

import (
	"net/http"
	"strings"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SecurityMiddleware handles security headers and CORS
type SecurityMiddleware struct {
	config config.SecurityConfig
}

// NewSecurityMiddleware creates a new security middleware
func NewSecurityMiddleware(cfg config.SecurityConfig) *SecurityMiddleware {
	return &SecurityMiddleware{
		config: cfg,
	}
}

// Handler returns the middleware handler function
func (s *SecurityMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Add security headers
		if s.config.EnableSecurityHeaders {
			s.addSecurityHeaders(c)
		}

		c.Next()
	}
}

// addSecurityHeaders adds security headers to response
func (s *SecurityMiddleware) addSecurityHeaders(c *gin.Context) {
	// X-Frame-Options
	frameOptions := s.config.FrameOptions
	if frameOptions == "" {
		frameOptions = "DENY"
	}
	c.Header("X-Frame-Options", frameOptions)

	// X-Content-Type-Options
	contentTypeOptions := s.config.ContentTypeOptions
	if contentTypeOptions == "" {
		contentTypeOptions = "nosniff"
	}
	c.Header("X-Content-Type-Options", contentTypeOptions)

	// X-XSS-Protection
	c.Header("X-XSS-Protection", "1; mode=block")

	// Referrer-Policy
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

	// Content-Security-Policy
	c.Header("Content-Security-Policy", "default-src 'self'")

	// Strict-Transport-Security (HSTS)
	if c.Request.TLS != nil {
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

// CORSMiddleware creates CORS middleware
func (s *SecurityMiddleware) CORSMiddleware() gin.HandlerFunc {
	if !s.config.EnableCORS {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	corsConfig := cors.DefaultConfig()

	// Set allowed origins
	if len(s.config.CORSOrigins) > 0 {
		corsConfig.AllowOrigins = s.config.CORSOrigins
	} else {
		corsConfig.AllowAllOrigins = true
	}

	// Set allowed methods
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

	// Set allowed headers
	corsConfig.AllowHeaders = []string{
		"Origin",
		"Content-Length",
		"Content-Type",
		"Authorization",
		"X-Requested-With",
		"X-Request-ID",
		"X-API-Key",
		"Accept",
		"Accept-Encoding",
		"Accept-Language",
		"Cache-Control",
	}

	// Set exposed headers
	corsConfig.ExposeHeaders = []string{
		"X-Request-ID",
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Reset",
	}

	// Allow credentials
	corsConfig.AllowCredentials = true

	return cors.New(corsConfig)
}

// NoCache middleware adds no-cache headers
func NoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate, value")
		c.Header("Expires", "Thu, 01 Jan 1970 00:00:00 GMT")
		c.Header("Last-Modified", "Thu, 01 Jan 1970 00:00:00 GMT")
		c.Next()
	}
}

// SecureHeaders middleware adds security headers
func SecureHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")

		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

// TrustedProxies middleware validates trusted proxies
func TrustedProxies(trustedProxies []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(trustedProxies) == 0 {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		trusted := false

		for _, proxy := range trustedProxies {
			if clientIP == proxy || strings.HasPrefix(clientIP, proxy) {
				trusted = true
				break
			}
		}

		if !trusted {
			c.Header("X-Forwarded-For", "")
			c.Header("X-Real-IP", "")
		}

		c.Next()
	}
}

// HTTPSRedirect middleware redirects HTTP to HTTPS
func HTTPSRedirect() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Header.Get("X-Forwarded-Proto") != "https" && c.Request.TLS == nil {
			httpsURL := "https://" + c.Request.Host + c.Request.RequestURI
			c.Redirect(http.StatusMovedPermanently, httpsURL)
			c.Abort()
			return
		}
		c.Next()
	}
}

// IPWhitelist middleware allows only whitelisted IPs
func IPWhitelist(allowedIPs []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(allowedIPs) == 0 {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		allowed := false

		for _, ip := range allowedIPs {
			if clientIP == ip {
				allowed = true
				break
			}
		}

		if !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "IP not whitelisted",
			})
			return
		}

		c.Next()
	}
}

// UserAgent middleware validates user agent
func UserAgent(requiredUserAgent string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if requiredUserAgent == "" {
			c.Next()
			return
		}

		userAgent := c.Request.UserAgent()
		if !strings.Contains(userAgent, requiredUserAgent) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Invalid user agent",
			})
			return
		}

		c.Next()
	}
}
