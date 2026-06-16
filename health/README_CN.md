# Health — 健康检查与就绪探针

提供 HTTP 健康检查端点、自定义探针和 Kubernetes/Istio 兼容的 liveness/readiness 探针。

## 特性

- **健康检查端点**：`/health`（存活探针）和 `/ready`（就绪探针）
- **自定义探针**：支持注册任意检查函数（数据库、Redis、外部服务）
- **K8s 兼容**：支持 Kubernetes `/healthz/live` 和 `/healthz/ready` 路径
- **Istio 兼容**：支持 Istio `istio.health` 注解
- **分组检查**：检查按服务组件分组；每组独立决定状态

## 快速开始

```go
import "github.com/astra-go/astra/health"

app := astra.New()

// 注册自定义探针
app.Register(health.New(
    health.WithProbe("mysql", func(ctx context.Context) error {
        return db.PingContext(ctx)
    }),
    health.WithProbe("redis", func(ctx context.Context) error {
        return redis.Ping(ctx).Err()
    }),
    health.WithProbe("downstream", func(ctx context.Context) error {
        _, err := http.Get("http://downstream/health")
        return err
    }),
))

app.GET("/health", health.Handler())
app.GET("/ready", health.ReadyHandler())
```

## API

### New

```go
func New(opts ...Option) *Health
```

`Option` 支持链式注册。

### WithProbe

```go
func WithProbe(name string, fn health.CheckFunc) Option

type CheckFunc func(ctx context.Context) error
// nil = 健康，非 nil = 不健康
```

### Handler / ReadyHandler

```go
// /health — 存活探针（App 是否存活），检查关键依赖
func Handler() astra.HandlerFunc

// /ready — 就绪探针（能否接收流量），检查所有依赖
func ReadyHandler() astra.HandlerFunc
```

### WithChecker（高级）

```go
// 添加完整的 Checker（非单个函数）
func WithChecker(name string, c Checker) Option

type Checker interface {
    Check(ctx context.Context) error // nil = 健康
}
```

## 配置

### 探针超时

默认探针超时为 5 秒；在 `CheckFunc` 中使用带超时的 context：

```go
health.WithProbe("slow-service", func(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    return service.Ping(ctx)
})
```

## 完整示例

```go
package main

import (
    "context"
    "net/http"
    "time"

    "github.com/astra-go/astra"
    "github.com/astra-go/astra/health"
)

func main() {
    app := astra.New()

    h := health.New(
        // 基础存活：App 是否运行
        health.WithProbe("app", func(ctx context.Context) error {
            return nil
        }),
        // 就绪：数据库连接
        health.WithProbe("db", func(ctx context.Context) error {
            return db.PingContext(ctx)
        }),
        // 就绪：缓存
        health.WithProbe("cache", func(ctx context.Context) error {
            return redis.Ping(ctx).Err()
        }),
    )

    // 注册路由
    app.GET("/health", h.Handler())      // 存活（只检查 App）
    app.GET("/ready", h.ReadyHandler())  // 就绪（检查所有）

    app.Run(":8080")
}
```

## Kubernetes 部署配置示例

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
```

## 模块依赖

无外部依赖。

## 注意事项

- `/health` 只检查 App 本身（应始终返回 200），`/ready` 检查所有依赖
- 探针函数应快速返回，避免探针超时影响判断
- 就绪探针返回非 200 时，Kubernetes 将 Pod 从 Service 中移除直至恢复