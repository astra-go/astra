.PHONY: help build run-api run-comment run-worker test test-integration benchmark docker-build docker-buildx migrate clean proto lint deps dev dev-full down k8s-deploy k8s-delete

# Binary output directory
BIN := bin

# Docker image name
IMAGE := reference-blog

# Kubernetes namespace
K8S_NS := reference-blog

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ── Build ──────────────────────────────────────────────────────────────────────

build: ## Build all services
	@echo "Building services..."
	@mkdir -p $(BIN)
	@go build -ldflags="-s -w" -o $(BIN)/api-server       ./cmd/api-server
	@go build -ldflags="-s -w" -o $(BIN)/comment-service  ./cmd/comment-service
	@go build -ldflags="-s -w" -o $(BIN)/worker           ./cmd/worker
	@echo "✓ Build complete → $(BIN)/"

# ── Run ───────────────────────────────────────────────────────────────────────

run-api: ## Run API server
	@DATABASE_DSN="postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable" \
		REDIS_ADDR="localhost:6379" \
		KAFKA_BROKERS="localhost:9092" \
		ELASTICSEARCH_ADDRESSES="http://localhost:9200" \
		JAEGER_ENDPOINT="http://localhost:14268/api/traces" \
		JWT_SECRET="dev-jwt-secret" \
		SERVER_MODE="debug" \
		go run ./cmd/api-server

run-comment: ## Run comment service (gRPC)
	@DATABASE_DSN="postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable" \
		JAEGER_ENDPOINT="http://localhost:14268/api/traces" \
		go run ./cmd/comment-service

run-worker: ## Run Kafka worker
	@DATABASE_DSN="postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable" \
		KAFKA_BROKERS="localhost:9092" \
		ELASTICSEARCH_ADDRESSES="http://localhost:9200" \
		JAEGER_ENDPOINT="http://localhost:14268/api/traces" \
		go run ./cmd/worker

# ── Test ──────────────────────────────────────────────────────────────────────

test: ## Run unit tests
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report → coverage.html"

test-integration: ## Run integration tests
	@go test -v -race -tags=integration ./tests/integration/...

benchmark: ## Run performance benchmarks
	@go test -bench=. -benchmem -benchtime=3s ./tests/benchmark/...

# ── Docker ────────────────────────────────────────────────────────────────────

docker-build: ## Build Docker images (single-platform)
	@docker build \
		-t $(IMAGE)/api-server:latest \
		-f deployments/docker/Dockerfile \
		--target api-server \
		.
	@docker build \
		-t $(IMAGE)/comment-service:latest \
		-f deployments/docker/Dockerfile \
		--target comment-service \
		.
	@docker build \
		-t $(IMAGE)/worker:latest \
		-f deployments/docker/Dockerfile \
		--target worker \
		.
	@echo "✓ Docker images built"

docker-buildx: docker-build ## Alias for multi-platform build
docker-buildx: IMG_TAG ?= latest
docker-buildx: ## Build and push multi-platform images (requires Docker BuildKit)
	@docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t $(IMAGE)/api-server:$(IMG_TAG) \
		-t $(IMAGE)/comment-service:$(IMG_TAG) \
		-t $(IMAGE)/worker:$(IMG_TAG) \
		-f deployments/docker/Dockerfile \
		--push \
		.

# ── Migration ─────────────────────────────────────────────────────────────────

migrate: ## Run database migrations
	@go run scripts/migrate.go

migrate-seed: ## Run migrations with seed data
	@go run scripts/migrate.go -seed

# ── Development ────────────────────────────────────────────────────────────────

dev: ## Start infrastructure (Docker Compose)
	@docker-compose up -d postgres redis kafka elasticsearch jaeger prometheus grafana
	@echo "Waiting for infrastructure (30s)..."
	@sleep 30
	@make migrate
	@echo "✓ Infrastructure ready. Run 'make run-api' etc. to start services."

dev-full: ## Start infrastructure + services (foreground)
	@make dev
	@echo "Starting services..."
	@trap 'kill 0' EXIT; \
		make run-api & \
		make run-comment & \
		make run-worker & \
		wait

down: ## Stop all Docker services
	@docker-compose down -v

# ── Kubernetes ────────────────────────────────────────────────────────────────

k8s-deploy: ## Deploy to Kubernetes
	@kubectl apply -f deployments/k8s/namespace.yaml
	@kubectl apply -f deployments/k8s/configmap.yaml
	@kubectl apply -f deployments/k8s/secrets.yaml
	@kubectl apply -f deployments/k8s/api-server.yaml
	@kubectl apply -f deployments/k8s/comment-service.yaml
	@kubectl apply -f deployments/k8s/worker.yaml
	@echo "✓ Deployed to namespace $(K8S_NS)"
	@kubectl -n $(K8S_NS) get pods

k8s-delete: ## Delete from Kubernetes
	@kubectl delete -f deployments/k8s/worker.yaml \
		-f deployments/k8s/comment-service.yaml \
		-f deployments/k8s/api-server.yaml \
		-f deployments/k8s/secrets.yaml \
		-f deployments/k8s/configmap.yaml \
		-f deployments/k8s/namespace.yaml
	@echo "✓ Removed from namespace $(K8S_NS)"

# ── Proto ────────────────────────────────────────────────────────────────────

proto: ## Generate gRPC code from proto files
	@protoc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		internal/proto/comment.proto
	@echo "✓ Proto generated"

# ── Quality ──────────────────────────────────────────────────────────────────

lint: ## Run linters
	@golangci-lint run ./...

deps: ## Download and tidy dependencies
	@go mod download
	@go mod tidy

clean: ## Clean build artifacts
	@rm -rf $(BIN)/ coverage.out coverage.html
	@echo "✓ Cleaned"
