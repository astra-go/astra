# Stream — Streaming Processing

Supports Server-Sent Events (SSE), client streaming, and bidirectional WebSocket streaming.

## Features

- **Server-Sent Events (SSE)**: Server push via `/events` endpoint for real-time data
- **Client Streaming**: Client uploads data stream, server receives and returns result
- **Bidirectional Streaming (Bidirectional)**: WebSocket full-duplex communication
- **Per-Stream Rate Limiting**: Each stream independently rate-limited
- **Serialization**: JSON (default) and other custom serializers

## Quick Start

### Server Push (SSE)

```go
app.GET("/events", stream.ServerStreamHandler(func(s contract.ServerStream) error {
    for _, item := range feed.Items() {
        if err := s.Send(item); err != nil {
            return err
        }
    }
    return nil
}))
```

### Client Streaming (File Upload)

```go
app.POST("/upload", stream.ClientStreamHandler(func(s contract.ClientStream) error {
    var total int
    for {
        var chunk Chunk
        if errors.Is(err, io.EOF) { break }
        if err != nil { return err }
        total += len(chunk.Data)
    }
    return s.SendAndClose(Result{Total: total})
}))
```

### Bidirectional Streaming (Real-Time Chat)

```go
app.GET("/ws/chat", stream.BidiHandler(func(s contract.BidiStream) error {
    for {
        var msg Message
        if err := s.Recv(&msg); errors.Is(err, io.EOF) {
            return nil
        } else if err != nil {
            return err
        }
        // Broadcast message
        broadcast(msg)
    }
}))
```

## API

### ServerStreamHandler

```go
func ServerStreamHandler(fn func(contract.ServerStream) error) astra.HandlerFunc

// contract.ServerStream interface:
type ServerStream interface {
    Context
    Send(v any) error       // Send data (JSON serialized)
    Done() <-chan struct{} // Stream end signal
}
```

### ClientStreamHandler

```go
func ClientStreamHandler(fn func(contract.ClientStream) error) astra.HandlerFunc

// contract.ClientStream interface:
type ClientStream interface {
    Context
    Recv(v any) error                  // Receive next message
    SendAndClose(v any) error          // Send result and close stream
    Done() <-chan struct{}
}
```

### BidiHandler

```go
func BidiHandler(fn func(contract.BidiStream) error) astra.HandlerFunc

// contract.BidiStream interface:
type BidiStream interface {
    Context
    Send(v any) error
    Recv(v any) error
    Done() <-chan struct{}
}
```

### Options

```go
type Options struct {
    Codec   astra.Serializer  // Serializer (default JSON)
    RateLimit *RateLimitConfig // Rate limiting
}

type RateLimitConfig struct {
    Rate  int // Messages per second
    Burst int // Burst capacity
}
```

## Config

### Global Stream Config

```go
import "github.com/astra-go/astra/stream"

app := astra.New(
    astra.WithStream(stream.Options{
        RateLimit: &stream.RateLimitConfig{
            Rate:  100, // Max 100 messages per second
            Burst: 200,
        },
    }),
)
```

### Custom Codec

```go
import (
    "github.com/msgpack/msgpack"
)

codec := msgpackSerializer{} // Implement astra.Serializer interface
stream.NewServerStreamHandler(fn, stream.Options{Codec: codec})
```

## Complete Example

```go
package main

import (
    "errors"
    "io"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/stream"
)

type Message struct{ Text string }

func main() {
    app := astra.New()

    // SSE real-time event push
    app.GET("/events", stream.ServerStreamHandler(func(s contract.ServerStream) error {
        for i := 0; i < 10; i++ {
            if err := s.Send(map[string]int{"count": i}); err != nil {
                return err
            }
        }
        return nil
    }))

    // Client stream processing
    app.POST("/upload", stream.ClientStreamHandler(func(s contract.ClientStream) error {
        var total int
        for {
            var chunk Message
            err := s.Recv(&chunk)
            if errors.Is(err, io.EOF) { break }
            if err != nil { return err }
            total += len(chunk.Text)
        }
        return s.SendAndClose(map[string]int{"bytes_received": total})
    }))

    app.Run(":8080")
}
```

## Module Dependencies

- `github.com/gorilla/websocket` — WebSocket support

## Notes

- SSE is compatible with most modern browsers but not IE; use polling as fallback for IE compatibility
- `s.Recv` returns `io.EOF` when client has closed the sending side
- `Done()` channel closes when stream ends, can be used for graceful client disconnect detection
