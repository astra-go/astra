# Security — Security Middleware

Enterprise-grade security middleware collection.

## Available Middleware

| Middleware | Description |
|-----------|-------------|
| `JWT()` | JWT authentication (HMAC/RSA/ECDSA) |
| `APIKey()` | API Key authentication |
| `Signature()` | Signature verification (HMAC-SHA256) |
| `RateLimit()` | Rate limiting (in-memory) |
| `RateLimitRedis()` | Distributed rate limiting (Redis) |
| `IPFilter()` | IP whitelist/blacklist |
| `Tenant()` | Multi-tenant support |
| `Canary()` | Canary release / gray release |

## Usage

```go
import sec "github.com/astra-go/astra/middleware/security"

app.Use(sec.JWT("secret"))
app.Use(sec.RateLimitRedis(redisClient, sec.RateLimitConfig{Rate: 100}))
```
