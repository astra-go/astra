# Deployment Guide

## Standard HTTP Deployment

```dockerfile
# Dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o server ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
```

```bash
docker build -t myapp:latest .
docker run -p 8080:8080 myapp:latest
```

---

## Reactor Engine Deployment (High Concurrency)

```go
// Replace the standard server
app.RunReactor(":8080")
```

The Reactor engine uses epoll on Linux and kqueue on macOS.
Recommended for **long connections + high concurrency** scenarios (WebSocket, SSE, IM).

**Kubernetes Deployment recommendations**:

```yaml
resources:
  requests:
    cpu: "500m"
    memory: "128Mi"
  limits:
    cpu: "2"
    memory: "512Mi"
# Reactor uses a small number of goroutines; memory footprint is far lower
# than net/http's one-goroutine-per-connection model
```

---

## HTTP/3 (QUIC) Deployment

```go
app.RunQUIC(":443", "/certs/tls.crt", "/certs/tls.key")
```

**Requirements**:
- UDP port 443 open to the internet (firewall rules)
- Valid TLS certificate (QUIC mandates TLS 1.3)
- Linux kernel ≥ 5.4 recommended (BBR congestion control)

---

## Health Check Configuration

```go
health.Register(app,
    health.Check{Name: "db",    Check: dbPing},
    health.Check{Name: "redis", Check: redisPing},
)
health.RegisterIstioProbes(app)
```

Kubernetes liveness / readiness probes:

```yaml
livenessProbe:
  httpGet:
    path: /live
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 3
  periodSeconds: 5
```

---

## Graceful Shutdown

All `Run*` methods perform graceful shutdown automatically upon receiving `SIGINT` / `SIGTERM`:

1. Stop accepting new connections
2. Wait for in-flight requests to complete (up to 30 s)
3. Call `OnStop` hooks (serially)
4. Exit

```go
// Custom shutdown timeout
app.RunServer(&http.Server{
    Addr:              ":8080",
    Handler:           app,
    ReadHeaderTimeout: 5 * time.Second,
    WriteTimeout:      30 * time.Second,
    IdleTimeout:       120 * time.Second,
})
```
