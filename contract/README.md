# Contract — Framework Core Interface Definitions

Core interface and type definitions for Astra framework, decoupling framework core from sub-modules.

## Features

- **Context Interface**: Defines request context (Request/Response/Params/Binding); sub-modules communicate with framework via this interface
- **Binder Interface**: Data binding abstraction supporting JSON/XML/Form/Query/Path from multiple sources
- **HTTPError**: HTTP error type, supports status code + message + internal error combination
- **ValidationError**: Field-level validation error type
- **Repository[T]**: Generic data access abstraction; business layer depends on this interface instead of directly on GORM
- **TxRunner**: Transaction runner, supports Context-propagated transactions
- **Stream Interfaces**: ServerStream / ClientStream / BidiStream define server push, client push, bidirectional streaming

## Core Interfaces

### Context Interface

`contract.Context` is the abstract interface of Astra request context; sub-packages and middleware should depend on this interface rather than `*astra.Ctx`:

```go
type Context interface {
    // Request/Response
    Request() *http.Request
    Writer() ResponseWriter

    // Route params
    Param(key string) string

    // Query params
    Query(key string) string
    DefaultQuery(key, defaultValue string) string

    // Form data
    PostForm(key string) string
    FormFile(key string) (*multipart.FileHeader, error)

    // Data binding
    BindJSON(obj any) error
    BindQuery(obj any) error
    BindPath(obj any) error
    BindForm(obj any) error

    ShouldBindJSON(obj any) error
    ShouldBindQuery(obj any) error

    // ShouldBind vs Bind: ShouldBind doesn't auto-call Validate

    // Middleware chain control
    Next()
    Abort()
    AbortWithStatus(code int)

    // Request context storage (kv)
    Set(key string, val any)
    Get(key string) (any, bool)

    // JSON response
    JSON(code int, v any) error
    String(code int, s string) error

    // Parse language/locale
    Language() string
}
```

### ResponseWriter Interface

```go
type ResponseWriter interface {
    http.ResponseWriter
    Status() int      // Written status code
    Size() int         // Bytes written
    Written() bool    // Whether WriteHeader was called
}
```

### HTTPError

```go
// Create HTTP error
err := contract.NewHTTPError(404, "user not found")

// Attach internal error (not exposed externally)
err = err.WithInternal(dbErr)

// Type check
if errors.Is(err, contract.NewHTTPError(401)) { ... }
```

### ValidationError

```go
// Single field error
type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

// Batch validation errors
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
    // "field=name: cannot be blank; field=email: format error"
}
```

## Data Binding Tags

| Tag | Source | Example |
|-----|--------|---------|
| `uri:"name"` | URL path parameter | `/users/:id` → `id` |
| `query:"name"` | URL query parameter | `?page=1` → `page` |
| `form:"name"` | Form body | `POST` form field |
| `header:"name"` | Request header | `X-Request-Id` |
| `json:"name"` | JSON body | Request body |

## Module Dependencies

No external dependencies. The `contract` package is Astra's infrastructure layer defining core framework abstractions.

## Notes

- Sub-packages and middleware should use `contract.Context` instead of `*astra.Ctx` to stay decoupled from the framework
- Difference between `ShouldBind` and `Bind`: former doesn't auto-return error on binding failure, suitable for handler to decide response
- `Validator` registration is in `github.com/astra-go/astra/binding` package
