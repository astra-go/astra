# ⭐ Astra — Go Web Framework for the Stars

> **Astra** — A next-generation Go Web framework built for the stars.

[![Go Reference](https://pkg.go.dev/badge/github.com/astra-go/astra.svg)](https://pkg.go.dev/github.com/astra-go/astra)
[![Go Report Card](https://goreportcard.com/badge/github.com/astra-go/astra)](https://goreportcard.com/report/github.com/astra-go/astra)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/astra-go/astra)](go.mod)
[![Version: v1.0.5](https://img.shields.io/badge/version-v1.0.5-blue.svg)](https://github.com/astra-go/astra/releases/tag/v1.0.5)

Astra is a **modern, high-performance** Go Web framework that distills the best practices from Gin, Echo, go-zero, Beego, and Kratos, featuring a lightweight core with a rich extensions ecosystem.

---

## ✨ Core Features

- **🚀 High Performance** — radix-tree router, zero-allocation Context pool, optimized JSON serialization (Sonic)
- **🧩 Component Architecture** — Unified `Component` interface, modules plug-and-play
- **🔧 Rich Built-in Middleware** — CORS, CSRF, Recovery, Logger, Compress, RateLimit, JWT, API Key, IP Filter, and more
- **📦 Full-Featured Extensions** — ORM, cache, message queue, config center, service discovery, gRPC, session, distributed lock, storage, and more
- **🔌 Plugin Ecosystem** — Prometheus metrics, OpenTelemetry, health checks, distributed transactions (Saga/TCC)
- **⚙️ Flexible Configuration** — Multi-source config (file, env, etcd, Nacos, Apollo), hot reload support
- **🔒 Enterprise Security** — JWT auth (with revocation), API Key, signature verification, multi-level cache
- **🌐 Multi-Protocol** — HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSocket, gRPC
- **📝 Convention Over Config → Extensible** — Zero-config defaults, optional advanced features when needed

---

## 📦 Installation

### Quick Install (Recommended)

```bash
# Install latest version
go get github.com/astra-go/astra@v1.0.5

# Or install specific module
go get github.com/astra-go/astra/cache@v1.0.5
go get github.com/astra-go/astra/orm@v1.0.5
```

### Version Information

| Component | Version | Install Command |
|-----------|---------|----------------|
| **Core** | `v1.0.5` | `go get github.com/astra-go/astra@v1.0.5` |
| **Cache** | `v1.0.5` | `go get github.com/astra-go/astra/cache@v1.0.5` |
| **ORM** | `v1.0.5` | `go get github.com/astra-go/astra/orm@v1.0.5` |
| **All Modules** | `v1.0.5` | See [docs/installation.md](docs/installation.md) |

> 💡 **Tip:** All sub-modules are versioned independently. Use `@v1.0.5` to ensure consistent versions across the monorepo.

---

## 🚀 Quick Start

## 🏗️ Project Structure

```
astra/
├── 📄 app.go           # Core App — routing, middleware, lifecycle
├── 📄 router.go         # radix-tree router
├── 📄 context.go        # Request context (zero-allocation design)
├── 📄 group.go          # Route groups
├── 📄 module.go         # Component registration system (v1 compatible)
├── 📄 lifecycle.go      # Start/stop hooks
├── 📄 options.go        # App configuration options
├── 📄 errors.go         # HTTP errors and business errors
│
├── 🧩 middleware/       # Built-in middleware
│   ├── cors.go          # Cross-origin
│   ├── csrf.go          # CSRF protection
│   ├── recovery.go      # Panic recovery
│   ├── logger.go        # Access log
│   ├── compress.go      # Gzip compression
│   ├── secure.go        # Security headers
│   ├── csp.go           # Content Security Policy
│   ├── timeout.go       # Request timeout
│   ├── requestid.go     # Request ID
│   ├── sanitize.go      # Input sanitization
│   ├── marketplace/     # Middleware marketplace
│   └── security/        # Security middleware (JWT/APIKey/RateLimit/Tenant/etc.)
│
├── 📦 cache/            # Cache abstraction (memory/redis/memcached)
├── 📦 orm/              # GORM integration (read-write separation, sharding, transaction propagation)
├── 📦 mq/               # Message queue (Kafka/RabbitMQ/RocketMQ/MQTT/NATS/Pulsar)
├── 📦 config/           # Config management (yaml/env/etcd/nacos/apollo)
├── 📦 session/          # Session management (Redis)
├── 📦 auth/             # Auth (OAuth2, RBAC)
├── 📦 storage/          # Object storage (S3/OSS/COS)
├── 📦 discovery/        # Service discovery (consul/etcd/k8s/nacos)
├── 📦 grpc/             # gRPC integration
├── 📦 notify/           # Notifications (email/SMS/push)
├── 📦 search/           # Search (Elasticsearch)
├── 📦 client/           # HTTP/gRPC client
├── 📦 taskqueue/        # Task queue
├── 📦 dtx/              # Distributed transactions (Saga/TCC)
├── 📦 lock/             # Distributed lock (etcd/redis)
├── 📦 loadbalance/      # Load balancing
├── 📦 runner/           # Background task scheduling (cron/dagu/gocron)
├── 📦 stream/           # Streaming processing
├── 📦 rule/             # Rule engine (Lua)
├── 📦 health/           # Health checks
├── 📦 metrics/          # Prometheus metrics
├── 📦 otel/             # OpenTelemetry
├── 📦 cron/             # Scheduled tasks
├── 📦 di/               # Dependency injection
├── 📦 alert/            # Alerting system
├── 📦 contract/         # Interface contracts
├── 📦 binding/          # Request binding
├── 📦 pagination/       # Pagination
├── 📦 render/           # Template rendering
├── 📦 validate/         # Data validation
├── 📦 websocket/        # WebSocket
├── 📦 quic/             # QUIC/HTTP3
├── 📦 log/              # Logging
├── 📦 i18n/             # Internationalization
├── 📦 upload/           # File upload
├── 📦 mongodb/          # MongoDB integration
├── 📦 netengine/        # Network engine
│
├── 📂 examples/         # Example projects
├── 📂 deploy/           # Deployment configs (Docker/Helm/Kustomize)
├── 📂 docs/             # Documentation
└── 📂 scripts/          # Build scripts
```

## 🚀 Quick Start

### Installation

```bash
go get github.com/astra-go/astra@latest
```

### Minimal Example

```go
package main

import (
    "log"
    "github.com/astra-go/astra"
)

func main() {
    app := astra.New()

    app.GET("/hello/:name", func(c *astra.Ctx) error {
        return c.JSON(200, astra.Map{
            "message": "Hello, " + c.Param("name"),
        })
    })

    log.Fatal(app.Run(":8080"))
}
```

Run it:

```bash
go run main.go
# Visit http://localhost:8080/hello/Astra
# Output: {"message":"Hello, Astra"}
```

### Full Application

```go
package main

import (
    "log"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
)

func main() {
    // Create app
    app := astra.New()

    // Global middleware
    app.Use(middleware.Logger())
    app.Use(middleware.Recovery())
    app.Use(middleware.CORS("https://example.com"))

    // Route groups
    api := app.Group("/api/v1")
    {
        api.GET("/users", listUsers)
        api.POST("/users", createUser)
        api.GET("/users/:id", getUser)
    }

    // Static files
    app.Static("/static", "./public")

    // Start server (graceful shutdown supported)
    log.Fatal(app.Run(":8080"))
}

func listUsers(c *astra.Ctx) error {
    return c.JSON(200, astra.Map{"users": []string{"Alice", "Bob"}})
}

func createUser(c *astra.Ctx) error {
    var user struct {
        Name  string `json:"name" validate:"required"`
        Email string `json:"email" validate:"required,email"`
    }
    if err := c.BindJSON(&user); err != nil {
        return err
    }
    return c.JSON(201, astra.Map{"id": 1, "name": user.Name})
}

func getUser(c *astra.Ctx) error {
    id := c.Param("id")
    return c.JSON(200, astra.Map{"id": id, "name": "Alice"})
}
```

## 📘 Documentation

Full documentation and tutorials are in the [`docs/`](docs/) directory:

| Document | Description |
|----------|-------------|
| [Installation Guide](docs/getting-started.md) | Installation and version selection |
| [Quick Start](docs/quick-start.md) | Three steps to get started |
| [Core Concepts](docs/core-concepts.md) | App, Context, routing, middleware |
| [Configuration](docs/configuration.md) | Config management and hot reload |
| [Middleware](docs/middleware.md) | Built-in and custom middleware |
| [Database ORM](docs/database-orm.md) | GORM integration, read-write separation, transactions |
| [Caching](docs/caching.md) | Unified multi-backend cache interface |
| [Message Queue](docs/message-queue.md) | Multi-broker message middleware |
| [Microservices](docs/microservices.md) | Service discovery, gRPC, distributed transactions |
| [Deployment](docs/deployment.md) | Docker, K8s, Helm |
| [Best Practices](docs/best-practices.md) | Engineering recommendations |

## 📦 Sub-Module Overview

Each sub-module is an independent Go module with its own version:

| Module | Path | Description |
|--------|------|-------------|
| **cache** | `astra/cache` | Cache abstraction: memory/redis/memcached |
| **orm** | `astra/orm` | GORM integration: read-write separation, sharding, transactions |
| **mq** | `astra/mq` | Message queue: Kafka/RabbitMQ/RocketMQ/MQTT/NATS/Pulsar |
| **config** | `astra/config` | Config management: file/env/etcd/Nacos/Apollo |
| **session** | `astra/session` | Session management (Redis backend) |
| **auth** | `astra/auth` | Authentication: OAuth2, RBAC |
| **storage** | `astra/storage` | Object storage: S3/OSS/COS |
| **discovery** | `astra/discovery` | Service discovery: Consul/etcd/K8s/Nacos |
| **grpc** | `astra/grpc` | gRPC server/client |
| **notify** | `astra/notify` | Notifications: email/SMS/push |
| **search** | `astra/search` | Search: Elasticsearch |
| **client** | `astra/client` | HTTP/gRPC client |
| **taskqueue** | `astra/taskqueue` | Task queue |
| **dtx** | `astra/dtx` | Distributed transactions: Saga/TCC |
| **lock** | `astra/lock` | Distributed lock: etcd/redis |
| **loadbalance** | `astra/loadbalance` | Load balancing |
| **runner** | `astra/runner` | Background task scheduling |
| **stream** | `astra/stream` | Stream processing |
| **rule** | `astra/rule` | Rule engine (Lua) |
| **health** | `astra/health` | Health checks |
| **metrics** | `astra/metrics` | Prometheus metrics |
| **otel** | `astra/otel` | OpenTelemetry |
| **cron** | `astra/cron` | Scheduled tasks |
| **di** | `astra/di` | Dependency injection |
| **alert** | `astra/alert` | Alerting system |
| **mongodb** | `astra/mongodb` | MongoDB integration |
| **netengine** | `astra/netengine` | Network engine |

## 🔧 Dependency Injection (`di`)

The `di` package provides a lightweight, generics-based dependency injection container with environment-aware provider sets.

```go
import "github.com/astra-go/astra/di"

// Define provider sets per environment
devSet := di.NewSet("dev",
    // Simple provider — no container dependency
    di.ProvideFunc(func() (*UserRepo, error) {
        return rep.NewMemoryUserRepo(), nil
    }),
)

prodSet := di.NewSet("prod",
    // Factory with container — resolve deps before building target
    di.FactoryFunc(func(c *di.Container) (*UserRepo, error) {
        db, _ := di.Invoke[*sql.DB](c)
        return rep.NewDBUserRepo(db), nil
    }),
)

// Register and resolve
container := di.New()
di.MustRegisterSets(container, "dev", devSet, prodSet)

userRepo, _ := di.Invoke[*UserRepo](container)
_ = userRepo
```

## 🛡️ Service Bootstrap (`boot`)

The `boot` package integrates config loading, structured logging, health endpoints, and graceful shutdown into a single startup scaffold.

```go
import "github.com/astra-go/astra/boot"

func main() {
    svc := boot.New("usercenter-svc",
        boot.WithConfigPath("config/service.yaml"),
        boot.WithEnvPrefix("USC"),
    )

    // Global middleware
    svc.Use(middleware.RequestID())
    svc.UseLogger()
    svc.UseCORS()

    // Register health checkers
    svc.RegisterHealthChecker(&boot.DBChecker{DB: db})
    svc.RegisterHealthChecker(&boot.RedisChecker{Client: redis})

    // Or use the function form for quick checks
    svc.RegisterHealthCheckerFunc("mysql", func(ctx context.Context) error {
        return db.PingContext(ctx)
    })

    // Config hot-reload callback
    svc.Watch(func() {
        svc.Logger().Info("config reloaded", "mode", svc.Cfg().Mode)
    })

    // Register routes
    svc.Router(func(app *astra.App) {
        app.GET("/api/v1/users/:id", getUser)
    })

    svc.Run()
}
```

## 🚨 Structured Error Codes (`errcode`)

The `errcode` package provides a global error registry, auto i18n key derivation, and common error wrappers.

```go
import "github.com/astra-go/astra/errcode"

// Define an error — auto-registers and derives HTTP status + i18n key
// USC-AUTH-1001 → HTTP 401 | i18n key: error.usc.auth.1001
var ErrUserNotFound = errcode.Define("USC-NOTF-2001", "usercenter-svc", "Account not found")
var ErrInvalidToken = errcode.Define("USC-AUTH-1001", "usercenter-svc", "Invalid or expired token")

// In a handler — return with extra context
func getUser(c *astra.Ctx) error {
    user, err := userSvc.Find(c.Request().Context(), c.Param("id"))
    if err != nil {
        return ErrUserNotFound.WithDetails("user_id", c.Param("id"))
    }
    return c.JSON(200, user)
}

// Wrap low-level errors with structured context
func findUser(db *sql.DB, id string) (*User, error) {
    var user User
    err := db.QueryRowContext(ctx, "SELECT * FROM users WHERE id=?", id).Scan(&user)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, ErrUserNotFound.WithDetails("id", id)
        }
        return nil, errcode.WrapDBError(err, "FindUserByID")
    }
    return &user, nil
}

// Registry introspection — generate error catalog at build time or in admin tools
func printErrorCatalog() {
    for _, e := range errcode.ListByService("usercenter-svc") {
        fmt.Printf("[%s] %s — HTTP %d\n", e.Code, e.Description, e.HTTPStatus)
    }
    // Or generate markdown:
    fmt.Println(errcode.MarkdownTable())
}
```

## 📄 License

[MIT License](LICENSE)

---

**Astra** — Built for the stars. ⭐
