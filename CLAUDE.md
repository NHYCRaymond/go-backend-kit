# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go Backend Kit is a comprehensive Go backend infrastructure library providing reusable components for building scalable web applications. It abstracts common backend concerns into well-structured, configurable modules.

## Development Commands

```bash
# Install dependencies
go mod tidy

# Build the project
go build ./...

# Run tests
go test ./...

# Run with coverage
go test -cover ./...

# Run example
cd examples/basic && go run main.go

# Clean module cache
go clean -modcache
```

## Architecture

The project follows clean architecture principles with these key layers:

- **config/**: YAML-based configuration with environment variable overrides
- **database/**: Database abstractions (MySQL/GORM, MongoDB, Redis)
- **middleware/**: HTTP middleware (auth, rate limiting, logging, security)
- **messagequeue/**: Message queue implementations (RabbitMQ)
- **monitoring/**: Prometheus metrics and observability
- **response/**: Standardized API response formatting
- **server/**: HTTP server with graceful shutdown
- **utils/**: Common utilities (pagination, validation)

## Key Patterns

**Middleware Pipeline**: Recovery → RequestID → Security → RateLimit → Logger → Auth

**Configuration Management**: YAML-first with environment variable overrides. Primary config file is `config.yaml`.

**Dependency Injection**: Configuration-driven initialization with interface-based abstractions.

**Standardized Responses**: All API responses follow consistent format with code, message, and data fields.

**Error Handling**: Custom error types with HTTP status codes and application-specific error constants.

## External Dependencies

Required services for full functionality:
- MySQL Server (database operations)
- Redis Server (caching and rate limiting)
- MongoDB (optional, document storage)
- RabbitMQ (optional, message queuing)

## Environment Variables

Configuration supports environment variable overrides:
- `SERVER_HOST`, `SERVER_PORT`, `SERVER_ENV`
- `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DBNAME`
- `REDIS_ADDRESS`, `REDIS_PASSWORD`, `REDIS_DB`
- `MONGO_URI`, `MONGO_DATABASE`
- `RABBITMQ_HOST`, `RABBITMQ_PORT`, `RABBITMQ_USER`, `RABBITMQ_PASSWORD`
- `JWT_SECRET_KEY`

## Usage Example

```go
// Load configuration
cfg, err := config.Load("config.yaml")

// Initialize dependencies
db, err := database.NewMySQL(cfg.Database.MySQL)
redis, err := database.NewRedis(cfg.Redis)

// Setup middleware
router := gin.New()
middleware.Setup(router, &cfg.Middleware, redis, logger, jwtService)

// Start server
srv := server.New(cfg.Server, router, logger)
srv.StartAndWait()
```

## Key Technologies

- **Web Framework**: Gin (v1.10.1)
- **Database**: GORM (v1.30.0), MongoDB driver (v1.17.4)
- **Caching**: Redis (v8.11.5)
- **Authentication**: JWT (v5.2.2)
- **Message Queue**: RabbitMQ (v1.10.0)
- **Monitoring**: Prometheus client (v1.22.0)
- **Configuration**: Viper (v1.20.1)

Requires Go 1.21+