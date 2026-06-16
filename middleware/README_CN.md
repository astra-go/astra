# Middleware — HTTP 中间件

Astra 内置通用 HTTP 中间件：请求日志、Panic 恢复、CORS、安全响应头等。

## 特性

- **Recovery**：Panic 恢复，防止崩溃
- **Logger**：结构化请求日志
- **CORS**：跨域资源共享配置
- **CSRF**：跨站请求伪造防护
- **Compress**：Gzip 响应压缩
- **Secure**：安全响应头（X-Frame-Options、HSTS 等）
- **CSP**：Content Security Policy
- **RequestID**：为每个请求生成唯一 ID
- **Timeout**：请求超时控制
- **Sanitize**：输入清理，移除 HTML 标签
- **安全中间件**（`middleware/security`）：JWT、API Key、RateLimit 等

## 快速开始

### 基础中间件

```go
import "github.com/astra-go/astra/middleware"

app.Use(middleware.Recovery())     // Panic 恢复
app.Use(middleware.Logger())       // 请求日志
app.Use(middleware.RequestID())    // 请求 ID
app.Use(middleware.Compress())     // Gzip 压缩
```

### CORS

```go
app.Use(middleware.CORS(middleware.CORSConfig{
    AllowOrigins:     []string{"https://example.com"},
    AllowMethods:    []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Authorization", "Content-Type"},
    AllowCredentials: true,
    MaxAge:          86400,
}))
```

### 安全中间件

```go
import "github.com/astra-go/astra/middleware/security"

// JWT 认证
app.Use(security.JWT(security.JWTConfig{
    Secret: os.Getenv("JWT_SECRET"),
}))

// API Key 认证
app.Use(security.APIKey(security.APIKeyConfig{
    Header: "X-API-Key",
    Query:  "api_key",
}))

// 限流
app.Use(security.RateLimit(security.RateLimitConfig{
    Max:        100,
    Expiration: time.Minute,
}))
```

## API

### Recovery

```go
middleware.Recovery() astra.MiddlewareFunc
// 捕获 Panic，返回 500 并记录日志
```

### Logger

```go
middleware.Logger() astra.MiddlewareFunc
// 记录：方法、路径、状态码、延迟、客户端 IP、请求 ID
```

### CORS

```go
middleware.CORS(cfg CORSConfig) astra.MiddlewareFunc

type CORSConfig struct {
    AllowOrigins     []string
    AllowMethods     []string
    AllowHeaders     []string
    ExposeHeaders    []string
    AllowCredentials bool
    MaxAge           int  // 秒
}
```

### Compress

```go
middleware.Compress() astra.MiddlewareFunc
// 自动识别 Accept-Encoding，支持 Gzip/Deflate/Br
```

### Secure

```go
middleware.Secure() astra.MiddlewareFunc
// 设置 X-Frame-Options、X-Content-Type-Options、HSTS 等
```

### CSP

```go
middleware.CSP(middleware.CSPConfig{
    Directives: map[string]string{
        "default-src": "'self'",
        "script-src":  "'self' 'unsafe-inline'",
    },
})
```

### RequestID

```go
middleware.RequestID() astra.MiddlewareFunc
// 设置请求 ID 到 Header（X-Request-ID）和 Context
```

### Timeout

```go
middleware.Timeout(5 * time.Second) astra.MiddlewareFunc
// 设置请求超时，超时返回 504 Gateway Timeout
```

## 安全中间件 API

### JWT

```go
security.JWT(cfg JWTConfig) astra.MiddlewareFunc

type JWTConfig struct {
    Secret      string
    TokenLookup string // "header:Authorization" | "query:token"
    SigningMethod string // "HS256" 等
}
```

### RateLimit

```go
security.RateLimit(cfg RateLimitConfig) astra.MiddlewareFunc

type RateLimitConfig struct {
    Max        int
    Expiration time.Duration
    KeyGenerator func(*astra.Ctx) string // 自定义 key
    Storage    security.Storage            // 传入 Redis 存储
}
```

## 完整示例

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
)

func main() {
    app := astra.New()

    // 全局中间件
    app.Use(middleware.Recovery())
    app.Use(middleware.Logger())
    app.Use(middleware.RequestID())
    app.Use(middleware.CORS(middleware.CORSConfig{
        AllowOrigins: []string{"https://example.com"},
        AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
    }))

    app.GET("/api/hello", func(c *astra.Ctx) error {
        rid, _ := c.Get("request_id") // 获取请求 ID
        return c.JSON(200, astra.Map{"message": "Hello", "request_id": rid})
    })

    app.Run(":8080")
}
```

## 模块依赖

| 子包 | 依赖 |
|------------|-----------|
| `middleware/security` | `github.com/golang-jwt/jwt/v5`（JWT） |
| `middleware/security` | `github.com/redis/go-redis/v9`（Redis 限流） |

## 注意事项

- `Logger` 中间件应在 `Recovery` 之后注册，确保日志记录不被 Panic 中断
- 生产环境 CORS `AllowOrigins` 应显式指定，避免使用 `*`
- 内存限流不支持分布式限流；生产环境建议使用 Redis 存储