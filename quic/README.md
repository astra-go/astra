# QUIC — HTTP/3 Support

QUIC and HTTP/3 protocol support, based on standard library `net/http` and `quic-go`.

## Features

- **HTTP/3**: Supports HTTP/3 protocol for both client and server
- **Dual Protocol Listening**: Can listen on HTTP/1.1 and HTTP/3 simultaneously
- **0-RTT**: Supports 0-RTT connection establishment (subsequent connections after first)
- **Connection Migration**: Connections survive network switches

## Quick Start

### HTTP/3 Server

```go
import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/quic"
)

app := astra.New()

// Listen on HTTP/1.1 (:8080) and HTTP/3 (:4433) simultaneously
app.RunQuic(":8080", ":4433", "server.crt", "server.key")
```

### HTTP/3 Client

```go
import "github.com/astra-go/astra/quic"

client := quic.NewClient(quic.Config{
    Addr:    "https://localhost:4433",
    Insecure: true, // For development
})

resp, err := client.Get(ctx, "/api/data")
```

## API

### app.RunQuic

```go
func (app *App) RunQuic(httpAddr, quicAddr, certFile, keyFile string) error
```

Starts HTTP/1.1 server and HTTP/3 QUIC server simultaneously.

### quic.NewClient

```go
func NewClient(cfg Config) (*Client, error)

type Config struct {
    Addr           string
    Insecure       bool              // Skip TLS verification (for development)
    HandshakeTimeout time.Duration  // Handshake timeout
}
```

### Client Methods

```go
client.Get(ctx, path)       (*http.Response, error)
client.Post(ctx, path, body)(*http.Response, error)
// ... other standard HTTP methods
```

## Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `HandshakeTimeout` | `time.Duration` | `10s` | QUIC handshake timeout |
| `KeepAlive` | `time.Duration` | `30s` | Keep-alive interval |

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/quic"
)

func main() {
    app := astra.New()

    app.GET("/api/hello", func(c *astra.Ctx) error {
        return c.String(200, "Hello via HTTP/3!")
    })

    // HTTP/1.1 on :8080, HTTP/3 on :4433
    if err := app.RunQuic(":8080", ":4433", "certs/server.crt", "certs/server.key"); err != nil {
        panic(err)
    }
}
```

## Module Dependencies

- `github.com/quic-go/quic-go` — QUIC protocol implementation
- `golang.org/x/crypto` — TLS encryption

## Notes

- HTTP/3 requires TLS certificates; self-signed certificates only for development
- Configure appropriate KeepAlive on client to prevent connection timeout closure
- HTTP/3 downgrade: clients that don't support HTTP/3 auto-fallback to HTTP/1.1/2
