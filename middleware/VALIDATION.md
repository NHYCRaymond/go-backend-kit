# Input Validation Middleware

A comprehensive security middleware for validating and sanitizing HTTP request inputs to protect against common web vulnerabilities.

## Features

- **XSS Protection**: Detects and blocks cross-site scripting attempts
- **SQL Injection Prevention**: Identifies SQL injection patterns
- **Path Traversal Protection**: Prevents directory traversal attacks
- **Command Injection Prevention**: Blocks shell command injection attempts
- **File Upload Validation**: Validates file types and sizes
- **Request Size Limiting**: Prevents DoS through large requests
- **Header Validation**: Sanitizes HTTP headers
- **Output Sanitization**: Helper function for safe HTML output

## Installation

```go
import "github.com/yourusername/go-backend-kit/middleware"
```

## Usage

### Basic Usage

```go
// Use default configuration
router.Use(middleware.InputValidation(nil))

// Or use custom configuration
config := &middleware.ValidationConfig{
    MaxRequestSize: 5 * 1024 * 1024, // 5MB
    MaxFileSize:    2 * 1024 * 1024, // 2MB
    SkipPaths:      []string{"/health", "/metrics"},
    AllowedFileTypes: []string{
        "image/jpeg",
        "image/png",
        "application/pdf",
    },
}
router.Use(middleware.InputValidation(config))
```

### Request Size Limiting

```go
// Apply size limit to specific routes
router.POST("/upload", middleware.RequestSizeLimit(10*1024*1024), uploadHandler)
```

### Output Sanitization

```go
// Sanitize user input before displaying
func handler(c *gin.Context) {
    userInput := c.Query("name")
    safeName := middleware.SanitizeOutput(userInput)
    c.JSON(200, gin.H{
        "message": "Hello, " + safeName,
    })
}
```

## Configuration Options

```go
type ValidationConfig struct {
    // Maximum allowed request body size (default: 10MB)
    MaxRequestSize int64
    
    // Paths that should bypass validation
    SkipPaths []string
    
    // Additional malicious patterns to check
    CustomPatterns []*regexp.Regexp
    
    // Allowed MIME types for file uploads
    AllowedFileTypes []string
    
    // Maximum file size for uploads (default: 5MB)
    MaxFileSize int64
}
```

## Protected Against

### 1. Cross-Site Scripting (XSS)
- Script tags: `<script>alert('XSS')</script>`
- Event handlers: `<img onerror=alert('XSS')>`
- JavaScript URLs: `javascript:alert('XSS')`
- Embedded content: `<iframe>`, `<object>`, `<embed>`

### 2. SQL Injection
- Union queries: `1 UNION SELECT * FROM users`
- Boolean conditions: `1' OR '1'='1`
- Comments: `--`, `/**/`
- Stacked queries: `1; DROP TABLE users`

### 3. Path Traversal
- Directory traversal: `../../../etc/passwd`
- Windows paths: `..\..\windows\system32`
- URL encoded: `%2e%2e%2f`
- Double encoding: `....//`

### 4. Command Injection
- Shell operators: `; | & $ ``
- Command substitution: `$(command)`, `${command}`
- Backticks: `` `command` ``

### 5. Other Attacks
- Null byte injection: `\x00`
- Buffer overflow attempts (excessive input length)
- Malicious file uploads
- Header injection

## Example: Secure File Upload

```go
func setupFileUpload(router *gin.Engine) {
    config := &middleware.ValidationConfig{
        MaxRequestSize: 50 * 1024 * 1024, // 50MB total
        MaxFileSize:    10 * 1024 * 1024, // 10MB per file
        AllowedFileTypes: []string{
            "image/jpeg",
            "image/png",
            "image/gif",
            "application/pdf",
            "application/zip",
        },
    }
    
    router.Use(middleware.InputValidation(config))
    
    router.POST("/upload", func(c *gin.Context) {
        file, header, err := c.Request.FormFile("file")
        if err != nil {
            c.JSON(400, gin.H{"error": "No file provided"})
            return
        }
        defer file.Close()
        
        // File has already been validated by middleware
        // Safe to process...
        
        c.JSON(200, gin.H{
            "filename": header.Filename,
            "size":     header.Size,
            "message":  "File uploaded successfully",
        })
    })
}
```

## Performance Considerations

- The middleware uses compiled regular expressions for efficiency
- Form data validation is less strict to avoid false positives
- Large request bodies are rejected early to prevent memory exhaustion
- Skip paths feature allows bypassing validation for specific endpoints

## Best Practices

1. **Always validate on the server**: Never rely solely on client-side validation
2. **Use skip paths sparingly**: Only exclude truly safe endpoints like health checks
3. **Combine with other security measures**: Use HTTPS, authentication, and rate limiting
4. **Keep patterns updated**: Review and update validation patterns regularly
5. **Log validation failures**: Monitor blocked requests to identify attack patterns
6. **Sanitize output**: Always sanitize data before displaying to users

## Customization

### Adding Custom Patterns

```go
config := &middleware.ValidationConfig{
    CustomPatterns: []*regexp.Regexp{
        regexp.MustCompile(`(?i)custom-danger-pattern`),
        regexp.MustCompile(`(?i)another-pattern`),
    },
}
```

### Excluding Specific Endpoints

```go
config := &middleware.ValidationConfig{
    SkipPaths: []string{
        "/health",
        "/metrics",
        "/api/v1/public/*",
    },
}
```

## Integration with Other Middleware

The validation middleware works well with other security middleware:

```go
router := gin.New()

// Order matters: validate inputs first
router.Use(middleware.InputValidation(nil))
router.Use(middleware.RateLimiter(rateLimitConfig))
router.Use(middleware.SecureHeaders())
router.Use(middleware.Auth(authConfig))
```

## Testing

The middleware includes comprehensive tests covering:
- XSS attack vectors
- SQL injection patterns
- Path traversal attempts
- File upload validation
- Header validation
- Edge cases and performance

Run tests with: `go test ./middleware -run TestInputValidation`