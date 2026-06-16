# gRPC — gRPC 双栈服务端

同时运行 HTTP 和 gRPC 服务，支持 gRPC-Gateway HTTP→gRPC 代理。

## 特性

- **双栈服务端**：HTTP + gRPC 共享或分用端口
- **gRPC-Gateway**：通过 HTTP JSON 访问 gRPC 服务（自动生成 OpenAPI 文档）
- **Kratos 风格超时**：每个 gRPC 调用可配置超时时间
- **中间件支持**：Unary 拦截器（Recovery、Tracing、Logger）
- **元数据传播**：自动传递 Trace ID、Tenant ID 等
- **gRPC-Web**：通过 WebSocket 从浏览器使用 gRPC 服务
- **健康检查**：内置 gRPC Health Service

## 快速开始

```go
import "github.com/astra-go/astra/grpc"

s := grpcserver.New(app,
    grpcserver.WithHTTPAddr(":8080"),
    grpcserver.WithGRPCAddr(":9090"),
    grpcserver.WithTimeout(5*time.Second),
    grpcserver.WithUnaryInterceptors(
        grpcserver.UnaryInterceptorRecovery(),
        grpcserver.UnaryInterceptorTracing(),
        grpcserver.UnaryInterceptorLogger(),
    ),
)

// 注册 gRPC 服务
pb.RegisterUserServiceServer(s.GRPC, &server{})

// 挂载 gRPC-Gateway（HTTP → gRPC 代理）
s.Gateway().GET("/swagger/*any", swagger.Handler())

s.Run()
```

## API

### NewServer

```go
func NewServer(app *astra.App, opts ...Option) *Server
```

### 常用选项

| 选项 | 说明 |
|--------|-------------|
| `WithHTTPAddr(addr)` | HTTP 监听地址，默认 `:8080` |
| `WithGRPCAddr(addr)` | gRPC 监听地址，默认 `:9090` |
| `WithTimeout(d)` | 每次调用超时，默认无限制 |
| `WithUnaryInterceptors(...)` | 添加 Unary 拦截器 |
| `WithGateway(prefix, registrar)` | 挂载 gRPC-Gateway |
| `WithTLS(tlsConfig)` | gRPC TLS 配置 |

### 拦截器

```go
// Panic 恢复拦截器
UnaryInterceptorRecovery()

// OpenTelemetry 追踪拦截器
UnaryInterceptorTracing()

// 请求日志拦截器
UnaryInterceptorLogger()

// 自定义拦截器
UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
    // 前置处理
    resp, err = handler(ctx, req)
    // 后置处理
    return
})
```

### Gateway 注册

```go
s.Gateway().GET("/path", handler) // 向 Gateway 添加 HTTP 路由

// 通过 gRPC-Gateway 自动将 gRPC 服务暴露为 HTTP
grpcserver.WithGateway("/", func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
    return pb.RegisterUserServiceHandler(ctx, mux, conn)
})
```

## 配置

### TLS 配置

```go
cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
s := grpcserver.New(app,
    grpcserver.WithTLS(&tls.Config{
        Certificates: []tls.Certificate{cert},
    }),
)
```

### 元数据传播

自动从 gRPC Metadata 中提取并注入 Context，可通过中间件扩展：

```go
grpcserver.WithUnaryInterceptors(
    grpcserver.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
        md, _ := metadata.FromIncomingContext(ctx)
        // 从 metadata 中提取 tenant_id
        tenant := md.Get("tenant_id")
        return handler(metadata.NewIncomingContext(ctx, md), req)
    }),
)
```

## 完整示例

```go
package main

import (
    "context"
    "log"
    "net"
    "time"

    "github.com/astra-go/astra"
    "github.com/astra-go/astra/grpc"
    "google.golang.org/grpc"
    "google.golang.org/grpc/reflection"
    pb "github.com/my/proto"
)

type server struct {
    pb.UnimplementedGreeterServer
}

func (s *server) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloReply, error) {
    return &pb.HelloReply{Message: "Hello, " + req.Name}, nil
}

func main() {
    app := astra.New()

    s := grpcserver.New(app,
        grpcserver.WithHTTPAddr(":8080"),
        grpcserver.WithGRPCAddr(":9090"),
        grpcserver.WithTimeout(5*time.Second),
        grpcserver.WithUnaryInterceptors(
            grpcserver.UnaryInterceptorRecovery(),
        ),
    )

    pb.RegisterGreeterServer(s.GRPC, &server{})
    reflection.Register(s.GRPC) // 启用 reflection（用于开发）

    if err := s.Run(); err != nil {
        log.Fatal(err)
    }
}
```

## 模块依赖

- `google.golang.org/grpc` — gRPC 核心库
- `github.com/grpc-ecosystem/grpc-gateway/v2` — HTTP → gRPC 代理

## 注意事项

- 生产环境务必配置 TLS（`WithTLS`）；禁止明文传输
- `WithTimeout` 设置的是每次 RPC 的超时，而非连接超时
- gRPC-Gateway 注册在 HTTP 服务器上；gRPC 服务注册在 gRPC 服务器上；两者独立