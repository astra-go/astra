# Stream — 流式处理

支持 Server-Sent Events（SSE）、客户端流和双向 WebSocket 流。

## 特性

- **Server-Sent Events（SSE）**：通过 `/events` 端点服务端推送实时数据
- **客户端流**：客户端上传数据流，服务端接收并返回结果
- **双向流（Bidirectional）**：WebSocket 全双工通信
- **单流限流**：每条流独立限流
- **序列化**：JSON（默认）和其他自定义序列化器

## 快速开始

### 服务端推送（SSE）

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

### 客户端流（文件上传）

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

### 双向流（实时聊天）

```go
app.GET("/ws/chat", stream.BidiHandler(func(s contract.BidiStream) error {
    for {
        var msg Message
        if err := s.Recv(&msg); errors.Is(err, io.EOF) {
            return nil
        } else if err != nil {
            return err
        }
        // 广播消息
        broadcast(msg)
    }
}))
```

## API

### ServerStreamHandler

```go
func ServerStreamHandler(fn func(contract.ServerStream) error) astra.HandlerFunc

// contract.ServerStream 接口：
type ServerStream interface {
    Context
    Send(v any) error       // 发送数据（JSON 序列化）
    Done() <-chan struct{} // 流结束信号
}
```

### ClientStreamHandler

```go
func ClientStreamHandler(fn func(contract.ClientStream) error) astra.HandlerFunc

// contract.ClientStream 接口：
type ClientStream interface {
    Context
    Recv(v any) error                  // 接收下一条消息
    SendAndClose(v any) error          // 发送结果并关闭流
    Done() <-chan struct{}
}
```

### BidiHandler

```go
func BidiHandler(fn func(contract.BidiStream) error) astra.HandlerFunc

// contract.BidiStream 接口：
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
    Codec   astra.Serializer  // 序列化器（默认 JSON）
    RateLimit *RateLimitConfig // 限流配置
}

type RateLimitConfig struct {
    Rate  int // 每秒消息数
    Burst int // 突发容量
}
```

## 配置

### 全局流配置

```go
import "github.com/astra-go/astra/stream"

app := astra.New(
    astra.WithStream(stream.Options{
        RateLimit: &stream.RateLimitConfig{
            Rate:  100, // 最多每秒 100 条消息
            Burst: 200,
        },
    }),
)
```

### 自定义编解码器

```go
import (
    "github.com/msgpack/msgpack"
)

codec := msgpackSerializer{} // 实现 astra.Serializer 接口
stream.NewServerStreamHandler(fn, stream.Options{Codec: codec})
```

## 完整示例

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

    // SSE 实时事件推送
    app.GET("/events", stream.ServerStreamHandler(func(s contract.ServerStream) error {
        for i := 0; i < 10; i++ {
            if err := s.Send(map[string]int{"count": i}); err != nil {
                return err
            }
        }
        return nil
    }))

    // 客户端流处理
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

## 模块依赖

- `github.com/gorilla/websocket` — WebSocket 支持

## 注意事项

- SSE 兼容大多数现代浏览器但不支持 IE；IE 兼容使用轮询作为降级方案
- `s.Recv` 在客户端关闭发送端时返回 `io.EOF`
- `Done()` 通道在流结束时关闭，可用于优雅检测客户端断开