package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-resty/resty/v2"
)

// HTTPFetcher implements Fetcher for HTTP requests
type HTTPFetcher struct {
	client *resty.Client
}

// NewHTTPFetcher creates a new HTTP fetcher
func NewHTTPFetcher() *HTTPFetcher {
	client := resty.New().
		SetTimeout(30 * time.Second).
		SetRedirectPolicy(resty.FlexibleRedirectPolicy(10)).
		SetRetryCount(3).
		SetRetryWaitTime(5 * time.Second).
		SetRetryMaxWaitTime(20 * time.Second)

	return &HTTPFetcher{
		client: client,
	}
}

// Fetch fetches content from URL
func (f *HTTPFetcher) Fetch(ctx context.Context, t *task.Task) (*task.Response, error) {

	// Create request with context
	req := f.client.R().
		SetContext(ctx)

	// Set headers
	if len(t.Headers) > 0 {
		req.SetHeaders(t.Headers)
	}

	// Set default User-Agent if not present
	if _, exists := t.Headers["User-Agent"]; !exists {
		req.SetHeader("User-Agent", "Mozilla/5.0 (compatible; Crawler/1.0)")
	}

	// Set cookies
	for name, value := range t.Cookies {
		req.SetCookie(&http.Cookie{
			Name:  name,
			Value: value,
		})
	}

	// Set body for POST requests
	if t.Method == "POST" && len(t.Body) > 0 {
		// Try to parse body as JSON and set content type
		req.SetHeader("Content-Type", "application/json")
		req.SetBody(t.Body)

	}

	// Note: Request-level timeout override is not supported in resty v2
	// Timeout is set at client level during initialization

	// Execute request
	startTime := time.Now()
	var resp *resty.Response
	var err error

	switch t.Method {
	case "GET":
		resp, err = req.Get(t.URL)
	case "POST":
		resp, err = req.Post(t.URL)
	case "PUT":
		resp, err = req.Put(t.URL)
	case "DELETE":
		resp, err = req.Delete(t.URL)
	case "HEAD":
		resp, err = req.Head(t.URL)
	case "PATCH":
		resp, err = req.Patch(t.URL)
	default:
		resp, err = req.Get(t.URL)
	}

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	
	// Build response
	body := resp.Body()
	
	// For API requests, check if response is valid JSON
	if t.Type == "api" && resp.StatusCode() != 200 {
		// Include status code in error for debugging
		bodyPreview := string(body)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		// This will be caught by the extractor
		// We just add more context to the response
	}

	response := &task.Response{
		StatusCode: resp.StatusCode(),
		Headers:    resp.Header(),
		Body:       body,
		URL:        resp.Request.URL,
		HTML:       string(body),
		Duration:   time.Since(startTime).Milliseconds(),
		Size:       int64(len(body)),
	}

	return response, nil
}

// GetType returns the fetcher type
func (f *HTTPFetcher) GetType() string {
	return "http"
}
