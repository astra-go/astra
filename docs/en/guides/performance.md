# Performance Tuning Guide

## Choosing an Engine

| Scenario | Recommended Engine | Reason |
|----------|--------------------|--------|
| Short requests, high QPS (API gateway) | `app.Run()` standard engine | Mature net/http scheduler |
| Long connections, high concurrency (WebSocket, SSE) | `app.RunReactor()` | O(1) goroutines, low memory |
| Ultra-low latency, SO_REUSEPORT needed | Reactor + ListenReusePort | Multi-process/container same-port listening |
| HTTPS public service | `app.RunTLS()` | Standard TLS 1.3 |
| Mobile / lossy networks | `app.RunQUIC()` | QUIC 0-RTT, packet loss recovery |

---

## Reactor Engine Tuning

```go
engine, _ := netengine.New(app, netengine.ReactorConfig{
    // Event loops = CPU count (IO-bound)
    NumLoops: runtime.NumCPU(),

    // Worker pool = CPU * 4 (CPU-bound handlers)
    // Worker pool = CPU * 8~16 (IO-wait handlers, e.g. database queries)
    WorkerPoolSize: runtime.NumCPU() * 8,

    // Connection queue buffer: concurrent connections / NumLoops
    ConnChannelBuffer: 256,
})
```

**Monitoring metrics**:

```go
// Prometheus metrics (requires middleware.Prometheus)
astra_active_conns          // current active connections
astra_worker_queue_depth    // worker queue backlog
astra_request_duration_ms   // P50/P95/P99 latency
```

---

## Memory Allocation Optimization

Astra uses `sync.Pool` internally to recycle `Context` objects.
Ways to reduce allocations at the handler level:

```go
// ✅ Write directly to the writer — zero-copy
c.Writer().Write(precomputedBytes)

// ✅ Use c.String instead of fmt.Sprintf + c.JSON
c.String(200, "hello %s", name)

// ⚠️ Avoid allocating maps on hot paths
// Not recommended:
c.JSON(200, map[string]any{"key": value})

// Recommended: use pre-defined structs
type Response struct { Key string `json:"key"` }
c.JSON(200, Response{Key: value})
```

---

## Router Performance

- Route tree lookup is `O(k)` (k = number of path segments), independent of total route count.
- Static routes take priority over parameter routes; parameter routes take priority over wildcard routes.
- Avoid registering too many middleware on high-frequency paths (each middleware adds one function call).

---

## Connection Pool Configuration

```go
// Database connection pool (avoid burst goroutine storms)
db, _ := orm.Open(orm.Config{
    DSN:             dsn,
    MaxOpenConns:    runtime.NumCPU() * 4,
    MaxIdleConns:    runtime.NumCPU(),
    ConnMaxLifetime: 30 * time.Minute,
    ConnMaxIdleTime: 5 * time.Minute,
})

// Redis connection pool
rdb := redis.NewClient(&redis.Options{
    PoolSize:        runtime.NumCPU() * 4,
    MinIdleConns:    runtime.NumCPU(),
    ConnMaxIdleTime: 5 * time.Minute,
})
```

---

## Compression Strategy

```go
app.Use(middleware.Gzip(middleware.GzipConfig{
    Level:     gzip.BestSpeed,  // speed over compression ratio (level 1, not level 9)
    MinLength: 1024,            // skip compression for responses < 1 KB (overhead > gain)
    SkipPaths: []string{
        "/metrics",   // Prometheus metrics (already text)
        "/ws",        // WebSocket
    },
}))
```

---

## pprof Profiling

```go
// Expose only on the management port
admin := astra.New()
admin.Use(middleware.Pprof("/debug/pprof"))
go admin.Run(":6060")
```

```bash
# CPU profile (30 seconds)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Memory heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine leak detection
curl http://localhost:6060/debug/pprof/goroutine?debug=1 | head -50
```

---

## Benchmark Results

The following data comes from `make bench-all`, tested on **Apple M4 · Go 1.22 · 3 rounds × 2s/round (loopback)**.

Run with:

```bash
make bench-all

# Compare against baseline
go install golang.org/x/perf/cmd/benchstat@latest
make bench-save-baseline
# … make changes …
make bench-compare
```

---

### Routing & Middleware Chain (root package)

| Benchmark | ns/op | B/op | allocs/op | Notes |
|-----------|------:|-----:|----------:|-------|
| `Router_Static` | 114 | 208 | 4 | Static path hit |
| `Router_Static_100` | 290 | 208 | 4 | 100-route tree; O(k) regardless of total routes |
| `Router_Param` | 116 | 208 | 4 | Single `:id` parameter |
| `Router_Param_Deep` | 146 | 208 | 4 | 3-segment param path |
| `Router_Regex` | 154 | 208 | 4 | `{id:[0-9]+}` regex constraint |
| `Router_Wildcard` | 120 | 208 | 4 | `*path` wildcard |
| `Router_NotFound` | 765 | 1 567 | 16 | No route matched |

**Middleware chain expansion cost** (~4 ns per additional handler, **zero extra heap allocations**):

| Handler count | ns/op | B/op | allocs/op |
|--------------:|------:|-----:|----------:|
| 0 (pure handler) | 113 | 208 | 4 |
| 1 | 114 | 208 | 4 |
| 3 | 119 | 208 | 4 |
| 5 | 122 | 208 | 4 |
| 10 | 136 | 208 | 4 |
| Abort (chain break) | 123 | 216 | 5 |

---

### Context Response Writing

| Benchmark | ns/op | B/op | allocs/op |
|-----------|------:|-----:|----------:|
| `Context_JSON_Small` (~120 B) | 450 | 1 028 | 9 |
| `Context_JSON_Medium` (~350 B) | 634 | 1 381 | 10 |
| `Context_JSON_Large` (100 items ~12 KB) | 6 299 | 25 953 | 13 |
| `Context_String` | 383 | 992 | 8 |
| `Context_QueryParams` (5 params) | 559 | 688 | 11 |
| `ServeHTTP_Parallel_Static` | 91 | 208 | 4 |
| `ServeHTTP_Parallel_JSON` | 450 | 1 284 | 10 |

---

### Reactor Engine (netengine)

**Worker Pool micro-benchmark (white-box, directly accessing internal types)**:

| Benchmark | ns/op | B/op | allocs/op |
|-----------|------:|-----:|----------:|
| `WorkerPool_TrySubmit` (single goroutine, non-blocking) | 130 | 0 | 0 |
| `WorkerPool_Submit_Parallel` (blocking concurrent) | 87 | 0 | 0 |
| `WorkerPool_TrySubmit_Parallel` (non-blocking concurrent) | 84 | 0 | 0 |

**Real TCP end-to-end round-trip** (reactor accept → event loop → worker → handler → response):

| Benchmark | ns/op | Equiv. QPS | B/op | allocs/op |
|-----------|------:|-----------:|-----:|----------:|
| `Reactor_HTTP_Keepalive` (single connection reuse, hot path) | 27 071 | ~37 000 | 21 019 | 24 |
| `Reactor_HTTP_NewConn` (new connection each time, cold path) | 83 598 | ~12 000 | 62 481 | 58 |
| `Reactor_HTTP_Parallel` (GOMAXPROCS concurrent) | 13 098 | ~76 000 | 8 656 | 23 |

> The new-connection path (83 µs) includes TCP three-way handshake (loopback ~10 µs) and reactor accept overhead.
> The concurrent persistent-connection path (13 µs / ~76k QPS) is the reference value for production.

---

### Per-Middleware Overhead

| Benchmark | ns/op | B/op | allocs/op | Notes |
|-----------|------:|-----:|----------:|-------|
| `CORS_Passthrough` | 140 | 208 | 4 | Same-origin request, no headers added |
| `CORS_CrossOrigin` | 478 | 944 | 8 | Cross-origin, writes ACAO header |
| `CORS_Preflight` | 949 | 1 580 | 16 | OPTIONS preflight, full allow list |
| `Recovery_NoPanic` | 142 | 208 | 4 | Normal path, near-zero overhead |
| `Recovery_Panic` | ~4 000 | ~1 500 | ~18 | panic → recover → 500 full path |
| `JWT_ValidToken` | 3 246 | 2 929 | 58 | HS256 valid (double-parse eliminated) |
| `JWT_MissingToken` | 824 | 1 555 | 15 | No token, early 401, no crypto |
| `JWT_InvalidSignature` | 2 751 | 4 141 | 68 | Bad signature, full parse + verify failure |

---

### Full-Stack Integration (complete request path)

`httptest.ResponseRecorder` measurement: routing → global middleware → route middleware → handler → response write.

| Benchmark | ns/op | B/op | allocs/op |
|-----------|------:|-----:|----------:|
| `Baseline` (no middleware, NoContent) | 132 | 208 | 4 |
| `StaticRoute_JSON` | 1 033 | 1 412 | 10 |
| `ParamRoute_JSON` | 1 053 | 1 413 | 10 |
| `POST_BindJSON_Response` (JSON bind + response) | 2 559 | 8 067 | 37 |
| `Middleware3_JSON` (RequestID + Recovery + CORS) | 1 024 | 1 526 | 15 |
| `Middleware5_JWT_JSON` (+ JWT + audit stub) | 3 091 | 4 153 | 65 |
| `GroupedAPI` (multi-Group middleware inheritance) | 640 | 1 412 | 10 |
| `Parallel_Static` (GOMAXPROCS concurrent, no MW) | 77 | 208 | 4 |
| `Parallel_JSON_3MW` (GOMAXPROCS concurrent, 3 MW) | 1 053 | 1 528 | 15 |
| `LargeList_JSON` (200 items ~30 KB) | 35 207 | 85 235 | 13 |

---

### Conclusions and Tuning Recommendations

| Conclusion | Tuning Recommendation |
|------------|-----------------------|
| Route lookup 111–282 ns, O(k) regardless of total routes | Don't worry about route count in large apps |
| Each additional middleware ~4 ns, zero extra allocations | Impact is negligible for ≤10 middleware |
| JWT HS256 validation ~2.2 µs, 58 allocs (after Phase 1 optimization) | Remaining allocs are in the library; for very high QPS consider LRU cache or PASETO |
| Reactor concurrent persistent connections ~76k QPS | Short-lived / non-pooled clients significantly reduce throughput |
| Large JSON list (30 KB) 13 allocs | Use pagination to limit list size; consider streaming JSON |

---

### Optimization Techniques Summary

The framework reduced the routing core from **10 allocs/req** to **4 allocs/req**, JSON responses from **17 allocs** to **10 allocs**, and JWT verification from **105 allocs** to **58 allocs** through three rounds of systematic allocation analysis:

| Technique | Allocs Eliminated | File |
|-----------|:-----------------:|------|
| `responseWriter` embedded as value field, interface points to heap pointer | 1 | `context.go` |
| Inline `[8]Param` array, params slice with no heap allocation | 1 | `context.go` |
| `matchRoute` uses `IndexByte` for inline path parsing | 2 | `router.go` |
| `routeKey string` field, eliminates `string→any` boxing | 1/req | `context.go`, `router.go` |
| `[8]kvPair` linear store instead of map | 1~2/req | `context_store.go` |
| Pre-allocated `[]string` singletons for Content-Type | 1/resp | `context_response.go` |
| `goccy/go-json` replacing `encoding/json` | 3~5/JSON | `serializer.go` |
| `RequestID` buffer pool (`[48]byte`) | 1/req | `middleware/requestid.go` |
| Eliminate JWT second `ParseWithClaims` (double HMAC+JSON) | ~44/req | `middleware/jwt.go` |
| `registeredClaimKeys` promoted to package-level variable | 1/req | `middleware/jwt.go` |
| JWT `Extra` map lazy allocation | 1/req | `middleware/jwt.go` |
| `queryCache url.Values` lazy init, `Query()` reuses parsed result | N-1/req (N queries) | `context.go`, `context_request.go` |
| Content-Length 0–1023 pre-built `[]string` cache | 2/JSON resp (< 1 KB) | `context_response.go` |
| `responseWriter.WriteString` + `io.WriteString` | 1/String resp | `response_writer.go`, `context_response.go` |
