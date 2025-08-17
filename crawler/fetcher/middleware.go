package fetcher

import (
	"log/slog"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiterMiddleware implements rate limiting
type RateLimiterMiddleware struct {
	name    string
	limiter RateLimiter
}

// NewRateLimiterMiddleware creates a rate limiter middleware
func NewRateLimiterMiddleware(limiter RateLimiter) Middleware {
	return &RateLimiterMiddleware{
		name:    "rate_limiter",
		limiter: limiter,
	}
}

func (m *RateLimiterMiddleware) Name() string {
	return m.name
}

func (m *RateLimiterMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	// Wait for rate limit
	if err := m.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit error: %w", err)
	}
	
	return next(ctx, req)
}

// CacheMiddleware implements request caching
type CacheMiddleware struct {
	name  string
	cache Cache
}

// NewCacheMiddleware creates a cache middleware
func NewCacheMiddleware(cache Cache) Middleware {
	return &CacheMiddleware{
		name:  "cache",
		cache: cache,
	}
}

func (m *CacheMiddleware) Name() string {
	return m.name
}

func (m *CacheMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	// Only cache GET requests
	if req.Method != "GET" && req.Method != "" {
		return next(ctx, req)
	}
	
	// Generate cache key
	key := m.generateCacheKey(req)
	
	// Check cache
	if cached, found := m.cache.Get(key); found {
		if resp, ok := cached.(*Response); ok {
			// Mark as cached
			if resp.Metadata == nil {
				resp.Metadata = make(map[string]interface{})
			}
			resp.Metadata["cached"] = true
			return resp, nil
		}
	}
	
	// Execute request
	resp, err := next(ctx, req)
	if err != nil {
		return resp, err
	}
	
	// Cache successful responses
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		m.cache.Set(key, resp, 5*time.Minute)
	}
	
	return resp, nil
}

func (m *CacheMiddleware) generateCacheKey(req *Request) string {
	h := md5.New()
	h.Write([]byte(req.Method))
	h.Write([]byte(req.URL))
	
	// Include important headers
	if req.Headers != nil {
		for k, v := range req.Headers {
			h.Write([]byte(k))
			h.Write([]byte(v))
		}
	}
	
	return hex.EncodeToString(h.Sum(nil))
}

// RetryMiddleware implements retry logic
type RetryMiddleware struct {
	name       string
	maxRetries int
	backoff    BackoffStrategy
}

// NewRetryMiddleware creates a retry middleware
func NewRetryMiddleware(maxRetries int) Middleware {
	return &RetryMiddleware{
		name:       "retry",
		maxRetries: maxRetries,
		backoff:    NewExponentialBackoff(),
	}
}

func (m *RetryMiddleware) Name() string {
	return m.name
}

func (m *RetryMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	var resp *Response
	var err error
	
	retries := m.maxRetries
	if req.RetryCount > 0 {
		retries = req.RetryCount
	}
	
	for i := 0; i <= retries; i++ {
		resp, err = next(ctx, req)
		
		// Check if should retry
		if err == nil && resp != nil && resp.StatusCode < 500 {
			return resp, nil
		}
		
		// Check custom retry condition
		if req.RetryCondition != nil && !req.RetryCondition(resp, err) {
			return resp, err
		}
		
		// Don't retry on last attempt
		if i == retries {
			break
		}
		
		// Wait before retry
		waitTime := m.backoff.NextInterval(i)
		select {
		case <-time.After(waitTime):
			// Continue to next retry
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		
		// Add retry metadata
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata["retry_attempt"] = i + 1
	}
	
	return resp, err
}

// LoggingMiddleware implements request logging
type LoggingMiddleware struct {
	name   string
	logger *slog.Logger
}

// NewLoggingMiddleware creates a logging middleware
func NewLoggingMiddleware(logger *slog.Logger) Middleware {
	return &LoggingMiddleware{
		name:   "logging",
		logger: logger,
	}
}

func (m *LoggingMiddleware) Name() string {
	return m.name
}

func (m *LoggingMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	// Log request
	m.logger.Debug("Request started",
		"method", req.Method,
		"url", req.URL,
		"user_agent", req.UserAgent,
		"proxy", req.Proxy)
	
	startTime := time.Now()
	resp, err := next(ctx, req)
	duration := time.Since(startTime)
	
	// Log response
	if err != nil {
		m.logger.Error("Request failed",
			"method", req.Method,
			"url", req.URL,
			"duration", duration,
			"error", err)
	} else {
		m.logger.Debug("Request completed",
			"method", req.Method,
			"url", req.URL,
			"status", resp.StatusCode,
			"duration", duration,
			"bytes", len(resp.Body))
	}
	
	return resp, err
}

// MetricsMiddleware collects request metrics
type MetricsMiddleware struct {
	name            string
	totalRequests   int64
	successRequests int64
	failedRequests  int64
	totalDuration   int64
}

// NewMetricsMiddleware creates a metrics middleware
func NewMetricsMiddleware() Middleware {
	return &MetricsMiddleware{
		name: "metrics",
	}
}

func (m *MetricsMiddleware) Name() string {
	return m.name
}

func (m *MetricsMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	atomic.AddInt64(&m.totalRequests, 1)
	
	startTime := time.Now()
	resp, err := next(ctx, req)
	duration := time.Since(startTime)
	
	atomic.AddInt64(&m.totalDuration, int64(duration))
	
	if err == nil && resp != nil && resp.StatusCode < 400 {
		atomic.AddInt64(&m.successRequests, 1)
	} else {
		atomic.AddInt64(&m.failedRequests, 1)
	}
	
	return resp, err
}

func (m *MetricsMiddleware) GetMetrics() map[string]interface{} {
	total := atomic.LoadInt64(&m.totalRequests)
	success := atomic.LoadInt64(&m.successRequests)
	failed := atomic.LoadInt64(&m.failedRequests)
	totalDuration := atomic.LoadInt64(&m.totalDuration)
	
	avgDuration := time.Duration(0)
	if total > 0 {
		avgDuration = time.Duration(totalDuration / total)
	}
	
	return map[string]interface{}{
		"total_requests":   total,
		"success_requests": success,
		"failed_requests":  failed,
		"success_rate":     float64(success) / float64(total) * 100,
		"avg_duration":     avgDuration,
	}
}

// CircuitBreakerMiddleware implements circuit breaker pattern
type CircuitBreakerMiddleware struct {
	name           string
	failureThreshold int
	successThreshold int
	timeout        time.Duration
	
	mu             sync.RWMutex
	failures       int
	successes      int
	lastFailTime   time.Time
	state          CircuitState
}

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// NewCircuitBreakerMiddleware creates a circuit breaker middleware
func NewCircuitBreakerMiddleware(failureThreshold, successThreshold int, timeout time.Duration) Middleware {
	return &CircuitBreakerMiddleware{
		name:             "circuit_breaker",
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
		state:            CircuitClosed,
	}
}

func (m *CircuitBreakerMiddleware) Name() string {
	return m.name
}

func (m *CircuitBreakerMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	m.mu.RLock()
	state := m.state
	m.mu.RUnlock()
	
	// Check circuit state
	switch state {
	case CircuitOpen:
		// Check if timeout has passed
		m.mu.RLock()
		if time.Since(m.lastFailTime) > m.timeout {
			m.mu.RUnlock()
			m.mu.Lock()
			m.state = CircuitHalfOpen
			m.successes = 0
			m.mu.Unlock()
		} else {
			m.mu.RUnlock()
			return nil, fmt.Errorf("circuit breaker is open")
		}
		
	case CircuitHalfOpen:
		// Allow limited requests
	}
	
	// Execute request
	resp, err := next(ctx, req)
	
	// Update circuit state based on result
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		m.failures++
		m.successes = 0
		m.lastFailTime = time.Now()
		
		if m.failures >= m.failureThreshold {
			m.state = CircuitOpen
		}
	} else {
		m.successes++
		m.failures = 0
		
		if m.state == CircuitHalfOpen && m.successes >= m.successThreshold {
			m.state = CircuitClosed
		}
	}
	
	return resp, err
}

// ThrottleMiddleware implements request throttling
type ThrottleMiddleware struct {
	name         string
	maxConcurrent int
	semaphore    chan struct{}
}

// NewThrottleMiddleware creates a throttle middleware
func NewThrottleMiddleware(maxConcurrent int) Middleware {
	return &ThrottleMiddleware{
		name:          "throttle",
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
	}
}

func (m *ThrottleMiddleware) Name() string {
	return m.name
}

func (m *ThrottleMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	select {
	case m.semaphore <- struct{}{}:
		defer func() { <-m.semaphore }()
		return next(ctx, req)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HeaderMiddleware adds/modifies headers
type HeaderMiddleware struct {
	name    string
	headers map[string]string
}

// NewHeaderMiddleware creates a header middleware
func NewHeaderMiddleware(headers map[string]string) Middleware {
	return &HeaderMiddleware{
		name:    "header",
		headers: headers,
	}
}

func (m *HeaderMiddleware) Name() string {
	return m.name
}

func (m *HeaderMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	// Add headers to request
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	
	for k, v := range m.headers {
		if _, exists := req.Headers[k]; !exists {
			req.Headers[k] = v
		}
	}
	
	return next(ctx, req)
}

// CompressionMiddleware handles response compression
type CompressionMiddleware struct {
	name string
}

// NewCompressionMiddleware creates a compression middleware
func NewCompressionMiddleware() Middleware {
	return &CompressionMiddleware{
		name: "compression",
	}
}

func (m *CompressionMiddleware) Name() string {
	return m.name
}

func (m *CompressionMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	// Add Accept-Encoding header
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers["Accept-Encoding"] = "gzip, deflate, br"
	
	resp, err := next(ctx, req)
	
	// Response decompression is handled by resty automatically
	return resp, err
}

// FingerprintMiddleware generates request fingerprints
type FingerprintMiddleware struct {
	name string
}

// NewFingerprintMiddleware creates a fingerprint middleware
func NewFingerprintMiddleware() Middleware {
	return &FingerprintMiddleware{
		name: "fingerprint",
	}
}

func (m *FingerprintMiddleware) Name() string {
	return m.name
}

func (m *FingerprintMiddleware) Process(ctx context.Context, req *Request, next Handler) (*Response, error) {
	// Generate fingerprint
	fingerprint := m.generateFingerprint(req)
	
	// Add to metadata
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata["fingerprint"] = fingerprint
	
	resp, err := next(ctx, req)
	
	// Add fingerprint to response
	if resp != nil {
		if resp.Metadata == nil {
			resp.Metadata = make(map[string]interface{})
		}
		resp.Metadata["fingerprint"] = fingerprint
	}
	
	return resp, err
}

func (m *FingerprintMiddleware) generateFingerprint(req *Request) string {
	h := md5.New()
	h.Write([]byte(req.Method))
	h.Write([]byte(req.URL))
	h.Write([]byte(req.UserAgent))
	
	if req.Body != nil {
		h.Write([]byte(fmt.Sprintf("%v", req.Body)))
	}
	
	return hex.EncodeToString(h.Sum(nil))
}

// Interfaces

// RateLimiter controls request rate
type RateLimiter interface {
	Wait(ctx context.Context) error
	Allow() bool
}

// Cache stores responses
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration)
	Delete(key string)
	Clear()
}

// BackoffStrategy defines retry backoff strategy
type BackoffStrategy interface {
	NextInterval(retry int) time.Duration
}

// ExponentialBackoff implements exponential backoff
type ExponentialBackoff struct {
	baseDelay time.Duration
	maxDelay  time.Duration
}

// NewExponentialBackoff creates exponential backoff
func NewExponentialBackoff() BackoffStrategy {
	return &ExponentialBackoff{
		baseDelay: 1 * time.Second,
		maxDelay:  30 * time.Second,
	}
}

func (b *ExponentialBackoff) NextInterval(retry int) time.Duration {
	delay := b.baseDelay * time.Duration(1<<uint(retry))
	if delay > b.maxDelay {
		delay = b.maxDelay
	}
	return delay
}

// DefaultRateLimiter implements a simple rate limiter
type DefaultRateLimiter struct {
	limiter *rate.Limiter
}

// NewDefaultRateLimiter creates a default rate limiter
func NewDefaultRateLimiter(rps int) RateLimiter {
	return &DefaultRateLimiter{
		limiter: rate.NewLimiter(rate.Limit(rps), rps),
	}
}

func (l *DefaultRateLimiter) Wait(ctx context.Context) error {
	return l.limiter.Wait(ctx)
}

func (l *DefaultRateLimiter) Allow() bool {
	return l.limiter.Allow()
}

// MemoryCache implements a simple in-memory cache
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]*cacheItem
}

type cacheItem struct {
	value     interface{}
	expiresAt time.Time
}

// NewMemoryCache creates a memory cache
func NewMemoryCache() Cache {
	cache := &MemoryCache{
		items: make(map[string]*cacheItem),
	}
	
	// Start cleanup goroutine
	go cache.cleanup()
	
	return cache
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	item, exists := c.items[key]
	if !exists {
		return nil, false
	}
	
	if time.Now().After(item.expiresAt) {
		return nil, false
	}
	
	return item.value, true
}

func (c *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.items[key] = &cacheItem{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
}

func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*cacheItem)
}

func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, item := range c.items {
			if now.After(item.expiresAt) {
				delete(c.items, key)
			}
		}
		c.mu.Unlock()
	}
}