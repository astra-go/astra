# QUIC — HTTP/3 支持

基于标准库 `net/http` 和 `quic-go` 的 QUIC 和 HTTP/3 协议支持。

## 特性

- **HTTP/3**：支持客户端和服务端的 HTTP/3 协议
- **双协议监听**：可同时监听 HTTP/1.1 和 HTTP/3
- **0-RTT**：支持 0-RTT 连接建立（首次之后的连接）
- **连接迁移**：连接可在网络切换后保持

## 快速开始

### HTTP/3 服务端

```go
import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/quic"
)

app := astra.New()

// 同时监听 HTTP/1.1 (:8080) 和 HTTP/3 (:4433)
app.RunQuic(":8080", ":4433", "server.crt", "server.key")
```

### HTTP/3 客户端

```go
import "github.com/astra-go/astra/quic"

client := quic.NewClient(quic.Config{
    Addr:    "https://localhost:4433",
    Insecure: true, // 用于开发
})

resp, err := client.Get(ctx, "/api/data")
```

## API

### app.RunQuic

```go
func (app *App) RunQuic(httpAddr, quicAddr, certFile, keyFile string) error
```

同时启动 HTTP/1.1 服务器和 HTTP/3 QUIC 服务器。

### quic.NewClient

```go
func NewClient(cfg Config) (*Client, error)

type Config struct {
    Addr           string
    Insecure       bool              // 跳过 TLS 验证（用于开发）
    HandshakeTimeout time.Duration  // 握手超时
}
```

### 客户端方法

```go
client.Get(ctx, path)       (*http.Response, error)
client.Post(ctx, path, body)(*http.Response, error)
// ... 其他标准 HTTP 方法
```

## 配置

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `HandshakeTimeout` | `time.Duration` | `10s` | QUIC 握手超时 |
| `KeepAlive` | `time.Duration` | `30s` | 保活间隔 |

## 完整示例

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

    // HTTP/1.1 在 :8080，HTTP/3 在 :4433
    if err := app.RunQuic(":8080", ":4433", "certs/server.crt", "certs/server.key"); err != nil {
        panic(err)
    }
}
```

## 模块依赖

- `github.com/quic-go/quic-go` — QUIC 协议实现
- `golang.org/x/crypto` — TLS 加密

## 注意事项

- HTTP/3 需要 TLS 证书；自签名证书仅用于开发
- 客户端需配置适当的 KeepAlive 防止连接超时关闭
- HTTP/3 降级：不支持 HTTP/3 的客户端自动降级到 HTTP/1.1/2