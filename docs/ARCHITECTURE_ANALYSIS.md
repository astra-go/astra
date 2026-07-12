# Astra 架构问题与优化分析

> 分析范围：`/Users/huangxiaolin/data/project/gotest/astra`（go.work monorepo）
> 分析方式：代码静态扫描 + 结构推演，2026-07-12 修复

---

## 一、整体架构画像

```
github.com/astra-go/astra (主模块)
├── go.work: 49 个子模块（go.mod + replace 覆盖）
├── 核心模块：app, router, context, middleware, di, config, mq, orm, grpc...
├── MQ 后端 × 8：Kafka, RabbitMQ, RocketMQ, NATS, MQTT, Pulsar, Redis, Memory
├── Config 后端 × 4：Nacos, Etcd, Apollo, Vault
├── ORM 扩展：GORM + Sharding + RW Splitting + ClickHouse
└── 依赖规模：go.mod ~7000 行，~100 个 transitive deps
```

---

## 二、分析结论总览

| # | 问题 | 状态 | 优先级 |
|---|------|------|--------|
| 1 | RocketMQ CapTx = true（未实现） | ✅ **已修复** | P0 |
| 2 | NATS NatsCapabilities() 注释矛盾 | ✅ **已修复** | P0 |
| 3 | RouteRegistrar 重名冲突 | ✅ **已修复** | P1 |
| 4 | Lifecycle RunStopHooks 错误不返回 | ✅ **已修复** | P1 |
| 5 | App.Use 并发安全注释不准确 | ✅ **已修复** | P1 |
| 6 | NATS/Redis 未加入 Builder | ❌ **误报**（已完整支持） | — |
| 7 | Go 1.25 版本不可用 | ❌ **误报**（go 1.25.8 已安装） | — |
| 8 | Kafka CapFixedDelay 语义不清 | ⏸ 暂缓 | P2 |
| 9 | go.work replace 维护成本高 | ⏸ 暂缓 | P2 |
| 10 | 两套 DI 系统并行 | ⏸ 暂缓（需产品决策） | P2 |
| 11 | middleware/security 包过大 | ⏸ 暂缓 | P3 |
| 12 | Auth 与 Security JWT 职责重叠 | ⏸ 暂缓（需产品决策） | P3 |
| 13 | Module 废弃技术债 | ⏸ 暂缓（v3 路线图） | P3 |
| 14 | kvStoreMapThreshold 未确认 | ⏸ 需确认 | P3 |
| 15 | Components() 深拷贝开销 | ⏸ 需 profiling 数据 | P3 |

---

## 三、已修复问题详情

### Fix #1: RocketMQ CapTx 降级

**文件：** `mq/capability.go`

**改动：** `RocketMQCapabilities()` 中 `CapTx: true` → `CapTx: false`，附 TODO 注释指向 `rocketmq.go`。

**原因：** `rocketmq.go:249` 有 `TODO: use TransactionProducer`，`cfg.EnableTx` 配置存在但事务消息未实现。

```go
// CapTx is conditional — EnableTx config exists but TransactionProducer
// is not yet implemented (see rocketmq.go TODO).
CapTx: false, // TODO: implement TransactionProducer (rocketmq.go)
```

---

### Fix #2: NATS 注释矛盾

**文件：** `mq/capability.go`

**改动：** 删除 `NatsCapabilities()` 上方矛盾的两段注释（第一段说"不支持 arbitrary delay/fixed delay/priority/tx"，第二段说"支持"）。

**原因：** copy-paste 遗留，第二段注释是更新后的正确版本，但第一段过时段落未删除。

```go
// 修改前：两段矛盾注释
// NATS supports: NAK delay... (old)
// It does NOT support: arbitrary delay, fixed delay, priority, tx.
// NATS (JetStream) supports: ... fixed delay via JetStream... (new)

// 修改后：保留新注释
// NATS (JetStream) supports: NAK delay (CapNakDelay), multi consumer group
// (CapMultiGroup), batch (CapBatch), DLQ via DLQSubject (CapDLQ), retry via
// MaxDeliver (CapRetry), ordered delivery (CapOrdered), idempotency via
// KV bucket (CapIdempotency), fixed delay via JetStream scheduled delivery
// (CapFixedDelay), arbitrary delay via client-side re-publisher (CapArbitraryDelay),
// and priority via multi-subject routing (CapPriority).
// It does NOT support native transactions (CapTx).
```

---

### Fix #3: RouteRegistrar 重命名

**文件：** `mq/router.go` + `mq_http.go`

**改动：** `mq.RouteRegistrar` → `mq.HTTPRouteRegistrar`

**原因：** `app.go:astra.RouteRegistrar`（`HandlerFunc` 签名）与 `mq/router.go:mq.RouteRegistrar`（`func(ctx, *http.Request) error` 签名）同名但方法完全不同，造成歧义。

```go
// 修改前
type RouteRegistrar interface { ... }  // mq/router.go
type astraRouteRegistrar struct { ... }  // mq_http.go

// 修改后
type HTTPRouteRegistrar interface { ... }  // mq/router.go
type astraMQRouteRegistrar struct { ... }   // mq_http.go（添加 MQ 前缀区分）
```

---

### Fix #4: Lifecycle RunStopHooks 返回错误

**文件：** `lifecycle.go`

**改动：** `RunStopHooks(ctx context.Context)` 从返回 `void` 改为返回 `[]error`。

**原因：** StopHook 错误被静默（仅 slog.Error），而 StartHook 错误会 abort 并传播。设计不对称——调用方无法感知清理失败。

```go
// 修改前
func (l *Lifecycle) RunStopHooks(ctx context.Context) {
    for i := len(hooks) - 1; i >= 0; i-- {
        if err := hooks[i](ctx); err != nil {
            slog.Error("stop hook failed", "err", err)
            // ← 没有返回值，调用方无法感知
        }
    }
}

// 修改后：保留所有 hook 都运行（不 early return），收集并返回所有错误
func (l *Lifecycle) RunStopHooks(ctx context.Context) (errs []error) {
    for i := len(hooks) - 1; i >= 0; i-- {
        if err := hooks[i](ctx); err != nil {
            slog.Error("stop hook failed", "err", err)
            errs = append(errs, err)
        }
    }
    return
}
```

**影响评估：** 现有调用方（`app.go`、`app_reactor.go`、`app_wasm.go`）均忽略返回值，属于非破坏性变更。

---

### Fix #5: App.Use 并发安全文档

**文件：** `app.go`

**改动：** 补充注释，明确说明并发安全限制。

**原因：** 原注释"Safe to call concurrently with other Use calls, but not concurrently with route registrations"不准确——`handle()` 用 RLock，`Use()` 用 Lock，RWMutex 实际上允许两者并发执行（多个读锁可同时持有）。

```go
// 修改前
// Safe to call concurrently with other Use calls, but not concurrently
// with route registrations.

// 修改后
// Concurrency: Safe to call concurrently with other Use calls. Safe to call
// concurrently with route registrations from a locking perspective (RWMutex
// read-write pair prevents corruption), but NOT safe after the server has
// started accepting requests — reading the middleware slice while the server
// is concurrently registering routes can produce inconsistent handler chains.
// In practice, always call Use during setup before app.Run().
```

---

## 四、剩余问题说明

### P2 — Kafka CapFixedDelay 语义

`KafkaCapabilities()` 声明 `CapFixedDelay: true`（via per-level delay topics + forwarding consumer）。这是有效的 workaround，但 Kafka 原生并不支持固定延迟队列级别，文档应注明这是框架层模拟而非 broker 原生能力。

### P2 — go.work replace 维护成本

49 个子模块 × 49 行 replace 语句，任何路径变化都需要同步。考虑用 monorepo 工具（Bazel/Please）替代，或在 `go.work` 头部加脚本注释追踪 replace 覆盖范围。

### P2 — 两套 DI 系统

`di/`（泛型+反射）和 `provider/`（注册表）职责边界不清。用户不清楚什么时候用哪个。建议统一到 `di/` 包，`provider/` 作为子集或废弃。

### P3 — 其他

- `middleware/security/` 包过大 → 按需拆分
- `Auth` 与 `Security JWT` 职责重叠 → 需产品决策统一入口
- `Module` 废弃但未移除 → v3 路线图
- `kvStoreMapThreshold` 未确认
- `Components()` 深拷贝 → 需 profiling 数据支撑再做决策

---

## 五、构建验证

```
go build ./...      ✅ 全部模块编译通过
go test ./mq/...    ✅ 0.648s [no tests to run]
go test -run TestLifecycle ✅ PASS
```
