# Changelog

All notable changes to **Astra** are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Support policy**:
> - Each minor release (`v1.x.0`) receives patch fixes for **12 months** after the next minor is published.
> - Major releases (`v2.0.0`) receive security patches for **24 months** after the next major is published.
> - See [docs/versioning.md](docs/versioning.md) for the full stability and support matrix.

---

## [2.0.0] — 2026-06-03

### Changed
- **模块结构重组（Breaking Change）**: go.work 核心模块数 47 → 30（-36.2%），达到并超越 ADR-005 目标
  - `mq/*` 7 个子模块合并为统一 `mq/` 模块（build tags + 统一 `Broker` 接口）
  - `discovery/*` 5 个子模块合并为统一 `discovery/` 模块（build tags + 统一 `Registry` 接口）
  - `config/*` 4 个子模块合并为统一 `config/` 模块（统一 `ConfigClient` 接口）
  - `lua/` 合并入 `rule/lua/`（build tag: `lua`）
  - `runner/*` 4 个子包扁平化为 `runner/`（build tags: `cron`, `dagu`, `gocron`, `tqrunner`）
  - `taskqueue/*` 5 个子包扁平化为 `taskqueue/`（build tags: `kafka`, `mongo`, `rabbitmq`, `redis`, `rocketmq`）
  - `notify/*` 3 个子包扁平化为 `notify/`（build tags: `email`, `push`, `sms`）
- **删除的兼容层**: `config/nacos/`, `config/etcd/`, `config/apollo/` 兼容包已移除

### Added
- `scripts/migrate-config.sh` — Config 模块迁移脚本
- `docs/config/migration-guide.md` — Config 模块迁移指南
- `runner/README.md`, `taskqueue/README.md`, `notify/README.md` — 模块文档
- `discovery/README.md` — Discovery 模块文档

### Migration Guide
- 参见 `docs/migration-guide-mq-v2.md`（MQ 迁移）
- 参见 `docs/config/migration-guide.md`（Config 迁移）
- `lua/` 用户: 修改 import 为 `github.com/astra-go/astra/rule/lua`，编译时加 `-tags=lua`

### Planned
- WebAssembly (WASM) compilation target support
- Server-sent events (SSE) built-in middleware

---

## [1.1.0] — Unreleased

### Added
- **`docs/en`: English documentation** — full English translation of all Chinese
  documentation under `docs/en/`, mirroring the existing `docs/` structure:
  - `docs/en/index.md` — framework overview and feature table
  - `docs/en/getting-started/` — installation, three-step quickstart, first app,
    and recommended project structure
  - `docs/en/api/` — Core API (App / Context / Router / error types), built-in
    middleware reference (20+ middleware), and extension modules reference
    (Reactor engine, HTTP/3, OTel, Prometheus, gRPC, circuit breaker, service
    discovery, MQ, cache, ORM, Elasticsearch, Saga, alert engine, OAuth2/OIDC,
    GraphQL, health checks)
  - `docs/en/guides/` — performance tuning guide (benchmark results table,
    Reactor engine tuning, memory allocation tips, connection pool configuration,
    pprof profiling), security hardening guide, and deployment guide
  - `docs/en/migration/` — migration overview, v0.x → v1.0 guide (one-click
    check script + 8 breaking change details), v1.x → v2.0 planned guide
  - `docs/en/versioning.md` — SemVer rules, stability levels, support lifecycle
  - `docs/en/contributing.md` — dev environment setup, commit convention,
    PR process, documentation build

- **`docs` / `examples`: progressive Getting Started overhaul** — addresses the
  onboarding friction caused by DI + Module + Plugin + lifecycle concepts
  stacking on first encounter:
  - `examples/hello/main.go` — 18-line minimal template with zero extra
    concepts; directly comparable to a Gin/Echo hello-world.
  - `examples/quickstart/main.go` — real-service template (~90 lines) covering
    middleware, route groups, request binding + validation, JWT-protected
    sub-group, and lifecycle hooks; no DI/Module/Plugin.
  - `docs/getting-started/quickstart.md` — three-step progressive guide
    (Hello World → real service → large app) with a concept-introduction table
    showing which feature to reach for and when. DI, Module, and Plugin remain
    entirely optional.
  - `astra.go` package doc updated: Quick start reduced to 5 lines, pointers
    added to all three templates.
  - `mkdocs.yml` nav: "快速上手（三步）" inserted as the first landing page
    after Installation.

- **`app`: `NewSlim()` constructor — slim startup mode** — a second constructor
  plugin / lifecycle subsystem and the `go-playground/validator` dependency are
  unwanted. Compared with `New()`:
  - `lifecycle` field is left `nil`; `OnStart` / `OnStop` return `ErrSlimMode`
    instead of registering hooks.
  - `Register` (`module.go`) and `RegisterPlugin` return `ErrSlimMode`.
  - `Options.Binder` is `nil` (`slimDefaultOptions`), so `go-playground/validator`
    (~1.2 MB linked binary) is not imported.
  - The `*App` type is identical to `New()`; all route registration methods,
    middleware, `ServeHTTP`, and graceful shutdown work unchanged. Accepting
    `*astra.App` parameters in middleware or helpers requires no adaptation.
  - `runWithGracefulShutdown` guards lifecycle calls with a nil check so slim
    apps start and shut down cleanly without a lifecycle instance.

  New symbols:
  - `NewSlim(opts ...Option) *App` — slim constructor (`app.go`)
  - `slimDefaultOptions() *Options` — defaults with `Binder: nil` (`options.go`)
  - `ErrSlimMode` — sentinel error returned by disabled operations (`errors.go`)
  - `App.slim bool` — internal flag set only by `NewSlim`

  `OnStart` and `OnStop` signatures changed from `func(…)` (no return) to
  `func(…) error` to surface `ErrSlimMode`; callers that discarded the return
  value are unaffected at compile time.

  Six new tests in `astra_test.go` cover: routing works on a slim app,
  `OnStart` / `OnStop` / `RegisterPlugin` / `Register` each return `ErrSlimMode`,
  and `New()` hooks still register without error.

- **`astractl gen schema`: native OpenAPI 3.1 spec generation from Go types** —
  new `gen schema` sub-command generates a fully compliant OpenAPI 3.1 spec by
  statically analysing Go source files with `go/ast`. No swaggo, no external
  tools required.

  Key capabilities:
  - All exported structs are extracted as `components/schemas`; doc comments
    become `description` fields.
  - `json` tag drives field names and `omitempty` influences the `required`
    array; `validate` tag constraints are mapped to JSON Schema keywords:
    `required` → required array, `min`/`max` → `minimum`/`maximum` (numbers)
    or `minLength`/`maxLength` (strings), `oneof` → `enum`.
  - Rich type mapping: `time.Time` → `date-time`, `uuid.UUID` → `uuid`,
    `sql.Null*` → nullable, `*T` → nullable, named structs → `$ref`.
  - Handler functions annotated with `@router METHOD /path` are emitted as
    `paths` entries. `@param` supports `path`/`query`/`header`/`cookie`/`body`;
    `@success`/`@failure` support swaggo-compatible `{object} TypeName "desc"`
    syntax with automatic `$ref` resolution.
  - JSON (default) or YAML output selected by the `--out` file extension.

  New file: `cmd/astractl/internal/gen/schema/schema.go`

  Usage:
  ```bash
  astractl gen schema --dir ./internal/handler --out api/openapi.json \
      --title "My API" --version 1.0.0
  # YAML output
  astractl gen schema --dir ./internal/handler --out api/openapi.yaml
  ```

### Changed
- **`middleware`: eliminate interface dispatch overhead via `astra.Unwrap`** —
  all built-in middleware now calls `astra.Unwrap(c)` once at handler entry to
  obtain the concrete `*Ctx`, then uses direct method calls for the remainder of
  the request. This eliminates vtable dispatch (~3–5 ns each) on every hot
  method call — `Request()`, `Writer()`, `ClientIP()`, `Header()`, `SetHeader()`,
  `Set()`, `Next()` — without changing any public API or breaking existing tests.

  Changes by file:
  - `context.go` — new exported `Unwrap(c contract.Context) (*Ctx, bool)` helper;
    third-party middleware can call it once at entry to opt in to the same fast path.
  - `middleware/logger.go` — unwrap at closure entry; `Request()` called once and
    cached in `req`, eliminating 3 redundant interface dispatches; `Writer()`,
    `ClientIP()`, `Next()` switched to direct calls (saves ~6 vtable dispatches/request).
  - `middleware/cors.go` — unwrap at closure entry; `Header()`, `Request()`,
    `Writer()`, `AbortWithStatus()` switched to direct calls on both the preflight
    and actual-request paths.
  - `middleware/requestid.go` — fast path uses `ctx.Header`, `ctx.SetHeader`,
    `ctx.Set` directly; interface fallback preserved for test mocks.
  - `middleware/jwt.go` — unwrap at closure entry; `extractTokenDirect` added as
    a concrete-type variant of `extractToken`; `Next()` switched to direct call.
  - `middleware/ratelimit.go` — default `KeyFunc` and the rate-exceeded path both
    use `Unwrap` to eliminate `ClientIP()` and `Writer().Header()` vtable dispatch.

  A fallback branch (`if !ok`) is present in every closure; it takes the original
  interface path and is the only branch exercised by unit tests that inject mocks.
  Production traffic always takes the fast path because `context_flow.go` passes
  `*Ctx` as `contract.Context` to every handler.

- **`astractl`: modularize CLI into `internal/` sub-packages** — the 2752-line
  monolithic `cmd/astractl/main.go` has been split into 22 modular files under
  `cmd/astractl/internal/`:

  - **panic-on-error eliminated**: 19 `template.Must()` calls at package init
    replaced with `sync.Once` lazy initialization in `internal/tmpl/templates.go`
    (template parse errors are now surfaced as runtime errors, not startup
    panics). All 49 `fatalf()` / `os.Exit(1)` call sites across gen commands
    replaced with proper `error` returns — every `gen` sub-command now returns
    `error` and lets `main()` handle termination.

  - **Friendly three-section error format**: new `internal/cli.CLIError` type
    (`Msg`, `Hint`, `Example` fields + `Render()`) produces structured output:
    ```
    [error] read nonexistent.proto: open nonexistent.proto: no such file or directory
      hint:    ensure the .proto file path is correct and readable
      example: astractl gen proto api/service.proto
    ```

  - **New `astractl doctor` command**: 6 pre-flight environment checks —
    go module (go.mod readable, module name extractable), project layout
    (simple / ddd / unknown, lists missing dirs), DI scan readiness
    (grep for `di.Provide*` calls), proto files (`*.proto` in working dir),
    OpenAPI files (`openapi.yaml` / `swagger.yaml`), and writable directory
    (write probe). Each check reports ✓ (pass), ! (warning), or ✗ (fail) with
    an optional hint line. Implemented in `internal/doctor/doctor.go`
    (`RunDoctor()`, `Print()`, `HasFailures()`).

  All existing flags (`--dir`, `--pkg`, `--force`, `--scan`, `--grpc`,
  `--contract`, `--impl`) and all generated file content are **fully backward
  compatible**; no user-facing change to any existing command.

- **`context_response`: `jsonBufPool` — oversized buffer eviction + `JSONStream` routing** —
  `jsonBufPool` (`sync.Pool` of `*bytes.Buffer`) previously returned every buffer
  to the pool unconditionally. A large-payload response caused the buffer to grow
  its backing array; the expanded buffer was then retained in the pool, inflating
  RSS under mixed small/large traffic.

  Two complementary fixes:

  1. **Cap-and-discard** (`context_response.go`): after each `JSON()` call the
     deferred cleanup now checks `buf.Cap()`. Buffers exceeding `jsonBufMaxCap`
     (64 KB) are dropped instead of returned — their backing arrays are eligible
     for GC immediately. Responses within the threshold continue to use the pool
     with zero overhead change.
     ```go
     const jsonBufMaxCap = 64 * 1024
     defer func() {
         if buf.Cap() <= jsonBufMaxCap {
             jsonBufPool.Put(buf)
         }
     }()
     ```

  2. **Routing guidance** (`context_response.go` doc): `JSONStream` doc now
     explicitly states "use for responses that may exceed 64 KB", aligning the
     architectural boundary with the pool eviction threshold. Large list endpoints
     should call `c.JSONStream` to bypass the pool entirely (no `Content-Length`,
     but zero intermediate buffer allocation).

  New `export_test.go` exports (`JsonBufPool`, `JsonBufMaxCap`) and
  `jsonbufpool_test.go` white-box tests verify: oversized buffers are not
  returned to the pool after a request, and normal-sized buffers continue to be
  reused correctly.

- **`context`: KV Store — `map+mutex` → `[]kvPair` dynamic slice, lock removed** —
  `c.Set` / `c.Get` previously acquired `keysMu sync.RWMutex` on every call,
  including the inline `smallKeys` path that never touches the overflow map.
  In the standard sequential middleware chain (single goroutine per request) this
  was pure overhead with no protection value (~10–30 ns of atomic CAS + memory
  fence per op). Root cause: two stacked defects:

  1. **Wrong lock granularity**: the mutex wrapped the entire `Set`/`Get` body
     even when the inline array was sufficient. 10-middleware chains × 2 ops each
     = 20 uncontested atomic operations per request.
  2. **`smallKeysCap = 8` too low**: a typical chain (RequestID + Logger + JWT +
     RateLimit + Tracing + RBAC + Tenant) performs ≥9 `c.Set` calls, triggering
     `make(map[string]any)` heap allocation + map hashing overhead + O(N)
     `delete` loop in `reset()`.

  Three options were evaluated (split-level lock / larger cap / slice):
  the slice approach (Option C) was chosen as the only design without
  structural debt — no mutex, no map, no magic cap constant to tune.

  Changes:
  - `context.go`: removed `smallKeysCap`, `smallKeys [8]kvPair`, `smallLen int8`,
    `keys map[string]any`, `keysMu sync.RWMutex` (~296 B saved per pooled `*Ctx`;
    24 B RWMutex + 256 B inline array). Added `kvStore []kvPair` (nil until first
    `Set`; `reset()` uses `[:0]` to retain the backing array across requests).
    Removed `"sync"` import.
  - `context_store.go`: complete rewrite. `Set` scans `kvStore` for in-place
    update then `append`s; `Get` is a plain linear scan. Zero mutex operations
    on any path. `contract.RouteKey` fast-path unchanged.
  - `reset()`: replaced `smallKeys` zeroing + `delete(c.keys)` loop with a
    single `for i := range kv { kv[i] = kvPair{} }` + `c.kvStore = kv[:0]`.

  Concurrency contract (same as Gin / Echo): `c.Set` / `c.Get` are not
  goroutine-safe. Copy values into local variables before launching goroutines
  from a handler.

  Benchmark results (Apple M4 · Go 1.25):

  | Scenario | Before | After |
  |---|---|---|
  | Per `Set`/`Get` (inline path) | ~10–30 ns mutex overhead | **0 ns** |
  | 12-key request (old overflow) | +1 alloc (map) + O(N) delete | **208 B / 4 allocs** |
  | 4-key request | 208 B / 4 allocs | **208 B / 4 allocs** (unchanged) |
  | `Ctx` struct size | +280 B (RWMutex + inline array) | **−280 B** |

  12-key and 4-key requests now produce identical allocation profiles; all 4
  allocs come from `httptest.NewRecorder()` in benchmarks, not from the framework.

- **`app`: `sealPool()` — Pool pre-warming on startup** — `sealPool()` (called
  once inside `runWithGracefulShutdown` before the server begins accepting
  requests) now pre-warms the `sync.Pool` by allocating `runtime.GOMAXPROCS(0)`
  `*Ctx` objects and immediately returning them to the pool. This ensures each
  P's local slot is populated before the first request wave, eliminating the
  cold-start burst of `pool.New` calls that previously caused a brief GC spike
  under high initial concurrency. Applications with low concurrency or gradual
  ramp-up are unaffected.

- **`benchmarks`: sync.Pool contention analysis — benchmark corrected** —
  root-cause investigation of the reported "Pool contention at >= GOMAXPROCS×2"
  revealed the issue was a benchmark artefact, not a framework defect:
  - The original `BenchmarkIntegration_Parallel_Static` and
    `BenchmarkServeHTTP_Parallel_Static` call `httptest.NewRecorder()` inside
    the hot loop (208 B / 4 allocs/op per iteration). Under cpu=8, concurrent
    allocation from 8 goroutines contends on `mcentral`, raising per-op latency
    independent of `sync.Pool`.
  - The additional cpu=4→cpu=8 latency regression (52→65 ns/op) is an Apple M4
    asymmetric-core effect (4 efficiency cores + 6 performance cores): the extra
    4 goroutines are scheduled on efficiency cores (~3× slower IPC).
  - When isolated (shared `ResponseRecorder`, 0 allocs), the Pool scales
    linearly: cpu=1 → 23 ns/op, cpu=4 → 6.8 ns/op, cpu=8 → 5.0 ns/op (4.6×
    speedup, no plateau).
  - `BenchmarkIntegration_Parallel_Static` comment updated to document the
    `NewRecorder` allocation source.
  - New `BenchmarkIntegration_Parallel_Static_WarmPool` added as the canonical
    Pool-isolation benchmark (0 B / 0 allocs, linear scaling to cpu=8).
  - `BenchmarkServeHTTP_Parallel_Static` comment updated with the same
    clarification.

- **`router`: 静态子节点首字节分发表（`childIndex`）— O(n) → O(1) 静态查找** —
  `matchSegments` 中同级静态子节点的匹配原先采用 `children []*node` 线性扫描，
  当某层存在大量兄弟节点时（如 REST API 顶层：`/users /orders /products /settings /webhooks …`）
  最坏情况为 O(n)，`BenchmarkRouter_Static_100` 可观测到退化（命中最后一个兄弟节点 `/route/99`
  约 276 ns/op，是单路由的 2.6×）。

  **方案（首字节分发表，Scheme B）**：

  ```
  node.childIndex *[256]int16
    -1  → 该首字节无子节点（absent）
    -2  → 多个子节点共享该首字节（collision，回退线性扫描）
    ≥ 0 → children[idx] 直接命中
  ```

  - `node` 新增 `childIndex *[256]int16`（指针延迟分配，叶节点零开销；`[256]int16` = 512 B，
    仅在有静态子节点时分配一次）
  - `newChildIndex()` 初始化全 `-1`（absent）的 256 项数组
  - `recordChildIndex()` 在 `insertNode` 追加子节点后更新：首次写入存索引，
    同首字节再次写入标 `collision`（`-2`）
  - `matchSegments` 静态扫描改为三路分支：
    - `idx ≥ 0` → 直接命中 `children[idx]`，一次字符串比较即可确认或排除
    - `collision` → 退化线性扫描（同首字节节点极少见于真实 REST API）
    - `absent` → 跳过，无需任何扫描

  新增 `BenchmarkRouter_Static_REST`：注册 25 个首字母各异的资源路由
  （`/users /orders /products /auth /settings /metrics /health /webhooks …`），
  命中 `/webhooks`；优化后耗时 **~107 ns/op = 单路由基线**，证明 O(1) 分发已生效。

  基准对比（Apple M4 · Go 1.25 · 3 轮 × 1s）：

  | 基准 | 优化前 ns/op | 优化后 ns/op | 变化 |
  |---|---:|---:|---:|
  | `Router_Static`（单路由） | 106 | 108 | 持平 |
  | `Router_Static_REST`（25 资源，新增） | — | **107** | O(1) |
  | `Router_Static_100`（100 路由，数字后缀） | 276 | 281 | 持平* |
  | `Router_NotFound` | 384 | 384 | 持平 |

  \* `Router_Static_100` 注册 `/route/0`–`/route/99`，十进制后缀仅有 10 种首字节（`'0'`–`'9'`），
  大量节点共享同一首字节（`collision`），回退线性扫描，此为数字命名的人造极端场景。
  真实 REST API 各资源段首字母通常互不相同，效果等同 O(1)。

- **`router`: regex route fast-path matchers** — 14 well-known patterns
  (`[0-9]+`, `\d+`, `[a-zA-Z0-9]+`, `[a-zA-Z0-9_-]+`, etc.) now bypass the
  regexp engine entirely at match time; a direct byte-scan (`fastMatcher`) is
  stored on the tree node at registration and called instead of
  `regexp.MatchString`. Parallel benchmark: `BenchmarkRouter_Regex_FastPath_Parallel`
  ~70 ns/op vs ~77 ns/op for a custom pattern through the regexp engine.
- **`router`: global regexp cache** (`getOrCompileRegexp` / `regexpCache sync.Map`) —
  identical patterns registered across different route prefixes now share a
  single `*regexp.Regexp` instance, eliminating redundant compilation and pool
  fragmentation under concurrent load. `findRegexChild` updated to use pointer
  equality instead of string comparison.
- **`router`: `allowedMethods` replaces `methodNotAllowed` — RFC 9110 §15.5.6
  compliance** — the old `methodNotAllowed(path string) bool` helper traversed
  all method trees to detect a 405 situation but never set the `Allow` response
  header, violating RFC 9110 §15.5.6 ("MUST generate an Allow header field").
  Replaced with `allowedMethods(path string) string` that returns a
  comma-separated list of matched methods (or `""` for a true 404) and is
  called once per 405 request:
  - `Router.methodRoots []methodRoot` — an ordered `[]methodRoot` slice
    maintained by `Add()` using insertion sort keyed on `methodOrderRank()`.
    Produces a stable, deterministic traversal order
    (`GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS`) regardless of registration
    order or map hash seed. Prior code used `map[string]*node` iteration
    (non-deterministic Allow value across runs).
  - `allowedMethods` uses a stack-allocated `[128]byte` buffer to build the
    comma-separated value with zero heap allocations on the hot path.
  - `Handle()` now sets `c.rw.ResponseWriter.Header().Set("Allow", allow)`
    before delegating to `methodNotAllowedChain`, satisfying the RFC.
  - `maxParamDepth()` updated to iterate `r.methodRoots` instead of `r.trees`.
  - Benchmark before/after: the old early-exit path measured ~100 ns regardless
    of method count (short-circuiting after the first match — incorrect for RFC
    compliance). The new full-traversal path: 1-method ~115 ns, 5-method
    ~270 ns (~2.7× slower for 5 trees). The regression is the **necessary cost
    of correctness**: RFC 9110 §15.5.6 requires enumerating all matched methods;
    early-exit was a false optimization that violated the spec. The 405 path is
    an error path; 270 ns absolute is fully acceptable, and the 200/404 hot
    path is entirely unaffected.
  - Three new tests: `TestMethodNotAllowed_AllowHeader` (correct methods),
    `TestMethodNotAllowed_AllowHeaderOrder` (stable order), and
    `TestNotFound_NoAllowHeader` (404 has no Allow field).

- **`router` / `context` / `app`: deep-param route zero-allocation** — routes
  with more than `maxRouteParams` (8) path parameters previously triggered a
  512 B heap allocation and +47% latency per request when the 9th `append` in
  `matchSegments` exceeded the inline `paramsArr` capacity. Fixed with three
  coordinated changes:
  - `router.go`: `maxParamDepth()` + `nodeParamDepth()` traverse all method
    trees at startup to compute the actual maximum param depth.
  - `context.go`: `overflowParams Params` field added to `Ctx`; `reset()` uses
    `overflowParams[:0]` as the backing slice when non-nil, falling back to the
    inline `paramsArr[:0]` for normal routes — zero-allocation in both cases.
  - `app.go`: `sealPool()` called inside `runWithGracefulShutdown` (after all
    routes are registered, before the first request). When `depth > maxRouteParams`
    it re-wires `pool.New` to pre-allocate `overflowParams` with `cap=depth`.
    Applications with ≤ 8 params per route are entirely unaffected.

  Benchmark results (Apple M4 · Go 1.25 · `count=4`):

  | Benchmark | ns/op | B/op | allocs/op |
  |---|---|---|---|
  | `DeepParam_8_NoSeal` (baseline) | 227 | 208 | 4 |
  | `DeepParam_9_NoSeal` (before fix) | 344 | **720** | **5** |
  | `DeepParam_9_Sealed` (after fix) | **243** | **208** | **4** |
  | `DeepParam_12_Sealed` (after fix) | 307 | 208 | 4 |

  9-param routes: −29% ns/op, −512 B/op, −1 alloc/op.

- **`astractl/gen/wire`: four AST scan improvements** — `gen wire --scan` 的
  四项关键能力补全，消除 Beta 阶段遗留的已知限制：

  1. **`--recursive` 深度递归**（`scan.go`）：原实现使用 `os.ReadDir` 仅扫描一层
     直接子目录；改为 `filepath.WalkDir` 实现真正的任意深度递归，跳过以 `.` 开头
     的隐藏目录。

  2. **多类型参数泛型支持**（`scan.go` `exprToString`）：原实现对 `ast.IndexExpr`
     （单类型参数）已处理，但缺少 `ast.IndexListExpr` 分支；补全后可正确解析
     `Map[K, V]`、`Result[T, E]` 等多类型参数泛型表达式，各参数间以 `, ` 连接。

  3. **Import 路径自动收集**（`scan.go` `collectImports` + `ScanResult.Imports`）：
     新增 `collectImports()` 函数，遍历每个扫描文件的 `ast.ImportSpec`，以本地别名
     （或路径末段）为 key、完整 import path 为 value 写入 `ScanResult.Imports`，跳过
     `_` 和 `.` 导入。`BuildDIGen` 将收集到的 imports 与框架固定 imports 合并后写入
     生成文件，消除跨包类型名称歧义；import 块按别名字母序排列保证确定性输出。

  4. **多包聚合模式**（`wire.go` + `build.go`）：两个新 flag：
     - `--export-func NAME`：生成 `func NAME(c *di.Container)` 而非 `initDI`，
       不创建容器、不调用 `BindApp`，适合子包注册入口。
     - `--aggregate`：多包聚合——读取 `go.mod` 确定模块路径（向上遍历至根），
       遍历 `--dir` 下每个直接非隐藏子目录独立扫描并写 `di_gen.go`（`RegisterDI`
       模式），再在根目录写聚合 `di_gen.go`（`initDI` 按字母顺序调用所有子包
       `RegisterDI`）。

     新增 `SubPkg` 结构体（`Alias / ImportPath / FuncName`）和 `BuildRootDIGen`
     函数（`build.go`）；`BuildDIGen` 新增 `exportFunc string` 参数。`Run` 拆分为
     `runSingle` / `runAggregate` 两条路径，`readModulePath` 向上遍历父目录查找
     `go.mod` 并解析 `module` 指令。

  典型用法：
  ```bash
  # 多包项目一键生成
  astractl gen wire --scan --aggregate --dir ./internal --force

  # 单独为子包生成可被聚合的 RegisterDI
  astractl gen wire --scan --export-func RegisterDI --dir ./internal/user --force
  ```

  所有已有 flag（`--scan`、`--dir`、`--pkg`、`--force`、`--provider-funcs`、
  `--recursive`）行为与生成内容完全向后兼容。

### Testing
- **`orm/clickhouse`: testcontainers integration tests** — replaced the
  previous env-var / manual-container approach with
  `testcontainers-go/modules/clickhouse v0.42.0`. `TestMain` starts a
  `clickhouse/clickhouse-server:24-alpine` container once per binary and tears
  it down after all tests complete. No external container or environment
  variable is required; run with:
  ```
  go test -tags integration -v ./orm/clickhouse/...
  ```
  Seven test cases covering: connectivity (`Open` + `Ping`), DDL + CRUD,
  batch insert (100 rows), raw SQL with parameters, connection-pool settings
  (`MaxOpenConns`), idempotent `CREATE TABLE IF NOT EXISTS` (3×), and empty
  table query. Dependencies added to `orm/go.mod`:
  `github.com/testcontainers/testcontainers-go v0.42.0` and
  `github.com/testcontainers/testcontainers-go/modules/clickhouse v0.42.0`.

- **`search/elastic`: testcontainers integration tests** — replaced the
  previous env-var / manual-container approach with
  `testcontainers-go/modules/elasticsearch v0.42.0`. `TestMain` starts a
  `docker.elastic.co/elasticsearch/elasticsearch:8.13.0` container (security
  enabled via `estc.WithPassword`) once per binary. TLS CA cert is read from
  `ctr.Settings.CACert` and passed to the client. Run with:
  ```
  go test -tags integration -v ./search/elastic/...
  ```
  Ten test cases covering: index create/delete lifecycle, mapping with keyword
  field (duplicate-index error path), index + term search round-trip, document
  overwrite (upsert semantics), bulk index (10 docs), delete, delete of
  non-existent document (silent 404), pagination with disjoint-page assertion,
  terms aggregation bucket count, and `_source` field filtering. Dependencies
  added to `search/go.mod`:
  `github.com/testcontainers/testcontainers-go v0.42.0` and
  `github.com/testcontainers/testcontainers-go/modules/elasticsearch v0.42.0`.

### CI
- **Benchmark regression gate** (`bench` job in `.github/workflows/ci.yml`) —
  `main` push saves `bench-main.txt` as the baseline; PR runs compare via
  `benchstat` and block merge on statistically significant regressions ≥ +10%.
  Three defects in the original implementation are fixed:
  - `cache/save` `path` was `bench-current.txt` while `cache/restore` expected
    `bench-main.txt`; unified to `bench-main.txt` so the baseline is actually
    retrieved on PRs.
  - `./...` expanded into modules that require external services (`orm/`,
    `search/`, `mq/`), causing the bench job to fail; scope narrowed to the
    four self-contained suites: `.`, `./netengine/`, `./middleware/`,
    `./benchmarks/`.
  - Statistically insignificant rows (`~`) in `benchstat` output are now
    filtered before the regression check to avoid false positives.

---

## [1.0.0] — 2026-04-20

**Production-ready stable release.** All public APIs are now covered by the
Go 1 compatibility promise within each major version. No breaking changes will
be introduced in `v1.x` patch or minor releases.

### Highlights
- Reactor network engine stabilised (`netengine`)
- Full OAuth2 / OIDC client (`auth/oauth2`)
- HTTP/3 (QUIC) support (`RunQUIC`)
- GraphQL mount helper (`graphql`)
- Apache Pulsar MQ backend (`mq/pulsar`)
- Apollo config source (`config/apollo`)
- Kubernetes service discovery (`discovery/k8s`)
- ClickHouse GORM driver (`orm/clickhouse`)
- Elasticsearch / OpenSearch client (`search/elastic`)
- Saga distributed transaction (`dtx`)
- Canary (grey-release) middleware (`middleware.Canary`)
- Istio / service-mesh health probes (`health.RegisterIstioProbes`)
- Alert rule engine (`alert`)

### Security
- `netengine`: `close()` now calls `wakeup()` before `poller.close()` to prevent
  EBADF spurious ERROR log on clean shutdown.
- `netengine`: orphaned connections queued in `addCh` are drained and closed
  during shutdown, preventing `activeConns` from leaking.
- `netengine`: idle connections' file descriptors are explicitly removed from
  the poller before closing during shutdown.
- `middleware/ratelimit`: cleanup goroutine now exits when the context is
  cancelled. `NewRateLimiter` returns a `stop()` function for controlled teardown.
- `router`: duplicate method+path registration now emits an `slog.Warn` log
  instead of silently overwriting the earlier handler.
- `binding`: slice field population is limited to `MaxSliceParams = 1000`
  elements to prevent DoS via large query-string arrays.
- `context`: `ClientIP()` performs right-to-left XFF traversal past trusted
  proxies to prevent IP spoofing.
- `context`: CIDR trusted-proxy list is pre-compiled at startup.
- JWT middleware: `Leeway` sentinel value `StrictJWTLeeway = -1ns` allows
  disabling clock skew tolerance explicitly.
- `gzip`: `WriteHeader` is buffered so `Content-Encoding: gzip` is always set
  before the header frame is committed.

### Fixed
- `health/istio`: `x-envoy-upstream-service-time` header moved before
  `next(c)` so it is included in responses over real HTTP connections.

---

## [0.10.0] — 2026-03-15

### Added
- **`auth/oauth2`** — OAuth2 / OIDC authorization-code flow with PKCE, secure
  cookie state store, `LoginHandler`, `CallbackHandler`, `RefreshToken`,
  `FetchUserInfo`.
- **`graphql`** — `Mount(app, handler, opts...)` helper; optional GraphQL
  Playground HTML page served from an embedded template.
- **`app_quic.go`** — `App.RunQUIC(addr, cert, key)` starts HTTP/3 over QUIC
  and simultaneously serves HTTP/1.1+2 with the `Alt-Svc` upgrade header.
- **`mq/pulsar`** — Apache Pulsar producer / consumer, satisfies `mq.Producer`
  / `mq.Consumer` interfaces.
- **`config/apollo`** — Apollo configuration centre source; implements
  `config.Source` + `config.Watchable`.
- **`discovery/k8s`** — Kubernetes Endpoints-based service discovery using
  `k8s.io/client-go`; supports in-cluster and kubeconfig modes.
- **`orm/clickhouse`** — ClickHouse GORM driver wrapper with connection pool
  configuration.
- **`search/elastic`** — Elasticsearch / OpenSearch client: `Index`,
  `BulkIndex`, `Search`, `Delete`, `DeleteIndex`, `CreateIndex`.
- **`dtx`** — `Saga` distributed transaction: sequential forward execution,
  reverse compensation on failure, `CompensationErrors` collection.
- **`middleware.Canary`** — grey-release traffic colouring by header regex,
  cookie regex, or hash-based user-ID modulo routing.
- **`health.RegisterIstioProbes`** — registers `/healthz/live` and
  `/healthz/ready` for Istio alongside the standard `/live` / `/ready` endpoints.
- **`alert`** — expression-driven alert rule engine; `WebhookChannel` (HTTP POST
  JSON), `LogChannel` (slog); `For` duration semantics; `ActiveAlerts()`.

---

## [0.9.0] — 2026-02-01

### Added
- **Reactor network engine** (`netengine`) — epoll (Linux) / kqueue (macOS)
  event-driven server. `O(1)` goroutines for idle connections; configurable
  `NumLoops`, `WorkerPoolSize`, `ConnChannelBuffer`; `ListenReusePort`,
  `Listen` with `SO_REUSEPORT` + `TCP_FASTOPEN` options; `ActiveConns()` gauge.
- **`App.RunReactor(addr)`** — convenience method to start the Reactor engine.
- **`middleware.SecureHeaders`** — `X-Content-Type-Options`, `X-Frame-Options`,
  `X-XSS-Protection`, `Strict-Transport-Security`, `Referrer-Policy`.
- **`middleware.CSRF`** — double-submit cookie + SameSite token validation.
- **`middleware.Signature`** — HMAC-SHA256 request signature verification.
- **`middleware.CSP`** — `Content-Security-Policy` builder.
- **`middleware.IPFilter`** — IP allow/block list with CIDR support.
- **`middleware.LongPoll`** — channel-based long-poll subscription.

### Changed
- `context.ClientIP()` now traverses `X-Forwarded-For` right-to-left past
  pre-compiled CIDR trusted-proxy ranges. **Requires re-configuration if you
  set `TrustedProxies` by string — they are now compiled at `New()` time.**
- `middleware.JWT` accepts `StrictJWTLeeway = -1ns` to disable leeway explicitly.

### Fixed
- gzip `WriteHeader` race: `Content-Encoding` header is now set before the
  status code is committed to the underlying `ResponseWriter`.

---

## [0.8.0] — 2025-12-10

### Added
- **`runner`** — unified task runner: `HTTP`, `GRPC`, `Cron`, `Worker` backends;
  parallel start with aggregated lifecycle; optional `slog.Logger`.
- **`middleware.Audit`** — structured audit log with user, action, resource, IP,
  latency, and outcome fields.
- **`middleware.Tenant`** — multi-tenant isolation; per-tenant rate limits,
  allowed origin lists, and data-scope tagging.
- **`middleware.Pprof`** — exposes `net/http/pprof` endpoints under a
  configurable prefix (default `/debug/pprof`).
- **`di`** — generic dependency-injection container: `di.Register[T]`,
  `di.MustResolve[T]`, scoped and singleton lifetimes, cycle detection.
- **`App.RegisterPlugin`** — plugin system: `Plugin.Install(app)` called during
  `App.Run`; allows third-party packages to self-register routes and middleware.

### Changed
- `App.OnStart` / `App.OnStop` hooks now accept `context.Context` and run
  serially in registration order. **If you relied on parallel hook execution
  (a bug in v0.7.x), wrap concurrent work in a single hook.**

---

## [0.7.0] — 2025-10-05

### Added
- **`discovery`** — `Registry` interface; Nacos, Consul, Etcd backends.
- **`loadbalancer`** — `RoundRobin`, `WeightedRoundRobin`, `LeastConn`,
  `ConsistentHash`, `Random` strategies.
- **`retry`** — configurable retry with jitter, exponential back-off, custom
  `ShouldRetry` predicate.
- **`circuit`** — `ConsecutiveFailures` and `AdaptiveFailureRate` circuit-breaker
  strategies; `OnStateChange` callback.
- **`client`** — fluent HTTP client with middleware chain, circuit breaker,
  retry, and timeout.
- **`middleware.APIKey`** — API-key authentication with rotating keys.

---

## [0.6.0] — 2025-08-20

### Added
- **`lock`** — distributed lock: Redis, Etcd, in-memory backends.
- **`session`** — pluggable session store: Redis, in-memory.
- **`storage`** — object storage: S3, GCS, Azure Blob, MinIO, local disk.
- **`migrate`** — declarative schema migration; `Up`, `Down`, `Status`.
- **`middleware.RateLimit`** — token-bucket rate limiter per client IP; configurable
  cleanup goroutine via `Context` field.

### Changed
- `App.New` options pattern introduced; `Options.Logger`, `Options.TrustedProxies`,
  `Options.MaxMultipartMemory` replace top-level setter methods.
  **Old setter methods removed — see migration guide.**

---

## [0.5.0] — 2025-07-01

### Added
- **`mq`** — message queue abstraction (`Producer`, `Consumer`); backends:
  NATS, Kafka, RabbitMQ, Redis Streams, AWS SQS.
- **`cache`** — LRU + TTL in-process cache; Redis-backed distributed cache.
- **`cron`** — cron scheduler wrapping `robfig/cron/v3` with structured logging.
- **`health`** — `/live`, `/ready`, `/startup` probes; configurable check
  functions; aggregated JSON output.

---

## [0.4.0] — 2025-05-15

### Added
- **`grpc`** — dual HTTP+gRPC stack; Kratos structured error codes;
  OTel tracing interceptor; `GRPCMiddlewareFunc` abstraction.
- **`otel`** — OpenTelemetry traces, metrics, and logs; `TraceID()` / `SpanID()`
  on `Context`; automatic span creation per HTTP request.
- **`orm`** — GORM wrapper with `WithContext`, connection pool helpers,
  `SoftDelete`, query tracing.
- **`redis`** — `go-redis/v9` wrapper with pipeline, pub/sub, `SCAN` iterator.
- **`mongo`** — `mongo-driver/v2` wrapper with typed collection helper.

---

## [0.3.0] — 2025-03-10

### Added
- **`binding`** — unified `ShouldBind` / `MustBind`; sources: JSON, XML, YAML,
  TOML, form, query, path, header, cookie; `go-playground/validator/v10`.
- **`validate`** — standalone validator with custom tag registration.
- **`config`** — `Source` interface; file (JSON/YAML/TOML/ENV), `.env`, Nacos,
  Etcd backends; hot-reload via `Watchable`; `config.Scan` into structs.
- **`log`** — `slog`-backed structured logger; `log.Ctx(ctx)` to extract
  request-scoped logger; `WithFields` / `With` chaining.
- **`middleware.RequestID`** — `X-Request-ID` propagation.
- **`middleware.Timeout`** — per-handler deadline enforcement.

### Changed
- `Context.JSON`, `Context.XML` now accept a status code as the first argument.
  **`c.JSON(v)` is removed — use `c.JSON(200, v)`.**

### Deprecated
- `App.SetLogger(l)` — use `astra.New(astra.WithLogger(l))` instead (removed in v0.6.0).

---

## [0.2.0] — 2025-01-20

### Added
- Middleware chain: `Use(mw ...)`, group-level middleware.
- Built-in middleware: `Logger`, `Recovery`, `CORS`, `Gzip`.
- `Context.Set` / `Context.Get` typed key-value store.
- `App.Group(prefix, mw...)` route groups.
- `App.Static(prefix, dir)` static file serving.
- `App.Any(path, handlers...)` for all HTTP methods.
- `Context.Redirect(code, url)`.
- `HTTPError` and `NewHTTPError` for structured error responses.

### Changed
- Handler signature changed from `func(w http.ResponseWriter, r *http.Request)`
  to `func(c *astra.Context) error`.
  **All `http.HandlerFunc` adapters must be wrapped with `astra.WrapH`.**

---

## [0.1.0] — 2024-11-05

### Added
- Radix-tree router: static, param (`:id`), wildcard (`*path`), regex segments.
- HTTP methods: `GET`, `POST`, `PUT`, `DELETE`, `PATCH`, `HEAD`, `OPTIONS`.
- `Context` wrapping `http.ResponseWriter` + `*http.Request`.
- `Context.Param`, `Context.Query`, `Context.Header` accessors.
- `Context.JSON`, `Context.String`, `Context.HTML`, `Context.NoContent` renderers.
- Graceful shutdown on `SIGINT` / `SIGTERM`.
- `App.Run(addr)`, `App.RunTLS(addr, cert, key)`.

---

[Unreleased]: https://github.com/astra-go/astra/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/astra-go/astra/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/astra-go/astra/compare/v0.10.0...v1.0.0
[0.10.0]: https://github.com/astra-go/astra/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/astra-go/astra/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/astra-go/astra/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/astra-go/astra/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/astra-go/astra/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/astra-go/astra/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/astra-go/astra/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/astra-go/astra/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/astra-go/astra/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/astra-go/astra/releases/tag/v0.1.0
