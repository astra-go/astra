# Microservices

Astra provides a full suite of microservices components: service discovery, gRPC integration, distributed transactions, load balancing, and more.

## Service Discovery

### Consul

```go
import "github.com/astra-go/astra/discovery"

// Register service
registry, _ := discovery.NewConsulRegistryFromConfig(&api.Config{
    Address: "localhost:8500",
})

err := registry.Register(context.Background(), &discovery.ServiceInstance{
    Name:    "user-service",
    ID:      "user-service-1",
    Address: "192.168.1.100",
    Port:    8080,
    Metadata: map[string]string{
        "version": "1.0.0",
        "region":  "cn-east",
    },
})

// Discover services
instances, err := registry.Discover(context.Background(), "user-service")
for _, inst := range instances {
    log.Printf("Found: %s:%d", inst.Address, inst.Port)
}
```

### etcd

```go
import "github.com/astra-go/astra/discovery"

client, _ := discovery.NewEtcdClient([]string{"localhost:2379"}, 5*time.Second)
registry := discovery.NewEtcdRegistry(client, "/services/")

// Register
registry.Register(ctx, &discovery.ServiceInstance{
    Name:    "user-service",
    Address: "192.168.1.100",
    Port:    8080,
})

// Discover
instances, _ := registry.Discover(ctx, "user-service")
```

### Kubernetes

```go
import "github.com/astra-go/astra/discovery"

registry, _ := discovery.NewK8sRegistry(discovery.K8sConfig{
    Namespace: "default",
})
```

### Nacos

```go
import "github.com/astra-go/astra/discovery"

registry := discovery.NewNacosRegistry(client, discovery.NacosConfig{
    Endpoint:  "localhost:8848",
    Namespace: "public",
})
```

### Health Checks

```go
import "github.com/astra-go/astra/discovery"

// Auto-add health check when registering
registrar.Register(ctx, &discovery.ServiceInstance{
    Name:    "user-service",
    Address: "192.168.1.100",
    Port:    8080,
    HealthCheck: &discovery.HealthCheck{
        Interval: "10s",
        Timeout:  "3s",
        Path:     "/health",
    },
})
```

## gRPC Integration

### Start gRPC Server

```go
import (
    "github.com/astra-go/astra/grpc"
    pb "path/to/proto"
)

// Create gRPC server
gs := grpc.NewServer(grpc.Config{
    Port: 9090,
    // Optional: TLS
    TLS: &grpc.TLSConfig{
        CertFile: "server.crt",
        KeyFile:  "server.key",
    },
})

// Register service implementation
pb.RegisterUserServiceServer(gs, &userServer{})

// Start
gs.Start()
defer gs.GracefulStop()
```

### gRPC Gateway (HTTP → gRPC)

```go
import "github.com/astra-go/astra/grpc"

app := astra.New()

// Register gRPC gateway proxy
grpc.RegisterGateway(app, grpc.GatewayConfig{
    Endpoint: "localhost:9090", // gRPC server address
    // gRPC service mapping
    Services: []string{"UserService"},
})
```

### gRPC Client

```go
import (
    "github.com/astra-go/astra/client"
    pb "path/to/proto"
)

conn, err := client.NewGRPCConn(client.GRPCConfig{
    Address: "localhost:9090",
    Insecure: true,
})

svc := pb.NewUserServiceClient(conn)
resp, err := svc.GetUser(ctx, &pb.GetUserRequest{Id: 1})
```

## Distributed Transactions

### Saga Pattern

```go
import "github.com/astra-go/astra/dtx"

// Define Saga transaction
saga := dtx.NewSaga("create-order")

// Add forward operation
saga.AddStep(&dtx.SagaStep{
    Name: "deduct-inventory",
    Action: func(ctx context.Context) error {
        return inventoryService.Deduct(ctx, 1)
    },
    Compensate: func(ctx context.Context) error {
        return inventoryService.Restore(ctx, 1) // Compensation
    },
})

saga.AddStep(&dtx.SagaStep{
    Name: "create-order",
    Action: func(ctx context.Context) error {
        return orderService.Create(ctx, "order-123")
    },
    Compensate: func(ctx context.Context) error {
        return orderService.Cancel(ctx, "order-123")
    },
})

saga.AddStep(&dtx.SagaStep{
    Name: "charge-payment",
    Action: func(ctx context.Context) error {
        return paymentService.Charge(ctx, 99.99)
    },
    Compensate: func(ctx context.Context) error {
        return paymentService.Refund(ctx, 99.99)
    },
})

// Execute Saga (auto-rollback compensation on failure)
err := saga.Execute(ctx)
if err != nil {
    log.Printf("Saga failed, rolled back: %v", err)
}
```

### TCC Pattern

```go
import "github.com/astra-go/astra/dtx"

tcc := dtx.NewTCC("transfer-funds")

tcc.AddStep(&dtx.TCCStep{
    Name: "account-a",
    Try: func(ctx context.Context) error {
        return accountA.Freeze(ctx, 100)
    },
    Confirm: func(ctx context.Context) error {
        return accountA.Deduct(ctx, 100)
    },
    Cancel: func(ctx context.Context) error {
        return accountA.Unfreeze(ctx, 100)
    },
})

tcc.AddStep(&dtx.TCCStep{
    Name: "account-b",
    Try: func(ctx context.Context) error {
        return accountB.PrepareReceive(ctx, 100)
    },
    Confirm: func(ctx context.Context) error {
        return accountB.Add(ctx, 100)
    },
    Cancel: func(ctx context.Context) error {
        return accountB.CancelReceive(ctx, 100)
    },
})

err := tcc.Execute(ctx)
```

## Load Balancing

```go
import "github.com/astra-go/astra/loadbalance"

// Create load balancer
lb := loadbalance.New(loadbalance.Config{
    Strategy: loadbalance.StrategyRoundRobin, // Round Robin
    // StrategyLeastConnections               // Least Connections
    // StrategyIPHash                         // IP Hash
    // StrategyWeightedRoundRobin             // Weighted Round Robin
})

// Register backends
lb.Add("user-service", []*loadbalance.Endpoint{
    {Address: "192.168.1.10:8080", Weight: 5},
    {Address: "192.168.1.11:8080", Weight: 3},
    {Address: "192.168.1.12:8080", Weight: 2},
})

// Get endpoint
ep, err := lb.Next("user-service")
if err == nil {
    resp, _ := http.Post("http://"+ep.Address+"/api/users", ...)
}
```

## Observability

### OpenTelemetry

```go
import "github.com/astra-go/astra/otel"

// Initialize OTel
shutdown, err := otel.Setup(context.Background(), otel.Config{
    ServiceName:    "user-service",
    ServiceVersion: "1.0.0",
    Exporter:       otel.ExporterOTLP,
    Endpoint:       "otel-collector:4318",
})
defer shutdown()

// Astra auto-injects tracing and metrics
```

### Prometheus Metrics

```go
import "github.com/astra-go/astra/metrics"

// Start metrics HTTP server (exposes /metrics, /health, /dashboard)
ms := metrics.NewServer(":9091")
go ms.Start()

// Or use middleware for request-level metrics
app.Use(metrics.Middleware())
```

## End-to-End Tracing

### gRPC Metadata Propagation

```go
import "github.com/astra-go/astra/grpc"

// Auto-propagate trace ID, tenant ID, etc.
// Server auto-extracts from gRPC metadata
// Client auto-injects into gRPC metadata
```

### HTTP Request Propagation

```go
// Astra auto-extracts and propagates trace context from HTTP headers
// Trace ID: X-Request-Id
// Tenant ID: X-Tenant-ID
```
