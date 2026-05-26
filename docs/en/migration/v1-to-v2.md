# Migration Guide: v1.x → v2.0

!!! info "Status: Planned"
    v2.0 has not been released yet. This document describes **planned** breaking changes
    to help you assess migration cost in advance.
    The final release may differ from this document — refer to the official CHANGELOG when it ships.

---

## Planned Changes Overview

| Change | Impact | Pre-deprecated in Version |
|--------|--------|--------------------------|
| `Context` changed to an interface type | Moderate | v1.2 |
| Remove `astra.H` type alias | Low | v1.1 |
| `MiddlewareFunc` parameter type narrowed | Low | v1.3 |
| Error handler signature unified | Low | v1.2 |
| Configuration options moved to `astra.Options` struct | Low | v1.1 |
| `Group` return type changed | Low | v1.3 |

---

## Change Details

### 1. `Context` Changed to an Interface Type

v2.0 will hide `*astra.Ctx` (the current concrete type) as a private implementation;
the public API will uniformly use `astra.Context` (interface).

**v1.x (current)**:

```go
func handler(c *astra.Context) error { ... }   // *Ctx concrete type
```

**v2.0 (planned)**:

```go
func handler(c astra.Context) error { ... }   // interface type
```

**Impact**: global replacement of `*astra.Context` → `astra.Context` in function signatures.
The `contract.Context` interface already exists; v2.0 will merge the two.

**Early adoption (under v1.x)**: use `contract.Context` instead of `*astra.Context`:

```go
// This already works today — no changes needed under v2.0
func handler(c contract.Context) error { ... }
```

---

### 2. Remove `astra.H` Type Alias

`astra.H` is a type alias for `map[string]any`. v2.0 removes this alias to reduce re-exports.

**v1.x**:

```go
c.JSON(200, astra.H{"key": "value"})
```

**v2.0**:

```go
c.JSON(200, map[string]any{"key": "value"})
// or define your own: type H = map[string]any
```

**Bulk migration** (v1.x → v2.0):

```bash
sed -i 's/astra\.H{/map[string]any{/g' **/*.go
```

---

### 3. Structured Configuration Options

v2.0 replaces the scattered `With*` option functions with a direct `astra.Options` struct,
reducing import-path dependencies and improving IDE autocomplete.

**v1.x**:

```go
app := astra.New(
    astra.WithLogger(l),
    astra.WithTrustedProxies("10.0.0.0/8"),
    astra.WithMaxMultipartMemory(32 << 20),
)
```

**v2.0 (planned)**:

```go
app := astra.New(astra.Options{
    Logger:              l,
    TrustedProxies:      []string{"10.0.0.0/8"},
    MaxMultipartMemory:  32 << 20,
})
```

`With*` functions will be marked Deprecated in v1.1 and removed in v2.0.

---

### 4. Error Handler Signature Unified

v2.0's global error handler accepts `astra.Context` (interface), complementing change #1.

**v1.x**:

```go
astra.New(astra.WithErrorHandler(func(c *astra.Context, err error) {
    c.JSON(500, astra.H{"error": err.Error()})
}))
```

**v2.0**:

```go
astra.New(astra.Options{
    ErrorHandler: func(c astra.Context, err error) {
        c.JSON(500, map[string]any{"error": err.Error()})
    },
})
```

---

## Early Adoption Recommendations

Even before upgrading to v2.0, the following practices reduce future migration effort:

1. **Use `contract.Context` instead of `*astra.Context`** as the handler parameter type internally
2. **Avoid `astra.H`** — use `map[string]any` directly
3. **Use `middleware.NewRateLimiter`** instead of `middleware.RateLimit` (already recommended in v1.0)
4. **Keep handler functions stateless** — inject dependencies via `c.Get(key)` rather than closure capture

---

## Feedback

The v2.0 design is still under discussion — your input is welcome:

- [GitHub Discussion: v2.0 API Design](https://github.com/astra-go/astra/discussions)
- [RFC: Context as Interface](https://github.com/astra-go/astra/issues)
