# 性能调优指南

## 基准选择

| 场景 | 推荐引擎 | 原因 |
|------|----------|------|
| 短请求、高 QPS（API 网关） | `app.Run()` 标准引擎 | net/http 调度器成熟稳定 |
| 长连接、高并发（WebSocket、SSE） | `app.RunReactor()` | O(1) goroutine，低内存 |
| 极低延迟、需 SO_REUSEPORT | Reactor + ListenReusePort | 多进程/容器同端口监听 |
| HTTPS 外网服务 | `app.RunTLS()` | 标准 TLS 1.3 |
| 移动端 / 弱网 | `app.RunQUIC()` | QUIC 0-RTT，丢包恢复 |

---

## Reactor 引擎调优

```go
engine, _ := netengine.New(app, netengine.ReactorConfig{
    // 事件循环数 = CPU 核心数（IO 密集型）
    NumLoops: runtime.NumCPU(),

    // Worker 池 = CPU * 4（CPU 密集型 handler）
    // Worker 池 = CPU * 8~16（IO 等待型 handler，如数据库查询）
    WorkerPoolSize: runtime.NumCPU() * 8,

    // 连接队列缓冲：并发连接数 / NumLoops
    ConnChannelBuffer: 256,
})
```

**监控指标**：

```go
// Prometheus 指标（需配合 middleware.Prometheus）
astra_active_conns          // 当前活跃连接
astra_worker_queue_depth    // Worker 队列积压
astra_request_duration_ms   // P50/P95/P99 延迟
```

---

## 内存分配优化

Astra 框架内部对 `Context` 使用 `sync.Pool` 复用，
handler 层面减少分配的方法：

```go
// ✅ 直接写入 writer，零拷贝
c.Writer().Write(precomputedBytes)

// ✅ 使用 c.String 而非 fmt.Sprintf + c.JSON
c.String(200, "hello %s", name)

// ⚠️ 避免在热路径上分配 map
// 不推荐：
c.JSON(200, map[string]any{"key": value})

// 推荐：使用预定义结构体
type Response struct { Key string `json:"key"` }
c.JSON(200, Response{Key: value})
```

---

## 路由性能

- 路由树查找是 `O(k)`（k = 路径段数），与路由总数无关。
- 静态路由优先于参数路由，参数路由优先于通配路由。
- 避免在高频路径上注册过多中间件（每个中间件增加一次函数调用）。

---

## 连接池配置

```go
// 数据库连接池（避免突发 goroutine 风暴）
db, _ := orm.Open(orm.Config{
    DSN:             dsn,
    MaxOpenConns:    runtime.NumCPU() * 4,
    MaxIdleConns:    runtime.NumCPU(),
    ConnMaxLifetime: 30 * time.Minute,
    ConnMaxIdleTime: 5 * time.Minute,
})

// Redis 连接池
rdb := redis.NewClient(&redis.Options{
    PoolSize:        runtime.NumCPU() * 4,
    MinIdleConns:    runtime.NumCPU(),
    ConnMaxIdleTime: 5 * time.Minute,
})
```

---

## 压缩策略

```go
app.Use(middleware.Gzip(middleware.GzipConfig{
    Level:     gzip.BestSpeed,  // 速度优先（level 1），而非最大压缩（level 9）
    MinLength: 1024,            // 小于 1KB 的响应不压缩（压缩开销 > 收益）
    SkipPaths: []string{
        "/metrics",   // Prometheus 指标不压缩（已是文本）
        "/ws",        // WebSocket 不压缩
    },
}))
```

---

## pprof 分析

```go
// 仅在管理端口暴露
admin := astra.New()
admin.Use(middleware.Pprof("/debug/pprof"))
go admin.Run(":6060")
```

```bash
# CPU profile（30 秒）
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 内存 heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# goroutine 泄漏检测
curl http://localhost:6060/debug/pprof/goroutine?debug=1 | head -50
```

---

## 基准测试结果

以下数据来自 `make bench-all`，测试环境 **Apple M4 · Go 1.22 · 3 轮 × 2s/轮（loopback）**。

运行方式：

```bash
make bench-all

# 与基线对比
go install golang.org/x/perf/cmd/benchstat@latest
make bench-save-baseline
# … 修改 …
make bench-compare
```

---

### 路由 & 中间件链（根包）

| 基准 | ns/op | B/op | allocs/op | 说明 |
|------|------:|-----:|----------:|------|
| `Router_Static` | 114 | 208 | 4 | 静态路径命中 |
| `Router_Static_100` | 290 | 208 | 4 | 100 路由树，O(k) 不随路由数增长 |
| `Router_Param` | 116 | 208 | 4 | `:id` 单参数 |
| `Router_Param_Deep` | 146 | 208 | 4 | 3 段参数路径 |
| `Router_Regex` | 154 | 208 | 4 | `{id:[0-9]+}` 正则约束 |
| `Router_Wildcard` | 120 | 208 | 4 | `*path` 通配 |
| `Router_NotFound` | 765 | 1 567 | 16 | 未匹配路由 |

**中间件链扩展代价**（每增加 1 个 handler 仅 ~4 ns，**零额外堆分配**）：

| handler 数 | ns/op | B/op | allocs/op |
|-----------:|------:|-----:|----------:|
| 0（纯 handler） | 113 | 208 | 4 |
| 1 | 114 | 208 | 4 |
| 3 | 119 | 208 | 4 |
| 5 | 122 | 208 | 4 |
| 10 | 136 | 208 | 4 |
| Abort（链中断） | 123 | 216 | 5 |

---

### Context 响应写入

| 基准 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| `Context_JSON_Small`（~120 B） | 450 | 1 028 | 9 |
| `Context_JSON_Medium`（~350 B） | 634 | 1 381 | 10 |
| `Context_JSON_Large`（100 项 ~12 KB） | 6 299 | 25 953 | 13 |
| `Context_String` | 383 | 992 | 8 |
| `Context_QueryParams`（5 个参数） | 559 | 688 | 11 |
| `ServeHTTP_Parallel_Static` | 91 | 208 | 4 |
| `ServeHTTP_Parallel_JSON` | 450 | 1 284 | 10 |

---

### Reactor 引擎（netengine）

**Worker Pool 微基准（白盒，直接访问内部类型）**：

| 基准 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| `WorkerPool_TrySubmit`（单协程，非阻塞） | 130 | 0 | 0 |
| `WorkerPool_Submit_Parallel`（阻塞并发） | 87 | 0 | 0 |
| `WorkerPool_TrySubmit_Parallel`（非阻塞并发） | 84 | 0 | 0 |

**真实 TCP 端到端往返**（reactor accept → event loop → worker → handler → response）：

| 基准 | ns/op | 等效 QPS | B/op | allocs/op |
|------|------:|--------:|-----:|----------:|
| `Reactor_HTTP_Keepalive`（单连接复用，热路径） | 27 071 | ~37 000 | 21 019 | 24 |
| `Reactor_HTTP_NewConn`（每次新建连接，冷路径） | 83 598 | ~12 000 | 62 481 | 58 |
| `Reactor_HTTP_Parallel`（GOMAXPROCS 并发） | 13 098 | ~76 000 | 8 656 | 23 |

> 新建连接路径（83 µs）包含 TCP 三次握手（loopback ~10 µs）和 reactor accept 开销。
> 并发持久连接路径（13 µs / ~76k QPS）是生产场景的参考值。

---

### 各中间件开销

| 基准 | ns/op | B/op | allocs/op | 说明 |
|------|------:|-----:|----------:|------|
| `CORS_Passthrough` | 140 | 208 | 4 | 同源请求，无头部添加 |
| `CORS_CrossOrigin` | 478 | 944 | 8 | 跨域，写 ACAO 头 |
| `CORS_Preflight` | 949 | 1 580 | 16 | OPTIONS 预检，返回完整 allow 列表 |
| `Recovery_NoPanic` | 142 | 208 | 4 | 正常路径，接近零开销 |
| `Recovery_Panic` | ~4 000 | ~1 500 | ~18 | panic → recover → 500 完整路径 |
| `JWT_ValidToken` | 3 246 | 2 929 | 58 | HS256 验证通过（双重解析已消除） |
| `JWT_MissingToken` | 824 | 1 555 | 15 | 无 token，早退 401，无密码学运算 |
| `JWT_InvalidSignature` | 2 751 | 4 141 | 68 | 签名错误，完整解析 + 验证失败 |

---

### 全栈集成（完整请求路径）

`httptest.ResponseRecorder` 测量：路由 → 全局中间件 → 路由中间件 → handler → 响应写入。

| 基准 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| `Baseline`（无中间件，NoContent） | 132 | 208 | 4 |
| `StaticRoute_JSON` | 1 033 | 1 412 | 10 |
| `ParamRoute_JSON` | 1 053 | 1 413 | 10 |
| `POST_BindJSON_Response`（JSON 绑定 + 响应） | 2 559 | 8 067 | 37 |
| `Middleware3_JSON`（RequestID + Recovery + CORS） | 1 024 | 1 526 | 15 |
| `Middleware5_JWT_JSON`（+ JWT + audit stub） | 3 091 | 4 153 | 65 |
| `GroupedAPI`（多 Group 中间件继承） | 640 | 1 412 | 10 |
| `Parallel_Static`（GOMAXPROCS 并发，无 MW） | 77 | 208 | 4 |
| `Parallel_JSON_3MW`（GOMAXPROCS 并发，3 MW） | 1 053 | 1 528 | 15 |
| `LargeList_JSON`（200 项 ~30 KB） | 35 207 | 85 235 | 13 |

---

### 结论与调优建议

| 结论 | 调优建议 |
|------|----------|
| 路由查找 111–282 ns，O(k) 与路由总数无关 | 不必担心大型应用路由注册数量 |
| 每增加 1 个中间件 ~4 ns，零额外分配 | 10 个中间件以内对延迟影响可忽略 |
| JWT HS256 验证单次 ~2.2 µs、58 allocs（Phase 1 优化后） | 剩余 allocs 在库内部；高 QPS 场景进一步考虑 LRU 缓存或 PASETO |
| Reactor 并发持久连接 ~76k QPS | 短连接/无连接池客户端会大幅降低吞吐 |
| 大 JSON 列表（30 KB）13 allocs | 分页限制单次列表大小，使用流式 JSON |

---

### 优化技术摘要

框架内部通过三轮系统性 alloc 分析将路由核心从 **10 allocs/req** 降至 **4 allocs/req**，JSON 响应从 **17 allocs** 降至 **10 allocs**，JWT 验证从 **105 allocs** 降至 **58 allocs**：

| 手段 | 消除 alloc | 所在文件 |
|------|:----------:|---------|
| `responseWriter` 嵌入为值字段，接口指向堆内指针 | 1 | `context.go` |
| 内联 `[8]Param` 数组，params 切片无堆分配 | 1 | `context.go` |
| `matchRoute` 用 `IndexByte` 内联路径解析 | 2 | `router.go` |
| `routeKey string` 字段，消除 `string→any` boxing | 1/req | `context.go`, `router.go` |
| `[8]kvPair` 线性 store 替代 map | 1~2/req | `context_store.go` |
| Content-Type 预分配 `[]string` 单例 | 1/resp | `context_response.go` |
| `goccy/go-json` 替换 `encoding/json` | 3~5/JSON | `serializer.go` |
| `RequestID` buffer pool（`[48]byte`） | 1/req | `middleware/requestid.go` |
| 消除 JWT 第二次 `ParseWithClaims`（双重 HMAC+JSON） | ~44/req | `middleware/jwt.go` |
| `registeredClaimKeys` 提升为包级变量 | 1/req | `middleware/jwt.go` |
| JWT `Extra` map 懒分配 | 1/req | `middleware/jwt.go` |
| `queryCache url.Values` 懒初始化，Query() 复用解析结果 | N-1/req（N次查询时） | `context.go`, `context_request.go` |
| Content-Length 0–1023 预建 `[]string` 缓存 | 2/JSON resp（< 1 KB） | `context_response.go` |
| `responseWriter.WriteString` + `io.WriteString` | 1/String resp | `response_writer.go`, `context_response.go` |

