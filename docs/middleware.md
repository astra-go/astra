# Middleware

Astra ships with a rich set of built-in middleware covering security, logging, compression, timeout, and more.

## Usage

Middleware can be registered at three levels:

```go
// 1. Global middleware — all routes
app.Use(middleware.Logger())
app.Use(middleware.Recovery())

// 2. Group middleware — this group only
api := app.Group("/api")
api.Use(middleware.CORS("*"))

// 3. Route-level middleware — this route only
app.GET("/admin", adminAuthMiddleware, adminHandler)
```

## Built-in Middleware

### Logger (Request Logging)

```go
import "github.com/astra-go/astra/middleware"

// Default logging (slog)
app.Use(middleware.Logger())

// Custom config
app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
    Format:     "method=${method} path=${path} status=${status} latency=${latency}\n",
    IsProduction: true,
}))
```

Log format placeholders: `${method}`, `${path}`, `${status}`, `${latency}`, `${ip}`, `${ua}`

### Recovery (Panic Recovery)

```go
// Default recovery (production-friendly 500 response)
app.Use(middleware.Recovery())

// Custom config
app.Use(middleware.RecoveryWithConfig(middleware.RecoveryConfig{
    IsProduction: true,    // Hide stack trace in production
    PrintStack:   false,   // Don't print stack
}))
```

### CORS (Cross-Origin)

```go
// Allow specific origin
app.Use(middleware.CORS("https://example.com"))

// Multiple origins
app.Use(middleware.CORS("https://app.com", "https://admin.com"))

// All origins
app.Use(middleware.CORS("*"))

// Fully custom
app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
    AllowOrigins:     []string{"https://example.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Authorization", "Content-Type"},
    ExposeHeaders:    []string{"X-Request-Id"},
    AllowCredentials: true,
    MaxAge:           3600,
}))
```

### CSRF (Cross-Site Request Forgery Protection)

```go
app.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
    Secret:     "your-32-byte-secret-key",
    CookieName: "_csrf",
    HeaderName: "X-CSRF-Token",
    Secure:     true,       // Send only over HTTPS
    SameSite:   http.SameSiteStrictMode,
}))
```

### Compress (Gzip Compression)

```go
app.Use(middleware.Compress())

// Custom level
app.Use(middleware.CompressWithConfig(middleware.CompressConfig{
    Level: gzip.BestSpeed, // BestSpeed / BestCompression / DefaultCompression
}))
```

### RequestID (Request Tracing ID)

```go
app.Use(middleware.RequestID()) // Auto-generates UUID
// Or provide a custom generator

// Response header X-Request-ID
// Accessible via c.GetString("request_id")
```

### Secure (Security Headers)

```go
app.Use(middleware.Secure())
```

Automatically adds security headers:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
- `Strict-Transport-Security: max-age=31536000`

### CSP (Content Security Policy)

```go
app.Use(middleware.CSP(middleware.CSPConfig{
    DefaultSrc: []string{"'self'"},
    ScriptSrc:  []string{"'self'", "https://cdn.example.com"},
    StyleSrc:   []string{"'self'", "'unsafe-inline'"},
    ImgSrc:     []string{"'self'", "data:"},
    ConnectSrc: []string{"'self'"},
    FontSrc:    []string{"'self'"},
}))
```

### Timeout (Request Timeout)

```go
app.Use(middleware.Timeout(30 * time.Second))
```

## Security Middleware (security module)

Security middleware lives in the standalone sub-module `github.com/astra-go/astra/middleware/security`:

```go
import sec "github.com/astra-go/astra/middleware/security"
```

### JWT Authentication

```go
// Using HMAC-SHA256
app.Use(sec.JWT("your-secret-key"))

// Using RSA
app.Use(sec.JWTWithConfig(sec.JWTConfig{
    SigningMethod: jwt.SigningMethodRS256,
    PublicKey:     publicKey,
    // Or load from file
    // PublicKeyPath: "public.pem",
}))

// Get user info
app.GET("/profile", func(c *astra.Ctx) error {
    claims := sec.GetClaims(c)
    return c.JSON(200, claims)
})
```

### JWT Revocation (Blacklist)

```go
// JWT revocation via Redis cache
import (
    cacheredis "github.com/astra-go/astra/cache/redis"
    sec "github.com/astra-go/astra/middleware/security"
)

redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
cache, _ := cacheredis.New(cacheredis.Config{Client: redisClient})

app.Use(sec.JWTWithConfig(sec.JWTConfig{
    SigningKey:  "secret",
    RevokeCache: cache, // Enable revocation check
}))

// Revoke on logout
app.POST("/logout", func(c *astra.Ctx) error {
    claims := sec.GetClaims(c)
    sec.RevokeToken(store, c.GetString("jwt_raw_token"), int64(expireAt))
    return c.JSON(200, astra.Map{"ok": true})
})
```

### Generate JWT

```go
import "github.com/golang-jwt/jwt/v5"

claims := jwt.MapClaims{
    "sub":   "user_42",
    "aud":   []string{"myapp"},
    "role":  "admin",
    "name":  "Alice",
}

token, err := sec.GenerateJWT(claims, "your-secret-key")
```

### API Key Authentication

```go
app.Use(sec.APIKey(sec.APIKeyConfig{
    Keys: map[string]string{
        "sk-live-abc123": "project-alpha",
        "sk-live-def456": "project-beta",
    },
    Header: "X-API-Key", // Read from request header
}))

// Get API key info in handler
app.GET("/projects", func(c *astra.Ctx) error {
    project := c.GetString("api_key_project")
    return c.JSON(200, astra.Map{"project": project})
})
```

### Signature Verification

```go
app.Use(sec.Signature(sec.SignatureConfig{
    Secret:  "shared-secret",
    Header:  "X-Signature",      // Signature header
    TTL:     5 * time.Minute,    // Signature validity period
    Method:  sec.SignHMACSHA256, // Signature algorithm
}))
```

### IP Filtering

```go
// Whitelist mode
app.Use(sec.IPFilter(sec.IPFilterConfig{
    AllowIPs: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.1.0/24", "127.0.0.1"},
    Mode:     sec.Whitelist,
}))

// Blacklist mode
app.Use(sec.IPFilter(sec.IPFilterConfig{
    DenyIPs: []string{"10.0.0.5"},
    Mode:    sec.Blacklist,
}))
```

### Rate Limiting

```go
// In-memory rate limiting
app.Use(sec.RateLimit(sec.RateLimitConfig{
    Rate:   100,                 // Per second
    Burst:  200,                 // Burst cap
    TTL:    1 * time.Minute,    // Window
}))

// Redis distributed rate limiting (shared across instances)
import sec "github.com/astra-go/astra/middleware/security"

// Use Redis backend (requires -tags redis build)
app.Use(sec.DistributedRateLimit(redisAddr, 1000, 2000))

// Per-user/IP rate limiting
app.Use(sec.RateLimitWithConfig(sec.RateLimitConfig{
    Rate:  100,
    Burst: 200,
    KeyFunc: func(c *astra.Ctx) string {
        return c.ClientIP() // Rate limit by IP
    },
}))
```

### Tenant Isolation (Multi-Tenant)

```go
// Read tenant ID from request header (default Header: X-Tenant-ID)
app.Use(sec.Tenant(sec.TenantConfig{
    Header: "X-Tenant-ID",
}))

// Get in handler
app.GET("/data", func(c *astra.Ctx) error {
    tenantID := sec.TenantID(c)
    return c.JSON(200, astra.Map{"tenant": tenantID})
})
```

### Canary Release

```go
app.Use(sec.Canary([]sec.CanaryRule{
    // Exact match via request header
    {Header: "X-Canary", HeaderRE: "^v2$", Version: "v2"},
    // 10% user canary (by user_id hash, same user always lands in same group)
    {UserIDKey: "user_id", Modulo: 10, Remainder: 0, Version: "canary"},
}))

// Read version in handler
app.GET("/data", func(c *astra.Ctx) error {
    version, _ := c.Get("canary_version")
    if version == "canary" {
        return handleCanary(c)
    }
    return handleStable(c)
})
```

## Custom Middleware

```go
// Simple middleware
func MyMiddleware(c *astra.Ctx) error {
    start := time.Now()
    err := c.Next()
    log.Printf("%s %s took %v", c.Method(), c.Path(), time.Since(start))
    return err
}

// Middleware with config
type MyConfig struct {
    Header string
}

func MyMiddlewareWithConfig(config MyConfig) astra.MiddlewareFunc {
    return func(c *astra.Ctx) error {
        if config.Header != "" {
            c.SetHeader(config.Header, "value")
        }
        return c.Next()
    }
}

// Skipper: skip certain requests
func SkipMiddleware(c *astra.Ctx) bool {
    return c.Path() == "/health" // Skip health check
}
```

## Middleware Execution Order

Middleware executes in registration order, forming an onion model:

```go
app.Use(middleware.Logger())    // 1. First
app.Use(middleware.CORS("*"))   // 2.
app.Use(middleware.Auth())      // 3.

app.GET("/data", handler)       // 4. Handler last
```

Execution flow: `Logger → CORS → Auth → Handler → Auth → CORS → Logger` (return path)
