# Core Concepts

## 1. App — Application Instance

`App` is the core of Astra, managing routes, middleware, lifecycle hooks, and the HTTP server.

```go
// Create with defaults
app := astra.New()

// Custom configuration
app := astra.New(
    astra.WithMode(astra.ModeProd),        // Production mode
    astra.WithShutdownTimeout(30),          // Graceful shutdown timeout
    astra.WithSerializer(sonic.ConfigStd), // Use Sonic JSON
    astra.WithRenderer(render.HTMLEngine{TemplateDir: "templates/"}),
)
```

### Lifecycle

App provides before-start / after-stop hooks:

```go
// Runs before start
app.OnStart(func(ctx context.Context) error {
    db, _ := gorm.Open(...)
    app.Set("db", db) // Store shared app-level data
    return nil
})

// Runs on stop (reverse order)
app.OnStop(func(ctx context.Context) error {
    return db.Close()
})

// Lifecycle is automatic — app.Run() runs OnStart first, then starts HTTP;
// on signal, HTTP closes first, then OnStop runs
```

## 2. Ctx — Request Context

`Ctx` is the per-request context object, using **zero-allocation design** (pool reuse, inline arrays) to provide a complete set of request/response operations.

```go
app.GET("/example", func(c *astra.Ctx) error {
    // ── Request Info ──
    path := c.Path()              // Request path
    method := c.Method()          // HTTP method
    ip := c.ClientIP()            // Client IP
    ua := c.UserAgent()           // User-Agent
    ct := c.ContentType()         // Content-Type

    // ── Parameter Retrieval ──
    name := c.Param("name")       // URL path param /users/:name
    idInt, _ := strconv.Atoi(c.Param("id")) // Auto-convert to int
    q := c.Query("q")             // Query param ?q=hello
    page := c.DefaultQuery("page", "1")

    // ── Request Body Binding ──
    var user User
    c.BindJSON(&user)             // JSON binding + validation
    c.BindForm(&user)             // Form binding
    c.BindQuery(&user)            // Query param binding
    c.Bind(&user)                 // Auto-infer Content-Type

    // ── Response Output ──
    c.JSON(200, obj)              // JSON response
    c.XML(200, obj)               // XML response
    c.String(200, "text")         // Plain text
    c.HTML(200, "<h1>Hi</h1>")   // HTML
    c.NoContent(204)              // No content
    c.Redirect(301, "/new")       // Redirect
    c.File("report.pdf")          // File response
    c.Blob(200, "image/png", data) // Binary data

    // ── Context Storage ──
    c.Set("user_id", 42)           // Store request-level data
    uid := c.GetInt("user_id")     // Read

    // ── Request Flow Control ──
    c.Next()                      // Pass to next middleware/handler
    c.Abort()                     // Stop subsequent processing
    c.AbortWithStatus(403)        // Stop and return status
})
```

### Zero-Allocation Design

Ctx reuses objects via sync.Pool — each request gets from the pool and returns to it:

```go
// View pool statistics
stats := app.PoolStats()
fmt.Printf("Hit: %d, Miss: %d, Active: %d\n", stats.Hit, stats.Miss, stats.Active)
```

## 3. Routing System

Astra uses **radix-tree** (基数树) for high-performance routing, supporting parameterized paths and middleware chains.

### Route Registration

```go
// Basic routes
app.GET("/users", handler)
app.POST("/users", handler)
app.PUT("/users/:id", handler)
app.DELETE("/users/:id", handler)
app.PATCH("/users/:id", handler)
app.HEAD("/ping", handler)
app.OPTIONS("/users", handler)

// Match all methods
app.Any("/webhook", handler)

// Static files
app.Static("/static", "./public")
app.StaticFile("/favicon.ico", "./favicon.ico")
```

### Route Groups

```go
// Groups share prefix and middleware
api := app.Group("/api/v1", authMiddleware)
{
    api.GET("/users", listUsers)
    api.POST("/users", createUser)

    // Nested groups
    admin := api.Group("/admin", adminMiddleware)
    {
        admin.GET("/dashboard", dashboard)
    }
}
```

### Route Parameters

```go
// Named params :param
app.GET("/users/:id", func(c *astra.Ctx) error {
    id := c.Param("id")       // String
    idInt, _ := strconv.Atoi(c.Param("id")) // Auto-convert to int
    return c.JSON(200, astra.Map{"id": id})
})

// Wildcard *
app.GET("/files/*path", func(c *astra.Ctx) error {
    path := c.Param("path") // Includes leading /
    return c.File("." + path)
})
```

## 4. Middleware

Middleware functions process the request/response chain, executing before or after route handlers.

### Using Middleware

```go
// Global middleware (all routes)
app.Use(middleware.Logger())
app.Use(middleware.Recovery())

// Group middleware (this group only)
api := app.Group("/api", middleware.CORS("*"))
api.Use(middleware.RequestID())

// Route-level middleware
app.GET("/admin", adminMiddleware, adminHandler)
```

### Writing Custom Middleware

```go
func AuthMiddleware(c *astra.Ctx) error {
    token := c.Header("Authorization")
    if token == "" {
        return astra.NewHTTPError(401, "missing token")
    }
    // Validate token...
    c.Set("user_id", 42)
    return c.Next() // Pass through
}

func TimingMiddleware(c *astra.Ctx) error {
    start := time.Now()
    err := c.Next() // Execute subsequent handlers
    elapsed := time.Since(start)
    slog.Info("request completed", "path", c.Path(), "elapsed", elapsed)
    return err
}
```

## 5. Component

Component is Astra's unified modular interface — all extension modules implement this interface:

```go
type Component interface {
    Name() string
    Init(app *App) error
}
```

```go
// Register components
app.Register(
    health.NewModule(health.WithProbe("db", dbProbe)),
    myComponent,
)

// Inline component
app.Register(astra.NewComponentFunc("setup", func(app *astra.App) error {
    app.Use(middleware.CORS("*"))
    return nil
}))
```

## 6. Error Handling

Astra provides two error types:

### HTTP Errors

```go
// Predefined HTTP errors
return astra.ErrNotFound
return astra.ErrUnauthorized
return astra.ErrBadRequest
return astra.ErrInternalServerError

// With custom message
return astra.NewHTTPError(400, "invalid email format")
```

### Business Errors

```go
// Define business error
var ErrUserNotFound = astra.NewAppError(
    "USER_NOT_FOUND",
    http.StatusNotFound,
    "User does not exist",
)

// Attach context when using
return ErrUserNotFound.WithData(astra.Map{"user_id": id})
return ErrUserNotFound.WithInternal(dbErr) // Keep internal error (not exposed to client)
```

## 7. Error Handler Hook

```go
app := astra.New(
    astra.WithErrorHandler(func(c *astra.Ctx, err error) {
        // Custom global error handling
        code := http.StatusInternalServerError
        if httpErr, ok := err.(*astra.HTTPError); ok {
            code = httpErr.Code
        }
        c.JSON(code, astra.Map{"error": err.Error()})
    }),
)
```
