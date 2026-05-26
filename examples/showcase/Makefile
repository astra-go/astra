.PHONY: dev down proto tidy test test-integration build \
        run-api run-grpc run-worker \
        lint bench bench-smoke bench-stress validate \
        migrate-up migrate-down migrate-status seed

# ── Infrastructure ────────────────────────────────────────────────────────────
dev:
	docker compose up -d postgres redis jaeger prometheus grafana
	@echo "Waiting for Postgres..."
	@until docker compose exec postgres pg_isready -U showcase > /dev/null 2>&1; do sleep 1; done
	@echo "Infrastructure ready."
	@echo "  Jaeger UI:   http://localhost:16686"
	@echo "  Prometheus:  http://localhost:9090"
	@echo "  Grafana:     http://localhost:3000  (admin/admin)"

# Start the full stack (infra + app services) from Docker images.
stack:
	docker compose up -d --build
	@echo "Full stack started. API: http://localhost:8080"

down:
	docker compose down -v

# ── Code generation ───────────────────────────────────────────────────────────
proto:
	protoc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/inventory.proto
	mv proto/inventory.pb.go internal/pb/inventorypb/
	mv proto/inventory_grpc.pb.go internal/pb/inventorypb/

# ── Dependencies ──────────────────────────────────────────────────────────────
tidy:
	go mod tidy

# ── Build ─────────────────────────────────────────────────────────────────────
build:
	go build -o bin/api     ./cmd/api
	go build -o bin/grpc    ./cmd/grpc
	go build -o bin/worker  ./cmd/worker

# ── Run (requires: make dev) ──────────────────────────────────────────────────
run-api:
	go run ./cmd/api

run-grpc:
	go run ./cmd/grpc

run-worker:
	go run ./cmd/worker

# ── Tests ─────────────────────────────────────────────────────────────────────
test:
	go test ./internal/... -count=1 -race

test-integration:
	go test ./internal/... -count=1 -race -tags integration -v

# ── Lint ──────────────────────────────────────────────────────────────────────
# Requires: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
lint:
	golangci-lint run ./...

# ── Performance tests ─────────────────────────────────────────────────────────
# Requires: https://k6.io/docs/get-started/installation/
# Set JWT env var to a valid token before running.
bench:
	k6 run --env SCENARIO=load perf/load_test.js

bench-smoke:
	k6 run --env SCENARIO=smoke perf/load_test.js

bench-stress:
	k6 run --env SCENARIO=stress perf/load_test.js

# ── Database migrations ───────────────────────────────────────────────────────
# Requires: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
DB_URL ?= postgres://root:abc123456@localhost:5432/showcase?sslmode=disable

migrate-up:
	migrate -path migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path migrations -database "$(DB_URL)" down 1

migrate-status:
	migrate -path migrations -database "$(DB_URL)" version

seed:
	psql "$(DB_URL)" -f migrations/seed.sql

# ── Docker Compose validation ─────────────────────────────────────────────────
validate:
	docker compose config --quiet && echo "docker-compose.yml: OK"
	go build ./... && echo "go build: OK"
	go vet ./... && echo "go vet: OK"
	go test ./internal/... -count=1 && echo "unit tests: OK"
