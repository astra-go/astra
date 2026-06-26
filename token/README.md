# Token — Access/Refresh Token Lifecycle

[![Go Reference](https://pkg.go.dev/badge/github.com/astra-go/astra/token.svg)](https://pkg.go.dev/github.com/astra-go/astra/token)

Package `token` provides a complete token lifecycle manager for Astra applications. It handles the creation, storage, lookup, and revocation of opaque Access + Refresh token pairs.

## When to use this package

This package complements [`astra/middleware/security/jwt`](https://github.com/astra-go/astra/tree/main/middleware/security/jwt.go):

| Concern | Package |
|---------|---------|
| **Signature validation** (JWT verify) | `astra/middleware/security` — JWT middleware |
| **Token lifecycle** (issue, store, revoke) | `astra/token` — this package |
| **Replay protection** (JTI) | `astra/token` — this package |
| **Single sign-out** (SSO) | `astra/token` — this package |

## Features

- **Opaque tokens** — SHA-256 hashed before storage; raw tokens never persisted
- **Dual token model** — short-lived Access + long-lived Refresh token pairs
- **SSO enforcement** — issuing a new token invalidates the previous one for the same user
- **Blacklist** — revoke individual tokens (logout, password change)
- **JTI replay protection** — prevent one-time tokens from being reused
- **Dual backend** — `MemoryManager` for dev/test, `RedisManager` for production

## Quick start

```go
import (
    "context"
    "time"
    "github.com/astra-go/astra/token"
)

func main() {
    ctx := context.Background()

    // Memory backend (development / testing)
    mgr := token.NewMemoryManager()

    // Issue a token pair for user 42
    access, refresh, entry, err := mgr.Issue(ctx, 42, 15*time.Minute, 7*24*time.Hour)
    if err != nil {
        panic(err)
    }

    // Lookup by access token
    found, _ := mgr.FindByAccess(ctx, access)
    if found != nil {
        // token is valid
    }

    // Revoke on logout
    mgr.Blacklist(ctx, access, 15*time.Minute)

    // SSO: next issue invalidates previous tokens
    mgr.Issue(ctx, 42, 15*time.Minute, 7*24*time.Hour)

    // JTI replay protection
    mgr.SetJTI(ctx, "some-jti", 5*time.Minute)
    used, _ := mgr.IsJTIUsed(ctx, "some-jti")
}
```

## Redis backend (production)

```go
import (
    "github.com/astra-go/astra/token"
    "github.com/redis/go-redis/v9"
)

rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
mgr := token.NewRedisManager(rdb, "myapp:token:")

// Same API as MemoryManager
access, refresh, entry, _ := mgr.Issue(ctx, 42, 15*time.Minute, 7*24*time.Hour)
```

## Architecture

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
    │<── 200 OK ──────────────────│                               │
    │                             │                               │
    │  POST /auth/logout          │                               │
    │────────────────────────────>│                               │
    │                             │──── Blacklist(access, 15m) ─>│
    │<── 200 OK ─────────────────│                               │
```

## Best practices

1. **Short access TTL** — 15–30 minutes. Use Refresh tokens for longer sessions.
2. **Matched blacklist TTL** — always pass the remaining access token TTL to `Blacklist`.
3. **JTI for critical operations** — password reset, email change, payment confirmation.
4. **SSO for sensitive apps** — `Issue` automatically invalidates old tokens per user ID.


## API Reference

### Manager Interface

```go
type Manager interface {
    // Issue creates a new token pair. If the user already has active tokens, they will be replaced (SSO).
    Issue(ctx context.Context, uin int64, accessTTL, refreshTTL time.Duration) (accessToken string, refreshToken string, entry *Entry, err error)

    // FindByAccess looks up by Access Token. Tokens in the blacklist return nil.
    FindByAccess(ctx context.Context, token string) (*Entry, error)

    // FindByRefresh looks up by Refresh Token.
    FindByRefresh(ctx context.Context, refreshToken string) (*Entry, error)

    // DeleteByAccess deletes the entry for the specified Access Token.
    DeleteByAccess(ctx context.Context, token string) error

    // DeleteByRefresh deletes the entry for the specified Refresh Token.
    DeleteByRefresh(ctx context.Context, refreshToken string) error

    // DeleteByAccount deletes all tokens for a user (log out all devices / SSO).
    DeleteByAccount(ctx context.Context, uin int64) error

    // Blacklist marks a token as revoked. ttl should match the token's remaining validity period.
    Blacklist(ctx context.Context, token string, ttl time.Duration) error

    // IsBlacklisted checks whether a token is in the blacklist.
    IsBlacklisted(ctx context.Context, token string) (bool, error)

    // SetJTI records a used JWT ID to prevent replay attacks.
    SetJTI(ctx context.Context, jti string, ttl time.Duration) error

    // IsJTIUsed checks whether a JTI has already been used.
    IsJTIUsed(ctx context.Context, jti string) (bool, error)
}
```

### Entry structure

```go
type Entry struct {
    UIN           int64  // User / account identifier
    TokenHash     string // SHA-256(accessToken)
    RefreshHash   string // SHA-256(refreshToken)
    ExpiresAt     int64  // Access Token expiration time (Unix timestamp)
    RefreshExpiry int64  // Refresh Token expiration time (Unix timestamp)
    CreatedAt     int64  // Issuance time (Unix timestamp)
}
```