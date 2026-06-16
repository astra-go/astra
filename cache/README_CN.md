# Cache — 缓存抽象

统一缓存接口，支持多后端（进程内内存、Redis、Memcached）。

## 特性

- **统一接口**：`Cache` 接口定义 Get/Set/Delete/Exists/Flush/Close；所有后端可互换
- **JSON 辅助**：`GetJSON`/`SetJSON` 自动序列化/反序列化 Go 结构体
- **缓存穿透防护**：`GetOrSet` 提供 cache-aside 模式，自动处理并发源查询
- **无侵入**：业务代码无需知晓使用了哪种缓存后端

## 快速开始

### 进程内内存缓存

```go
import "github.com/astra-go/astra/cache/memory"

c := memory.New()
c.Set(ctx, "key", []byte("value"), time.Hour)
data, _ := c.Get(ctx, "key")
```

### Redis 缓存

```go
import cacheredis "github.com/astra-go/astra/cache/redis"

c, _ := cacheredis.New(cacheredis.Config{Addr: "localhost:6379"})
c.Set(ctx, "key", []byte("value"), time.Hour)
```

## API

### Cache 接口

```go
type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)    // 未命中返回 ErrCacheMiss
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, keys ...string) error        // 不存在的 key 静默忽略
    Exists(ctx context.Context, key string) (bool, error)
    Flush(ctx context.Context) error                         // 清空所有 key，慎用
    Close() error
}
```

### JSON 辅助

```go
// 从缓存读取并自动反序列化到 v
func GetJSON(ctx context.Context, c Cache, key string, v any) error

// 自动序列化并存入缓存
func SetJSON(ctx context.Context, c Cache, key string, v any, ttl time.Duration) error

// 缓存穿透防护：未命中时调用 fetchFn 从源加载，结果自动写入缓存
func GetOrSet(ctx context.Context, c Cache, key string, v any, ttl time.Duration, fetchFn func() ([]byte, error)) error
```

### GetOrSet 示例

```go
var user User
err := cache.GetOrSet(ctx, c, "user:42", &user, time.Minute, func() ([]byte, error) {
    return json.Marshal(db.FindUser(42))
})
```

## 配置

### memory.New

```go
memory.New() // 使用默认值
memory.New(memory.Config{
    MaxEntries: 10000,       // 最大条目数，超出后 LRU 淘汰
    OnEvict:    func(key string, val []byte) {}, // 淘汰回调
})
```

### cacheredis.New

```go
cacheredis.New(cacheredis.Config{
    Addr:      "localhost:6379",
    Password:  "",           // Redis 密码
    DB:        0,
    KeyPrefix: "cache:",    // 避免多应用 key 冲突的 key 前缀
})
```

## 完整示例

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

    // JSON 辅助
    user := User{ID: 1, Name: "Alice"}
    cache.SetJSON(ctx, c, "user:1", &user, time.Hour)

    var u User
    if err := cache.GetJSON(ctx, c, "user:1", &u); err == nil {
        println(u.Name) // Alice
    }
}
```

## 模块依赖

无外部依赖（接口定义包）。后端依赖：
- `cache/memory`：无外部依赖
- `cache/redis`：`github.com/redis/go-redis/v9`
- `cache/memcached`：`github.com/bradfitz/gomemcache`

## 注意事项

- `memory` 后端不适合在同一进程多实例间共享（请使用 Redis）
- `ErrCacheMiss` 可用于判断缓存未命中以执行业务逻辑（如回退到 DB 查询）
- 生产环境建议为所有 key 添加应用级前缀，避免与其他系统冲突