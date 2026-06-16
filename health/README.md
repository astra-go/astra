# Health — Health Checks and Readiness Probes

Provides HTTP health check endpoints, custom probes, and Kubernetes/Istio-compatible liveness/readiness probes.

## Features

- **Health Check Endpoints**: `/health` (liveness probe) and `/ready` (readiness probe)
- **Custom Probes**: Supports registering arbitrary check functions (database, Redis, external services)
- **K8s Compatible**: Supports Kubernetes `/healthz/live` and `/healthz/ready` paths
- **Istio Compatible**: Supports Istio `istio.health` annotations
- **Grouped Checks**: Checks grouped by service component; each group independently determines status

## Quick Start

```go
import "github.com/astra-go/astra/health"

app := astra.New()

// Register custom probes
app.Register(health.New(
    health.WithProbe("mysql", func(ctx context.Context) error {
        return db.PingContext(ctx)
    }),
    health.WithProbe("redis", func(ctx context.Context) error {
        return redis.Ping(ctx).Err()
    }),
    health.WithProbe("downstream", func(ctx context.Context) error {
        _, err := http.Get("http://downstream/health")
        return err
    }),
))

app.GET("/health", health.Handler())
app.GET("/ready", health.ReadyHandler())
```

## API

### New

```go
func New(opts ...Option) *Health
```

`Option` supports chainable registration.

### WithProbe

```go
func WithProbe(name string, fn health.CheckFunc) Option

type CheckFunc func(ctx context.Context) error
// nil return = healthy, non-nil = unhealthy
```

### Handler / ReadyHandler

```go
// /health — Liveness probe (is app alive), checks critical dependencies
func Handler() astra.HandlerFunc

// /ready — Readiness probe (can receive traffic), checks all dependencies
func ReadyHandler() astra.HandlerFunc
```

### WithChecker (Advanced)

```go
// Add a complete Checker (not single function)
func WithChecker(name string, c Checker) Option

type Checker interface {
    Check(ctx context.Context) error // nil = healthy
}
```

## Config

### Probe Timeout

Default probe timeout is 5 seconds; use context with timeout in `CheckFunc`:

```go
health.WithProbe("slow-service", func(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    return service.Ping(ctx)
})
```

## Complete Example

```go
package main

import (
    "context"
    "net/http"
    "time"

    "github.com/astra-go/astra"
    "github.com/astra-go/astra/health"
)

func main() {
    app := astra.New()

    h := health.New(
        // Basic liveness: is app running
        health.WithProbe("app", func(ctx context.Context) error {
            return nil
        }),
        // Readiness: database connection
        health.WithProbe("db", func(ctx context.Context) error {
            return db.PingContext(ctx)
        }),
        // Readiness: cache
        health.WithProbe("cache", func(ctx context.Context) error {
            return redis.Ping(ctx).Err()
        }),
    )

    // Register routes
    app.GET("/health", h.Handler())      // Liveness (checks app only)
    app.GET("/ready", h.ReadyHandler())  // Readiness (checks all)

    app.Run(":8080")
}
```

## Kubernetes Deployment Config Example

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
```

## Module Dependencies

No external dependencies.

## Notes

- `/health` only checks app itself (should always return 200), `/ready` checks all dependencies
- Probe functions should return quickly to avoid probe timeout affecting judgment
- When readiness probe returns non-200, Kubernetes removes Pod from Service until recovery
