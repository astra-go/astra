# gRPC — gRPC Dual-Stack Server

Runs HTTP and gRPC servers simultaneously, supports gRPC-Gateway HTTP→gRPC proxy.

## Features

- **Dual-Stack Server**: HTTP + gRPC on shared or separate ports
- **gRPC-Gateway**: Access gRPC services via HTTP JSON (auto-generates OpenAPI docs)
- **Kratos-Style Timeout**: Each gRPC call configurable with timeout
- **Middleware Support**: Unary interceptors (Recovery, Tracing, Logger)
- **Metadata Propagation**: Auto-passes Trace ID, Tenant ID, etc.
- **gRPC-Web**: Use gRPC services from browsers via WebSocket
- **Health Checks**: Built-in gRPC Health Service

## Quick Start

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

// Register gRPC service
pb.RegisterUserServiceServer(s.GRPC, &server{})

// Mount gRPC-Gateway (HTTP → gRPC proxy)
s.Gateway().GET("/swagger/*any", swagger.Handler())

s.Run()
```

## API

### NewServer

```go
func NewServer(app *astra.App, opts ...Option) *Server
```

### Common Options

| Option | Description |
|--------|-------------|
| `WithHTTPAddr(addr)` | HTTP listen address, default `:8080` |
| `WithGRPCAddr(addr)` | gRPC listen address, default `:9090` |
| `WithTimeout(d)` | Per-call timeout, default unlimited |
| `WithUnaryInterceptors(...)` | Add Unary interceptors |
| `WithGateway(prefix, registrar)` | Mount gRPC-Gateway |
| `WithTLS(tlsConfig)` | gRPC TLS config |

### Interceptors

```go
// Panic recovery interceptor
UnaryInterceptorRecovery()

// OpenTelemetry tracing interceptor
UnaryInterceptorTracing()

// Request logging interceptor
UnaryInterceptorLogger()

// Custom interceptor
UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
    // Pre-processing
    resp, err = handler(ctx, req)
    // Post-processing
    return
})
```

### Gateway Registration

```go
s.Gateway().GET("/path", handler) // Add HTTP route to Gateway

// Auto-expose gRPC service as HTTP via gRPC-Gateway
grpcserver.WithGateway("/", func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
    return pb.RegisterUserServiceHandler(ctx, mux, conn)
})
```

## Config

### TLS Config

```go
cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
s := grpcserver.New(app,
    grpcserver.WithTLS(&tls.Config{
        Certificates: []tls.Certificate{cert},
    }),
)
```

### Metadata Propagation

Auto-extracts from gRPC Metadata and injects into Context, extensible via middleware:

```go
grpcserver.WithUnaryInterceptors(
    grpcserver.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
        md, _ := metadata.FromIncomingContext(ctx)
        // Extract tenant_id from metadata
        tenant := md.Get("tenant_id")
        return handler(metadata.NewIncomingContext(ctx, md), req)
    }),
)
```

## Complete Example

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
    reflection.Register(s.GRPC) // Enable reflection (for development)

    if err := s.Run(); err != nil {
        log.Fatal(err)
    }
}
```

## Module Dependencies

- `google.golang.org/grpc` — gRPC core library
- `github.com/grpc-ecosystem/grpc-gateway/v2` — HTTP → gRPC proxy

## Notes

- Always configure TLS (`WithTLS`) in production; never transmit in plaintext
- `WithTimeout` sets per-RPC timeout, not connection timeout
- gRPC-Gateway registers on HTTP server; gRPC service registers on gRPC server; they are independent
