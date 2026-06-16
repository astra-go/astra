# ORM — GORM Integration

Deep GORM integration with Astra, featuring transaction propagation, read-write separation, table sharding, and N+1 detection.

## Features

- **Transaction Propagation**: Transactions passed through Context — Service layer doesn't need to know about transaction boundaries
- **Read-Write Separation**: Auto-route reads to replica, writes to primary
- **Table Sharding**: Auto-route to different tables/databases based on shard key
- **Generic Repository**: `orm.ProvideRepository[T]` registers a generic Repository, supports DI injection
- **N+1 Detection**: Automatically warns about N+1 query performance issues during development
- **Multi-Tenant**: Multi-tenant data isolation via Schema or Discriminator

## Quick Start

### Middleware Style

```go
import "github.com/astra-go/astra/orm"

app.Use(orm.Middleware(db))

// Get db with transaction propagation in handler
db := orm.DB(c)
users := db.Find(&users)
```

### Generic Repository Style

```go
import "github.com/astra-go/astra/orm"

type User struct {
    ID   uint
    Name string
}

// Register in Module.Install
orm.ProvideRepository[User](container, db)

// Use in Service layer
repo := di.MustInvoke[contract.Repository[User]](container)
user, _ := repo.FindByID(ctx, 1)
```

## API

### Middleware

```go
func Middleware(db *gorm.DB) astra.MiddlewareFunc
```

Registers ORM middleware to App, auto-extracts transactional db from Context.

### orm.DB

```go
func DB(c contract.Context) *gorm.DB
```

Gets GORM DB instance with transaction propagation from request Context.

### orm.ProvideRepository[T]

```go
func ProvideRepository[T any](c *di.Container, db *gorm.DB) error
```

Registers `*Repository[T]` to DI container. T must implement `repository.Model` interface (has `GetID()` method).

### Manual Transaction Control

```go
// Auto-transaction
err := orm.Tx(c, func(tx *gorm.DB) error {
    return tx.Create(&order).Error
})
```

### Table Sharding

```go
orm.UseShard(func(id uint) string {
    return fmt.Sprintf("orders_%02d", id%16) // Split into 16 tables
})

// Auto-route
db := orm.DB(c)
db.Create(&order) // Auto-writes to orders_03
```

## Config

### Read-Write Separation

```go
orm.UseReadDB(masterDB, []*gorm.DB{replica1, replica2})
```

### Multi-Tenant

```go
orm.UseTenant(orm.TenantConfig{
    Mode:        orm.TenantSchema, // or TenantDiscriminator
    GetTenantID: func(c contract.Context) string { return c.Get("tenant_id") },
})
```

## Complete Example

```go
package main

import (
    "github.com/astra-go/astra/orm"
    "github.com/astra-go/astra"
)

type User struct {
    ID   uint
    Name string
}

func main() {
    db, _ := gorm.Open(mysql.Open(dsn), &gorm.Config{})

    app := astra.New()
    app.Use(orm.Middleware(db))

    app.GET("/users", func(c *astra.Ctx) error {
        db := orm.DB(c)
        var users []User
        db.Find(&users)
        return c.JSON(200, users)
    })

    app.Run(":8080")
}
```

## Module Dependencies

- `gorm.io/gorm` — ORM core library
- `gorm.io/driver/mysql` — MySQL driver (or other database drivers)

## Notes

- `orm.DB(c)` must be used with `orm.Middleware`, otherwise returns raw db
- ClickHouse driver doesn't support transaction middleware and will panic (see `orm/clickhouse_guard_test.go`)
- Shard routing function must be registered at app startup and shouldn't change during runtime
