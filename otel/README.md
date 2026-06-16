# OTel — OpenTelemetry Integration

OpenTelemetry distributed tracing and metrics integration, supports OTLP export.

## Features

- **Auto Tracing**: HTTP/gRPC requests auto-create Spans
- **OTLP Export**: Supports exporting Trace and Metrics to OTEL Collector
- **Logging Integration**: `SpanExtractor` injects Trace ID into logs
- **Propagation Protocols**: W3C TraceContext and B3 format auto-inject and extract
- **Metrics Export**: Prometheus format and OTLP dual export

## Quick Start

```go
import (
    "context"
    astraotel "github.com/astra-go/astra/otel"
    "github.com/astra-go/astra/log"
)

ctx := context.Background()

shutdown, err := astraotel.Setup(ctx, astraotel.Config{
    ServiceName:  "my-service",
    ServiceVersion: "1.0.0",
    Exporter:    astraotel.ExporterOTLP,
    Endpoint:    "http://otel-collector:4318",
})
defer shutdown()

// Enable logging integration
log.SetSpanExtractor(astraotel.SpanExtractor)
```

## API

### Setup

```go
func Setup(ctx context.Context, cfg Config) (func(), error)
```

Initializes OpenTelemetry, returns shutdown function (call on app exit).

### Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `ServiceName` | `string` | — | Service name (required) |
| `ServiceVersion` | `string` | `""` | Service version |
| `Exporter` | `string` | `ExporterOTLP` | Export method |
| `Endpoint` | `string` | — | Collector address |
| `Insecure` | `bool` | `true` | Use TLS or not |

### Exporter Types

| Constant | Description |
|----------|-------------|
| `ExporterOTLP` | OTLP over HTTP (recommended) |
| `ExporterStdout` | Stdout (for debugging) |
| `ExporterJaeger` | Jaeger native protocol |

### SpanExtractor

```go
// Inject Trace/Span ID into logs
func SpanExtractor(ctx context.Context) (traceID, spanID string)
```

## Complete Example

```go
package main

import (
    "context"
    "github.com/astra-go/astra/otel"
    "github.com/astra-go/astra/log"
)

func main() {
    ctx := context.Background()

    // Initialize OTel
    shutdown, _ := otel.Setup(ctx, otel.Config{
        ServiceName:  "order-service",
        ServiceVersion: "1.0.0",
        Exporter:    otel.ExporterOTLP,
        Endpoint:    "http://localhost:4318",
        Insecure:    true,
    })
    defer shutdown()

    // Enable log Trace ID injection
    log.SetSpanExtractor(otel.SpanExtractor)

    log.Info("application started")
}
```

## Module Dependencies

- `go.opentelemetry.io/otel` — OTel core
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` — OTLP HTTP exporter

## Notes

- `Setup` must be called before creating HTTP/gRPC servers
- Call `shutdown` function on app exit to ensure data is flushed
- In production, `Insecure` should be `false` and TLS certificates configured
