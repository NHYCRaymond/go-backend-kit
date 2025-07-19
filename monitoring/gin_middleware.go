package monitoring

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Gin specific metrics
	GinRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gin_requests_total",
			Help: "Total number of HTTP requests processed by Gin",
		},
		[]string{"method", "route", "status_code"},
	)

	GinRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gin_request_duration_seconds",
			Help:    "Duration of HTTP requests processed by Gin",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "route"},
	)

	GinRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gin_request_size_bytes",
			Help:    "Size of HTTP requests processed by Gin",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // 1KB to 512KB
		},
		[]string{"method", "route"},
	)

	GinResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gin_response_size_bytes",
			Help:    "Size of HTTP responses processed by Gin",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // 1KB to 512KB
		},
		[]string{"method", "route"},
	)

	GinActiveRequests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gin_active_requests",
			Help: "Number of active HTTP requests being processed by Gin",
		},
		[]string{"method", "route"},
	)

	// API endpoint specific metrics
	APIEndpointMetrics = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_endpoint_requests_total",
			Help: "Total number of requests per API endpoint",
		},
		[]string{"endpoint", "method", "status_code", "version"},
	)

	APIEndpointDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "api_endpoint_duration_seconds",
			Help:    "Duration of API endpoint requests",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"endpoint", "method", "version"},
	)

	// SLA metrics
	SLAViolations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sla_violations_total",
			Help: "Total number of SLA violations",
		},
		[]string{"endpoint", "threshold"},
	)
)

// responseWriter wraps gin.ResponseWriter to capture response size
type ginResponseWriter struct {
	gin.ResponseWriter
	size int
}

func (w *ginResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.size += n
	return n, err
}

func (w *ginResponseWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	w.size += n
	return n, err
}

// MetricsConfig holds configuration for metrics middleware
type MetricsConfig struct {
	// SLA thresholds for different endpoints (in seconds)
	SLAThresholds map[string]float64
	// Skip paths that should not be monitored
	SkipPaths []string
	// Group similar paths to reduce cardinality
	PathGrouping map[string]string
}

// DefaultMetricsConfig returns default configuration
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		SLAThresholds: map[string]float64{
			"/api/v1/users":    0.5,  // 500ms
			"/api/v1/orders":   1.0,  // 1s
			"/api/v1/products": 0.25, // 250ms
		},
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/ready",
			"/live",
		},
		PathGrouping: map[string]string{
			`/api/v1/users/\d+`:    "/api/v1/users/:id",
			`/api/v1/orders/\d+`:   "/api/v1/orders/:id",
			`/api/v1/products/\d+`: "/api/v1/products/:id",
		},
	}
}

// GinMetricsMiddleware returns a Gin middleware that collects metrics
func GinMetricsMiddleware(config *MetricsConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultMetricsConfig()
	}

	return func(c *gin.Context) {
		// Skip monitoring for certain paths
		for _, path := range config.SkipPaths {
			if c.Request.URL.Path == path {
				c.Next()
				return
			}
		}

		start := time.Now()
		method := c.Request.Method
		
		// Get normalized route path
		route := normalizeRoute(c.FullPath(), c.Request.URL.Path, config.PathGrouping)
		
		// Wrap response writer to capture response size
		wrapped := &ginResponseWriter{
			ResponseWriter: c.Writer,
			size:          0,
		}
		c.Writer = wrapped

		// Increment active requests
		GinActiveRequests.WithLabelValues(method, route).Inc()
		defer GinActiveRequests.WithLabelValues(method, route).Dec()

		// Get request size
		requestSize := c.Request.ContentLength
		if requestSize > 0 {
			GinRequestSize.WithLabelValues(method, route).Observe(float64(requestSize))
		}

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)
		durationSeconds := duration.Seconds()

		// Get status code
		statusCode := c.Writer.Status()
		statusCodeStr := strconv.Itoa(statusCode)

		// Get request ID for monitoring
		requestID := ""
		if id, exists := c.Get("request_id"); exists {
			if reqID, ok := id.(string); ok {
				requestID = reqID
			}
		}
		
		// Record metrics
		GinRequestsTotal.WithLabelValues(method, route, statusCodeStr).Inc()
		GinRequestDuration.WithLabelValues(method, route).Observe(durationSeconds)
		GinResponseSize.WithLabelValues(method, route).Observe(float64(wrapped.size))

		// Record API endpoint metrics
		apiVersion := extractAPIVersion(route)
		endpoint := extractEndpoint(route)
		APIEndpointMetrics.WithLabelValues(endpoint, method, statusCodeStr, apiVersion).Inc()
		APIEndpointDuration.WithLabelValues(endpoint, method, apiVersion).Observe(durationSeconds)

		// Check SLA violations
		if threshold, exists := config.SLAThresholds[route]; exists {
			if durationSeconds > threshold {
				SLAViolations.WithLabelValues(route, fmt.Sprintf("%.3fs", threshold)).Inc()
			}
		}

		// Record slow requests (>1s) as warnings
		if durationSeconds > 1.0 {
			RecordError("slow_request", "api", "warning")
			// Log slow request with request ID
			if requestID != "" {
				// Could integrate with structured logging here
			}
		}

		// Record 5xx errors
		if statusCode >= 500 {
			RecordError("server_error", "api", "error")
		}

		// Record 4xx errors
		if statusCode >= 400 && statusCode < 500 {
			RecordError("client_error", "api", "warning")
		}
	}
}

// normalizeRoute normalizes route path to reduce cardinality
func normalizeRoute(fullPath, requestPath string, pathGrouping map[string]string) string {
	// If Gin provides a route template, use it
	if fullPath != "" {
		return fullPath
	}

	// Apply path grouping rules
	for pattern, replacement := range pathGrouping {
		if matched, _ := regexp.MatchString(pattern, requestPath); matched {
			return replacement
		}
	}

	// Default to request path (might cause high cardinality)
	return requestPath
}

// extractAPIVersion extracts API version from route
func extractAPIVersion(route string) string {
	if strings.Contains(route, "/v1/") {
		return "v1"
	}
	if strings.Contains(route, "/v2/") {
		return "v2"
	}
	return "unknown"
}

// extractEndpoint extracts endpoint name from route
func extractEndpoint(route string) string {
	parts := strings.Split(route, "/")
	if len(parts) >= 4 && parts[1] == "api" {
		return parts[3] // /api/v1/users -> users
	}
	return "unknown"
}

// RecordAPICallWithContext records an API call with additional context
func RecordAPICallWithContext(endpoint, method, status, version string, duration time.Duration) {
	APIEndpointMetrics.WithLabelValues(endpoint, method, status, version).Inc()
	APIEndpointDuration.WithLabelValues(endpoint, method, version).Observe(duration.Seconds())
}

// RecordSLAViolation records an SLA violation
func RecordSLAViolation(endpoint string, threshold float64) {
	SLAViolations.WithLabelValues(endpoint, fmt.Sprintf("%.3fs", threshold)).Inc()
}

// GetAPIMetrics returns current API metrics for health checks
func GetAPIMetrics() map[string]interface{} {
	return map[string]interface{}{
		"endpoints_monitored": len(DefaultMetricsConfig().SLAThresholds),
		"active_requests":     "check_prometheus", // Would need to query Prometheus
		"avg_response_time":   "check_prometheus", // Would need to query Prometheus
	}
}