# Quick Start

This tutorial takes you from zero to a complete Astra Web service in three steps.

## Step 1: Create Project

```bash
mkdir myapp && cd myapp
go mod init myapp
go get github.com/astra-go/astra@latest
```

## Step 2: Write Your First API

Create `main.go`:

```go
package main

import (
    "log"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
)

func main() {
    // Create app instance
    app := astra.New()

    // Register global middleware
    app.Use(middleware.Logger())   // Request logging
    app.Use(middleware.Recovery()) // Panic recovery

    // Route: GET /hello/:name
    app.GET("/hello/:name", func(c *astra.Ctx) error {
        name := c.Param("name")
        return c.JSON(200, astra.Map{
            "message": "Hello, " + name,
            "status":  "ok",
        })
    })

    // Route: GET /ping (health check)
    app.GET("/ping", func(c *astra.Ctx) error {
        return c.JSON(200, astra.Map{"pong": true})
    })

    // Start server
    log.Println("Server starting on :8080")
    log.Fatal(app.Run(":8080"))
}
```

## Step 3: Run and Test

```bash
go run main.go
```

In another terminal:

```bash
# Health check
curl http://localhost:8080/ping
# {"pong":true}

# Route with parameter
curl http://localhost:8080/hello/Astra
# {"message":"Hello, Astra","status":"ok"}

curl http://localhost:8080/hello/World
# {"message":"Hello, World","status":"ok"}
```

## Advanced: Full CRUD Example

```go
package main

import (
    "log"
    "strconv"
    "sync"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
)

type User struct {
    ID   int    `json:"id"`
    Name string `json:"name" validate:"required"`
}

var (
    users   = []User{{ID: 1, Name: "Alice"}}
    nextID  = 2
    usersMu sync.Mutex
)

func main() {
    app := astra.New()
    app.Use(middleware.Logger())
    app.Use(middleware.Recovery())
    app.Use(middleware.CORS("*"))

    // Route groups
    v1 := app.Group("/api/v1")
    {
        v1.GET("/users", listUsers)
        v1.POST("/users", createUser)
        v1.GET("/users/:id", getUser)
        v1.PUT("/users/:id", updateUser)
        v1.DELETE("/users/:id", deleteUser)
    }

    log.Fatal(app.Run(":8080"))
}

func listUsers(c *astra.Ctx) error {
    usersMu.Lock()
    defer usersMu.Unlock()
    return c.JSON(200, astra.Map{"users": users})
}

func createUser(c *astra.Ctx) error {
    var user User
    if err := c.BindJSON(&user); err != nil {
        return astra.NewHTTPError(400, "invalid request body")
    }
    if err := c.Validate(&user); err != nil {
        return astra.NewHTTPError(422, err.Error())
    }

    usersMu.Lock()
    user.ID = nextID
    nextID++
    users = append(users, user)
    usersMu.Unlock()

    return c.JSON(201, user)
}

func getUser(c *astra.Ctx) error {
    id, _ := strconv.Atoi(c.Param("id"))
    usersMu.Lock()
    defer usersMu.Unlock()

    for _, u := range users {
        if u.ID == id {
            return c.JSON(200, u)
        }
    }
    return astra.NewHTTPError(404, "user not found")
}

func updateUser(c *astra.Ctx) error {
    id, _ := strconv.Atoi(c.Param("id"))
    var update User
    if err := c.BindJSON(&update); err != nil {
        return astra.NewHTTPError(400, "invalid request body")
    }

    usersMu.Lock()
    defer usersMu.Unlock()

    for i, u := range users {
        if u.ID == id {
            users[i].Name = update.Name
            return c.JSON(200, users[i])
        }
    }
    return astra.NewHTTPError(404, "user not found")
}

func deleteUser(c *astra.Ctx) error {
    id, _ := strconv.Atoi(c.Param("id"))
    usersMu.Lock()
    defer usersMu.Unlock()

    for i, u := range users {
        if u.ID == id {
            users = append(users[:i], users[i+1:]...)
            return c.NoContent(204)
        }
    }
    return astra.NewHTTPError(404, "user not found")
}
```

Test CRUD:

```bash
# List
curl http://localhost:8080/api/v1/users

# Create
curl -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{"name":"Bob"}'

# Get single
curl http://localhost:8080/api/v1/users/1

# Update
curl -X PUT http://localhost:8080/api/v1/users/1 \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice Updated"}'

# Delete
curl -X DELETE http://localhost:8080/api/v1/users/1
```

## Next Steps

- [Core Concepts](core-concepts.md) — Deep dive into Astra's core components
- [Configuration](configuration.md) — Learn config management
- [Middleware](middleware.md) — Master the middleware system
- [Database ORM](database-orm.md) — Integrate databases
