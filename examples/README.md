# Astra Examples

Learn step by step through the following paths; every example can be run directly with `go run`.

## Learning Path

```
① hello      — 18 lines, verify installation (5 min)
      ↓
② basic      — Route groups, middleware, request binding and validation (15 min)
      ↓
③ jwt        — JWT auth protecting routes (10 min)
      ↓
④ crud       — Complete CRUD + database (30 min)
      ↓
⑤ orm        — GORM integration, Repository pattern (20 min)
      ↓
⑥ mq         — Message queue, producer + consumer (20 min)
      ↓
⑦ showcase   — Production-grade complete example (reference)
```

---

## Example Descriptions

### ① hello — Minimal Template

```bash
cd hello && go run main.go
curl http://localhost:8080/hello/world
```

Demonstrates: `astra.New()` + routing + `app.Run()`. Gin/Echo-compatible API, no extra concepts.

---

### ② basic — Core Features

```bash
cd basic && go run main.go
```

Demonstrates:
- Global middleware (Recovery / Logger / RequestID / CORS / Timeout)
- Route groups (`app.Group`)
- Path params, query params
- JSON request binding and validation (`c.BindJSON` + `validate` tag)
- Lifecycle hooks (`app.OnStart` / `app.OnStop`)

---

### ③ jwt — JWT Authentication

```bash
cd jwt && go run main.go
# Login to get token
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}'
# Use token to access protected route
curl http://localhost:8080/api/v1/profile \
  -H "Authorization: Bearer <token>"
```

Demonstrates:
- `middleware.JWT` protecting route groups
- Login handler generating JWT
- Public and protected route groups

---

### ④ crud — Complete CRUD

```bash
cd crud && go run main.go
curl -X POST http://localhost:8080/api/v1/items \
  -H "Content-Type: application/json" \
  -d '{"name":"Apple","price":10}'
curl http://localhost:8080/api/v1/items
```

Demonstrates:
- RESTful CRUD interfaces (GET / POST / PUT / DELETE)
- Request binding and validation
- Unified error handling
- In-memory Store (replaceable with real database)

---

### ⑤ orm — GORM Integration

```bash
cd orm && go run main.go
```

Demonstrates:
- `astra/orm` module integrating GORM
- Generic `Repository[T]` pattern
- Transaction helper `RunTx`
- Module system organizing code

> Requires PostgreSQL; connection string configured via `DATABASE_URL` env var.

---

### ⑥ mq — Message Queue

```bash
cd mq && go run main.go
```

Demonstrates:
- `astra/mq` unified interface
- Producer sending messages
- Consumer processing messages
- Graceful shutdown

> Uses NATS by default; requires local `docker run -p 4222:4222 nats`.

---

### ⑦ showcase — Production-Grade Example

Complete production-grade application including:
- Module + DI container organizing dependencies
- OTel distributed tracing
- Prometheus metrics
- gRPC dual-stack
- Adaptive circuit breaker
- Health checks (K8s / Istio)

Suitable as reference template for new projects.

---

## Quick Project Scaffold Generation

Use `astractl` CLI to quickly generate runnable project scaffold:

```bash
# Install CLI
go install github.com/astra-go/astra/cmd/astractl@latest

# Generate new project
astractl new myapp

# Generate CRUD handler
astractl gen crud User --with-service

# Generate handler from proto file
astractl gen proto api/user.proto
```

See [astractl docs](../docs/getting-started/quickstart.md#astractl-cli) for details.
