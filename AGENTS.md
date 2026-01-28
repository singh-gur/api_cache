# AGENTS.md

## Project Overview

Go caching reverse proxy (API Cache Proxy) that sits between clients and upstream
services. Uses Valkey (Redis fork) for cache storage. Provides per-endpoint caching,
rate limiting, and retry with exponential backoff. Standard library `net/http` for
HTTP handling (no framework). Module path: `github.com/singh-gur/api_cache`.

## Build / Lint / Test Commands

```bash
# Build binary
go build -o api-cache ./cmd/api-cache

# Run all tests (verbose)
go test -v ./...

# Run a single test by name (use -run with regex)
go test -v -run TestGenerateCacheKey ./internal/cache/...

# Run all tests in a single package
go test -v ./internal/config/...

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Format code
go fmt ./...

# Lint (requires golangci-lint)
golangci-lint run ./...

# Run all checks (format + test + lint)
# via justfile:
just check

# Install / tidy dependencies
go mod download && go mod tidy
```

The project uses a `justfile` (not Makefile). Key recipes: `just build`, `just test`,
`just fmt`, `just lint`, `just check`, `just run`, `just docker-up`, `just docker-down`.

## Project Structure

```
cmd/api-cache/main.go        # Entrypoint: config loading, server setup, graceful shutdown
internal/
  cache/cache.go              # Valkey client, cache key generation, get/set/delete
  cache/cache_test.go         # Unit tests for cache key generation
  config/config.go            # YAML config loading, validation, regex compilation, query param matching
  config/config_test.go       # Unit tests for endpoint matching (exact, regex, query param)
  logger/logger.go            # Global logrus logger initialization + helpers
  middleware/ratelimit.go     # Per-path rate limiting middleware (golang.org/x/time/rate)
  proxy/proxy.go              # HTTP handler: cache lookup, upstream forwarding, retry logic
```

## Code Style Guidelines

### Language & Version

- Go 1.25+ (see `go.mod`)
- No web framework; uses `net/http` stdlib directly (`http.ServeMux`, `http.Handler`)

### Import Ordering

Group imports in this order, separated by blank lines:
1. Standard library (`context`, `fmt`, `net/http`, `time`, etc.)
2. Third-party packages (`github.com/redis/go-redis/v9`, `github.com/sirupsen/logrus`, etc.)
3. Internal packages (`github.com/singh-gur/api_cache/internal/...`)

```go
import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/singh-gur/api_cache/internal/config"
    "github.com/singh-gur/api_cache/internal/logger"
)
```

### Naming Conventions

- **Packages**: short, lowercase, single-word (`cache`, `config`, `proxy`, `logger`, `middleware`)
- **Types**: PascalCase exported structs (`Client`, `Handler`, `Config`, `CachedResponse`)
- **Unexported fields/methods**: camelCase (`compiledRegex`, `httpClient`, `isRetryableStatus`)
- **Config struct tags**: `yaml:"snake_case"` for YAML, `json:"snake_case"` for JSON
- **Constructors**: `NewXxx` pattern (`NewClient`, `NewHandler`, `NewRateLimiter`)
- **Test functions**: `TestFunctionName_Scenario` (e.g., `TestGetEndpointCacheConfig_ExactMatch`)

### Formatting

- Use `go fmt` / `gofmt` (standard Go formatting)
- No custom formatter configuration

### Types & Structs

- Config structs use `yaml:"field_name"` tags for YAML deserialization
- JSON serialization uses `json:"snake_case"` tags (see `CachedResponse`)
- Unexported fields that shouldn't be serialized use `yaml:"-"` tag
- Pointer receivers for methods on stateful structs (`*Client`, `*Handler`, `*RateLimiter`)
- Pass config as `*config.Config` (pointer to avoid copying)

### Error Handling

- Wrap errors with `fmt.Errorf("context: %w", err)` for error chains
- Return `nil` for "not found" cases (e.g., cache miss returns `nil, nil`)
- Fatal errors at startup use `logger.Log.Fatalf(...)` or `os.Exit(1)`
- HTTP errors use `http.Error(w, message, statusCode)` with lowercase messages
- Log errors with structured fields: `logger.WithField("error", err).Error("message")`

### Logging

- Uses `github.com/sirupsen/logrus` via a global `logger.Log` singleton
- Structured logging with `logger.WithFields(map[string]interface{}{...})` or `logger.WithField(key, value)`
- Log levels: Debug for cache hits, Info for request lifecycle, Warn for retries/rate limits, Error for failures
- Do NOT use `fmt.Println` for application logging; only use `fmt.Fprintf(os.Stderr, ...)` for pre-logger startup errors

### HTTP Patterns

- Handlers implement `http.Handler` interface (`ServeHTTP` method) or return `http.HandlerFunc`
- Middleware uses the `func(http.Handler) http.Handler` pattern
- Routes registered on `http.NewServeMux()`
- Health endpoint at `/health` returns JSON `{"status":"healthy","service":"api-cache"}`
- Cache status communicated via `X-Cache: HIT` / `X-Cache: MISS` response headers

### Testing Patterns

- Table-driven tests with `[]struct{ name string; ... }` and `t.Run(tt.name, ...)`
- Test files in the same package as the code under test (e.g., `package cache`)
- Construct test structs directly (no mocking framework); e.g., `&Client{config: cfg}`
- Use `t.Error`/`t.Errorf` for non-fatal assertions, `t.Fatal`/`t.Fatalf` when test cannot continue
- No external test dependencies; tests use only stdlib `testing` package

### Concurrency

- `sync.RWMutex` for concurrent map access (see `RateLimiter.limiters`)
- Double-checked locking pattern: read-lock check, then write-lock with re-check
- Goroutines for server startup; `os.Signal` channel for graceful shutdown
- Context propagation: pass `context.Context` from `r.Context()` through to cache/upstream calls

### Configuration

- YAML-based config files (`config.yaml`, `config.docker.yaml`)
- Config loaded once at startup via `config.Load(path)`
- Regex patterns in config are compiled at load time and stored as unexported `compiledRegex` fields
- Query param matching supports two modes:
  - `match_query_params` (`map[string][]string`): exact match against a list of allowed values
  - `match_query_params_regex` (`map[string][]string`): regex match against a list of patterns, compiled at load time into `compiledQueryParamRegex`
- Endpoints with query param matching take precedence; endpoints without serve as fallbacks
- Validation runs at load time; fail fast on invalid config

### Dependencies

- `github.com/redis/go-redis/v9` - Valkey/Redis client
- `github.com/sirupsen/logrus` - Structured logging
- `golang.org/x/time` - Rate limiting (`rate.Limiter`)
- `gopkg.in/yaml.v3` - YAML config parsing

### Docker

- Multi-stage Dockerfile: `golang:1.25-alpine` builder, `alpine:latest` runtime
- Runs as non-root user (`appcache:1000`)
- Binary built with `-ldflags="-w -s"` and `CGO_ENABLED=0`
- `docker-compose.yml` orchestrates proxy + Valkey + mock upstream
