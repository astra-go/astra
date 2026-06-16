# Cache — Cache Abstraction

Unified cache interface supporting multiple backends (in-process memory, Redis, Memcached).

## Features

- **Unified Interface**: `Cache` interface defines Get/Set/Delete/Exists/Flush/Close — all backends are interchangeable
- **JSON Helpers**: `GetJSON`/`SetJSON` auto-serialize/deserialize Go structs
- **Cache Penetration Protection**: `GetOrSet` provides cache-aside pattern, handles concurrent source queries automatically
- **Non-invasive**: business code doesn't need to know which cache backend is used

## Quick Start

### In-Process Memory Cache

```go
import "github.com/astra-go/astra/cache/memory"

c := memory.New()
c.Set(ctx, "key", []byte("value"), time.Hour)
data, _ := c.Get(ctx, "key")
```

### Redis Cache

```go
import cacheredis "github.com/astra-go/astra/cache/redis"

c, _ := cacheredis.New(cacheredis.Config{Addr: "localhost:6379"})
c.Set(ctx, "key", []byte("value"), time.Hour)
```

## API

### Cache Interface

```go
type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)    // Returns ErrCacheMiss if not found
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, keys ...string) error        // Missing keys silently ignored
    Exists(ctx context.Context, key string) (bool, error)
    Flush(ctx context.Context) error                         // Clear all keys, use with caution
    Close() error
}
```

### JSON Helpers

```go
// Read from cache and auto-unmarshal into v
func GetJSON(ctx context.Context, c Cache, key string, v any) error

// Auto-marshal and store in cache
func SetJSON(ctx context.Context, c Cache, key string, v any, ttl time.Duration) error

// Cache penetration protection: on miss, call fetchFn to load from source, result auto-written to cache
func GetOrSet(ctx context.Context, c Cache, key string, v any, ttl time.Duration, fetchFn func() ([]byte, error)) error
```

### GetOrSet Example

```go
var user User
err := cache.GetOrSet(ctx, c, "user:42", &user, time.Minute, func() ([]byte, error) {
    return json.Marshal(db.FindUser(42))
})
```

## Config

### memory.New

```go
memory.New() // Use defaults
memory.New(memory.Config{
    MaxEntries: 10000,       // Max entries, LRU eviction when exceeded
    OnEvict:    func(key string, val []byte) {}, // Eviction callback
})
```

### cacheredis.New

```go
cacheredis.New(cacheredis.Config{
    Addr:      "localhost:6379",
    Password:  "",           // Redis password
    DB:        0,
    KeyPrefix: "cache:",    // Key prefix to avoid conflicts between apps
})
```

## Complete Example

```go
package main

import (
    "context"
    "encoding/json"
    "time"

    cacheredis "github.com/astra-go/astra/cache/redis"
)

type User struct{ ID int; Name string }

func main() {
    ctx := context.Background()
    c, _ := cacheredis.New(cacheredis.Config{Addr: "localhost:6379"})
    defer c.Close()

    // JSON helper
    user := User{ID: 1, Name: "Alice"}
    cache.SetJSON(ctx, c, "user:1", &user, time.Hour)

    var u User
    if err := cache.GetJSON(ctx, c, "user:1", &u); err == nil {
        println(u.Name) // Alice
    }
}
```

## Module Dependencies

None (interface definition package). Backend dependencies:
- `cache/memory`: no external dependencies
- `cache/redis`: `github.com/redis/go-redis/v9`
- `cache/memcached`: `github.com/bradfitz/gomemcache`

## Notes

- `memory` backend is not suitable for sharing across multiple instances of the same process (use Redis instead)
- `ErrCacheMiss` can be used to determine cache miss for business logic (e.g., fall back to DB query)
- In production, recommend adding app-level prefix to all keys to avoid conflicts with other systems
