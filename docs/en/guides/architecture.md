# Architecture: Module, Plugin & DI Container

Astra provides three mechanisms for organising application code and extending the framework. Each has a distinct responsibility and layer, but they compose freely.

---

## How the Three Relate

```
┌────────────────────────────────────────────────────────────────┐
│                          *astra.App                            │
│                                                                │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Module / Plugin  —  the "installation unit"            │   │
│  │                                                         │   │
│  │  Each Module/Plugin registers into App via              │   │
│  │  Install/Init:                                          │   │
│  │    · Routes        app.GET / app.Group(...)             │   │
│  │    · Middleware    app.Use(...)                         │   │
│  │    · Lifecycle     app.OnStart / app.OnStop             │   │
│  │    · Sub-modules   app.Register(innerModule)            │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              ↑                                 │
│                  consumed by Module/Plugin                      │
│                              ↑                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  di.Container  —  dependency injection container        │   │
│  │                                                         │   │
│  │  Manages construction, sharing, and lifecycle of        │   │
│  │  singleton services:                                    │   │
│  │    di.Provide[*DB](c, factory)                          │   │
│  │    db := di.MustInvoke[*DB](c)                          │   │
│  │    c.BindApp(app)  ← wires container Start/Stop         │   │
│  └─────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────┘
```

**Module / Plugin** — "install functionality into App"  
**di.Container** — "inject dependencies into Module/Plugin"

The two layers are orthogonal and independent. Use them separately or together.

---

## Comparison Table

| Dimension | `Module` | `Plugin` | `di.Container` |
|-----------|----------|----------|----------------|
| **Key method** | `Install(app *App) error` | `Init(app *App) error` | `di.Provide` / `di.Invoke` |
| **Use case** | Your own business units (API layer, domain services, …) | Third-party library adapters (Prometheus, Swagger, OAuth2, …) | Shared singleton dependencies (DB, Redis, config, …) |
| **Registration** | `app.Register(m)` | `app.RegisterPlugin(p)` or `app.Register(astra.PluginAsModule(p))` | `di.Provide[T](container, factory)` |
| **Duplicate detection** | ✅ (name is unique) | ✅ (shares namespace via PluginAsModule) | ✅ (type + name unique) |
| **Lifecycle** | `app.OnStart/OnStop` | `app.OnStart/OnStop` | `c.OnStart/OnStop` + `c.BindApp(app)` |
| **Dependency sharing** | Manual constructor args | Manual constructor args | Automatic: `di.MustInvoke[T](c)` |
| **Best fit** | Medium-large apps, feature-oriented modules | Reusable library integrations | Services with complex dependency graphs |
| **Import cost** | Zero — built into core | Zero — built into core | `github.com/astra-go/astra/di` |

---

## Decision Tree

```
I need to organise some code — which mechanism should I use?
│
├─ It's a reusable third-party library adapter
│   (another project will import it)?
│   └─ Yes → Plugin  (implement Init, register via app.RegisterPlugin)
│
├─ It's application-specific business logic
│   (user service, device API, auth middleware, …)?
│   └─ Yes → Module  (implement Install, register via app.Register)
│
└─ I need to share a singleton across multiple Modules/Plugins
    (database connection, Redis client, config object, …)?
    └─ Yes → di.Container  (di.Provide to register, di.MustInvoke to resolve,
                             c.BindApp to wire lifecycle)
```

> **Quick rule**: Plugin = "I am a library", Module = "I am a feature", DI = "I am a dependency".

---

## Typical Combinations

### Scenario 1: Module with direct constructor args (small service, no DI)

```go
// main.go
db, _ := sql.Open("postgres", os.Getenv("DATABASE_URL"))

app := astra.New()
app.Register(
    user.NewModule(db),   // Module receives db as a constructor argument
    order.NewModule(db),
)
app.Run(":8080")
```

Use this when the dependency graph is shallow and the number of modules is small.

---

### Scenario 2: di.Container manages dependencies, Modules consume them (medium-large service)

```go
// main.go
c := di.New()

di.Provide[*sql.DB](c, func(_ *di.Container) (*sql.DB, error) {
    return sql.Open("postgres", os.Getenv("DATABASE_URL"))
})
di.Provide[*redis.Client](c, func(_ *di.Container) (*redis.Client, error) {
    return redis.NewClient(&redis.Options{Addr: "localhost:6379"}), nil
})

app := astra.New()
c.BindApp(app) // wires DB/Redis Close into App's graceful shutdown

app.Register(
    user.NewModuleFromContainer(c),
    order.NewModuleFromContainer(c),
)
app.Run(":8080")
```

```go
// user/module.go
type Module struct{ c *di.Container }

func NewModuleFromContainer(c *di.Container) astra.Module {
    return &Module{c: c}
}

func (m *Module) Name() string { return "user" }

func (m *Module) Install(app *astra.App) error {
    db := di.MustInvoke[*sql.DB](m.c)
    svc := NewUserService(db)
    g := app.Group("/users")
    g.GET("",     svc.List)
    g.POST("",    svc.Create)
    g.GET("/:id", svc.Get)
    return nil
}
```

---

### Scenario 3: Plugin integrates a library, Module owns business logic

```go
app := astra.New()

// Plugin: third-party library integration (Swagger UI)
app.RegisterPlugin(swagger.New(swagger.Config{
    BasePath: "/docs",
    Title:    "My API",
}))

// Module: application business logic
app.Register(
    user.NewModule(db),
    order.NewModule(db),
)

app.Run(":8080")
```

Or use a single `app.Register` call to mix Plugins and Modules:

```go
app.Register(
    astra.PluginAsModule(swagger.New(swagger.Config{BasePath: "/docs"})),
    user.NewModule(db),
    order.NewModule(db),
)
```

---

### Scenario 4: All three together (production-grade application)

```go
func main() {
    // 1. Build the DI container
    c := di.New()
    di.Provide[*sql.DB](c, newDB)
    di.Provide[*redis.Client](c, newRedis)
    di.Provide[*UserService](c, func(c *di.Container) (*UserService, error) {
        db := di.MustInvoke[*sql.DB](c)
        return NewUserService(db), nil
    })

    // 2. Create App, bind container lifecycle
    app := astra.New()
    c.BindApp(app)

    // 3. Register Plugins (third-party) and Modules (business)
    app.Register(
        astra.PluginAsModule(swagger.New(swagger.Config{BasePath: "/docs"})),
        astra.PluginAsModule(observability.NewModule(observability.Config{
            ServiceName:  "my-svc",
            OTELEndpoint: "http://otel:4318",
        })),
        user.NewModuleFromContainer(c),
        order.NewModuleFromContainer(c),
    )

    app.Run(":8080")
}
```

---

## Common Mistakes

| Mistake | Correct approach |
|---------|-----------------|
| Calling `sql.Open` inside `Plugin.Init` | Put the DB in `di.Container`; Plugin only registers routes/middleware |
| Putting all code in one giant Module | Split by domain: `user.Module`, `order.Module`, `payment.Module` |
| Using Module to implement a reusable third-party integration | Use Plugin instead — other projects can drop it in with `app.RegisterPlugin` |
| Providing dependencies in `di.Container` but forgetting `c.BindApp(app)` | Call `c.BindApp(app)` once in `main` so DB/Redis close on graceful shutdown |
| Registering the same name in both a Module and a Plugin | They share the same namespace; duplicate registration returns an error — check with `app.HasModule(name)` |
