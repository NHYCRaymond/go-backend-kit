# Distributed Crawler System

A distributed web crawler system with hub-node architecture.

## Configuration

The system supports three levels of configuration (in order of precedence):

1. **Command-line flags** (highest priority)
2. **Environment variables** 
3. **Configuration files** (TOML format)

### Using Configuration Files

#### Hub Configuration

```bash
# Using TOML config file
./hub -config config/hub.toml

# Override specific settings
./hub -config config/hub.toml -redis localhost:6380 -grpc :50052
```

#### Node Configuration

```bash
# Using TOML config file
./node -config config/node.toml

# Override specific settings
./node -config config/node.toml -redis localhost:6380 -hub localhost:50052
```

### Using Environment Variables

```bash
# Copy example env file
cp .env.example .env

# Edit .env with your settings
vim .env

# Run with environment variables
export $(cat .env | xargs) && ./hub
export $(cat .env | xargs) && ./node
```

### Configuration Priority

Settings are applied in this order (later overrides earlier):
1. Default values in code
2. Configuration file (TOML)
3. Environment variables
4. Command-line flags

### Example: Development Setup

```bash
# Start Redis
redis-server

# Start Hub with config
./hub -config config/hub.toml -log debug

# Start Node with config
./node -config config/node.toml -log debug

# Start TUI Dashboard
./tui -redis localhost:6379
```

### Example: Production Setup

```bash
# Using environment variables for secrets
export REDIS_PASSWORD=your_redis_password
export NODE_ID=prod-node-01

# Start with production config
./hub -config config/production/hub.toml
./node -config config/production/node.toml
```

## Configuration Files

### Hub Configuration (hub.toml)

- `server`: gRPC and HTTP server settings
- `redis`: Redis connection and prefix
- `cluster`: Cluster identification and limits
- `coordinator`: Task coordination settings
- `dispatcher`: Task dispatching settings
- `communication`: Node communication timeouts
- `monitoring`: Metrics and monitoring
- `logging`: Log level and format

### Node Configuration (node.toml)

- `node`: Node identity and capabilities
- `hub`: Hub connection settings
- `redis`: Redis connection settings
- `crawler`: Crawling behavior and rate limits
- `fetcher`: HTTP fetching settings
- `extractor`: Data extraction settings
- `storage`: Storage backend configuration
- `monitoring`: Node metrics reporting
- `logging`: Log level and format

## Benefits of TOML Configuration

1. **Structured**: Clear hierarchy for complex settings
2. **Type-safe**: Explicit types for all values
3. **Documented**: Inline comments explain options
4. **Readable**: Human-friendly format
5. **Standard**: Common in Go ecosystem

## Migration from Command-Line Flags

Instead of:
```bash
./hub -redis localhost:6379 -grpc :50051 -log debug
```

Use:
```bash
./hub -config config/hub.toml
```

Or with overrides:
```bash
./hub -config config/hub.toml -log debug
```