## 改进路线图

基于综合评估报告（v7, 2026-04）的分析结论，按优先级列出已知短板及改进方向。

### P0 — 关键，立即着手

#### 1. 社区建设与文档国际化

| 事项 | 现状 | 目标 |
|------|------|------|
| 英文文档 | 仅中文 README / 注释 | 完整英文 docs 站，英文 Quickstart |
| examples 目录 | 少量示例 | 覆盖 CRUD / JWT / WebSocket / MQ 的完整可运行示例 |
| GitHub Discussions | 未启用 | 开启 Q&A / Show & Tell / Ideas 三类讨论 |
| Issue 模板 | 无结构 | Bug Report / Feature Request / Performance 三套模板 |
| awesome-go 收录 | 未提交 | 提交 PR 进入 `avelino/awesome-go` |

#### 2. JWT 验证 allocs 优化（已完成）

**Phase 1（已完成）**：消除 `parseToken` 双重解析，`JWT_ValidToken` 从 **105 → 58 allocs/req**（-45%），速度 4 808 → 2 244 ns/op（-53%）。详见"内存分配优化历程 · 第三轮"。

**Phase 2（已完成）**：引入 `claimsPool` + `ndPool`，缓存 key 改为 token 签名段（减少哈希长度 ~78%），TTL 改为 80% 剩余有效期。

- 无缓存路径：**58 → ~35 allocs**（-40%），**2 244 → ~1 800 ns/op**（-20%）
- LRU cache hit 路径：**~5 allocs**（纯 httptest 开销，无密码学运算）
- 实现：`claimsPool` / `ndPool` / `parseTokenPooled` / `releaseClaims`，中间件明确调用 `c.Next()` 后回收 Claims

详见"内存分配优化历程 · 第五轮"。

---

### P1 — 重要，本季度完成

#### 3. Reactor 引擎新连接优化（已完成）

**已完成**：`connStatePool` + `dispatchNewDirect` 彻底消除新连接的 epoll/kqueue 注册轮回开销：

| 措施 | 效果 |
|------|------|
| `connStatePool` 池化 `*connState` | 热路径 −3 allocs（struct + bufio 缓冲区） |
| `dispatchNewDirect` 直接派发 | NewConn 延迟 84 µs → 65 µs（−22%），短连接不进入 poller |
| 懒注册（`registered bool`） | keep-alive rearm 用 `poller.add`；后续 rearm 用 `poller.mod`；close-only 连接零 `epoll_ctl` 调用 |

详见"内存分配优化历程 · 第六轮"。

#### 4. Content-Length 零分配（已完成）

**已完成**：`JSON()` 使用 `init()` 预建的 0–1023 `[]string` 缓存，小于 1 KB 的响应 Content-Length 设置完全零分配（原来 2 allocs：string + slice）。

#### 5. 大列表 JSON 无缓冲流式输出（已完成）

**已完成**：新增 `c.JSONStream()` API，大列表响应直接编码到 `ResponseWriter`，彻底跳过 pooled `bytes.Buffer` 中间缓冲；

- `reuseWriter.w` 泛化为 `io.Writer`，复用 `goJsonEncPool`，零额外分配
- `streamEncoder` 接口（与 `bufEncoder` 对称）+ `EncodeStream` 方法
- `contract.Context` 接口同步添加 `JSONStream(code int, obj any) error`
- allocs **12 → 10**（−17%），生产环境消除 ~43 KB 堆缓冲及 `WriteTo` 拷贝

详见"内存分配优化历程 · 第七轮"。

---

### P2 — 工程质量 ✅ 已完成

#### 5. 基准测试 CI 门禁

防止性能回归无感知地合入主干，已集成至 `.github/workflows/ci.yml` 的 `bench` job：

```yaml
# .github/workflows/ci.yml — bench job（已落地）
- name: Run benchmarks
  # 限定核心 4 个 suite，不触碰需要外部服务的模块（orm/search/mq）
  run: |
    go test -bench=. -benchmem -count=6 -benchtime=1s \
      . ./netengine/ ./middleware/ ./benchmarks/ \
      2>/dev/null | tee bench-current.txt

- name: Restore baseline     # PR：从 cache 拉 main 分支的 bench-main.txt
  uses: actions/cache/restore@v4
  with:
    path: bench-main.txt
    key: bench-main-${{ github.base_ref }}

- name: Compare with benchstat
  run: |
    benchstat bench-main.txt bench-current.txt | tee bench-diff.txt
    # 过滤统计噪声行（~），仅对统计显著且 ≥+10% 的退化阻断 PR 合并
    regressions=$(grep -E '\+[1-9][0-9]+\.[0-9]+%' bench-diff.txt \
                  | grep -v '~' || true)
    if [ -n "$regressions" ]; then
      echo "::error::Performance regression detected (>= 10%)"
      exit 1
    fi

- name: Save baseline        # main push：更新 cache，path 与 Restore 一致
  uses: actions/cache/save@v4
  with:
    path: bench-main.txt
    key: bench-main-${{ github.ref_name }}
```

关注基准：`Router_Static`、`Context_JSON_Small`、`Integration_Baseline`、`Parallel_Static`

> **修复要点（相对原始示意）**
> - 原 save `path: bench-current.txt` / restore `path: bench-main.txt` 路径不一致，导致 baseline 永远取不到；统一改为 `bench-main.txt`。
> - `./...` 会遍历 `orm/`、`search/`、`mq/` 等需要外部服务的模块，导致 bench job 报错；改为显式列举 4 个核心 suite。
> - 新增 `grep -v '~'` 过滤统计不显著行，避免噪声误判触发门禁。

#### 6. 高级模块集成测试补全

| 模块 | 之前 | 现状 |
|------|------|------|
| `orm/clickhouse` | 无集成测试 | ✅ testcontainers e2e：DDL、批量写入（100 行）、参数化查询、连接池配置、空表、幂等 DDL（`-tags integration`） |
| `search/elastic` | 单元测试 | ✅ testcontainers ES8 端到端：Index / BulkIndex / Search / Delete / 分页（不相交验证）/ 聚合（bucket 数量）/ 字段过滤 / 非存在文档删除（`-tags integration`） |
| `mq/pulsar` | 仅构建验证 | ✅ Pulsar 往返测试：单条发布消费、批量发布、消息 header 透传（`-tags integration`） |
| `config/apollo` | 无测试 | ✅ Apollo mock：httptest 模拟 Apollo HTTP API，覆盖 Load / Watch / 命名空间（普通 `go test`） |

容器由 **testcontainers-go** 在 `TestMain` 内自动启动和销毁，无需手动 `docker run` 或配置环境变量。运行方式：

```bash
# ClickHouse e2e（自动启动 clickhouse/clickhouse-server:24-alpine）
go test -tags integration -v ./orm/clickhouse/...

# Elasticsearch 8 e2e（自动启动 elasticsearch:8.13.0，含 TLS/xpack）
go test -tags integration -v ./search/elastic/...
```

新增边界测试覆盖：

| 场景 | ClickHouse | Elasticsearch |
|------|:----------:|:-------------:|
| 批量写入后计数验证 | ✅ 100 行批量 | ✅ 10 文档 BulkIndex |
| 幂等操作 | ✅ IF NOT EXISTS 三次 DDL | ✅ 同 ID 覆盖写验证 _source |
| 空集合查询 | ✅ 空表 Find 返回 0 行 | ✅ 删除后搜索 Total=0 |
| 分页不重叠 | — | ✅ page1 ∩ page2 = ∅ |
| 字段过滤 | — | ✅ Source filter 排除未指定字段 |
| 非存在资源 | — | ✅ 删除不存在文档不报错 |
| 聚合 bucket 数 | — | ✅ terms agg bucket 数量断言 |

#### 7. 大参数路由零分配（已完成）

**问题**：`Ctx` 结构体内嵌 `paramsArr [8]Param`（`maxRouteParams = 8`）作为路由参数的 inline backing array。当路由路径参数超过 8 个时，`matchSegments` 内的 `append(params, ...)` 超出容量，Go runtime 分配新 backing array（cap 扩容至 16，`16×32B = 512 B`），每请求额外 1 alloc、耗时 +47%。

**方案选型**：

| 方案 | 改动 | ≤8 参数开销 | >8 参数开销 | 结论 |
|------|------|------------|------------|------|
| A：扩大常量 `maxRouteParams=16` | 1 行 | +256 B/Ctx（浪费） | 消除 | 不推荐 |
| B：运行时 Pool 兜底 | 中等 | 不变 | 减少（pool 复用） | 复杂度高 |
| **C：启动扫描精确定尺（采用）** | 中等 | **不变** | **消除** | **推荐** |

**实现（三处改动）**：

```go
// router.go — 启动时扫描所有方法树，返回最深参数段数
func (r *Router) maxParamDepth() int { ... }
func nodeParamDepth(n *node, depth int) int { ... } // 递归遍历 paramNode / regexNode / catchAllNode

// context.go — 新增 overflowParams 字段
overflowParams Params // 非 nil 时由 reset() 用作 params 的 backing array

// context.go — reset() 按深度分支
if c.overflowParams != nil {
    c.params = c.overflowParams[:0]   // 预分配的深参数切片，零分配
} else {
    c.params = c.paramsArr[:0]        // 常规 inline array，零分配
}

// app.go — Run() 前调用，重置 pool.New 闭包
func (a *App) sealPool() {
    depth := r.maxParamDepth()
    if depth <= maxRouteParams { return } // 常规路由，无变化
    a.pool.New = func() any {
        c := a.allocateContext()
        c.overflowParams = make(Params, 0, depth) // 一次性按需分配
        c.params = c.overflowParams
        return c
    }
}
```

`sealPool()` 在 `runWithGracefulShutdown` 入口调用，确保所有路由注册完毕后一次性扫描，不影响运行时热路径。

**基准数据（Apple M4 · Go 1.25）**：

| bench | ns/op | B/op | allocs/op | 说明 |
|---|---|---|---|---|
| `DeepParam_8_NoSeal` | 227 | 208 | 4 | 8 参数，inline，修复前后一致 |
| `DeepParam_9_NoSeal` | 344 | **720** | **5** | 9 参数，修复前：溢出 +512 B，+47% |
| `DeepParam_9_Sealed` | **243** | **208** | **4** | 9 参数，修复后：与 8 参数齐平，**−29% ns、−512 B** |
| `DeepParam_12_Sealed` | 307 | 208 | 4 | 12 参数，修复后：0 溢出分配 |

> 对绝大多数应用（参数 ≤8）**零成本**——`sealPool` 检测后直接返回，`Ctx` 结构体大小不变。仅在注册了深参数路由的应用中，pool 里的 `*Ctx` 多一个指向预分配切片的指针字段（8 B）。

#### 8. sync.Pool 高并发争用分析（已完成）

**原始报告**：`BenchmarkIntegration_Parallel_Static` 和 `BenchmarkServeHTTP_Parallel_Static` 在 GOMAXPROCS 个 goroutine 共享同一 Pool 时暴露争用，极高并发（>= GOMAXPROCS × 2）下吞吐量趋于平缓。

**根因分析**：完整拆解后，问题来自**两个与 Pool 无关的因素**，Pool 本身无争用。

| 维度 | 原始 benchmark（含 NewRecorder） | WarmPool benchmark（Pool 隔离） |
|---|---|---|
| cpu=1 | 143 ns/op, 4 allocs | 23 ns/op, **0 allocs** |
| cpu=4 | 57 ns/op（2.5× 提速） | 6.8 ns/op（3.4× 提速） |
| cpu=8 | 69 ns/op（**退步** vs cpu=4） | 5.0 ns/op（**4.6× 提速**，持续线性） |

- **噪声来源**：`httptest.NewRecorder()` 在热循环内每次触发 208 B/4 allocs 的堆分配；cpu=8 时 8 个 goroutine 并发分配，`mcentral` 锁产生竞争。真实 net/http 不存在此分配，生产场景不受影响。
- **硬件效应**：Apple M4 非对称核心（4 效能核 + 6 性能核）。cpu=4 全落在性能核，cpu=8 额外 4 个 goroutine 落在效能核（IPC 约 1/3），拉高平均 ns/op——这是硬件调度器行为。

**实际存在的轻微问题**：冷启动 Pool miss。服务启动时 Pool 为空，前 GOMAXPROCS 个并发请求会调用 `allocateContext()`，在极高并发冷启动时产生一小波 GC 压力。

**修复（已合并入 `sealPool()`）**：在服务监听前预分配 `GOMAXPROCS` 个 Ctx 并归还 Pool，确保每个 P 的本地 slot 在第一个请求到达前已预热，零代码变更对调用方。

```go
// app.go — sealPool() 末尾追加（每次服务启动执行一次）
n := runtime.GOMAXPROCS(0)
warmCtxs := make([]*Ctx, n)
for i := range warmCtxs {
    warmCtxs[i] = a.pool.New().(*Ctx)
}
for _, c := range warmCtxs {
    a.pool.Put(c)
}
```

**Benchmark 修正**：新增 `BenchmarkIntegration_Parallel_Static_WarmPool`（共享 Recorder，0 allocs），隔离 Pool Get/reset/Handle/Put 的纯开销；原 `BenchmarkIntegration_Parallel_Static` 注释更正，明确标注 4 allocs 来自 `httptest.NewRecorder`。

#### 9. Slim 启动模式（已完成）

**问题**：`astra.New()` 无论使用场景如何，都会初始化完整的 Lifecycle（含 hook 切片 + mutex）、Module / Plugin 注册表，并通过 `defaultOptions()` 引入 `binding.Default`（拉入 `go-playground/validator/v10`，约 1.2 MB 编译后体积）。对 Serverless / FaaS / 极简微服务场景造成：

- **冷启动延迟**：validator 反射初始化 + 大型二进制加载
- **内存基线偏高**：lifecycle hooks 切片 + 完整 options 结构
- **依赖传递**：即使不用 binding，也被静态链接进二进制

**方案选型**：

| 方案 | 二进制裁剪 | API 兼容 | 实现复杂度 | 迁移成本 |
|------|:----------:|:--------:|:----------:|:--------:|
| A：Build Tags（`slim` 构建标签） | ★★★★★ | ★★ | 中 | 高（改构建命令） |
| B：`WithSlim()` 功能选项 | ★★ | ★★★★ | 低 | 极低 |
| **C：`NewSlim()` 独立构造函数（采用）** | ★★★ | **★★★★★** | **最低** | **极低** |
| D：`astra/slim` 子包 | ★★★★★ | ★★ | 高 | 高（改 import） |

选择方案 C 的核心原因：`*App` 类型统一（所有接受 `*astra.App` 的 middleware/helper 不需要改动），改动 < 50 行，新旧服务无缝共存。

**实现（四处改动）**：

```go
// app.go — App struct 新增 slim 标志
type App struct {
    ...
    slim bool // true 时禁用 lifecycle/plugin/module 子系统
}

// app.go — 新构造函数，lifecycle 保持 nil
func NewSlim(opts ...Option) *App {
    options := slimDefaultOptions() // Binder: nil，其余与 defaultOptions 一致
    for _, opt := range opts { opt(options) }
    options.prepareTrustedNets()
    app := &App{options: options, slim: true}
    app.router = newRouter(app)
    app.pool.New = func() any { return app.allocateContext() }
    return app
}

// app.go — lifecycle 调用改为 nil guard；OnStart/OnStop 返回 error
func (a *App) OnStart(fn func(context.Context) error) error {
    if a.slim { return ErrSlimMode }
    a.lifecycle.OnStart(fn); return nil
}
func (a *App) OnStop(fn func(context.Context) error) error {
    if a.slim { return ErrSlimMode }
    a.lifecycle.OnStop(fn); return nil
}

// app.go — RegisterPlugin / module.go Register 同样 guard
func (a *App) RegisterPlugin(plugins ...Plugin) error {
    if a.slim { return ErrSlimMode }
    ...
}
func (a *App) Register(modules ...Module) error {
    if a.slim { return ErrSlimMode }
    ...
}

// options.go — slim 版 defaults，Binder: nil
func slimDefaultOptions() *Options {
    return &Options{..., Binder: nil}
}

// errors.go — 新增哨兵错误
var ErrSlimMode = fmt.Errorf("astra: operation not available in slim mode (use astra.New())")
```

**使用方式**：

```go
// 极简 Serverless 处理器 — 路由 + pool，无 DI/Plugin/Lifecycle
app := astra.NewSlim()
app.GET("/health", func(c *astra.Ctx) error {
    return c.JSON(200, map[string]string{"status": "ok"})
})
app.Run(":8080")

// 禁用功能调用时返回 ErrSlimMode，可用 errors.Is 检测
if err := app.OnStart(hook); errors.Is(err, astra.ErrSlimMode) {
    // 应切换为 astra.New()
}
```

**收益**：

| 能力 | `New()` | `NewSlim()` |
|------|:-------:|:-----------:|
| 路由 / 中间件 / ServeHTTP | ✅ | ✅ |
| 优雅关闭 | ✅ | ✅ |
| Lifecycle hooks（OnStart/OnStop）| ✅ | ❌ ErrSlimMode |
| Module / Plugin 注册 | ✅ | ❌ ErrSlimMode |
| `c.Bind` / `c.ShouldBind`（validator）| ✅ | ❌ Binder nil |
| `go-playground/validator` 链接 | 是 | **否**（不导入）|
| Lifecycle struct 分配 | 是 | **否** |

> `*App` 类型完全相同，中间件、路由组等所有公共 API 不受影响。仅在调用被禁用功能时快速失败（`ErrSlimMode`），便于早期发现配置错误。

> **迁移说明**：`OnStart` / `OnStop` 的签名由 `func(…)` 无返回值改为 `func(…) error`，以便返回 `ErrSlimMode`。使用 `New()` 的调用方只需在调用处接收并检查返回的 `error`（`New()` 下始终返回 `nil`，不影响现有逻辑）。

#### 10. methodNotAllowed 全树遍历 → RFC 合规 Allow 头（已完成）

**问题**：`methodNotAllowed(path string) bool` 遍历所有 HTTP 方法树检测 405 场景，但从不设置 `Allow` 响应头，违反 RFC 9110 §15.5.6（"405 响应必须包含 Allow 字段，列出请求 URL 支持的所有方法"）。此外，底层使用 `map[string]*node` 遍历，返回顺序不确定，导致相同路由在不同运行中产生不同的 `Allow` 值。

**方案对比**：

| 方案 | RFC 合规 | Allow 顺序 | 性能 | 结论 |
|------|:--------:|:----------:|:----:|------|
| 旧 `methodNotAllowed(bool)` | ❌（无 Allow 头��| ❌（map 随机） | N 次 matchRoute | 不合规 |
| 提前退出（找到第一个匹配即返回） | ❌（无完整 Allow）| — | < N 次 | 仍违规 |
| **全遍历 `allowedMethods(string)`（采用）** | ✅ | ✅（有序 slice）| N 次 matchRoute | RFC 合规 |

> 405 本身是非主路径（客户端错误），全遍历代价完全可接受。

**实现（三处改动）**���

```go
// router.go — 有序方法列表（确保 Allow 头值稳定可预期）
type methodRoot struct { method string; root *node }
var methodOrder = []string{
    http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
    http.MethodDelete, http.MethodHead, http.MethodOptions,
}

// Router.Add() 维护有序切片（启动路径，insertion sort，O(N) 可接受）
func (r *Router) Add(method, path string, handlers HandlerChain) {
    ...
    rank := methodOrderRank(method)
    // 按 rank 插入 r.methodRoots，保持有序
}

// allowedMethods：替换 methodNotAllowed，单次遍历构建 Allow 字符串
func (r *Router) allowedMethods(path string) string {
    var buf [128]byte  // 栈分配，零堆 alloc
    n := 0
    for _, mr := range r.methodRoots {
        _, _, _, found := matchRoute(mr.root, path, nil)
        if !found { continue }
        if n > 0 { buf[n] = ','; buf[n+1] = ' '; n += 2 }
        n += copy(buf[n:], mr.method)
    }
    if n == 0 { return "" }
    return string(buf[:n])
}

// Handle()：设置 Allow 头后触发 405 链
if allow := r.allowedMethods(path); allow != "" {
    c.rw.ResponseWriter.Header().Set("Allow", allow)
    c.handlers = r.methodNotAllowedChain
} else {
    c.handlers = r.notFoundChain
}
```

**基准数据（Apple M4 · Go 1.25）**：

| 场景 | 修复前（无 Allow 头，早退） | 修复后（RFC 合规，全量遍历） |
|---|---|---|
| 1 个方法树，HEAD 请求 | ~100 ns | ~115 ns |
| 5 个方法树，HEAD 请求 | ~100 ns（早退 = 1 次 matchRoute） | ~270 ns（5 次 matchRoute，全量） |
| Allow 头正确性 | ❌ RFC 违规（无 Allow 头） | ✅ RFC 合规 |
| Allow 头顺序稳定性 | — | `GET, POST, DELETE`（固定） |
| B/op | — | 0（栈分配 buf，仅最终 `string()` 1 次堆） |

> 5 个方法树场景变慢约 **2.7×** 是正确行为导致的**必要代价**：原代码用早退规避了 RFC 要求的全量遍历，属于错误实现。405 是错误路径，270 ns 绝对值完全可接受；正常 200 / 404 路径不受任何影响。

**新增测试**：
- `TestMethodNotAllowed_AllowHeader`：验证 `Allow` 包含 GET/POST/DELETE，不含 PATCH
- `TestMethodNotAllowed_AllowHeaderOrder`：验证固定顺序 `"GET, POST, DELETE"`（不随注册顺序变化）
- `TestNotFound_NoAllowHeader`：验证真 404 路径无 `Allow` 头

---

#### 11. Context KV Store 去锁 + 动态切片（已完成）

**问题**：`c.Set` / `c.Get` 存在两层叠加缺陷：

**缺陷一：锁粒度错误**。`keysMu sync.RWMutex` 包裹了整个 `Set`/`Get` 函数体，即使操作只访问 `smallKeys` inline 数组（从不触碰 overflow map）也无例外。标准请求中，中间件链是单 goroutine 串行执行的，`*Ctx` 不存在并发访问——锁只有成本，没有保护价值。每次 `Set`/`Get` 均触发 `LOCK CMPXCHG` + 全局内存屏障，10 层中间件 × 2 ops/层 = 每请求 20 次无效原子操作。

**缺陷二：`smallKeysCap = 8` 阈值偏低，溢出触发 map 分配**。典型中间件链（RequestID + Logger + JWT + RateLimit + Tracing + RBAC + Tenant）合计 ≥9 次 `c.Set`，超出后触发 `make(map[string]any)` 堆分配 + map 哈希开销 + `reset()` 时的 O(N) `delete` 循环。

**三个方案对比**：

| 方案 | inline 路径去锁 | 无 map | 无魔数 cap | 实现复杂度 |
|------|:--------------:|:------:|:----------:|:----------:|
| A：分层锁（smallKeys 无锁，map 有锁）| ✅ | ❌ | ❌ | 低 |
| B：A + 扩大 `smallKeysCap` 到 16 | ✅ | ❌（>16 仍溢出）| ❌ | 低 |
| **C：`[]kvPair` 动态切片，彻底去锁（采用）** | ✅ | ✅ | ✅ | **最低** |

方案 C 是唯一没有历史包袱的终态设计：无锁、无 map、无需调参。

**实现（三处改动）**：

```go
// context.go — 删除五个旧字段，新增一个
// 删除: smallKeysCap const、smallKeys [8]kvPair、smallLen int8
// 删除: keys map[string]any、keysMu sync.RWMutex（节省 ~296 B/Ctx）
kvStore []kvPair  // nil 直到首次 Set；reset() 用 [:0] 保留 backing array

// context_store.go — 全新实现，零 mutex
func (c *Ctx) Set(key string, value any) {
    // routeKey fast path 不变...
    for i := range c.kvStore {
        if c.kvStore[i].key == key {
            c.kvStore[i].value = value  // 原地更新，无重复 key
            return
        }
    }
    c.kvStore = append(c.kvStore, kvPair{key: key, value: value})
}

func (c *Ctx) Get(key string) (any, bool) {
    // routeKey fast path 不变...
    for i := range c.kvStore {
        if c.kvStore[i].key == key {
            return c.kvStore[i].value, true
        }
    }
    return nil, false
}

// context.go reset() — 替换旧的 smallKeys 清理 + delete(c.keys)
kv := c.kvStore
for i := range kv { kv[i].key = ""; kv[i].value = nil }  // 释放 GC 引用
c.kvStore = kv[:0]  // 保留 backing array，下次请求零分配
```

**并发安全约定**（与 Gin / Echo 一致）：`c.Set` / `c.Get` 非 goroutine 安全。handler 内启动 goroutine 时，请在启动前将所需值复制到局部变量。

**基准数据（Apple M4 · Go 1.25）**：

| 场景 | 改造前（inline array + mutex + map 溢出）| 改造后（[]kvPair，零锁）|
|---|---|---|
| ≤8 key，每次 Set/Get | ~10–30 ns mutex 开销/op | **0 ns mutex 开销** |
| 12 key（溢出场景） | +1 alloc（map make）+ O(N) delete | **208 B / 4 allocs**（与 4 key 齐平）|
| `Ctx` 结构体大小 | +24 B（RWMutex）+256 B（inline array）| **节省 ~280 B/Ctx** |
| Pool 对象内存压力 | 高并发下 N_goroutine × 280 B 额外占用 | 消除 |

> 12 个 key 的请求与 4 个 key **分配完全相同（208 B / 4 allocs）**，4 allocs 全部来自 `httptest.NewRecorder()`，框架本身零分配。

---

### P3 — 加分项，后续迭代


#### 7. HTTP/2 ALPN 协商 & EarlyHints

✅ 已实现（`app_reactor.go` + `context_response.go`）

`RunReactorTLS` 的 `tls.Config` 已自动注入 `NextProtos: ["h2", "http/1.1"]`，修复 ALPN 协商；h2 连接在 worker goroutine 中由 `net/http` http2 包（`http2.Server.ServeConn`）独立处理，worker 立即归池，accept 永不阻塞。

`Push()` API 已标记 Deprecated，推荐使用 `EarlyHints`（RFC 8297 103 interim 响应）替代：

```go
// Push initiates an HTTP/2 server push for the given target path.
// Deprecated: 推荐使用 EarlyHints 替代。
// Returns http.ErrNotSupported when the underlying connection is HTTP/1.1
// or the client has disabled push via SETTINGS_ENABLE_PUSH=0.
func (c *Ctx) Push(target string, opts *http.PushOptions) error {
    if p, ok := c.writer.(http.Pusher); ok {
        return p.Push(target, opts)
    }
    return http.ErrNotSupported
}
```

```go
// 推荐用法：EarlyHints 发送 103 interim 响应，提示浏览器预加载资源
if err := c.EarlyHints(
    []string{"/static/app.css", "/static/app.js"},
    map[string]string{"as": "style"},
); err != nil {
    return err
}
return c.HTML(200, page)
```

> Reactor TLS 路径通过 ALPN 已实现 h2 支持；`EarlyHints` 无需额外配置即可在 h2 连接上生效。

#### 8. 英文生态布道

- 在 **Reddit r/golang** 发布 Astra vs Gin/Echo 对比评测（附基准数据）
- 在 **Hacker News** 的 "Show HN" 栏目展示框架核心优势
- 在 **GitHub Trending**（Go 分类）刷新展示
- 撰写 **Dev.to / Medium** 技术博客：《Building a production API with Astra in 30 minutes》

---

### 短板现状速查

| 短板 | 严重度 | 改进状态 | 目标版本 |
|------|:------:|:--------:|:--------:|
| 社区 / Stars 少 | 🔴 高 | 待启动 | v1.1 |
| JWT allocs 优化 Phase 1（105→58）| 🟢 已完成 | ✅ v1.0 已完成 | v1.0 |
| JWT allocs 优化 Phase 2（58→~35，cache hit ~5）| 🟢 已完成 | ✅ v1.1 已完成 | v1.1 |
| Reactor NewConn 优化（84µs→65µs，−22%）| 🟢 已完成 | ✅ v1.1 已完成 | v1.1 |
| 404 路径优化（770→403 ns，−48%）| 🟢 已完成 | ✅ v7 已完成 | v1.0 |
| ConsistentHash 重建（86µs→21µs，−99.7% allocs）| 🟢 已完成 | ✅ v7 已完成 | v1.0 |
| POST+JSON 绑定（34→24 allocs，−29%）| 🟢 已完成 | ✅ v7 已完成 | v1.0 |
| Content-Length 零分配（< 1 KB）| 🟢 已完成 | ✅ v1.0 已完成 | v1.0 |
| 大列表 JSON Stream 无缓冲输出（allocs 12→10，消除 ~43 KB 堆缓冲）| 🟢 已完成 | ✅ v1.1 已完成 | v1.1 |
| 基准 CI 门禁 | 🟡 低 | ✅ 已完成 | v1.1 |
| 大参数路由零分配（9 参数 −29% ns、−512 B）| 🟡 低 | ✅ 已完成 | v1.1 |
| Slim 启动模式（NewSlim / 禁用 validator/lifecycle/DI）| 🟡 低 | ✅ 已完成 | v1.1 |
| Context KV Store 去锁 + 动态切片（map+mutex → []kvPair，12 key 仍 0 allocs）| 🟡 中 | ✅ 已完成 | v1.1 |
| methodNotAllowed → allowedMethods：RFC 9110 §15.5.6 Allow 头合规（1-method ~115 ns，5-method ~270 ns，0 allocs）| 🟡 低 | ✅ 已完成 | v1.1 |
| sync.Pool 争用误报：benchmark 噪声修正 + 冷启动预热（Pool 本身无争用）| 🟡 低 | ✅ 已完成 | v1.1 |
| ClickHouse/Elastic 集成测试（testcontainers，自动启停容器）| 🟡 低 | ✅ 已完成 | v1.1 |
| HTTP/2 ALPN 协商（RunReactorTLS h2 分流）+ EarlyHints（103 interim）+ Push Deprecated | ⚪ 加分 | ✅ 已完成 | v1.3 |
| 英文文档 | 🔴 高 | 待启动 | v1.1 |
| RunReactorTLS 未显式设置 `MinVersion`（安全配置） | 🔴 安全 | ✅ 已完成 | v1.2 |
| `signal.Stop` 未在 Reactor 关闭后调用（资源泄漏） | 🟡 中 | ✅ 已完成 | v1.2 |
| `BindJSON` body 上限 1 MiB 固定不可配置 | 🟡 中 | ✅ 已完成 | v1.2 |
| `BindPath` 每请求堆分配（违反零分配设计）| 🟡 中 | ✅ 已完成 | v1.2 |
| handler 链 int8 硬上限 127（`abortIndex`）| 🟡 中 | ✅ 已完成 | v1.2 |
| `BindQuery` 未复用 `queryCache`（每次二次解析 URL）| 🟢 低 | ✅ 已完成 | v1.2 |
| `MustBind*` 验证失败不写响应体（客户端收空 422）| 🟢 低 | ✅ 已完成 | v1.2 |
| `NewSlim` Binder=nil 调用绑定 API 触发 panic | 🟢 低 | ✅ 已完成 | v1.2 |
| `TrustedProxies` 无效条目静默跳过无日志警告 | 🟢 低 | ✅ 已完成 | v1.2 |
| `GetInt`/`GetBool` 类型不匹配静默返回零值 | 🟢 低 | ✅ 已完成 | v1.2 |
| `BindXML` 无缓冲池（与 `BindJSON` 分配策略不对称）| 🟢 低 | ✅ 已完成 | v1.2 |

> 进度更新见 [CHANGELOG.md](CHANGELOG.md) 和 [GitHub Milestones](https://github.com/astra-go/astra/milestones)。

---

