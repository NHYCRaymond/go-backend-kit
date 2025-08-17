package fetcher

import (
	"log/slog"
	"context"
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/core"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/go-resty/resty/v2"
)

// Fetcher is the high-level abstraction for HTTP requests
type Fetcher interface {
	// Execute performs the HTTP request
	Execute(ctx context.Context, req *Request) (*Response, error)

	// ExecuteBatch performs multiple requests
	ExecuteBatch(ctx context.Context, reqs []*Request) ([]*Response, error)

	// WithMiddleware adds middleware to the fetcher
	WithMiddleware(middleware ...Middleware) Fetcher

	// WithOptions applies options to the fetcher
	WithOptions(opts ...Option) Fetcher

	// Clone creates a copy of the fetcher with isolated settings
	Clone() Fetcher

	// Stats returns fetcher statistics
	Stats() *Stats
}

// Request represents a high-level HTTP request
type Request struct {
	// Basic
	ID       string            `json:"id"`
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers,omitempty"`
	Cookies  []*http.Cookie    `json:"cookies,omitempty"`
	Body     interface{}       `json:"body,omitempty"`
	FormData map[string]string `json:"form_data,omitempty"`

	// Advanced
	Proxy          string         `json:"proxy,omitempty"`
	UserAgent      string         `json:"user_agent,omitempty"`
	Timeout        time.Duration  `json:"timeout,omitempty"`
	RetryCount     int            `json:"retry_count,omitempty"`
	RetryCondition RetryCondition `json:"-"`

	// Features
	FollowRedirect   bool `json:"follow_redirect"`
	DisableKeepAlive bool `json:"disable_keep_alive"`
	SkipTLSVerify    bool `json:"skip_tls_verify"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Priority int                    `json:"priority"`

	// Context for request lifecycle
	Context context.Context `json:"-"`
}

// Response represents a high-level HTTP response
type Response struct {
	// Basic
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`

	// Request info
	Request *Request `json:"-"`
	URL     string   `json:"url"`
	Method  string   `json:"method"`

	// Timing
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`

	// Network info
	RemoteAddr string `json:"remote_addr,omitempty"`
	TLSVersion string `json:"tls_version,omitempty"`

	// Error if any
	Error    error  `json:"-"`
	ErrorMsg string `json:"error,omitempty"`
	
	// Metadata for middleware use
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Stats contains fetcher statistics
type Stats struct {
	TotalRequests   int64         `json:"total_requests"`
	SuccessRequests int64         `json:"success_requests"`
	FailedRequests  int64         `json:"failed_requests"`
	BytesDownloaded int64         `json:"bytes_downloaded"`
	BytesUploaded   int64         `json:"bytes_uploaded"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	LastRequestTime time.Time     `json:"last_request_time"`
}

// Middleware processes requests before/after execution
type Middleware interface {
	// Name returns middleware name
	Name() string

	// Process handles the request
	Process(ctx context.Context, req *Request, next Handler) (*Response, error)
}

// Handler is the request handler function
type Handler func(ctx context.Context, req *Request) (*Response, error)

// Option configures the fetcher
type Option func(*fetcherImpl)

// RetryCondition determines if a request should be retried
type RetryCondition func(resp *Response, err error) bool

// defaultFetcher is the standard implementation
type fetcherImpl struct {
	// Core
	client      *resty.Client
	middlewares []Middleware
	logger      *slog.Logger

	// Components
	proxyPool     *ProxyPool
	userAgentPool *UserAgentPool
	rateLimiter   RateLimiter
	cache         Cache

	// Configuration
	config *Config

	// Statistics
	stats   *Stats
	statsMu sync.RWMutex
}

// Config contains fetcher configuration
type Config struct {
	// Timeouts
	Timeout         time.Duration `json:"timeout"`
	IdleConnTimeout time.Duration `json:"idle_conn_timeout"`

	// Connection pool
	MaxIdleConns        int `json:"max_idle_conns"`
	MaxIdleConnsPerHost int `json:"max_idle_conns_per_host"`
	MaxConnsPerHost     int `json:"max_conns_per_host"`

	// Retry
	MaxRetries       int           `json:"max_retries"`
	RetryWaitTime    time.Duration `json:"retry_wait_time"`
	RetryMaxWaitTime time.Duration `json:"retry_max_wait_time"`

	// Features
	EnableCookie      bool `json:"enable_cookie"`
	EnableCache       bool `json:"enable_cache"`
	EnableMetrics     bool `json:"enable_metrics"`
	EnableCompression bool `json:"enable_compression"`

	// TLS
	InsecureSkipVerify bool     `json:"insecure_skip_verify"`
	TLSClientCertFile  string   `json:"tls_client_cert_file"`
	TLSClientKeyFile   string   `json:"tls_client_key_file"`
	RootCAs            []string `json:"root_cas"`

	// Debug
	Debug        bool `json:"debug"`
	DebugBody    bool `json:"debug_body"`
	TraceRequest bool `json:"trace_request"`
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Timeout:             30 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     100,
		MaxRetries:          3,
		RetryWaitTime:       1 * time.Second,
		RetryMaxWaitTime:    10 * time.Second,
		EnableCookie:        true,
		EnableCompression:   true,
		EnableMetrics:       true,
	}
}

// New creates a new fetcher with default settings
func New(opts ...Option) Fetcher {
	config := DefaultConfig()
	logger := logging.GetLogger()

	// Create resty client
	client := resty.New()
	client.SetTimeout(config.Timeout)
	client.SetRetryCount(config.MaxRetries)
	client.SetRetryWaitTime(config.RetryWaitTime)
	client.SetRetryMaxWaitTime(config.RetryMaxWaitTime)

	// Configure transport
	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify,
		},
	}
	client.SetTransport(transport)

	// Enable features
	if config.EnableCookie {
		client.SetCookieJar(http.DefaultClient.Jar)
	}

	if config.Debug {
		client.SetDebug(true)
	}

	fetcher := &fetcherImpl{
		client:      client,
		middlewares: []Middleware{},
		logger:      logger,
		config:      config,
		stats:       &Stats{},
	}

	// Apply options
	for _, opt := range opts {
		opt(fetcher)
	}

	// Add default middlewares
	fetcher.setupDefaultMiddlewares()

	return fetcher
}

// Execute performs a single HTTP request
func (f *fetcherImpl) Execute(ctx context.Context, req *Request) (*Response, error) {
	// Apply context
	if req.Context == nil {
		req.Context = ctx
	}

	// Build middleware chain
	handler := f.buildHandler()

	// Execute through middleware chain
	resp, err := f.executeWithMiddleware(req.Context, req, handler)

	// Update statistics
	f.updateStats(resp, err)

	return resp, err
}

// ExecuteBatch performs multiple requests
func (f *fetcherImpl) ExecuteBatch(ctx context.Context, reqs []*Request) ([]*Response, error) {
	responses := make([]*Response, len(reqs))
	var wg sync.WaitGroup

	// Use semaphore to limit concurrency
	sem := make(chan struct{}, f.config.MaxConnsPerHost)

	for i, req := range reqs {
		wg.Add(1)
		go func(idx int, r *Request) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := f.Execute(ctx, r)
			if err != nil {
				resp = &Response{
					Request:  r,
					Error:    err,
					ErrorMsg: err.Error(),
				}
			}
			responses[idx] = resp
		}(i, req)
	}

	wg.Wait()
	return responses, nil
}

// WithMiddleware adds middleware to the fetcher
func (f *fetcherImpl) WithMiddleware(middleware ...Middleware) Fetcher {
	f.middlewares = append(f.middlewares, middleware...)
	return f
}

// WithOptions applies options to the fetcher
func (f *fetcherImpl) WithOptions(opts ...Option) Fetcher {
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Clone creates a copy of the fetcher
func (f *fetcherImpl) Clone() Fetcher {
	// Create new resty client with same config
	newClient := resty.New()

	// Copy configuration
	newClient.SetTimeout(f.config.Timeout)
	newClient.SetRetryCount(f.config.MaxRetries)
	newClient.SetRetryWaitTime(f.config.RetryWaitTime)
	newClient.SetRetryMaxWaitTime(f.config.RetryMaxWaitTime)

	// Copy transport settings
	if transport, ok := f.client.GetClient().Transport.(*http.Transport); ok {
		newTransport := transport.Clone()
		newClient.SetTransport(newTransport)
	}

	return &fetcherImpl{
		client:        newClient,
		middlewares:   append([]Middleware{}, f.middlewares...),
		logger:        f.logger,
		proxyPool:     f.proxyPool,
		userAgentPool: f.userAgentPool,
		rateLimiter:   f.rateLimiter,
		cache:         f.cache,
		config:        f.config,
		stats:         &Stats{},
	}
}

// Stats returns fetcher statistics
func (f *fetcherImpl) Stats() *Stats {
	f.statsMu.RLock()
	defer f.statsMu.RUnlock()

	return &Stats{
		TotalRequests:   f.stats.TotalRequests,
		SuccessRequests: f.stats.SuccessRequests,
		FailedRequests:  f.stats.FailedRequests,
		BytesDownloaded: f.stats.BytesDownloaded,
		BytesUploaded:   f.stats.BytesUploaded,
		AvgResponseTime: f.stats.AvgResponseTime,
		LastRequestTime: f.stats.LastRequestTime,
	}
}

// buildHandler builds the core request handler
func (f *fetcherImpl) buildHandler() Handler {
	return func(ctx context.Context, req *Request) (*Response, error) {
		// Create resty request
		restyReq := f.client.R().SetContext(ctx)

		// Set method
		if req.Method == "" {
			req.Method = "GET"
		}

		// Set headers
		if req.Headers != nil {
			restyReq.SetHeaders(req.Headers)
		}

		// Set user agent
		if req.UserAgent != "" {
			restyReq.SetHeader("User-Agent", req.UserAgent)
		} else if f.userAgentPool != nil {
			restyReq.SetHeader("User-Agent", f.userAgentPool.Get())
		}

		// Set cookies
		if req.Cookies != nil {
			for _, cookie := range req.Cookies {
				restyReq.SetCookie(cookie)
			}
		}

		// Set body
		if req.Body != nil {
			restyReq.SetBody(req.Body)
		}

		// Set form data
		if req.FormData != nil {
			restyReq.SetFormData(req.FormData)
		}

		// Set proxy
		if req.Proxy != "" {
			f.client.SetProxy(req.Proxy)
		} else if f.proxyPool != nil {
			proxy := f.proxyPool.Get()
			if proxy != nil {
				f.client.SetProxy(proxy.URL)
			}
		}

		// Set timeout
		if req.Timeout > 0 {
			restyReq.SetContext(context.WithValue(ctx, "timeout", req.Timeout))
		}

		// Execute request
		startTime := time.Now()
		restyResp, err := restyReq.Execute(req.Method, req.URL)
		duration := time.Since(startTime)

		// Build response
		resp := &Response{
			Request:   req,
			URL:       req.URL,
			Method:    req.Method,
			StartTime: startTime,
			EndTime:   time.Now(),
			Duration:  duration,
		}

		if err != nil {
			resp.Error = err
			resp.ErrorMsg = err.Error()
			return resp, err
		}

		// Copy response data
		resp.StatusCode = restyResp.StatusCode()
		resp.Body = restyResp.Body()
		resp.Headers = make(map[string]string)

		for key, values := range restyResp.Header() {
			if len(values) > 0 {
				resp.Headers[key] = values[0]
			}
		}

		return resp, nil
	}
}

// executeWithMiddleware executes request through middleware chain
func (f *fetcherImpl) executeWithMiddleware(ctx context.Context, req *Request, handler Handler) (*Response, error) {
	// Build middleware chain
	chain := handler

	// Apply middlewares in reverse order
	for i := len(f.middlewares) - 1; i >= 0; i-- {
		middleware := f.middlewares[i]
		next := chain
		chain = func(ctx context.Context, req *Request) (*Response, error) {
			return middleware.Process(ctx, req, next)
		}
	}

	return chain(ctx, req)
}

// setupDefaultMiddlewares sets up default middleware chain
func (f *fetcherImpl) setupDefaultMiddlewares() {
	// Add rate limiter middleware
	if f.rateLimiter != nil {
		f.WithMiddleware(NewRateLimiterMiddleware(f.rateLimiter))
	}

	// Add cache middleware
	if f.cache != nil && f.config.EnableCache {
		f.WithMiddleware(NewCacheMiddleware(f.cache))
	}

	// Add metrics middleware
	if f.config.EnableMetrics {
		f.WithMiddleware(NewMetricsMiddleware())
	}

	// Add retry middleware
	f.WithMiddleware(NewRetryMiddleware(f.config.MaxRetries))

	// Add logging middleware
	f.WithMiddleware(NewLoggingMiddleware(f.logger))
}

// updateStats updates fetcher statistics
func (f *fetcherImpl) updateStats(resp *Response, err error) {
	if !f.config.EnableMetrics {
		return
	}

	f.statsMu.Lock()
	defer f.statsMu.Unlock()

	f.stats.TotalRequests++
	f.stats.LastRequestTime = time.Now()

	if err == nil && resp != nil {
		f.stats.SuccessRequests++
		f.stats.BytesDownloaded += int64(len(resp.Body))

		// Update average response time
		if f.stats.AvgResponseTime == 0 {
			f.stats.AvgResponseTime = resp.Duration
		} else {
			f.stats.AvgResponseTime = (f.stats.AvgResponseTime + resp.Duration) / 2
		}
	} else {
		f.stats.FailedRequests++
	}
}

// Options

// WithLogger sets the logger
func WithLogger(logger *slog.Logger) Option {
	return func(f *fetcherImpl) {
		f.logger = logger
	}
}

// WithConfig sets the configuration
func WithConfig(config *Config) Option {
	return func(f *fetcherImpl) {
		f.config = config

		// Apply config to resty client
		f.client.SetTimeout(config.Timeout)
		f.client.SetRetryCount(config.MaxRetries)
		f.client.SetRetryWaitTime(config.RetryWaitTime)
		f.client.SetRetryMaxWaitTime(config.RetryMaxWaitTime)
		f.client.SetDebug(config.Debug)
	}
}

// WithProxyPool sets the proxy pool
func WithProxyPool(pool *ProxyPool) Option {
	return func(f *fetcherImpl) {
		f.proxyPool = pool
	}
}

// WithUserAgentPool sets the user agent pool
func WithUserAgentPool(pool *UserAgentPool) Option {
	return func(f *fetcherImpl) {
		f.userAgentPool = pool
	}
}

// WithRateLimiter sets the rate limiter
func WithRateLimiter(limiter RateLimiter) Option {
	return func(f *fetcherImpl) {
		f.rateLimiter = limiter
	}
}

// WithCache sets the cache
func WithCache(cache Cache) Option {
	return func(f *fetcherImpl) {
		f.cache = cache
	}
}

// WithTransport sets custom transport
func WithTransport(transport *http.Transport) Option {
	return func(f *fetcherImpl) {
		f.client.SetTransport(transport)
	}
}

// Convert to core.Response for compatibility
func (r *Response) ToCoreResponse() *core.Response {
	return &core.Response{
		StatusCode: r.StatusCode,
		Headers:    convertHeaders(r.Headers),
		Body:       r.Body,
		URL:        r.URL,
		Timestamp:  r.StartTime,
		Error:      r.Error,
	}
}

func convertHeaders(headers map[string]string) map[string][]string {
	result := make(map[string][]string)
	for k, v := range headers {
		result[k] = []string{v}
	}
	return result
}
