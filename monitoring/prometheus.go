package monitoring

import (
	"crypto/md5"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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