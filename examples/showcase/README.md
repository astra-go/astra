# Showcase — Astra Production Reference Application

A full-featured e-commerce / SaaS demo that shows how to combine every major
Astra sub-module in a single, production-ready application.

## What it demonstrates

| Feature | Module | Where |
|---------|--------|-------|
| Multi-tenant ORM + AutoMigrate | `astra/orm` | `internal/db`, `internal/repository` |
| Generic `Repository[T]` + tenant scoping | `astra/orm` | `internal/repository/tenant_repo.go` |
| Atomic stock decrement (no TOCTOU) | `astra/orm` | `internal/repository/repos.go` |
| Read-through Redis cache | `astra/cache` | `internal/service/cached_product_svc.go` |
| Async task queue (order emails, reports) | `astra/taskqueue` | `internal/service/order_svc.go`, `cmd/worker` |
| Casbin RBAC (admin / seller / buyer) | `astra/auth/rbac` | `config/rbac_*.{conf,csv}`, `cmd/api/main.go` |
| OAuth2 login (Google + GitHub) | `astra/auth/oauth2` | `internal/handler/auth_handler.go` |
| JWT issuance + middleware | `astra/middleware` | `internal/service/user_svc.go` |
| gRPC dual-stack (HTTP :8081 + gRPC :9091) | `astra/grpc` | `cmd/grpc`, `internal/grpc` |
| OTel tracing + Prometheus metrics | `astra/otel` | `cmd/api/main.go`, `cmd/grpc/main.go` |
| Canary / feature-flag middleware | `astra/middleware` | `cmd/api/main.go` |
| `TxMiddleware` for atomic order creation | `astra/orm` | `cmd/api/main.go` |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  cmd/api  (HTTP :8080)          cmd/grpc  (HTTP :8081 + gRPC :9091)
│     │                                │
│  middleware chain                middleware chain
│  RequestID → Tracing → Logger    RequestID → Tracing → Logger
│  Recovery → CORS → JWT → RBAC    Recovery
│  Canary                               │
│     │                           grpcserver.New
│  handlers                       InventoryService (gRPC)
│  ProductHandler                       │
│  OrderHandler ──TxMiddleware──►  productRepo
│  AdminHandler                         │
│  AuthHandler (OAuth2)                 ▼
│     │                           PostgreSQL
│     ▼
│  services
│  CachedProductSvc ──► Redis
│  OrderSvc ──────────► TaskQueue ──► cmd/worker
│  UserSvc ───────────► JWT
│     │
│     ▼
│  repositories (TenantRepository[T])
│     │
│     ▼
│  PostgreSQL
└─────────────────────────────────────────────────────────────┘

Observability: Jaeger (traces) + Prometheus + Grafana
```

## Quick start

```bash
# 1. Start infrastructure
docker compose up -d

# 2. Run the API server
go run ./cmd/api

# 3. (Optional) Run the gRPC dual-stack server
go run ./cmd/grpc

# 4. (Optional) Run the background worker
go run ./cmd/worker
```

Jaeger UI: http://localhost:16686  
Grafana:   http://localhost:3000 (admin/admin)  
Prometheus: http://localhost:9090

## API reference

### Public endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/auth/google/login` | Redirect to Google OAuth2 |
| GET | `/auth/google/callback` | Google OAuth2 callback → JWT |
| GET | `/auth/github/login` | Redirect to GitHub OAuth2 |
| GET | `/auth/github/callback` | GitHub OAuth2 callback → JWT |

### Protected endpoints (Bearer JWT required)

#### Products

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/products` | buyer, seller, admin | List products (page 1 cached) |
| POST | `/api/v1/products` | seller, admin | Create product |
| GET | `/api/v1/products/:id` | buyer, seller, admin | Get product |
| PUT | `/api/v1/products/:id` | seller, admin | Update product |
| DELETE | `/api/v1/products/:id` | seller, admin | Delete product |

#### Orders

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/orders` | buyer, seller, admin | List orders |
| GET | `/api/v1/orders/:id` | buyer, seller, admin | Get order with items |
| POST | `/api/v1/orders` | buyer, admin | Create order (atomic stock decrement) |

#### Admin

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/admin/users/:id` | admin | Get user |
| PUT | `/api/v1/admin/users/:id/role` | admin | Update user role |

### gRPC services (`:9091`)

```protobuf
service InventoryService {
  rpc GetStock(GetStockRequest) returns (StockResponse);
  rpc BatchGetStock(BatchGetStockRequest) returns (BatchStockResponse);
  rpc DecrStock(DecrStockRequest) returns (StockResponse);
  rpc ListLowStock(ListLowStockRequest) returns (stream StockItem);
}
```

Use `grpcurl` or Evans to explore:

```bash
grpcurl -plaintext localhost:9091 list
grpcurl -plaintext -d '{"product_id":1,"tenant_id":1}' \
  localhost:9091 showcase.inventory.v1.InventoryService/GetStock

# Batch query
grpcurl -plaintext -d '{"tenant_id":1,"product_ids":[1,2,3]}' \
  localhost:9091 showcase.inventory.v1.InventoryService/BatchGetStock

# Server-streaming low-stock alert
grpcurl -plaintext -d '{"tenant_id":1,"threshold":10}' \
  localhost:9091 showcase.inventory.v1.InventoryService/ListLowStock
```

## Configuration

All settings are read from environment variables with sensible defaults for local development.

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://showcase:showcase@localhost:5432/showcase?sslmode=disable` | PostgreSQL DSN |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `JWT_SECRET` | `change-me-in-production` | HS256 signing key |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | _(empty, disabled)_ | OTLP gRPC endpoint (e.g. `localhost:4317`) |
| `OTEL_STDOUT` | _(empty)_ | Set to any value to print spans to stdout |
| `GOOGLE_CLIENT_ID` | — | Google OAuth2 client ID |
| `GOOGLE_CLIENT_SECRET` | — | Google OAuth2 client secret |
| `GITHUB_CLIENT_ID` | — | GitHub OAuth2 client ID |
| `GITHUB_CLIENT_SECRET` | — | GitHub OAuth2 client secret |
| `HTTP_ADDR` | `:8081` | HTTP listen address (cmd/grpc only) |
| `GRPC_ADDR` | `:9091` | gRPC listen address (cmd/grpc only) |

## Running tests

```bash
# Unit + repository tests (SQLite in-memory, no external deps)
go test ./...

# With race detector
go test -race ./...

# Integration tests (requires Docker)
go test -tags integration -v ./internal/integration/...
```

Integration test coverage:

| Test | What it verifies |
|------|-----------------|
| `TestProductRepo_CRUD_Postgres` | Full create/read/update/delete lifecycle |
| `TestProductRepo_TenantIsolation_Postgres` | Cross-tenant access is blocked |
| `TestProductRepo_FindLowStock_Postgres` | Threshold query returns correct rows |
| `TestOrderSvc_Create_Postgres` | Order creation decrements stock atomically |
| `TestOrderSvc_Create_CanaryDiscount_Postgres` | v2 canary applies 5 % loyalty discount |
| `TestOrderSvc_Create_FindAll_Pagination_Postgres` | Pagination returns correct page size |

## Project layout

```
examples/showcase/
├── cmd/
│   ├── api/        # HTTP-only entry point (REST + OAuth2 + RBAC)
│   ├── grpc/       # Dual-stack HTTP+gRPC entry point
│   └── worker/     # Background task worker
├── config/
│   ├── rbac_model.conf
│   └── rbac_policy.csv
├── internal/
│   ├── db/         # Open, Migrate, Seed
│   ├── domain/     # Entities + DTOs
│   ├── grpc/       # gRPC service implementations
│   ├── handler/    # HTTP handlers + auth handler
│   ├── integration/# Testcontainers integration tests
│   ├── pb/         # Generated protobuf code
│   ├── repository/ # TenantRepository[T] + concrete repos
│   └── service/    # Business logic + cache wrapper
├── proto/          # .proto source files
├── docker-compose.yml
└── Makefile
```

## RBAC roles

| Role | Products | Orders | Admin |
|------|----------|--------|-------|
| `admin` | Full CRUD | Full CRUD | Yes |
| `seller` | GET + POST + PUT + DELETE | GET only | No |
| `buyer` | GET only | GET + POST | No |

New OAuth2 users are assigned the `buyer` role automatically. Admins can
promote users via `PUT /api/v1/admin/users/:id/role`.

## Canary deployment

10% of users (user_id % 10 == 0) and requests with `X-Canary: true` are
routed to the `v2` checkout flow. The canary version is stored in the request
context and can be read by handlers to serve different behaviour.
