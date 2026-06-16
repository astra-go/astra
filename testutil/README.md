# TestUtil — Application Testing Utilities

Astra testing utilities for HTTP route and response assertion testing.

## Features

- **Test App Creation**: No need to start a real HTTP server
- **Request Construction**: `GET`, `POST` etc. for quick HTTP request construction
- **Response Assertions**: Assert status codes, response body content, JSON fields
- **Middleware Testing**: Can register middleware in test app then test
- **Ginkgo Integration**: `testutil` supports Ginkgo BDD-style testing framework

## Quick Start

```go
import (
    "testing"
    "github.com/astra-go/astra/testutil"
)

func TestHello(t *testing.T) {
    app := testutil.NewTestApp()

    app.GET("/hello", func(c *astra.Ctx) error {
        return c.String(200, "Hello, World!")
    })

    ts := testutil.NewServer(t, app)
    ts.GET("/hello").
        AssertStatus(200).
        AssertBodyContains("Hello")
}
```

## API

### NewTestApp

```go
func NewTestApp() *astra.App
```

Creates test App (doesn't listen on port, doesn't start server).

### NewServer

```go
func NewServer(t TB, app *astra.App) *httptest.Server
```

Creates real HTTP test server (using `net/http/httptest`).

### Request Construction

```go
ts.GET(path string)       *RequestBuilder
ts.POST(path string)      *RequestBuilder
ts.PUT(path string)       *RequestBuilder
ts.DELETE(path string)    *RequestBuilder
ts.PATCH(path string)     *RequestBuilder
ts.OPTIONS(path string)   *RequestBuilder
```

### RequestBuilder

```go
// Set request body
r.WithJSON(body)          // JSON body
r.WithForm(kv)           // Form data
r.WithHeader(key, val)    // Custom header

// Send and assert
r.AssertStatus(code int)  *Response
r.AssertJSON(path string, expect any) // Assert JSON field
r.AssertBodyContains(substr string)
r.AssertHeader(key, expectVal string)
```

## Complete Example

```go
package main

import (
    "encoding/json"
    "net/http"
    "strings"
    "testing"

    "github.com/astra-go/astra"
    "github.com/astra-go/astra/testutil"
)

func TestUserAPI(t *testing.T) {
    app := testutil.NewTestApp()

    // Register routes and middleware
    app.Use(middleware.Logger())
    app.GET("/users/:id", func(c *astra.Ctx) error {
        return c.JSON(200, astra.Map{
            "id":   c.Param("id"),
            "name": "Alice",
        })
    })

    ts := testutil.NewServer(t, app)

    // Test route matching
    ts.GET("/users/42").
        AssertStatus(http.StatusOK).
        AssertJSON("id", "42").
        AssertJSON("name", "Alice")

    // Test JSON request
    body := `{"email": "alice@example.com"}`
    ts.POST("/users").
        WithHeader("Content-Type", "application/json").
        WithJSON(map[string]string{"email": "alice@example.com"}).
        AssertStatus(http.StatusCreated).
        AssertBodyContains("created")
}

func TestMiddlewareChain(t *testing.T) {
    var called bool
    app := testutil.NewTestApp()

    app.Use(func(next astra.HandlerFunc) astra.HandlerFunc {
        return func(c *astra.Ctx) error {
            called = true
            return next(c)
        }
    })
    app.GET("/test", func(c *astra.Ctx) error {
        return c.String(200, "ok")
    })

    ts := testutil.NewServer(t, app)
    ts.GET("/test").AssertStatus(200)

    if !called {
        t.Error("Middleware was not called")
    }
}
```

## Module Dependencies

- `net/http/httptest` — Standard library HTTP testing utilities

## Notes

- `testutil.NewTestApp()` creates App without any built-in middleware registered; must register manually
- `testutil.NewServer` uses a real HTTP server; suitable for integration testing
- In unit tests, use `NewTestApp()` to test handlers directly without starting server
