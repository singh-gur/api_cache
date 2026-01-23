# API Cache Proxy - Justfile
# Run `just` or `just --list` to see all available recipes

# Variables
binary_name := "api-cache"
docker_image := "api-cache:latest"
config_file := "config.yaml"

# Default recipe - show help
default:
    @just --list

# Build the application
build:
    @echo "Building {{binary_name}}..."
    go build -o {{binary_name}} ./cmd/api-cache
    @echo "Build complete: {{binary_name}}"

# Run the application locally
run: build
    @echo "Running {{binary_name}}..."
    ./{{binary_name}} -config {{config_file}}

# Run tests
test:
    @echo "Running tests..."
    go test -v ./...

# Run tests with coverage
test-coverage:
    @echo "Running tests with coverage..."
    go test -v -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
    @echo "Cleaning..."
    rm -f {{binary_name}}
    rm -f coverage.out coverage.html
    @echo "Clean complete"

# Build Docker image
docker-build:
    @echo "Building Docker image..."
    docker build -t {{docker_image}} .
    @echo "Docker image built: {{docker_image}}"

# Start services with docker-compose
docker-up:
    @echo "Starting services..."
    docker-compose up -d
    @echo "Services started. API Cache Proxy available at http://localhost:8080"

# Stop services
docker-down:
    @echo "Stopping services..."
    docker-compose down
    @echo "Services stopped"

# View logs from all services
docker-logs:
    docker-compose logs -f

# View logs for proxy service
docker-logs-proxy:
    docker-compose logs -f api-cache

# View logs for valkey service
docker-logs-valkey:
    docker-compose logs -f valkey

# Restart services
docker-restart: docker-down docker-up

# Install dependencies
deps:
    @echo "Installing dependencies..."
    go mod download
    go mod tidy
    @echo "Dependencies installed"

# Format code
fmt:
    @echo "Formatting code..."
    go fmt ./...
    @echo "Code formatted"

# Run linter
lint:
    @echo "Running linter..."
    golangci-lint run ./...

# Development setup
dev-setup: deps
    @echo "Setting up development environment..."
    docker run -d -p 6379:6379 --name api-cache-valkey-dev valkey/valkey:latest
    @echo "Development environment ready"

# Stop development environment
dev-teardown:
    @echo "Tearing down development environment..."
    -docker stop api-cache-valkey-dev
    -docker rm api-cache-valkey-dev
    @echo "Development environment cleaned up"

# Run the test script
test-cache:
    @echo "Running cache test script..."
    ./scripts/test-cache.sh

# Check if services are healthy
health-check:
    @echo "Checking service health..."
    @curl -s http://localhost:8080/health | jq '.' || echo "Service not responding"

# View cache keys in Valkey
cache-keys:
    @echo "Cache keys in Valkey:"
    docker exec -it api-cache-valkey valkey-cli KEYS 'cache:*'

# Clear all cache
cache-clear:
    @echo "Clearing all cache..."
    docker exec -it api-cache-valkey valkey-cli FLUSHDB
    @echo "Cache cleared"

# Monitor Valkey commands in real-time
cache-monitor:
    @echo "Monitoring Valkey commands (Ctrl+C to stop)..."
    docker exec -it api-cache-valkey valkey-cli MONITOR

# Build and run locally
dev: build
    @echo "Starting local development server..."
    ./{{binary_name}} -config {{config_file}}

# Full rebuild (clean + build)
rebuild: clean build

# Run all checks (fmt, test, lint)
check: fmt test lint

# Quick start - setup and run everything
quickstart: docker-up
    @echo ""
    @echo "✅ API Cache Proxy is running!"
    @echo ""
    @echo "Try these commands:"
    @echo "  curl http://localhost:8080/health"
    @echo "  curl http://localhost:8080/get"
    @echo "  just test-cache"
    @echo ""
    @echo "View logs:"
    @echo "  just docker-logs"
    @echo ""
    @echo "Stop services:"
    @echo "  just docker-down"
