# API Cache Proxy - Justfile
# Run `just` or `just --list` to see all available recipes

# Variables
binary_name := "api-cache"
docker_image := "api-cache:latest"
registry_image := "regv2.gsingh.io/personal/api_cache"
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

# Git-derived image tags: git tag (or short commit hash) + branch name
git_tag := `git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD`
git_branch := `git rev-parse --abbrev-ref HEAD | sed 's/[^a-zA-Z0-9._-]/-/g'`

# Build and tag image for registry (git tag/hash + branch)
img-build:
    @echo "Building image for registry..."
    docker build --load -t {{registry_image}}:{{git_tag}} -t {{registry_image}}:{{git_branch}} .
    @echo "Image built: {{registry_image}}:{{git_tag}} {{registry_image}}:{{git_branch}}"

# Push image to registry (git tag/hash + branch)
img-push:
    @echo "Pushing {{registry_image}}:{{git_tag}} and {{registry_image}}:{{git_branch}}..."
    docker push {{registry_image}}:{{git_tag}}
    docker push {{registry_image}}:{{git_branch}}
    @echo "Pushed: {{registry_image}}:{{git_tag}} {{registry_image}}:{{git_branch}}"

# Build and push image to registry
img-publish: img-build img-push
    @echo "Published {{registry_image}}:{{git_tag}} and {{registry_image}}:{{git_branch}}"

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

# Set Concourse pipeline
ci-set TARGET:
    ci/set-pipeline.sh {{TARGET}}

# Trigger test job
ci-test TARGET:
    fly -t {{TARGET}} trigger-job -j api-cache/test

# Trigger build job
ci-build TARGET:
    fly -t {{TARGET}} trigger-job -j api-cache/build-and-push

# Trigger release job
ci-release TARGET:
    fly -t {{TARGET}} trigger-job -j api-cache/release

# Watch build job logs
ci-watch TARGET:
    fly -t {{TARGET}} watch -j api-cache/build-and-push

# Destroy pipeline
ci-destroy TARGET:
    fly -t {{TARGET}} destroy-pipeline -p api-cache

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
