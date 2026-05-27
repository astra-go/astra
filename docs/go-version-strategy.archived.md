> **ARCHIVED 2026-05-27** — This was a migration analysis document exploring a downgrade to Go 1.22.
> The project decided to stay on Go 1.25.1 (ADR-004 Accepted). See `docs/adr/ADR-004-go-version-strategy.md`.

# Go Version Compatibility Strategy

## Current State

- Root `go.mod`: `go 1.25.1`
- All submodules: `go 1.25.x`
- **Root cause**: 3 core packages import OpenTelemetry directly

## Dependency Chain Analysis

### Core module's otel dependency

These packages are in the **root go.mod** and import otel directly:

| Package | otel imports | Can be moved out? |
|---------|-------------|-------------------|
| `middleware/tracing.go` | otel, attribute, codes, propagation, semconv, trace | ✅ → `observability` submodule |
| `middleware/logger.go` | otel/trace only | ✅ → conditional compile or interface |
| `log/log.go` | otel/trace only | ✅ → conditional compile or interface |

### Minimum viable Go 1.22 dependency versions

| Dependency | Current | Go 1.22 compatible | Notes |
|-----------|---------|-------------------|-------|
| `go.opentelemetry.io/otel` | v1.42.0 (go 1.25) | v1.34.0 (go 1.22) | Last version supporting 1.22 |
| `golang.org/x/net` | v0.51.0 (go 1.25) | v0.33.0 (go 1.22) | Transitive via otel |
| `go.etcd.io/etcd/client/v3` | v3.6.10 (go 1.25) | v3.5.16 (go 1.21) | Only in discovery/ submodule |
| `github.com/hashicorp/consul/api` | v1.34.x (go 1.25) | v1.28.3 (go 1.22) | Only in discovery/ submodule |
| `k8s.io/client-go` | v0.32.3 (go 1.23) | v0.31.3 (go 1.22) | Only in discovery/ submodule |

## Strategy: Decouple Core from OTel

### Phase 1: Extract tracing middleware to submodule (recommended)

Move `middleware/tracing.go` → `observability/tracing.go` (already a submodule).

Core `middleware/` only keeps a lightweight `tracing_stub.go` with build tags:

```go
//go:build !otel

package middleware

// Tracing is a no-op when OTel is not imported.
func Tracing() contract.MiddlewareFunc {
    return func(next contract.Handler) contract.Handler {
        return next
    }
}
```

```go
//go:build otel

package middleware

import "github.com/astra-go/astra/observability"

func Tracing() contract.MiddlewareFunc {
    return observability.Tracing()
}
```

### Phase 2: Logger/Log otel/trace decoupling

Replace direct `otel/trace` imports with a minimal `TraceContext` interface:

```go
// log/tracecontext.go
type TraceContext interface {
    TraceID() string
    SpanID() string
    IsSampled() bool
}
```

Provide otel implementation in `observability/` and a no-op in core.

### Phase 3: Downgrade dependencies

After decoupling:
1. Root `go.mod`: otel v1.34.0, x/net v0.33.0 → `go 1.22.0`
2. `discovery/`: etcd v3.5.16, consul v1.28.3, k8s v0.31.3 → `go 1.22.0`
3. All other submodules: `go 1.22.0`

### Phase 4: Remove discovery/ from root go.mod

Move `discovery/` from root `require` → only import via users who need it.
Root module should not transitively depend on etcd/consul/k8s.

## Estimated Effort

| Phase | Work | Risk | Timeline |
|-------|------|------|----------|
| Phase 1 | Move 1 file, add 2 stub files | Low | 1-2 hours |
| Phase 2 | Interface extraction, ~5 files | Medium | 2-4 hours |
| Phase 3 | go.mod downgrade + tidy | Low | 30 min |
| Phase 4 | Remove discovery dep | Low | 15 min |

## Alternative: Keep Go 1.25, Document Clearly

If decoupling is deferred:
- Document minimum Go version prominently in README
- Add `//go:build go1.25` tags where 1.25 features are used
- Track otel v1.34.0 compatibility in a branch for future migration
