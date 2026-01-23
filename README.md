# API Cache Proxy

A high-performance, configurable caching proxy that sits between clients and upstream services, providing intelligent caching, rate limiting, and retry capabilities using Valkey (Redis fork) as the cache storage.

## Features

- **Smart Caching**: Analyzes request headers and query parameters to generate unique cache keys
- **Configurable TTL**: Per-endpoint cache expiration configuration
- **Rate Limiting**: Global and per-endpoint rate limiting to prevent abuse
- **Retry Logic**: Automatic retry with exponential backoff for transient failures
- **Advanced Logging**: Structured JSON logging with configurable levels
- **High Performance**: Lightweight design optimized for minimal overhead
- **Docker Support**: Complete Docker and docker-compose setup for easy deployment

## Architecture

```
Client → API Cache Proxy → Valkey Cache
                ↓
         Upstream Service
```

The proxy intercepts requests, checks the cache, and either:
1. Returns cached response (cache hit)
2. Forwards to upstream, caches response, and returns it (cache miss)

## Prerequisites

- Docker and Docker Compose
- Go 1.25+ (for local development)
- (Optional) [just](https://github.com/casey/just) - A handy command runner

### Installing just (optional but recommended)

```bash
# macOS
brew install just

# Linux
curl --proto '=https' --tlsv1.2 -sSf https://just.systems/install.sh | bash -s -- --to /usr/local/bin

# Or use cargo
cargo install just
```

## Quick Start

### Using Docker Compose (Recommended)

1. Clone the repository:
```bash
git clone https://github.com/singh-gur/api_cache.git
cd api_cache
```

2. Start all services:
```bash
docker-compose up -d
```

This will start:
- Valkey cache on port 6379
- API Cache Proxy on port 8080
- Mock upstream service on port 9000

3. Test the proxy:
```bash
# Health check
curl http://localhost:8080/health

# Test caching (using mock upstream)
curl http://localhost:8080/get
curl http://localhost:8080/get  # This should be served from cache
```

### Local Development

1. Install dependencies:
```bash
just deps
# or: go mod download
```

2. Start Valkey:
```bash
docker run -d -p 6379:6379 valkey/valkey:latest
```

3. Run the application:
```bash
just run
# or: go run cmd/api-cache/main.go -config config.yaml
```

## Configuration

The application is configured via YAML files:

- **`config.yaml`** - Default configuration for local development (uses `localhost`)
- **`config.docker.yaml`** - Docker-specific configuration (uses Docker service names)
- **`config.example.yaml`** - Fully commented example showing all available options

For local development, use `config.yaml`. For Docker deployments, `docker-compose.yml` automatically uses `config.docker.yaml`.

### Server Configuration

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s
```

### Valkey Configuration

```yaml
valkey:
  host: "localhost"
  port: 6379
  password: ""
  db: 0
  max_retries: 3
  pool_size: 10
  min_idle_conns: 5
```

### Cache Configuration

Configure default TTL and per-endpoint caching rules:

```yaml
cache:
  default_ttl: 300s  # Used for endpoints not explicitly configured
  max_ttl: 3600s
  endpoints:
    # Exact path match
    - path: "/api/v1/users"
      methods: ["GET"]
      ttl: 600s
      cache_key_headers: ["Authorization"]
      cache_key_query_params: ["page", "limit"]
    
    # Regex pattern match - matches /api/v1/users/123, /api/v1/users/456, etc.
    - path_regex: "^/api/v1/users/[0-9]+$"
      methods: ["GET"]
      ttl: 900s
      cache_key_headers: ["Authorization"]
```

**Endpoint Matching**:
- Endpoints can be matched by exact `path` or regex `path_regex`
- Exact path matches are checked first, then regex patterns
- If no endpoint matches, `default_ttl` is used
- Only GET requests are cached by default

**Cache Key Generation**: The proxy generates cache keys based on:
- HTTP method
- Request path
- Configured query parameters
- Configured headers

**Unconfigured Endpoints**: If an endpoint is not explicitly configured:
- GET requests are still cached using `default_ttl`
- Cache keys include only method and path (no specific headers/params)
- Rate limiting uses global settings

### Rate Limiting

```yaml
rate_limit:
  enabled: true
  requests_per_second: 100  # Global default
  burst: 200
  endpoints:
    # Exact path match
    - path: "/api/v1/search"
      requests_per_second: 10
      burst: 20
    
    # Regex pattern - rate limit all admin endpoints
    - path_regex: "^/api/v1/admin/.*"
      requests_per_second: 5
      burst: 10
```

Rate limiting supports both exact path matching and regex patterns, just like cache configuration.

### Retry Configuration

```yaml
retry:
  enabled: true
  max_attempts: 3
  initial_backoff: 100ms
  max_backoff: 2s
  backoff_multiplier: 2.0
  retryable_status_codes: [500, 502, 503, 504]
```

### Upstream Configuration

```yaml
upstream:
  base_url: "http://localhost:9000"
  timeout: 30s
  max_idle_conns: 100
  max_conns_per_host: 10
```

### Logging Configuration

```yaml
logging:
  level: "info"  # debug, info, warn, error
  format: "json"  # json, text
  output: "stdout"  # stdout, stderr, file
  file_path: "/var/log/api-cache.log"
```

## Regex Pattern Matching

Both cache and rate limit configurations support regex patterns for flexible endpoint matching.

### Common Regex Patterns

```yaml
# Match user IDs: /api/v1/users/123, /api/v1/users/456
path_regex: "^/api/v1/users/[0-9]+$"

# Match all admin endpoints: /api/v1/admin/users, /api/v1/admin/settings
path_regex: "^/api/v1/admin/.*"

# Match UUID paths: /api/v1/resources/550e8400-e29b-41d4-a716-446655440000
path_regex: "^/api/v1/resources/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"

# Match versioned APIs: /api/v1/..., /api/v2/...
path_regex: "^/api/v[0-9]+/.*"

# Match file extensions: /static/images/logo.png, /static/css/style.css
path_regex: "^/static/.*\\.(png|jpg|css|js)$"
```

### Matching Priority

1. **Exact path match** is checked first
2. **Regex patterns** are checked in order of definition
3. **First match wins** - subsequent patterns are not evaluated
4. **No match** - uses default settings (default_ttl for cache, global rate limit)

### Tips

- Use `^` and `$` anchors to match the entire path
- Test your regex patterns before deploying
- More specific patterns should be defined before general ones
- Exact paths are faster than regex - use them when possible

## API Endpoints

### Health Check

```bash
GET /health
```

Returns the health status of the service.

### Proxy Endpoints

All other endpoints are proxied to the upstream service with caching applied based on configuration.

## Response Headers

The proxy adds the following headers to responses:

- `X-Cache`: `HIT` or `MISS` indicating cache status
- `X-Cache-Time`: Timestamp when the response was cached (only on cache hits)

## Building

### Build Binary

```bash
just build
# or: go build -o api-cache ./cmd/api-cache
```

### Build Docker Image

```bash
just docker-build
# or: docker build -t api-cache:latest .
```

## Performance Considerations

- **Lightweight**: Minimal overhead with efficient cache key generation
- **Connection Pooling**: Configurable connection pools for both Valkey and upstream
- **Concurrent Requests**: Handles multiple concurrent requests efficiently
- **Memory Efficient**: Streams large responses without buffering entire body

## Monitoring

The application provides structured JSON logs with the following information:

- Request method, path, and duration
- Cache hit/miss status
- Rate limit violations
- Retry attempts
- Error conditions

Example log entry:
```json
{
  "cache": "hit",
  "duration": 5,
  "level": "info",
  "msg": "Request served from cache",
  "status": 200,
  "time": "2026-01-23T12:00:00Z"
}
```

## Development

### Project Structure

```
.
├── cmd/
│   └── api-cache/
│       └── main.go           # Application entry point
├── internal/
│   ├── cache/
│   │   └── cache.go          # Cache client and operations
│   ├── config/
│   │   └── config.go         # Configuration management
│   ├── logger/
│   │   └── logger.go         # Logging setup
│   ├── middleware/
│   │   └── ratelimit.go      # Rate limiting middleware
│   └── proxy/
│       └── proxy.go          # Proxy handler with caching
├── scripts/
│   └── test-cache.sh         # Testing script
├── config.yaml               # Local configuration
├── config.docker.yaml        # Docker configuration
├── docker-compose.yml        # Docker Compose setup
├── Dockerfile                # Container image definition
├── justfile                  # Task runner (replaces Makefile)
└── README.md
```

### Running Tests

```bash
just test
# or: go test ./...

# With coverage
just test-coverage
```

## Deployment

### Docker Compose

The easiest way to deploy is using docker-compose:

```bash
docker-compose up -d
```

### Kubernetes

For Kubernetes deployment, you'll need to:
1. Build and push the Docker image to your registry
2. Create ConfigMap for configuration
3. Deploy Valkey (or use managed Redis/Valkey)
4. Deploy the API Cache Proxy with appropriate service and ingress

### Environment Variables

You can override configuration values using environment variables in the format:
```
SECTION_KEY=value
```

## Troubleshooting

### Cache Not Working

1. Check Valkey connection:
```bash
docker logs api-cache-valkey
```

2. Verify configuration:
```bash
docker logs api-cache-proxy
```

3. Check cache headers in response:
```bash
curl -v http://localhost:8080/your-endpoint
```

### Rate Limiting Issues

Check the rate limit configuration and adjust `requests_per_second` and `burst` values as needed.

### High Latency

1. Check upstream service performance
2. Verify Valkey connection pool settings
3. Review retry configuration
4. Check network latency between services

## Contributing

Contributions are welcome! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License - see LICENSE file for details

## Useful Commands (using just)

```bash
# Quick start everything
just quickstart

# Build and run locally
just dev

# Run tests
just test

# View logs
just docker-logs

# Check cache keys
just cache-keys

# Clear cache
just cache-clear

# Health check
just health-check

# See all available commands
just --list
```

## Support

For issues and questions, please open an issue on GitHub.
