package fetcher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-resty/resty/v2"
)

// HTTPFetcher implements Fetcher for HTTP requests
type HTTPFetcher struct {
	client *resty.Client
	logger *slog.Logger
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
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
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
		f.logger.Debug("Request headers set",
			"task_id", t.ID,
			"headers", t.Headers)
	}

	// Set default User-Agent if not present
	if _, exists := t.Headers["User-Agent"]; !exists {
		req.SetHeader("User-Agent", "Mozilla/5.0 (compatible; Crawler/1.0)")
	}

	// Set cookies
	if len(t.Cookies) > 0 {
		for name, value := range t.Cookies {
			req.SetCookie(&http.Cookie{
				Name:  name,
				Value: value,
			})
		}
		f.logger.Debug("Request cookies set",
			"task_id", t.ID,
			"cookies_count", len(t.Cookies))
	}

	// Set body for POST requests
	if t.Method == "POST" && len(t.Body) > 0 {
		// Check if Content-Type is already set in headers
		contentType := ""
		for key, value := range t.Headers {
			if strings.ToLower(key) == "content-type" {
				contentType = value
				break
			}
		}
		
		// Only set default Content-Type if not already specified
		if contentType == "" {
			req.SetHeader("Content-Type", "application/json")
			f.logger.Debug("Setting default Content-Type to application/json",
				"task_id", t.ID)
		} else {
			// Content-Type is already set in headers, it will be applied above
			f.logger.Debug("Using specified Content-Type",
				"task_id", t.ID,
				"content_type", contentType)
		}
		
		req.SetBody(t.Body)
		f.logger.Info("POST request body set",
			"task_id", t.ID,
			"body", string(t.Body),
			"body_size", len(t.Body),
			"content_type", contentType)
	} else if t.Method == "POST" {
		f.logger.Warn("POST request without body",
			"task_id", t.ID,
			"url", t.URL)
	}

	// Note: Request-level timeout override is not supported in resty v2
	// Timeout is set at client level during initialization

	// Log request details
	methodToUse := t.Method
	if methodToUse == "" {
		methodToUse = "GET"
	}
	f.logger.Info("Fetching URL",
		"url", t.URL,
		"method", methodToUse,
		"task_id", t.ID,
		"task_type", t.Type,
		"headers_count", len(t.Headers),
		"has_body", len(t.Body) > 0,
		"body_preview", string(t.Body))

	// Execute request
	startTime := time.Now()
	var resp *resty.Response
	var err error

	// Ensure methodToUse is set (already set above, but check again)
	if methodToUse == "" {
		methodToUse = "GET"
		f.logger.Debug("No method specified, defaulting to GET",
			"task_id", t.ID)
	}

	switch methodToUse {
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
		f.logger.Warn("Unknown method, defaulting to GET",
			"task_id", t.ID,
			"method", methodToUse)
		resp, err = req.Get(t.URL)
	}

	if err != nil {
		f.logger.Error("Request failed",
			"url", t.URL,
			"method", t.Method,
			"error", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	
	// Build response
	body := resp.Body()
	elapsedTime := time.Since(startTime)
	
	// Log response details
	f.logger.Info("Response received",
		"url", t.URL,
		"status_code", resp.StatusCode(),
		"body_size", len(body),
		"duration_ms", elapsedTime.Milliseconds(),
		"content_type", resp.Header().Get("Content-Type"),
		"task_id", t.ID)
	
	// For API requests, check if response is valid JSON
	if t.Type == "api" && resp.StatusCode() != 200 {
		// Include status code in error for debugging
		bodyPreview := string(body)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		f.logger.Warn("API request returned non-200 status",
			"url", t.URL,
			"status_code", resp.StatusCode(),
			"body_preview", bodyPreview)
		// This will be caught by the extractor
		// We just add more context to the response
	}
	
	// Log body content for debugging
	if len(body) > 0 {
		bodyPreview := string(body)
		// For API responses, show more content
		if t.Type == "api" {
			if len(bodyPreview) > 2000 {
				bodyPreview = bodyPreview[:2000] + "..."
			}
			f.logger.Info("API Response body",
				"task_id", t.ID,
				"url", t.URL,
				"body", bodyPreview)
		} else {
			if len(bodyPreview) > 500 {
				bodyPreview = bodyPreview[:500] + "..."
			}
			f.logger.Debug("Response body preview",
				"url", t.URL,
				"preview", bodyPreview)
		}
	}

	response := &task.Response{
		StatusCode: resp.StatusCode(),
		Headers:    resp.Header(),
		Body:       body,
		URL:        resp.Request.URL,
		HTML:       string(body),
		Duration:   elapsedTime.Milliseconds(),
		Size:       int64(len(body)),
	}

	f.logger.Info("Response built",
		"url", t.URL,
		"response_size", response.Size,
		"duration_ms", response.Duration,
		"task_id", t.ID)

	return response, nil
}

// GetType returns the fetcher type
func (f *HTTPFetcher) GetType() string {
	return "http"
}
