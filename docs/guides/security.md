# 安全加固指南

## 安全中间件快速配置

```go
app := astra.New(astra.WithTrustedProxies("10.0.0.0/8"))

// 顺序重要：Recovery 最外层，Logger 次之
app.Use(
    middleware.Recovery(),
    middleware.Logger(),
    middleware.RequestID(),
    middleware.SecureHeaders(),
    middleware.CORS(corsCfg),
    middleware.CSRF(csrfCfg),
)
```

---

## 可信代理配置

```go
// 单个 CDN/LB IP
astra.WithTrustedProxies("203.0.113.10")

// 内网网段
astra.WithTrustedProxies("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")

// 云提供商元数据（示例）
astra.WithTrustedProxies("169.254.0.0/16")  // AWS 链路本地
```

`ClientIP()` 从 `X-Forwarded-For` 右到左遍历，跳过可信代理 IP，
返回第一个不可信的 IP。未配置可信代理时，始终返回 `RemoteAddr`。

---

## JWT 安全配置

```go
app.Use(middleware.JWT(middleware.JWTConfig{
    SigningKey:    []byte(os.Getenv("JWT_SECRET")),  // 从环境变量读取
    SigningMethod: "HS256",
    // RS256 非对称签名（推荐生产）
    // SigningKey:    publicKey,
    // SigningMethod: "RS256",

    // 禁用时钟偏差（严格模式）
    Leeway: middleware.StrictJWTLeeway,

    // 自定义未授权响应
    ErrorHandler: func(c contract.Context, err error) error {
        return astra.NewHTTPError(401, "invalid token")
    },
}))
```

### 高安全场景：强制使用 StrictJWTLeeway

`DefaultJWTLeeway`（5s）容忍时钟漂移，但同时为令牌提供了最多 5 秒的"续命窗口"。
对于以下场景，**必须**将 `Leeway` 设为 `StrictJWTLeeway`，否则过期令牌在窗口内仍可被重放：

| 场景 | 风险 | 说明 |
|------|------|------|
| 密码重置链接 | 高 | 令牌一次性使用，5s 窗口允许攻击者抢先重放 |
| 邮件地址验证 | 高 | 同上；一旦被验证，同一令牌不应再次有效 |
| 短信 / 邮件 OTP | 高 | 一次性令牌严禁宽限 |
| 支付/转账授权 | 极高 | 财务操作不允许任何重放窗口 |
| 设备绑定 / 解绑 | 高 | 操作不可逆，不应存在容错窗口 |

```go
// 密码重置接口 — 必须使用 StrictJWTLeeway
app.POST("/auth/reset-password",
    middleware.JWTWithConfig(middleware.JWTConfig{
        Secret: os.Getenv("RESET_TOKEN_SECRET"),
        Leeway: middleware.StrictJWTLeeway, // 禁止任何时钟宽限
    }),
    resetPasswordHandler,
)

// 邮件验证接口 — 同上
app.GET("/auth/verify-email",
    middleware.JWTWithConfig(middleware.JWTConfig{
        Secret: os.Getenv("EMAIL_TOKEN_SECRET"),
        Leeway: middleware.StrictJWTLeeway,
    }),
    verifyEmailHandler,
)
```

> **注意**：`StrictJWTLeeway` 不替代令牌撤销机制。对于一次性令牌，验证通过后应
> 立即将其标记为已使用（例如写入 Redis/DB），阻止同一令牌在有效期内被再次提交。

---

## CORS 防护

> **生产环境警告**：`middleware.CORS()` 使用 `DefaultCORSConfig`（`AllowOrigins: ["*"]`），
> 允许任意域发起跨域请求。生产环境**必须**使用 `CORSStrict` 或 `CORSWithConfig` 配置明确的白名单域名。

```go
// ✅ 生产环境推荐：CORSStrict 要求显式来源，禁止传入 "*"
app.Use(middleware.CORSStrict(
    "https://app.example.com",
    "https://admin.example.com",
))

// ✅ 需要 AllowCredentials 等高级选项时使用完整配置
app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
    AllowOrigins:     []string{"https://app.example.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Authorization", "Content-Type"},
    AllowCredentials: true,
    MaxAge:           86400,
}))

// ⚠ 仅开发环境
app.Use(middleware.CORS())
```

---

## CSRF 防护

```go
app.Use(middleware.CSRF(middleware.CSRFConfig{
    TokenLength:    32,
    CookieName:     "_csrf",
    CookieSameSite: http.SameSiteLaxMode,
    CookieSecure:   true,   // HTTPS only
    CookieHTTPOnly: true,
    Header:         "X-CSRF-Token",
    // 跳过 webhook / API key 鉴权路由
    Skipper: func(c contract.Context) bool {
        return strings.HasPrefix(c.Request().URL.Path, "/webhook")
    },
}))
```

---

## 速率限制

```go
// 全局限制（防爬虫）
mw, stop := middleware.NewRateLimiter(100, 20)
app.OnStop(func(_ context.Context) error { stop(); return nil })
app.Use(mw)

// 登录接口严格限制
loginGroup := app.Group("/auth",
    middleware.RateLimitWithConfig(middleware.RateLimitConfig{
        Rate:  5,     // 5 req/s
        Burst: 3,
        KeyFunc: func(c contract.Context) string {
            return c.ClientIP()
        },
    }),
)
loginGroup.POST("/login", loginHandler)
```

---

## IP 过滤

```go
// 管理接口仅内网访问
admin := app.Group("/admin",
    middleware.IPFilter(middleware.IPFilterConfig{
        AllowList: []string{"10.0.0.0/8", "127.0.0.1"},
    }),
)
```

---

## 安全响应头

```go
app.Use(middleware.SecureHeadersWithConfig(middleware.SecureHeadersConfig{
    XContentTypeOptions:      "nosniff",
    XFrameOptions:            "DENY",
    XXSSProtection:           "1; mode=block",
    StrictTransportSecurity:  "max-age=63072000; includeSubDomains; preload",
    ReferrerPolicy:           "strict-origin-when-cross-origin",
    ContentSecurityPolicy:    "default-src 'self'; script-src 'self' 'nonce-{NONCE}'",
}))
```

---

## 请求签名验证（Webhook）

```go
app.POST("/webhook/github",
    middleware.Signature(middleware.SignatureConfig{
        Secret:    []byte(os.Getenv("GITHUB_WEBHOOK_SECRET")),
        Header:    "X-Hub-Signature-256",
        Algorithm: "sha256",
        Prefix:    "sha256=",
    }),
    githubWebhookHandler,
)
```

---

## 绑定 DoS 防护

框架默认限制 slice 字段最多 `MaxSliceParams = 1000` 个元素，
防止通过超大 query string 数组耗尽内存。

对于 JSON body，使用标准库的 `http.MaxBytesReader` 或框架的 `WithMaxMultipartMemory`：

```go
app := astra.New(astra.WithMaxMultipartMemory(8 << 20))  // 8 MB
```

---

## govulncheck 集成

```bash
# 安装
go install golang.org/x/vuln/cmd/govulncheck@latest

# 扫描
govulncheck ./...

# CI 集成（GitHub Actions）
- uses: golang/govulncheck-action@v1
  with:
    go-version-input: '1.25'
    check-latest: true
```
