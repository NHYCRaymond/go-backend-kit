package fetcher

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Proxy represents a proxy server
type Proxy struct {
	URL      string    `json:"url"`
	Type     string    `json:"type"` // http, https, socks5
	Username string    `json:"username,omitempty"`
	Password string    `json:"password,omitempty"`
	
	// Statistics
	SuccessCount int64     `json:"success_count"`
	FailureCount int64     `json:"failure_count"`
	LastUsed     time.Time `json:"last_used"`
	LastChecked  time.Time `json:"last_checked"`
	
	// Status
	Alive    bool      `json:"alive"`
	Latency  time.Duration `json:"latency"`
	
	// Metadata
	Country  string                 `json:"country,omitempty"`
	City     string                 `json:"city,omitempty"`
	Provider string                 `json:"provider,omitempty"`
	Tags     []string               `json:"tags,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ProxyPool manages a pool of proxy servers
type ProxyPool struct {
	mu        sync.RWMutex
	proxies   []*Proxy
	strategy  ProxyStrategy
	
	// Rotation
	current   int32
	
	// Health check
	checker   ProxyChecker
	checkInterval time.Duration
	stopChan  chan struct{}
}

// ProxyStrategy defines proxy selection strategy
type ProxyStrategy string

const (
	ProxyStrategyRoundRobin ProxyStrategy = "round_robin"
	ProxyStrategyRandom     ProxyStrategy = "random"
	ProxyStrategyLeastUsed  ProxyStrategy = "least_used"
	ProxyStrategyBestLatency ProxyStrategy = "best_latency"
	ProxyStrategyWeighted   ProxyStrategy = "weighted"
)

// ProxyChecker checks proxy health
type ProxyChecker interface {
	Check(proxy *Proxy) error
}

// ProxyPoolConfig contains proxy pool configuration
type ProxyPoolConfig struct {
	Proxies       []*Proxy
	Strategy      ProxyStrategy
	CheckInterval time.Duration
	Checker       ProxyChecker
}

// NewProxyPool creates a new proxy pool
func NewProxyPool(config *ProxyPoolConfig) *ProxyPool {
	if config.Strategy == "" {
		config.Strategy = ProxyStrategyRoundRobin
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 5 * time.Minute
	}
	if config.Checker == nil {
		config.Checker = &DefaultProxyChecker{}
	}
	
	pool := &ProxyPool{
		proxies:       config.Proxies,
		strategy:      config.Strategy,
		checker:       config.Checker,
		checkInterval: config.CheckInterval,
		stopChan:      make(chan struct{}),
	}
	
	// Start health checker
	go pool.healthCheckLoop()
	
	return pool
}

// Add adds a proxy to the pool
func (p *ProxyPool) Add(proxy *Proxy) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.proxies = append(p.proxies, proxy)
}

// Remove removes a proxy from the pool
func (p *ProxyPool) Remove(proxyURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	for i, proxy := range p.proxies {
		if proxy.URL == proxyURL {
			p.proxies = append(p.proxies[:i], p.proxies[i+1:]...)
			break
		}
	}
}

// Get returns a proxy based on strategy
func (p *ProxyPool) Get() *Proxy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if len(p.proxies) == 0 {
		return nil
	}
	
	// Filter alive proxies
	aliveProxies := p.getAliveProxies()
	if len(aliveProxies) == 0 {
		return nil
	}
	
	var selected *Proxy
	
	switch p.strategy {
	case ProxyStrategyRandom:
		selected = p.selectRandom(aliveProxies)
	case ProxyStrategyLeastUsed:
		selected = p.selectLeastUsed(aliveProxies)
	case ProxyStrategyBestLatency:
		selected = p.selectBestLatency(aliveProxies)
	case ProxyStrategyWeighted:
		selected = p.selectWeighted(aliveProxies)
	default: // ProxyStrategyRoundRobin
		selected = p.selectRoundRobin(aliveProxies)
	}
	
	if selected != nil {
		selected.LastUsed = time.Now()
	}
	
	return selected
}

// GetByTag returns proxies with specific tag
func (p *ProxyPool) GetByTag(tag string) []*Proxy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	var matched []*Proxy
	for _, proxy := range p.proxies {
		for _, t := range proxy.Tags {
			if t == tag {
				matched = append(matched, proxy)
				break
			}
		}
	}
	
	return matched
}

// GetByCountry returns proxies from specific country
func (p *ProxyPool) GetByCountry(country string) []*Proxy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	var matched []*Proxy
	for _, proxy := range p.proxies {
		if proxy.Country == country {
			matched = append(matched, proxy)
		}
	}
	
	return matched
}

// MarkSuccess marks a proxy as successful
func (p *ProxyPool) MarkSuccess(proxyURL string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	for _, proxy := range p.proxies {
		if proxy.URL == proxyURL {
			atomic.AddInt64(&proxy.SuccessCount, 1)
			break
		}
	}
}

// MarkFailure marks a proxy as failed
func (p *ProxyPool) MarkFailure(proxyURL string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	for _, proxy := range p.proxies {
		if proxy.URL == proxyURL {
			atomic.AddInt64(&proxy.FailureCount, 1)
			
			// Disable proxy if too many failures
			if proxy.FailureCount > 10 && proxy.SuccessCount < proxy.FailureCount/2 {
				proxy.Alive = false
			}
			break
		}
	}
}

// Size returns the pool size
func (p *ProxyPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.proxies)
}

// AliveCount returns number of alive proxies
func (p *ProxyPool) AliveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	count := 0
	for _, proxy := range p.proxies {
		if proxy.Alive {
			count++
		}
	}
	return count
}

// Stats returns pool statistics
func (p *ProxyPool) Stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	total := len(p.proxies)
	alive := 0
	totalSuccess := int64(0)
	totalFailure := int64(0)
	
	for _, proxy := range p.proxies {
		if proxy.Alive {
			alive++
		}
		totalSuccess += proxy.SuccessCount
		totalFailure += proxy.FailureCount
	}
	
	return map[string]interface{}{
		"total":         total,
		"alive":         alive,
		"dead":          total - alive,
		"total_success": totalSuccess,
		"total_failure": totalFailure,
		"success_rate":  float64(totalSuccess) / float64(totalSuccess+totalFailure) * 100,
	}
}

// Close stops the proxy pool
func (p *ProxyPool) Close() {
	close(p.stopChan)
}

// Selection strategies

func (p *ProxyPool) getAliveProxies() []*Proxy {
	var alive []*Proxy
	for _, proxy := range p.proxies {
		if proxy.Alive {
			alive = append(alive, proxy)
		}
	}
	return alive
}

func (p *ProxyPool) selectRoundRobin(proxies []*Proxy) *Proxy {
	if len(proxies) == 0 {
		return nil
	}
	
	index := atomic.AddInt32(&p.current, 1)
	return proxies[int(index)%len(proxies)]
}

func (p *ProxyPool) selectRandom(proxies []*Proxy) *Proxy {
	if len(proxies) == 0 {
		return nil
	}
	
	return proxies[rand.Intn(len(proxies))]
}

func (p *ProxyPool) selectLeastUsed(proxies []*Proxy) *Proxy {
	if len(proxies) == 0 {
		return nil
	}
	
	selected := proxies[0]
	minUsage := selected.SuccessCount + selected.FailureCount
	
	for _, proxy := range proxies[1:] {
		usage := proxy.SuccessCount + proxy.FailureCount
		if usage < minUsage {
			selected = proxy
			minUsage = usage
		}
	}
	
	return selected
}

func (p *ProxyPool) selectBestLatency(proxies []*Proxy) *Proxy {
	if len(proxies) == 0 {
		return nil
	}
	
	selected := proxies[0]
	minLatency := selected.Latency
	
	for _, proxy := range proxies[1:] {
		if proxy.Latency < minLatency && proxy.Latency > 0 {
			selected = proxy
			minLatency = proxy.Latency
		}
	}
	
	return selected
}

func (p *ProxyPool) selectWeighted(proxies []*Proxy) *Proxy {
	if len(proxies) == 0 {
		return nil
	}
	
	// Calculate weights based on success rate
	totalWeight := 0.0
	weights := make([]float64, len(proxies))
	
	for i, proxy := range proxies {
		total := proxy.SuccessCount + proxy.FailureCount
		if total > 0 {
			weights[i] = float64(proxy.SuccessCount) / float64(total)
		} else {
			weights[i] = 0.5 // Default weight for new proxies
		}
		totalWeight += weights[i]
	}
	
	// Select based on weight
	r := rand.Float64() * totalWeight
	cumulative := 0.0
	
	for i, weight := range weights {
		cumulative += weight
		if r <= cumulative {
			return proxies[i]
		}
	}
	
	return proxies[len(proxies)-1]
}

// Health check

func (p *ProxyPool) healthCheckLoop() {
	ticker := time.NewTicker(p.checkInterval)
	defer ticker.Stop()
	
	// Initial check
	p.checkAll()
	
	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.checkAll()
		}
	}
}

func (pp *ProxyPool) checkAll() {
	pp.mu.RLock()
	proxies := make([]*Proxy, len(pp.proxies))
	copy(proxies, pp.proxies)
	pp.mu.RUnlock()
	
	var wg sync.WaitGroup
	for _, proxy := range proxies {
		wg.Add(1)
		go func(p *Proxy) {
			defer wg.Done()
			
			startTime := time.Now()
			err := pp.checker.Check(p)
			latency := time.Since(startTime)
			
			p.LastChecked = time.Now()
			p.Latency = latency
			p.Alive = err == nil
		}(proxy)
	}
	
	wg.Wait()
}

// DefaultProxyChecker is the default proxy health checker
type DefaultProxyChecker struct{}

func (c *DefaultProxyChecker) Check(proxy *Proxy) error {
	// Parse proxy URL
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		return err
	}
	
	// Create HTTP client with proxy
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	
	if proxy.Username != "" && proxy.Password != "" {
		proxyURL.User = url.UserPassword(proxy.Username, proxy.Password)
	}
	
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
	
	// Test proxy with a simple request
	resp, err := client.Get("http://httpbin.org/ip")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return fmt.Errorf("proxy check failed with status: %d", resp.StatusCode)
	}
	
	return nil
}

// UserAgentPool manages a pool of user agents
type UserAgentPool struct {
	mu         sync.RWMutex
	userAgents []string
	weights    []int
	current    int32
	strategy   UserAgentStrategy
}

// UserAgentStrategy defines user agent selection strategy
type UserAgentStrategy string

const (
	UserAgentStrategyRoundRobin UserAgentStrategy = "round_robin"
	UserAgentStrategyRandom     UserAgentStrategy = "random"
	UserAgentStrategyWeighted   UserAgentStrategy = "weighted"
)

// UserAgentPoolConfig contains user agent pool configuration
type UserAgentPoolConfig struct {
	UserAgents []string
	Weights    []int
	Strategy   UserAgentStrategy
}

// NewUserAgentPool creates a new user agent pool
func NewUserAgentPool(config *UserAgentPoolConfig) *UserAgentPool {
	if config.Strategy == "" {
		config.Strategy = UserAgentStrategyRandom
	}
	
	// Use default user agents if none provided
	if len(config.UserAgents) == 0 {
		config.UserAgents = DefaultUserAgents()
	}
	
	return &UserAgentPool{
		userAgents: config.UserAgents,
		weights:    config.Weights,
		strategy:   config.Strategy,
	}
}

// Add adds a user agent to the pool
func (p *UserAgentPool) Add(userAgent string, weight int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.userAgents = append(p.userAgents, userAgent)
	if weight > 0 {
		p.weights = append(p.weights, weight)
	}
}

// Remove removes a user agent from the pool
func (p *UserAgentPool) Remove(userAgent string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	for i, ua := range p.userAgents {
		if ua == userAgent {
			p.userAgents = append(p.userAgents[:i], p.userAgents[i+1:]...)
			if len(p.weights) > i {
				p.weights = append(p.weights[:i], p.weights[i+1:]...)
			}
			break
		}
	}
}

// Get returns a user agent based on strategy
func (p *UserAgentPool) Get() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if len(p.userAgents) == 0 {
		return ""
	}
	
	switch p.strategy {
	case UserAgentStrategyRoundRobin:
		return p.getRoundRobin()
	case UserAgentStrategyWeighted:
		return p.getWeighted()
	default: // UserAgentStrategyRandom
		return p.getRandom()
	}
}

// GetDesktop returns a desktop user agent
func (p *UserAgentPool) GetDesktop() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	desktop := []string{}
	for _, ua := range p.userAgents {
		if !isMobileUserAgent(ua) {
			desktop = append(desktop, ua)
		}
	}
	
	if len(desktop) == 0 {
		return ""
	}
	
	return desktop[rand.Intn(len(desktop))]
}

// GetMobile returns a mobile user agent
func (p *UserAgentPool) GetMobile() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	mobile := []string{}
	for _, ua := range p.userAgents {
		if isMobileUserAgent(ua) {
			mobile = append(mobile, ua)
		}
	}
	
	if len(mobile) == 0 {
		return ""
	}
	
	return mobile[rand.Intn(len(mobile))]
}

// Size returns the pool size
func (p *UserAgentPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.userAgents)
}

func (p *UserAgentPool) getRoundRobin() string {
	index := atomic.AddInt32(&p.current, 1)
	return p.userAgents[int(index)%len(p.userAgents)]
}

func (p *UserAgentPool) getRandom() string {
	return p.userAgents[rand.Intn(len(p.userAgents))]
}

func (p *UserAgentPool) getWeighted() string {
	if len(p.weights) != len(p.userAgents) {
		return p.getRandom()
	}
	
	totalWeight := 0
	for _, w := range p.weights {
		totalWeight += w
	}
	
	r := rand.Intn(totalWeight)
	cumulative := 0
	
	for i, weight := range p.weights {
		cumulative += weight
		if r < cumulative {
			return p.userAgents[i]
		}
	}
	
	return p.userAgents[len(p.userAgents)-1]
}

func isMobileUserAgent(ua string) bool {
	// Simple mobile detection
	mobileKeywords := []string{"Mobile", "Android", "iPhone", "iPad", "Windows Phone"}
	for _, keyword := range mobileKeywords {
		if contains(ua, keyword) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && s[0:len(substr)] == substr) || contains(s[1:], substr))
}

// DefaultUserAgents returns default user agents
func DefaultUserAgents() []string {
	return []string{
		// Chrome - Windows
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
		
		// Chrome - Mac
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
		
		// Firefox - Windows
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:119.0) Gecko/20100101 Firefox/119.0",
		
		// Firefox - Mac
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15) Gecko/20100101 Firefox/120.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15) Gecko/20100101 Firefox/119.0",
		
		// Safari - Mac
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Safari/605.1.15",
		
		// Edge - Windows
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 Edg/119.0.0.0",
		
		// Mobile - Android Chrome
		"Mozilla/5.0 (Linux; Android 13; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
		
		// Mobile - iPhone Safari
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1",
		
		// Mobile - iPad Safari
		"Mozilla/5.0 (iPad; CPU OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (iPad; CPU OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1",
	}
}