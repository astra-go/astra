# Astra 框架中间件过度工程化优化方案

## 1. 现状分析

### 1.1 问题定义

Astra 将 22 个中间件全部内置在 `middleware/` 包中（5673 行代码 / 28 个文件），与核心框架共享同一个 `go.mod`。这意味着：

- **依赖膨胀**：只用 `Recovery()` 的项目也会拉取 `golang-jwt`、`prometheus`、`quic-go`、`opentelemetry` 等重量级依赖
- **编译时间**：修改任意中间件触发全包重编译
- **版本耦合**：JWT 库升级必须和核心框架同版本发布
- **职责模糊**：`middleware` 包承载了从基础设施（Recovery/RequestID）到业务逻辑（Tenant/Canary/Audit）的所有层次

### 1.2 中间件分层画像

根据**外部依赖**和**业务深度**两个维度，22 个中间件可归为四层：

| 层级 | 中间件 | 行数 | 外部依赖 | 典型使用率 |
|------|--------|------|----------|-----------|
| **L0 基础设施** | Recovery, RequestID, Secure, Timeout, Sanitize | 376 | 无 | 90%+ |
| **L1 Web 必备** | CORS, CSRF, CSP, Compress, Logger | 982 | 无 | 70%+ |
| **L2 可观测性** | Metrics, Tracing, Audit | 665 | prometheus, otel | 30-50% |
| **L3 业务/集成** | JWT, RateLimit, APIKey, Signature, Tenant, IPFilter, Canary, LongPoll, Pprof | 2305 | jwt, redis | 10-30% |

**核心矛盾**：L0 的 376 行代码被 L3 的 2305 行及其重量级依赖绑架了。

### 1.3 依赖传导链

```
用户 import "astra/middleware" 的 Recovery()
  → 编译整个 middleware 包
    → 拉取 golang-jwt/jwt/v5        (JWT 中间件)
    → 拉取 prometheus/client_golang   (Metrics 中间件)
    → 拉取 opentelemetry/*            (Tracing 中间件)
    → 拉取 go-redis/redis             (Redis 构建, 已用 tag 隔离)
```

一条 `import _ "astra/middleware"` 的依赖链总重约 **15MB+ 传递依赖**。

---

## 2. 优化方案：三层拆分 + 接口解耦

### 2.1 总体架构

```
astra/
├── middleware/              ← L0+L1（零/轻依赖，编译到核心）
│   ├── recovery.go
│   ├── requestid.go
│   ├── secure.go
│   ├── timeout.go
│   ├── sanitize.go
│   ├── cors.go
│   ├── csrf.go
│   ├── csp.go
│   ├── compress.go
│   └── logger.go
├── middleware/observability/ ← L2 子模块（独立 go.mod）
│   ├── metrics.go
│   ├── tracing.go
│   └── audit.go
└── middleware/security/      ← L3 子模块（独立 go.mod）
    ├── jwt.go
    ├── jwt_cache.go
    ├── ratelimit.go
    ├── ratelimit_advanced.go
    ├── apikey.go
    ├── signature.go
    ├── tenant.go
    ├── ipfilter.go
    ├── canary.go
    ├── longpoll.go
    └── pprof.go
```

### 2.2 依赖隔离效果

| 模块 | go.mod 直接依赖 | 传递依赖大小 | 编译时间影响 |
|------|-----------------|-------------|-------------|
| `middleware/` (L0+L1) | astra core only | ~0 | 零 |
| `middleware/observability` | astra + prometheus + otel | ~8MB | 仅 L2 用户 |
| `middleware/security` | astra + golang-jwt + go-redis(tag) | ~4MB | 仅 L3 用户 |

**效果**：只用 Recovery+CORS 的项目，依赖从 ~15MB 降到 0MB。

### 2.3 导入路径变化（Breaking Change）

```go
// Before
import "github.com/astra-go/astra/middleware"
app.Use(middleware.JWT(secret))
app.Use(middleware.Metrics())

// After
import (
    "github.com/astra-go/astra/middleware"
    secur "github.com/astra-go/astra/middleware/security"
    obs "github.com/astra-go/astra/middleware/observability"
)
app.Use(middleware.Recovery())   // L0: 不变
app.Use(middleware.CORS())       // L1: 不变
app.Use(secur.JWT(secret))       // L3: 改路径
app.Use(obs.Metrics())           // L2: 改路径
```

---

## 3. 详细拆分策略

### 3.1 L0 基础设施层 — 留在核心

**保留理由**：使用率 90%+，零外部依赖，属于框架"开箱即用"的最小承诺。

| 中间件 | 行数 | 理由 |
|--------|------|------|
| Recovery | 72 | 防止 panic 崩溃，必须开箱即用 |
| RequestID | 58 | 链路追踪基础，几乎所有项目都需要 |
| Secure | 153 | 安全头部，Secure by Default 承诺 |
| Timeout | 41 | 请求超时保护，基础设施级 |
| Sanitize | 52 | 敏感参数脱敏工具函数，被 Logger/Tracing 共用 |

**注意**：Sanitize 目前是内部工具函数（`redactQuery`/`sanitizeRawQuery`），被 Logger 和 Tracing 引用。拆分后 Logger 留在 L1，Tracing 去 L2——需要将 Sanitize 提取为共享内部包或将其核心函数移入 `internal/sanitize/`。

### 3.2 L1 Web 必备层 — 留在核心

**保留理由**：Web 应用 70%+ 使用率，零外部依赖（OTel 依赖已通过接口解耦），是"Astra 是 Web 框架"的基本能力证明。

| 中间件 | 行数 | 理由 |
|--------|------|------|
| CORS | 195 | 跨域是 Web 基本需求 |
| CSRF | 304 | 安全默认承诺的一部分 |
| CSP | 129 | 安全默认承诺的一部分 |
| Compress | 252 | 响应压缩，标准库实现 |
| Logger | 154 | 访问日志，依赖 Sanitize |

**Logger 的 OTel 依赖处理**：Logger 目前可选输出 OTel span events。拆分策略：
- Logger 核心逻辑留在 L1，OTel 输出改为 **可选接口注入**
- 定义 `LogExporter` 接口，OTel 实现放到 `observability/` 包
- L1 的 Logger 默认只写 slog，零外部依赖

### 3.3 L2 可观测性层 — 独立子模块

**拆分理由**：重量级依赖（prometheus ~8MB + otel），使用率 30-50%，只有上生产的项目才需要。

| 中间件 | 行数 | 拆分后依赖 |
|--------|------|-----------|
| Metrics | 229 | prometheus/client_golang |
| Tracing | 216 | opentelemetry/* |
| Audit | 220 | 零（但语义属于可观测性） |

**子模块结构**：

```
middleware/observability/
├── go.mod              module github.com/astra-go/astra/middleware/observability
├── metrics.go
├── tracing.go
├── audit.go
└── internal/           # 共享内部工具
    └── sanitize.go     # 从核心包复制/引用
```

### 3.4 L3 业务/集成层 — 独立子模块

**拆分理由**：最重的代码量（2305 行），最重的依赖（jwt + redis），最低的使用率（10-30%）。

| 中间件 | 行数 | 拆分后依赖 |
|--------|------|-----------|
| JWT | 549 | golang-jwt/jwt/v5 |
| JWT Cache | 107 | 无（接口层） |
| JWT Cache Multilevel | 112 | go-redis (build tag) |
| JWT Cache Redis | 342 | go-redis |
| JWT Generate | 38 | golang-jwt/jwt/v5 |
| RateLimit | 211 | 无 |
| RateLimit Advanced | 441 | 无 |
| RateLimit Redis | 542 | go-redis |
| APIKey | 125 | 无 |
| Signature | 259 | 无 |
| Tenant | 215 | 无 |
| IPFilter | 206 | 无 |
| Canary | 177 | 无 |
| LongPoll | 173 | 无 |
| Pprof | 101 | 无 |

**进一步细分方案**（可选，更激进的拆分）：

```
middleware/security/
├── go.mod
├── auth/               # 认证子包
│   ├── jwt.go
│   ├── jwt_cache.go
│   ├── jwt_cache_redis.go   (build tag)
│   ├── jwt_generate.go
│   └── apikey.go
├── ratelimit/          # 限流子包
│   ├── ratelimit.go
│   ├── ratelimit_advanced.go
│   └── ratelimit_redis.go   (build tag)
├── integrity/          # 完整性子包
│   └── signature.go
└── governance/         # 治理子包
    ├── tenant.go
    ├── ipfilter.go
    ├── canary.go
    └── longpoll.go
```

---

## 4. 关键解耦技术

### 4.1 Sanitize 共享问题

**现状**：`sanitize.go` 提供 `redactQuery`/`sanitizeRawQuery`，被 L1 的 Logger 和 L2 的 Tracing 同时引用。

**方案 A — 内部共享包**（推荐）：

```
astra/internal/sanitize/
├── sanitize.go        # 从 middleware/sanitize.go 移入
└── sanitize_test.go
```

- `middleware/` 和 `middleware/observability/` 都 import `astra/internal/sanitize`
- Go 的 `internal/` 机制保证不暴露给外部用户
- 唯一成本：observability 子模块需要 `replace` 指向核心仓库

**方案 B — 接口注入**：

```go
// middleware/logger.go
type QueryRedactor interface {
    RedactQuery(rawQuery string) string
}

type LoggerConfig struct {
    // QueryRedactor optionally redacts sensitive params from log output.
    // When nil, a built-in redactor using DefaultSensitiveParams is used.
    QueryRedactor QueryRedactor
}
```

- L1 的 Logger 内置默认实现（无依赖）
- L2 的 Tracing 注入 OTel-aware 的实现
- 成本：每个消费者需要自己实现或引入

### 4.2 Logger OTel 解耦

**现状**：`logger.go` 直接 `import "go.opentelemetry.io/otel/trace"` 获取 span context。

**方案 — 接口注入**：

```go
// middleware/logger.go
type SpanExtractor interface {
    SpanID(r *http.Request) string
    TraceID(r *http.Request) string
}

type LoggerConfig struct {
    // SpanExtractor extracts distributed tracing IDs from the request.
    // When nil, trace/span IDs are not logged.
    // Use middleware/observability.otelSpanExtractor to enable OTel integration.
    SpanExtractor SpanExtractor
}
```

```go
// middleware/observability/otel_bridge.go
package observability

import "github.com/astra-go/astra/middleware"

type otelSpanExtractor struct{}

func (e *otelSpanExtractor) SpanID(r *http.Request) string {
    ctx := r.Context()
    span := trace.SpanFromContext(ctx)
    if !span.SpanContext().IsValid() { return "" }
    return span.SpanContext().SpanID().String()
}

func OTelSpanExtractor() middleware.SpanExtractor {
    return &otelSpanExtractor{}
}

// 用法:
//   app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
//       SpanExtractor: observability.OTelSpanExtractor(),
//   }))
```

### 4.3 RateLimit App 依赖解耦

**现状**：`ratelimit.go` 和 `ratelimit_advanced.go` 的 Config 有 `App *astra.App` 字段。

**方案 — 接口注入**：

```go
type RateLimiter interface {
    Allow(key string) bool
    Reserve(key string) bool
}

type RateLimitConfig struct {
    // Limiter provides the rate limiting backend.
    // When nil, an in-memory token bucket limiter is used.
    Limiter RateLimiter
    // KeyFunc extracts the rate limit key from the request.
    KeyFunc func(*astra.Ctx) string
    // ExceededHandler is called when rate limit is exceeded.
    ExceededHandler astra.HandlerFunc
}
```

- 去掉 `App *astra.App` 字段，改为纯接口
- Redis 后端实现 `RateLimiter` 接口，通过依赖注入传入
- 核心 RateLimit 完全脱离 `astra.App`，可独立测试

---

## 5. 实施路线图

### Phase 1 — 内部重构（零 Breaking Change）

**目标**：在不改 API 的前提下，将外部依赖从核心中间件中抽走。

1. **提取 `internal/sanitize/`**：将 `sanitize.go` 移入，`middleware/` 和后续子模块共享
2. **Logger OTel 解耦**：引入 `SpanExtractor` 接口，默认 nil（不输出 trace ID），OTel 桥接移入 `observability/`
3. **RateLimit App 解耦**：引入 `RateLimiter` 接口，`App` 字段标记 Deprecated
4. **Metrics 接口化**：引入 `MeterProvider` 接口（已有 `metric.MeterProvider`），去掉直接 prometheus import

**验证**：全量测试通过，`middleware/` 包仍可整体 import。

### Phase 2 — 子模块拆分（Breaking Change）

**目标**：创建 `middleware/observability/` 和 `middleware/security/` 子模块。

1. **创建 `middleware/observability/go.mod`**：迁入 metrics.go、tracing.go、audit.go + OTel 桥接
2. **创建 `middleware/security/go.mod`**：迁入 jwt*.go、ratelimit*.go、apikey.go、signature.go、tenant.go、ipfilter.go、canary.go、longpoll.go、pprof.go
3. **核心 `middleware/` 保留 L0+L1**：recovery、requestid、secure、timeout、cors、csrf、csp、compress、logger
4. **更新 `go.work`**：添加两个新子模块
5. **核心 `go.mod` 瘦身**：移除 `golang-jwt`、`prometheus/client_golang`、`opentelemetry/*`

**Breaking Changes**：
- `middleware.JWT()` → `security.JWT()`
- `middleware.Metrics()` → `observability.Metrics()`
- `middleware.Tracing()` → `observability.Tracing()`
- 等等

**迁移成本估算**：用户平均改 2-5 行 import。

### Phase 3 — 向后兼容层（过渡期）

**目标**：提供 1-2 个大版本的过渡期，让用户渐进迁移。

1. **核心 `middleware/` 保留类型别名 + 包装函数**：

```go
// middleware/jwt.go (compat layer, deprecated)
// Deprecated: Use github.com/astra-go/astra/middleware/security.JWT instead.
func JWT(secret string) astra.HandlerFunc {
    return security.JWT(secret)
}
```

2. **`go vet` 检测**：配合 `// Deprecated` 注释，`go vet` 自动提示迁移
3. **2 个大版本后移除**：如 v2.x 保留兼容层，v3.x 完全移除

---

## 6. 依赖瘦身效果预估

### 当前 `go.mod` 直接依赖（核心模块）

```
golang-jwt/jwt/v5        → security 子模块
prometheus/client_golang  → observability 子模块
opentelemetry/*           → observability 子模块
quic-go/quic-go           → 保留（app_quic.go 在核心）
gorilla/websocket         → 保留（websocket/ 在核心）
go-playground/validator   → 保留（validate/ 在核心）
expr-lang/expr            → 保留（alert/rule 在核心）
robfig/cron               → 保留（cron/ 在核心）
```

### 拆分后核心 `go.mod`

```
quic-go/quic-go
gorilla/websocket
go-playground/validator
expr-lang/expr
robfig/cron
goccy/go-json
klauspost/compress
google/uuid
modernc.org/sqlite
```

**依赖数量**：从 ~15 个直接依赖降到 ~9 个，**移除 3 个最重的**（jwt + prometheus + otel）。

**二进制体积影响**（预估）：

| 场景 | 当前 | 拆分后 | 瘦身 |
|------|------|--------|------|
| 最小使用（Recovery+CORS） | ~18MB | ~8MB | -56% |
| + JWT 认证 | ~18MB | ~12MB | -33% |
| + 可观测性全套 | ~18MB | ~18MB | 0% |
| 全量使用 | ~18MB | ~18MB | 0% |

---

## 7. 风险与对冲

| 风险 | 影响 | 对冲 |
|------|------|------|
| Breaking Change 导致用户流失 | 高 | Phase 3 兼容层 + 2 版本过渡期 |
| 子模块版本同步复杂度 | 中 | go.work + replace 指令 + CI 单测 |
| Sanitize 共享循环依赖 | 低 | internal/ 包 + 单向依赖 |
| 社区中间件生态断裂 | 中 | 提供迁移脚本 + 文档 |

---

## 8. 与竞品的对比

| 框架 | 中间件组织方式 | 依赖隔离 |
|------|---------------|---------|
| **Gin** | 全部内置，无拆分 | 无（但零外部依赖） |
| **Echo** | 全部内置，无拆分 | 无（但零外部依赖） |
| **Fiber** | 全部内置，无拆分 | 无（依赖 fasthttp） |
| **Chi** | 独立包 `middleware/` | 无外部依赖，但也没有 JWT 等 |
| **GoKit** | 完全独立包 | 每个中间件独立 go.mod |
| **Astra (当前)** | 全部内置 | 无（依赖最重） |
| **Astra (优化后)** | 三层拆分 | L0+L1 零依赖，L2/L3 按需引入 |

**Astra 的独特优势**：Gin/Echo 零依赖是因为它们中间件少且简单。Astra 的 22 个中间件功能远超竞品，拆分后能在保持功能深度的同时实现依赖轻量化——这是竞品做不到的。

---

## 9. 总结

**核心论点**：22 个中间件不是问题，全部挤在一个 `go.mod` 才是问题。

**一句话方案**：L0+L1 留核心（零依赖），L2 观测性独立子模块，L3 安全/业务独立子模块，接口注入解耦共享逻辑。

**预期收益**：
- 最小使用场景依赖 **-56%**
- 核心模块编译时间 **-40%**（去掉 prometheus/otel 编译）
- 中间件可独立版本发布
- 框架"最小承诺"更清晰：Recovery + CORS + Secure = 零外部依赖

**实施周期**：
- Phase 1（内部重构）：1-2 周，零 Breaking Change
- Phase 2（子模块拆分）：2-3 周，Breaking Change + 兼容层
- Phase 3（过渡期）：2 个大版本周期（~6 个月）
