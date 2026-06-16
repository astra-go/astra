# Router — Radix-Tree High-Performance Router

HTTP router based on method-keyed radix trie.

## Features

- **O(k) Time Complexity**: k = number of URL path characters, close to constant time
- **Named Parameters**: `/:param` style parameter routes, auto-extracted to `c.Param("param")`
- **Wildcards**: `/static/*filepath` captures remaining path
- **Exact Match Priority**: Static routes take priority over parameter routes, avoids `/users/new` being matched by `/:id`
- **MethodNotAllowed**: Auto-detects and returns 405 with correct Allow header
- **Route Snapshot**: Supports export/import route tree for testing and documentation generation

## Route Syntax

| Syntax | Description | Example |
|--------|-------------|---------|
| `/:param` | Named parameter | `/:id` → `/123` |
| `/*filepath` | Wildcard (captures remaining path) | `/static/*filepath` |
| `/:param+` | Parameter repetition (one or more segments) | `/:path+` → `/a/b/c` |
| `/:file(.*\.png)` | Regex constraint (Go regexp format) | — |

## Quick Start

```go
import "github.com/astra-go/astra/router"

r := router.New()

// Static routes
r.Add(http.MethodGet, "/", indexHandler)
r.Add(http.MethodGet, "/about", aboutHandler)

// Parameter routes
r.Add(http.MethodGet, "/users/:id", getUser)

// Regex constraint
r.Add(http.MethodGet, "/files/:filename(.*\\.png)", servePNG)
```

## API

### New

```go
func New() *Router
```

### router.Add

```go
func (r *Router) Add(method, path string, handlers ...astra.HandlerFunc) *Router
```

Registers route. `handlers` is middleware chain (last is handler).

### router.Handle

```go
func (r *Router) Handle(c *astra.Ctx) error
```

Handles request, typical usage:

```go
app.Use(func(c *astra.Ctx) error {
    return r.Handle(c)
})
```

### c.Param

```go
func (c *astra.Ctx) Param(key string) string
```

Gets route parameter value:

```go
app.GET("/users/:id/posts/:post_id", func(c *astra.Ctx) error {
    userID := c.Param("id")
    postID := c.Param("post_id")
    return c.String(200, fmt.Sprintf("user=%s, post=%s", userID, postID))
})
```

### Other Route Methods

```go
r.GET(path, handlers...)   // = r.Add(http.MethodGet, path, handlers...)
r.POST(path, handlers...)
r.PUT(path, handlers...)
r.PATCH(path, handlers...)
r.DELETE(path, handlers...)
r.OPTIONS(path, handlers...)
r.HEAD(path, handlers...)
```

## Config

### StrictSlash (Trailing Slash Redirect)

```go
r := router.New(router.WithStrictSlash(true))
// /users/ redirects to /users
```

### WithMaxParamValueLen

```go
r := router.New(router.WithMaxParamValueLen(256))
// Rejects path parameter values exceeding 256 bytes
```

## Complete Example

```go
package main

import (
    "fmt"
    "net/http"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/router"
)

func main() {
    r := router.New()

    r.GET("/", func(c *astra.Ctx) error {
        return c.String(200, "Home")
    })

    r.GET("/users/:id", func(c *astra.Ctx) error {
        return c.String(200, fmt.Sprintf("User ID: %s", c.Param("id")))
    })

    r.GET("/files/*filepath", func(c *astra.Ctx) error {
        return c.String(200, fmt.Sprintf("File path: %s", c.Param("filepath")))
    })

    app := astra.New()
    app.Use(func(c *astra.Ctx) error {
        return r.Handle(c)
    })

    app.Run(":8080")
}
```

## Module Dependencies

No external dependencies.

## Notes

- Routes typically registered via `astra.New()`, no need to use router package directly
- Wildcard parameter `*filepath` doesn't include leading `/` when retrieved
- When `/users/new` and `/:id` are both registered, the former takes priority (exact match rule)
- Parameter values have no length limit by default; limit via `WithMaxParamValueLen`
