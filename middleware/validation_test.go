package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupValidationTest() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	return r
}

func TestInputValidation_Clean(t *testing.T) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	tests := []struct {
		name   string
		body   string
		status int
	}{
		{
			name:   "Clean JSON",
			body:   `{"name": "John Doe", "email": "john@example.com"}`,
			status: http.StatusOK,
		},
		{
			name:   "Clean form data",
			body:   "name=John+Doe&email=john@example.com",
			status: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/test", strings.NewReader(tt.body))
			if strings.Contains(tt.body, "{") {
				req.Header.Set("Content-Type", "application/json")
			} else {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.status, w.Code)
		})
	}
}

func TestInputValidation_XSS(t *testing.T) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	xssPayloads := []string{
		`<script>alert('XSS')</script>`,
		`<img src=x onerror=alert('XSS')>`,
		`<iframe src="javascript:alert('XSS')"></iframe>`,
		`<object data="javascript:alert('XSS')"></object>`,
		`<embed src="javascript:alert('XSS')">`,
		`<link rel="stylesheet" href="javascript:alert('XSS')">`,
		`<meta http-equiv="refresh" content="0;javascript:alert('XSS')">`,
	}

	for _, payload := range xssPayloads {
		name := payload
		if len(name) > 20 {
			name = name[:20]
		}
		t.Run(name, func(t *testing.T) {
			body := `{"input": "` + payload + `"}`
			req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestInputValidation_SQLInjection(t *testing.T) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.GET("/search", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	sqlPayloads := []string{
		"1' OR '1'='1",
		"1; DROP TABLE users--",
		"1 UNION SELECT * FROM passwords",
		"1' AND 1=1--",
		"admin'--",
	}

	for _, payload := range sqlPayloads {
		name := payload
		if len(name) > 10 {
			name = name[:10]
		}
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/search?q="+url.QueryEscape(payload), nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestInputValidation_PathTraversal(t *testing.T) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.GET("/file/*path", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	pathPayloads := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32",
		"....//....//....//etc/passwd",
		"../../etc/passwd", // This will be URL-encoded to %2e%2e%2f...
	}

	for _, payload := range pathPayloads {
		name := payload
		if len(name) > 10 {
			name = name[:10]
		}
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/file/"+url.PathEscape(payload), nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestInputValidation_QueryParams(t *testing.T) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.GET("/search", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	tests := []struct {
		name   string
		query  string
		status int
	}{
		{
			name:   "Clean query",
			query:  "?q=golang&page=1",
			status: http.StatusOK,
		},
		{
			name:   "XSS in query",
			query:  "?q=<script>alert('xss')</script>",
			status: http.StatusBadRequest,
		},
		{
			name:   "SQL injection in query",
			query:  "?id=" + url.QueryEscape("1' OR '1'='1"),
			status: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/search"+tt.query, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.status, w.Code)
		})
	}
}

func TestInputValidation_Headers(t *testing.T) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	tests := []struct {
		name   string
		header string
		value  string
		status int
	}{
		{
			name:   "Clean header",
			header: "User-Agent",
			value:  "Mozilla/5.0",
			status: http.StatusOK,
		},
		{
			name:   "XSS in header",
			header: "X-Custom",
			value:  "<script>alert('xss')</script>",
			status: http.StatusBadRequest,
		},
		{
			name:   "Command injection in header",
			header: "X-Forwarded-For",
			value:  "127.0.0.1; rm -rf /",
			status: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set(tt.header, tt.value)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.status, w.Code)
		})
	}
}

func TestInputValidation_SkipPaths(t *testing.T) {
	config := &ValidationConfig{
		SkipPaths: []string{"/health", "/metrics"},
	}

	r := setupValidationTest()
	r.Use(InputValidation(config))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Should skip validation for /health
	req := httptest.NewRequest("GET", "/health?q=<script>alert('xss')</script>", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Should validate /test
	req = httptest.NewRequest("GET", "/test?q=<script>alert('xss')</script>", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRequestSizeLimit(t *testing.T) {
	r := setupValidationTest()
	r.Use(RequestSizeLimit(1024)) // 1KB limit
	r.POST("/upload", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Small request - should pass
	smallBody := strings.Repeat("a", 512)
	req := httptest.NewRequest("POST", "/upload", strings.NewReader(smallBody))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Large request - should fail
	largeBody := strings.Repeat("a", 2048)
	req = httptest.NewRequest("POST", "/upload", strings.NewReader(largeBody))
	req.ContentLength = int64(len(largeBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestInputValidation_FileUpload(t *testing.T) {
	config := &ValidationConfig{
		MaxRequestSize: 10 * 1024 * 1024, // 10MB
		MaxFileSize:    1024 * 1024,      // 1MB
		AllowedFileTypes: []string{
			"image/jpeg",
			"image/png",
		},
	}

	r := setupValidationTest()
	r.Use(InputValidation(config))
	r.POST("/upload", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Create multipart form
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Add a valid file field
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		`form-data; name="file"; filename="test.jpg"`)
	h.Set("Content-Type", "image/jpeg")
	fw, err := w.CreatePart(h)
	assert.NoError(t, err)
	fw.Write([]byte("fake image data"))

	// Add a form field with clean content
	w.WriteField("description", "A test image")
	w.Close()

	req := httptest.NewRequest("POST", "/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestInputValidation_FileUpload_MaliciousFilename(t *testing.T) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.POST("/upload", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Create multipart form with malicious filename
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Add file with XSS in filename
	fw, err := w.CreateFormFile("file", "<script>alert('xss')</script>.jpg")
	assert.NoError(t, err)
	fw.Write([]byte("fake image data"))
	w.Close()

	req := httptest.NewRequest("POST", "/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestSanitizeOutput(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    `<script>alert('xss')</script>`,
			expected: `&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;`,
		},
		{
			input:    `Hello & "World"`,
			expected: `Hello &amp; &quot;World&quot;`,
		},
		{
			input:    `Normal text`,
			expected: `Normal text`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeOutput(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInputValidation_NullBytes(t *testing.T) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	body := "data=test\x00value"
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestInputValidation_ExcessiveLength(t *testing.T) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Create a string longer than 10000 characters
	longString := strings.Repeat("a", 10001)
	body := `{"data": "` + longString + `"}`

	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func BenchmarkInputValidation(b *testing.B) {
	r := setupValidationTest()
	r.Use(InputValidation(nil))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	body := `{"name": "John Doe", "email": "john@example.com", "message": "This is a test message"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func ExampleInputValidation() {
	r := gin.New()

	// Use default configuration
	r.Use(InputValidation(nil))

	// Or use custom configuration
	config := &ValidationConfig{
		MaxRequestSize: 5 * 1024 * 1024, // 5MB
		SkipPaths:      []string{"/health", "/metrics"},
		AllowedFileTypes: []string{
			"image/jpeg",
			"image/png",
			"application/pdf",
		},
		MaxFileSize: 2 * 1024 * 1024, // 2MB
	}
	r.Use(InputValidation(config))

	// Add request size limit for specific routes
	r.POST("/upload", RequestSizeLimit(10*1024*1024), func(c *gin.Context) {
		// Handle file upload
	})
}
