# Extension Modules API

Astra's heavyweight integrations are published as independent sub-modules — import only what you need without polluting the core `go.mod`.

---

## Reactor Network Engine (netengine)

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

| Field | Default | Description |
|-------|---------|-------------|
| `NumLoops` | CPU count | Number of event loops |
| `WorkerPoolSize` | CPU×4 | Handler goroutine pool size |
| `ConnChannelBuffer` | 64 | Connection accept buffer per event loop |
| `Logger` | `slog.Default()` | Structured logger |

```go
engine.ActiveConns()      // current active connection count
engine.NumLoops()         // number of event loops
engine.WorkerPoolSize()   // worker pool size
engine.Close()            // immediate shutdown (for tests)
```

**`ListenReusePort`** / **`Listen`** support the `SO_REUSEPORT` + `TCP_FASTOPEN` combination.

---

## HTTP/3 — QUIC

```go
// Start HTTP/3 + TLS HTTP/2 simultaneously (automatically writes Alt-Svc upgrade header)
err := app.RunQUIC(":443", "cert.pem", "key.pem")
```

Requirements: Linux ≥ 5.4 / macOS 12+, go 1.21+.

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

// In a handler
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
// Exposes /metrics
```

---

## gRPC Dual-Stack

```go
import "github.com/astra-go/astra/grpc"

srv := grpc.NewServer(grpc.Config{
    Addr:             ":9090",
    TLSCertFile:      "cert.pem",
    TLSKeyFile:       "key.pem",
    UnaryMiddleware:  []grpc.UnaryMiddlewareFunc{grpc.Recovery(), grpc.Logger()},
    StreamMiddleware: []grpc.StreamMiddlewareFunc{},
})

// HTTP + gRPC sharing a single port (h2c multiplexing)
app.RunGRPC(srv)
```

---

## Circuit Breaker (circuit)

```go
import "github.com/astra-go/astra/circuit"

cb := circuit.New(circuit.Config{
    Strategy:        circuit.ConsecutiveFailures(5),
    // Strategy:     circuit.AdaptiveFailureRate(0.5, 20),
    Timeout:         10 * time.Second,  // half-open timeout
    OnStateChange:   func(from, to circuit.State) { slog.Info("cb", "from", from, "to", to) },
})

err := cb.Execute(ctx, func(ctx context.Context) error {
    return downstreamCall(ctx)
})
```

---

## Service Discovery (discovery)

```go
import (
    "github.com/astra-go/astra/discovery"
    "github.com/astra-go/astra/discovery/nacos"  // or consul / etcd / k8s
)

reg, _ := nacos.New(nacos.Config{Addr: "localhost:8848"})

// Register
reg.Register(ctx, &discovery.ServiceInstance{
    ID: "svc-1", Name: "user-service", Addr: "10.0.0.1", Port: 8080,
})

// Discover
instances, _ := reg.Discover(ctx, "user-service")

// Watch for changes
ch, _ := reg.Watch(ctx, "user-service")
for instances := range ch { ... }
```

Kubernetes backend:

```go
import "github.com/astra-go/astra/discovery/k8s"

reg, _ := k8s.New(k8s.Config{Namespace: "default", InCluster: true})
```

---

## Message Queue (mq)

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

## Cache (cache)

```go
import "github.com/astra-go/astra/cache"

// Local LRU + TTL
c := cache.NewMemory(cache.MemoryConfig{Capacity: 1000, DefaultTTL: 5 * time.Minute})

// Redis distributed cache
c = cache.NewRedis(cache.RedisConfig{Addr: "localhost:6379", DefaultTTL: time.Hour})

c.Set(ctx, "key", value, cache.WithTTL(30*time.Second))
var v MyType
c.Get(ctx, "key", &v)
c.Delete(ctx, "key")
```

---

## Database (orm)

```go
import "github.com/astra-go/astra/orm"

db, _ := orm.Open(orm.Config{
    DSN:             "user:pass@tcp(localhost:3306)/db?parseTime=True",
    MaxOpenConns:    20,
    MaxIdleConns:    5,
    ConnMaxLifetime: time.Hour,
    Logger:          orm.NewSlogLogger(slog.Default()),
})

// Query with tracing
db.WithContext(ctx).Find(&users)
```

ClickHouse adapter:

```go
import "github.com/astra-go/astra/orm/clickhouse"

db, _ := clickhouse.Open(clickhouse.Config{DSN: "clickhouse://localhost:9000/db"})
```

---

## Elasticsearch (search/elastic)

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

## Saga Distributed Transactions (dtx)

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
        Name:    "send-email",   // irreversible step: no Compensate provided
        Forward: func(ctx context.Context) error { return emailSvc.Send(ctx, email) },
    },
).Execute(ctx)

if result.Err != nil {
    log.Printf("saga failed at %s: %v", result.FailedStep, result.Err)
}
```

---

## Alert Rule Engine (alert)

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

## OAuth2 / OIDC (auth/oauth2)

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

## GraphQL Mount

```go
import "github.com/astra-go/astra/graphql"

// Handler generated by gqlgen (user installs go get github.com/99designs/gqlgen separately)
h := handler.NewDefaultServer(generated.NewExecutableSchema(cfg))

graphql.Mount(app, h, graphql.Options{
    Path:           "/graphql",
    PlaygroundPath: "/playground",
    PlaygroundTitle: "My API",
})
```

---

## Health Checks + Istio Probes

```go
import "github.com/astra-go/astra/health"

health.Register(app,
    health.Check{Name: "database", Check: func(ctx context.Context) error { return db.PingContext(ctx) }},
    health.Check{Name: "redis",    Check: func(ctx context.Context) error { return rdb.Ping(ctx).Err() }},
)
// GET /live   → 200 / 503
// GET /ready  → 200 / 503 (aggregates all Checks)

// Additional Istio probe paths
health.RegisterIstioProbes(app, health.WithIstioHeaders())
// GET /healthz/live  → liveness
// GET /healthz/ready → readiness
```
