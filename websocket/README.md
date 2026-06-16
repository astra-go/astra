# WebSocket — WebSocket Support

WebSocket support based on `gorilla/websocket`, providing Upgrader and streaming read/write helpers.

## Features

- **Standard Upgrader**: Upgrades HTTP connection to WebSocket connection
- **Read/Write Separation**: `ReadMessage`/`WriteMessage` supports text and binary frames
- **Read/Write Timeout**: Supports setting read/write timeout to prevent connection freeze
- **Heartbeat**: Supports Ping/Pong heartbeat to keep connection alive
- **Connection Pool**: Supports concurrent read/write

## Quick Start

### Basic Usage

```go
import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/websocket"
)

app.GET("/ws", func(c *astra.Ctx) error {
    conn, err := websocket.Upgrade(c.Request().Writer, c.Request().Request)
    if err != nil {
        return err
    }
    defer conn.Close()

    for {
        msgType, msg, err := conn.ReadMessage()
        if err != nil {
            return nil
        }
        conn.WriteMessage(msgType, msg)
    }
})
```

### With Heartbeat

```go
app.GET("/ws", func(c *astra.Ctx) error {
    conn, err := websocket.Upgrade(c.Request().Writer, c.Request().Request)
    if err != nil {
        return err
    }
    defer conn.Close()

    conn.SetReadLimit(512)
    conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    conn.SetPongHandler(func(string) error {
        conn.SetReadDeadline(time.Now().Add(60 * time.Second))
        return nil
    })

    // Send Ping regularly
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        for range ticker.C {
            if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }()

    for {
        _, msg, err := conn.ReadMessage()
        if err != nil {
            return nil
        }
        conn.WriteMessage(websocket.TextMessage, msg)
    }
})
```

## API

### websocket.Upgrade

```go
func Upgrade(w http.ResponseWriter, r *http.Request) (*gorilla.Conn, error)
```

Upgrades HTTP request to WebSocket connection.

### gorilla.Conn Methods

| Method | Description |
|--------|-------------|
| `ReadMessage()` | Read next message, returns (type, []byte, error) |
| `WriteMessage(type, data)` | Send message |
| `Close()` | Close connection |
| `SetReadLimit(n int64)` | Set max read bytes |
| `SetReadDeadline(t time.Time)` | Set read timeout |
| `SetWriteDeadline(t time.Time)` | Set write timeout |
| `SetPongHandler(fn)` | Set Pong response handler |
| `SetPingHandler(fn)` | Set Ping handler |

### Message Types

```go
websocket.TextMessage   // Text frame
websocket.BinaryMessage  // Binary frame
websocket.CloseMessage  // Close frame
websocket.PingMessage   // Ping frame
websocket.PongMessage   // Pong frame
```

## Config

### Upgrader Config

```go
upgrader := gorilla.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return r.Header.Get("Origin") == "https://example.com"
    },
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}
conn, _ := upgrader.Upgrade(w, r, nil)
```

## Complete Example

```go
package main

import (
    "fmt"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/websocket"
    "log"
    "net/http"
)

func main() {
    app := astra.New()

    app.GET("/ws", func(c *astra.Ctx) error {
        conn, err := websocket.Upgrade(c.Request().Writer, c.Request().Request)
        if err != nil {
            return err
        }
        defer conn.Close()

        for {
            msgType, msg, err := conn.ReadMessage()
            if err != nil {
                log.Printf("read error: %v", err)
                break
            }
            fmt.Printf("Received: %s\n", msg)
            if err := conn.WriteMessage(msgType, msg); err != nil {
                log.Printf("write error: %v", err)
                break
            }
        }
        return nil
    })

    app.Run(":8080")
}
```

## Module Dependencies

- `github.com/gorilla/websocket` — WebSocket core library

## Notes

- `CheckOrigin` recommended in production to prevent WebSocket hijacking attacks
- Ping/Pong heartbeat is the recommended way to keep connections alive behind load balancers
- Set read/write timeouts for long idle periods to avoid connection leaks
