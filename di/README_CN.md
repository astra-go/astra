# DI — 轻量级依赖注入容器

Astra 轻量级依赖注入（IoC）容器，支持构造函数注入、命名依赖和泛型。

## 特性

- **构造函数注入**：通过分析函数签名自动解析依赖
- **泛型支持**：`di.Invoke[T]` 和 `di.Provide[T]` 使用 Go 泛型，无需类型断言
- **命名依赖**：通过 `Name` tag 支持同一类型的多个实例
- **生命周期**：支持 Singleton 和 Transient 注册模式
- **GORM 集成**：`orm.ProvideRepository[T]` 自动注册泛型 Repository

## 快速开始

```go
import "github.com/astra-go/astra/di"

container := di.New()

// 注册单例（函数式）
container.Provide(func() *gorm.DB {
    db, _ := gorm.Open(mysql.Open(dsn), &gorm.Config{})
    return db
})

// 注册服务
container.Provide(NewUserService) // 构造函数依赖自动解析

// 解析
var svc *UserService
if err := container.Resolve(&svc); err != nil {
    panic(err)
}

// 泛型解析
repo := di.MustInvoke[*UserRepository](container)
```

## API

### di.New

```go
func New() *Container
```

创建空容器。

### container.Provide

```go
// 泛型版本（推荐）
func Provide[T any](c *Container, fn func(*Container) (T, error)) error

// 类型注册版本
func Provide(c *Container, typ reflect.Type, fn any) error
```

向容器注册类型或实例。`fn` 的参数从容器自动解析。

### container.Invoke

```go
// 泛型版本（推荐）
func Invoke[T any](c *Container) (T, error)

// 类型版本
func Invoke(c *Container, typ reflect.Type, fn any) error
```

从容器解析并调用函数。

### container.Resolve

```go
func (c *Container) Resolve(v any) error
```

解析单例实例到指针变量（`v` 必须为 `*T`）。

### 命名依赖

```go
// 注册命名实例
di.ProvideNamed[DB](container, "primary", primaryDB)
di.ProvideNamed[DB](container, "replica", replicaDB)

// 按名称解析
primary := di.MustInvokeNamed[*DB](container, "primary")
```

### 常见错误

| 错误 | 说明 |
|-------|-------------|
| `di.ErrDuplicate` | 同一类型已注册 |
| `di.ErrNotFound` | 容器中找不到类型 |
| `di.ErrCyclicDependency` | 检测到循环依赖 |

## 完整示例

```go
package main

import (
    "fmt"
    "github.com/astra-go/astra/di"
)

type Database struct{ DSN string }
type Cache struct{ Addr string }

type UserService struct {
    db    *Database
    cache *Cache
}

// NewUserService 的依赖从容器自动解析
func NewUserService(db *Database, cache *Cache) *UserService {
    return &UserService{db: db, cache: cache}
}

func main() {
    container := di.New()

    // 注册依赖
    container.Provide(func() *Database { return &Database{DSN: "localhost"} })
    container.Provide(func() *Cache { return &Cache{Addr: "localhost:6379"} })

    // 注册服务（依赖自动注入）
    if err := di.Provide(container, func(c *di.Container) (*UserService, error) {
        db := di.MustInvoke[*Database](c)
        cache := di.MustInvoke[*Cache](c)
        return NewUserService(db, cache), nil
    }); err != nil {
        panic(err)
    }

    // 解析并使用
    svc := di.MustInvoke[*UserService](container)
    fmt.Println(svc.db.DSN, svc.cache.Addr)
}
```

## 模块依赖

- `github.com/astra-go/astra/orm` — `orm.ProvideRepository[T]` 基于此容器注册

## 注意事项

- 避免循环依赖（A → B → A）；容器在启动时检测并返回 `ErrCyclicDependency`
- 推荐使用泛型版本 `di.Provide[T]` / `di.Invoke[T]` 避免运行时反射开销
- 构造函数参数顺序不影响解析；容器按类型匹配而非位置