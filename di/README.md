# DI — Lightweight Dependency Injection Container

Astra's lightweight dependency injection (IoC) container, supporting constructor injection, named dependencies, and generics.

## Features

- **Constructor Injection**: Automatically resolves dependencies by analyzing function signatures
- **Generic Support**: `di.Invoke[T]` and `di.Provide[T]` use Go generics, no type assertions needed
- **Named Dependencies**: Supports multiple instances of the same type via `Name` tag
- **Lifecycle**: Supports Singleton and Transient registration modes
- **GORM Integration**: `orm.ProvideRepository[T]` auto-registers generic Repository

## Quick Start

```go
import "github.com/astra-go/astra/di"

container := di.New()

// Register singleton (functional)
container.Provide(func() *gorm.DB {
    db, _ := gorm.Open(mysql.Open(dsn), &gorm.Config{})
    return db
})

// Register service
container.Provide(NewUserService) // Constructor dependencies auto-resolved

// Resolve
var svc *UserService
if err := container.Resolve(&svc); err != nil {
    panic(err)
}

// Generic resolution
repo := di.MustInvoke[*UserRepository](container)
```

## API

### di.New

```go
func New() *Container
```

Creates an empty container.

### container.Provide

```go
// Generic version (recommended)
func Provide[T any](c *Container, fn func(*Container) (T, error)) error

// Type registration version
func Provide(c *Container, typ reflect.Type, fn any) error
```

Registers a type or value instance to container. `fn`'s parameters auto-resolved from container.

### container.Invoke

```go
// Generic version (recommended)
func Invoke[T any](c *Container) (T, error)

// Type version
func Invoke(c *Container, typ reflect.Type, fn any) error
```

Resolves and calls function from container.

### container.Resolve

```go
func (c *Container) Resolve(v any) error
```

Resolves singleton instance to pointer variable (`v` must be `*T`).

### Named Dependencies

```go
// Register named instances
di.ProvideNamed[DB](container, "primary", primaryDB)
di.ProvideNamed[DB](container, "replica", replicaDB)

// Resolve by name
primary := di.MustInvokeNamed[*DB](container, "primary")
```

### Common Errors

| Error | Description |
|-------|-------------|
| `di.ErrDuplicate` | Same type already registered |
| `di.ErrNotFound` | Type not found in container |
| `di.ErrCyclicDependency` | Cyclic dependency detected |

## Complete Example

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

// NewUserService dependencies auto-resolved from container
func NewUserService(db *Database, cache *Cache) *UserService {
    return &UserService{db: db, cache: cache}
}

func main() {
    container := di.New()

    // Register dependencies
    container.Provide(func() *Database { return &Database{DSN: "localhost"} })
    container.Provide(func() *Cache { return &Cache{Addr: "localhost:6379"} })

    // Register service (dependencies auto-injected)
    if err := di.Provide(container, func(c *di.Container) (*UserService, error) {
        db := di.MustInvoke[*Database](c)
        cache := di.MustInvoke[*Cache](c)
        return NewUserService(db, cache), nil
    }); err != nil {
        panic(err)
    }

    // Resolve and use
    svc := di.MustInvoke[*UserService](container)
    fmt.Println(svc.db.DSN, svc.cache.Addr)
}
```

## Module Dependencies

- `github.com/astra-go/astra/orm` — `orm.ProvideRepository[T]` registers based on this container

## Notes

- Avoid circular dependencies (A → B → A); container detects and returns `ErrCyclicDependency` at startup
- Recommend using generic versions `di.Provide[T]` / `di.Invoke[T]` to avoid runtime reflection overhead
- Constructor parameter order doesn't affect resolution; container matches by type, not position
