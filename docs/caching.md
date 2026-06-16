# Caching

Astra's cache module provides a unified `Cache` interface supporting multiple backends — switch backends by only changing the initialization code.

## Interface Definition

```go
type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, keys ...string) error
    Exists(ctx context.Context, key string) (bool, error)
    Flush(ctx context.Context) error
    Close() error
}
```

## Memory Cache

```go
import (
    cachemem "github.com/astra-go/astra/cache/memory"
)

cache := cachemem.New(cachemem.Config{
    CleanupInterval: 5 * time.Minute, // Expire key cleanup
})

// Usage
cache.Set(ctx, "greeting", []byte("Hello"), 1*time.Hour)
val, err := cache.Get(ctx, "greeting")
```

## Redis Cache

```go
import (
    "github.com/redis/go-redis/v9"
    cacheredis "github.com/astra-go/astra/cache/redis"
)

// Option 1: pass options
cache, err := cacheredis.New(cacheredis.Config{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
})

// Option 2: pass existing client
rdb := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})
cache := cacheredis.NewWithClient(rdb, cacheredis.Config{
    KeyPrefix: "myapp:", // Unified key prefix
})
```

## Memcached Cache

```go
import (
    cachememcached "github.com/astra-go/astra/cache/memcached"
)

cache, err := cachememcached.New(cachememcached.Config{
    Addrs: []string{"localhost:11211"},
})
```

## JSON Helper Functions

```go
// Define struct
type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

// Cache JSON
user := User{ID: 1, Name: "Alice"}
cache.SetJSON(ctx, "user:1", user, 10*time.Minute)

// Read JSON
var cached User
err := cache.GetJSON(ctx, "user:1", &cached)
if err == cache.ErrCacheMiss {
    // Cache miss
}
```

## Cache Penetration Protection (GetOrSet)

```go
// Auto-fill cache
var user User
err := cache.GetOrSet(ctx, "user:1", &user, 10*time.Minute, func() (any, error) {
    // Called on cache miss (load from DB)
    var u User
    db.First(&u, 1)
    return u, nil
})
```

## Delete Patterns

```go
// Delete single
cache.Delete(ctx, "user:1")

// Batch delete
cache.Delete(ctx, "user:1", "user:2", "user:3")

// Flush all
cache.Flush(ctx)
```

## Integration with ORM

```go
import (
    "github.com/astra-go/astra/cache"
    "github.com/astra-go/astra/orm"
)

// Read from cache first on query
app.GET("/users/:id", func(c *astra.Ctx) error {
    id := c.Param("id")
    key := "user:" + id

    // Read from cache first
    var user User
    err := cache.GetJSON(ctx, key, &user)
    if err == cache.ErrCacheMiss {
        // Cache miss, query DB
        db := orm.DB(c)
        db.First(&user, id)

        // Write to cache
        cache.SetJSON(ctx, key, user, 10*time.Minute)
    } else if err != nil {
        return err
    }

    return c.JSON(200, user)
})

// Delete cache on update
app.PUT("/users/:id", func(c *astra.Ctx) error {
    id := c.Param("id")

    var update User
    c.BindJSON(&update)

    db := orm.DB(c)
    db.Model(&User{}).Where("id = ?", id).Updates(update)

    // Delete cache
    cache.Delete(ctx, "user:"+id)

    return c.JSON(200, astra.Map{"ok": true})
})
```

## Config Summary

| Parameter | Memory | Redis | Memcached |
|-----------|--------|-------|-----------|
| Persistence | ❌ | ✅ Configurable | ❌ |
| Distributed | ❌ | ✅ | ✅ |
| TTL Support | ✅ | ✅ | ✅ |
| Atomic Operations | ❌ | ✅ | ❌ |
| Use Case | Monolith/Testing | Production clusters | Simple caching |
