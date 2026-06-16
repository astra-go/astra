# Lock — 分布式锁

统一分布式锁抽象，提供 Redis 和 etcd 实现。

## 特性

- **统一接口**：`Locker` 接口隐藏后端差异；切换存储引擎无需修改业务代码
- **阻塞获取**：`Lock` 阻塞直到获取锁或 context 取消
- **非阻塞获取**：`TryLock` 立即返回，适用于异步竞争场景
- **自动续期（Redis）**：Redis 实现支持自动续期（看门狗），防止持有者崩溃导致锁永久持有
- **幂等释放**：`ReleaseFunc` 可安全多次调用

## 快速开始

### Redis 锁

```go
import (
    "github.com/astra-go/astra/lock"
    lockredis "github.com/astra-go/astra/lock/redis"
)

redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
locker := lockredis.New(redisClient)

// 阻塞获取（30 秒后自动释放）
release, err := locker.Lock(ctx, "order:pay:123", 30*time.Second)
if err != nil { return err }
defer release()

// 业务逻辑
doPayment()
```

### 非阻塞获取

```go
release, err := locker.TryLock(ctx, "order:pay:123", 30*time.Second)
if errors.Is(err, lock.ErrNotAcquired) {
    return errors.New("order is being processed, please try again later")
}
defer release()
```

### etcd 锁

```go
import locketcd "github.com/astra-go/astra/lock/etcd"

client, _ := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
locker := locketcd.New(client)

release, err := locker.Lock(ctx, "order:pay:123", 30*time.Second)
defer release()
```

## API

### Locker 接口

```go
type Locker interface {
    // Lock 阻塞直到获取锁，超时返回错误
    Lock(ctx context.Context, key string, ttl time.Duration) (ReleaseFunc, error)

    // TryLock 非阻塞，获取失败返回 ErrNotAcquired
    TryLock(ctx context.Context, key string, ttl time.Duration) (ReleaseFunc, error)
}
```

### ReleaseFunc

```go
type ReleaseFunc func()
// 可安全多次调用；多次解锁不会报错
```

### 错误

```go
var ErrNotAcquired = errors.New("lock: not acquired")
// TryLock 获取失败时返回
```

### Redis 锁配置

```go
lockredis.New(client, lockredis.Config{
    KeyPrefix:   "lock:",        // key 前缀
    AutoRenewal: true,           // 启用看门狗自动续期（推荐）
    RenewalInterval: 10*time.Second, // 续期间隔（默认 ttl/3）
})
```

## 完整示例

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

    // 非阻塞获取锁
    release, err := locker.TryLock(ctx, key, 30*time.Second)
    if errors.Is(err, lock.ErrNotAcquired) {
        fmt.Println("Order already locked by another request")
        return
    }
    if err != nil {
        panic(err)
    }
    defer release()

    // 执行业务
    fmt.Println("Processing order...")
    time.Sleep(time.Second)
    fmt.Println("Order processed")
}
```

## 模块依赖

| 子包 | 依赖 |
|-------------|-----------|
| `lock/redis` | `github.com/redis/go-redis/v9` |
| `lock/etcd` | `go.etcd.io/etcd/client/v3` |

## 注意事项

- `Lock` 的 ttl 必须大于正常业务执行时间，避免锁提前释放
- `Lock` 阻塞直到获取锁；设置合理的 ctx 超时时间
- `TryLock` 适用于异步任务，不适合同步阻塞场景
- Redis 锁建议启用 `AutoRenewal`，防止持有者进程 GC 暂停导致锁过期
- etcd 锁基于 Lease；客户端断开自动释放锁