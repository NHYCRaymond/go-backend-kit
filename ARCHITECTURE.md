# Go Backend Kit Architecture

## Overview

Go Backend Kit is a comprehensive backend infrastructure library that provides reusable components for building scalable Go web applications. It follows clean architecture principles and emphasizes modularity, testability, and performance.

## Core Principles

1. **Interface-Driven Design**: All major components are defined by interfaces, allowing for easy testing and swapping of implementations.
2. **Configuration-First**: All components are configured through a centralized configuration system with environment variable overrides.
3. **Observability Built-In**: Metrics, logging, and tracing are first-class citizens in the architecture.
4. **Security by Default**: Security middleware and validation are included and enabled by default.

## Project Structure

```
go-backend-kit/
├── config/              # Configuration management
├── database/            # Database abstractions and implementations
│   ├── base.go         # Base database structure for code reuse
│   ├── interface.go    # Database interface definition
│   ├── factory.go      # Database factory for creating instances
│   └── drivers/        # Specific database implementations
├── middleware/          # HTTP middleware components
├── monitoring/          # Prometheus metrics and Grafana dashboards
├── validation/          # Shared validation logic
├── decimal/            # Precise decimal/money handling
├── lock/               # Distributed locking
├── response/           # Standardized API responses
├── server/             # HTTP server with graceful shutdown
├── utils/              # Common utilities
└── examples/           # Example applications
```

## Component Architecture

### 1. Database Layer (`/database`)

The database layer provides a unified interface for different database systems:

```go
type Database interface {
    Connect(ctx context.Context) error
    Disconnect(ctx context.Context) error
    HealthCheck(ctx context.Context) error
    GetClient() interface{}
    GetConfig() interface{}
    Stats() DatabaseStats
}
```

**Key Features:**
- Base structure (`BaseDatabase`) reduces code duplication
- Factory pattern for creating database instances
- Built-in health checking and statistics
- Support for MySQL (via GORM), MongoDB, and Redis

### 2. Middleware Stack (`/middleware`)

The middleware system follows a layered approach with proper ordering:

```
1. Recovery (Panic recovery)
2. Request ID (Unique request identification)
3. Security Headers (XSS, CSRF protection)
4. Rate Limiting (DDoS protection)
5. Logger (Structured logging with request context)
6. Authentication (JWT-based)
```

**Key Components:**
- `Manager`: Orchestrates middleware setup
- `RequestID`: Generates and propagates request IDs
- `RateLimit`: Redis-based rate limiting
- `InputValidation`: Protects against XSS, SQL injection, etc.

### 3. Configuration System (`/config`)

Hierarchical configuration with multiple sources:

```yaml
server:
  host: localhost
  port: 8080
  env: ${SERVER_ENV:development}  # Environment variable override
```

**Features:**
- YAML-based configuration files
- Environment variable overrides
- Type-safe configuration structures
- Validation on load

### 4. Monitoring & Observability (`/monitoring`)

Comprehensive monitoring setup:

- **Metrics**: Prometheus-based metrics for HTTP, database, and business operations
- **Dashboards**: Pre-configured Grafana dashboards
- **Alerts**: Prometheus alerting rules

### 5. Specialized Components

#### Distributed Lock (`/lock`)
- Redis-based distributed locking
- Automatic lock renewal
- Deadlock prevention
- Generic type support

#### Money/Decimal (`/decimal`)
- Precise decimal arithmetic
- Currency operations
- Database serialization
- JSON marshaling

#### Input Validation (`/validation`)
- Shared validation patterns
- Security-focused validation
- Reusable validation functions

## Request Flow

```
Client Request
    ↓
Gin Router
    ↓
Recovery Middleware
    ↓
Request ID Middleware
    ↓
Security Headers
    ↓
Rate Limiter
    ↓
Logger
    ↓
Input Validation
    ↓
Authentication (if required)
    ↓
Handler Function
    ↓
Business Logic
    ↓
Database Operations
    ↓
Response Formatting
    ↓
Client Response
```

## Dependency Management

The project uses a dependency injection pattern through configuration:

```go
// Example initialization
cfg, _ := config.Load("config.yaml")
db := database.NewFactory().Create(database.TypeMySQL, cfg.Database.MySQL)
redis := database.NewFactory().Create(database.TypeRedis, cfg.Redis)
middleware.NewManager(cfg, redis, logger, jwtService)
```

## Error Handling

Consistent error handling across all layers:

1. **Database Errors**: Wrapped with context and logged
2. **Validation Errors**: Return 400 with specific error messages
3. **Authentication Errors**: Return 401/403 with appropriate messages
4. **Internal Errors**: Logged with full context, return generic 500 to client

## Testing Strategy

1. **Unit Tests**: For individual components
2. **Integration Tests**: For database operations
3. **Middleware Tests**: Using httptest
4. **Benchmark Tests**: For performance-critical paths

## Performance Considerations

1. **Connection Pooling**: All database connections use pooling
2. **Caching**: Redis-based caching for frequently accessed data
3. **Rate Limiting**: Prevents resource exhaustion
4. **Graceful Shutdown**: Ensures clean connection closure

## Security Architecture

Multiple layers of security:

1. **Input Validation**: All inputs validated against malicious patterns
2. **SQL Injection Prevention**: Parameterized queries only
3. **XSS Protection**: Output encoding and CSP headers
4. **Rate Limiting**: Prevents brute force attacks
5. **Authentication**: JWT with refresh tokens
6. **HTTPS Enforcement**: Via security headers

## Extensibility

The architecture supports easy extension:

1. **New Database Types**: Implement the Database interface
2. **Custom Middleware**: Add to the middleware chain
3. **New Validators**: Add to the validation package
4. **Custom Metrics**: Register with Prometheus

## Best Practices

1. **Always use interfaces** for major components
2. **Configure through files**, not code
3. **Log structured data** with context
4. **Validate all inputs** at the edge
5. **Handle errors explicitly** with context
6. **Write tests** for all new functionality
7. **Document public APIs** with examples