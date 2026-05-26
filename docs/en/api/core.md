# Core API — App / Context / Router

> **Stability: Stable** (v1.0+)

---

## App

`App` is the framework's central entry point, holding the route tree, middleware chain, and lifecycle hooks.

### Creating an App

```go
import "github.com/astra-go/astra"

app := astra.New()                          // default options
app := astra.New(
    astra.WithLogger(myLogger),
    astra.WithTrustedProxies("10.0.0.0/8", "172.16.0.0/12"),
    astra.WithMaxMultipartMemory(32 << 20), // 32 MB
)
```

#### `Option` Functions

| Function | Description |
|----------|-------------|
| `WithLogger(l *slog.Logger)` | Replace the default `slog.Default()` |
| `WithTrustedProxies(cidr ...string)` | Set trusted proxy CIDRs (used by `ClientIP()` XFF traversal) |
| `WithMaxMultipartMemory(n int64)` | Multipart memory limit, default 32 MB |
| `WithErrorHandler(fn ErrorHandlerFunc)` | Replace the global error handler |

---

### Route Registration

```go
app.GET("/users",          listUsers)
app.POST("/users",         createUser)
app.PUT("/users/:id",      updateUser)
app.DELETE("/users/:id",   deleteUser)
app.PATCH("/users/:id",    patchUser)
app.HEAD("/users/:id",     headUser)
app.OPTIONS("/users",      optionsUsers)
app.Any("/ws",             handleWebSocket)  // register all methods
```

#### Path Patterns

| Pattern | Example | Matches |
|---------|---------|---------|
| Static | `/about` | `/about` |
| Parameter | `/users/:id` | `/users/42` |
| Wildcard | `/files/*path` | `/files/a/b/c` |
| Regex | `/order/:id([0-9]+)` | `/order/123` (digits only) |

---

### Middleware

```go
// Global
app.Use(middleware.Logger(), middleware.Recovery())

// Group
v1 := app.Group("/v1", middleware.JWT(jwtCfg))
v1.GET("/profile", getProfile)

// Single route
app.GET("/admin", adminOnly, adminHandler)
```

---

### Static Files

```go
app.Static("/assets", "./public")
// GET /assets/style.css → ./public/style.css
```

---

### Modules & Plugins

Astra provides two extension interfaces with distinct responsibilities, both installed through the same unified registration path:

| Interface | Method | Use case |
|-----------|--------|----------|
| `Module` | `Install(app *App) error` | Your own business units (API layer, domain services, …) |
| `Plugin` | `Init(app *App) error` | Third-party library adapters (Prometheus, Swagger, …) |

```go
// Module: application business logic
type APIModule struct{ db *sql.DB }
func (m *APIModule) Name() string { return "api" }
func (m *APIModule) Install(app *astra.App) error {
    app.GET("/users", m.listUsers)
    return nil
}
app.Register(&APIModule{db: db})

// Plugin: third-party library integration
app.RegisterPlugin(swagger.New(swagger.Config{BasePath: "/docs"}))

// Mixed registration (unified Register call; both share the same namespace)
app.Register(
    astra.PluginAsModule(swagger.New(swagger.Config{BasePath: "/docs"})),
    &APIModule{db: db},
)
```

Not sure which to reach for? See the [Architecture Guide: Module / Plugin / DI Container](../guides/architecture.md).

---

### Lifecycle

```go
app.OnStart(func(ctx context.Context) error {
    return db.Connect(ctx)
})
app.OnStop(func(ctx context.Context) error {
    return db.Close(ctx)
})
```

Hooks execute **serially** in registration order, receiving a context with a deadline.

---

### Startup Methods

| Method | Description |
|--------|-------------|
| `Run(addr)` | HTTP/1.1 + HTTP/2 (h2c) |
| `RunTLS(addr, cert, key)` | HTTPS (TLS 1.2+) |
| `RunQUIC(addr, cert, key)` | HTTP/3 (QUIC) + TLS upgrade Alt-Svc |
| `RunServer(srv *http.Server)` | Custom `http.Server` |
| `RunReactor(addr)` | Reactor mode (epoll/kqueue) |
| `ServeHTTP(w, r)` | Implements `http.Handler`, embeddable in any HTTP server |

All startup methods perform **graceful shutdown** (30 s timeout) upon receiving `SIGINT` / `SIGTERM`.

---

## Context

`Context` wraps `http.ResponseWriter` + `*http.Request`, providing all the framework's request/response convenience methods.

### Alias

```go
type Context = Ctx   // *astra.Context is an alias for *astra.Ctx
```

---

### Reading Requests

```go
// Path parameters
id := c.Param("id")               // string

// Query string
q  := c.Query("q")                // string
q  := c.QueryDefault("q", "all")  // string with default value
qs := c.QueryArray("tag")         // []string

// Header
token := c.Header("Authorization") // string

// Cookie
v, err := c.Cookie("session")

// Raw request
r := c.Request()   // *http.Request
```

---

### Binding and Validation

```go
type CreateUserReq struct {
    Name  string `json:"name"  validate:"required,max=64"`
    Email string `json:"email" validate:"required,email"`
}

var req CreateUserReq
if err := c.ShouldBind(&req); err != nil {
    return err   // returns *ValidationError; framework auto-renders 422
}
// MustBind: panics on failure (use in tests only)
c.MustBind(&req)
```

`ShouldBind` automatically selects the decoder based on `Content-Type` (JSON/XML/YAML/TOML/Form).
Struct fields support `form:`, `query:`, `json:`, `header:`, `cookie:` tags.

---

### Response Rendering

```go
c.JSON(200, data)                     // application/json
c.XML(200, data)                      // application/xml
c.String(200, "hello %s", name)       // text/plain
c.HTML(200, "<h1>%s</h1>", title)     // text/html
c.Blob(200, "image/png", bytes)       // binary
c.File("/path/to/file.pdf")           // file download (Content-Disposition)
c.Stream(200, "text/event-stream", r) // streaming response
c.NoContent(204)                      // no response body
c.Redirect(302, "/login")
```

---

### Returning Errors

```go
// Return an error directly from a handler; the framework handles it centrally
return astra.NewHTTPError(404, "user not found")
return astra.NewHTTPError(422, astra.H{"field": "email", "msg": "invalid"})

// With a business error code
return astra.NewAppError("USER_NOT_FOUND", 404, "user does not exist")
```

---

### Context Store

```go
c.Set("user", user)
v := c.Get("user")          // any
u := c.MustGet("user")      // any; panics if key is absent
```

---

### IP / Network

```go
ip  := c.ClientIP()         // real client IP, piercing trusted proxies
raw := c.RealIP()           // RemoteAddr (raw connection IP)
```

---

### Direct Writer Access

```go
w := c.Writer()             // http.ResponseWriter
w.Header().Set("X-Custom", "v")
w.WriteHeader(200)
fmt.Fprint(w, "raw output")
```

---

## Router

The router is implemented with a **Radix Tree**, with a separate tree per HTTP method.

### Conflict Detection

Since v0.9+, registering two handlers for the same method+path emits a `slog.Warn`:

```
WARN  astra: route conflict: handler overwritten  method=GET path=/users/:id
```

The newly registered handler **overwrites** the old one. Different methods on the same path do not conflict.

### HandlersChain

Each route corresponds to a `HandlersChain` (`[]HandlerFunc`). The framework calls them in order; any non-nil error breaks the chain and is passed to the error handler.

```go
type HandlerFunc    func(c *Context) error
type MiddlewareFunc func(c *Context) error   // same type, semantically "not a terminal"
type HandlersChain  []HandlerFunc
```

---

## Error Types

| Type | Constructor | Description |
|------|-------------|-------------|
| `*HTTPError` | `NewHTTPError(code, msg...)` | HTTP status code + message |
| `*AppError` | `NewAppError(bizCode, httpStatus, msg)` | Business code + HTTP status |
| `ValidationErrors` | Produced automatically by `ShouldBind` | Per-field validation error list |

```go
var he *astra.HTTPError
if errors.As(err, &he) {
    log.Printf("http %d: %v", he.Code, he.Message)
}

var ae *astra.AppError
if errors.As(err, &ae) {
    log.Printf("biz %s: %v", ae.BizCode, ae.Message)
}
```
