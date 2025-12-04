.PHONY: help build run test lint docker-build docker-up docker-down clean deploy

help:
	@echo "APK Analysis Platform - Go Version"
	@echo ""
	@echo "Usage:"
	@echo "  make build             Build binary"
	@echo "  make run               Run in development mode"
	@echo "  make test              Run all tests with coverage"
	@echo "  make lint              Run linter"
	@echo "  make docker-build      Build Docker image"
	@echo "  make docker-up         Start Docker Compose"
	@echo "  make deploy            Full deployment (build + up)"
	@echo ""
	@echo "Testing:"
	@echo "  make test-unit         Run unit tests (repo/service/handler)"
	@echo "  make test-integration  Run integration tests"
	@echo "  make test-e2e          Run end-to-end tests"
	@echo "  make test-stress       Run stress tests (10-50 concurrent)"
	@echo "  make test-all          Run complete test suite"
	@echo "  make test-quick        Run quick tests (short mode)"
	@echo ""
	@echo "Benchmarks:"
	@echo "  make bench             Run all benchmarks"
	@echo "  make bench-stress      Run stress benchmarks"

build:
	@echo "Building..."
	@mkdir -p bin
	@go build -o bin/server ./cmd/server
	@echo "✓ Build complete: bin/server"

run:
	@echo "Running in development mode..."
	@go run ./cmd/server --config ./configs/config.yaml

test:
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"
	@go tool cover -func=coverage.out | grep total

# 单元测试
.PHONY: test-unit test-repo test-service test-handler
test-unit:
	@echo "Running unit tests..."
	@go test ./internal/repository -v -cover
	@go test ./internal/service -v -cover
	@go test ./internal/api/handlers -v -cover

test-repo:
	@echo "Running Repository tests..."
	@go test ./internal/repository -v -cover

test-service:
	@echo "Running Service tests..."
	@go test ./internal/service -v -cover

test-handler:
	@echo "Running Handler tests..."
	@go test ./internal/api/handlers -v -cover

# 性能测试
.PHONY: bench bench-repo bench-service bench-handler
bench:
	@echo "Running benchmarks..."
	@go test ./... -bench=. -benchmem

bench-repo:
	@go test ./internal/repository -bench=. -benchmem

bench-service:
	@go test ./internal/service -bench=. -benchmem

bench-handler:
	@go test ./internal/api/handlers -bench=. -benchmem

# 集成测试
.PHONY: test-integration test-stress test-e2e
test-integration:
	@echo "Running integration tests..."
	@go test ./tests/integration -v -timeout 5m

test-e2e:
	@echo "Running end-to-end tests..."
	@go test ./tests/integration -v -run TestEndToEnd -timeout 5m

# 压力测试
test-stress:
	@echo "Running stress tests (10 concurrent tasks)..."
	@go test ./tests/stress -v -run TestStress_10ConcurrentTasks -timeout 10m
	@echo ""
	@echo "Running stress tests (50 concurrent tasks)..."
	@go test ./tests/stress -v -run TestStress_50ConcurrentTasks -timeout 10m

test-stress-full:
	@echo "Running full stress test suite..."
	@go test ./tests/stress -v -timeout 30m

bench-stress:
	@echo "Running stress benchmarks..."
	@go test ./tests/stress -bench=. -benchmem -timeout 10m

# 完整测试套件
.PHONY: test-all test-quick
test-all:
	@echo "========================================"
	@echo "Running complete test suite..."
	@echo "========================================"
	@echo ""
	@echo "1. Unit Tests"
	@make test-unit
	@echo ""
	@echo "2. Integration Tests"
	@make test-integration
	@echo ""
	@echo "3. Stress Tests"
	@make test-stress
	@echo ""
	@echo "========================================"
	@echo "✅ All tests completed!"
	@echo "========================================"

test-quick:
	@echo "Running quick test suite (short mode)..."
	@go test -short ./... -v

lint:
	@echo "Running linter..."
	@golangci-lint run

docker-build:
	@echo "Building Docker image..."
	@docker build -t apk-analysis-go:latest .
	@echo "✓ Docker image built"

docker-up:
	@echo "Starting Docker Compose..."
	@docker-compose up -d
	@echo "✓ Services started"
	@echo "  Dashboard: http://localhost:3000"
	@echo "  API: http://localhost:8080"

docker-down:
	@echo "Stopping Docker Compose..."
	@docker-compose down
	@echo "✓ Services stopped"

deploy: docker-build docker-up
	@echo "✓ Deployment complete!"

clean:
	@echo "Cleaning..."
	@rm -rf bin/ coverage.out coverage.html
	@docker-compose down -v
	@echo "✓ Clean complete"

# 开发辅助命令
.PHONY: deps fmt

deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies updated"

fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✓ Code formatted"
