# Best Practices

## Project Structure

Recommended project structure:

```
myapp/
├── cmd/
│   └── server/
│       └── main.go          # Entry point (create App, register components, start)
├── internal/
│   ├── handler/             # Request handlers
│   ├── service/             # Business logic layer
│   ├── repository/          # Data access layer
│   ├── middleware/          # Custom middleware
│   └── model/               # Data models
├── pkg/
│   └── config/              # Config structs
├── config/
│   ├── config.yaml          # Default config
│   ├── config.dev.yaml      # Dev environment
│   └── config.prod.yaml     # Production environment
├── migrations/              # Database migrations
├── public/                  # Static assets
├── templates/               # HTML templates
├── Dockerfile
├── Makefile
├── go.mod
└── go.sum
```

## Config Management

1. **Layered Config**: defaults → file → env vars → remote config center
2. **Sensitive Info**: passwords and keys always injected via env vars or Secret management
3. **Config Validation**: validate all config at startup using `config.Validate()`
4. **Hot Reload**: use remote config center for no-restart updates in production
5. **Validation**: validate all config at startup to avoid runtime issues

```go
type Config struct {
    Server ServerConfig `yaml:"server"`
    DB     DBConfig    `yaml:"db"     validate:"required"`
    Redis  RedisConfig `yaml:"redis"  validate:"required"`
}

// Validate at startup
var cfg Config
config.Scan(&cfg)
if err := config.Validate(&cfg); err != nil {
    log.Fatalf("Config validation failed: %v", err)
}
```

## Error Handling

### Unified Error Format

```go
// Global error handler
app := astra.New(
    astra.WithErrorHandler(func(c *astra.Ctx, err error) {
        code := http.StatusInternalServerError
        message := "internal server error"

        var httpErr *astra.HTTPError
        if errors.As(err, &httpErr) {
            code = httpErr.Code
            message = httpErr.Message
        }

        var appErr *astra.AppError
        if errors.As(err, &appErr) {
            c.JSON(appErr.HTTPStatus, astra.Map{
                "code":    appErr.Code,
                "message": appErr.Message,
            })
            return
        }

        c.JSON(code, astra.Map{"error": message})
    }),
)

// Define business error codes
var (
    ErrUserNotFound         = astra.NewAppError("USER_NOT_FOUND", 404, "User does not exist")
    ErrInvalidInput          = astra.NewAppError("INVALID_INPUT",  400, "Invalid input parameters")
    ErrInsufficientBalance   = astra.NewAppError("INSUFFICIENT_BALANCE", 402, "Insufficient balance")
)
```

## Logging

```go
import "log/slog"

// Use slog structured logging
slog.Info("user registered",
    "user_id", user.ID,
    "email", user.Email,
    "source", c.ClientIP(),
)

// Set log level
slog.SetLogLoggerLevel(slog.LevelDebug) // Development
slog.SetLogLoggerLevel(slog.LevelInfo)  // Production
```

## Performance Optimization

### 1. JSON Serialization

Use Sonic instead of default JSON:

```go
import "github.com/bytedance/sonic"

app := astra.New(
    astra.WithSerializer(sonic.ConfigStd),
)
```

### 2. Connection Pool

```go
// Database connection pool
orm.SetPool(db, orm.PoolConfig{
    MaxOpenConns:    100,               // CPU cores * 2~4
    MaxIdleConns:    25,
    ConnMaxLifetime: 30 * time.Minute,
})

// Redis connection pool
redisClient := redis.NewClient(&redis.Options{
    Addr:         "localhost:6379",
    PoolSize:     20,
    MinIdleConns: 5,
})
```

### 3. Cache Strategy

- Use cache for data with high read frequency and low update frequency
- Set reasonable TTL (5~30 minutes recommended)
- Delete cache on update instead of updating cache (Cache-Aside pattern)
- Use GetOrSet to prevent cache penetration

### 4. Middleware Optimization

```go
// Health check paths skip logging and auth
app.Use(func(c *astra.Ctx) error {
    if c.Path() == "/ping" || c.Path() == "/health" {
        return c.Next()
    }
    return nil  // Skip this middleware
})
```

## Security

### Least-Privilege Middleware

```go
// Configure different middleware per route group
public := app.Group("/api/public")
public.GET("/health", healthHandler)

api := app.Group("/api/v1")
api.Use(sec.JWT("secret"))
// In-memory rate limiting
api.Use(sec.RateLimit(100, 200))

// Distributed rate limiting (requires -tags redis build)
api.Use(sec.DistributedRateLimit("localhost:6379", 100, 200))

admin := api.Group("/admin", sec.IPFilter(sec.IPFilterConfig{
    AllowIPs: []string{"10.0.0.0/8"},
    Mode:     sec.Whitelist,
}))
admin.GET("/users", adminHandler)
```

### Security Headers

```go
app.Use(middleware.Secure())
app.Use(middleware.CSP(middleware.CSPConfig{...}))
```

## Testing

Astra provides a `testutil` package to simplify testing:

```go
import (
    "testing"
    "github.com/astra-go/astra/testutil"
    "github.com/astra-go/astra"
)

func TestGetUser(t *testing.T) {
    app := testutil.NewTestApp()
    app.GET("/users/:id", getUserHandler)

    s := testutil.NewServer(t, app)
    
    // GET request test
    s.GET("/users/1").
        AssertStatus(200).
        AssertBodyContains("Alice").
        AssertHeader("Content-Type", "application/json")

    // POST request test
    s.POST("/users").
        WithJSON(astra.Map{"name": "Bob"}).
        AssertStatus(201)

    // With headers
    s.GET("/users/1").
        WithHeader("Authorization", "Bearer token").
        AssertStatus(200)
}
```

## Graceful Shutdown

```go
func main() {
    app := astra.New(astra.WithShutdownTimeout(30 * time.Second))

    // Register cleanup hooks (reverse order)
    app.OnStop(func(ctx context.Context) error {
        slog.Info("closing database connection...")
        return db.Close()
    })
    app.OnStop(func(ctx context.Context) error {
        slog.Info("closing Redis connection...")
        return redisClient.Close()
    })
    app.OnStop(func(ctx context.Context) error {
        slog.Info("closing message queue producer...")
        return producer.Close()
    })

    slog.Info("starting server...")
    if err := app.Run(":8080"); err != nil {
        slog.Error("server exited with error", "error", err)
    }
    slog.Info("server gracefully stopped")
}
```

## Version Management

Astra sub-modules have independent version numbers. Recommendations:

1. **Core Framework**: follow major version upgrades
2. **Extension Modules**: select versions as needed, no need to upgrade all at once
3. **go.mod Lock**: commit go.sum to ensure reproducible builds

```go
// Use replace directive for local dev/debug
replace github.com/astra-go/astra => /path/to/local/astra
```

## Monitoring

```go
import (
    "github.com/astra-go/astra/health"
    "github.com/astra-go/astra/metrics"
)

app.Register(health.NewModule(
    health.WithProbe("db", dbHealthCheck),
    health.WithProbe("redis", redisHealthCheck),
))

// Start metrics server (exposes /metrics, /health, /dashboard)
ms := metrics.NewServer(":9091")
go ms.Start()

// Or use middleware for metrics collection
app.Use(metrics.Middleware())
```
