# Log — Structured Logging

Structured logging based on Go standard library `log/slog`, supporting JSON/Text dual format, multi-output, and OpenTelemetry context injection.

## Features

- **Structured Logging**: Key-value pair recording, natural JSON format support
- **Dual Format**: `json` (recommended for production) or `text` (development-friendly)
- **Multi-Output**: Write to multiple Writers simultaneously (stdout + file)
- **Context Propagation**: Supports extracting Trace ID/Span ID and injecting into logs
- **OTel Integration**: `SetSpanExtractor` auto-injects distributed tracing context after registration

## Quick Start

```go
import "github.com/astra-go/astra/log"

// Initialize global logger
log.SetDefault(log.New(log.Config{
    Level:  log.LevelInfo,
    Format: "json", // "json" or "text"
    Output: os.Stdout,
}))

log.Info("server started", "addr", ":8080", "pid", os.Getpid())
log.Warn("slow query", "duration_ms", 1500, "sql", "SELECT ...")
log.Error("request failed", "err", err, "path", "/api/users")
```

## API

### log.New

```go
func New(cfg Config) *slog.Logger
```

### Config

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Level` | `Level` | `LevelInfo` | Min log level |
| `Format` | `string` | `"text"` | `"json"` or `"text"` |
| `Output` | `io.Writer` | — | Single output (ignored if `Outputs` set) |
| `Outputs` | `[]io.Writer` | — | Multiple output targets |
| `AddSource` | `bool` | `false` | Whether to record source location |

### log.SetDefault

```go
func SetDefault(l *slog.Logger)
```

Sets global default logger. Subsequent `log.Info`/`log.Warn` etc. use this logger.

### log.SetSpanExtractor

```go
func SetSpanExtractor(fn func(ctx context.Context) (traceID, spanID string))
```

Registers OpenTelemetry trace context extractor. After registration, logs auto-carry `trace_id` and `span_id`:

```go
import astraotel "github.com/astra-go/astra/otel"
log.SetSpanExtractor(astraotel.SpanExtractor)
```

## Config

### Multi-Output Example

```go
f, _ := os.Create("app.log")
log.New(log.Config{
    Level:   log.LevelDebug,
    Format:  "json",
    Outputs: []io.Writer{os.Stdout, f}, // Write to console and file simultaneously
})
```

### Development/Production Config

```go
// Development
log.New(log.Config{Level: log.LevelDebug, Format: "text"})

// Production
log.New(log.Config{Level: log.LevelInfo, Format: "json"})
```

## Complete Example

```go
package main

import (
    "os"
    "github.com/astra-go/astra/log"
)

func main() {
    // Write to stdout and file simultaneously
    f, _ := os.Create("app.log")
    logger := log.New(log.Config{
        Level:   log.LevelDebug,
        Format:  "json",
        Outputs: []io.Writer{os.Stdout, f},
    })

    log.SetDefault(logger)

    log.Info("app starting",
        "version", "1.0.0",
        "env", "production",
    )

    log.With("request_id", "abc123").Warn("rate limit approaching",
        "current_rpm", 980,
    )

    log.Error("database connection lost",
        "err", "connection refused",
        "host", "localhost",
    )
}
```

## Module Dependencies

- `go.opentelemetry.io/otel` — Distributed tracing context extraction (optional)

## Notes

- Log levels follow `slog` spec: Debug < Info < Warn < Error
- Production recommends `json` format for easy parsing by log collection systems (ELK/Loki)
- `SetSpanExtractor` only needs to be called once at startup; ensure OpenTelemetry is initialized first
- `log.With` returns a derived logger with extra context, not modifying the original logger
