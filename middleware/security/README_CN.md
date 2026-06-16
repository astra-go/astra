# Security — 企业级安全中间件集合

## 可用中间件

| 中间件 | 说明 |
|-----------|-------------|
| `JWT()` | JWT 认证（HMAC/RSA/ECDSA） |
| `APIKey()` | API Key 认证 |
| `Signature()` | 签名校验（HMAC-SHA256） |
| `RateLimit()` | 限流（内存） |
| `RateLimitRedis()` | 分布式限流（Redis） |
| `IPFilter()` | IP 白名单/黑名单 |
| `Tenant()` | 多租户支持 |
| `Canary()` | 金丝雀发布/灰度发布 |

## 使用方式

```go
import sec "github.com/astra-go/astra/middleware/security"

app.Use(sec.JWT("secret"))
app.Use(sec.RateLimitRedis(redisClient, sec.RateLimitConfig{Rate: 100}))
```