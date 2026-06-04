.PHONY: help build run-api run-comment run-worker test test-integration benchmark docker-build migrate clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build all services
	@echo "Building services..."
	@cd cmd/api-server && go build -o ../../bin/api-server .
	@cd cmd/comment-service && go build -o ../../bin/comment-service .
	@cd cmd/worker && go build -o ../../bin/worker .
	@echo "✓ Build complete"

run-api: ## Run API server
	@go run cmd/api-server/main.go

run-comment: ## Run comment service (gRPC)
	@go run cmd/comment-service/main.go

run-worker: ## Run Kafka worker
	@go run cmd/worker/main.go

test: ## Run unit tests
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Test coverage report: coverage.html"

test-integration: ## Run integration tests
	@go test -v -race -tags=integration ./tests/integration/...

benchmark: ## Run performance benchmarks
	@go test -bench=. -benchmem ./tests/benchmark/...

docker-build: ## Build Docker images
	@docker build -t reference-blog/api-server:latest -f deployments/docker/Dockerfile --target api-server .
	@docker build -t reference-blog/comment-service:latest -f deployments/docker/Dockerfile --target comment-service .
	@docker build -t reference-blog/worker:latest -f deployments/docker/Dockerfile --target worker .

migrate: ## Run database migrations
	@echo "Running database migrations..."
	@go run scripts/migrate.go

clean: ## Clean build artifacts
	@rm -rf bin/ coverage.out coverage.html
	@echo "✓ Cleaned"

proto: ## Generate gRPC code from proto files
	@protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		internal/proto/comment.proto
	@echo "✓ Proto files generated"

lint: ## Run linters
	@golangci-lint run ./...

deps: ## Download dependencies
	@go mod download
	@go mod tidy

dev: ## Start all services in development mode
	@docker-compose up -d postgres redis kafka elasticsearch
	@echo "Waiting for services to be ready..."
	@sleep 10
	@make migrate
	@echo "Starting services..."
	@trap 'kill 0' EXIT; \
		go run cmd/api-server/main.go & \
		go run cmd/comment-service/main.go & \
		go run cmd/worker/main.go & \
		wait

down: ## Stop all Docker services
	@docker-compose down -v
