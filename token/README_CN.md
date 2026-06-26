# Token — Access/Refresh Token 生命周期管理

[![Go Reference](https://pkg.go.dev/badge/github.com/astra-go/astra/token.svg)](https://pkggo.dev/github.com/astra-go/astra/token)

Package `token` 为 Astra 应用提供完整的 Token 生命周期管理。它负责 Access + Refresh Token 对的创建、存储、查找和撤销。

## 何时使用本包

本包与 [`astra/middleware/security/jwt`](https://github.com/astra-go/astra/tree/main/middleware/security/jwt.go) 互补：

| 职责 | 包 |
|------|---|
| **签名验证**（JWT verify） | `astra/middleware/security` — JWT 中间件 |
| **Token 生命周期**（签发、存储、撤销） | `astra/token` — 本包 |
| **重放攻击防护**（JTI） | `astra/token` — 本包 |
| **单点登出**（SSO） | `astra/token` — 本包 |

## 功能特性

- **不透明令牌** — 存储前经 SHA-256 哈希，原始 Token 永不落盘
- **双令牌模型** — 短期 Access + 长期 Refresh Token 对
- **SSO 强制** — 签发新 Token 时自动失效该用户的旧 Token
- **黑名单机制** — 撤销单个 Token（登出、改密时使用）
- **JTI 防重放** — 防止一次性 Token 被重复使用
- **双后端支持** — `MemoryManager` 用于开发/测试，`RedisManager` 用于生产

## 快速开始

```go
import (
    "context"
    "time"
    "github.com/astra-go/astra/token"
)

func main() {
    ctx := context.Background()

    // Memory 后端（开发 / 测试）
    mgr := token.NewMemoryManager()

    // 为用户 42 签发 Token 对
    access, refresh, entry, err := mgr.Issue(ctx, 42, 15*time.Minute, 7*24*time.Hour)
    if err != nil {
        panic(err)
    }

    // 按 Access Token 查找
    found, _ := mgr.FindByAccess(ctx, access)
    if found != nil {
        // token 有效
    }

    // 登出时撤销
    mgr.Blacklist(ctx, access, 15*time.Minute)

    // SSO: 下次签发会失效之前的 Token
    mgr.Issue(ctx, 42, 15*time.Minute, 7*24*time.Hour)

    // JTI 防重放
    mgr.SetJTI(ctx, "some-jti", 5*time.Minute)
    used, _ := mgr.IsJTIUsed(ctx, "some-jti")
}
```

## Redis 后端（生产环境）

```go
import (
    "github.com/astra-go/astra/token"
    "github.com/redis/go-redis/v9"
)

rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
mgr := token.NewRedisManager(rdb, "myapp:token:")

// API 与 MemoryManager 完全一致
access, refresh, entry, _ := mgr.Issue(ctx, 42, 15*time.Minute, 7*24*time.Hour)
```

## 架构图

```
  Client                     Token Manager                    Redis / Memory
    │                             │                               │
    │  POST /auth/login           │                               │
    │────────────────────────────>│                               │
    │                             │──── Issue(42, 15m, 7d) ─────>│
    │<── {access, refresh} ──────│                               │
    │                             │                               │
    │  GET /api/resource          │                               │
    │  Authorization: Bearer xxx  │                               │
    │────────────────────────────>│                               │
    │                             │──── FindByAccess(access) ────>│
    │<── 200 OK ─────────────────│                               │
    │                             │                               │
    │  POST /auth/logout          │                               │
    │────────────────────────────>│                               │
    │                             │──── Blacklist(access, 15m) ─>│
    │<── 200 OK ─────────────────│                               │
```

## 最佳实践

1. **短期 Access TTL** — 15–30 分钟。更长的会话请使用 Refresh Token。
2. **黑名单 TTL 要匹配** — 调用 `Blacklist` 时，传入 Access Token 的剩余有效期。
3. **关键操作使用 JTI** — 密码重置、邮箱修改、支付确认等场景。
4. **敏感应用启用 SSO** — `Issue` 会自动失效同一用户 ID 的旧 Token。

## API 参考

### Manager 接口

```go
type Manager interface {
    // Issue 创建新的 Token 对。如果用户已有活跃 Token，会被替换（SSO）。
    Issue(ctx context.Context, uin int64, accessTTL, refreshTTL time.Duration) (accessToken string, refreshToken string, entry *Entry, err error)

    // FindByAccess 按 Access Token 查找。黑名单中的 Token 返回 nil。
    FindByAccess(ctx context.Context, token string) (*Entry, error)

    // FindByRefresh 按 Refresh Token 查找。
    FindByRefresh(ctx context.Context, refreshToken string) (*Entry, error)

    // DeleteByAccess 删除指定 Access Token 的条目。
    DeleteByAccess(ctx context.Context, token string) error

    // DeleteByRefresh 删除指定 Refresh Token 的条目。
    DeleteByRefresh(ctx context.Context, refreshToken string) error

    // DeleteByAccount 删除用户的所有 Token（登出所有设备 / SSO）。
    DeleteByAccount(ctx context.Context, uin int64) error

    // Blacklist 标记 Token 为已撤销。ttl 应与 Token 剩余有效期匹配。
    Blacklist(ctx context.Context, token string, ttl time.Duration) error

    // IsBlacklisted 检查 Token 是否在黑名单中。
    IsBlacklisted(ctx context.Context, token string) (bool, error)

    // SetJTI 记录已使用的 JWT ID，防止重放攻击。
    SetJTI(ctx context.Context, jti string, ttl time.Duration) error

    // IsJTIUsed 检查 JTI 是否已被使用。
    IsJTIUsed(ctx context.Context, jti string) (bool, error)
}
```

### Entry 结构体

```go
type Entry struct {
    UIN           int64  // 用户 / 账户标识符
    TokenHash     string // SHA-256(accessToken)
    RefreshHash   string // SHA-256(refreshToken)
    ExpiresAt     int64  // Access Token 过期时间（Unix 时间戳）
    RefreshExpiry int64  // Refresh Token 过期时间（Unix 时间戳）
    CreatedAt     int64  // 签发时间（Unix 时间戳）
}
```