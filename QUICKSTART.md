# Quick Start Guide

Get the API Cache Proxy up and running in 5 minutes!

## Prerequisites

- Docker and Docker Compose installed
- OR Go 1.25+ installed for local development
- (Optional) [just](https://github.com/casey/just) command runner for convenience

## Option 1: Docker Compose (Recommended)

### 1. Start the services

```bash
just quickstart
# or: docker-compose up -d
```

This starts:
- **Valkey** (cache storage) on port 6379
- **API Cache Proxy** on port 8080
- **Mock upstream service** (httpbin) on port 9000

### 2. Verify it's running

```bash
# Check health
curl http://localhost:8080/health

# Expected output:
# {"status":"healthy","service":"api-cache"}
```

### 3. Test caching

```bash
# First request (cache MISS)
curl -v http://localhost:8080/get 2>&1 | grep X-Cache
# Expected: X-Cache: MISS

# Second request (cache HIT)
curl -v http://localhost:8080/get 2>&1 | grep X-Cache
# Expected: X-Cache: HIT
```

### 4. Run the test script

```bash
./scripts/test-cache.sh
```

### 5. View logs

```bash
# All services
just docker-logs
# or: docker-compose logs -f

# Just the proxy
just docker-logs-proxy
# or: docker-compose logs -f api-cache

# Just Valkey
just docker-logs-valkey
# or: docker-compose logs -f valkey
```

### 6. Stop services

```bash
just docker-down
# or: docker-compose down
```

## Option 2: Local Development

### 1. Install dependencies

```bash
just deps
# or: go mod download
```

### 2. Start Valkey

```bash
docker run -d -p 6379:6379 --name valkey-dev valkey/valkey:latest
```

### 3. Configure upstream

Edit `config.yaml` and set your upstream service URL:

```yaml
upstream:
  base_url: "http://your-upstream-service:port"
```

### 4. Run the proxy

```bash
just run
# or: go run cmd/api-cache/main.go -config config.yaml
```

Or build and run separately:

```bash
just build
./api-cache -config config.yaml
```

### 5. Test it

```bash
curl http://localhost:8080/health
curl http://localhost:8080/your-endpoint
```

## Configuration

### Quick Configuration Changes

Edit `config.yaml` (or `config.docker.yaml` for Docker):

**Change cache TTL:**
```yaml
cache:
  default_ttl: 600s  # 10 minutes instead of 5
```

**Adjust rate limits:**
```yaml
rate_limit:
  requests_per_second: 200  # Increase from 100
  burst: 400
```

**Change log level:**
```yaml
logging:
  level: "debug"  # More verbose logging
```

**Configure endpoint-specific caching:**
```yaml
cache:
  endpoints:
    # Exact path
    - path: "/api/v1/your-endpoint"
      methods: ["GET"]
      ttl: 1800s  # 30 minutes
      cache_key_query_params: ["id", "filter"]
    
    # Regex pattern - match all user detail pages
    - path_regex: "^/api/v1/users/[0-9]+$"
      methods: ["GET"]
      ttl: 900s
```

After changing config, restart:
```bash
just docker-restart
# or: docker-compose restart api-cache
```

## Common Use Cases

### 1. Cache API responses with user-specific data

```yaml
cache:
  endpoints:
    - path: "/api/v1/user/profile"
      methods: ["GET"]
      ttl: 300s
      cache_key_headers: ["Authorization"]  # Different cache per user
```

### 2. Rate limit expensive endpoints

```yaml
rate_limit:
  endpoints:
    - path: "/api/v1/expensive-operation"
      requests_per_second: 5
      burst: 10
```

### 3. Cache search results

```yaml
cache:
  endpoints:
    - path: "/api/v1/search"
      methods: ["GET"]
      ttl: 600s
      cache_key_query_params: ["q", "page", "limit"]
```

## Monitoring

### Check cache statistics

```bash
# View all cache keys
just cache-keys

# Clear all cache
just cache-clear

# Monitor commands in real-time
just cache-monitor

# Or connect to Valkey directly
docker exec -it api-cache-valkey valkey-cli
```

### View application metrics

Check the JSON logs for:
- `cache: "hit"` or `cache: "miss"`
- `duration` in milliseconds
- `status` HTTP status codes

```bash
docker-compose logs api-cache | grep cache
```

## Troubleshooting

### Cache not working?

1. Check Valkey is running:
   ```bash
   docker exec -it api-cache-valkey valkey-cli PING
   ```

2. Verify cache keys are being created:
   ```bash
   docker exec -it api-cache-valkey valkey-cli KEYS 'cache:*'
   ```

3. Check logs for errors:
   ```bash
   docker-compose logs api-cache | grep -i error
   ```

### Rate limiting too aggressive?

Increase limits in config:
```yaml
rate_limit:
  requests_per_second: 1000
  burst: 2000
```

### Upstream connection issues?

1. Verify upstream URL in config
2. Check network connectivity
3. Review retry settings:
   ```yaml
   retry:
     max_attempts: 5
     max_backoff: 5s
   ```

## Next Steps

- Read the full [README.md](README.md) for detailed documentation
- Customize `config.yaml` for your use case
- Add more endpoint-specific configurations
- Set up monitoring and alerting
- Deploy to production with Kubernetes

## Useful Commands

```bash
# Quick start everything
just quickstart

# Build
just build

# Run tests
just test

# Start with Docker
just docker-up

# View logs
just docker-logs

# Stop services
just docker-down

# Clean up
just clean

# See all available commands
just --list
```

## Getting Help

- Check the [README.md](README.md) for detailed documentation
- Review [config.example.yaml](config.example.yaml) for all options
- Open an issue on GitHub for bugs or questions
