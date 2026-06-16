# Session — HTTP Session Management

Plugin-based HTTP session management. Current implementation uses Redis storage.

## Features

- **Data Security**: Cookie only stores HMAC-SHA256 signed Session ID, no user data exposed
- **Tamper-Proof**: Signature verification ensures clients cannot forge Session IDs
- **Session Fixation Protection**: Call `RegenerateID()` after login to get new Session ID
- **Auto-Save**: Dirty sessions automatically written back to storage after request processing
- **Multiple Storage Backends**: Currently provides Redis storage; other storage can implement the interface

## Quick Start

```go
import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/session"
    sessredis "github.com/astra-go/astra/session/redis"
)

store := sessredis.New(redisClient, sessredis.Config{
    KeyPrefix: "sess:",
    SecretKey: "your-secret-key-min-32bytes!!",
})

app.Use(session.Middleware(store))

// Login
app.POST("/login", func(c *astra.Ctx) error {
    sess := session.Get(c)
    sess.Set("user_id", 42)
    return c.JSON(200, astra.Map{"ok": true})
})

// Get login status
app.GET("/profile", func(c *astra.Ctx) error {
    sess := session.Get(c)
    uid, ok := sess.GetInt("user_id")
    if !ok {
        return astra.NewHTTPError(401, "not authenticated")
    }
    return c.JSON(200, astra.Map{"user_id": uid})
})
```

## API

### session.Get

```go
func Get(c contract.Context) Session
```

Gets current session from request Context.

### Session Interface

```go
type Session interface {
    ID() string                        // Get Session ID
    Get(key string) (any, bool)       // Get value
    GetInt(key string) (int, bool)   // Get int
    Set(key string, val any)          // Set value
    Delete(key string)                // Delete key
    Clear()                           // Clear all keys
    RegenerateID() error               // Regenerate Session ID (prevent fixation)
    Destroy() error                    // Destroy session (logout)
    Save() error                       // Manual save
}
```

### Config Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `KeyPrefix` | `string` | `"sess:"` | Redis key prefix |
| `SecretKey` | `string` | — | HMAC signing key (≥32 bytes) |
| `MaxAge` | `time.Duration` | `24h` | Cookie validity |
| `CookieName` | `string` | `"session_id"` | Cookie name |
| `Domain` | `string` | `""` | Cookie domain |
| `Secure` | `bool` | `true` | HTTPS only |
| `HTTPOnly` | `bool` | `true` | Block JavaScript access |
| `SameSite` | `http.SameSite` | `LaxMode` | CSRF protection level |

## Complete Example

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/session"
    sessredis "github.com/astra-go/astra/session/redis"
)

func main() {
    store := sessredis.New(redisClient, sessredis.Config{
        KeyPrefix: "sess:",
        SecretKey: "change-this-secret-key-32bytes!",
    })

    app := astra.New()
    app.Use(session.Middleware(store))

    app.POST("/login", func(c *astra.Ctx) error {
        sess := session.Get(c)
        sess.Set("user_id", 42)
        return c.JSON(200, astra.Map{"ok": true})
    })

    app.POST("/logout", func(c *astra.Ctx) error {
        sess := session.Get(c)
        sess.Destroy()
        return c.JSON(200, astra.Map{"ok": true})
    })

    app.GET("/profile", func(c *astra.Ctx) error {
        sess := session.Get(c)
        uid, ok := sess.GetInt("user_id")
        if !ok {
            return astra.NewHTTPError(401, "not authenticated")
        }
        return c.JSON(200, astra.Map{"user_id": uid})
    })

    app.Run(":8080")
}
```

## Module Dependencies

- `github.com/redis/go-redis/v9` — Redis client

## Notes

- `SecretKey` must be a random string ≥32 bytes; never hardcode or commit to repo
- Must call `RegenerateID()` after successful login to prevent session fixation attacks
- No need to manually clear Cookie after `Destroy()` — middleware handles it automatically on response
