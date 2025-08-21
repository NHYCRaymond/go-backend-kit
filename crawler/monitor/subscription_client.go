package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/types"
	"github.com/patrickmn/go-cache"
)

// SubscriptionClient communicates with external subscription service
type SubscriptionClient struct {
	baseURL    string
	httpClient *http.Client
	cache      *cache.Cache
}

// NewSubscriptionClient creates a new subscription client
func NewSubscriptionClient(baseURL string, timeoutSeconds int) *SubscriptionClient {
	return &SubscriptionClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
		cache: cache.New(60*time.Second, 2*time.Minute), // 1 minute cache with 2 minute cleanup
	}
}

// GetSubscribers queries subscribers for a specific source and time slot
func (sc *SubscriptionClient) GetSubscribers(ctx context.Context, source, timeSlot string) []types.Subscriber {
	// Skip if no base URL configured (for testing)
	if sc.baseURL == "" {
		return sc.getMockSubscribers(source, timeSlot)
	}

	// Check cache first
	cacheKey := fmt.Sprintf("%s:%s", source, timeSlot)
	if cached, found := sc.cache.Get(cacheKey); found {
		return cached.([]types.Subscriber)
	}

	// Query external service
	subscribers, err := sc.querySubscribers(ctx, source, timeSlot)
	if err != nil {
		// Log error and return empty list
		fmt.Printf("Failed to query subscribers: %v\n", err)
		return []types.Subscriber{}
	}

	// Cache the result
	sc.cache.Set(cacheKey, subscribers, cache.DefaultExpiration)

	return subscribers
}

// querySubscribers makes HTTP request to subscription service
func (sc *SubscriptionClient) querySubscribers(ctx context.Context, source, timeSlot string) ([]types.Subscriber, error) {
	url := fmt.Sprintf("%s/api/subscriptions/query", sc.baseURL)
	
	query := types.SubscriptionQuery{
		Source:   source,
		TimeSlot: timeSlot,
	}

	jsonData, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscription service returned status %d", resp.StatusCode)
	}

	var response types.SubscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("subscription service returned failure")
	}

	return response.Subscribers, nil
}

// getMockSubscribers returns mock subscribers for testing
func (sc *SubscriptionClient) getMockSubscribers(source, timeSlot string) []types.Subscriber {
	// Return empty list in production
	// This can be configured to return test data during development
	if source == "test" {
		return []types.Subscriber{
			{
				UserID:     "test_user_1",
				WebhookURL: "https://oapi.dingtalk.com/robot/send?access_token=test",
				UserName:   "Test User",
			},
		}
	}
	return []types.Subscriber{}
}

// InvalidateCache invalidates the cache for a specific source and time slot
func (sc *SubscriptionClient) InvalidateCache(source, timeSlot string) {
	cacheKey := fmt.Sprintf("%s:%s", source, timeSlot)
	sc.cache.Delete(cacheKey)
}

// InvalidateAllCache clears all cached data
func (sc *SubscriptionClient) InvalidateAllCache() {
	sc.cache.Flush()
}

// SetBaseURL updates the base URL (useful for testing)
func (sc *SubscriptionClient) SetBaseURL(url string) {
	sc.baseURL = url
}

// HealthCheck checks if the subscription service is healthy
func (sc *SubscriptionClient) HealthCheck(ctx context.Context) error {
	if sc.baseURL == "" {
		return nil // Skip health check if no URL configured
	}

	url := fmt.Sprintf("%s/health", sc.baseURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}