## Reactor 网络引擎（netengine）

`netengine` 是 Astra 内置的高性能 Reactor 模式网络引擎，参考字节跳动 Hertz/Netpoll 的设计思路，
**直接调用 `golang.org/x/sys/unix` 的 epoll（Linux）和 kqueue（macOS/BSD）**，
绕过 `net/http` 的"每连接一 goroutine"模型，用少量 goroutine 支撑海量并发连接。

### 架构原理

```
                          ┌─────────────────────────────────────────────────────┐
                          │              Reactor 网络引擎（netengine）            │
                          │                                                      │
  客户端连接               │  Accept 循环（永不阻塞）                               │
  ─────────►  net.Listener ──round-robin──► [Loop-0] [Loop-1] … [Loop-N-1]    │
                          │                    │  每个 Loop 持有                  │
                          │                    │  一个 epoll/kqueue 实例          │
                          │                    │                                 │
                          │   空闲连接在此零 goroutine 挂起                        │
                          │   （FD 注册在 epoll/kqueue，不占 goroutine 栈）         │
                          │                    │ FD 可读（ONESHOT/EV_DISPATCH）    │
                          │                    ▼                                 │
                          │         有界 Worker Pool（P goroutine）               │
                          │         默认 P = 4 × GOMAXPROCS                      │
                          │                    │                                 │
                          │         [TLS] Handshake（5s 超时，worker 内完成）      │
                          │                    │                                 │
                          │         ┌──────────┴──────────┐                     │
                          │         │ ALPN = h2            │ ALPN = h1 / 非TLS  │
                          │         ▼                      ▼                    │
                          │  go http2.Server.ServeConn   handler.ServeHTTP      │
                          │  （goroutine 接管，worker 归池）← 标准 http.Handler    │
                          └─────────────────────────────────────────────────────┘
```

**关键设计决策**：

| 机制 | 实现细节 | 解决的问题 |
|------|----------|-----------|
| `EPOLLONESHOT` / `EV_DISPATCH` | 事件触发后 FD 自动禁用，处理完再 `mod()` 重新启用 | 防止同一 FD 被多个 worker 并发读取 |
| connState 所有权协议 | 新连接直接标记 `stateDispatched` 入 `e.conns`；keep-alive 时由 `rearmConn` 注册 poller 并置 `stateIdle`；关闭时 CAS 至 `stateClosed` | 无锁并发安全，三态 CAS 消除竞争 |
| 有界 worker pool | `submit()` 在队列满时产生背压（阻塞 event loop 提交） | 防止慢业务代码创建无限 goroutine |
| 直接派发新连接（`dispatchNewDirect`） | 新连接跳过 addCh → poller.add → epoll 事件轮回，直接提交 worker；首次 keep-alive rearm 时才调用 `poller.add`，后续 rearm 调用 `poller.mod` | 消除新连接的 epoll/kqueue 注册延迟，短连接路径不进入 poller |
| `connStatePool` | `sync.Pool` 复用 `*connState`；`bufio.Reader.Reset(nc)` 复用 16 KiB 缓冲区 | 新连接开销 −3 allocs（struct + bufio 缓冲区）；keep-alive 路径 0 额外 allocs |
| `bufio.Reader` 复用 | 每连接一个 `bufio.Reader`，跨 Keep-Alive 请求复用 | 正确处理 HTTP 管道化，避免重复缓冲区分配 |
| 直接响应序列化 | `statusLineCache[600]string` 预计算状态行，`strconv.AppendInt` 栈分配写 Content-Length，`respBufWriterPool` 复用 `bufio.Writer`，直接写 `[]byte` 体，消除 `http.Response` / `io.NopCloser` / `strings.NewReader` 3 处中间对象 | 比 `http.Response.Write` 减少约 5 次堆分配/请求 |
| `SO_REUSEPORT` 支持 | `ListenReusePort(network, addr)` 创建可多进程共享的监听器 | 支持 Prefork 模式，每进程堆更小，STW pause 更短 |

### API 使用

```go
// RunReactor：等价于 Run，但使用 Reactor 引擎
// 不支持 epoll/kqueue 的平台（如 Windows）自动回退到标准 net/http
if err := app.RunReactor(":8080"); err != nil {
    log.Fatal(err)
}

// RunReactorTLS：TLS 版本，自动在 tls.Config 中注入 NextProtos: ["h2", "http/1.1"]
// 无需额外配置，框架自动完成 ALPN 协商：h2 连接由 net/http http2 包接管，h1 走 Reactor handler
if err := app.RunReactorTLS(":443", "cert.pem", "key.pem"); err != nil {
    log.Fatal(err)
}

// RunReactorHandler：允许在 App 外层包裹标准 http.Handler 中间件（CORS、限流等）后再交给 Reactor
if err := app.RunReactorHandler(":8080", corsMiddleware(app)); err != nil {
    log.Fatal(err)
}

// RunReactorTLSHandler：TLS + 自定义 handler 版本
if err := app.RunReactorTLSHandler(":443", "cert.pem", "key.pem", myHandler); err != nil {
    log.Fatal(err)
}
```

### 兼容性边界

Reactor 引擎绕过了 `net/http` 的连接管理，以下特性**不可用**：

| 特性 | 状态 | 替代方案 |
|------|------|---------|
| `http.Hijacker`（WebSocket 升级） | ❌ 不支持 | 使用 `app.RunServer` 标准模式 |
| `http.Flusher` / `http.ResponseController`（SSE、流式响应） | ❌ 响应全量缓冲后才写入 | 使用 `app.RunServer` 标准模式 |
| `http2.ConfigureServer`（自定义 H2 参数） | ❌ 无 `*http.Server` 实例 | `RunServer` + `http2.ConfigureServer`，或 TLS 模式下直接修改 `http2.Server` 字段 |
| 依赖 `*http.Server` 内部字段的第三方中间件 | ❌ 不支持 | 使用 `app.RunServer` 标准模式 |
| 仅操作请求/响应头、体的普通 Handler 中间件 | ✅ 完全兼容 | 通过 `RunReactorHandler` 包裹即可 |

需要完整 `net/http` 兼容时，切换到 `RunServer` 一行即可，API 完全兼容：

```go
// 显式使用标准 net/http，获得 Hijacker / Flusher / http2.ConfigureServer 全部能力
srv := &http.Server{Addr: ":8080", Handler: app}
http2.ConfigureServer(srv, nil) // 完整 H2 控制
app.RunServer(srv)
```

### Prefork 模式（SO_REUSEPORT）

当 P99 延迟需要压到 1 ms 以下，或单进程 GC 堆过大时，可用 `ListenReusePort` 实现
多进程 Prefork 部署——OS 内核将连接均匀分发给每个 worker 进程，每进程堆更小，
GC stop-the-world 停顿影响面缩小：

```go
// 在每个 worker 进程（通过 os/exec 或 syscall.Fork 启动）中:
ln, err := netengine.ListenReusePort("tcp", ":8080")
if err != nil {
    // SO_REUSEPORT 不可用（Windows），降级为普通监听
    ln, err = net.Listen("tcp", ":8080")
}
if err != nil {
    log.Fatal(err)
}

engine, _ := netengine.New(app, netengine.ReactorConfig{})
engine.Serve(ln)  // 每个进程独立运行完整 Engine，业务代码不变
```

> **何时需要 Prefork**：现代 Go（1.14+）的 GC STW pause 通常 ≤ 500 µs，多数服务
> 直接 `RunReactor` 即可。仅当 P99 延迟要求极严（< 1 ms）且业务确认 GC 是瓶颈时
> 才需要 Prefork。

### 精细调优

```go
engine, err := netengine.New(app, netengine.ReactorConfig{
    NumLoops:       4,               // event loop 数量，默认 GOMAXPROCS
    WorkerPoolSize: 32,              // worker goroutine 上限，默认 4×GOMAXPROCS
    ReadBufferSize: 32 * 1024,       // per-conn 读缓冲，默认 16 KiB
    ReadTimeout:    15 * time.Second,
    WriteTimeout:   30 * time.Second,
    Logger:         slog.Default(),
})
if err != nil {
    // 平台不支持（Windows），使用标准 net/http
}
ln, _ := net.Listen("tcp", ":8080")
engine.Serve(ln)
```

### 与标准 net/http 的性能对比

| 场景 | net/http（Gin/Echo） | Astra RunReactor |
|------|----------------------|-----------------|
| 10k 空闲 Keep-Alive 连接 | ~10,000 goroutine（约 20–80 MB 栈） | ≤ N loops + P workers（约 50 goroutine） |
| goroutine 调度压力 | 随连接数线性增长 | 固定（仅随 worker 数增长） |
| 单连接延迟（低负载） | 相近（微秒级） | 相近（微秒级） |
| 吞吐量（短连接）| 基准值 | 相近（瓶颈在业务逻辑，非网络层） |
| 吞吐量（长连接 × 大并发）| GC 压力随 goroutine 数上升 | GC 压力稳定（goroutine 数量受控） |
| 平台支持 | 全平台 | Linux（epoll）、macOS/BSD（kqueue）；Windows 自动回退 |

> **适用场景**：API 网关、长连接服务、WebSocket 聚合器、连接数 >> 并发请求数的场景。
> 短连接、低并发服务使用标准 `Run` 即可，两者 API 完全一致。

---

