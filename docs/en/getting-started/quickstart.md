# Getting Started

Astra's learning path has three steps — go as deep as you need, no need to master everything at once.

---

## Step 1 — Hello World (5 minutes)

Exactly the same style as Gin / Echo — no extra concepts required.

```bash
mkdir myapp && cd myapp
go mod init myapp
go get github.com/astra-go/astra@latest
```

```go
// main.go
package main

import (
    "net/http"
    "github.com/astra-go/astra"
)

func main() {
    app := astra.New()

    app.GET("/", func(c astra.Context) error {
        return c.JSON(http.StatusOK, astra.Map{"message": "hello astra"})
    })

    app.GET("/hello/:name", func(c astra.Context) error {
        return c.JSON(http.StatusOK, astra.Map{"hello": c.Param("name")})
    })

    app.Run(":8080")
}
```

```bash
go run main.go
curl http://localhost:8080/hello/world
# {"hello":"world"}
```

Full example: [`examples/hello`](../../examples/hello/main.go)

---

## Step 2 — A Real Service (15 minutes)

Add middleware, route groups, request binding & validation, and graceful shutdown.

```go
package main

import (
    "context"
    "fmt"
    "net/http"

    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
)

type CreateItemReq struct {
    Name  string `json:"name"  validate:"required,max=64"`
    Price int    `json:"price" validate:"gte=0"`
}

func main() {
    app := astra.New()

    // Global middleware
    app.Use(
        middleware.Recovery(),
        middleware.Logger(),
        middleware.RequestID(),
    )

    // Route group
    v1 := app.Group("/api/v1")
    v1.GET("/items", listItems)
    v1.POST("/items", createItem)

    // Authenticated sub-group
    admin := v1.Group("/admin")
    admin.Use(middleware.JWT("your-secret"))
    admin.GET("/stats", statsHandler)

    // Lifecycle hooks (optional)
    app.OnStart(func(_ context.Context) error {
        fmt.Println("server ready")
        return nil
    })

    app.Run(":8080")
}
```

Binding & validation: the framework automatically returns `422 + field errors` when `ShouldBind` fails — no manual handling needed.

```go
func createItem(c astra.Context) error {
    var req CreateItemReq
    if err := c.ShouldBind(&req); err != nil {
        return err // → 422 {"errors": [{"field":"name","msg":"required"}]}
    }
    // ...
    return c.JSON(http.StatusCreated, result)
}
```

Full example: [`examples/quickstart`](../../examples/quickstart/main.go)

---

## Step 3 — Large Applications (as needed)

When code grows beyond a single file, use **Modules** to split domain boundaries.

```go
// Each Module independently registers routes, middleware, and lifecycle hooks
type UserModule struct{ db *sql.DB }

func (m *UserModule) Name() string { return "user" }
func (m *UserModule) Install(app *astra.App) error {
    g := app.Group("/api/v1/users")
    g.GET("",    m.list)
    g.POST("",   m.create)
    g.GET("/:id", m.get)

    app.OnStop(func(ctx context.Context) error {
        return m.db.Close()
    })
    return nil
}

// main.go — assembly only
func main() {
    app := astra.New()
    app.Use(middleware.Recovery(), middleware.Logger())

    app.Register(
        &UserModule{db: mustDB()},
        &OrderModule{db: mustDB()},
    )

    app.Run(":8080")
}
```

**DI Container** (optional) — introduce when inter-module dependencies become complex:

```go
c := di.New()

di.Provide[*sql.DB](c, func(c *di.Container) (*sql.DB, error) {
    return sql.Open("postgres", os.Getenv("DATABASE_URL"))
})

di.Provide[*UserService](c, func(c *di.Container) (*UserService, error) {
    db, _ := di.Invoke[*sql.DB](c)
    return NewUserService(db), nil
})

c.BindApp(app) // Wire DI lifecycle into app's graceful shutdown
```

**Plugin** (optional) — for encapsulating third-party integrations (Prometheus, Jaeger, etc.). The difference from Module is semantic: Plugin is a cross-cutting concern, Module is a business domain.

Full example: [`examples/basic`](../../examples/basic/main.go)

---

## Concept Cheat Sheet

| Concept | Required | Equivalent to |
|---------|----------|---------------|
| `astra.New()` + `app.GET/POST` + `app.Run` | ✅ Step 1 | Gin `r := gin.New()` |
| `app.Group` + `app.Use` | ✅ Step 1 | Gin `r.Group` |
| `OnStart` / `OnStop` | ⬜ When you have a database | Custom start/stop logic |
| `Module` | ⬜ When code spans multiple files | — |
| `Plugin` | ⬜ When integrating third parties | — |
| `di.Container` | ⬜ When dependency graph is complex | Wire / Uber fx |

---

## Next Steps

- [Recommended Project Structure](project-structure.md)
- [Middleware List](../guides/middleware.md)
- [Error Handling](../guides/error-handling.md)
- [Module System](../guides/modules.md)
