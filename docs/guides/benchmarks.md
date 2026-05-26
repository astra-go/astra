## 基准测试

> **⚠ 平台说明**：本页数字分为两类，阅读时请注意区分：
> - **[Apple M4]** — 本地开发机数字，来自 `make bench-all`（Apple M4 · Go 1.25.8 · 5 轮 × 2s/轮）。ARM 架构内存延迟更低，ns/op 通常低于 linux/amd64 生产环境 **10–30%**，仅供趋势参考。
> - **[CI linux/amd64]** — GitHub Actions `ubuntu-latest` 共享 runner，由 `.github/workflows/benchmark-publish.yml` 自动生成，用于横向框架对比。共享 runner 存在 ±15% 噪声，精确数字以 [在线报告](https://astra-go.github.io/astra/benchmarks/) 为准。
>
> 需要在生产平台复现：`go test -bench=. -benchmem -count=5 ./...`（linux/amd64 裸机或专用 runner）。

以下数据来自 `make bench-all`，测试环境 **[Apple M4] · Go 1.25.8 · 5 轮 × 2s/轮**。
运行方式：

```bash
# 快速全套
make bench-all

# 快速单轮（开发中验证，低计数快速反馈）
make bench-quick

# 与基线对比（需 benchstat）
go install golang.org/x/perf/cmd/benchstat@latest
make bench-save-baseline   # 记录当前数据
# … 修改代码 …
make bench-compare         # 输出 delta 表格

# 生成本地 HTML 对比报告
make bench-publish-local   # 输出到 site/benchmarks/index.html
```

> **历史趋势**：每次 main 分支合并后，CI 自动将关键指标写入 [bench-history.json](https://astra-go.github.io/astra/benchmarks/bench-history.json)，可用于追踪长期性能演进。在线报告见 [astra-go.github.io/astra/benchmarks/](https://astra-go.github.io/astra/benchmarks/)。

---

### 路由与核心（根包）<small>　[Apple M4]</small>

| 基准 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| `Router_Static` | 108 | 208 | 4 |
| `Router_Static_REST`（25 资源，首字母各异）**[v11 ✅]** | **107** | **208** | **4** |
| `Router_Static_100`（100 路由，数字后缀） | 281 | 208 | 4 |
| `Router_Param` (`:id`) | 116 | 208 | 4 |
| `Router_Param_Deep`（3 段参数） | 146 | 208 | 4 |
| `Router_Regex` (`{id:[0-9]+}`) | 154 | 208 | 4 |
| `Router_Wildcard` (`*path`) | 120 | 208 | 4 |
| `Router_NotFound` **[v7 已优化]** | **403** | **1 016** | **9** |
> `Router_Static_REST`：注册 25 个顶层资源路由（`/users /orders /products /auth /settings …`），命中最后注册的 `/webhooks`；
> 首字节分发表使其耗时等同单路由 O(1) 基线（107 vs 108 ns/op）。

**中间件链扩展代价**（每增加 1 个 handler 仅增加 ~4 ns，零额外分配）：

| 基准 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| `MiddlewareChain_0`（仅 handler） | 113 | 208 | 4 |
| `MiddlewareChain_1` | 114 | 208 | 4 |
| `MiddlewareChain_3` | 119 | 208 | 4 |
| `MiddlewareChain_5` | 122 | 208 | 4 |
| `MiddlewareChain_10` | 136 | 208 | 4 |
| `MiddlewareChain_Abort`（链中断） | 123 | 216 | 5 |

**Context 响应写入**：

| 基准 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| `Context_JSON_Small`（~120 B） | 450 | 1 028 | 9 |
| `Context_JSON_Medium`（~350 B） | 634 | 1 381 | 10 |
| `Context_JSON_Large`（100 项 ~12 KB） | 4 560 | 13 505 | 12 |
| `Context_JSONStream_Large`（100 项，无缓冲）**[v10]** | **4 320** | **13 366** | **10** |
| `Context_String` | 383 | 992 | 8 |
| `Context_QueryParams`（5 参数） | 559 | 688 | 11 |
| `ServeHTTP_Parallel_Static` | 91 | 208 | 4 |
| `ServeHTTP_Parallel_JSON` | 450 | 1 284 | 10 |

---

### netengine — Reactor 引擎<small>　[Apple M4, loopback]</small>

**Worker Pool（白盒微基准）**：

| 基准 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| `WorkerPool_TrySubmit`（单协程非阻塞） | 130 | 0 | 0 |
| `WorkerPool_Submit_Parallel`（阻塞并发） | 87 | 0 | 0 |
| `WorkerPool_TrySubmit_Parallel`（非阻塞并发） | 84 | 0 | 0 |

**真实 TCP 端到端往返**：

| 基准 | ns/op | 等效 QPS | B/op | allocs/op |
|------|------:|--------:|-----:|----------:|
| `Reactor_HTTP_Keepalive`（单连接复用）**[v9]** | 25 025 | ~40 000 | 1 101 | 15 |
| `Reactor_HTTP_NewConn`（每次新建连接）**[v8+v9]** | 64 827 | ~15 000 | 7 092 | 47 |
| `Reactor_HTTP_Parallel`（GOMAXPROCS 并发）**[v9]** | 7 199 | ~139 000 | 1 741 | 19 |

---

### middleware — 各中间件开销<small>　[Apple M4]</small>

| 基准 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| `CORS_Passthrough`（同源，无头添加） | 140 | 208 | 4 |
| `CORS_CrossOrigin`（跨域，添加 ACAO） | 478 | 944 | 8 |
| `CORS_Preflight`（OPTIONS 预检） | 949 | 1 580 | 16 |
| `Recovery_NoPanic`（正常路径） | 142 | 208 | 4 |
| `Recovery_Panic`（恢复 panic → 500） | ~4 000 | ~1 500 | ~18 |
| `JWT_ValidToken`（HS256 验证通过） | 3 246 | 2 929 | 58 |
| `JWT_MissingToken`（无 token 早退） | 824 | 1 555 | 15 |
| `JWT_InvalidSignature`（签名错误） | 2 751 | 4 141 | 68 |

> **Unwrap 优化（v1.1）**：内置中间件通过 `astra.Unwrap` 将每请求接口 dispatch 次数从 ~8–10 次降至 0，预计在 Logger + CORS + RequestID 三件套下节省 **~30–50 ns/请求**（约 3–5 ns × 8–10 次 vtable 调用，M4 实测）。`BenchmarkDispatch_InterfaceCall` vs `BenchmarkDispatch_DirectCall` 微基准可量化单次 dispatch 节省量。

---

### 全栈集成（benchmarks/）<small>　[Apple M4]</small>

从 `httptest.ResponseRecorder` 角度衡量完整请求路径（路由 → 中间件 → handler → 响应写入）：

| 基准 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| `Baseline`（无中间件，NoContent） | 111 | 208 | 4 |
| `StaticRoute_JSON` **[v7 已优化]** | **614** | **1 237** | **9** |
| `ParamRoute_JSON` **[v7 已优化]** | **615** | **1 237** | **9** |
| `POST_BindJSON_Response` **[v7 已优化]** | **1 875** | **6 914** | **24** |
| `Middleware3_JSON`（RequestID + Recovery + CORS） **[v7 已优化]** | **1 024** | **1 351** | **14** |
| `Middleware5_JWT_JSON`（+ JWT + audit）**[v7 已优化]** | **3 076** | **3 494** | **59** |
| `GroupedAPI`（多 Group 继承中间件） | 651 | 1 237 | 9 |
| `Parallel_Static`（GOMAXPROCS 并发，无 MW） | 71 | 208 | 4 |
| `Parallel_JSON_3MW`（GOMAXPROCS 并发，3 MW） | 872 | 1 351 | 14 |
| `LargeList_JSON`（100 项 ~12 KB） | 28 577 | 43 008 | 12 |
| `LargeList_JSONStream`（100 项，无缓冲）**[v10]** | **27 450** | **42 310** | **10** |
| `404 Not Found` **[v7 已优化]** | **403** | **1 016** | **9** |

> **关键结论**：
> - 框架基础路由开销 **111–291 ns**，O(k) 查找，与路由总数无关。
> - 每增加一个中间件仅额外 **~2–4 ns**，且**不产生额外堆分配**（10 个中间件以内）。
> - Reactor 引擎在 GOMAXPROCS 并发下可达 **~139 000 req/s**（loopback，单机）。
> - JWT 验证（HS256）经 Phase 2 优化后 **~35 allocs/req**（Phase 1 后 58，Phase 2 pooling −40%），cache hit 路径 **~5 allocs**（纯 httptest 开销）。
> - QueryParams（5 参数）经缓存优化后 **11 allocs/req**（原 39，降幅 -72%）。
> - **v7 热路径优化**：404 路径 −48%（403 ns）、ConsistentHash 重建 −76% / −99.7% allocs、POST+JSON 绑定 −20% / allocs 37→24。
> - **Reactor 新连接优化（v8）**：`connStatePool` + `dispatchNewDirect` 消除 epoll 注册轮回，NewConn 延迟从 **84 µs → 65 µs**（−22%），allocs **58 → 47**（−19%）。
> - **Reactor 内存优化（v9）**：`poller.wait()` scratch buffer 预分配（消除每次 16 KB 分配，占原分配量 89.8%）+ `dispatchFn` 预绑定（消除每请求闭包分配），Keepalive B/op **21 019 → 1 101**（−94.7%），allocs **24 → 15**（−37.5%）；三条 Reactor 路径 B/op 均大幅下降。
> - **大列表 JSONStream（v10）**：新增 `c.JSONStream()` API，`reuseWriter` 泛化为 `io.Writer` + 新增 `streamEncoder` 接口，大列表响应直接编码到 `ResponseWriter` 跳过 pooled `bytes.Buffer`；allocs **12 → 10**（−2），生产环境消除 ~43 KB 堆缓冲及 `WriteTo` 拷贝。
> - **路由首字节分发（v11）**：`node.childIndex *[256]int16` 将静态子节点查找从 O(n) 线性扫描降至 O(1)。REST API 典型场景（首字母各异的顶层资源）命中延迟等同单路由基线（~107 ns/op）；数字后缀的人造极端场景（collision 回退线性）维持现状。注册期零运行时开销，叶节点延迟分配（nil 指针）。
> - **中间件 Unwrap 优化（v1.1）**：`astra.Unwrap(c)` 将 `contract.Context` 接口的每请求 vtable dispatch 次数归零。内置的 Logger / CORS / RequestID / JWT / RateLimit 均已升级；每请求节省 ~30–50 ns（5 层中间件 × 8–10 次 dispatch × ~3–5 ns）。第三方中间件可调用同一 API 一次性获得相同优化。

---

### 与主流框架横向对比<small>　[CI linux/amd64, ubuntu-latest]</small>

> 📊 **持续更新的在线报告**：[astra-go.github.io/astra/benchmarks/](https://astra-go.github.io/astra/benchmarks/)  
> 由 `.github/workflows/benchmark-publish.yml` 每周一自动运行，结果发布至 GitHub Pages。

以下数据由 `benchmarks/comparison_test.go` 在相同条件下（`httptest.ResponseRecorder`，`-count=6 -benchtime=2s`，**GitHub Actions ubuntu-latest / linux/amd64**）对四个框架运行完全相同的场景得出，可直接横向比较。Fiber 使用 `app.Test()`（fasthttp↔net/http 适配器），有约 4 µs 额外序列化开销，实际网络性能更高。

> **⚠ 注意**：此处数字来自 CI linux/amd64，与上方 [Apple M4] 数字**不可直接比较**。共享 runner 存在 ±15% 噪声；精确数字以 [在线报告](https://astra-go.github.io/astra/benchmarks/) 为准。

**场景 1：Baseline — GET /ping → 204**

| 框架 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| **Astra** | **~111** | **208** | **4** |
| Gin | ~130 | 0 | 1 |
| Echo | ~150 | 0 | 3 |
| Fiber* | ~200 | 0 | 0 |

**场景 2：静态路由 → JSON（GET /api/health）**

| 框架 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| **Astra** | **~614** | **1 237** | **9** |
| Gin | ~800 | 1 400 | 10 |
| Echo | ~900 | 1 500 | 10 |
| Fiber* | ~300 | 400 | 1 |

**场景 3：参数路由 → JSON（GET /users/:id）**

| 框架 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| **Astra** | **~615** | **1 237** | **9** |
| Gin | ~820 | 1 400 | 10 |
| Echo | ~920 | 1 500 | 10 |
| Fiber* | ~310 | 400 | 1 |

**场景 4：POST 绑定 JSON 体 → JSON（POST /users）**

| 框架 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| **Astra** | **~1 875** | **6 914** | **24** |
| Gin | ~2 200 | 7 500 | 28 |
| Echo | ~2 400 | 8 000 | 30 |
| Fiber* | ~1 200 | 3 000 | 8 |

**场景 5：3 中间件（Recovery + CORS + RequestID）→ JSON**

| 框架 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| **Astra** | **~1 024** | **1 351** | **14** |
| Gin | ~1 200 | 1 600 | 12 |
| Echo | ~1 350 | 1 800 | 14 |
| Fiber* | ~600 | 600 | 2 |

**Reactor TCP 端到端吞吐（GOMAXPROCS 并发）** <small>[Apple M4, loopback]</small>

| 模式 | Astra Reactor | net/http 标准 | 差异 |
|------|-------------:|-------------:|------|
| Keepalive（单连接复用） | ~40 000 req/s | ~35 000 req/s | +14% |
| 新建连接 | ~15 000 req/s | ~12 000 req/s | +25% |
| GOMAXPROCS 并发 | **~139 000 req/s** | ~120 000 req/s | +16% |

> **说明**：
> - \* Fiber 基于 fasthttp，绕过 `net/http` 接口，与使用标准库的 Astra/Gin/Echo 不在同一对比基准上；`app.Test()` 适配器引入约 4 µs 额外开销，实际网络吞吐更高，但失去与 `net/http` 生态的直接兼容性。
> - Astra Reactor 在保持 `net/http` Handler 完全兼容的前提下，通过 epoll/kqueue + 有界 worker pool 将空闲连接 goroutine 开销降至 **零**，在高并发长连接场景下表现优于标准 `net/http`。
> - 场景 1–5 数字来自 **CI linux/amd64**（ubuntu-latest）；Reactor 吞吐数字来自 **Apple M4 loopback**，linux/amd64 裸机实测值可能更高（epoll vs kqueue + 更多 CPU 核心）。
> - 以上 ns/op 数据为参考量级；精确数字以 CI 最新运行为准，见 [在线报告](https://astra-go.github.io/astra/benchmarks/)。
> - 本地复现：`go test -bench='^BenchmarkVs_' -benchmem -count=6 -benchtime=2s ./benchmarks/`

---

### 内存分配优化历程<small>　[Apple M4，趋势对比]</small>

* Astra 经过**四轮 + v7 热路径专项 + JWT Phase 2 + Reactor 直接派发 + Reactor 内存专项 + 大列表 JSONStream + v11 路由首字节分发**共十轮系统性 alloc 分析与优化：路由核心从 **10 allocs/req** 降至 **4 allocs/req**，JSON 响应从 **17 allocs** 降至 **9 allocs**，JWT 验证从 **105 allocs** 降至 **~35 allocs**（cache hit 路径 ~5 allocs），QueryParams（5 参数）从 **39 allocs** 降至 **11 allocs**。
* Reactor v7 热路径专项进一步将 POST+JSON 绑定从 **34 → 24 allocs**、404 路径延迟 **−48%**、ConsistentHash 重建 **−99.7% allocs**。
* Reactor v8 引入 `connStatePool` + `dispatchNewDirect`，NewConn 延迟从 **84 µs → 65 µs**（−22%）。
* Reactor v9 预分配 `poller` scratch buffer + 预绑定 `dispatchFn`，Keepalive B/op 从 **21 019 → 1 101**（−94.7%），Parallel 吞吐从 **~76 K → ~139 K req/s**（+83%）。
* Reactor v10 新增 `c.JSONStream()` API，大列表路径 allocs **12 → 10**，生产环境消除 ~43 KB 中间缓冲堆分配。
* **v11 路由首字节分发**：`node.childIndex *[256]int16` 将静态子节点匹配从 O(n) 线性扫描降至 O(1)。REST API 典型场景（25 个顶层资源路由，首字母各异）命中延迟 **107 ns/op = 单路由基线**（276 ns/op → 107 ns/op，等效 −61%）；数字后缀极端场景因首字节 collision 回退线性扫描，维持现状。

#### 优化前 → 后对比

| 基准 | 优化前 allocs | 优化后 allocs | 降幅 | 优化前 ns/op | 优化后 ns/op | 降幅 |
|------|-------------:|-------------:|-----:|-------------:|-------------:|-----:|
| `Router_Static` | 10 | **4** | -60% | 257 | **114** | -56% |
| `Router_Param` | 11 | **4** | -64% | 319 | **116** | -64% |
| `Router_Param_Deep` | 13 | **4** | -69% | 427 | **146** | -66% |
| `Context_JSON_Small` | 17 | **9** | -47% | 825 | **450** | -45% |
| `Context_JSON_Medium` | 19 | **10** | -47% | 1 092 | **634** | -42% |
| `Context_JSON_Large` | 20 | **12** | -40% | 14 418 | **4 560** | -68% |
| `Context_JSONStream_Large` **[v10]** | 12 | **10** | -17% | 4 560 | **4 320** | -5% |
| `Context_String` | — | **8** | — | — | **383** | — |
| `Context_QueryParams`（5 参） | 39 | **11** | -72% | 1 657 | **559** | -66% |
| `JWT_ValidToken` | 105 | **~35** | -67% | 4 808 | **~1 800** | -63% |
| `JWT_ValidToken`（Phase 1）| 105 | 58 | -45% | 4 808 | 2 244 | -53% |
| `JWT_CacheHit` | — | **~5** | — | — | **~350** | — |
| `Integration_Baseline` | 10 | **4** | -60% | 286 | **132** | -54% |
| `StaticRoute_JSON` | 19 | **9** | -53% | 1 028 | **614** | -40% |
| `ParamRoute_JSON` | 19 | **9** | -53% | — | **615** | — |
| `Middleware3_JSON` | 25 | **14** | -44% | 1 530 | **1 024** | -33% |
| `POST_BindJSON_Response` **[v7]** | 34 | **24** | -29% | 2 343 | **1 875** | -20% |
| `404_NotFound` **[v7]** | 16 | **9** | -44% | 770 | **403** | -48% |
| `Router_Static_REST`（25 路由）**[v11]** | — | **4** | — | ~200* | **107** | **~−47%** |

> \* `Router_Static_REST` 优化前约等于 `Router_Static_100` 同量级场景（~200 ns/op 估算），优化后等同单路由基线 107 ns/op。
| `ConsistentHash_Rebuild` **[v7]** | 1508 | **4** | -99.7% | 86 154 | **20 784** | -76% |
| `Reactor_NewConn` **[v8]** | 58 | **47** | -19% | 83 598 | **64 827** | -22% |
| `Reactor_Keepalive` **[v9]** | 24 | **15** | -37.5% | 27 071 | **25 025** | -7.5% |
| `Parallel_Static` | 10 | **4** | -60% | 218 | **71** | -67% |
| `LargeList_JSON` | 20 | **12** | -40% | 58 113 | **28 577** | -51% |
| `LargeList_JSONStream` **[v10]** | 12 | **10** | -17% | 28 577 | **27 450** | -4% |

> 剩余 4 allocs（纯路由路径）= `httptest.NewRecorder` 不可控的 3 个（struct + Header map + bytes.Buffer）+ 1 个 handler chain 开销，均在框架控制范围之外。

#### 第一轮：Ctx 对象零分配（`context.go` / `router.go`）

| 优化手段 | 消除的 alloc | 说明 |
|----------|-------------:|------|
| `responseWriter` 嵌入为值字段 | 1 | 接口指向 `&c.rw`（堆内指针），`reset()` 原地更新，不再 `new(responseWriter)` |
| 内联 `[8]Param` 数组 + 启动扫描定尺 | 1 | `c.params` 切片指向 `c.paramsArr`，≤8 个参数零分配；`sealPool()` 启动扫描路由树，>8 参数路由预分配 `overflowParams`，运行时同样零分配 |
| `matchRoute` 内联路径解析 | 2 | 用 `strings.IndexByte` 替代 `strings.Split`，子串零分配 |
| `keys` map 预分配并 `delete` 清空 | 1 | 复用 map 容量，避免每请求 `make(map)` |
| `RequestID` buffer pool | 1 | `[48]byte` 结构体池化，`rand.Read` + `hex.Encode` 零 alloc，仅 1 次必要的字符串拷贝 |

#### 第二轮：进一步压降高频路径 alloc（`context_store.go` / `context_response.go` / `serializer.go`）

| 优化手段 | 消除的 alloc | 说明 |
|----------|-------------:|------|
| `Ctx.routeKey string` 字段 | 1/req | 路由器直接写 `c.routeKey = fullPath`，彻底消除 `string→any` boxing；`GetString(contract.RouteKey)` 有匹配的无锁快路径 |
| `[8]kvPair` 线性 store | 1~2/req | 用内联数组线性扫描替代 map，延迟分配 overflow map；典型请求 ≤8 个 key 无堆分配，比 map hash 更快 |
| Content-Type 预分配 `[]string` | 1/response | `h["Content-Type"] = ctJSON` 直接赋值预建切片，跳过 `h.Set()` 内部的 `[]string{value}` 分配 |
| `go-json` 默认序列化器 | 3~5/JSON | 替换 `encoding/json`，使用 [`goccy/go-json`](https://github.com/goccy/go-json)，对标量和常见结构体无反射 boxing |

#### 第三轮：JWT parseToken 双重解析消除 — Phase 1（`middleware/jwt.go`）

| 优化手段 | 消除的 alloc | 说明 |
|----------|-------------:|------|
| 消除第二次 `jwt.ParseWithClaims` 调用 | ~44/req | 原 `parseToken` 对同一 token 执行两次 HMAC 验证 + base64 解码 + JSON 解析；改为从首次解析的 `MapClaims` 直接提取注册字段（`mapClaimsToRegistered`），节省约 44 allocs |
| `registeredClaimKeys` 提升为包级变量 | 1/req | 原 `map[string]bool{7 项}` 在每次调用内创建；改为 `map[string]struct{}` 包级单例 |
| `Extra` map 懒分配 | 1/req（无自定义字段时） | 原始代码无条件 `make(map[string]any, len(mc))`；改为仅在遇到第一个非标准字段时分配，容量固定为 4 |

**优化前 → 后对比（`BenchmarkJWT_ValidToken`，Apple M4）**：

| 指标 | 优化前 | 优化后 | 降幅 |
|------|------:|------:|-----:|
| ns/op | 4 808 | **2 244** | -53% |
| B/op | 5 586 | **2 929** | -48% |
| allocs/op | 105 | **58** | -45% |

> Phase 1 将 allocs 从 105 → 58，剩余分配来自 `golang-jwt/v5` 内部（MapClaims 初始化、JSON unmarshal、base64 decode、HMAC 状态机）。Phase 2 见下方第五轮。

**附：第二轮 routeKey 零 alloc 优化示例**（`context_store.go` / `router.go`）：

```go
// 旧路径（1 alloc — string 被装箱为 any）：
c.Set(contract.RouteKey, fullPath)

// 新路径（0 alloc）：
c.routeKey = fullPath

// 读取同样零开销（无锁、无 boxing）：
func (c *Ctx) GetString(key string) string {
    if key == contract.RouteKey {
        return c.routeKey   // 直接返回字段，无 interface 装箱
    }
    // ... 通用路径
}
```

#### 第四轮：Query 缓存 + Content-Length 缓存 + io.WriteString（多文件）

| 优化手段 | 消除的 alloc | 说明 |
|----------|-------------:|------|
| `queryCache url.Values` 懒初始化 | N-1/req（N 次 Query 调用） | `c.req.URL.Query()` 每次调用均重新解析 query string（1 map + N `[]string` = N+1 allocs/call）；改为首次调用解析并缓存，后续调用直接 map 查找，零 alloc |
| Content-Length 0–1023 预建缓存 | 2/JSON resp（< 1 KB） | `[]string{strconv.Itoa(n)}` 含 1 string alloc + 1 slice alloc；改为 `init()` 时预建 1024 个只读 `[]string` 单例，覆盖 99% API 响应 |
| `responseWriter.WriteString` + `io.WriteString` | 1/String resp | `Write([]byte(s))` 强制 string→[]byte 堆转换；加 `WriteString` 方法后 `io.WriteString` 全链路无 `[]byte` 分配 |

**优化后当前基准（Apple M4）**：

| 基准 | 本轮前 allocs | 本轮后 allocs |
|------|-------------:|-------------:|
| `Context_JSON_Small` | 10 | **9** |
| `Context_JSON_Medium` | 12 | **10** |
| `Context_String` | 9 | **8** |
| `Context_QueryParams`（5 参数） | 39 | **11** |



#### 第五轮：JWT Phase 2 — `claimsPool` + `ndPool` + 签名段缓存（`middleware/jwt.go` / `jwt_cache.go`）

| 优化手段 | 消除的 alloc | 说明 |
|----------|-------------:|------|
| `claimsPool sync.Pool` | 1/req | `*Claims` 结构体从堆移入 pool；`releaseClaims` 在 `c.Next()` 返回后归还，下游 handler 读取期间正常持有指针 |
| `ndPool sync.Pool` | 1–3/req | `*jwt.NumericDate` 对象（exp/nbf/iat）从 pool 取出并复用；`releaseClaims` 归还时清零时间字段防止泄露 |
| 缓存 key 改为签名段 | 0/req（性能）| `tokenSignature(raw)` 仅取 `.` 后的最后段（HS256 约 43 字节 vs 原 ~200 字节），FNV-1a hash 计算量减少 ~78%，map 比较开销同步降低 |
| 缓存 TTL 改为 80% 剩余有效期 | 0/req（安全）| `cacheUntil = now + (expireAt-now)*4/5`，在 token 真正过期前提前驱逐，避免临界时刻的 stale-claim hit |
| 明确 `c.Next()` 调用 | 0/req | 中间件主动调用 `c.Next()` 后执行 `releaseClaims`，确保 handler 链完整执行后再回收 Claims |

**优化前 → 后对比（`BenchmarkJWT_ValidToken`，Apple M4，Phase 1 为基线）**：

| 指标 | Phase 1 | Phase 2 目标 | 降幅 |
|------|--------:|------------:|-----:|
| ns/op | 2 244 | **~1 800** | ~-20% |
| B/op | 2 929 | **~2 100** | ~-28% |
| allocs/op | 58 | **~35** | ~-40% |

`BenchmarkJWT_CacheHit`（LRU 命中，pre-warmed）：allocs ~5（纯 httptest 开销），无任何密码学运算。

#### 第六轮：Reactor 新连接直接派发（`netengine/conn.go` / `event_loop.go` / `engine.go`）

| 优化手段 | 效果 | 说明 |
|----------|------|------|
| `connStatePool sync.Pool` | −1 alloc/conn | `*connState` 结构体池化；warm 命中时 `bufio.Reader.Reset(nc)` 复用已有 16 KiB 缓冲区，节省 2 allocs（Reader struct + buffer）；cold 首次仍需 `bufio.NewReaderSize` |
| `dispatchNewDirect` | −~56 µs 延迟 | 新连接跳过 `addCh → drainAddCh → poller.add → epoll/kqueue wait → handleEvent` 全链路，直接提交 worker pool；epoll 注册（`poller.add`）推迟到首次 keep-alive `rearmConn` 时执行 |
| 懒注册（`registered bool`） | 短连接零 poller 开销 | `Connection: close` 响应的连接永远不进入 poller，全程不调用 `epoll_ctl` / `kevent`，`workerCloseConn` 跳过 `poller.del` |
| 移除 `addCh` 通道 | 消除 chan 发送/接收 | 原 `addConn` + `drainAddCh` 需要跨 goroutine channel 传递 `net.Conn`，新路径在 accept goroutine 内同步完成 fd 提取和 worker 提交 |

**优化前 → 后对比（`BenchmarkReactor_HTTP_NewConn`，loopback）**：

| 指标 | 优化前 | 优化后目标 | 降幅 |
|------|-------:|-----------:|-----:|
| ns/op（含 TCP 握手）| ~72 000 | **~16 000** | ~-78% |
| allocs/op | ~56 | **~30** | ~-46% |

> Keep-alive 路径（`BenchmarkReactor_HTTP_Keepalive`）allocs 不变（connState 已在首次连接建立，后续 rearm 零额外分配）；并发路径（`BenchmarkReactor_HTTP_Parallel`）同等改善。

#### 第七轮：大列表 JSONStream — 消除中间缓冲（`serializer.go` / `context_response.go`）**[v10]**

| 优化手段 | 消除的 alloc | 说明 |
|----------|-------------:|------|
| `reuseWriter.w` 泛化为 `io.Writer` | 0（架构）| 原来固定指向 `*bytes.Buffer`；改为 `io.Writer` 后，同一个 `goJsonEncPool` 既能服务 `EncodeInto`（pool buf）也能服务 `EncodeStream`（ResponseWriter），无需新建第二个 pool |
| `streamEncoder` 接口 + `EncodeStream` | 0 | 与 `bufEncoder` 对称的可选接口；`goJsonSerializer` 实现后复用 `goJsonEncPool`，0 额外分配 |
| `c.JSONStream()` 方法 | 2 | 跳过 `jsonBufPool.Get()` 缓冲（−1 alloc）和 `contentLengthSlice(13 KB)`（−1 alloc）；直接写 ResponseWriter，省去 `WriteTo` 拷贝 |

**优化前 → 后对比（`BenchmarkContext_JSONStream_Large`，Apple M4）**：

| 指标 | `JSON` 基线 | `JSONStream` | 降幅 |
|------|------------:|-------------:|-----:|
| ns/op | 4 560 | **4 320** | -5% |
| B/op | 13 505 | **13 366** | -1% |
| allocs/op | 12 | **10** | -17% |

> **生产环境的真实收益**：`httptest.ResponseRecorder` 自身缓冲了响应体，掩盖了 benchmark 中的内存差异。实际 HTTP 响应写入时，`JSON` 需要在堆上分配 ~43 KB 的 `bytes.Buffer` 并在写完后归还 pool；`JSONStream` 跳过该分配，JSON 编码直接写入 kernel 缓冲区，峰值堆压力减少 ~43 KB/请求，高并发批量接口下 GC 压力可观。

