package monitoring

import (
	"crypto/md5"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTP metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// Database metrics
	DBConnectionsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "db_connections_active",
			Help: "Number of active database connections",
		},
		[]string{"database"},
	)

	DBConnectionsIdle = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "db_connections_idle",
			Help: "Number of idle database connections",
		},
		[]string{"database"},
	)

	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "Duration of database queries",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	DBQueryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "db_query_total",
			Help: "Total number of database queries",
		},
		[]string{"operation", "status"},
	)

	// Redis metrics
	RedisCommandsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_commands_total",
			Help: "Total number of Redis commands",
		},
		[]string{"command", "status"},
	)

	RedisCommandsDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_commands_duration_seconds",
			Help:    "Duration of Redis commands",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"command"},
	)

	RedisConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "redis_connections_active",
			Help: "Number of active Redis connections",
		},
	)

	// Message queue metrics
	MessageQueuePublishedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "message_queue_published_total",
			Help: "Total number of messages published to queue",
		},
		[]string{"queue", "status"},
	)

	MessageQueueConsumedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "message_queue_consumed_total",
			Help: "Total number of messages consumed from queue",
		},
		[]string{"queue", "status"},
	)

	MessageQueueProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "message_queue_processing_duration_seconds",
			Help:    "Duration of message processing",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"queue"},
	)

	// Application metrics
	AppInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_info",
			Help: "Application information",
		},
		[]string{"version", "instance_id"},
	)

	AppUptime = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "app_uptime_seconds",
			Help: "Application uptime in seconds",
		},
	)

	// Custom business metrics
	ActiveUsers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_users",
			Help: "Number of active users",
		},
	)

	RequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "requests_in_flight",
			Help: "Number of requests currently being processed",
		},
	)

	// System metrics
	CPUUsage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "cpu_usage_percent",
			Help: "Current CPU usage percentage",
		},
	)

	MemoryUsage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_usage_bytes",
			Help: "Current memory usage in bytes",
		},
	)

	MemoryTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_total_bytes",
			Help: "Total available memory in bytes",
		},
	)

	Goroutines = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "goroutines_total",
			Help: "Number of goroutines currently running",
		},
	)

	GCDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "gc_duration_seconds",
			Help:    "Time spent in garbage collection",
			Buckets: prometheus.DefBuckets,
		},
	)

	// Middleware metrics
	AuthenticationAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "authentication_attempts_total",
			Help: "Total number of authentication attempts",
		},
		[]string{"status", "method"},
	)

	RateLimitHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_hits_total",
			Help: "Total number of rate limit hits",
		},
		[]string{"key", "endpoint"},
	)

	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache_type", "key"},
	)

	CacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"cache_type", "key"},
	)

	// Business domain metrics
	UserRegistrations = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "user_registrations_total",
			Help: "Total number of user registrations",
		},
	)

	UserLogins = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "user_logins_total",
			Help: "Total number of user logins",
		},
		[]string{"status"},
	)

	APICallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_calls_total",
			Help: "Total number of API calls",
		},
		[]string{"service", "operation", "status"},
	)

	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "errors_total",
			Help: "Total number of errors",
		},
		[]string{"type", "component", "severity"},
	)

	// External service metrics
	ExternalServiceCalls = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_service_calls_total",
			Help: "Total number of external service calls",
		},
		[]string{"service", "endpoint", "status"},
	)

	ExternalServiceDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "external_service_duration_seconds",
			Help:    "Duration of external service calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "endpoint"},
	)
)

// StartServer starts the Prometheus metrics server
func StartServer(cfg *config.MonitoringConfig, logger *slog.Logger) {
	if cfg.Port == 0 {
		cfg.Port = 9090
	}

	instanceID := getInstanceID()
	logger.Info("Starting Prometheus metrics server", "port", cfg.Port, "instance_id", instanceID)

	// Register instance info
	AppInfo.WithLabelValues("unknown", instanceID).Set(1)

	// Start uptime tracking
	startTime := time.Now()
	go func() {
		for {
			AppUptime.Set(time.Since(startTime).Seconds())
			time.Sleep(10 * time.Second)
		}
	}()

	// Start system metrics collection
	go collectSystemMetrics()

	mux := http.NewServeMux()

	// Metrics endpoint
	metricsPath := "/metrics"
	if cfg.Path != "" {
		metricsPath = cfg.Path
	}
	mux.Handle(metricsPath, promhttp.Handler())

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Ready endpoint for Kubernetes readiness probe
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	})

	// Live endpoint for Kubernetes liveness probe
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Alive"))
	})

	addr := fmt.Sprintf(":%d", cfg.Port)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			logger.Error("Prometheus metrics server failed", "error", err)
		}
	}()

	logger.Info("Prometheus metrics server started", "address", addr, "path", metricsPath)
}

// getInstanceID generates a unique instance ID
func getInstanceID() string {
	// Try environment variable first
	if instanceID := os.Getenv("INSTANCE_ID"); instanceID != "" {
		return instanceID
	}

	// Fallback to hostname-based ID
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return fmt.Sprintf("%x", md5.Sum([]byte(hostname)))
}

// HTTPMetricsMiddleware returns a middleware that collects HTTP metrics
func HTTPMetricsMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Increment in-flight requests
			RequestsInFlight.Inc()
			defer RequestsInFlight.Dec()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Serve the request
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()

			HTTPRequestsTotal.WithLabelValues(
				r.Method,
				r.URL.Path,
				fmt.Sprintf("%d", wrapped.statusCode),
			).Inc()

			HTTPRequestDuration.WithLabelValues(
				r.Method,
				r.URL.Path,
			).Observe(duration)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RecordDBMetrics records database metrics
func RecordDBMetrics(operation string, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}

	DBQueryTotal.WithLabelValues(operation, status).Inc()
	DBQueryDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordRedisMetrics records Redis metrics
func RecordRedisMetrics(command string, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}

	RedisCommandsTotal.WithLabelValues(command, status).Inc()
	RedisCommandsDuration.WithLabelValues(command).Observe(duration.Seconds())
}

// RecordMessageQueueMetrics records message queue metrics
func RecordMessageQueueMetrics(queue, operation string, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}

	if operation == "publish" {
		MessageQueuePublishedTotal.WithLabelValues(queue, status).Inc()
	} else if operation == "consume" {
		MessageQueueConsumedTotal.WithLabelValues(queue, status).Inc()
	}

	MessageQueueProcessingDuration.WithLabelValues(queue).Observe(duration.Seconds())
}

// SetActiveUsers sets the number of active users
func SetActiveUsers(count float64) {
	ActiveUsers.Set(count)
}

// SetDBConnections sets database connection metrics
func SetDBConnections(database string, active, idle int) {
	DBConnectionsActive.WithLabelValues(database).Set(float64(active))
	DBConnectionsIdle.WithLabelValues(database).Set(float64(idle))
}

// SetRedisConnections sets Redis connection metrics
func SetRedisConnections(active int) {
	RedisConnectionsActive.Set(float64(active))
}

// SetAppInfo sets application information
func SetAppInfo(version, instanceID string) {
	AppInfo.WithLabelValues(version, instanceID).Set(1)
}

// collectSystemMetrics collects system metrics periodically
func collectSystemMetrics() {
	var m runtime.MemStats
	for {
		// Collect memory stats
		runtime.ReadMemStats(&m)
		MemoryUsage.Set(float64(m.Alloc))
		MemoryTotal.Set(float64(m.Sys))

		// Collect goroutine count
		Goroutines.Set(float64(runtime.NumGoroutine()))

		// Collect GC stats
		GCDuration.Observe(float64(m.PauseNs[(m.NumGC+255)%256]) / 1e9)

		time.Sleep(15 * time.Second)
	}
}

// RecordAuthenticationAttempt records authentication attempt metrics
func RecordAuthenticationAttempt(status, method string) {
	AuthenticationAttempts.WithLabelValues(status, method).Inc()
}

// RecordRateLimitHit records rate limit hit metrics
func RecordRateLimitHit(key, endpoint string) {
	RateLimitHits.WithLabelValues(key, endpoint).Inc()
}

// RecordCacheHit records cache hit metrics
func RecordCacheHit(cacheType, key string) {
	CacheHits.WithLabelValues(cacheType, key).Inc()
}

// RecordCacheMiss records cache miss metrics
func RecordCacheMiss(cacheType, key string) {
	CacheMisses.WithLabelValues(cacheType, key).Inc()
}

// RecordUserRegistration records user registration metrics
func RecordUserRegistration() {
	UserRegistrations.Inc()
}

// RecordUserLogin records user login metrics
func RecordUserLogin(status string) {
	UserLogins.WithLabelValues(status).Inc()
}

// RecordAPICall records API call metrics
func RecordAPICall(service, operation, status string) {
	APICallsTotal.WithLabelValues(service, operation, status).Inc()
}

// RecordError records error metrics
func RecordError(errorType, component, severity string) {
	ErrorsTotal.WithLabelValues(errorType, component, severity).Inc()
}

// RecordExternalServiceCall records external service call metrics
func RecordExternalServiceCall(service, endpoint, status string, duration time.Duration) {
	ExternalServiceCalls.WithLabelValues(service, endpoint, status).Inc()
	ExternalServiceDuration.WithLabelValues(service, endpoint).Observe(duration.Seconds())
}

// GetCacheHitRatio calculates cache hit ratio
func GetCacheHitRatio(cacheType string) float64 {
	// This would need to be implemented based on your cache implementation
	// For now, return 0 as placeholder
	return 0.0
}

// DatabaseMetricsCollector collects database metrics from database instances
type DatabaseMetricsCollector struct {
	databases map[string]interface{}
}

// NewDatabaseMetricsCollector creates a new database metrics collector
func NewDatabaseMetricsCollector(databases map[string]interface{}) *DatabaseMetricsCollector {
	return &DatabaseMetricsCollector{
		databases: databases,
	}
}

// CollectDatabaseMetrics collects metrics from all registered databases
func (c *DatabaseMetricsCollector) CollectDatabaseMetrics() {
	for name, _ := range c.databases {
		// This would need to be implemented based on your database interface
		// For now, just set placeholder values
		SetDBConnections(name, 10, 5) // active, idle
	}
}

// HealthCheckCollector provides health check metrics
type HealthCheckCollector struct {
	healthChecks map[string]func() error
}

// NewHealthCheckCollector creates a new health check collector
func NewHealthCheckCollector() *HealthCheckCollector {
	return &HealthCheckCollector{
		healthChecks: make(map[string]func() error),
	}
}

// RegisterHealthCheck registers a health check function
func (c *HealthCheckCollector) RegisterHealthCheck(name string, check func() error) {
	c.healthChecks[name] = check
}

// CollectHealthMetrics collects health check metrics
func (c *HealthCheckCollector) CollectHealthMetrics() {
	for name, check := range c.healthChecks {
		err := check()
		if err != nil {
			RecordError("health_check", name, "error")
		}
	}
}
