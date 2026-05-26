# 部署指南

## 标准 HTTP 部署

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

## Reactor 引擎部署（高并发）

```go
// 替换标准服务器
app.RunReactor(":8080")
```

Reactor 引擎在 Linux 上使用 epoll，macOS 上使用 kqueue，
推荐用于 **长连接 + 高并发** 场景（如 WebSocket、SSE、IM）。

**Kubernetes Deployment 建议**：

```yaml
resources:
  requests:
    cpu: "500m"
    memory: "128Mi"
  limits:
    cpu: "2"
    memory: "512Mi"
# Reactor 使用少量 goroutine，内存占用远低于 net/http 每连接一 goroutine 模式
```

---

## HTTP/3 (QUIC) 部署

```go
app.RunQUIC(":443", "/certs/tls.crt", "/certs/tls.key")
```

**要求**：
- UDP 端口 443 对外开放（防火墙规则）
- TLS 证书有效（QUIC 强制 TLS 1.3）
- Linux kernel ≥ 5.4 推荐（BBR 拥塞控制）

---

## 健康检查配置

```go
health.Register(app,
    health.Check{Name: "db",    Check: dbPing},
    health.Check{Name: "redis", Check: redisPing},
)
health.RegisterIstioProbes(app)
```

Kubernetes liveness / readiness probe：

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

## 优雅关闭

所有 `Run*` 方法在收到 `SIGINT` / `SIGTERM` 后自动执行优雅关闭：

1. 停止接受新连接
2. 等待 in-flight 请求完成（最长 30 s）
3. 调用 `OnStop` 钩子（串行）
4. 退出

```go
// 自定义关闭超时
app.RunServer(&http.Server{
    Addr:              ":8080",
    Handler:           app,
    ReadHeaderTimeout: 5 * time.Second,
    WriteTimeout:      30 * time.Second,
    IdleTimeout:       120 * time.Second,
})
```
