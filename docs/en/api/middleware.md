# Built-in Middleware API

All middleware implements `contract.MiddlewareFunc` (i.e. `func(c contract.Context) error`)
and is mounted via `app.Use(mw)`, route groups, or individual routes.

---

## Logger — Request Logging

```go
app.Use(middleware.Logger())

// Custom format
app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
    Format:   "${time} ${method} ${path} ${status} ${latency}\n",
    Output:   os.Stdout,
    SkipPaths: []string{"/health", "/metrics"},
}))
```

Output fields: `${time}` `${method}` `${path}` `${status}` `${latency}` `${ip}`
`${bytes_in}` `${bytes_out}` `${request_id}`.

---

## Recovery — Panic Recovery

```go
app.Use(middleware.Recovery())

// Custom handler
app.Use(middleware.RecoveryWithConfig(middleware.RecoveryConfig{
    Handler: func(c *astra.Context, val any) error {
        slog.Error("panic", "val", val)
        return astra.NewHTTPError(500, "internal server error")
    },
    PrintStack: true,   // print stack trace in development mode
}))
```

---

## CORS — Cross-Origin Resource Sharing

```go
app.Use(middleware.CORS())   // default: allow all origins (for development)

app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
    AllowOrigins:     []string{"https://example.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Authorization", "Content-Type"},
    AllowCredentials: true,
    MaxAge:           86400,
}))
```

---

## JWT — JSON Web Token Authentication

```go
app.Use(middleware.JWT(middleware.JWTConfig{
    SigningKey:    []byte("secret"),
    SigningMethod: "HS256",
    TokenLookup:  "header:Authorization:Bearer ",
    ContextKey:   "user",            // retrieve via c.Get("user") → *jwt.Token
    Leeway:       5 * time.Second,   // allowed clock skew
    // Leeway: middleware.StrictJWTLeeway  // disable clock skew (v0.9+)
}))
```

#### Accessing Claims

```go
token := c.MustGet("user").(*jwt.Token)
claims := token.Claims.(jwt.MapClaims)
userID := claims["sub"].(string)
```

---

## RateLimit — Token Bucket Rate Limiting

```go
// Simple usage (goroutine runs indefinitely; suitable for top-level apps)
app.Use(middleware.RateLimit(100, 20))   // 100 req/s, burst 20

// Testing / dynamic scenarios (controllable goroutine lifetime)
mw, stop := middleware.NewRateLimiter(100, 20)
defer stop()
app.Use(mw)

// Full configuration
app.Use(middleware.RateLimitWithConfig(middleware.RateLimitConfig{
    Rate:  100,
    Burst: 20,
    KeyFunc: func(c contract.Context) string {
        return c.Header("X-API-Key")   // rate limit per API key
    },
    ExceededHandler: func(c contract.Context) error {
        return astra.NewHTTPError(429, "too many requests")
    },
    Context: ctx,   // controls the cleanup goroutine lifetime
}))
```

---

## Timeout

```go
app.Use(middleware.Timeout(30 * time.Second))
// Returns 503 Service Unavailable on timeout
```

---

## RequestID — Request Tracing

```go
app.Use(middleware.RequestID())
// Read: c.Get(middleware.RequestIDKey) or response header X-Request-ID
```

---

## Gzip — Response Compression

```go
app.Use(middleware.Gzip(middleware.GzipConfig{
    Level:     gzip.DefaultCompression,
    MinLength: 1024,   // skip compression for responses smaller than 1 KB
    SkipPaths: []string{"/stream"},
}))
```

---

## CSRF — Cross-Site Request Forgery Protection

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

// Read the token (inject into templates)
token := c.Get(middleware.CSRFTokenKey).(string)
```

---

## SecureHeaders — Security Response Headers

```go
app.Use(middleware.SecureHeaders())

// Custom configuration
app.Use(middleware.SecureHeadersWithConfig(middleware.SecureHeadersConfig{
    XContentTypeOptions:    "nosniff",
    XFrameOptions:          "DENY",
    StrictTransportSecurity: "max-age=31536000; includeSubDomains",
    ContentSecurityPolicy:  "default-src 'self'",
    ReferrerPolicy:         "strict-origin-when-cross-origin",
}))
```

---

## Pprof — Performance Profiling

```go
// Expose only on an internal management port
admin := astra.New()
admin.Use(middleware.Pprof("/debug/pprof"))
admin.Run(":6060")
```

---

## APIKey — API Key Authentication

```go
app.Use(middleware.APIKey(middleware.APIKeyConfig{
    Validator: func(key string, c contract.Context) (bool, error) {
        return keyStore.Validate(key), nil
    },
    Lookup: "header:X-API-Key,query:api_key",
}))
```

---

## Audit — Audit Logging

```go
app.Use(middleware.Audit(middleware.AuditConfig{
    UserFunc:     func(c contract.Context) string { return c.Get("userID").(string) },
    ActionFunc:   func(c contract.Context) string { return c.Request().Method },
    ResourceFunc: func(c contract.Context) string { return c.Request().URL.Path },
    Writer:       auditLogger,
}))
```

---

## Tenant — Multi-Tenancy

```go
app.Use(middleware.Tenant(middleware.TenantConfig{
    Extractor: func(c contract.Context) (string, error) {
        return c.Header("X-Tenant-ID"), nil
    },
    RateLimits:     map[string]float64{"free": 10, "pro": 1000},
    AllowedOrigins: map[string][]string{"acme": {"https://acme.com"}},
}))

// Read the current tenant
tenantID := c.Get(middleware.TenantIDKey).(string)
```

---

## Canary — Canary Releases

```go
app.Use(middleware.Canary([]middleware.CanaryRule{
    // By Header + regex
    {Header: "X-Canary", HeaderRE: "^1$", Version: "v2"},
    // By Cookie
    {Cookie: "beta", Version: "v2"},
    // By user ID hash (10% of traffic)
    {UserIDKey: "userID", Modulo: 10, Remainder: 0, Version: "v2"},
}))

// Read the matched version (empty string = stable)
ver := c.Get("canary_version").(string)
```

---

## Signature — HMAC Request Signature Verification

```go
app.Use(middleware.Signature(middleware.SignatureConfig{
    Secret:    []byte("webhook-secret"),
    Header:    "X-Signature-256",
    Algorithm: "sha256",
}))
```

---

## CSP — Content Security Policy

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

## IPFilter — IP Allow/Block List

```go
app.Use(middleware.IPFilter(middleware.IPFilterConfig{
    AllowList: []string{"10.0.0.0/8", "127.0.0.1"},  // allow list takes precedence
    BlockList: []string{"1.2.3.4"},
    Forbidden: func(c contract.Context) error {
        return astra.NewHTTPError(403, "forbidden")
    },
}))
```

---

## LongPoll — Long Polling

```go
broker := middleware.NewLongPollBroker()
app.Use(middleware.LongPoll(broker))

// Push an event (server side)
broker.Publish("channel-1", event)

// Client: GET /events?channel=channel-1&timeout=30
```
