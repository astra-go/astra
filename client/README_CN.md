# Client — 服务感知的 HTTP 客户端

集成服务发现、负载均衡、熔断器、重试和分布式追踪的 HTTP 客户端。

## 特性

- **服务发现集成**：内置 InMemory 注册中心，支持 Consul/etcd/Nacos
- **负载均衡**：RoundRobin、P2C 等策略，按服务名路由
- **熔断器**：每个目标服务独立熔断器；失败率超阈值自动开启
- **自动重试**：可配置的指数退避重试策略
- **分布式追踪**：OpenTelemetry 端到端传播
- **自动解析**：`WithAutoResolve` 后台订阅实例变化，消除每次请求 DNS 开销

## 快速开始

```go
import (
    "github.com/astra-go/astra/client"
    "github.com/astra-go/astra/discovery"
    "github.com/astra-go/astra/loadbalance"
)

// 1. 创建注册中心并注册服务实例
reg := discovery.NewInMemoryRegistry()
reg.Register(ctx, &discovery.ServiceInstance{
    Name:    "user-svc",
    Address: "10.0.0.1",
    Port:    8080,
    Weight:  100,
})

// 2. 创建客户端
cli := client.New(
    client.WithRegistry(reg),
    client.WithBalancer(loadbalance.NewRoundRobin()),
    client.WithTimeout(5*time.Second),
)
defer cli.Close()

// 3. 发起请求（按服务名，非硬编码地址）
resp, err := cli.Get(ctx, "user-svc", "/users/42")
```

## API

### client.New

```go
func New(opts ...Option) *Client
```

### 常用选项

| 选项 | 说明 |
|--------|-------------|
| `WithRegistry(r)` | 设置服务注册中心 |
| `WithBalancer(b)` | 设置负载均衡器（默认 RoundRobin） |
| `WithTimeout(d)` | 设置请求超时（默认 30s） |
| `WithRetryPolicy(p)` | 设置重试策略 |
| `WithAutoResolve(ctx)` | 后台订阅实例变化，消除 DNS 开销 |
| `WithCircuitBreaker(cfg)` | 设置熔断器参数 |
| `WithTracer(t)` | 设置 OpenTelemetry Tracer |

### HTTP 请求方法

```go
// 标准 HTTP 方法，返回 *http.Response
cli.Get(ctx, serviceName, path)       (*http.Response, error)
cli.Post(ctx, serviceName, path, body) (*http.Response, error)
cli.Put(ctx, serviceName, path, body)  (*http.Response, error)
cli.Delete(ctx, serviceName, path)     (*http.Response, error)
cli.Patch(ctx, serviceName, path, body)(*http.Response, error)
cli.Do(ctx, serviceName, req)         (*http.Response, error)
```

### gRPC 连接管理

```go
// 创建 gRPC 客户端连接
conn, err := client.NewGRPCConn(client.GRPCConfig{
    Address:  "localhost:9090",
    Insecure: true, // 生产环境配置 TLS
})
defer conn.Close()
```

## 配置

### 熔断器默认参数

| 参数 | 默认值 | 说明 |
|-------|---------|-------------|
| `FailureRateThreshold` | `0.5` | 失败率阈值（50%） |
| `MinRequestAmount` | `10` | 熔断器触发的最小请求数 |
| `SlowCallThreshold` | `5s` | 慢调用超时阈值 |
| `HalfOpenMaxAttempts` | `5` | 半开状态最大尝试次数 |

### 重试策略

```go
import "github.com/astra-go/astra/retry"

cli := client.New(
    client.WithRetryPolicy(retry.NewFixedPolicy(3, 100*time.Millisecond)),
)
```

## 完整示例

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

## 模块依赖

- `github.com/astra-go/astra/discovery` — 服务发现
- `github.com/astra-go/astra/loadbalance` — 负载均衡
- `github.com/astra-go/astra/circuit` — 熔断器
- `github.com/astra-go/astra/retry` — 重试策略
- `go.opentelemetry.io/otel` — 分布式追踪

## 注意事项

- `WithAutoResolve` 需要一个长生命周期的 ctx（如 `context.Background()`）直到 `cli.Close()`
- 生产环境务必为 gRPC 连接配置 TLS；禁止使用 `Insecure: true`
- 每个目标服务有独立的熔断器；熔断开启后该服务暂时不可用（直接返回错误）