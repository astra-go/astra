# WebSocket — WebSocket 支持

基于 `gorilla/websocket` 的 WebSocket 支持，提供 Upgrader 和流式读写辅助工具。

## 特性

- **标准 Upgrader**：将 HTTP 连接升级为 WebSocket 连接
- **读写分离**：`ReadMessage`/`WriteMessage` 支持文本和二进制帧
- **读写超时**：支持设置读写超时，防止连接冻结
- **心跳**：支持 Ping/Pong 心跳保持连接活跃
- **连接池**：支持并发读写

## 快速开始

### 基础用法

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

### 心跳示例

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

    // 定期发送 Ping
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

将 HTTP 请求升级为 WebSocket 连接。

### gorilla.Conn 方法

| 方法 | 说明 |
|--------|-------------|
| `ReadMessage()` | 读取下一条消息，返回 (type, []byte, error) |
| `WriteMessage(type, data)` | 发送消息 |
| `Close()` | 关闭连接 |
| `SetReadLimit(n int64)` | 设置最大读取字节数 |
| `SetReadDeadline(t time.Time)` | 设置读取超时 |
| `SetWriteDeadline(t time.Time)` | 设置写入超时 |
| `SetPongHandler(fn)` | 设置 Pong 响应处理器 |
| `SetPingHandler(fn)` | 设置 Ping 处理器 |

### 消息类型

```go
websocket.TextMessage   // 文本帧
websocket.BinaryMessage  // 二进制帧
websocket.CloseMessage  // 关闭帧
websocket.PingMessage   // Ping 帧
websocket.PongMessage   // Pong 帧
```

## 配置

### Upgrader 配置

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

## 完整示例

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

## 模块依赖

- `github.com/gorilla/websocket` — WebSocket 核心库

## 注意事项

- 生产环境建议配置 `CheckOrigin` 防止 WebSocket 劫持攻击
- Ping/Pong 心跳是在负载均衡器后保持连接活跃的推荐方式
- 长时间空闲时设置读写超时以避免连接泄漏