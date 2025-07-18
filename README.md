# Go Backend Kit

A comprehensive Go backend infrastructure library that provides reusable components for building scalable web applications with Gin, complete with database connections, caching, messaging, monitoring, and middleware.

## 🚀 Features

### Infrastructure Components
- **Database**: MySQL (GORM), MongoDB, Redis connection management
- **Message Queue**: RabbitMQ with connection pooling and auto-reconnection
- **Monitoring**: Prometheus metrics integration
- **Caching**: Redis-based caching with distributed locking
- **Logging**: Structured logging with rotation support

### Middleware System
- **Authentication**: JWT-based authentication and authorization
- **Rate Limiting**: Configurable rate limiting with Redis backend
- **Request Tracking**: Request ID generation and correlation
- **Error Handling**: Centralized error handling with custom error types
- **Security**: CORS, security headers, and request validation
- **Logging**: Structured request/response logging

### Utilities
- **Configuration**: YAML-based configuration with environment override
- **Response**: Standardized API response formatting
- **Pagination**: Database query pagination helpers
- **Validation**: Request validation utilities

## 📦 Installation

```bash
go get github.com/NHYCRaymond/go-backend-kit
```

## 🎯 Quick Start

### Basic Setup

```go
package main

import (
	"log"
	
	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/database"
	"github.com/NHYCRaymond/go-backend-kit/middleware"
	"github.com/NHYCRaymond/go-backend-kit/server"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize database
	db, err := database.NewMySQL(cfg.Database.MySQL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Initialize Redis
	redis, err := database.NewRedis(cfg.Redis)
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	// Create router
	router := gin.New()

	// Setup middleware
	middleware.Setup(router, &cfg.Middleware, redis)

	// Start server
	srv := server.New(router, cfg.Server)
	srv.Start()
}
```

### Configuration Example

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 8080
  env: "development"

database:
  mysql:
    host: "localhost"
    port: 3306
    user: "root"
    password: "password"
    dbname: "myapp"

redis:
  address: "localhost:6379"
  password: ""
  db: 0

middleware:
  auth:
    skip_paths: ["/health", "/metrics"]
    token_header: "Authorization"
    require_bearer: true
  
  rate_limit:
    enable: true
    window: "1m"
    limit: 100
    
  logger:
    enable_body: true
    max_body_size: 1024
```

## 📚 Documentation

### Database

#### MySQL with GORM
```go
import "github.com/NHYCRaymond/go-backend-kit/database"

db, err := database.NewMySQL(config.MySQLConfig{
    Host:     "localhost",
    Port:     3306,
    User:     "root",
    Password: "password",
    DBName:   "myapp",
})
```

#### MongoDB
```go
client, err := database.NewMongo(context.Background(), config.MongoConfig{
    URI:      "mongodb://localhost:27017",
    Database: "myapp",
})
```

#### Redis
```go
redis, err := database.NewRedis(config.RedisConfig{
    Address:  "localhost:6379",
    Password: "",
    DB:       0,
})
```

### Middleware

#### JWT Authentication
```go
import "github.com/NHYCRaymond/go-backend-kit/middleware"

// Setup JWT middleware
jwtMiddleware := middleware.NewJWT(middleware.JWTConfig{
    SecretKey: "your-secret-key",
    SkipPaths: []string{"/login", "/register"},
})

router.Use(jwtMiddleware.Handler())
```

#### Rate Limiting
```go
rateLimiter := middleware.NewRateLimit(redis, middleware.RateLimitConfig{
    Window: "1m",
    Limit:  100,
})

router.Use(rateLimiter.Handler())
```

#### Request Logging
```go
logger := middleware.NewLogger(middleware.LoggerConfig{
    EnableBody:  true,
    MaxBodySize: 1024,
})

router.Use(logger.Handler())
```

### Monitoring

#### Prometheus Metrics
```go
import "github.com/NHYCRaymond/go-backend-kit/monitoring"

// Start metrics server
monitoring.StartServer(&monitoring.Config{
    Port: 9090,
    Path: "/metrics",
})

// Custom metrics
monitoring.HTTPRequestsTotal.WithLabelValues("GET", "/api/users", "200").Inc()
monitoring.HTTPRequestDuration.WithLabelValues("GET", "/api/users").Observe(0.5)
```

### Message Queue

#### RabbitMQ
```go
import "github.com/NHYCRaymond/go-backend-kit/messagequeue"

// Initialize RabbitMQ
mq, err := messagequeue.NewRabbitMQ(messagequeue.RabbitMQConfig{
    Host:     "localhost",
    Port:     5672,
    Username: "guest",
    Password: "guest",
    Vhost:    "/",
})

// Publish message
err = mq.Publish("exchange", "routing.key", []byte("message"))

// Consume messages
messages, err := mq.Consume("queue-name")
for msg := range messages {
    // Process message
    log.Printf("Received: %s", msg.Body)
    msg.Ack(false)
}
```

### Response Formatting

```go
import "github.com/NHYCRaymond/go-backend-kit/response"

// Success response
response.Success(c, data)

// Error response
response.Error(c, response.ErrInvalidRequest, "Invalid input")

// Paginated response
response.Paginated(c, data, pagination)
```

### Error Handling

```go
import "github.com/NHYCRaymond/go-backend-kit/errors"

// Define custom errors
var (
    ErrUserNotFound = errors.New("USER_NOT_FOUND", "User not found")
    ErrInvalidToken = errors.New("INVALID_TOKEN", "Invalid access token")
)

// In handlers
if user == nil {
    return errors.ErrUserNotFound
}
```

## 🏗️ Architecture

The library follows clean architecture principles:

```
github.com/NHYCRaymond/go-backend-kit/
├── config/          # Configuration management
├── database/        # Database connections (MySQL, MongoDB, Redis)
├── middleware/      # HTTP middleware components
├── messagequeue/    # Message queue implementations
├── monitoring/      # Metrics and monitoring
├── response/        # API response utilities
├── errors/          # Error definitions and handling
├── server/          # HTTP server utilities
├── utils/           # Common utilities
└── examples/        # Usage examples
```

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Built with [Gin](https://github.com/gin-gonic/gin) HTTP framework
- Database integration with [GORM](https://gorm.io/)
- Monitoring with [Prometheus](https://prometheus.io/)
- Message queue with [RabbitMQ](https://www.rabbitmq.com/)