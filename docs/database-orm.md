# Database ORM

Astra's ORM module provides deep GORM integration with transaction propagation, read-write separation, table sharding, and connection pool management.

## Quick Start

```go
import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/orm"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "strconv"
)

func main() {
    // Connect to database
    dsn := "host=localhost user=postgres password=secret dbname=myapp port=5432 sslmode=disable"
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        panic(err)
    }

    // Configure connection pool
    orm.SetPool(db, orm.PoolConfig{
        MaxOpenConns:    100,
        MaxIdleConns:    10,
        ConnMaxLifetime: 30 * time.Minute,
        ConnMaxIdleTime: 5 * time.Minute,
    })

    // Create App and register ORM middleware
    app := astra.New()
    app.Use(orm.Middleware(db))

    // Use in handler
    app.GET("/users", func(c *astra.Ctx) error {
        db := orm.DB(c)
        var users []User
        db.Find(&users)
        return c.JSON(200, users)
    })

    app.Run(":8080")
}

type User struct {
    ID        uint      `gorm:"primaryKey"`
    Name      string    `gorm:"size:100"`
    Email     string    `gorm:"uniqueIndex;size:255"`
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

## Transaction Management

### Auto Transaction (Middleware Style)

```go
// Each request auto-starts a transaction
app.POST("/orders", orm.TxMiddleware(db), createOrder)

func createOrder(c *astra.Ctx) error {
    db := orm.DB(c) // Auto-gets transaction db

    var req OrderReq
    c.BindJSON(&req)

    // All operations in same transaction
    order := Order{UserID: req.UserID, Amount: req.Amount}
    db.Create(&order)

    var user User
    db.First(&user, req.UserID)
    user.Balance -= req.Amount
    db.Save(&user)

    return c.JSON(201, order)
    // Handler returns nil → auto-commit
    // Handler returns error → auto-rollback
}
```

### Manual Transaction

```go
// Manual control
err := db.Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&order).Error; err != nil {
        return err // Rollback
    }
    if err := tx.Model(&user).Update("balance", gorm.Expr("balance - ?", amount)).Error; err != nil {
        return err // Rollback
    }
    return nil // Commit
})
```

## Service Layer Transaction Propagation

Propagate transactions through Context, decoupling HTTP layer and business layer:

```go
// Define service
type UserService struct {
    db *gorm.DB
}

func (s *UserService) Create(ctx context.Context, name, email string) (User, error) {
    // FromCtx auto-gets transaction if present
    db := orm.FromCtx(ctx, s.db)

    user := User{Name: name, Email: email}
    err := db.Create(&user).Error
    return user, err
}

// Use in handler
func createUserHandler(c *astra.Ctx) error {
    var req CreateUserReq
    c.BindJSON(&req)

    svc := &UserService{db: orm.FromCtx(c.Request().Context(), globalDB)}
    user, err := svc.Create(c.Request().Context(), req.Name, req.Email)
    if err != nil {
        return err
    }
    return c.JSON(201, user)
}
```

## Multiple Databases and Read-Write Separation

```go
// Create database manager
mgr := orm.NewManager()

// Register multiple databases
primary, _ := gorm.Open(postgres.Open(primaryDSN), &gorm.Config{})
replica, _ := gorm.Open(postgres.Open(replicaDSN), &gorm.Config{})

mgr.Register("primary", primary)
mgr.Register("replica", replica)

// Use middleware
app.Use(mgr.Middleware())

// Read-write separation component
rw := orm.NewReadWriter(primary, replica,
    orm.WithReadWeight(3),    // Read load weight
    orm.WithWriteWeight(1),
)
app.Use(rw.Middleware())

// Usage
app.GET("/users", func(c *astra.Ctx) error {
    // Read operations auto-routed to replica
    db := orm.DB(c)
    var users []User
    db.Find(&users)
    return c.JSON(200, users)
})

app.POST("/users", func(c *astra.Ctx) error {
    // Write operations routed to primary
    db := orm.DB(c)
    db.Create(&user)
    return c.JSON(201, user)
})
```

## Table Sharding

```go
// Shard by user ID
shard, err := orm.NewShardRouter(orm.ShardConfig{
    ShardKey:  "user_id",          // Shard key
    ShardNum:  16,                 // Number of shards
    TableName: func(base string, shardID int) string {
        return fmt.Sprintf("%s_%02d", base, shardID)
    },
})

// Auto-route to correct shard on query
app.GET("/users/:id/orders", func(c *astra.Ctx) error {
    userID, _ := strconv.Atoi(c.Param("id"))
    db := orm.DB(c).Set("shard:key", userID)

    var orders []Order
    db.Where("user_id = ?", userID).Find(&orders)
    return c.JSON(200, orders)
})
```

## N+1 Query Detection

```go
// Enable N+1 detection in development
detector := orm.NewNPlusOneDetector(
    orm.WithNPlusOneThreshold(5),
    orm.WithNPlusOneLogLevel(slog.LevelWarn),
)
db.Use(detector)
```

## GORM Plugin Integration

```go
import (
    "github.com/astra-go/astra/orm"
    "gorm.io/plugin/opentelemetry/tracing"
)

// Enable OpenTelemetry tracing
db.Use(tracing.NewPlugin())

// Register to Astra
app.Use(orm.Middleware(db))
```

## More Supported Databases

| Database | Driver |
|----------|--------|
| PostgreSQL | `gorm.io/driver/postgres` |
| MySQL | `gorm.io/driver/mysql` |
| SQLite | `gorm.io/driver/sqlite` |
| SQL Server | `gorm.io/driver/sqlserver` |
| ClickHouse | `github.com/astra-go/astra/orm/clickhouse` |
| TiDB | MySQL driver compatible |
