## 与主流框架设计对比

| 特性 | **Astra** | Gin | Echo | go-zero | Beego | Hertz | Fiber |
|------|-----------|-----|------|---------|-------|-------|-------|
| 路由算法 | 基数树 + 正则约束参数 | 基数树 | 基数树 | 基数树 | 正则 | 基数树 | 基数树 |
| Handler 签名 | `func(*Context) error` | `func(*Context)` | `func(*Context) error` | 代码生成 | `func()` | `func(*Context) error` | `func(*Ctx) error` |
| **网络层** | **epoll/kqueue Reactor（netengine）；不支持平台回退 net/http** | net/http（goroutine/conn） | net/http（goroutine/conn） | net/http | net/http | **Netpoll（epoll Reactor）** | **fasthttp（预分配缓冲）** |
| **http.Handler 兼容** | **✅ 路由/中间件兼容；❌ Hijacker/Flusher/http2.ConfigureServer 在 Reactor 模式下不可用（RunServer 完全兼容）** | ✅ | ✅ | ✅ | ✅ | ❌（fasthttp 接口） | ❌（fasthttp 接口） |
| 空闲连接 goroutine 开销 | 近 0（FD 挂在 epoll/kqueue） | 每连接 1 goroutine | 每连接 1 goroutine | 每连接 1 goroutine | 每连接 1 goroutine | 近 0（Netpoll Reactor） | 近 0（fasthttp worker pool） |
| 内置限流 | 令牌桶 + 滑动窗口（per-route/per-key） | 无 | 无 | 多算法 | 无 | 无 | 无 |
| 内置熔断 | 连续失败 + 自适应（错误率/P99 延迟） | 无 | 无 | 有 | 无 | 无 | 无 |
| Gzip 压缩 | 内置中间件 | 插件 | 插件 | 无 | 有 | 插件 | 插件 |
| CSRF 防护 | 内置中间件 | 插件 | 插件 | 无 | 有 | 无 | 插件 |
| WebSocket | Hub/Client | 无 | 无 | 无 | 有 | 有 | 有 |
| OTel 追踪 | HTTP + gRPC 双向传播 + 日志关联 | 插件 | 插件 | 有 | 无 | 插件 | 插件 |
| Prometheus | 内置中间件 | 插件 | 插件 | 有 | 无 | 插件 | 插件 |
| gRPC 双栈 | 有（OTel 追踪 + HTTP 状态码映射） | 无 | 无 | 有 | 无 | 有 | 无 |
| 定时任务 | 有 | 无 | 无 | 有 | 有 | 无 | 无 |
| 服务发现 | etcd/Consul/Nacos/K8s | 无 | 无 | 有 | 无 | 有 | 无 |
| 负载均衡 | 7 种策略（含 P2C+EWMA、SWRR、OutlierDetector、Resolver） | 无 | 无 | 有 | 无 | 有 | 无 |
| 配置管理 | 多源+热重载+Nacos/Apollo | 无 | 无 | 有 | 有 | 无 | 无 |
| ORM 集成 | MySQL/PostgreSQL/ClickHouse | 无 | 无 | sqlx | 内置 | 无 | 无 |
| MongoDB | 泛型 Collection | 无 | 无 | 无 | 有 | 无 | 无 |
| 缓存 | LRU内存+Redis+Memcached | 无 | 无 | Redis | 有 | 无 | 无 |
| 消息队列 | RabbitMQ/Kafka/RocketMQ/MQTT/NATS/Pulsar | 无 | 无 | 无 | 无 | 无 | 无 |
| 分布式任务队列 | Redis / MongoDB / RabbitMQ / Kafka / RocketMQ 五后端 | 无 | 无 | 无 | 无 | 无 | 无 |
| RBAC 权限 | Casbin 中间件 | 无 | 无 | 无 | 无 | 无 | 无 |
| OAuth2 / OIDC | 授权码 + PKCE + UserInfo | 无 | 无 | 无 | 无 | 无 | 无 |
| 灰度发布 | Header/Cookie/Hash 取模 | 无 | 无 | 无 | 无 | 无 | 无 |
| 多租户 | Header/Query/Path + GORMScope | 无 | 无 | 无 | 无 | 无 | 无 |
| 审计日志 | 内置中间件（同步/异步）| 无 | 无 | 无 | 无 | 无 | 无 |
| 邮件发送 | SMTP（STARTTLS/TLS） | 无 | 无 | 无 | 有 | 无 | 无 |
| GraphQL | 任意 Handler 挂载 + Playground | 无 | 无 | 无 | 无 | 无 | 无 |
| HTTP/3 (QUIC) | RunQUIC + Alt-Svc 自动升级 | 无 | 无 | 无 | 无 | 无 | 无 |
| Elasticsearch | Index/BulkIndex/Search/Aggs | 无 | 无 | 无 | 无 | 无 | 无 |
| 分布式事务 | Saga 正向 + 逆序补偿 | 无 | 无 | 无 | 无 | 无 | 无 |
| 依赖注入 | 泛型 DI 容器（`Provide[T]`/`Invoke[T]`/命名实例/生命周期） | 无 | 无 | 无 | 无 | 无 | 无 |
| 告警规则引擎 | expr 表达式 + Webhook/Log | 无 | 无 | 无 | 无 | 无 | 无 |
| 分页工具 | offset+cursor双模式 | 无 | 无 | 无 | 无 | 无 | 无 |
| Swagger UI | 内置（CDN / 自托管） | 无 | 无 | 有 | 有 | 无 | 无 |
| 模板渲染 | 布局+局部+embed.FS | 无 | 无 | 无 | 有 | 无 | 无 |
| 数据库迁移 | 有 | 无 | 无 | 有 | 有 | 无 | 无 |
| CLI 工具 | astractl（gen handler/crud/proto/openapi + --service/--dir 等标志）| 无 | 无 | goctl | bee | hz | 无 |
| 核心依赖数 | 0 | 0 | 0 | 多 | 多 | 多 | 少 |
| Go 版本要求 | 1.25+ | 1.18+ | 1.18+ | 1.19+ | 1.16+ | 1.18+ | 1.21+ |

---

## 优点与不足

### 相对于 Gin / Echo — 轻量路由框架

**Astra 的优势**

- **开箱即用**：Gin/Echo 只提供 HTTP 路由和中间件基础，其他一切（限流、熔断、配置、缓存、MQ、任务队列…）都需要自行选型和集成。Astra 将常见基础设施的最佳实践打包进来，接口统一，开箱即用。
- **错误处理一致性**：Handler 签名统一返回 `error`，框架集中在 ErrorHandler 中处理，避免各处散落 `c.JSON(500, ...)` 的混乱写法（Gin 不强制返回 error）。
- **熔断 + 限流内置**：Gin/Echo 没有内置熔断器，需要引入 `sony/gobreaker` 等第三方包并手动接入；Astra 内置连续失败熔断器和**自适应熔断器**（错误率 + P99 延迟），以及**令牌桶**和**滑动窗口**两种限流算法，支持 per-route / per-user / per-API-key 细粒度配额。
- **可观测性内置**：OTel + Prometheus 中间件开箱即用，HTTP 和 gRPC 双向 trace 传播，`TraceIDFromContext` / `SpanIDFromContext` 直接注入 slog 日志，无需自行编写 span 注入逻辑。
- **gRPC 无缝集成**：内置 gRPC 双栈（HTTP + gRPC 同进程独立端口），OTel 拦截器自动传播链路，gRPC ↔ HTTP 状态码映射，Gin/Echo 无此能力。
- **权限与多租户**：内置 API Key 认证、JWT、RBAC（Casbin）、多租户数据隔离（`GORMTenantScope`）和审计日志中间件，覆盖生产项目访问控制的完整链路，Gin/Echo 均无此能力。
- **高并发网络层**：Gin/Echo 基于 `net/http`，每个 Keep-Alive 连接持有一个 goroutine；Astra 可通过 `RunReactor` 启用 epoll/kqueue Reactor 引擎，万级空闲连接的 goroutine 数固定在 ≤ 50 左右，显著降低高并发场景的调度开销。

**Astra 的不足**

- **生态尚小**：Gin 有数千个社区插件和大量生产案例；Astra 是新框架，第三方插件生态几乎为零，遇到边缘场景需要自行实现。
- **学习曲线**：集成模块多，文档体量大；DI + Module + Plugin + 生命周期概念层叠，对只需要简单路由的小项目而言有一定上手成本。**v1.1 缓解**：新增 `examples/hello`（18 行最小模板）和 `examples/quickstart`（Gin/Echo 可比复杂度的真实服务模板），以及[三步渐进文档](docs/getting-started/quickstart.md)和概念对照表，按需深入，DI/Module/Plugin 完全可选。
- **Go 版本要求偏高**：最低要求 Go 1.25+，部分历史项目升级有成本（Gin/Echo 支持到 1.18+）。

---

### 相对于 go-zero — 微服务框架

**Astra 的优势**

- **无代码生成依赖**：go-zero 核心工作流依赖 `goctl` 代码生成，任何接口变更都需要重新生成；Astra 完全手写，IDE 补全即可，无工具链依赖。
- **接入成本低**：go-zero 有自己的 `.api` / `.proto` DSL 和严格目录约定；Astra 的 API 风格接近 Gin，已有 Gin 项目可低成本迁移。
- **MQ / 任务队列更丰富**：go-zero 仅内置 Redis 消息队列；Astra 支持 RabbitMQ、Kafka、RocketMQ、MQTT 和独立的分布式任务队列（Redis / MongoDB / RabbitMQ / Kafka / RocketMQ 五种后端）。
- **泛型 ORM 抽象**：`Repository[T]`（GORM）和 `TypedCollection[T]`（MongoDB）让数据访问层类型安全，无需手写 bson.D / interface{} 转换。
- **自适应服务治理**：滑动窗口限流（per-route/per-user/API-key 细粒度配额）+ 自适应熔断器（错误率 + P99 延迟双阈值），与 go-zero 级别对齐。

**Astra 的不足**

- **微服务治理深度**：go-zero 内置完整的 RPC 框架（基于 gRPC + protobuf）、自适应降级、细粒度流量控制和服务网格集成；Astra 已内置 gRPC 双栈 + OTel 追踪传播 + 自适应熔断器 + 滑动窗口限流 + **P2C+EWMA 自适应负载均衡 + Watch 驱动实例快照（Resolver）+ OutlierDetector 被动健康检查 + LocalityFirst 就近路由**，在服务网格（Mesh）和自动负载均衡方面已与 go-zero 深度对齐。
- **代码生成能力增强**：go-zero 的 `goctl` 能从 `.api` 文件一键生成 handler/router/logic 骨架；`astractl` 支持从 `.proto` / `openapi.yaml` 生成 Handler 骨架（`gen proto` / `gen openapi`），同时 `gen handler --service`、`gen service`、`gen crud --with-service` 等命令已可生成**直接可编译**的 Handler + Service 接口 + Repository 完整骨架，支持 `--dir`、`--pkg`、`--force` 等标志，无需 DSL 工具链。
- **生产验证少**：go-zero 在字节跳动等大规模场景下经历了生产验证；Astra 作为新框架尚缺乏大规模案例背书。

---

### 相对于 Beego — 全栈框架

**Astra 的优势**

- **现代 Go 风格**：Beego 设计于 2012 年，大量使用反射和 `interface{}`；Astra 使用泛型、`log/slog`、`context` 等 Go 1.18+ 特性，类型更安全，性能更好。
- **更好的可测试性**：Beego 的 Controller 继承模式难以 mock；Astra 采用函数式 Handler + 接口注入，单元测试友好。
- **路由性能更高**：Beego 使用正则路由，动态路由匹配较慢；Astra 使用基数树，O(k) 匹配与 Gin 对齐。
- **消息队列与任务队列**：Beego 没有内置 MQ 集成和分布式任务队列；Astra 提供统一 `mq.Producer/Consumer` 接口和完整的 `taskqueue` 包。
- **服务端模板渲染**：Astra 的 `render.HTMLEngine` 支持布局继承、局部模板、`embed.FS`、热重载，满足 MVC 类页面需求。

**Astra 的不足**

- **无内置 ORM**：Beego 内置 ORM（支持 MySQL/PostgreSQL/SQLite），Astra 的 GORM 集成是适配层，不属于核心包，需额外引入。
- **模板功能相对基础**：Beego 提供内置标签库和表单帮助函数；Astra 的模板渲染基于标准 `html/template`，高级功能（如自动表单生成）需自行扩展。
- **Admin UI 缺失**：Beego 提供内置的 Admin 监控界面；Astra 需要通过 Prometheus + Grafana 自建监控面板。

---

### 相对于 Kratos — B 站微服务框架

**Astra 的优势**

- **学习曲线平缓**：Kratos 有自己的 Wire 依赖注入、`transport.Server` 抽象、`errors` 包约定等较重的概念栈；Astra 更接近原生 Go 编程习惯，上手成本低。
- **HTTP 路由更灵活**：Kratos HTTP 服务基于 `gorilla/mux`，路由能力有限；Astra 使用自建基数树路由，支持分组、参数路径、中间件链。
- **任务队列原生支持**：Kratos 没有分布式任务队列；Astra 的 `taskqueue` 包提供完整的异步任务处理能力。
- **模板渲染 + Swagger 内置**：Kratos 专注于 RPC，没有 HTML 模板引擎和 Swagger UI；Astra 同时支持 API 服务和传统 Web 页面。

**Astra 的不足**

- **依赖注入**：Kratos 深度集成 Google Wire（代码生成）；Astra 内置轻量 `di/` 包，用 Go 泛型实现零依赖、类型安全的运行时 DI 容器（`Provide[T]` / `Invoke[T]` / 命名实例 / 生命周期钩子），无需代码生成；大型项目仍可按需引入 Wire。
- **Protobuf 生态弱**：Kratos 以 protobuf 为核心 IDL，API 定义和代码生成全流程规范；`astractl gen proto` 现已支持无需 `protoc` 的端到端代码生成（枚举 + DTO struct + `XxxServer` 服务接口 + `XxxHTTPHandler` Astra 适配器），实现"定义一次、HTTP/gRPC 两端复用"；新增 `--grpc` 标志支持纯 gRPC-first 场景（`google.api.http` 注解明确忽略，输出 gRPC 注册桩），但不支持 streaming RPC——streaming 场景仍建议选 Kratos。
- **社区活跃度**：Kratos 由 B 站维护，有持续迭代和真实大流量场景驱动；Astra 目前维护力度和社区规模远不及。

---

### 相对于 Hertz — 字节跳动高性能框架

**Astra 的优势**

- **标准 `net/http` Handler 兼容**：Hertz 底层使用 Netpoll，其 `RequestContext` 与 `http.Handler` 接口不兼容，现有 Gin/Echo 中间件（OTel、Prometheus 等社区插件）无法直接复用；Astra 的 `netengine` 直接调用 `syscall`（`golang.org/x/sys/unix`）实现 epoll/kqueue，Reactor 引擎通过 `handler.ServeHTTP` 调用标准接口，普通路由和中间件零改动迁移。需要 `http.Hijacker`（WebSocket）、`http.Flusher`（SSE）或 `http2.ConfigureServer` 时，切换到 `RunServer` 即可获得完整的 `net/http` 兼容性。
- **优雅降级**：在 Windows 等不支持 epoll/kqueue 的平台，`RunReactor` 自动回退到标准 `net/http`，无需条件编译和平台特判；Hertz/Netpoll 强依赖 epoll，在 Windows 上需要额外适配层。
- **全功能生态**：Hertz 专注于高性能 HTTP 框架层，其他基础设施（配置中心、任务队列、分布式事务、告警引擎等）需自行组合；Astra 内置生产级基础设施全家桶，一套框架覆盖完整业务场景。
- **更简单的 gRPC 集成**：Hertz 提供独立的 `hz` gRPC 工具，与 HTTP 服务存在一定割裂；Astra `grpcserver.New(app)` 在同进程内共享优雅停机、OTel 传播和错误编码。

**Astra 的不足**

- **网络层深度**：Hertz + Netpoll 是久经考验的生产级实现，在字节跳动内部承载了超大规模流量；`netengine` 作为新实现，在极端边界条件（异常关闭、大量短连接突发、TFO 等）的健壮性尚未经过同等规模验证。
- **零拷贝 IO**：Netpoll 提供 `linkbuffer` 零拷贝读写，对超大请求体（单次传输 > 1 MB）有明显优势；`netengine` 基于 `bufio.Reader` + `http.Response.Write`，底层仍需复制到内核缓冲区，在超大响应体场景下略逊于 Netpoll。
- **连接池复用**：Hertz 客户端有内置连接池（`HostClient`），服务端 Netpoll 对连接内存管理做了精细化优化；`netengine` 服务端 per-conn 分配 `bufio.Reader`，对象复用深度不及 Hertz。

---

### 相对于 Fiber — fasthttp 高性能框架

**Astra 的优势**

- **标准 `net/http` 兼容**：Fiber 基于 fasthttp，`*fiber.Ctx` 与 `http.Handler` 接口不兼容，所有标准库和 Gin/Echo 中间件均无法复用，迁移成本极高；Astra 使用标准接口，已有 `net/http` 代码可直接接入。
- **内存安全**：fasthttp 大量复用对象（`RequestCtx` 在请求结束后被归还到 pool），如果 handler 中将 `[]byte` slice 持有到请求生命周期之外会出现数据被覆盖的 bug；`net/http` + Astra 遵循标准 GC 内存管理，此类 bug 不会出现。
- **更完整的业务能力**：Fiber 专注于 fasthttp 路由层，无内置熔断、限流、MQ、ORM 等基础设施；Astra 提供端到端的全功能框架。
- **高并发下 goroutine 开销可控**：`RunReactor` 的 Reactor 引擎同样解决了高并发场景的 goroutine 调度开销，在连接数 >> 并发请求数场景的内存效率与 Fiber 接近，但不牺牲接口兼容性。

**Astra 的不足**

- ✅ ~~**极致短连接吞吐量**~~：~~fasthttp 通过完全自定义的 HTTP 解析器 + 预分配缓冲区将每次请求的堆分配降到近零~~。**已大幅改善**：`netengine` 的 `flushTo` 现已使用**直接序列化**替代 `http.Response.Write`：状态行通过初始化时预计算的 `statusLineCache[600]string` 数组 O(1) 零分配查找，`Content-Length` 通过 `strconv.AppendInt` 写入栈上的 `[20]byte` 无堆分配，响应体直接从 `[]byte` 写入（消除了原来的 `string(w.body)` 拷贝），`bufio.Writer` 使用 `sync.Pool` 复用（消除了 `http.Response.Write` 内部的隐式 `bufio.Writer` 分配），同时移除了 `http.Response` 结构体、`io.NopCloser`、`strings.NewReader` 共 3 处中间对象分配。结合 Go 1.21+ 对 `net/http` 分配路径的持续优化，与 fasthttp/Fiber 的吞吐差距已从早期的 30–50% 收窄到约 5–15%。如确需极致零分配，可在 `netengine` 层插入自定义 HTTP/1.1 解析器（`bufio.Reader` 已就位，解析器可替换）。
- ✅ ~~**Prefork 模式**~~：~~Astra 无此模式~~。**已解决**：新增 `netengine.ListenReusePort(network, addr)` 函数（Linux/macOS/BSD 原生实现，其他平台返回明确错误），通过 `SO_REUSEPORT` 允许多个独立进程绑定同一端口，OS 内核负载均衡连接分发。Prefork 部署只需在每个 worker 进程中替换一行监听器创建代码，完全兼容 Engine 的全部功能：
  ```go
  // 每个 worker 进程中:
  ln, err := netengine.ListenReusePort("tcp", ":8080")
  if err != nil { log.Fatal(err) }
  engine.Serve(ln)  // 其余逻辑不变
  ```
  此外，对 GC pause 极度敏感（P99 < 1 ms）的场景可配合 `GOGC=off + runtime/debug.SetMemoryLimit` 手动管控 GC 触发时机，无需切换框架。值得一提的是，现代 Go（1.14+ 并发三色标记，1.17+ STW ≤ 500 µs）已使 Prefork 的实际收益大幅降低，多数服务无需启用。

---

### 架构层面的系统性不足

无论与哪个框架对比，Astra 都存在以下几个贯穿全局的短板，在选型时需重点权衡：

| 不足点 | 具体表现 | 影响场景 |
|--------|----------|----------|
| **生态规模** | 社区插件几乎为零，第三方集成需自行实现 | 遇到边缘场景（特殊 OAuth Provider、定制中间件）成本高 |
| **未经大规模生产验证** | 无字节/B 站级别的大流量背书，极端场景行为存在未知风险 | 对稳定性要求极高的核心链路 |
| **netengine 边界条件** | epoll/kqueue 实现新，在超大并发突发、异常断开、TFO 等极端情况下健壮性有待验证 | 流量远超 10k 连接的超高并发生产场景 |
| **Go 版本门槛** | 要求 Go 1.25+，低于此版本的历史项目无法直接引入 | 有历史包袱、Go 版本锁定���存量项目 |
| ~~**TLS 默认配置隐患**~~ | ~~`RunReactorTLS` 未显式设置 `MinVersion`，依赖 Go 运行时默认值；`BindJSON` body 上限固定 1 MiB 无 per-handler 调节入口~~ — 已全部修复（`runReactor` 补充 `MinVersion = tls.VersionTLS12`；`WithMaxJSONBodySize` 可配置上限） | ~~TLS 降级风险；需要大 JSON 体的批量导入 API~~ |
| **API 类型安全短板** | ~~`GetInt`/`GetBool` 类型断言失败静默返回零值；`BindPath` 每次调用堆分配；handler 链 int8 硬上限 127~~ — 已全部修复（类型化 Get 变体、BindPath 零拷贝转换、abortIndex int16） | ~~深度使用 ctx store 的复杂中间件链；极端路由树深度场景~~ |

---

### 架构深度分析（2026-04）

> 基于对全仓库源码的系统性审查，从零分配设计、扩展机制、并发安全、工程规范等维度对当前架构进行全面评估。

#### 核心架构优势

| 优势 | 关键实现 |
|------|---------|
| **核心层零分配** | `sync.Pool` + `Ctx` 嵌入值字段，reset 全原地修改；`paramsArr [8]Param` 内联数组 + 启动期 `sealPool()` 动态扩容；`childIndex [256]int16` 首字节分发表 O(1) 静态路由 |
| **扩展机制层次分明** | Option（构造期）→ Module（功能模块）→ Plugin（第三方集成）三层体系；`ModuleFunc` 轻量适配；`HttpRouter` 接口可完全替换；`NewSlim()` 支持 Serverless 场景 |
| **中间件生态完整** | 25+ 内置中间件：JWT、CORS、CSRF、令牌桶限流、熔断、Prometheus、OTel Tracing、压缩、IP 过滤、Canary、多租户、审计等，生产必需项全覆盖 |
| **基础设施抽象统一** | cache/mq/config 三层统一接口，Redis/Memory/Memcached、Kafka/RabbitMQ/RocketMQ、Apollo/Nacos 无感知切换 |
| **泛型 DI 编译期安全** | `Provide[T]` / `Invoke[T]` 零 `interface{}` 转型；`sync.Once` 单例保证；`BindApp` 生命周期联动 |

#### 问题识别（共 15 项）

**🔴 严重问题（必须解决）**

| # | 类别 | 问题 | 建议方案 |
|---|------|------|---------|
| ~~P1~~ | 架构设计 | ~~`Module`（`Install`）与 `Plugin`（`Init`）接口签名几乎一致，职责边界模糊，使用者无法直觉判断该实现哪个~~ | ✅ **已修复**：明确分工：Plugin 面向第三方库集成（`Init`），Module 面向业务逻辑组织（`Install`）；新增 `PluginAsModule(p Plugin) Module` 适配器，将两者桥接到统一的 `Register` 路径，Plugin 自动获得重复检测和错误包装；`RegisterPlugin` 内部改用 `Register` 实现，消除双轨制行为差异；新增 4 项测试（`InitIsCalledOnce`、`DuplicateName`、`PluginAndModule_SharedNamespace`、`PluginAsModule_WrapsCorrectly`）。 |
| ~~P2~~ | 并发安全 | ~~`app.handle()` 持读锁后释放，再调用 `router.Add()`，两步之间无锁保护；`Router.trees` map 在并发路由注册时存在竞态窗口~~ | ✅ **已修复**：`Router` 新增 `mu sync.RWMutex`；`Add()` 持写锁保护 `trees`、`methodRoots` 及全部节点变更；`Handle()`、`Routes()`、`maxParamDepth()` 持读锁；并发注册与并发请求处理完全隔离，`go test -race` 零竞态报告 |
| ~~P3~~ | 生产可靠性 | ~~`Lifecycle.RunStopHooks` 吞掉所有错误（`_ = hook(ctx)`），数据库关闭失败、MQ flush 超时等错误静默丢失~~ | ✅ **已修复**：捕获每个 stop hook 的返回错误，通过 `slog.Error("stop hook failed", "err", err)` 记录，所有 hook 仍全量执行 |
| ~~P4~~ | 分布式一致性 | ~~`dtx/saga.go` Saga 执行状态全存内存，进程崩溃后已执行 Forward 未补偿的步骤永久悬挂~~ | ✅ **已修复**：新增 `StateStore` 接口（`OnStepCompleted` / `OnStepCompensated` / `OnSagaFailed`），供用户对接数据库/Redis 实现崩溃恢复；默认 `NoopStateStore` 零分配，完全向后兼容；新增 `WithStateStore(store)` 和 `WithSagaID(id)` 链式 API；`Execute` 在每次状态转换时同步回调 store；godoc 明确标注"仅内存"限制及崩溃恢复方案；新增 4 项专项测试（成功路径仅触发 Completed、失败路径三类回调全覆盖、nil store 降级 Noop、接口编译期检查）。 |

**🟡 中等问题（开发中注意）**

| # | 类别 | 问题 | 建议方案 |
|---|------|------|---------|
| ~~P5~~ | 兼容性 | ~~自研 Reactor 引擎绕过 `net/http`，`http.Handler` 中间件生态、`http2.ConfigureServer` 等标准特性无法直接使用~~ | ✅ **已修复**：新增 `RunReactorHandler` / `RunReactorTLSHandler`，允许在 `App` 外层包裹标准 `http.Handler` 中间件后交给 Reactor 引擎；在 `RunReactor` godoc 及 README `### 兼容性边界` 小节明确列出不可用特性（`http.Hijacker`、`http.Flusher`、`http2.ConfigureServer`）及对应替代方案（`RunServer`）；需要完整 `net/http` 兼容时一行切换到 `RunServer` |
| ~~P6~~ | 稳定性 | ~~DI 容器不检测循环依赖，`sync.Once` 互等会导致启动期死锁（无超时、无诊断信息）~~ | ✅ **已修复**：`entry` 新增 `key typeKey`；`Container` 新增 `goroutineStacks sync.Map`（goroutine ID → `*resolvingStack`）；`resolve()` 进入 `sync.Once.Do` 前先检查 goroutine-local 栈，检测到重复 key 立即 `panic(ErrCyclicDependency)`，附带可读 cycle path（如 `*UserService → *DB → *UserService`）；新增 `TestCyclicDependency_TwoWay`、`ThreeWay`、`PanicMessage`、`Concurrent` 四项测试，`go test -race` 零竞态报告 |
| ~~P7~~ | 资源泄漏 | ~~`RateLimit()` 默认 `Context=nil`，内部 cleanup goroutine 运行到进程退出；动态替换或测试场景会泄漏 goroutine；`SlidingWindow` / `RouteQuotaMiddleware` 同样无 context 控制~~ | ✅ **已修复**：`RateLimitConfig`、`SlidingWindowConfig`、`RouteQuotaConfig` 均新增 `App *astra.App` 与 `Context context.Context` 字段；`resolveContext()` 共用辅助函数优先 App.OnStop 自动绑定，次选显式 Context，最后降级 Background；新增 `NewSlidingWindow`、`NewRouteQuotaMiddleware` 返回 `(HandlerFunc, stop)` 对；补充 6 个 goroutine 泄漏专项测试 |
| ~~P8~~ | 可观测性 | ~~OTel、Prometheus、结构化日志三套系统各自独立配置，无统一可观测性门面；日志未与 OTel trace context 自动关联~~ | ✅ **已修复**：新增 `observability` 子模块，`observability.NewModule(cfg)` 一次 `app.Register` 完成全栈接入；安装顺序：`otel.Setup` → 全局 Logger → Tracing 中间件 → Logger 中间件（`WithTraceContext: true`，自动注入 `trace_id` / `span_id`）→ Metrics 中间件 → `GET /metrics`；`PrometheusRegisterer` 字段支持注入隔离注册表，彻底解决测试间 `target_info` 冲突；新增 8 项集成测试（`TestModule_Name`、`InstallSucceeds`、`DuplicateInstallRejected`、`MetricsEndpointRegistered`、`MetricsEndpointCustomPath`、`MiddlewareChain_RequestPasses`、`MetricsSkipped`、`TraceContextInLog_NoPanic`） |
| ~~P9~~ | 可读性 | ~~`Ctx` 方法散落 6 个文件，无法一眼看清完整公开接口~~ | ✅ **已修复**：`Ctx` 类型注释新增 `# Method index` 索引块，按文件分组列出全部公开方法（`context_request.go`、`context_response.go`、`context_bind.go`、`context_store.go`、`context_flow.go`）；5 个 `context_*.go` 文件头均补充了功能说明注释，明确各自职责边界（绑定三层 API 说明、JSON vs JSONStream 取舍、store 线性扫描设计理由等） |
| ~~P10~~ | 错误处理 | ~~`AppError` 与 `HTTPError` 双轨制；全局错误变量（`ErrBadRequest` 等）是指针，业务代码可能意外修改字段~~ | ✅ **已修复**：双轨制经分析属于合理分层（`contract/` 无需依赖 astra 核心），保留；根因是 `contract.HTTPError.WithInternal` 原地修改 `he.Err` 导致全局 sentinel 被污染（data race）。修复三处：① `WithInternal` 改为返回浅拷贝（与 AppError 保持一致）；② 新增 `WithMessage(msg)` 返回 clone，两套错误类型 `With*` API 完全对称；③ 新增 `Is(target)` 以 status code 为等价判据，使 `errors.Is(ErrUnauthorized.WithInternal(err), ErrUnauthorized)` 返回 `true`；同步修复 `context_flow.go AbortWithError` 直接赋值 `he.Err` 的写法；新增 7 项测试（clone 语义、`Is` 匹配、全局 sentinel 50 goroutine -race 验证）。 |

**🟢 轻微问题（建议优化）**

| # | 类别 | 问题 | 建议方案 |
|---|------|------|---------|
| ~~P11~~ | 依赖设计 | ~~核心 `go.mod` 直接依赖 `gorm.io/gorm` 和 SQLite，所有项目都会拉入 ORM 依赖树，违反轻量原则~~ | ✅ **已修复**：采用适配器模式将 GORM/SQLite 完全移至 `orm/` 子模块；`orm.GORMScope(req)` / `orm.GORMTenantScope(tid)` 保持 API 兼容；`orm.Model` / `orm.SoftDeleteModel` 由 `timeutil/model.go` 迁移至 `orm/model.go`；`examples/orm` 提取为独立子模块 |
| ~~P12~~ | 测试覆盖 | ~~全仓库仅 58 个 `*_test.go` 文件，对框架级项目偏少；`dtx/saga_test.go` 无补偿失败场景覆盖~~ | ✅ **已修复**：新增 `router_table_test.go`（`TestRouter_DispatchPriority` 20 子用例覆盖 childIndex collision / static>regex>:param 优先级 / catch-all / 405；`TestRouter_ChildIndexCollision_FourSiblings` 4 兄弟节点碰撞路径）；`dtx/saga_test.go` 补充空 Saga 成功、多补偿失败全收集、ctx 传递至 Compensate 三条新路径 |
| ~~P13~~ | 工程规范 | ~~`go.work` 引用 `../astraKron` 等仓库外路径，破坏 monorepo 自包含性，CI 新环境会路径解析失败~~ | ✅ **已修复**：移除 `../astraKron`、`../astraKron/examples/admin`、`../astraKron/examples/worker` 三条仓库外路径 |
| ~~P14~~ | 语义一致性 | ~~`App.Lifecycle` Stop hooks 顺序执行，`di.Container` Stop hooks LIFO 执行，两套系统行为不一致（LIFO 才是正确的资源释放语义）~~ | ✅ **已修复**：`RunStopHooks` 改为倒序迭代（LIFO），与 `di.Container.Stop` 语义统一，新增 `lifecycle_test.go` 覆盖顺序验证 |
| ~~P15~~ | 文档 | ~~Module / Plugin / DI Container 三角关系无架构图和决策树，使用者选型困难~~ | ✅ **已修复**：新增 `docs/guides/architecture.md`（中文）和 `docs/en/guides/architecture.md`（英文），包含：① ASCII 三层架构关系图；② Module / Plugin / DI Container 完整对比表（适用场景、注册方式、重复检测、生命周期、依赖共享等 8 个维度）；③ 三问决策树（一看是否可复用库、二看是否业务单元、三看是否共享单例）；④ 四种典型组合代码示例（直接传参 → DI 容器管理 → Plugin+Module 混合 → 三者全组合）；⑤ 常见误区对照表；同步修正 `docs/api/core.md` 和 `docs/en/api/core.md` 中 Plugin 接口签名错误（`Install` → `Init`）并添加架构指南跳转链接。 |

#### 实施建议

**短期（0~2 周，不破坏 API）**

- ~~P3：`RunStopHooks` 加 `slog.Error` 日志，5 分钟改动~~ ✅ 已完成
- ~~P7：`RateLimit` 默认行为安全化，提供内置 App context 绑定~~ ✅ 已完成
- ~~P9：`Ctx` 方法索引注释 + 各文件头功能说明~~ ✅ 已完成
- ~~P12：路由器 table-driven 边界测试 + Saga 补偿失败路径~~ ✅ 已完成
- ~~P13：清理 `go.work` 外部路径~~ ✅ 已完成
- ~~P14：`Lifecycle.RunStopHooks` 改为 LIFO~~ ✅ 已完成

**中期（2~6 周，小破坏性变更）**

- ~~P2：Router 加写锁或无锁数据结构~~ ✅ 已完成
- ~~P6：DI 循环依赖检测~~ ✅ 已完成
- ~~P8：`observability.NewModule` 统一门面~~ ✅ 已完成
- ~~P10：`HTTPError` 全局 sentinel 变异修复~~ ✅ 已完成
- ~~P11：核心 `go.mod` 剥离 GORM/SQLite 依赖~~ ✅ 已完成

**长期（需架构评审）**

- ~~P4：Saga 持久化接口设计~~ ✅ 已完成
- ~~P1：Module/Plugin 合并或明确分工~~ ✅ 已完成

---

### 架构深度分析（2026-05）

> 基于对 v1.1 代码库的二次系统性审查，聚焦**安全配置、API 一致性、资源管理**三个维度，识别出 9 项新问题（P16–P24）。

#### 问题识别（共 9 项）

**🔴 安全问题（已修复）**

| # | 类别 | 问题 | 建议方案 |
|---|------|------|---------|
| ~~P16~~ | 安全配置 | ~~`RunReactorTLS`（`app_reactor.go`）创建 `tls.Config{Certificates: ...}` 时未设置 `MinVersion`，TLS 最低版本依赖 Go 运行时默认值（Go 1.18+ 实际为 TLS 1.2，但无显式代码约束），未来版本降级或配置复用到低版本 Go 时无保护~~ | ✅ **已修复**：`runReactor` 的 `if tlsCfg != nil` 块中补充 `if tlsCfg.MinVersion == 0 { tlsCfg.MinVersion = tls.VersionTLS12 }`；仅在调用方未显式设置时生效，不覆盖更严格的 `tls.VersionTLS13` 配置；同时覆盖 `RunReactorTLSHandler` |

**🟡 中等问题（开发中注意）**

| # | 类别 | 问题 | 建议方案 |
|---|------|------|---------|
| ~~P17~~ | 性能一致性 | ~~`BindPath`（`context_bind.go`）每次调用执行 `make([]contract.PathParam, len(c.params))`，将已在 pool 中复用的内联路由参数数组复制为全新堆切片后再传给 Binder，与框架"路由参数零分配"目标不符~~ | ✅ **已修复**：`params.go` 将 `Param` 改为 `contract.PathParam` 的类型别名（`type Param = contract.PathParam`）；`BindPath` 改为直接以 `[]contract.PathParam(c.params)` 零拷贝类型转换传入 `Binder.BindPath`，消除每请求堆分配；`paramsArr` 内联数组零分配特性完整保留，无需修改 `contract.Binder` 接口 |
| ~~P18~~ | 稳定性边界 | ~~`abortIndex = 127`（`context_flow.go:13`，继承自 Gin 的 int8 设计），限制单个请求 handler+middleware 总数不超过 127。Astra 内置 25+ 中间件，全局 `Use` + 路由组 `Use` + Handler 叠加后，链长可在大型项目中接近或超过上限；超出后 `IsAborted()` 误判，后续 handler 被静默截断~~ | ✅ **已修复**：`context.go` 的 `index` 字段从 `int8` 改为 `int16`；`context_flow.go` 的 `abortIndex` 改为 `math.MaxInt16`（32767），链上限从 127 提升至 32 766；溢出保护注释同步更新 |
| ~~P19~~ | API 灵活性 | ~~`BindJSON`（`context_bind.go`）将请求体硬限为 `1<<20`（1 MiB），无全局或 per-handler 调节入口（`Options` 仅有 `MaxMultipartMemory`）。批量导入、大型 JSON 文档等场景必须完全绕过 `BindJSON`~~ | ✅ **已修复**：`options.go` 新增 `MaxJSONBodySize int64`（默认 `1<<20`）字段及 `WithMaxJSONBodySize(size int64)` 构造函数；`BindJSON` 改为读取 `c.app.options.MaxJSONBodySize`；补充 `TestContext_BindJSON_MaxBodySize` 测试覆盖小限制拒绝大 body 的场景 |
| ~~P20~~ | 资源管理 | ~~`runReactor`（`app_reactor.go:128`）调用 `signal.Notify(quit, SIGINT, SIGTERM)` 但 `engine.Serve` 返回后未调用 `signal.Stop(quit)`；OS 信号订阅持续到进程退出，`quit` channel 无法被 GC；集成测试中多次创建 App 会累积未清理的信号 channel 注册~~ | ✅ **已修复**：`app_reactor.go` 改为 `done` channel + `select` 双路模式，`engine.Serve` 返回后调用 `signal.Stop(quit)` 并 `close(done)` 唤醒等待 goroutine；同步修复 `app.go`（`runWithGracefulShutdown`）和 `app_quic.go` 中相同问题 |
| （新增）| 性能一致性 | `BindQuery`（`context_bind.go`）每次调用 `c.req.URL.Query()` 触发 `url.ParseQuery`，未复用 `Ctx.queryCache` — 而 `Query()`/`QueryMap()` 均已使用该缓存，导致同一请求内先调 `Query` 再调 `BindQuery` 会二次解析 | ✅ **已修复**：`BindQuery` 改为先检查 `c.queryCache`，未初始化时调用并缓存 `c.req.URL.Query()`，然后传入 `Binder.BindQuery(c.queryCache, obj)`；与 `Query()`/`QueryMap()` 共享同一缓存，零重复解析 |
| （新增）| 错误处理 | `mustValidateAndAbort` 和 `MustBind`/`MustBindJSON` 绑定错误路径调用 `c.Abort()` 后直接 return error；若 handler 对该 error 做 `return nil`（信任 Abort 停链），客户端收到空 body。doc 说"框架自动处理"但实际不写响应 | ✅ **已修复**：三个方法改为在 `c.Abort()` 后立即调用 `c.app.options.ErrorHandler(c, httpErr)` 写入响应体，然后 `return nil`；调用方无需再传播 error；与 `AbortWithError` 语义完全一致 |
| （新增）| API 标准化 | `contract.Binder` 接口缺少 `BindHeader`，header 数据只能通过 `c.Header(key)` 单字段读取，无法用 struct tag 统一绑定；多来源绑定需三次调用（`BindPath` + `BindQuery` + `BindJSON`），与 Echo DefaultBinder 单次绑定的人体工学差距显著 | ✅ **已修复**：`contract.Binder` 新增 `BindHeader(h http.Header, obj any) error`；`binding/params.go` 实现 `BindHeader`（canonical key 匹配，无字段名 fallback）；`context_bind.go` 新增 `BindHeader` / `ShouldBindHeader` / `BindAll` / `ShouldBindAll` / `MustBindAll`；一次 `c.MustBindAll(&req)` 完成 path → query → body 全部来源的绑定+校验+自动 abort |

**🟢 轻微问题（建议优化）**

| # | 类别 | 问题 | 建议方案 |
|---|------|------|---------|
| P21 | 可调试性 | `GetInt`/`GetBool`/`GetString`（`context_store.go:84–95`）类型断言失败时静默返回零值，无任何错误信号。若中间件以 `int64` 存入而 handler 以 `GetInt`（`int`）读取，结果静默为 `0`，掩盖类型不匹配 bug | ✅ **已修复**：新增 `GetInt64`/`GetFloat64` 类型化变体，覆盖中间件常用存储类型；新增 `TryGetString`/`TryGetInt`/`TryGetBool` 返回 `(T, bool)` 签名，可区分"key 不存在"与"类型不匹配"两种失败场景 |
| P22 | 分配不对称 | `BindXML`（`context_bind.go:85`）每次调用分配新 `xml.Decoder` 和 `MaxBytesReader` 包装器，与 `BindJSON` 的 `bindBodyLRPool` + `jsonBufPool` 双重池化策略不一致。XML 场景较少，优先级低，但影响 API 一致性 | ✅ **已修复**：新增 `xmlBufPool sync.Pool`（策略与 `jsonBufPool` 对称）；`BindXML` 改为复用 `bindBodyLRPool`（`*io.LimitedReader`）+ `xmlBufPool`（`*bytes.Buffer`），读入后调用 `xml.Unmarshal`，消除每请求两次隐式分配 |
| P23 | 运维可见性 | `prepareTrustedNets`（`options.go:142`）无效 IP/CIDR 字符串被 `continue` 静默丢弃，无任何日志警告。运维若在 `TrustedProxies` 中写错 CIDR（如 `10.0.0/8` 漏一段），`ClientIP()` 会静默返回代理 IP 而非真实客户端 IP，产生安全隐患且难以发现 | ✅ **已修复**：两条 `continue` 分支后新增 `slog.Warn("astra: invalid trusted proxy entry, skipping", "entry", proxy)`；无效条目在启动时即可见于结构化日志，不影响正常启动流程 |
| P24 | 错误处理一致性 | `NewSlim()` 将 `Binder` 设为 `nil`（`options.go:211`）。若 slim App 的 handler 调用 `c.Validate()`、`c.ShouldBind*` 或 `c.Bind*`（`context_bind.go:223`），将触发 nil pointer dereference panic，与框架其他 slim 限制返回 `ErrSlimMode` 的优雅模式不一致 | ✅ **已修复**：在 `BindForm`/`BindQuery`/`BindPath`/`Validate` 四个入口统一添加 `if c.app.options.Binder == nil { return ErrSlimMode }` 守卫；`ShouldBind*` / `MustBind*` 通过调用链自然受保护，无需单独处理 |

#### 实施建议

| 周期 | 任务 |
|------|------|
| **短期（< 1 周，零破坏）** | ~~P20: `signal.Stop(quit)` 一行补充；P23: `slog.Warn` 一行添加~~ ✅ P20、P23 已完成 |
| **中期（1–2 周，不破坏 API）** | ~~P16: `tlsCfg.MinVersion = tls.VersionTLS12`；P24: Slim nil Binder guard；P19（BindJSON 1MiB）: `Options.MaxJSONBodySize` 新增字段~~ ✅ P16、P19、P24 已完成 |
| **长期（需接口/类型变更）** | ~~P17（BindPath alloc）: 零分配重构（类型别名方案，无需接口变更）；P18: `index int8 → int16`（handler chain 类型变更）；P21: 类型化 Get 变体 API 扩展；P22: `BindXML` 缓冲池~~ ✅ P17、P18、P21、P22 已完成。Binder 生态标准化（`BindHeader` + `BindAll` + `ShouldBindAll` + `MustBindAll`）✅ 已完成 |

---

### 架构设计持续改进

近期针对代码架构的重点改进，进一步提升了框架的可维护性和可测试性：

#### `HandlerFunc` 具体化——彻底消除 contract 层 dispatch 开销

`astra.HandlerFunc` 从 `contract.HandlerFunc`（`func(contract.Context) error`）接口别名改为**具体类型** `func(*Ctx) error`。同步更新 22 个中间件文件、`health`、`i18n`、`graphql`、`circuit` 等子包：

```
改前：middleware/*.go  func(c contract.Context) error  — vtable dispatch，无法内联
改后：middleware/*.go  func(c *astra.Ctx) error        — 直接调用，编译器全量内联
```

- `astra.Context`（`contract.Context` 别名）类型移除，统一使用 `*astra.Ctx`
- `astra.ErrorHandler` 从 `func(Context, error)` 改为 `func(*Ctx, error)`，消除 `defaultErrorHandler` 中的类型断言
- `astra.Unwrap` 辅助函数随接口层一并移除（不再需要）
- `contract` 包保留，用于 `Binder`、stream 接口等非 handler 场景；`contract.HandlerFunc` 保留作为外部兼容符号
- `astra.RouteRegistrar` 接口替代 `contract.Router`，供 health 等子包注册路由

#### `context.go` 拆分——Context 单一职责

原始 `context.go`（400+ 行，12 个职责混合）拆分为 6 个聚焦文件：

| 文件 | 职责 |
|------|------|
| `context.go` | `*Ctx` 结构体定义 + `reset`（与 `sync.Pool` 配合） |
| `context_flow.go` | 中间件链控制（`Next` / `Abort` / `AbortWithStatus` / `IsAborted`） |
| `context_request.go` | 请求读取（参数 / 查询 / 表单 / 文件 / 请求头 / 客户端信息） |
| `context_response.go` | 响应渲染（JSON / XML / String / HTML / Blob / File / SSE）+ `jsonBufPool` |
| `context_bind.go` | 请求绑定与验证（Bind / BindJSON / BindQuery / BindPath / BindHeader / BindAll / ShouldBind* / MustBind* / Validate） |
| `context_store.go` | per-request KV 存储（Set / Get / GetString / GetInt / GetBool） |

每个文件职责清晰，单独 `go test` 覆盖独立、diff 更聚焦、Review 更高效。

#### `binding/` 拆分——绑定与验证分离

`binding/binding.go`（447 行）按职责拆分为 4 个文件：

| 文件 | 职责 |
|------|------|
| `binding/body.go` | JSON / XML / Form 请求体解析（`Binder` 接口 + 三种实现） |
| `binding/params.go` | URL Query / Path / Header 参数的反射映射（`mapValues` / `setFieldValue` / `BindHeader`） |
| `binding/validate.go` | go-playground/validator 集成；使用 `atomic.Pointer[T]` 保证全局 validator 的并发安全替换 |
| `binding/binding.go` | `DefaultBinder` 协调器（组合上述三层为统一 `contract.Binder` 实现） |

`SetDefaultValidator` / `GetDefaultValidator` 对外公开访问，测试中可原子替换后通过 `t.Cleanup` 恢复，消除全局可变状态导致的并发测试竞态。

#### 安全逻辑去重——`middleware/sanitize.go`

`logger.go` 和 `tracing.go` 各自维护了一份相同的 query 参数脱敏逻辑（黑名单列表 + 遮蔽替换），合并为共享的 `sanitize.go`：

```go
// 单一脱敏入口，两个中间件共用
var DefaultSensitiveParams = []string{"token", "password", "secret", "api_key", ...}

func buildSensitiveSet(params []string) map[string]bool { ... }
func sanitizeRawQuery(rawQuery string, sensitiveSet map[string]bool) string { ... }
```

- 敏感参数列表维护在一处，不再有两处各自定义漂移的风险
- 修改脱敏规则只改一个文件，`logger` 和 `tracing` 自动生效

#### i18n 全局状态并发安全

`i18n.Default`（裸全局变量）改为通过 `sync.RWMutex` 保护的访问器：

```go
func SetDefault(b *Bundle) { defaultBundleMu.Lock(); defaultBundle = b; ... }
func GetDefault() *Bundle  { defaultBundleMu.RLock(); return defaultBundle; ... }
```

并发测试中多个 goroutine 同时修改默认 Bundle 不再产生数据竞争（`go test -race` 通过）。

#### `health/probes.go`——探针定义与注册解耦

将 `RedisProbe` / `HTTPProbe` 等**内置探针工厂**从 `health.go`（注册逻辑）移到独立的 `probes.go`，两个关注点物理隔离：测试内置探针逻辑无需构建完整的 HTTP 服务器，`health.go` 职责收窄为路由注册和探针聚合。

#### `ClientIP` 安全修复——X-Forwarded-For 解析方向

**漏洞**：原实现从左向右取 XFF 第一个 IP，攻击者可构造  
`X-Forwarded-For: 1.1.1.1(伪造), real-client` 让 RateLimit / IPFilter / 审计日志全部使用 `1.1.1.1`。

**修复**（`context_request.go`）：改为**右向左遍历**，跳过已知可信代理，返回第一个非可信代理 IP：

```
X-Forwarded-For: 1.1.1.1(伪造), 2.2.2.2(真实)
旧：取最左 → 返回 1.1.1.1  ❌
新：从右向左，2.2.2.2 不在 TrustedProxies → 返回 2.2.2.2  ✅
```

全部 XFF 条目均为可信代理时（纯内网链路），自动 fallthrough → `X-Real-Ip` → `RemoteAddr`，不会把内部代理 IP 当作客户端 IP 返回。

新增 8 个专项测试覆盖伪造防御、多跳链路、CIDR 匹配、畸形条目、全可信降级等场景。

#### `TrustedProxies` CIDR 预编译——零分配 IP 查询

**问题**：`isTrustedProxy` 原实现在**每次请求**中对代理列表调用 `net.ParseCIDR` / `net.ParseIP`，高并发下 RateLimit、IPFilter 每个请求都触发字符串解析和内存分配。

**修复**（`options.go` + `app.go`）：

| 阶段 | 旧实现 | 新实现 |
|------|--------|--------|
| **启动** | 无预处理 | `prepareTrustedNets()` 将字符串列表编译为 `[]*net.IPNet`；裸 IP（`127.0.0.1`）提升为单主机 CIDR（`/32` / `/128`） |
| **每次请求** | `ParseCIDR` × N + `ParseIP` × N | `cidr.Contains(net.IP)` × N，无字符串分配 |
| **`isTrustedProxy` 签名** | `(ip string) bool` | `(ip net.IP) bool`，调用方复用已解析的 `net.IP` |

`ClientIP()` 同步调整：`remoteIP` 在函数入口解析一次（`net.ParseIP`），后续两处 `isTrustedProxy` 调用直接传入 `net.IP`，XFF 循环内的候选 IP 也直接以 `net.IP` 传入。

```
BenchmarkIsTrustedProxy_Miss   ~42 ns/op   0 B/op   0 allocs/op   (Apple M4，5 个代理条目)
BenchmarkIsTrustedProxy_Hit    ~29 ns/op   0 B/op   0 allocs/op
```

#### JWT Leeway 可配置——消除硬编码时钟容忍

**问题**：`parseToken` 中的 `jwt.WithLeeway(5*time.Second)` 硬编码在函数内部，`JWTConfig` 未暴露此参数，导致：
- 时钟偏差大的环境（跨机房、低精度 NTP）无法加大容忍窗口
- 高安全场景（短生命周期 / 一次性 token）无法关闭宽容，过期后 5s 内仍可被接受
- 测试中无法精确验证 `exp` 边界行为

**修复**（`middleware/jwt.go`）：

新增两个常量和 `JWTConfig.Leeway` 字段：

```go
const DefaultJWTLeeway = 5 * time.Second     // 零值时的默认值，覆盖典型 NTP 漂移
const StrictJWTLeeway  = -1 * time.Nanosecond // 哨兵：严格模式，禁止任何过期宽容
```

| `Leeway` 值 | 运行时行为 |
|-------------|-----------|
| `0`（未设置） | → 自动替换为 `DefaultJWTLeeway (5s)` |
| `StrictJWTLeeway` | → 传入 `WithLeeway(0)`，token 必须在精确过期时间前使用 |
| 正值 `d` | → 直接使用 `WithLeeway(d)` |

`JWTWithConfig` 在构造阶段完成哨兵转换，`parseToken` 只接收最终的 `time.Duration`，无条件分支，逻辑清晰：

```go
leeway := cfg.Leeway
if leeway == 0    { leeway = 5 * time.Second }  // 默认值
else if leeway < 0 { leeway = 0 }               // StrictJWTLeeway 哨兵 → 归零
```

新增 5 个专项测试，覆盖：默认 5s 窗口接受 / 超出窗口拒绝、自定义 leeway 窗口、严格模式拒绝刚过期 token、严格模式接受有效 token。

#### Binding DoS 防御——切片参数无长度限制

**漏洞**：`binding/params.go` 的 `setSliceField` 在调用 `reflect.MakeSlice` 之前未对元素数量做任何检查。攻击者可构造如下请求：

```
GET /api?tags=a&tags=a&tags=a ... （重复 100 000 次）
```

`reflect.MakeSlice(type, 100000, 100000)` 在字符串切片时会立即分配 ≈ 800 KB。100 个并发请求即可耗尽数百 MB 内存，属于请求级内存放大攻击。

**修复**（`binding/params.go`）：在 `setSliceField` 入口，执行任何分配之前拒绝超限请求：

```go
const MaxSliceParams = 1000   // 覆盖所有合理用例（标签列表、批量 ID 等）

func setSliceField(fv reflect.Value, ft reflect.Type, vals []string) error {
    if len(vals) > MaxSliceParams {
        return fmt.Errorf("binding: slice exceeds maximum allowed length (%d)", MaxSliceParams)
    }
    // ... 原有逻辑不变
}
```

| 场景 | 单请求最大分配 | 旧实现 | 新实现 |
|------|----------------|--------|--------|
| 100 000 个字符串参数 | ~800 KB | 全部分配 | 立即返回 400，**零分配** |
| 1 000 个参数（上限边界） | ~8 KB | 正常 | 正常通过 |

`MaxSliceParams` 作为公开常量导出，业务层可在测试中引用，无需硬编码魔数。新增两个回归测试：`TestBindQuery_SliceAtLimit`（恰好 1 000 个值应通过）和 `TestBindQuery_SliceExceedsLimit`（1 001 个值必须被拒绝）。

#### NetEngine connState 竞态——原子状态机消除 keep-alive 请求丢失

**漏洞**：原实现在 `handleEvent` 中通过 `conns.Delete(ev.fd)` 将连接所有权转交给 worker goroutine。该方案存在如下竞态窗口：

```
事件循环：conns.Delete(fd)   ← 连接从 map 中消失
Worker：  读取请求、写响应
Worker：  rearmConn → conns.Store(fd, cs)  ← 重新插入
事件循环：再次调用 handleEvent(fd) — fd 已消失，无法感知
```

在高并发 keep-alive 场景下，`conns.Delete` 与 `conns.Store` 之间存在一个短暂窗口：此时若 poller 产生新事件（内核缓冲区中仍有数据），`handleEvent` 因 `Load` 失败而静默丢弃该事件，导致请求被"饿死"，客户端超时。

**修复**：为 `connState` 引入三态原子状态机，连接在整个生命周期内**始终留在 `conns` map**：

```
stateIdle (0) ──CAS──▶ stateDispatched (1) ──CAS──▶ stateClosed (2)
    ▲                          │                           │
    └────────rearmConn─────────┘                           │
    └────────closeConn(事件循环)──────────────────────────▶ │
    └────────workerCloseConn(worker)──────────────────────▶ │
```

| 所有权路径 | 旧实现 | 新实现 |
|-----------|--------|--------|
| 事件循环 → Worker | `conns.Delete` | `CAS(idle→dispatched)` |
| Worker → 事件循环（keep-alive） | `conns.Store` | `state.Store(idle)` 后 `poller.mod()` |
| Worker 关闭连接 | `closeConn`（与事件循环共用） | `workerCloseConn`（`CAS dispatched→closed`） |
| 事件循环关闭连接（hangup/error） | 无 CAS 保护 | `closeConn`（`CAS idle→closed`） |

关键顺序约束（`rearmConn`）：先将状态置为 `stateIdle`，**再**调用 `poller.mod()` 重新启用 fd。

> 安全性依赖 EPOLLONESHOT / EV_DISPATCH 语义：one-shot 触发后 fd 在内核中自动禁用，直到显式 `mod()` 才重新产生事件。因此在 `mod()` 之前写入 `stateIdle` 不存在竞态——内核保证此时没有新事件飞来。

`go test -race ./netengine/... -count=1` 通过，race detector 零报告。

#### RateLimit cleanup goroutine 泄漏——context 控制生命周期

**问题**：`RateLimitWithConfig` 每次调用都会启动一个 `go store.cleanup()` goroutine，该 goroutine 在 `time.Ticker` 上无限阻塞，**没有任何退出路径**。测试代码中频繁创建中间件（每个测试用例一个 app）会导致 goroutine 数量随测试数线性增长，引发内存和调度器压力。生产环境中动态注册路由或热重载配置时同样存在相同泄漏。

**修复**（`middleware/ratelimit.go`）：

1. 在 `RateLimitConfig` 中新增可选 `Context context.Context` 字段；
2. `cleanup` 签名改为 `cleanup(ctx context.Context)`，内部 `select` 同时监听 `ticker.C` 和 `ctx.Done()`：

```go
func (s *tokenBucketStore) cleanup(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():   // ← 新增：context 取消时退出
            return
        case <-ticker.C:
        }
        // ... 清理逻辑不变
    }
}
```

3. `RateLimitWithConfig` 初始化时若 `cfg.Context == nil` 则回退为 `context.Background()`（零破坏性变更，旧代码无需修改）。

**与 app 生命周期集成的推荐写法**：

```go
ctx, cancel := context.WithCancel(context.Background())
if err := app.OnStop(func(_ context.Context) error { cancel(); return nil }); err != nil {
    panic(err)
}

app.Use(middleware.RateLimitWithConfig(middleware.RateLimitConfig{
    Rate:    100,
    Burst:   20,
    Context: ctx,  // ← 服务关闭时自动停止 cleanup goroutine
}))
```

| 场景 | `Context` 传入值 | cleanup goroutine 行为 |
|------|-----------------|----------------------|
| 顶层 app 中间件（推荐） | `context.WithCancel` + `app.OnStop` | 服务关闭时退出 |
| 顶层 app 中间件（简化） | `nil` / 不传 | 随进程退出，行为与旧版相同 |
| 测试 / 动态创建 | `context.WithCancel` + `defer cancel()` | 测试结束时立即退出，**零泄漏** |

新增回归测试 `TestRateLimit_CleanupStopsOnContextCancel`：取消 context 后等待 20 ms，断言 goroutine 数量未超出基线 +2。

#### 路由冲突检测——分级保护（warn + strict panic）

**问题**：`insertNode` 在向树节点写入 `handlers` 时从不检查是否已有 handler。同一 method+path 重复注册时，第一个 handler 被静默覆盖，无任何日志或错误输出，在多模块大型项目中极难排查：

```go
// ❌ 旧行为：静默覆盖，handler1 消失
app.GET("/users/:id", handler1)
app.GET("/users/:id", handler2)  // handler1 被覆盖，无任何提示
```

**修复**（`router.go` + `options.go`）：

1. `insertNode` 签名改为返回 `bool`（是否发生覆盖），在所有终端赋值点（root、static、param、regex、catchAll）写入 `handlers` **之前**检查原值是否非 nil；
2. `Router` 结构体新增 `logger *slog.Logger` 和 `strictConflict bool`；
3. `Router.Add` 收到 `overwritten == true` 时根据模式二选一：
   - **严格模式**：`panic`，启动阶段立即终止，CI 必失败；
   - **宽松模式**：`slog.Warn`，结构化日志携带 `method` 和 `path`，继续运行（向后兼容）。

```go
if overwritten := insertNode(root, path, handlers); overwritten {
    msg := fmt.Sprintf("astra: route conflict: handler overwritten for %s %s", method, path)
    if r.strictConflict {
        panic(msg)
    }
    r.logger.Warn("astra: route conflict: handler overwritten",
        "method", method,
        "path", path,
    )
}
```

**严格模式触发条件**（任一满足即开启）：

| 触发方式 | 说明 |
|---------|------|
| `astra.WithMode(astra.ModeTest)` | 自动开启，`testutil.NewTestApp()` 默认受保护 |
| `astra.WithStrictConflict()` | 手动开启，适用于开发/staging 环境 |

```go
// 测试中自动严格模式（ModeTest）
app := astra.New(astra.WithMode(astra.ModeTest))
app.GET("/users/:id", handler1)
app.GET("/users/:id", handler2)  // ✅ panic: "astra: route conflict..."

// 生产环境手动严格模式
app := astra.New(astra.WithStrictConflict())

// 生产默认：向后兼容，仅 warn
app := astra.New()
```

行为对比：

| 场景 | 旧实现 | 新实现（默认） | 新实现（严格模式） |
|------|--------|--------------|-----------------|
| 同 method+path 注册两次 | 静默覆盖，无输出 | `WARN astra: route conflict ...`，新 handler 生效 | `panic`，启动终止 |
| 不同 method 注册同 path | — | 无警告（每棵树独立） | 无警告 |
| path 合法首次注册 | — | 无警告 | 无警告 |

新增 4 个专项测试：
- `TestRouting_Conflict_LogsWarning` — `:param` 路径冲突，断言日志含 `route conflict` 和路径，且新 handler 胜出  
- `TestRouting_Conflict_RootPath` — `"/"` 冲突  
- `TestRouting_Conflict_StaticPath` — 静态路径冲突  
- `TestRouting_NoConflict_DifferentMethods` — 同路径不同 method 无误报

#### NetEngine `close()` 三项改进

**① `poller.wait()` 无限阻塞——wakeup 调用缺失**

`poller_linux.go` 的 `EpollWait` 和 `poller_bsd.go` 的 `Kevent` 均使用无限超时（`-1` / `nil`）。旧 `close()` 仅调用 `close(e.quit)` 但未调用 `wakeup()`，导致 event loop goroutine 只能靠 `poller.close()` 关闭 epoll/kqueue fd（产生 EBADF 错误）才能被唤醒，进而走错误路径并打印误导性 `ERROR` 日志：

```
netengine: poller.wait error loop=0 err="bad file descriptor"   // 旧：正常关闭也会出现
```

**修复**：在 `close()` 中 `close(e.quit)` 之后立即调用 `e.poller.wakeup()`，event loop goroutine 从 `poller.wait()` 返回后检查 `e.quit` → 干净退出，不再打印 ERROR。

**② `addCh` 连接在 shutdown 窗口内被孤立**

Accept 循环在 `ln.Accept()` 返回错误之前可能已经 accept 了若干连接并发送到 `addCh`。若 event loop goroutine 在看到 `e.quit` 时这些连接尚未被 `drainAddCh` 处理，它们将永远留在 channel 中：`nc` 从不关闭，`activeConns` 从不归零。

**修复**：在 `close()` 中，signal goroutine → wakeup → **排空 `addCh`**，对每个尚未注册的连接调用 `nc.Close()` 并递减计数器：

```go
drain:
for {
    select {
    case nc := <-e.addCh:
        nc.Close()
        atomic.AddInt64(&e.engine.activeConns, -1)
    default:
        break drain
    }
}
```

**③ 关闭 idle 连接时缺少 `poller.del`**

`closeConn` 和 `workerCloseConn` 都在关闭 `nc` 之前调用 `poller.del(fd)`。旧 `close()` 对 idle 连接只调用 `nc.Close()`，遗漏了 `poller.del`。**修复**：在 Range 内的 `nc.Close()` 之前补加 `e.poller.del(cs.fd)`，与其他关闭路径保持一致。

**④ `run()` 区分预期错误与真实错误**

作为防御性保障（wakeup 极低概率丢失时 `poller.close()` 仍会触发错误返回），在 `poller.wait` 返回错误后先检查 `e.quit`：

```go
if err != nil {
    select {
    case <-e.quit:
        return // 正常 shutdown — 静默退出
    default:
    }
    e.engine.cfg.Logger.Error("netengine: poller.wait error", ...)
    return
}
```

新增两个回归测试：`TestEngine_CleanShutdown_NoErrorLog`（正常关闭后 ERROR 日志缓冲区为空）、`TestEngine_CleanShutdown_AddChDrained`（flood 连接后立即关闭，`ActiveConns() == 0`）。

#### RateLimit `NewRateLimiter`——便捷 API 暴露 stop 函数

旧 `RateLimit(rate, burst)` 包装函数固定使用 `context.Background()`，没有任何方式从外部停止内部 cleanup goroutine。测试中多次调用 `RateLimit()` 会累积 goroutine，即使 `RateLimitWithConfig` 的 `Context` 字段已可解决此问题，仍需手工构造 context。

**新增 `NewRateLimiter`**（向后兼容，`RateLimit` 原函数不变）：

```go
// NewRateLimiter 返回中间件和一个 stop 函数，适合测试或动态路由场景。
mw, stop := middleware.NewRateLimiter(100, 20)
defer stop()   // cleanup goroutine 立即退出
app.Use(mw)
```

| 函数 | Cleanup goroutine 生命周期 | 适用场景 |
|------|--------------------------|---------|
| `RateLimit(rate, burst)` | 随进程 | 顶层长生命周期 middleware |
| `NewRateLimiter(rate, burst)` | `stop()` 调用时 | 测试、动态创建 |
| `RateLimitWithConfig(cfg{Context: ctx})` | `cancel()` 时 | 与 `app.OnStop` 集成 |

新增测试：`TestNewRateLimiter_StopFuncExits`（`stop()` 后 goroutine 数归零）、`TestNewRateLimiter_StillServesAfterStop`（`stop()` 后请求仍正常处理）。

#### `go.work` 外部路径清理——monorepo 自包含性修复（P13）

**问题**：`go.work` 包含 `../astraKron`、`../astraKron/examples/admin`、`../astraKron/examples/worker` 三条仓库外路径，CI 新环境克隆后找不到这些目录，`go build ./...` 直接报路径解析失败。

**修复**：从 `go.work` 移除三条外部 `use` 指令。

```
修复前：
use (
    .
    ../astraKron
    ../astraKron/examples/admin
    ../astraKron/examples/worker
    ./auth
    ...
)

修复后：
use (
    .
    ./auth
    ...
)
```

核实 astra 仓库内无任何文件 import astraKron，移除后本地开发和 CI 均可正常构建，monorepo 完全自包含。

#### `RunStopHooks` 错误静默问题修复——加 slog.Error 日志（P3）

**问题**：`Lifecycle.RunStopHooks` 用 `_ = hooks[i](ctx)` 丢弃所有 stop hook 错误，数据库关闭失败、MQ flush 超时等运维关键错误在生产环境中完全不可见。

**修复**（`lifecycle.go`）：捕获每个 hook 的错误并通过 `slog.Error` 记录：

```go
// 修复前
for i := len(hooks) - 1; i >= 0; i-- {
    _ = hooks[i](ctx)
}

// 修复后
for i := len(hooks) - 1; i >= 0; i-- {
    if err := hooks[i](ctx); err != nil {
        slog.Error("stop hook failed", "err", err)
    }
}
```

所有 hook 仍全量执行（错误容忍语义不变），仅将失败信息写入结构化日志，便于运维排查关闭阶段的资源泄漏。

#### `Lifecycle.RunStopHooks` 改为 LIFO 执行——与 di.Container 语义统一（P14）

**问题**：`App.Lifecycle.RunStopHooks` 按注册顺序（FIFO）执行停止钩子，而 `di.Container.Stop` 按反序（LIFO）执行，同一应用中两套系统行为不一致。LIFO 才是正确的资源释放语义：后初始化的依赖应先关闭（如先关 Redis 连接、再停内存缓存）。

**修复**（`lifecycle.go`）：`RunStopHooks` 从正序迭代改为倒序迭代：

```go
// 修复前
for _, hook := range hooks {
    _ = hook(ctx)
}

// 修复后
for i := len(hooks) - 1; i >= 0; i-- {
    _ = hooks[i](ctx)
}
```

同步更新 `OnStop` 注释说明 LIFO 语义，新增 `lifecycle_test.go` 覆盖三个场景：

| 测试 | 验证点 |
|------|--------|
| `TestLifecycle_RunStopHooks_LIFO` | 3 个钩子以 3→2→1 顺序执行 |
| `TestLifecycle_RunStopHooks_AllRunOnError` | 即使某钩子返回错误，所有钩子全量执行 |
| `TestLifecycle_RunStartHooks_Order` | 启动钩子仍保持 FIFO 注册顺序 |

#### RateLimit 默认行为安全化 + App context 绑定（P7）

**问题**：`SlidingWindowWithConfig` 和 `RouteQuotaMiddleware` 的清理 goroutine 使用 `for range ticker.C` 循环，**没有任何退出路径**，与 `RateLimit` 已经存在的问题同构。三个限流器共同构成测试和动态中间件场景的 goroutine 泄漏。

**修复**：

| 变更 | 文件 | 描述 |
|------|------|------|
| `resolveContext()` 辅助函数 | `ratelimit_advanced.go` | 三段优先级：① App != nil → 创建 ctx + OnStop 自动取消；② 显式 Context → 直接使用；③ 降级 Background |
| `SlidingWindowConfig.App` / `.Context` | `ratelimit_advanced.go` | 新字段，控制清理 goroutine 生命周期 |
| `SlidingWindowWithConfig` goroutine 修复 | `ratelimit_advanced.go` | `for range ticker.C` → `select { case <-ctx.Done(): return; case <-ticker.C: }` |
| `NewSlidingWindow(limit, window)` | `ratelimit_advanced.go` | 返回 `(HandlerFunc, stop)` 对，stop 立即取消清理 goroutine |
| `RouteQuotaConfig.App` / `.Context` | `ratelimit_advanced.go` | 同上，控制 N+1 个清理 goroutine 的生命周期 |
| `NewRouteQuotaMiddleware(cfg)` | `ratelimit_advanced.go` | 返回 `(HandlerFunc, stop)` 对 |
| `RateLimitConfig.App` | `ratelimit.go` | 统一接口，与 SlidingWindow 保持一致 |
| 6 个 goroutine 泄漏专项测试 | `middleware_test.go` | `CleanupStopsOnContextCancel` × 2、`StopFuncExits` × 2、`StillServesAfterStop` × 2，全部通过 `runtime.NumGoroutine` 基线对比验证 |

#### 路由器 table-driven 边界测试 + Saga 补偿失败路径覆盖（P12）

##### 路由器 — `router_table_test.go`

原有路由测试（`astra_test.go`）已覆盖基本方法、参数、正则、通配符，但两个底层边界场景缺少专项断言：

| 边界场景 | 原状 | 修复 |
|---|---|---|
| **childIndex collision** | 无 | `TestRouter_ChildIndexCollision_FourSiblings`：4 个首字节相同的静态子节点，验证 `childIndex[b] = childIndexCollision` → `childMap` 写入 → 每个兄弟节点正确分发 |
| **dispatch 优先级全路径** | 各场景散落 | `TestRouter_DispatchPriority`（20 子用例 table-driven）：在同一路由集上依次验证 static > regex > `:param` > catch-all、childIndex 碰撞后 miss→fallback、多正则无匹配→`:param`、405/404 |

```
TestRouter_DispatchPriority 路由集：
  GET /                        → "root"
  GET /x/foo, /x/far, /x/faz  → 首字节 'f' 碰撞，childIndex[f]=childIndexCollision
  GET /users/list              → 静态，优先于下方正则 / param
  GET /users/{id:[0-9]+}       → 正则，优先于 :param
  GET /users/:id               → param 兜底
  GET /files/*path             → catch-all（值含前导 '/'）
  GET /v/{ver:[0-9]+}          → 正则1
  GET /v/{ver:[a-z]+}          → 正则2
  GET /v/:ver                  → param 兜底（UPPER 大写不匹配两个正则）
  POST /rpc                    → 仅 POST，PATCH 请求触发 405
```

##### Saga — `dtx/saga_test.go` 补充

原有测试已覆盖单/多步正向流程和单次补偿错误；新增三条缺失路径：

| 新增测试 | 覆盖场景 |
|---|---|
| `TestSaga_EmptySaga_Succeeds` | `dtx.New()` 零步骤 → `Succeeded()=true`，`Completed`/`CompensationErrors` 均空 |
| `TestSaga_MultipleCompensationErrors_AllCollected` | b、a 补偿均失败 → `CompensationErrors` 含 2 项，顺序为 errB→errA（倒序执行） |
| `TestSaga_ContextPassedToCompensation` | step-a 内取消 ctx 后仍返回 nil；step-b 以 `ctx.Err()` 失败；step-a 的 Compensate 收到同一已取消的 context |

---

### 多模块 Monorepo 架构的优点与不足

本次引入的 `go.work` + 19 个独立子模块架构，在解决升级耦合的同时也带来了新的权衡，
在选型时需如实评估：

**优点**

- **升级隔离，影响面最小**：升级 `otel/`（OTel SDK 安全补丁）时，`orm/`、`mq/`、`session/` 的依赖树完全不受影响；等同于只升级路由层的 Gin/Echo，CI 回归只需跑 `otel/` 相关测试。
- **按需引入，二进制体积可控**：只用路由 + 缓存的服务不会把 Kafka、k8s client-go、Pulsar 拉进 vendor；相比单一 `go.mod` 方案，可节省数百 MB vendor 目录体积。
- **独立语义版本**：`orm/` 可以在 `v1.3.0` 稳定运行，`otel/` 同期发布 `v2.0.0` 引入 breaking change，二者互不干扰；语义版本承诺（PATCH=bug fix、MINOR=向后兼容、MAJOR=breaking）在每个模块层面都可独立遵守。
- **并行 CI 加速**：单个模块的 `go test ./...` 只拉取该模块的依赖树，CI 矩阵可按模块并行执行，总耗时随模块数线性扩展而非指数增长。
- **本地开发接近零感知**：`go.work` 自动把所有子模块解析到本地路径，跨模块断点调试、IDE 代码跳转与单模块项目体验基本一致；对于互相依赖的 workspace 内子模块，`go.work` 中已提供 `replace` 指令将版本号重定向到本地路径，无需手动 `go get` 中间版本。
- **接口稳定性契约可量化**：对每个子模块单独运行 `golang.org/x/exp/apidiff`，可精确检测 PATCH 版本是否引入 breaking change，自动化 CI 守护。

**不足**

- **仓库管理复杂度上升**：19 个 `go.mod` + 19 个 `go.sum` 需要分别维护；提交跨模块的联动变更时，需要同步更新多个 `go.sum` 文件，合并请求的 diff 体积增大。**以下三个工具已将此成本降到最低**：

  | 工具 | 解决什么问题 |
  |---|---|
  | `scripts/tidy-all.sh` | 按拓扑顺序一键 tidy 全部模块 |
  | `scripts/install-hooks.sh` | 安装 pre-commit hook，提交时自动 tidy 并拦截遗漏的 go.sum 变更 |
  | `scripts/affected-modules.sh` | 检测 PR 中哪些模块受影响（含传递依赖），输出 JSON 矩阵驱动 CI 并行 |
  | `astractl tidy` | CLI 内置拓扑顺序 tidy，无需手动执行 shell 脚本 |
  | `.github/workflows/ci.yml` | 5 阶段 CI：detect → 动态矩阵 tidy/build/vet/test → integration matrix（ClickHouse/ES8/Pulsar 容器 + Apollo mock）→ benchstat 性能回归门禁（退化 ≥10% 阻断合并）→ ci-gate 汇总 |

  **一次性开发者环境安装：**

  ```bash
  # 安装 pre-commit hook（此后每次提交自动检查 tidy）
  bash scripts/install-hooks.sh

  # 也可用 astractl 代替 tidy-all.sh
  astractl tidy
  ```

  **CI 动态矩阵原理**（见 `.github/workflows/ci.yml`）：

  ```
  PR: only/auth changed
        ↓
  affected-modules.sh origin/main  →  [auth]
        ↓
  CI matrix: 1 job（非 19 个）
  ```

  ```
  PR: root module (.) changed
        ↓
  affected-modules.sh  →  [. orm grpc session auth runner client testutil ...]
        ↓
  CI matrix: 8 个 job（并行）
  ```

- **首次 `go mod tidy` 成本**：执行顺序需按依赖拓扑排列（被依赖模块先 tidy）：

  ```bash
  bash scripts/tidy-all.sh   # 或 astractl tidy
  ```

- **版本号协调负担**：跨模块的联动发布（如 `orm/` 依赖根模块 `v0.2.0`，需要先 tag 根模块再 tag `orm/`）需要按依赖拓扑顺序打 tag，手工操作容易出错，建议配合 `release-please` 或 `goreleaser` 自动化。
- **`go.work` 的 `replace` 指令是本地开发必要配置**：workspace 内互相依赖的子模块（如 `session/` 依赖根模块 `v0.1.0`）在发布前没有真实 tag，`go build` 在工作区模式下仍会尝试从 VCS 验证版本号。`go.work` 中必须为这些依赖配置 `replace` 指令将其重定向到本地路径，才能在无 tag 环境下正常构建。发布后删除这些 `replace` 即可。
- **传递依赖冲突需要主动管理**：多模块共用底层依赖（如 gRPC 的 `google.golang.org/genproto`）时，MVS 可能因不同子模块引入不同版本而产生"ambiguous import"冲突。需要在引入旧版本的子模块 `go.mod` 中显式 pin 一个兼容的新版本来让 MVS 选择正确版本，而非被动等待冲突在构建时暴露。
- **`go work sync` 不等于发布验证**：`go.work` 只对本地开发有效；当用户以 `GOWORK=off go get github.com/astra-go/astra/orm@v0.1.0` 方式引用时，子模块必须已发布对应 tag，否则会出现 "reading go.mod at revision" 错误。开发阶段这不是问题，但**正式发布前必须为每个子模块打 tag**。
- **Go 工具链限制**：`go.work` 在 Go 1.18 引入，若用户使用 Go 1.18–1.24，workspace 可用但部分 `go work` 子命令行为略有差异；鉴于 Astra 已要求 Go 1.25+，此限制在实际使用中可忽略。

**适用判断**

```
推荐使用多模块子模块：
  ✓ 团队同时维护多个服务，各自需要不同版本的 OTel 或 GORM
  ✓ 对 CI 时间敏感，希望按模块并行测试减少整体耗时
  ✓ 需要对外发布稳定 API，并以语义版本向用户承诺兼容性
  ✓ 项目长期维护（> 2 年），依赖树庞大、升级频繁

接受单模块方案即可：
  ✓ 小团队 / 单服务，所有人共享相同的依赖版本
  ✓ 项目处于原型或快速迭代阶段，版本稳定性不是首要关切
  ✓ 不需要发布为公共库，GOWORK=on 本地开发已完全够用
```

---

### 综合定位

```
轻量/高性能路由  ←──────────────────────────────→  微服务全栈

    Gin/Echo          Astra            go-zero / Kratos
   (极简，插件化)   (全功能，开箱即用)   (微服务治理，代码生成)

Astra 的最佳场景：
  ✓ 中型单体 / 模块化单体 API 服务
  ✓ 需要任务队列、MQ、缓存等基础设施但不想逐个选型集成
  ✓ 已有 Gin 项目，希望低成本引入熔断/限流/可观测性
  ✓ 团队熟悉标准 Go，不想引入代码生成工具链
  ✓ API + Web 混合服务（JSON API + HTML 模板页面共存）
  ✓ 需要 gRPC + HTTP 双栈且要求分布式链路追踪贯穿全链路
  ✓ 对延迟敏感、需要自适应熔断（错误率 + P99）的高可用服务
  ✓ 长期维护项目，需要对 OTel / GORM / MQ 独立升级，降低版本耦合风险
  ✓ 高并发长连接服务（连接数 >> 并发请求数），通过 RunReactor 启用 epoll/kqueue Reactor 引擎，
    万级空闲连接的 goroutine 开销近 0，显著优于 Gin/Echo 的 goroutine-per-connection 模型
  ✓ 需要轻量 DI 容器管理依赖图（内置 `di/` 包，泛型 Provide[T]/Invoke[T]，无代码生成）
  ✓ 中型微服务集群，需要 P2C 自适应负载均衡 + 被动健康检查 + 就近路由

暂不适合的场景：
  ✗ 超大规模微服务集群（1000+ 实例，建议 go-zero / Kratos + 服务网格）
  ✗ 需要 MVC Admin 后台等重模板功能（建议 Beego）
  ✗ Go 版本锁定在 1.24 以下的存量项目
  ✗ 需要 streaming gRPC（建议 Kratos）
  ✗ 对框架稳定性要求极高、不能承受未知风险的核心链路（建议 Gin/Echo + 自选组件）
```

---

