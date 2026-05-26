# 内置中间件 API

所有中间件均实现 `contract.MiddlewareFunc`（即 `func(c contract.Context) error`），
通过 `app.Use(mw)` 或路由组/单路由挂载。

---

## Logger — 请求日志

```go
app.Use(middleware.Logger())

// 自定义格式
app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
    Format:   "${time} ${method} ${path} ${status} ${latency}\n",
    Output:   os.Stdout,
    SkipPaths: []string{"/health", "/metrics"},
}))
```

输出字段：`${time}` `${method}` `${path}` `${status}` `${latency}` `${ip}`
`${bytes_in}` `${bytes_out}` `${request_id}`。

---

## Recovery — Panic 恢复

```go
app.Use(middleware.Recovery())

// 自定义处理器
app.Use(middleware.RecoveryWithConfig(middleware.RecoveryConfig{
    Handler: func(c *astra.Context, val any) error {
        slog.Error("panic", "val", val)
        return astra.NewHTTPError(500, "internal server error")
    },
    PrintStack: true,   // 开发模式打印堆栈
}))
```

---

## CORS — 跨域

```go
app.Use(middleware.CORS())   // 默认允许所有来源（开发用）

app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
    AllowOrigins:     []string{"https://example.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Authorization", "Content-Type"},
    AllowCredentials: true,
    MaxAge:           86400,
}))
```

---

## JWT — JSON Web Token 认证

```go
app.Use(middleware.JWT(middleware.JWTConfig{
    SigningKey:    []byte("secret"),
    SigningMethod: "HS256",
    TokenLookup:  "header:Authorization:Bearer ",
    ContextKey:   "user",            // c.Get("user") 获取 *jwt.Token
    Leeway:       5 * time.Second,   // 允许的时钟偏差
    // Leeway: middleware.StrictJWTLeeway  // 禁用时钟偏差（v0.9+）
}))
```

#### 获取声明

```go
token := c.MustGet("user").(*jwt.Token)
claims := token.Claims.(jwt.MapClaims)
userID := claims["sub"].(string)
```

---

## RateLimit — 限流（令牌桶）

```go
// 简单用法（goroutine 永不退出，适合顶层应用）
app.Use(middleware.RateLimit(100, 20))   // 100 req/s, burst 20

// 测试/动态场景（可控 goroutine 生命周期）
mw, stop := middleware.NewRateLimiter(100, 20)
defer stop()
app.Use(mw)

// 完整配置
app.Use(middleware.RateLimitWithConfig(middleware.RateLimitConfig{
    Rate:  100,
    Burst: 20,
    KeyFunc: func(c contract.Context) string {
        return c.Header("X-API-Key")   // 按 API Key 限流
    },
    ExceededHandler: func(c contract.Context) error {
        return astra.NewHTTPError(429, "too many requests")
    },
    Context: ctx,   // 控制 cleanup goroutine 生命周期
}))
```

---

## Timeout — 超时

```go
app.Use(middleware.Timeout(30 * time.Second))
// 超时后返回 503 Service Unavailable
```

---

## RequestID — 请求追踪

```go
app.Use(middleware.RequestID())
// 读取：c.Get(middleware.RequestIDKey) 或响应头 X-Request-ID
```

---

## Gzip — 响应压缩

```go
app.Use(middleware.Gzip(middleware.GzipConfig{
    Level:     gzip.DefaultCompression,
    MinLength: 1024,   // 小于 1KB 不压缩
    SkipPaths: []string{"/stream"},
}))
```

---

## CSRF — 跨站请求伪造防护

```go
app.Use(middleware.CSRF(middleware.CSRFConfig{
    TokenLength: 32,
    CookieName:  "_csrf",
    Header:      "X-CSRF-Token",
    SameSite:    http.SameSiteLaxMode,
    Skipper:     func(c contract.Context) bool {
        return c.Request().URL.Path == "/webhook"
    },
}))

// 读取令牌（注入到模板）
token := c.Get(middleware.CSRFTokenKey).(string)
```

---

## SecureHeaders — 安全响应头

```go
app.Use(middleware.SecureHeaders())

// 自定义
app.Use(middleware.SecureHeadersWithConfig(middleware.SecureHeadersConfig{
    XContentTypeOptions:    "nosniff",
    XFrameOptions:          "DENY",
    StrictTransportSecurity: "max-age=31536000; includeSubDomains",
    ContentSecurityPolicy:  "default-src 'self'",
    ReferrerPolicy:         "strict-origin-when-cross-origin",
}))
```

---

## Pprof — 性能分析

```go
// 仅在内部管理端口暴露
admin := astra.New()
admin.Use(middleware.Pprof("/debug/pprof"))
admin.Run(":6060")
```

---

## APIKey — 接口密钥认证

```go
app.Use(middleware.APIKey(middleware.APIKeyConfig{
    Validator: func(key string, c contract.Context) (bool, error) {
        return keyStore.Validate(key), nil
    },
    Lookup: "header:X-API-Key,query:api_key",
}))
```

---

## Audit — 审计日志

```go
app.Use(middleware.Audit(middleware.AuditConfig{
    UserFunc:     func(c contract.Context) string { return c.Get("userID").(string) },
    ActionFunc:   func(c contract.Context) string { return c.Request().Method },
    ResourceFunc: func(c contract.Context) string { return c.Request().URL.Path },
    Writer:       auditLogger,
}))
```

---

## Tenant — 多租户

```go
app.Use(middleware.Tenant(middleware.TenantConfig{
    Extractor: func(c contract.Context) (string, error) {
        return c.Header("X-Tenant-ID"), nil
    },
    RateLimits:     map[string]float64{"free": 10, "pro": 1000},
    AllowedOrigins: map[string][]string{"acme": {"https://acme.com"}},
}))

// 读取当前租户
tenantID := c.Get(middleware.TenantIDKey).(string)
```

---

## Canary — 灰度发布

```go
app.Use(middleware.Canary([]middleware.CanaryRule{
    // 按 Header + 正则
    {Header: "X-Canary", HeaderRE: "^1$", Version: "v2"},
    // 按 Cookie
    {Cookie: "beta", Version: "v2"},
    // 按用户 ID 哈希（10% 流量）
    {UserIDKey: "userID", Modulo: 10, Remainder: 0, Version: "v2"},
}))

// 读取命中版本（空字符串 = stable）
ver := c.Get("canary_version").(string)
```

---

## Signature — HMAC 请求签名

```go
app.Use(middleware.Signature(middleware.SignatureConfig{
    Secret:    []byte("webhook-secret"),
    Header:    "X-Signature-256",
    Algorithm: "sha256",
}))
```

---

## CSP — 内容安全策略

```go
app.Use(middleware.CSP(middleware.CSPConfig{
    Directives: map[string][]string{
        "default-src": {"'self'"},
        "script-src":  {"'self'", "cdn.example.com"},
        "img-src":     {"'self'", "data:"},
    },
    ReportURI: "/csp-report",
}))
```

---

## IPFilter — IP 黑白名单

```go
app.Use(middleware.IPFilter(middleware.IPFilterConfig{
    AllowList: []string{"10.0.0.0/8", "127.0.0.1"},  // 白名单优先
    BlockList: []string{"1.2.3.4"},
    Forbidden: func(c contract.Context) error {
        return astra.NewHTTPError(403, "forbidden")
    },
}))
```

---

## LongPoll — 长轮询

```go
broker := middleware.NewLongPollBroker()
app.Use(middleware.LongPoll(broker))

// 推送事件（服务端）
broker.Publish("channel-1", event)

// 客户端 GET /events?channel=channel-1&timeout=30
```
