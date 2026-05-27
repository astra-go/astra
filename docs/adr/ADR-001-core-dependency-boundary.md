# ADR-001: Core Module Dependency Boundary

## Status
Accepted — implemented 2026-05-27

## Date
2026-05-26

## Context
The core `github.com/astra-go/astra` module currently directly requires several sub-modules with heavy third-party dependencies:
- `middleware/security` → pulls in `golang-jwt/jwt`, `go-redis`
- `middleware/observability` → pulls in OpenTelemetry SDK, Prometheus client
- `discovery` → pulls in service discovery logic
- `quic-go` → HTTP/3 support, ~40 transitive dependencies
- `expr-lang/expr` → rule engine expression evaluator

This contradicts the "lightweight core" promise. Users who only want the router must download JWT, OTel, Redis, and other heavy deps.

## Decision
**Core module MUST NOT directly require any sub-module that has heavy third-party dependencies.** Instead:
1. Core should only depend on lightweight utilities
2. Heavy integrations (OTel, GORM, MQ, Redis, gRPC, etc.) live in their own sub-modules
3. Users install sub-modules explicitly when needed: `go get github.com/astra-go/astra/middleware/security`
4. Core examples may demonstrate sub-module usage but must not make them required for core functionality

## Options Considered

### Option A: Keep Status Quo
- **Pros**: Convenient for testing, unified API
- **Cons**: Dependency pollution, violates lightweight promise

### Option B: Core Depends on Zero Sub-Modules
- **Pros**: Truly lightweight, users pick and choose
- **Cons**: Testing requires restructuring, breaking change

### Option C: Only testutil as test dependency
- **Pros**: Balanced approach
- **Cons**: Requires go.mod to distinguish test vs. main dependencies (not natively supported)

## Decision
**Option B: Core depends on zero heavy sub-modules.**

All sub-modules with third-party dependencies become truly optional. Core examples will demonstrate how to use sub-modules, but they are not bundled with core.

## Implementation (2026-05-27)

### Changes made

**P0 — Removed heavy direct deps from core go.mod:**
- Removed `github.com/prometheus/client_golang` and `github.com/prometheus/client_model` (belonged to `otel/` and `observability/` sub-modules)
- Removed `github.com/golang-jwt/jwt/v5` (belonged to `middleware/security/`)
- Removed `go.opentelemetry.io/otel/trace` (belonged to `otel/`)
- Removed `modernc.org/sqlite` (test-only, used only in `migrate/migrate_test.go`)
- Removed `github.com/quic-go/quic-go` (moved to new `quic/` sub-module)

**P1 — Decoupled `log/log.go` from `otel/trace`:**
- Replaced hard `go.opentelemetry.io/otel/trace` import with an injectable `SpanExtractor` function type
- Added `log.SetSpanExtractor(fn SpanExtractor)` for opt-in OTel integration
- Added `otel.SpanExtractor` in the `otel/` sub-module as the concrete implementation
- Users wire it at startup: `log.SetSpanExtractor(astraotel.SpanExtractor)`

**P2 — Extracted `app_quic.go` into `quic/` sub-module:**
- Created `github.com/astra-go/astra/quic` sub-module with its own `go.mod`
- Moved HTTP/3 server logic to `quic/quic.go` with public `RunQUIC(app, addr, cert, key)` function
- Added `App.Start(ctx)`, `App.Stop(ctx)`, and `App.ShutdownTimeout()` public methods to core for external server lifecycle integration
- Removed `app_quic.go` and `app_quic_test.go` from core

**P3 — Annotated remaining test-only deps:**
- `middleware/security`, `middleware/observability`, `golang-jwt/jwt`, `prometheus/client_golang`, `modernc.org/sqlite` marked `// test-only` in go.mod
- These are used only in `_test.go` files within the core module; the `CheckTestDeps` mage target recognizes this annotation

**Examples isolation:**
- Created independent `go.mod` for `examples/basic`, `examples/cache`, `examples/jwt`, `examples/quickstart`, `examples/websocket`
- These examples previously pulled sub-module deps into the core module's dependency graph

**Test migration:**
- Moved Metrics/Tracing tests from `middleware/logger_metrics_tracing_test.go` → `middleware/observability/metrics_tracing_test.go`
- Moved SlidingWindow/RouteQuota tests → `middleware/security/ratelimit_test.go`
- Logger-only tests remain in `middleware/logger_test.go` (no heavy deps)

## Consequences
- **Breaking**: Users upgrading may need to update their `go.mod` if they relied on transitively-included deps
- **Documentation**: Must clearly document how to install optional sub-modules
- **CI**: Add depguard rules to prevent new heavy deps from entering core

## References
- ADR-002: Module/Plugin Interface Unification
- ADR-003: Rule Package Ownership
