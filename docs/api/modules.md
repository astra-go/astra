# 扩展模块 API

Astra 的重量级集成以独立子模块（sub-module）发布，按需引入，不污染核心 `go.mod`。

---

## Reactor 网络引擎（netengine）

```go
import "github.com/astra-go/astra/netengine"

engine, err := netengine.New(app, netengine.ReactorConfig{
    NumLoops:          runtime.NumCPU(),
    WorkerPoolSize:    runtime.NumCPU() * 4,
    ConnChannelBuffer: 256,
    Logger:            slog.Default(),
})

ln, _ := netengine.ListenReusePort("tcp", ":8080")
engine.Serve(ln)
```

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `NumLoops` | CPU 数 | 事件循环数量 |
| `WorkerPoolSize` | CPU×4 | Handler goroutine 池大小 |
| `ConnChannelBuffer` | 64 | 每个事件循环的连接接收缓冲 |
| `Logger` | `slog.Default()` | 结构化日志 |

```go
engine.ActiveConns()      // 当前活跃连接数
engine.NumLoops()         // 事件循环数量
engine.WorkerPoolSize()   // Worker 池大小
engine.Close()            // 立即关闭（测试用）
```

**`ListenReusePort`** / **`Listen`** 支持 `SO_REUSEPORT` + `TCP_FASTOPEN` 组合。

---

## HTTP/3 — QUIC

```go
// 同时启动 HTTP/3 + TLS HTTP/2（自动写入 Alt-Svc 升级头）
err := app.RunQUIC(":443", "cert.pem", "key.pem")
```

要求：Linux ≥ 5.4 / macOS 12+，go 1.21+。

---

## OpenTelemetry

```go
import "github.com/astra-go/astra/otel"

app.Use(otel.Middleware(otel.Config{
    ServiceName:    "my-service",
    ServiceVersion: "v1.0.0",
    Endpoint:       "http://otel-collector:4318",  // OTLP HTTP
    Sampler:        trace.AlwaysSample(),
}))

// 在 handler 中使用
traceID := c.TraceID()
spanID  := c.SpanID()
```

---

## Prometheus

```go
import "github.com/astra-go/astra/middleware"

app.Use(middleware.Prometheus(middleware.PrometheusConfig{
    Namespace:   "myapp",
    ConstLabels: prometheus.Labels{"env": "prod"},
}))
// 暴露 /metrics
```

---

## gRPC 双栈

```go
import "github.com/astra-go/astra/grpc"

srv := grpc.NewServer(grpc.Config{
    Addr:             ":9090",
    TLSCertFile:      "cert.pem",
    TLSKeyFile:       "key.pem",
    UnaryMiddleware:  []grpc.UnaryMiddlewareFunc{grpc.Recovery(), grpc.Logger()},
    StreamMiddleware: []grpc.StreamMiddlewareFunc{},
})

// HTTP+gRPC 共同监听同一端口（h2c 多路复用）
app.RunGRPC(srv)
```

---

## 熔断器（circuit）

```go
import "github.com/astra-go/astra/circuit"

cb := circuit.New(circuit.Config{
    Strategy:        circuit.ConsecutiveFailures(5),
    // Strategy:     circuit.AdaptiveFailureRate(0.5, 20),
    Timeout:         10 * time.Second,  // 半开超时
    OnStateChange:   func(from, to circuit.State) { slog.Info("cb", "from", from, "to", to) },
})

err := cb.Execute(ctx, func(ctx context.Context) error {
    return downstreamCall(ctx)
})
```

---

## 服务发现（discovery）

```go
import (
    "github.com/astra-go/astra/discovery"
    "github.com/astra-go/astra/discovery/nacos"  // 或 consul / etcd / k8s
)

reg, _ := nacos.New(nacos.Config{Addr: "localhost:8848"})

// 注册
reg.Register(ctx, &discovery.ServiceInstance{
    ID: "svc-1", Name: "user-service", Addr: "10.0.0.1", Port: 8080,
})

// 发现
instances, _ := reg.Discover(ctx, "user-service")

// 监听变化
ch, _ := reg.Watch(ctx, "user-service")
for instances := range ch { ... }
```

Kubernetes 后端：

```go
import "github.com/astra-go/astra/discovery/k8s"

reg, _ := k8s.New(k8s.Config{Namespace: "default", InCluster: true})
```

---

## 消息队列（mq）

```go
import (
    "github.com/astra-go/astra/mq"
    "github.com/astra-go/astra/mq/nats"   // nats / kafka / rabbitmq / redis / sqs / pulsar
)

p, _ := nats.NewProducer(nats.Config{URL: "nats://localhost:4222"})
p.Publish(ctx, "orders", []byte(`{"id":1}`))

c, _ := nats.NewConsumer(nats.ConsumerConfig{Config: nats.Config{URL: "nats://localhost:4222"}, Subject: "orders"})
c.Subscribe(ctx, func(msg *mq.Message) error {
    fmt.Println(string(msg.Body))
    return nil
})
```

---

## 缓存（cache）

```go
import "github.com/astra-go/astra/cache"

// 本地 LRU+TTL
c := cache.NewMemory(cache.MemoryConfig{Capacity: 1000, DefaultTTL: 5 * time.Minute})

// Redis 分布式缓存
c = cache.NewRedis(cache.RedisConfig{Addr: "localhost:6379", DefaultTTL: time.Hour})

c.Set(ctx, "key", value, cache.WithTTL(30*time.Second))
var v MyType
c.Get(ctx, "key", &v)
c.Delete(ctx, "key")
```

---

## 数据库（orm）

```go
import "github.com/astra-go/astra/orm"

db, _ := orm.Open(orm.Config{
    DSN:             "user:pass@tcp(localhost:3306)/db?parseTime=True",
    MaxOpenConns:    20,
    MaxIdleConns:    5,
    ConnMaxLifetime: time.Hour,
    Logger:          orm.NewSlogLogger(slog.Default()),
})

// 带 tracing 的查询
db.WithContext(ctx).Find(&users)
```

ClickHouse 适配器：

```go
import "github.com/astra-go/astra/orm/clickhouse"

db, _ := clickhouse.Open(clickhouse.Config{DSN: "clickhouse://localhost:9000/db"})
```

---

## Elasticsearch（search/elastic）

```go
import "github.com/astra-go/astra/search/elastic"

client, _ := elastic.New(elastic.Config{
    Addresses: []string{"http://localhost:9200"},
    Username: "elastic", Password: "changeme",
})

client.Index(ctx, elastic.IndexRequest{Index: "products", ID: "1", Doc: product})
client.BulkIndex(ctx, requests)

result, _ := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products"},
    Query: map[string]any{"match": map[string]any{"name": "shoes"}},
    Size:  10,
})
```

---

## Saga 分布式事务（dtx）

```go
import "github.com/astra-go/astra/dtx"

result := dtx.New(
    dtx.Step{
        Name:       "deduct-inventory",
        Forward:    func(ctx context.Context) error { return inventorySvc.Deduct(ctx, item) },
        Compensate: func(ctx context.Context) error { return inventorySvc.Restore(ctx, item) },
    },
    dtx.Step{
        Name:       "charge-payment",
        Forward:    func(ctx context.Context) error { return paymentSvc.Charge(ctx, amount) },
        Compensate: func(ctx context.Context) error { return paymentSvc.Refund(ctx, amount) },
    },
    dtx.Step{
        Name:    "send-email",   // 不可逆步骤：不提供 Compensate
        Forward: func(ctx context.Context) error { return emailSvc.Send(ctx, email) },
    },
).Execute(ctx)

if result.Err != nil {
    log.Printf("saga failed at %s: %v", result.FailedStep, result.Err)
}
```

---

## 告警规则引擎（alert）

```go
import "github.com/astra-go/astra/alert"

engine := alert.NewEngine(alert.EngineConfig{EvalInterval: 30 * time.Second})

engine.RegisterMetric("cpu_usage", func() float64 { return getCPU() })
engine.RegisterMetric("mem_usage", func() float64 { return getMem() })

engine.AddRule(alert.Rule{
    Name:     "high-cpu",
    Expr:     "cpu_usage > 90",
    For:      2 * time.Minute,
    Labels:   map[string]string{"severity": "critical"},
    Channels: []string{"webhook"},
})

engine.AddChannel(&alert.WebhookChannel{
    ChannelName: "webhook",
    URL:         "https://hooks.slack.com/...",
    Timeout:     5 * time.Second,
})

engine.Start(ctx)
defer engine.Stop()
```

---

## OAuth2 / OIDC（auth/oauth2）

```go
import "github.com/astra-go/astra/auth/oauth2"

cfg := oauth2.Config{
    ClientID:     "client-id",
    ClientSecret: "client-secret",
    RedirectURL:  "https://myapp.com/callback",
    Scopes:       []string{"openid", "email", "profile"},
    Endpoint:     oauth2.GoogleEndpoint,
    PKCE:         true,
    UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
    OnSuccess: func(c *astra.Context, t *oauth2.Token, info map[string]any) error {
        c.Set("user_email", info["email"])
        return c.Redirect(302, "/dashboard")
    },
}

app.GET("/oauth/login",    oauth2.LoginHandler(cfg))
app.GET("/oauth/callback", oauth2.CallbackHandler(cfg))
```

---

## GraphQL 挂载

```go
import "github.com/astra-go/astra/graphql"

// gqlgen 生成的 handler（用户自行 go get github.com/99designs/gqlgen）
h := handler.NewDefaultServer(generated.NewExecutableSchema(cfg))

graphql.Mount(app, h, graphql.Options{
    Path:           "/graphql",
    PlaygroundPath: "/playground",
    PlaygroundTitle: "My API",
})
```

---

## 健康检查 + Istio Probe

```go
import "github.com/astra-go/astra/health"

health.Register(app,
    health.Check{Name: "database", Check: func(ctx context.Context) error { return db.PingContext(ctx) }},
    health.Check{Name: "redis",    Check: func(ctx context.Context) error { return rdb.Ping(ctx).Err() }},
)
// GET /live   → 200 / 503
// GET /ready  → 200 / 503（汇总所有 Check）

// Istio 额外 probe 路径
health.RegisterIstioProbes(app, health.WithIstioHeaders())
// GET /healthz/live  → liveness
// GET /healthz/ready → readiness
```
