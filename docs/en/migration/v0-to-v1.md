# Migration Guide: v0.x → v1.0

This guide covers all changes required when upgrading from any v0.x release to **v1.0.0**.

!!! warning "Production Upgrade Recommendation"
    Validate in a staging environment before upgrading production. It is recommended to upgrade to v0.10.x (the final 0.x release) first, then to v1.0, so that deprecation warnings can surface issues in advance.

---

## One-Click Check Script

Run the following script from the project root to automatically scan for code patterns that need manual attention:

```bash
#!/usr/bin/env bash
echo "=== Checking v0→v1 migration issues ==="

# 1. Old SetLogger calls
grep -rn "\.SetLogger(" --include="*.go" . && echo "⚠️  Please use astra.WithLogger() instead"

# 2. c.JSON(data) missing status code
grep -rn 'c\.JSON([^2]' --include="*.go" . | grep -v 'c\.JSON(2[0-9][0-9]' && \
    echo "⚠️  c.JSON requires a status code as the first argument"

# 3. Old http.HandlerFunc not wrapped
grep -rn 'app\.\(GET\|POST\|PUT\|DELETE\|PATCH\)\(.*http\.HandlerFunc' --include="*.go" . && \
    echo "⚠️  Please wrap http.HandlerFunc with astra.WrapH()"

# 4. TrustedProxies string setter (removed in v0.9)
grep -rn '\.SetTrustedProxies(' --include="*.go" . && \
    echo "⚠️  Please use astra.WithTrustedProxies() Option instead"

echo "=== Scan complete ==="
```

---

## Change Details

### 1. Handler Signature (introduced in v0.2.0, finalized in v1.0)

**v0.1.x (removed)**:

```go
// Old: standard library signature
app.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("hello"))
})
```

**v1.0 (current)**:

```go
// New: Astra Context signature
app.GET("/hello", func(c *astra.Context) error {
    return c.String(200, "hello")
})

// Migrating existing http.HandlerFunc
app.GET("/legacy", astra.WrapH(myOldHandler))
```

**Migration steps**:

```bash
# For large codebases, sed can assist (manual review required)
# Change func(w http.ResponseWriter, r *http.Request) to func(c *astra.Context) error
```

---

### 2. `c.JSON` / `c.XML` Require a Status Code (introduced in v0.3.0, old API removed in v1.0)

**Old (v0.2.x)**:

```go
c.JSON(data)        // implicit 200
c.XML(data)         // implicit 200
```

**New (v1.0)**:

```go
c.JSON(200, data)   // explicit status code
c.XML(200, data)
c.JSON(201, created)
```

**Bulk migration**:

```bash
# For reference only — manually confirm the intended status code at each location
sed -i 's/c\.JSON(\([^,0-9]\)/c.JSON(200, \1/g' **/*.go
sed -i 's/c\.XML(\([^,0-9]\)/c.XML(200, \1/g'  **/*.go
```

---

### 3. `App.SetLogger` Removed (deprecated in v0.6.0, removed in v1.0)

**Old**:

```go
app := astra.New()
app.SetLogger(myLogger)
```

**New**:

```go
app := astra.New(astra.WithLogger(myLogger))
```

---

### 4. `App.SetTrustedProxies` Removed (deprecated in v0.9.0, removed in v1.0)

**Old**:

```go
app.SetTrustedProxies("10.0.0.0/8")
```

**New**:

```go
app := astra.New(astra.WithTrustedProxies("10.0.0.0/8", "172.16.0.0/12"))
```

!!! note "Behavior Change"
    Since v0.9+, trusted proxy CIDRs are compiled into `*net.IPNet` once at `New()` time;
    `ClientIP()` performs right-to-left XFF traversal. The left-to-right traversal of old versions **is no longer supported**.

---

### 5. `OnStart` / `OnStop` Hooks Serialized (v0.8.0)

In v0.7.x and earlier, multiple hooks ran concurrently (this was a bug).
v0.8.0+ changed this to **serial execution in registration order**.

**For scenarios requiring concurrent execution**:

```go
// Old: relying on concurrent side effects (not recommended)
app.OnStart(startDB)
app.OnStart(startCache)  // ran concurrently with startDB in v0.7.x

// New: manage concurrency within a single hook
app.OnStart(func(ctx context.Context) error {
    var eg errgroup.Group
    eg.Go(func() error { return startDB(ctx) })
    eg.Go(func() error { return startCache(ctx) })
    return eg.Wait()
})
```

---

### 6. `middleware.RateLimit` Cleanup Goroutine Control (v0.9.0 / v1.0)

The cleanup goroutine started internally by `RateLimit(rate, burst)` no longer runs forever on
`context.Background()` — the app's stop hook should cancel it.

**Recommended usage (v1.0)**:

```go
// Option A: NewRateLimiter (simplest, test-friendly)
mw, stop := middleware.NewRateLimiter(100, 20)
app.OnStop(func(_ context.Context) error { stop(); return nil })
app.Use(mw)

// Option B: control via the Context field
ctx, cancel := context.WithCancel(context.Background())
app.OnStop(func(_ context.Context) error { cancel(); return nil })
app.Use(middleware.RateLimitWithConfig(middleware.RateLimitConfig{
    Rate: 100, Burst: 20, Context: ctx,
}))
```

**If no control is needed** (acceptable for top-level apps):

```go
app.Use(middleware.RateLimit(100, 20))
// goroutine exits with the process — no leak
```

---

### 7. Duplicate Route Registration Behavior Change (v0.9.0 / v1.0)

Since v0.9+, registering a duplicate method+path emits `slog.Warn` (previously silently overwritten).
**Behavior is unchanged** (new handler overwrites old handler), but the log surfaces potential registration errors.

If your code has **intentional** overrides (e.g. test mocks), use route groups or separate `App` instances to avoid confusion.

---

### 8. `binding.MaxSliceParams` Default Limit (v0.9.0 / v1.0)

Slice fields are populated with at most **1000** elements (`MaxSliceParams = 1000`) during binding.
Elements beyond this limit are silently truncated.

If your use case genuinely requires larger slice inputs, use a JSON body instead:

```go
// Not recommended: large arrays in query string
GET /api?ids=1&ids=2&...&ids=5000   // truncated beyond 1000

// Recommended: JSON body
POST /api
{"ids": [1, 2, ..., 5000]}
```

---

## Upgrade Steps Summary

```bash
# 1. Update the dependency
go get github.com/astra-go/astra@v1.0.0

# 2. Run the migration check script (see above)

# 3. Fix compilation errors
go build ./...

# 4. Static analysis
staticcheck ./...

# 5. Run tests
go test ./... -race

# 6. Verify startup
go run ./cmd/server -addr :8080
```

---

## FAQ

**Q: Can v0.9 code be upgraded directly to v1.0?**

A: Usually yes. The v0.9 → v1.0 step has almost no new breaking changes (v1.0 is the stable release of v0.10).
Most changes were introduced between v0.2 and v0.9 — check the CHANGELOG to confirm the range of versions your upgrade path covers.

**Q: My project still uses `net/http` handlers. Do I have to migrate all of them?**

A: No. Use `astra.WrapH(h http.Handler)` to wrap them and keep using them as-is.
Incremental migration is recommended — prioritize new feature code first.

**Q: Will performance change after migration?**

A: The standard HTTP server performance in v1.0 is on par with v0.x.
If you want higher concurrency with the Reactor engine (epoll/kqueue):
```go
app.RunReactor(":8080")   // replaces app.Run(":8080")
```
