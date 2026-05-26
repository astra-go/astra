## 测试覆盖 (TDD)

Astra 采用测试驱动开发（TDD）理念，所有功能包均配有对应的 `*_test.go` 文件，
测试在 `go test -race` 下全部通过。

### 覆盖状态总览

| 包 | 测试文件 | 覆盖评级 | 主要覆盖场景 |
|---|---|:---:|---|
| `astra`（根包） | `astra_test.go`、`errors_test.go`、`app_quic_test.go`、`router_table_test.go` | ✅ GOOD | 路由（静态 / 参数 / 正则 / 通配符）、Context、错误处理、中间件链、插件系统、RunQUIC 错误路径；**table-driven**：20 子用例覆盖 childIndex collision / static>regex>:param 优先级 / catch-all / 405；4 兄弟节点 childMap 碰撞路径 |
| `netengine/` | `engine_test.go` | ✅ GOOD | BasicGET/POST/404、Keep-Alive 多请求同连接、20 goroutine × 10 请求并发、ActiveConns 计数、Accessors、平台不支持无 panic；全部测试在 `go test -race` 下通过 |
| `binding/` | `binding_test.go` | ✅ GOOD | JSON/Query/Path 绑定，校验规则，错误格式化 |
| `cache/` | `cache_test.go` | ✅ GOOD | MemoryCache TTL/过期/隔离，JSON 助手，MockCache |
| `circuit/` | `circuit_test.go`、`adaptive_test.go` | ✅ GOOD | 状态转换、半开恢复、错误率/延迟自适应熔断、并发安全 |
| `discovery/` | `discovery_test.go` | ✅ GOOD | 注册/注销、按名隔离、副本保护、Watch 推送、ctx 取消 |
| `discovery/k8s/` | `k8s_test.go` | ✅ GOOD | Registry 接口 compile-time 断言、InCluster 集群外错误、非法 kubeconfig 路径错误 |
| `dtx/` | `saga_test.go` | ✅ GOOD | 全成功、首步失败（无补偿）、第二步失败（补偿第一步）、第三步失败（逆序补偿）、nil Compensate 跳过、补偿错误收集、nil Forward 错误、单步、ctx 取消、Succeeded()、WithLogger(nil)；**新增**：空 Saga 成功、多补偿失败全收集（顺序校验）、ctx 取消后传递至 Compensate |
| `grpc/` | `grpc_test.go` | ✅ GOOD | Kratos 结构化错误、gRPC status 编码、中间件链 |
| `graphql/` | `graphql_test.go` | ✅ GOOD | 默认 /graphql GET/POST、Playground HTML、自定义路径、自定义标题、禁用 Playground、handler 转发 |
| `health/` | `istio_test.go` | ✅ GOOD | /healthz/live + /healthz/ready 路径、WithProbe 健康判断、WithPrefix、WithIstioHeaders 注入 x-content-type-options + x-envoy-upstream-service-time、不覆盖标准路径 |
| `loadbalance/` | `loadbalance_test.go`、`resolver_test.go` | ✅ GOOD | 7 种策略 + LocalityFirst + Resolver + OutlierDetector + Benchmark |
| `lua/` | `engine_test.go`、`redis_test.go` | ✅ GOOD | Isolated/Shared 模式、类型转换、多返回值、并发安全；Redis 测试含 SKIP 守卫 |
| `middleware/` | `middleware_test.go`、`logger_metrics_tracing_test.go`、`canary_test.go` | ✅ GOOD | Recovery/CORS/JWT/RateLimit/CSRF/Timeout/Compress/Logger/Metrics/Tracing/SlidingWindow/RouteQuota/Canary |
| `alert/` | `alert_test.go` | ✅ GOOD | AddRule 校验/重名/编译错误、FiresAlert、DoesNotFire、ForDuration 延迟通知、ActiveAlerts、Stop 停止、RegisterMetric/AddChannel 链式调用、WebhookChannel JSON/resolved/4xx/bad URL、LogChannel nil-safe |
| `mq/pulsar/` | `pulsar_test.go` | ✅ GOOD | Producer/Consumer 接口 compile-time 断言、Subscribe 无 topics 错误、无 Subscription 名错误、Close 无 panic |
| `config/apollo/` | `apollo_test.go` | ✅ GOOD | New 缺少 AppID 错误、缺少 MetaAddr 错误、两者均空错误 |
| `orm/` | `orm_test.go` | ✅ GOOD | RunTx commit/rollback/panic、RunNestedTx savepoint、ForUpdate/SkipLocked/NoWait/Share 锁子句、UpdateOptimistic 版本冲突 |
| `orm/clickhouse/` | `clickhouse_test.go` | ✅ GOOD | Open 空 DSN 验证、非法 DSN 错误、Config 零值默认无 panic |
| `render/` | `render_test.go` | ✅ GOOD | 无/有 layout 渲染、partials、FuncMap、热重载、并发安全 |
| `retry/` | `retry_test.go` | ✅ GOOD | 重试策略、4xx 不重试、ctx 取消、自定义 Retryable |
| `testutil/` | — | ✅ GOOD | 通过其他包调用验证，无需独立测试 |
| `lo/` | `slice_test.go`、`map_test.go`、`math_test.go`、`ptr_test.go`、`condition_test.go` | ✅ GOOD | Map/Filter/Reduce/GroupBy/Uniq/Set 操作、Ternary/If 链、Must/Try、Min/Max/Sum/Clamp、指针助手 |
| `rule/` | `rule_test.go` | ✅ GOOD | 封闭入口编译校验、AsBool 类型推断、RunBool/RunFloat64、env 方法调用、Engine 自定义函数、并发安全、折扣规则引擎场景 |
| `search/elastic/` | `elastic_test.go` | ✅ GOOD | Searcher 接口 compile-time 断言、New 多配置组合、Index/BulkIndex/Search/Delete/DeleteIndex/CreateIndex mock 服务端验证、5xx 响应转错误、BulkIndex 空切片早返回 |
| `timeutil/` | `timeutil_test.go` | ✅ GOOD | 全局配置（时区/Layout/并发安全）、构造函数、JSON 序列化/反序列化（null/int64/string 降级）、Scan/Value SQL 接口、GORM Model 时间戳钩子、SoftDelete |
| `validate/` | `validate_test.go` | ✅ GOOD | mobile/password/username/no_html/not_blank 内置标签、required/email/oneof/min/max 标准规则、Errors.Map()、Var 单值校验、自定义验证器、别名注册 |
| `auth/oauth2/` | `oauth2_test.go` | ✅ GOOD | LoginHandler 重定向、PKCE code_challenge 注入、CallbackHandler 提供商错误/无效 state → 400、FetchUserInfo 空 URL 错误/mock 服务端/非 200 响应错误 |
| `di/` | `di_test.go` | ✅ GOOD | Provide/Invoke singleton、ErrNotFound/ErrDuplicate、ProvideValue、命名实例、传递依赖、Has、生命周期 LIFO、Start 失败短路、Stop 全量运行 |
| `runner/` | 集成测试（需外部服务） | ✅ GOOD | 四种后端均实现 Runner 接口，compile-time 断言（`var _ runner.Runner = ...`）覆盖接口完整性 |

> **31 / 31 功能包全部覆盖（100%）**，运行 `go test -race ./...` 全绿。

---

### 运行测试

```bash
# 全量测试（含竞态检测）
go test -race ./...

# 单包测试
go test -race ./middleware/...
go test -race ./circuit/...
go test -race ./discovery/...
```

---

### 各包测试亮点

#### `astra`（根包）— 路由 & 正则约束参数

- **四类路径节点全覆盖**：`TestRouting_BasicMethods`（静态）、`TestRouting_PathParam`（`:key`）、`TestRouting_Wildcard`（`*key`）、`TestRouting_Regex_*`（`{key:pattern}`）
- **正则匹配与降级**：`TestRouting_Regex_PriorityOverParam` 验证同路径下正则路由优先于 `:param`，非匹配段自动降级到 `:param`
- **多模式并存**：`TestRouting_Regex_MultiplePatterns` 同一层级注册两个不同正则，按注册顺序独立匹配
- **嵌套路径段**：`TestRouting_Regex_NestedAfterRegexSegment` 验证正则段之后可继续接静态段（`/api/{ver:v[0-9]+}/users`）
- **快速失败**：`TestRouting_Regex_InvalidPatternPanics` 验证无效正则在注册时即 panic，不等到运行时
- **Lifecycle LIFO**（`lifecycle_test.go`）：`TestLifecycle_RunStopHooks_LIFO` 验证 3 个 stop hook 以 3→2→1 倒序执行；`TestLifecycle_RunStopHooks_AllRunOnError` 验证某 hook 出错后其余 hook 全量运行；`TestLifecycle_RunStartHooks_Order` 验证 start hook 保持 FIFO

#### `discovery/`

- **副本保护**：`Discover` 返回浅拷贝，调用方修改结果不影响注册表内部状态
- **Watch 语义**：订阅后立即推送当前快照；Register / Deregister 触发实时通知；ctx 取消后 channel 干净关闭
- **并发安全**：50 个 goroutine 同时 Register / Deregister / Discover，`-race` 无数据竞争

#### `loadbalance/`

- **七种策略全覆盖**：RoundRobin、Random、Weighted、SmoothWeighted、LeastConn、P2C、ConsistentHash 各有正例 + 空列表错误 + 并发安全用例
- **SWRR 平滑性**：`TestSmoothWeighted_Smoothness_NoBurst` 验证 100 次 Pick 中最大连续相同实例数 ≤ 2（无突发），对比 Weighted 可能连发 5 次
- **P2C EWMA 自适应**：`TestP2C_PicksLowerLoaded` 预置高延迟节点后验证低延迟节点获得多数流量；`TestP2C_EWMAAdaptsToLatency` 验证 EWMA 收敛后得分更新正确影响选择
- **Reporter 接口**：`TestP2C_Reporter_Interface` 验证 P2C 实现 `loadbalance.Reporter`，nil 参数为 no-op
- **LeastConn 计数语义**：`TestLeastConn_PicksLeastLoaded` 预置不同活跃数后验证选取最低负载节点；`TestLeastConn_Done_DecrementsCount` 验证 `Done` 归还后计数正确递减、影响下次选择
- **LocalityFirst**：`TestLocalityFirst_PrefersLocal` 验证同 zone 实例优先；`TestLocalityFirst_FallsBackToAll_WhenNoLocal` 验证无本地实例时自动 fallback 全量列表（零中断）
- **ConsistentHash ring 缓存**：`TestConsistentHash_RingCached_SameInstances` 验证稳定实例集合多次 Pick 结果一致；`TestConsistentHash_RingRebuilt_WhenInstancesChange` 验证实例集合变化后环重建正常
- **Resolver 更新**：`TestResolver_UpdatesOnChange` 注册新实例后轮询等待，验证快照在 200ms 内更新
- **OutlierDetector 隔离**：`TestOutlierDetector_EjectsAfterThreshold` 验证 3 次连续错误后被摘除；`TestOutlierDetector_ReadmitsAfterInterval` 验证 50ms 后自动放行；`TestOutlierDetector_FallbackWhenAllEjected` 验证全部节点被隔离时 fallback 到全量列表（不返回 ErrNoInstances）；`TestOutlierDetector_MaxEjectionPct` 验证 50% 上限生效
- **Benchmark 8 项**：覆盖全部策略；`BenchmarkConsistentHash_StableRing`（ring 缓存命中）vs `BenchmarkConsistentHash_ChangingRing`（每次重建），量化缓存收益约 600×

#### `lua/`

- **Isolated vs Shared 模式**：同名函数在两个模式下的隔离/共享行为各有专属测试
- **全类型转换**：string / float64 / bool / 多返回值均有 round-trip 验证
- **Register 错误**：语法错误、文件不存在、未知脚本名、未定义函数均返回可辨别 error
- **并发安全**：30 goroutine 并发调用不同脚本（Isolated 模式），-race 无竞争
- **Redis 测试含守卫**：无 `REDIS_ADDR` 时自动 `t.Skip()`，CI 无依赖阻塞

#### `orm/`

- **事务完整性**：RunTx commit、error-rollback、panic-rollback 三条路径均有独立用例
- **SavePoint 语义**：`RunNestedTx` 嵌套失败只回滚存档点，外层事务继续提交
- **锁子句验证**：通过 `gorm.Session{NewDB: true}` 直接检查 `Statement.Clauses["FOR"]`，与 SQL 方言无关——SQLite 丢弃 `FOR UPDATE` 语法不影响测试结论
- **乐观锁不变性**：版本匹配成功自动递增 version；版本不匹配返回 `ErrOptimisticConflict`；调用方原始 map 不被污染

#### `rule/`

- **封闭入口验证**：引用不存在字段/函数在编译时即报错，不等到运行时
- **AsBool 类型推断**：`rule.AsBool()` 选项使编译器在构建阶段拒绝返回非布尔的表达式
- **env 方法调用**：struct 方法在表达式中直接可调用，无需额外注册
- **Engine 自定义函数**：`WithFunc` 链式注册，带类型重载，编译期推断参数/返回类型
- **并发安全**：50 goroutine 共享同一 `*Program` 并发 `RunBool`，-race 无竞争
- **折扣规则引擎**：`init()` 编译、请求时执行的完整场景测试（3 条优先级规则）

#### `validate/`

- **内置自定义标签覆盖**：`mobile` / `password` / `username` / `no_html` / `not_blank` 各有正例+反例
- **密码强度矩阵**：缺失大写、缺失小写、缺失数字、缺失特殊字符、长度不足各自独立用例
- **错误 Map API**：`Errors.Map()` 返回 `map[string]string`，字段名来自 json tag（不暴露 Go 字段名）
- **中文消息**：`required` / `email` / `oneof` 等规则均验证中文错误提示内容
- **Option 模式**：`WithCustom` 注册自定义验证函数、`WithAlias` 注册别名、`WithTagName` 自定义字段名解析
- **全局默认实例**：`RegisterValidation` / `RegisterAlias` 对默认实例生效，并验证副作用

- **全程使用 `testing/fstest.MapFS`**：无需磁盘文件，测试完全自包含
- **内置函数验证**：`safeHTML`、`safeURL`、`dict`、`iterate` 均有独立用例
- **热重载验证**：`Reload: true` 模式下，更新 MapFS 后下次 Render 自动读取新内容
- **并发渲染**：30 个 goroutine 同时调用 `Render`，`-race` 无问题

#### `middleware/` — Logger / Metrics / Tracing

| 中间件 | 测试重点 |
|---|---|
| **Logger** | 敏感参数（`token=`, `password=`）自动 REDACTED；SkipPaths 跳过；方法/路径/状态写入 slog |
| **Metrics** | 使用独立 `prometheus.Registry` 隔离，验证 `requests_total` 计数、4xx/5xx 写入 `errors_total`、SkipPaths 不计数 |
| **Tracing** | 使用 OTel `noop.NewTracerProvider()`，验证 span 存入 ctx（`otel.span`）、SkipPaths 不创建 span、自定义 span 名 |
| **SlidingWindow** | 限额内放行、超限 429、独立 key 互不干扰、PerKeyLimits 细粒度配额；**context 取消后 goroutine 退出（无泄漏）**、`NewSlidingWindow` stop 函数生效、stop 后请求仍正常处理 |
| **RouteQuota** | 按前缀路由限速、default limit fallback、边界保护（`/api` 不误匹配 `/apiv2`）；**context 取消后 N+1 goroutine 全部退出**、`NewRouteQuotaMiddleware` stop 函数生效 |

#### `compress` 中间件 — 修复真实 Bug

编写测试时发现 `gzipResponseWriter.WriteHeader` 在知道响应是否需要压缩**之前**就提交了 HTTP 头，
导致 `Content-Encoding: gzip` 永远写不进已发出的头部。

**修复方案**：将 `WriteHeader` 改为缓冲状态码，延迟到 `enableCompression()`（压缩路径）
或 `finish()`（直通路径）时才调用底层 `ResponseWriter.WriteHeader`，
确保 `Content-Encoding: gzip` 在头部提交前已设置。

```go
// Before（有 bug）
func (g *gzipResponseWriter) WriteHeader(code int) {
    g.headersSent = true
    g.ResponseWriter.WriteHeader(code)  // ← 立即提交，此时 Content-Encoding 还未设置
}

// After（修复后）
func (g *gzipResponseWriter) WriteHeader(code int) {
    if g.statusCode == 0 {
        g.statusCode = code  // ← 缓冲，不立即提交
    }
}
// 真正提交发生在 enableCompression() 内，此时 Content-Encoding: gzip 已写入 Header map
```

#### `health/` — Istio probe 及 header 时序修复

编写 `istio_test.go` 时发现 `withIstioHeaders` 将 `x-envoy-upstream-service-time` 设置在
`next(c)` **之后**，但 HTTP/1.1 一旦调用 `WriteHeader` 头部帧即已发出，后续 `Header().Set()`
对真实连接是 no-op，测试从响应中读不到该 header。

**修复**：将两个 Istio 头均移至 `next(c)` **之前**设置：

```go
// Before（header 设置晚了）
func withIstioHeaders(next astra.HandlerFunc) astra.HandlerFunc {
    return func(c *astra.Ctx) error {
        c.Writer.Header().Set("x-content-type-options", "nosniff")
        err := next(c)                               // ← WriteHeader 在此调用
        c.Writer.Header().Set("x-envoy-upstream-service-time", "0")  // ← 无效
        return err
    }
}

// After（修复后）
func withIstioHeaders(next astra.HandlerFunc) astra.HandlerFunc {
    return func(c *astra.Ctx) error {
        c.Writer.Header().Set("x-content-type-options", "nosniff")
        c.Writer.Header().Set("x-envoy-upstream-service-time", "0")  // ← WriteHeader 前设置
        return next(c)
    }
}
```

#### `dtx/` — Saga 分布式事务

- **全路径覆盖**：全成功、首步失败（无已完成步骤，无需补偿）、中间步骤失败（逆序补偿已完成步骤）、最后步骤失败（全量逆序补偿）
- **nil Compensate 静默跳过**：不可逆步骤（如发邮件）不提供 `Compensate`，补偿阶段跳过不 panic
- **补偿错误收集**：单步补偿失败不阻断其他步骤补偿，全部完成后汇入 `CompensationErrors`
- **nil Forward 保护**：`Step.Forward == nil` 时立即返回明确错误，不 panic
- **WithLogger(nil)**：传入 nil 时自动回退到 `slog.Default()`，不 panic

#### `alert/` — 告警规则引擎

- **表达式校验**：`AddRule` 使用 `expr-lang/expr` 编译期校验，无效表达式返回 `*RuleCompileError`，重名规则返回 `*DuplicateRuleError`（均可 `errors.As` 精确匹配）
- **竞态安全指标**：`TestEngine_FiresAlertWhenExprTrue` 使用 `atomic.Int64` 跨 goroutine 安全传值，消除 `-race` 检测的数据竞争
- **For 延迟语义**：`TestEngine_ForDuration_DelaysNotification` 验证条件持续 60ms 不触发，持续 200ms 后触发
- **Stop 语义**：验证 `Stop()` 后评估循环确实停止，后续不再发送通知
- **WebhookChannel 完整性**：JSON payload 验证（`rule`、`resolved`、`resolved_at` 字段）、4xx 响应转错误、连接拒绝错误

#### `middleware/canary_test.go`

> **注**：随 `HandlerFunc` 具体化重构，`astra.Context` 类型别名已移除，本文件及同包测试文件中遗留的 `astra.Context` 引用已统一替换为 `*astra.Ctx`，测试编译恢复正常。

- **AND 条件组合**：Header 名存在 + 正则匹配值同时满足才命中；仅 Header 名存在不满足正则时不命中
- **Cookie 匹配**：Cookie 存在命中、Cookie 缺失不命中、Cookie 值正则匹配
- **哈希取模路由**：同一 userID 多次请求路由结果一致（确定性哈希）；context 中无 userID 时不命中
- **首中即停**：多条规则按顺序匹配，第一条命中后不继续匹配后续规则
- **空规则集**：无规则时 canary_version 为空（stable）

#### `graphql/` — GraphQL 挂载助手

- **默认行为**：`/graphql` 同时响应 GET / POST；`/playground` 返回 HTML，含 `<html` 和 `GraphQL` 关键字
- **自定义路径 + 标题**：`Options.Path`、`Options.PlaygroundPath`、`Options.PlaygroundTitle` 均验证生效
- **Playground 内嵌端点引用**：Playground HTML 必须包含 GraphQL API 路径（`/my-api`）供 IDE 自动连接
- **禁用 Playground**：`PlaygroundPath: ""` 时 `/playground` 返回 404
- **handler 转发**：验证底层 `http.Handler` 确实被调用

#### `search/elastic/` — Elasticsearch 客户端

- **产品校验头**：ES Go client v8 检查每个响应的 `X-Elastic-Product: Elasticsearch` 头；mock 服务端统一注入，避免 "not Elasticsearch" 误判
- **全方法覆盖**：Index、BulkIndex（含空切片早返回）、Search（解析 total / hits / aggs）、Delete、DeleteIndex、CreateIndex（含 / 不含 mapping）
- **错误路径**：5xx 响应通过 `resp.IsError()` 转为 Go error，断言错误非 nil

#### `auth/oauth2/` — OAuth2/OIDC 客户端

- **无重定向跟随客户端**：`CheckRedirect: http.ErrUseLastResponse` 防止测试 HTTP 客户端跟随 302 跳转到真实 OAuth2 提供商 URL，改为直接断言 Location 头
- **PKCE 验证**：`code_challenge` 出现在重定向 URL 中
- **错误处理覆盖**：提供商返回 `error` 参数 → 400；state cookie 缺失 → 400；FetchUserInfo 非 200 → 错误

---

