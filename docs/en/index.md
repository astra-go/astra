# Astra — Go Web Framework

Astra is a high-performance, full-featured web framework for Go developers, distilling the best ideas from Gin, Echo, Kratos, go-zero, and more.

## Feature Highlights

| Category | Features |
|----------|----------|
| **Network** | Reactor engine (epoll/kqueue), HTTP/3 QUIC |
| **Auth** | JWT · API Key · RBAC · OAuth2/OIDC |
| **Observability** | OpenTelemetry traces + metrics + logs · Prometheus |
| **Data** | GORM · Redis · MongoDB · ClickHouse · Elasticsearch |
| **Message Queue** | NATS · Kafka · RabbitMQ · Redis Streams · SQS · Pulsar |
| **Microservices** | gRPC dual-stack · Service discovery · Load balancing · Circuit breaker · Saga transactions |
| **Infrastructure** | Config center · Distributed locks · Session · Object storage · Alert engine |

## Quick Start

```go
package main

import "github.com/astra-go/astra"

func main() {
    app := astra.New()
    app.GET("/ping", func(c *astra.Context) error {
        return c.JSON(200, astra.H{"message": "pong"})
    })
    app.Run(":8080")
}
```

## Documentation Navigation

- [Getting Started](getting-started/installation.md) — Installation, first application
- [API Reference](api/core.md) — Complete API documentation
- [Versioning](versioning.md) — SemVer rules, support lifecycle
- [Migration Guide](migration/overview.md) — Cross-version upgrade guide
- [Changelog](changelog.md) — Full change history

## Version Status

| Version | Status | Go Requirement | Maintained Until |
|---------|--------|----------------|-----------------|
| **v1.0** (latest) | :white_check_mark: Actively maintained | ≥ 1.22 | 2027-04 |
| v0.10 | :wrench: Security fixes only | ≥ 1.22 | 2026-10 |
| v0.9  | :wrench: Security fixes only | ≥ 1.21 | 2026-08 |
| ≤ v0.8 | :x: End of life | — | — |
