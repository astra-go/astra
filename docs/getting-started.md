# Installation Guide

## Requirements

- **Go Version**: Go 1.22 or higher (Go 1.25+ recommended for best performance)
- **Operating System**: Linux, macOS, Windows
- **Architecture**: amd64, arm64

## Installing Astra Core

```bash
# Initialize Go module
go mod init myapp

# Install Astra core framework
go get github.com/astra-go/astra@latest
```

## Installing Sub-Modules

Astra uses a monorepo architecture where each sub-module is an independent Go module. Install as needed:

```bash
# Middleware (security module)
go get github.com/astra-go/astra/middleware/security@latest

# Cache
go get github.com/astra-go/astra/cache@latest
go get github.com/astra-go/astra/cache/redis@latest

# ORM
go get github.com/astra-go/astra/orm@latest

# Message Queue
go get github.com/astra-go/astra/mq@latest
go get github.com/astra-go/astra/mq/rabbitmq@latest
go get github.com/astra-go/astra/mq/kafka@latest

# Config Management
go get github.com/astra-go/astra/config@latest

# Session
go get github.com/astra-go/astra/session@latest

# Storage
go get github.com/astra-go/astra/storage@latest
go get github.com/astra-go/astra/storage/s3@latest

# Service Discovery
go get github.com/astra-go/astra/discovery@latest

# Other Modules
go get github.com/astra-go/astra/grpc@latest
go get github.com/astra-go/astra/lock@latest
go get github.com/astra-go/astra/notify@latest
```

## Version Selection

Astra follows semantic versioning. Each sub-module has its own version number:

- **@latest** — Latest stable version
- **@v1.0.x** — Specify major version
- **@v1.0.5** — Specify exact version

```bash
# Install specific version
go get github.com/astra-go/astra@v1.0.5
go get github.com/astra-go/astra/cache/redis@v1.0.5
```

## Verifying Installation

Create `main.go`:

```go
package main

import (
    "fmt"
    "github.com/astra-go/astra"
)

func main() {
    app := astra.New()
    app.GET("/", func(c *astra.Ctx) error {
        return c.String(200, "Astra is running!")
    })
    fmt.Println("Server starting on :8080")
    app.Run(":8080")
}
```

Run it:

```bash
go run main.go
# Visit http://localhost:8080/
# Output: Astra is running!
```

## Using Example Projects

Astra provides several complete example projects:

```bash
git clone https://github.com/astra-go/astra.git
cd astra/examples/quickstart
go run main.go
```

## FAQ

### 1. Module Not Found

Ensure Go version ≥ 1.22, then run:

```bash
go mod tidy
```

### 2. Version Incompatibility

Sub-module versions must match. Check each sub-module's `go.mod` for compatible versions.

### 3. China Mainland Proxy

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```
