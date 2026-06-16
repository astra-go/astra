# ORM — GORM 集成

深度 GORM 集成，支持事务传播、读写分离、表分片和 N+1 检测。

## 特性

- **事务传播**：事务通过 Context 传递 — Service 层无需知晓事务边界
- **读写分离**：自动将读请求路由到从库，写请求路由到主库
- **表分片**：基于分片 key 自动路由到不同的表/数据库
- **泛型 Repository**：`orm.ProvideRepository[T]` 注册泛型 Repository，支持 DI 注入
- **N+1 检测**：开发阶段自动警告 N+1 查询性能问题
- **多租户**：通过 Schema 或 Discriminator 实现多租户数据隔离

## 快速开始

### 中间件方式

```go
import "github.com/astra-go/astra/orm"

app.Use(orm.Middleware(db))

// 在处理器中获取带事务传播的 db
db := orm.DB(c)
users := db.Find(&users)
```

### 泛型 Repository 方式

```go
import "github.com/astra-go/astra/orm"

type User struct {
    ID   uint
    Name string
}

// 在 Module.Install 中注册
orm.ProvideRepository[User](container, db)

// 在 Service 层使用
repo := di.MustInvoke[contract.Repository[User]](container)
user, _ := repo.FindByID(ctx, 1)
```

## API

### Middleware

```go
func Middleware(db *gorm.DB) astra.MiddlewareFunc
```

向 App 注册 ORM 中间件，自动从 Context 提取事务性 db。

### orm.DB

```go
func DB(c contract.Context) *gorm.DB
```

从请求 Context 获取带事务传播的 GORM DB 实例。

### orm.ProvideRepository[T]

```go
func ProvideRepository[T any](c *di.Container, db *gorm.DB) error
```

向 DI 容器注册 `*Repository[T]`。T 必须实现 `repository.Model` 接口（有 `GetID()` 方法）。

### 手动事务控制

```go
// 自动事务
err := orm.Tx(c, func(tx *gorm.DB) error {
    return tx.Create(&order).Error
})
```

### 表分片

```go
orm.UseShard(func(id uint) string {
    return fmt.Sprintf("orders_%02d", id%16) // 分成 16 张表
})

// 自动路由
db := orm.DB(c)
db.Create(&order) // 自动写入 orders_03
```

## 配置

### 读写分离

```go
orm.UseReadDB(masterDB, []*gorm.DB{replica1, replica2})
```

### 多租户

```go
orm.UseTenant(orm.TenantConfig{
    Mode:        orm.TenantSchema, // 或 TenantDiscriminator
    GetTenantID: func(c contract.Context) string { return c.Get("tenant_id") },
})
```

## 完整示例

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

## 模块依赖

- `gorm.io/gorm` — ORM 核心库
- `gorm.io/driver/mysql` — MySQL 驱动（或其它数据库驱动）

## 注意事项

- `orm.DB(c)` 必须配合 `orm.Middleware` 使用，否则返回原始 db
- ClickHouse 驱动不支持事务中间件，会 panic（见 `orm/clickhouse_guard_test.go`）
- 分片路由函数须在 App 启动时注册，运行期间不应变更