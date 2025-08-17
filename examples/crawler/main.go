package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler"
	"github.com/NHYCRaymond/go-backend-kit/crawler/fetcher"
	"github.com/NHYCRaymond/go-backend-kit/crawler/pipeline"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/go-redis/redis/v8"
)

func main() {
	// Initialize logging
	if err := logging.InitLogger(logging.DefaultLoggerConfig()); err != nil {
		log.Fatal("Failed to init logger:", err)
	}

	logger := logging.GetLogger()
	logger.Info("Starting crawler example")

	// Create crawler configuration
	config := &crawler.Config{
		ID:      "example-crawler",
		Name:    "Example Web Crawler",
		Version: "1.0.0",
		Mode:    crawler.ModeStandalone,

		// Worker configuration
		Workers:       5,
		MaxConcurrent: 10,
		MaxDepth:      3,
		MaxRetries:    3,
		Timeout:       30 * time.Second,

		// Redis configuration
		RedisAddr:   "localhost:6379",
		RedisDB:     0,
		RedisPrefix: "crawler",

		// Features
		EnableScheduler:   true,
		EnableDistributed: false,
		EnableMetrics:     true,

		// Logging
		LogLevel: "info",
	}

	// Create crawler instance
	crawler, err := crawler.New(config)
	if err != nil {
		log.Fatal("Failed to create crawler:", err)
	}

	// Start crawler
	ctx := context.Background()
	if err := crawler.Start(ctx); err != nil {
		log.Fatal("Failed to start crawler:", err)
	}

	// Add seed URLs
	seedURLs := []string{
		"https://example.com",
		"https://httpbin.org/html",
		"https://www.github.com",
	}

	for _, url := range seedURLs {
		if err := crawler.AddSeed(url, map[string]interface{}{
			"source": "manual",
			"type":   "homepage",
		}); err != nil {
			logger.Error("Failed to add seed", "url", url, "error", err)
		} else {
			logger.Info("Added seed URL", "url", url)
		}
	}

	// Wait for some processing
	time.Sleep(30 * time.Second)

	// Get metrics
	metrics := crawler.GetMetrics()
	logger.Info("Crawler metrics",
		"requests_total", metrics.RequestsTotal,
		"requests_success", metrics.RequestsSuccess,
		"items_extracted", metrics.ItemsExtracted,
		"items_stored", metrics.ItemsStored,
	)

	// Graceful shutdown
	logger.Info("Stopping crawler...")
	if err := crawler.Stop(ctx); err != nil {
		logger.Error("Failed to stop crawler", "error", err)
	}

	logger.Info("Crawler stopped successfully")
}

// Example of creating a custom task definition
func createCustomTaskDefinition() *task.TaskDefinition {
	return &task.TaskDefinition{
		ID:          "github-repo-crawler",
		Name:        "GitHub Repository Crawler",
		Version:     "1.0.0",
		Description: "Crawls GitHub repository information",
		Category:    "api",

		Type:     task.TypeAPI,
		Priority: task.PriorityNormal,

		Method:         "GET",
		DefaultTimeout: 10 * time.Second,
		MaxRetries:     3,
		RetryInterval:  2 * time.Second,

		// Extraction rules for GitHub API
		ExtractRules: []task.ExtractRule{
			{
				Field:    "name",
				Selector: "$.name",
				Type:     "json",
				Required: true,
			},
			{
				Field:    "description",
				Selector: "$.description",
				Type:     "json",
			},
			{
				Field:    "stars",
				Selector: "$.stargazers_count",
				Type:     "json",
			},
			{
				Field:    "forks",
				Selector: "$.forks_count",
				Type:     "json",
			},
			{
				Field:    "language",
				Selector: "$.language",
				Type:     "json",
			},
		},

		// Link rules for generating child tasks
		LinkRules: []task.LinkRule{
			{
				Name:     "contributors",
				Selector: "$.contributors_url",
				Type:     "json",
				TaskType: task.TypeAPI,
				Priority: task.PriorityNormal,
				Depth:    2,
			},
		},

		Tags: []string{"github", "api", "repository"},
		Metadata: map[string]interface{}{
			"api_version": "v3",
		},
	}
}

// Example of creating a pipeline with custom processors
func createCustomPipeline() *pipeline.Pipeline {
	logger := logging.GetLogger()

	// Create pipeline
	p := pipeline.NewPipeline(&pipeline.PipelineConfig{
		Name:   "github-data-pipeline",
		Logger: logger,
	})

	// Add processors
	p.AddProcessor(&GitHubDataProcessor{})
	p.AddProcessor(&MetricsProcessor{})

	return p
}

// GitHubDataProcessor processes GitHub repository data
type GitHubDataProcessor struct{}

func (p *GitHubDataProcessor) Name() string {
	return "github-processor"
}

func (p *GitHubDataProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	// Process GitHub-specific data
	if m, ok := data.(map[string]interface{}); ok {
		// Add processing timestamp
		m["processed_at"] = time.Now()

		// Calculate repository score
		stars := getIntValue(m, "stars")
		forks := getIntValue(m, "forks")
		m["score"] = stars*2 + forks

		// Categorize by popularity
		if stars > 1000 {
			m["category"] = "popular"
		} else if stars > 100 {
			m["category"] = "moderate"
		} else {
			m["category"] = "niche"
		}
	}

	return data, nil
}

func (p *GitHubDataProcessor) CanProcess(data interface{}) bool {
	_, ok := data.(map[string]interface{})
	return ok
}

// MetricsProcessor collects metrics from processed data
type MetricsProcessor struct {
	totalProcessed int64
}

func (p *MetricsProcessor) Name() string {
	return "metrics-processor"
}

func (p *MetricsProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	p.totalProcessed++

	// Log every 100 items
	if p.totalProcessed%100 == 0 {
		logger := logging.GetLogger()
		logger.Info("Metrics processor progress",
			"total_processed", p.totalProcessed)
	}

	return data, nil
}

func (p *MetricsProcessor) CanProcess(data interface{}) bool {
	return true
}

// Helper function to get int value from map
func getIntValue(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

// Example of advanced crawler configuration with distributed mode
func createDistributedCrawler() (*crawler.Crawler, error) {
	config := &crawler.Config{
		ID:      "distributed-crawler",
		Name:    "Distributed Web Crawler",
		Version: "1.0.0",
		Mode:    crawler.ModeDistributed,

		// Fetcher configuration
		Fetcher: &fetcher.Config{
			Timeout:       30 * time.Second,
			MaxRetries:    3,
			RetryWaitTime: 2 * time.Second,
			RateLimit:     10, // 10 requests per second

			// Proxy pool configuration
			ProxyPool: &fetcher.ProxyPoolConfig{
				Proxies: []fetcher.ProxyConfig{
					{URL: "http://proxy1.example.com:8080"},
					{URL: "http://proxy2.example.com:8080"},
				},
				Strategy:      fetcher.ProxyStrategyRoundRobin,
				CheckInterval: 5 * time.Minute,
			},

			// Cache configuration
			EnableCache:  true,
			CacheTTL:     1 * time.Hour,
			MaxCacheSize: 100 * 1024 * 1024, // 100MB
		},

		// Pipeline configuration
		Pipeline: &pipeline.Config{
			Concurrency:        10,
			EnableCleaner:      true,
			EnableValidator:    true,
			EnableDeduplicator: true,
			DedupeKey:          "url",
		},

		// Task queue configuration
		TaskQueue: &task.QueueConfig{
			Type:     task.QueueTypeRedis,
			Capacity: 10000,
		},

		// Dispatcher configuration
		Dispatcher: &task.DispatcherConfig{
			Strategy:   task.StrategyLeastLoad,
			BatchSize:  100,
			MaxRetries: 3,
			Timeout:    5 * time.Minute,
		},

		// Node configuration for distributed mode
		Node: &distributed.NodeConfig{
			ID:         "node-001",
			MaxWorkers: 20,
			QueueSize:  1000,
		},

		// General settings
		Workers:           10,
		MaxConcurrent:     50,
		MaxDepth:          5,
		MaxRetries:        3,
		EnableScheduler:   true,
		EnableDistributed: true,
		EnableMetrics:     true,
		EnableProfiling:   true,

		RedisAddr:   "localhost:6379",
		RedisDB:     0,
		RedisPrefix: "distributed-crawler",
		LogLevel:    "debug",
	}

	return crawler.New(config)
}
