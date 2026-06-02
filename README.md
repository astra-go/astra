# Astra

A high-performance Go microservice framework with modular design.

## 📦 Install

### Prerequisites

- Go 1.21+ ([install go](https://go.dev/doc/install))
- Set GOPROXY (for China users):

```bash
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOPRIVATE=github.com/astra-go/astra
```

### Quick Install

```bash
# Install core module
go get github.com/astra-go/astra@v1.0.0

# Install specific sub-modules
go get github.com/astra-go/astra/orm@v1.0.0
go get github.com/astra-go/astra/cache@v1.0.0
go get github.com/astra-go/astra/grpc@v1.0.0
```

### Available Modules

| Module | Import Path | Description |
|--------|-------------|-------------|
| **core** | `github.com/astra-go/astra` | Core framework |
| **orm** | `github.com/astra-go/astra/orm` | ORM (GORM + drivers) |
| **cache** | `github.com/astra-go/astra/cache` | Cache (Redis + in-memory) |
| **grpc** | `github.com/astra-go/astra/grpc` | gRPC server/client |
| **auth** | `github.com/astra-go/astra/auth` | Authentication (JWT + OAuth2) |
| **config** | `github.com/astra-go/astra/config` | Configuration management |
| **discovery** | `github.com/astra-go/astra/discovery` | Service discovery (Consul/etcd/K8s/Nacos) |
| **mq** | `github.com/astra-go/astra/mq` | Message queue (Kafka/RabbitMQ/NATS) |
| **observability** | `github.com/astra-go/astra/observability` | Metrics/tracing/logging |
| **testutil** | `github.com/astra-go/astra/testutil` | Testing utilities |

> See [docs/modules.md](docs/modules.md) for the full list.

## 🚀 Quick Start

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/orm"
    "gorm.io/driver/mysql"
)

func main() {
    // Initialize ORM
    db, _ := orm.Open(mysql.Open("user:pass@/dbname"), &orm.Config{})
    
    // Start server
    app := astra.New()
    app.GET("/ping", func(ctx *astra.Context) {
        ctx.JSON(200, map[string]string{"message": "pong"})
    })
    app.Run(":8080")
}
```

## 📚 Documentation

- [Getting Started](docs/getting-started.md)
- [Module Reference](docs/modules.md)
- [Examples](examples/)
- [Contributing](CONTRIBUTING.md)

## 🛠️ Development Setup

```bash
# Clone the monorepo
git clone https://github.com/astra-go/astra.git
cd astra

# Install dependencies (uses go.work for local development)
go work sync

# Run tests
go test ./...

# Build all modules
make build
```

### Local Development with `replace` Directives

This monorepo uses `replace` directives for local development. After cloning:

```bash
# No manual setup needed — go.work is already configured
go work sync
```

To publish a new version:

```bash
# 1. Clean replace directives
bash scripts/drop-intra-replaces.sh

# 2. Commit clean state
git add -A && git commit -m "chore: clean go.mod for release"

# 3. Release
VERSION=v1.0.0 make release

# 4. Restore replace directives
bash scripts/sync-intra-replaces.sh
```

## 🤝 Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

## 📄 License

Apache License 2.0

## 🔗 Links

- **GitHub**: https://github.com/astra-go/astra
- **Documentation**: https://astra-go.github.io/docs
- **Examples**: [examples/](examples/)
- **Issue Tracker**: https://github.com/astra-go/astra/issues
