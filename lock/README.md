# Lock — Distributed Lock

Unified distributed lock abstraction with Redis and etcd implementations.

## Features

- **Unified Interface**: `Locker` interface hides backend differences; switching storage engine requires no business code changes
- **Blocking Acquisition**: `Lock` blocks until lock is acquired or context is cancelled
- **Non-Blocking Acquisition**: `TryLock` returns immediately, suitable for async竞争 scenarios
- **Auto Renewal (Redis)**: Redis implementation supports auto renewal (watchdog), prevents lock from being held forever if holder crashes
- **Idempotent Release**: `ReleaseFunc` can be safely called multiple times

## Quick Start

### Redis Lock

```go
import (
    "github.com/astra-go/astra/lock"
    lockredis "github.com/astra-go/astra/lock/redis"
)

redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
locker := lockredis.New(redisClient)

// Blocking acquire (auto-releases after 30 seconds)
release, err := locker.Lock(ctx, "order:pay:123", 30*time.Second)
if err != nil { return err }
defer release()

// Business logic
doPayment()
```

### Non-Blocking Acquisition

```go
release, err := locker.TryLock(ctx, "order:pay:123", 30*time.Second)
if errors.Is(err, lock.ErrNotAcquired) {
    return errors.New("order is being processed, please try again later")
}
defer release()
```

### etcd Lock

```go
import locketcd "github.com/astra-go/astra/lock/etcd"

client, _ := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
locker := locketcd.New(client)

release, err := locker.Lock(ctx, "order:pay:123", 30*time.Second)
defer release()
```

## API

### Locker Interface

```go
type Locker interface {
    // Lock blocks until lock is acquired, returns error on timeout
    Lock(ctx context.Context, key string, ttl time.Duration) (ReleaseFunc, error)

    // TryLock non-blocking, returns ErrNotAcquired if lock is held
    TryLock(ctx context.Context, key string, ttl time.Duration) (ReleaseFunc, error)
}
```

### ReleaseFunc

```go
type ReleaseFunc func()
// Safe to call multiple times; multiple unlocks won't error
```

### Errors

```go
var ErrNotAcquired = errors.New("lock: not acquired")
// Returned by TryLock when lock acquisition fails
```

### Redis Lock Config

```go
lockredis.New(client, lockredis.Config{
    KeyPrefix:   "lock:",        // Key prefix
    AutoRenewal: true,           // Enable watchdog auto-renewal (recommended)
    RenewalInterval: 10*time.Second, // Renewal interval (default ttl/3)
})
```

## Complete Example

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/astra-go/astra/lock"
    lockredis "github.com/astra-go/astra/lock/redis"
    "github.com/redis/go-redis/v9"
)

func main() {
    ctx := context.Background()
    client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    locker := lockredis.New(client)

    orderID := "order-123"
    key := fmt.Sprintf("order:lock:%s", orderID)

    // Non-blocking lock acquisition
    release, err := locker.TryLock(ctx, key, 30*time.Second)
    if errors.Is(err, lock.ErrNotAcquired) {
        fmt.Println("Order already locked by another request")
        return
    }
    if err != nil {
        panic(err)
    }
    defer release()

    // Execute business
    fmt.Println("Processing order...")
    time.Sleep(time.Second)
    fmt.Println("Order processed")
}
```

## Module Dependencies

| Sub-package | Dependency |
|-------------|-----------|
| `lock/redis` | `github.com/redis/go-redis/v9` |
| `lock/etcd` | `go.etcd.io/etcd/client/v3` |

## Notes

- `ttl` for `Lock` must be greater than normal business execution time to avoid premature lock release
- `Lock` blocks until lock is acquired; set reasonable ctx timeout
- `TryLock` is suitable for async tasks, not for sync blocking scenarios
- Redis lock recommends enabling `AutoRenewal` to prevent lock expiration due to holder process GC pause
- etcd lock is based on Lease; client disconnection auto-releases the lock
