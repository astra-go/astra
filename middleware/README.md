# Middleware — HTTP Middleware

Astra's built-in general-purpose HTTP middleware: request logging, Panic recovery, CORS, security headers, and more.

## Features

- **Recovery**: Panic recovery, prevents crashes
- **Logger**: Structured request logging
- **CORS**: Cross-origin resource sharing configuration
- **CSRF**: Cross-site request forgery protection
- **Compress**: Gzip response compression
- **Secure**: Security response headers (X-Frame-Options, HSTS, etc.)
- **CSP**: Content Security Policy
- **RequestID**: Generates unique ID for each request
- **Timeout**: Request timeout control
- **Sanitize**: Input sanitization, removes HTML tags
- **Security Middleware** (`middleware/security`): JWT, API Key, RateLimit, and more

## Quick Start

### Basic Middleware

```go
import "github.com/astra-go/astra/middleware"

app.Use(middleware.Recovery())     // Panic recovery
app.Use(middleware.Logger())       // Request logging
app.Use(middleware.RequestID())    // Request ID
app.Use(middleware.Compress())     // Gzip compression
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

### Security Middleware

```go
import "github.com/astra-go/astra/middleware/security"

// JWT authentication
app.Use(security.JWT(security.JWTConfig{
    Secret: os.Getenv("JWT_SECRET"),
}))

// API Key authentication
app.Use(security.APIKey(security.APIKeyConfig{
    Header: "X-API-Key",
    Query:  "api_key",
}))

// Rate limiting
app.Use(security.RateLimit(security.RateLimitConfig{
    Max:        100,
    Expiration: time.Minute,
}))
```

## API

### Recovery

```go
middleware.Recovery() astra.MiddlewareFunc
// Captures Panic, returns 500 and logs
```

### Logger

```go
middleware.Logger() astra.MiddlewareFunc
// Logs: method, path, status code, latency, client IP, request ID
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
    MaxAge           int  // Seconds
}
```

### Compress

```go
middleware.Compress() astra.MiddlewareFunc
// Auto-recognizes Accept-Encoding, supports Gzip/Deflate/Br
```

### Secure

```go
middleware.Secure() astra.MiddlewareFunc
// Sets X-Frame-Options, X-Content-Type-Options, HSTS, etc.
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
// Sets request ID to Header (X-Request-ID) and Context
```

### Timeout

```go
middleware.Timeout(5 * time.Second) astra.MiddlewareFunc
// Sets request timeout, returns 504 Gateway Timeout on timeout
```

## Security Middleware API

### JWT

```go
security.JWT(cfg JWTConfig) astra.MiddlewareFunc

type JWTConfig struct {
    Secret      string
    TokenLookup string // "header:Authorization" | "query:token"
    SigningMethod string // "HS256" etc.
}
```

### RateLimit

```go
security.RateLimit(cfg RateLimitConfig) astra.MiddlewareFunc

type RateLimitConfig struct {
    Max        int
    Expiration time.Duration
    KeyGenerator func(*astra.Ctx) string // Custom key
    Storage    security.Storage            // Pass for Redis
}
```

## Complete Example

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
)

func main() {
    app := astra.New()

    // Global middleware
    app.Use(middleware.Recovery())
    app.Use(middleware.Logger())
    app.Use(middleware.RequestID())
    app.Use(middleware.CORS(middleware.CORSConfig{
        AllowOrigins: []string{"https://example.com"},
        AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
    }))

    app.GET("/api/hello", func(c *astra.Ctx) error {
        rid, _ := c.Get("request_id") // Get request ID
        return c.JSON(200, astra.Map{"message": "Hello", "request_id": rid})
    })

    app.Run(":8080")
}
```

## Module Dependencies

| Sub-package | Dependency |
|------------|-----------|
| `middleware/security` | `github.com/golang-jwt/jwt/v5` (JWT) |
| `middleware/security` | `github.com/redis/go-redis/v9` (Redis rate limiting) |

## Notes

- `Logger` middleware registered after `Recovery` to ensure logging isn't interrupted by Panic
- In production, `AllowOrigins` in CORS should be explicitly specified; avoid using `*`
- In-memory RateLimit doesn't support distributed rate limiting; Redis storage recommended for production
