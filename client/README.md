# Client — Service-Aware HTTP Client

HTTP client with integrated service discovery, load balancing, circuit breaker, retry, and distributed tracing.

## Features

- **Service Discovery Integration**: Built-in InMemory registry, supports Consul/etcd/Nacos
- **Load Balancing**: RoundRobin, P2C and other strategies, routing by service name
- **Circuit Breaker**: Each target service has independent circuit breaker; auto-opens on failure threshold
- **Auto Retry**: Configurable exponential backoff retry policy
- **Distributed Tracing**: OpenTelemetry end-to-end propagation
- **Auto-Resolve**: `WithAutoResolve` subscribes to instance changes in background, eliminating per-request DNS overhead

## Quick Start

```go
import (
    "github.com/astra-go/astra/client"
    "github.com/astra-go/astra/discovery"
    "github.com/astra-go/astra/loadbalance"
)

// 1. Create registry and register service instances
reg := discovery.NewInMemoryRegistry()
reg.Register(ctx, &discovery.ServiceInstance{
    Name:    "user-svc",
    Address: "10.0.0.1",
    Port:    8080,
    Weight:  100,
})

// 2. Create client
cli := client.New(
    client.WithRegistry(reg),
    client.WithBalancer(loadbalance.NewRoundRobin()),
    client.WithTimeout(5*time.Second),
)
defer cli.Close()

// 3. Make requests (by service name, not hardcoded address)
resp, err := cli.Get(ctx, "user-svc", "/users/42")
```

## API

### client.New

```go
func New(opts ...Option) *Client
```

### Common Options

| Option | Description |
|--------|-------------|
| `WithRegistry(r)` | Set service registry |
| `WithBalancer(b)` | Set load balancer (default RoundRobin) |
| `WithTimeout(d)` | Set request timeout (default 30s) |
| `WithRetryPolicy(p)` | Set retry policy |
| `WithAutoResolve(ctx)` | Background subscribe to instance changes, eliminate DNS overhead |
| `WithCircuitBreaker(cfg)` | Set circuit breaker parameters |
| `WithTracer(t)` | Set OpenTelemetry Tracer |

### HTTP Request Methods

```go
// Standard HTTP methods, return *http.Response
cli.Get(ctx, serviceName, path)       (*http.Response, error)
cli.Post(ctx, serviceName, path, body) (*http.Response, error)
cli.Put(ctx, serviceName, path, body)  (*http.Response, error)
cli.Delete(ctx, serviceName, path)     (*http.Response, error)
cli.Patch(ctx, serviceName, path, body)(*http.Response, error)
cli.Do(ctx, serviceName, req)         (*http.Response, error)
```

### gRPC Connection Management

```go
// Create gRPC client connection
conn, err := client.NewGRPCConn(client.GRPCConfig{
    Address:  "localhost:9090",
    Insecure: true, // Configure TLS in production
})
defer conn.Close()
```

## Config

### Circuit Breaker Default Params

| Param | Default | Description |
|-------|---------|-------------|
| `FailureRateThreshold` | `0.5` | Failure rate threshold (50%) |
| `MinRequestAmount` | `10` | Min requests before circuit breaker triggers |
| `SlowCallThreshold` | `5s` | Slow call timeout threshold |
| `HalfOpenMaxAttempts` | `5` | Max attempts in half-open state |

### Retry Policy

```go
import "github.com/astra-go/astra/retry"

cli := client.New(
    client.WithRetryPolicy(retry.NewFixedPolicy(3, 100*time.Millisecond)),
)
```

## Complete Example

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/astra-go/astra/client"
    "github.com/astra-go/astra/discovery"
    "github.com/astra-go/astra/loadbalance"
)

type User struct{ ID int `json:"id"`; Name string `json:"name"` }

func main() {
    ctx := context.Background()
    reg := discovery.NewInMemoryRegistry()
    reg.Register(ctx, &discovery.ServiceInstance{
        Name: "user-svc", Address: "localhost", Port: 8080,
    })

    cli := client.New(
        client.WithRegistry(reg),
        client.WithBalancer(loadbalance.NewRoundRobin()),
        client.WithTimeout(3*time.Second),
    )
    defer cli.Close()

    resp, err := cli.Get(ctx, "user-svc", "/users/1")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    var user User
    json.NewDecoder(resp.Body).Decode(&user)
    fmt.Printf("%+v\n", user)
}
```

## Module Dependencies

- `github.com/astra-go/astra/discovery` — Service discovery
- `github.com/astra-go/astra/loadbalance` — Load balancing
- `github.com/astra-go/astra/circuit` — Circuit breaker
- `github.com/astra-go/astra/retry` — Retry policy
- `go.opentelemetry.io/otel` — Distributed tracing

## Notes

- `WithAutoResolve` needs a long-lived ctx (e.g., `context.Background()`) until `cli.Close()`
- Always configure TLS for gRPC connections in production; never use `Insecure: true`
- Each target service has an independent circuit breaker; when open, that service is temporarily unavailable (returns error directly)
