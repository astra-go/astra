# Changelog

All notable changes to the Showcase reference application are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

## [0.1.0] — 2026-05-08

### Added

**Day 1-2 — Project scaffold**
- Module layout: `cmd/api`, `cmd/grpc`, `cmd/worker`, `internal/{domain,db,repository,service,handler,grpc}`
- `docker-compose.yml` with Postgres, Redis, Jaeger, Prometheus, Grafana
- `Makefile` with `dev`, `build`, `test`, `proto` targets

**Day 3-4 — ORM + multi-tenant repository**
- `db.Open` / `db.Migrate` / `db.Seed` wiring GORM with Postgres and SQLite
- Generic `TenantRepository[T]` wrapping `astra/orm.Repository[T]` with automatic `tenant_id` scoping
- `ProductRepo` and `OrderRepo` with `FindLowStock`, `FindByUser` (paginated), `DecrStock` (atomic UPDATE)

**Day 5-6 — Service layer**
- `ProductSvc` with create / get / list / update / delete
- `OrderSvc` with atomic stock reservation: validate → insert order → decrement stock → enqueue email
- `UserSvc` with JWT issuance and role management

**Day 7-8 — HTTP handlers + RBAC**
- `ProductHandler`, `OrderHandler`, `AdminHandler`, `AuthHandler` (OAuth2 Google + GitHub)
- Casbin RBAC: `admin` full access, `seller` product CRUD, `buyer` read products + place orders
- `RequireRole` middleware helper

**Day 9-10 — Redis cache**
- `CachedProductSvc` wrapping `ProductSvc` with read-through Redis cache
- Cache invalidation on update and delete
- `astra/cache` Redis adapter

**Day 11-12 — TaskQueue worker**
- `handleOrderConfirmEmail`: decodes payload, simulates SMTP, logs retry count
- `handleReportGenerate`: decodes payload, simulates report generation
- Daily report cron at 02:00 with `WithUnique` deduplication (23 h window)

**Day 13-14 — gRPC dual-stack**
- `proto/inventory.proto`: `GetStock`, `BatchGetStock`, `DecrStock`, `ListLowStock` (server-streaming)
- `InventoryServer` with per-request tenant-scoped repos; `BatchGetStock` skips missing products
- `cmd/grpc` dual-stack server: HTTP `:8081` + gRPC `:9091`
- 8 unit tests covering all RPCs including tenant isolation

**Day 15-16 — OTel + Canary**
- `astraotel.Setup` in `cmd/api` and `cmd/grpc` with OTLP export + stdout fallback
- `middleware.Tracing` with `/health` skip; `middleware.LoggerWithConfig` with `WithTraceContext: true`
- Canary middleware after JWT: header rule, cookie rule, user-hash rule (10 % of users → v2)
- v2 canary applies 5 % loyalty discount in `OrderSvc.Create`

**Day 17-18 — Integration tests + docs**
- `internal/integration/integration_test.go` with testcontainers-go Postgres container
- 6 integration tests: CRUD, tenant isolation, low-stock query, order creation, canary discount, pagination
- `README.md` with architecture diagram, quick-start, API reference, observability guide
- `API.md` with full endpoint catalogue and request/response examples

**Day 19-20 — Polish**
- `docker-compose.yml` extended with `api`, `grpc`, `worker` application services and health-gate `depends_on`
- Multi-stage `Dockerfile` with three named targets (`api`, `grpc`, `worker`) on distroless base
- Grafana datasource auto-provisioning (`config/grafana/provisioning/`)
- Prometheus scrape config updated to target containerised services
- `perf/load_test.js` k6 script: smoke / load / stress scenarios with p95 thresholds
- `CONTRIBUTING.md` with development workflow, conventions, and PR checklist
- `Makefile` extended with `lint`, `bench`, `bench-smoke`, `bench-stress`, `validate` targets
