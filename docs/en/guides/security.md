# Security Hardening Guide

## Security Middleware Quick Setup

```go
app := astra.New(astra.WithTrustedProxies("10.0.0.0/8"))

// Order matters: Recovery outermost, Logger next
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

## Trusted Proxy Configuration

```go
// Single CDN/LB IP
astra.WithTrustedProxies("203.0.113.10")

// Internal network ranges
astra.WithTrustedProxies("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")

// Cloud provider metadata (example)
astra.WithTrustedProxies("169.254.0.0/16")  // AWS link-local
```

`ClientIP()` traverses `X-Forwarded-For` right-to-left, skipping trusted proxy IPs,
and returns the first untrusted IP. When no trusted proxies are configured, it always returns `RemoteAddr`.

---

## JWT Security Configuration

```go
app.Use(middleware.JWT(middleware.JWTConfig{
    SigningKey:    []byte(os.Getenv("JWT_SECRET")),  // read from environment variable
    SigningMethod: "HS256",
    // RS256 asymmetric signing (recommended for production)
    // SigningKey:    publicKey,
    // SigningMethod: "RS256",

    // Disable clock skew (strict mode)
    Leeway: middleware.StrictJWTLeeway,

    // Custom unauthorized response
    ErrorHandler: func(c contract.Context, err error) error {
        return astra.NewHTTPError(401, "invalid token")
    },
}))
```

---

## CSRF Protection

```go
app.Use(middleware.CSRF(middleware.CSRFConfig{
    TokenLength:    32,
    CookieName:     "_csrf",
    CookieSameSite: http.SameSiteLaxMode,
    CookieSecure:   true,   // HTTPS only
    CookieHTTPOnly: true,
    Header:         "X-CSRF-Token",
    // Skip webhook / API key authenticated routes
    Skipper: func(c contract.Context) bool {
        return strings.HasPrefix(c.Request().URL.Path, "/webhook")
    },
}))
```

---

## Rate Limiting

```go
// Global limit (anti-scraping)
mw, stop := middleware.NewRateLimiter(100, 20)
app.OnStop(func(_ context.Context) error { stop(); return nil })
app.Use(mw)

// Strict limit on login endpoint
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

## IP Filtering

```go
// Admin interface accessible only from internal network
admin := app.Group("/admin",
    middleware.IPFilter(middleware.IPFilterConfig{
        AllowList: []string{"10.0.0.0/8", "127.0.0.1"},
    }),
)
```

---

## Security Response Headers

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

## Request Signature Verification (Webhooks)

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

## Binding DoS Protection

The framework limits slice fields to at most `MaxSliceParams = 1000` elements by default,
preventing memory exhaustion through oversized query string arrays.

For JSON bodies, use the standard library's `http.MaxBytesReader` or the framework's `WithMaxMultipartMemory`:

```go
app := astra.New(astra.WithMaxMultipartMemory(8 << 20))  // 8 MB
```

---

## govulncheck Integration

```bash
# Install
go install golang.org/x/vuln/cmd/govulncheck@latest

# Scan
govulncheck ./...

# CI integration (GitHub Actions)
- uses: golang/govulncheck-action@v1
  with:
    go-version-input: '1.25'
    check-latest: true
```
