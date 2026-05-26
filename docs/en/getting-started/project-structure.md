# Recommended Project Structure

```
myapp/
├── cmd/
│   └── server/
│       └── main.go          # Entry point: App init, dependency injection, startup
├── internal/
│   ├── handler/             # HTTP handler functions
│   │   ├── user.go
│   │   └── order.go
│   ├── service/             # Business logic (no framework dependencies)
│   ├── repository/          # Data access layer
│   └── middleware/          # Project-specific custom middleware
├── config/
│   └── config.yaml          # Configuration file
├── docs/                    # API documentation (Swagger/OpenAPI)
├── migrations/              # Database migration files
├── go.mod
└── go.sum
```

## Entry Point Example

```go
// cmd/server/main.go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
    "myapp/internal/handler"
)

func main() {
    app := astra.New(
        astra.WithLogger(initLogger()),
        astra.WithTrustedProxies("10.0.0.0/8"),
    )

    app.Use(
        middleware.Recovery(),
        middleware.Logger(),
        middleware.RequestID(),
        middleware.SecureHeaders(),
    )

    handler.RegisterRoutes(app)

    app.OnStart(initDB)
    app.OnStop(closeDB)

    app.Run(":8080")
}
```

## Handler File Example

```go
// internal/handler/user.go
package handler

import "github.com/astra-go/astra"

func RegisterRoutes(app *astra.App) {
    g := app.Group("/api/v1/users")
    g.GET("",     listUsers)
    g.POST("",    createUser)
    g.GET("/:id", getUser)
}
```
