# Astra v1.1.0 Release Notes

> **发布状态**：Unreleased — 目标 v1.1.0
> **Go 最低版本**：1.21
> **向后兼容性**：除 `OnStart` / `OnStop` 签名变更外完全兼容（详见迁移说明）

---

## 概览

v1.1.0 围绕三个主题展开：**降低上手门槛**、**性能与正确性修复**、**测试基础设施补全**。

| 主题 | 交付内容 |
|------|---------|
| 上手体验 | 渐进式 Getting Started 文档 + 零依赖最小模板 + Slim 构造函数 |
| 路由正确性 | RFC 9110 §15.5.6 Allow 头合规（405 响应） |
| 路由性能 | 深参数路由零分配、Regex 快速路径、全局 Regexp 缓存 |
| 运行时性能 | sync.Pool 冷启动预热、Pool 争用根因分析 |
| 工程质量 | benchmark CI 门禁、ClickHouse / Elasticsearch / Pulsar / Apollo 集成测试 |

---

## 新增功能

### 渐进式 Getting Started 文档

DI + Module + Plugin + Lifecycle 概念层叠导致新手上手成本高于 Gin/Echo。v1.1.0 补充三档渐进式材料：

| 文件 | 定位 | 概念数 |
|------|------|:------:|
| `examples/hello/main.go` | 18 行最小模板，直接对标 Gin hello-world | 0 |
| `examples/quickstart/main.go` | ~90 行真实服务：中间件、路由组、请求绑定、JWT 保护、Lifecycle | 5 |
| `docs/getting-started/quickstart.md` | 三步渐进指南 + 概念引入时机表 | �� |

`astra.go` 包文档精简为 5 行 Quick Start，附指向三份模板的链接。`mkdocs.yml` 首页导航调整为"快速上手（三步）"。

DI、Module、Plugin 仍完全可选，不再出现在最小路径上。

---

### `NewSlim()` — Slim 启动模式

`astra.New()` 无论使用场景如何，都会引入完整的 Lifecycle、Module/Plugin 注册表和 `go-playground/validator`（约 1.2 MB 编译体积）。对 Serverless / FaaS / 极简微服务场景不友好。

```go
// 零 DI / 零 Plugin / 零 validator，路由 + Pool 全功能
app := astra.NewSlim()
app.GET("/health", func(c astra.Context) error {
    return c.JSON(200, map[string]string{"status": "ok"})
})
app.Run(":8080")
```

`*App` 类型与 `New()` 完全相同，中间件、路由组、`ServeHTTP`、优雅关闭行为不变。被禁用的功能调用时快速返回哨兵错误：

```go
if err := app.OnStart(hook); errors.Is(err, astra.ErrSlimMode) {
    // 需要 lifecycle 时切换为 astra.New()
}
```

| 能力 | `New()` | `NewSlim()` |
|------|:-------:|:-----------:|
| 路由 / 中间件 / ServeHTTP | ✅ | ✅ |
| 优雅关闭 | ✅ | ✅ |
| Lifecycle hooks（OnStart/OnStop）| ✅ | ❌ `ErrSlimMode` |
| Module / Plugin 注册 | ✅ | ❌ `ErrSlimMode` |
| `c.Bind` / `c.ShouldBind` | ✅ | ❌ Binder nil |
| `go-playground/validator` 链入 | 是 | **否** |

**新增符号**：`NewSlim(opts ...Option) *App`、`ErrSlimMode`

---

## 路由层改进

### RFC 9110 §15.5.6 Allow 头合规（405 修复）

**问题**：原 `methodNotAllowed(path) bool` 存在两层缺陷叠加：

- **正确性缺陷**：RFC 9110 §15.5.6 要求 405 响应必须携带 `Allow` 头，列出该路径支持的所有方法。原实现直接返回 `bool`，响应不含 `Allow` 头——RFC 违规。
- **虚假优化**：底层用 `map[string]*node` 随机迭代且早退（找到第一个匹配即返回）。正确实现必须遍历所有树以收集完整 Allow 列表，早退在语义上是错误的。

**修复**：替换为 `allowedMethods(path) string`，一次全量遍历同时完成判断 + 收集 + 构建：

```go
// router.go
func (r *Router) allowedMethods(path string) string {
    var buf [128]byte  // 栈分配，覆盖所有标准方法（最长约 59 字符）
    n := 0
    for _, mr := range r.methodRoots {  // 有序切片，非 map
        _, _, _, found := matchRoute(mr.root, path, nil)
        if !found { continue }
        if n > 0 { buf[n] = ','; buf[n+1] = ' '; n += 2 }
        n += copy(buf[n:], mr.method)
    }
    if n == 0 { return "" }
    return string(buf[:n])
}

// Handle() 在触发 405 链前写入 Allow 头
if allow := r.allowedMethods(path); allow != "" {
    c.rw.ResponseWriter.Header().Set("Allow", allow)
    c.handlers = r.methodNotAllowedChain
} else {
    c.handlers = r.notFoundChain
}
```

`Router.methodRoots []methodRoot` 是新增的有序切片，`Add()` 在注册路由时按 `methodOrderRank()` 插入排序（启动路径，O(N) 可接受），保证 Allow 值在不同进程运行间稳定不变（消除 `map` 随机迭代带来的不确定性）。

**前后对比**：

| 场景 | 修复前（早退，无 Allow 头） | 修复后（全量遍历，RFC 合规） |
|------|:---------------------------:|:----------------------------:|
| 1 个方法树 | ~100 ns · ❌ 无 Allow 头 | ~115 ns · ✅ Allow 合规 |
| 5 个方法树 | ~100 ns（早退 = 1 次 matchRoute）| ~270 ns（5 次 matchRoute）|
| Allow 头顺序 | — | `GET, POST, DELETE`（固定）|
| B/op | — | 0（栈分配 buf）|

> 5 树场景 ~2.7× 变慢是正确行为的必要代价：原早退 = 规范违反。405 为错误路径，270 ns 绝对值可接受；200/404 热路径完全不受影响。

**新增测试**：`TestMethodNotAllowed_AllowHeader`（方法集合正确）、`TestMethodNotAllowed_AllowHeaderOrder`（固定顺序）、`TestNotFound_NoAllowHeader`（404 无 Allow）

---

### 深参数路由零分配（> 8 路径参数）

`Ctx` 内嵌 `paramsArr [8]Param` 作为路由参数 inline backing array。第 9 个参数触发 `append` 扩容，每请求多 1 alloc（512 B）、耗时 +47%。

通过三处协调改动彻底消除：

1. **`router.go`**：`maxParamDepth()` + `nodeParamDepth()` 启动时扫描所有方法树，计算实际最大参数深度。
2. **`context.go`**：新增 `overflowParams Params` 字段；`reset()` 按深度分支，深参数路由使用预分配的 backing slice，浅参数路由沿用 inline array——两路均零分配。
3. **`app.go`**：`sealPool()` 在监听前调用（所有路由注册完毕后）。当 `depth > 8` 时重新绑定 `pool.New`，为每个 `*Ctx` 预分配 `cap=depth` 的 `overflowParams`。

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `DeepParam_8_NoSeal`（基线） | 227 | 208 | 4 |
| `DeepParam_9_NoSeal`（修复前） | 344 | **720** | **5** |
| `DeepParam_9_Sealed`（修复后） | **243** | **208** | **4** |
| `DeepParam_12_Sealed`（修复后）| 307 | 208 | 4 |

9 参数路由：**−29% ns/op、−512 B/op、−1 alloc/op**。参数 ≤8 的应用零成本（`sealPool` 检测后直接返回）。

---

### Regex 路由快速路径（14 个内置模式）

14 个常用正则模式（`[0-9]+`、`\d+`、`[a-zA-Z0-9]+`、`[a-zA-Z0-9_-]+` 等）在路由注册时预编译为直接字节扫描函数（`fastMatcher`），存储于树节点，匹配时完全绕过 regexp 引擎。

```
BenchmarkRouter_Regex_FastPath_Parallel   ~70 ns/op
BenchmarkRouter_Regex_Custom_Parallel     ~77 ns/op（非内置模式，走 regexp 引擎）
```

---

### 全局 Regexp 缓存

相同模式在不同路由前缀重复注册时，原来每次独立编译 `*regexp.Regexp`，导致 `sync.Pool` 碎片化。新增 `regexpCache sync.Map`（`getOrCompileRegexp`），相同模式全局共享一个实例。`findRegexChild` 改为指针相等比较（快于字符串比较）。

---

## 运行时改进

### sync.Pool 冷启动预热

服务启动时 Pool 为空，前 GOMAXPROCS 个并发请求会触发 `pool.New`，在极高并发冷启动时产生一小波 GC 压力。`sealPool()` 末尾追加预热逻辑，在监听前将 `GOMAXPROCS` 个 `*Ctx` 放入 Pool，确保每个 P 的本地 slot 在第一个请求到达前已就位：

```go
n := runtime.GOMAXPROCS(0)
warmCtxs := make([]*Ctx, n)
for i := range warmCtxs { warmCtxs[i] = a.pool.New().(*Ctx) }
for _, c := range warmCtxs { a.pool.Put(c) }
```

低并发或渐进流量的应用不受影响。

### sync.Pool 争用分析（Benchmark 修正）

对"极高并发下 Pool 吞吐平缓"的报告进行完整根因分析，结论：Pool 本身无争用，瓶颈来自两个无关因素：

| 噪声来源 | 说明 |
|---------|------|
| `httptest.NewRecorder()` | 热循环内每次 208 B / 4 allocs，cpu=8 时 8 个 goroutine 并发分配，`mcentral` 锁产生竞争 |
| Apple M4 非对称核心 | cpu=4 全落性能核；cpu=8 额外 4 个 goroutine 落效能核（IPC ≈ 1/3），拉高平均 ns/op |

Pool 隔离基准（共享 Recorder，0 allocs）：cpu=1 → 23 ns，cpu=4 → 6.8 ns，cpu=8 → 5.0 ns（4.6× 线性提速，无平缓）。

新增 `BenchmarkIntegration_Parallel_Static_WarmPool` 作为 Pool 隔离基准（0 B / 0 allocs）；原两个 Parallel 基准注释更正，明确标注 4 allocs 来自 `httptest.NewRecorder`。

---

## 测试基础设施

### 集成测试自动化（testcontainers-go）

| 模块 | 容器镜像 | 覆盖场景 |
|------|---------|---------|
| `orm/clickhouse` | `clickhouse-server:24-alpine` | DDL、批量写入（100 行）、参数化查询、连接池、幂等 DDL、空表查询 |
| `search/elastic` | `elasticsearch:8.13.0`（TLS 启用）| Index 生命周期、term 搜索、BulkIndex、分页不重叠、terms 聚合、字段过滤、非存在文档删除 |
| `mq/pulsar` | Pulsar standalone | 单条发布消费、批量发布、消息 header 透传 |
| `config/apollo` | httptest mock | Load / Watch / 命名空间（无需容器，普通 `go test`）|

全部通过 `TestMain` 自动启停，无需手动 `docker run` 或环境变量，使用 `-tags integration` 触发。

### Benchmark CI 门禁

`.github/workflows/ci.yml` 新增 `bench` job，对核心 4 个 suite 运行 `benchstat` 对比，统计显著且 ≥+10% 的退化自动阻断 PR 合并（过滤 `~` 噪声行）。覆盖基准：`Router_Static`、`Context_JSON_Small`、`Integration_Baseline`、`Parallel_Static`。

---

## 迁移说明

### `OnStart` / `OnStop` 签名变更

```go
// v1.0（旧）
app.OnStart(func(ctx context.Context) { ... })

// v1.1（新）— 接收并可选检查返回的 error
if err := app.OnStart(func(ctx context.Context) error { ...; return nil }); err != nil {
    // 使用 NewSlim() 时返回 ErrSlimMode，使用 New() 时始终为 nil
}
```

使用 `New()` 时返回值始终为 `nil`，不检查也不影响行为。

### `NewSlim()` 使用注意

- `c.Bind` / `c.ShouldBind` 在 Slim 模式下 Binder 为 nil，调用会 panic；需要绑定时使用 `New()`。
- `OnStart` / `OnStop` / `Register` / `RegisterPlugin` 返回 `ErrSlimMode`，不会实际注册，建议调用后用 `errors.Is` 检查。

---

## 完整变更清单

### Added
- `examples/hello/main.go` — 18 行零概念最小模板
- `examples/quickstart/main.go` — ~90 行真实服务模板
- `docs/getting-started/quickstart.md` — 三步渐进指南
- `NewSlim(opts ...Option) *App` — Slim 构造函数
- `ErrSlimMode` — Slim 模式哨兵错误
- `BenchmarkIntegration_Parallel_Static_WarmPool` — Pool 隔离基准
- `BenchmarkMethodNotAllowed_5Methods` / `BenchmarkMethodNotAllowed_1Method`
- `TestMethodNotAllowed_AllowHeader` / `TestMethodNotAllowed_AllowHeaderOrder` / `TestNotFound_NoAllowHeader`
- `orm/clickhouse`、`search/elastic`、`mq/pulsar`、`config/apollo` 集成测试

### Changed
- `OnStart` / `OnStop` 返回 `error`（原无返回值）
- `sealPool()` 新增 Pool 预热（GOMAXPROCS 个 `*Ctx`）
- `router`: `methodNotAllowed(bool)` → `allowedMethods(string)` + `methodRoots []methodRoot`
- `router`: 14 个内置 Regex 模式改为字节扫描快速路径
- `router`: 全局 `regexpCache sync.Map` 共享编译结果
- `router` / `context` / `app`: 深参数路由 `overflowParams` + `sealPool` 联动
- `BenchmarkIntegration_Parallel_Static` 注释修正（4 allocs 来自 NewRecorder）
- `BenchmarkServeHTTP_Parallel_Static` 注释修正（同上）
- `.github/workflows/ci.yml` Benchmark CI 门禁

### Fixed
- **RFC 9110 §15.5.6**：405 响应现在正确携带 `Allow` 头（原版本 RFC 违规）
- **深参数路由**：超过 8 个路径参数时不再触发 512 B 堆分配（+47% latency）
- **Regex 重复编译**：相同模式多处注册时不再重复编译 `*regexp.Regexp`

---

*完整 diff 见 [CHANGELOG.md](CHANGELOG.md)。*
