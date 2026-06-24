# Astra MQ 模块分析与优化方案

> 基于 github.com/astra-go/astra v1.0.5 源码分析
>
> 日期: 2026-06-23

---

## 目录

1. [背景与现状](#1-背景与现状)
2. [Astra MQ 替换 asynq 可行性分析](#2-astra-mq-替换-asynq-可行性分析)
3. [Astra MQ 统一多后端架构设计](#3-astra-mq-统一多后端架构设计)
4. [MQ 模块 7 项优化方案](#4-mq-模块-7-项优化方案)
5. [与 asynq 能力对比矩阵](#5-与-asynq-能力对比矩阵)
6. [附录](#6-附录)

---

## 1. 背景与现状

### 1.1 核心发现

**Astra MQ 模块在 v1.0.5 中尚未发布。**

| 检查项 | 结果 |
|--------|------|
| `github.com/astra-go/astra/mq/` 源码目录 | **不存在** |
| go.mod 中的 mq 依赖 | **不存在**（仅 cache/orc/orm/middleware/security） |
| mq 示例代码 | 仅 `examples/mq/main.go`（**InMemoryBroker**，无持久化、无延迟、无重试） |
| CI workflow (`mq-test-matrix.yml`) | 有定义（Kafka/RabbitMQ/NATS/MQTT/Pulsar/RocketMQ 测试矩阵） |
| v2 迁移指南 (`docs/migration-guide-mq-v2.md`) | 描述了 v2.0.0-beta.1 的模块合并方案（仅 import 路径变更，能力未增强） |
| v2.0.0-beta.1 发布状态 | **未发布** |

### 1.2 当前 MQ 能力（InMemoryBroker）

```go
// examples/mq/main.go — 当前唯一的 MQ 抽象
type Message struct {
    Topic   string
    Key     string
    Payload []byte
    Headers map[string]string
    Meta    map[string]any
}

type Handler func(ctx context.Context, msg *Message) error

type Producer interface {
    Publish(ctx context.Context, msg *Message) error
    Close() error
}

type Consumer interface {
    Subscribe(ctx context.Context, topics []string, group string, h Handler) error
    Close() error
}
```

**能力缺陷**：
- ❌ 无延迟投递（`Delay` 字段）
- ❌ 无幂等性支持（`IdempKey` 字段）
- ❌ 无消息 TTL
- ❌ 无重试次数控制（`RetryMax` 字段）
- ❌ 无链路追踪（`TraceID` 字段）
- ❌ 无语义化错误处理（只返回 `error`，无法区分永久失败/可重试失败）
- ❌ 无死信队列语义
- ❌ Handler 失败=永久丢失（InMemoryBroker 中 errors 向上冒泡停止消费）

### 1.3 ADR 约束

| ADR | 内容 | 对 MQ 的影响 |
|-----|------|-------------|
| ADR-001 | Core 模块不依赖重型子模块 | MQ 必须是独立子模块，用户显式 go get |
| ADR-005 | 子模块数量上限 40；同类功能模块合并 | MQ 各后端合并为单一 `mq/` 模块 |

---

## 2. Astra MQ 替换 asynq 可行性分析

### 2.1 当前 asynq 用量

async-svc 项目（基于 hibitokof/ginx → asynq）:

| 维度 | 数值 |
|------|------|
| 任务类型 | 21 类 |
| 延迟投递 | 阶梯重试（15s/30s/45s/60s/75s）、用户指定延迟 |
| 幂等性 | TaskID + UniqueKey（1 小时自动去重） |
| 消息 TTL | 默认 24h |
| 优先级 | 13 档（asynq.DefaultQueueName ~ asynq.QueueCrit） |
| 重试策略 | 指数退避（DefaultRetryDelayFunc）、可自定义 |
| 死信队列 | asynq 内置 DeadLetterServer，可配置处理逻辑 |
| 监控 | asynq Inspector 内置 Web UI 和 CLI |

### 2.2 全面替换的 6 个必要条件

#### 条件一：Astra MQ v2 模块正式发布

MQ 模块必须作为独立子模块发布到 `github.com/astra-go/astra/mq`。当前不存在。

#### 条件二：RocketMQ 后端（延迟消息）

Kafka **完全不支持**延迟消息。RocketMQ 作为延迟后端，需满足：

- 部署 RocketMQ 5.x 服务端（与 Kafka 完全不同的运维体系）
- RocketMQ 的延迟消息是**固定 18 个级别**：

```
Level  1: 1s     Level  7: 3m     Level 13: 9m
Level  2: 5s     Level  8: 4m     Level 14: 10m
Level  3: 10s    Level  9: 5m     Level 15: 20m
Level  4: 30s    Level 10: 6m     Level 16: 30m
Level  5: 1m     Level 11: 7m     Level 17: 1h
Level  6: 2m     Level 12: 8m     Level 18: 2h
```

自动发货的阶梯重试（15s→30s→45s→60s→75s）中 **45s 和 75s 无法精确匹配**。

#### 条件三：幂等能力自建

asynq 原生提供：

```go
asynq.NewTask(taskType, payload,
    asynq.TaskID("purchase:order_123"),
    asynq.Unique(time.Hour),
)
```

Astra MQ 的 `Message` 结构只有 Topic/Key/Payload/Headers/Meta，**无 TaskID 和 Unique 字段**。幂等需要：

- Redis SET NX 实现（多一个外部依赖）
- 或 DB 重复查询（性能损失）

#### 条件四：阶梯重试自建

asynq 内置 `MaxRetry(n)` + `DefaultRetryDelayFunc`（指数退避）。Astra MQ Consumer 消费失败后，需要业务侧自行实现：

```go
if err != nil {
    retryCount := getRetryCount(msg.Key)   // Redis or DB?
    delay := []time.Duration{15,30,45,60,75}[retryCount] * time.Second
    publishToDelayQueue(topic, payload, delay)
    incrRetryCount(msg.Key)
}
```

#### 条件五：消费者进程独立部署

| 维度 | asynq | Astra MQ Consumer |
|------|-------|-------------------|
| 部署模式 | 同一进程（Worker 与 HTTP Server 共存） | 独立消费进程 |
| 生命周期 | 与 App context 一致 | 需单独管理 |
| 状态协调 | 自动 | HTTP 还在跑但 Consumer 挂了？ |

#### 条件六：运维体系差异

| 维度 | asynq | RocketMQ |
|------|-------|---------|
| 部署 | 仅 Redis（已有） | RocketMQ 服务端（新增） |
| 监控 | Redis info 命令 / asynq dashboard | RocketMQ console / Prometheus exporter |
| 运维经验 | team 已有 | 需新学 |
| 故障恢复 | Redis 持久化 | 消息积压处理更复杂 |
| 成本 | Redis 内存（任务 < 10KB） | RocketMQ 磁盘存储 |

### 2.3 最终结论

| 方案 | 可行？ | 理由 |
|------|--------|------|
| Kafka 全面替代 asynq | ❌ | 无延迟能力，无幂等，无优先级，无阶梯重试 |
| RocketMQ 全面替代 asynq | ⚠️ | 固定 18 级无法匹配 45s/75s；模块未发布；运维成本高 |
| Astra MQ + Redis 补偿层 | ⚠️ | 依赖 v2 发布，Redis 补偿层增加复杂度 |
| **保持当前三层架构**（asynq + Astra cron + HTTP API） | ✅ | 零额外运维成本，asynq 满足全部需求 |

---

## 3. Astra MQ 统一多后端架构设计

### 3.1 各后端原生能力矩阵

| 能力 | RocketMQ | NATS JetStream | Kafka | RabbitMQ | MQTT | Pulsar | Redis |
|------|----------|---------------|-------|----------|------|--------|-------|
| **任意时间延迟** | ✅ `SetDelayTimestamp` | ✅ `NakWithDelay` | ❌ | ❌（仅 TTL 死信） | ❌ | ❌ | ⚠️ 需 Redis |
| **消息持久化** | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ |
| **多消费者组** | ✅ | ✅ | ✅ | ✅（vhost） | ❌ | ✅ | ❌ |
| **优先级队列** | ✅ 3 档 | ❌ | ❌ | ✅ 队列级 | ❌ | ✅ | ❌ |
| **消息顺序** | ✅ | ❌ | ⚠️ partition | ⚠️ 单队列 | ❌ | ⚠️ 单分区 | ❌ |
| **多语言 SDK** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### 3.2 三层架构

```
应用层（Handler / Worker）
         │
         ▼
┌─────────────────────────────────────────┐
│         Capability Matrix               │  ← 能力探测
│  检查各后端是否支持：                    │
│  - 任意时间延迟（RocketMQ v5/Pulsar）    │
│  - 固定级别延迟（RocketMQ v4/RabbitMQ）  │
│  - NAK + delay（NATS JetStream）         │
│  - 死信队列 + TTL（RabbitMQ）            │
└────────────────┬────────────────────────┘
                 │ 返回 CapabilitySet
                 ▼
┌─────────────────────────────────────────┐
│         Adapter（适配层）               │
│  RocketMQ → mq.RocketMQAdapter          │
│  NATS    → mq.NATSAdapter               │
│  Kafka   → mq.KafkaAdapter              │
│  RabbitMQ→ mq.RabbitMQAdapter           │
│  MQTT    → mq.MQTTAdapter               │
│  Pulsar  → mq.PulsarAdapter             │
└────────────────┬────────────────────────┘
                 │
       ┌─────────┴──────────┐
       ▼                   ▼
┌─────────────┐   ┌─────────────────┐
│   Redis     │   │  后端原生实现    │
│ 补偿层      │   │ (RocketMQ/NATS  │
│ (Sorted Set │   │  Kafka/RabbitMQ)│
│  延迟+幂等) │   └─────────────────┘
└─────────────┘
```

### 3.3 接口定义

```go
package mq

// ─── Message ───
type Message struct {
    Topic     string
    Key       string
    Payload   []byte
    Headers   map[string]string
    Meta      map[string]any
    Delay     time.Duration    // 延迟投递时长（0=立即投递）
    IdempKey  string           // 幂等键，框架自动去重
    TTL       time.Duration    // 消息存活时间
    RetryMax  int              // 最大重试次数（0=使用默认值）
    TraceID   string           // 链路追踪 ID（自动注入）
}

// ─── Handler ───
type Handler func(ctx context.Context, msg *Message) error

// ─── Producer ───
type Producer interface {
    Publish(ctx context.Context, msg *Message, opts ...Option) error
    Close() error
}

// ─── Consumer ───
type Consumer interface {
    Subscribe(ctx context.Context, topics []string, group string, h Handler) error
    Close() error
}

// ─── Capability ───
type Capability int

const (
    CapArbitraryDelay Capability = 1 << iota  // 任意时间延迟
    CapFixedDelay                                // 固定级别延迟
    CapNakDelay                                  // NAK + 延迟
    CapIdempotency                               // 原生幂等
    CapPriority                                  // 优先级队列
    CapOrdered                                   // 分区内有序
    CapDLQ                                       // 死信队列
    CapMultiGroup                                // 多消费者组
)

// ─── Adapter ───
type Adapter interface {
    Capabilities() map[Capability]bool
    Publish(ctx context.Context, msg *Message, opts ...Option) error
    Subscribe(ctx context.Context, topics []string, group string, h Handler) error
    Close() error
}
```

### 3.4 延迟投递适配策略

```go
// 延迟分发器
func (a *RocketMQAdapter) Publish(ctx context.Context, msg *Message, opts ...Option) error {
    if msg.Delay > 0 {
        // RocketMQ v5 原生：任意时间戳延迟
        msg.Raw().SetDelayTimestamp(time.Now().Add(msg.Delay))
    }
    return a.rocket.Publish(ctx, msg)
}

func (a *KafkaAdapter) Publish(ctx context.Context, msg *Message, opts ...Option) error {
    if msg.Delay > 0 {
        // Kafka 无原生延迟 → Redis Sorted Set 补偿
        return a.compensator.PublishDelay(ctx, msg.Topic, msg.Key, msg.Payload, msg.Delay)
    }
    return a.kafka.Publish(ctx, msg.Key, msg.Payload)
}

// Redis 延迟扫描器（内嵌到 MQ 模块生命周期）
func (c *RedisCompensator) ScanAndRequeue(ctx context.Context, producer Adapter) {
    ticker := time.NewTicker(time.Second)
    for {
        select {
        case <-ticker.C:
            now := time.Now().UnixMilli()
            items, err := c.client.ZRangeByScore(ctx, "mq:delay:*",
                &redis.ZRangeBy{Min: "0", Max: fmt.Sprintf("%d", now), Count: 100})
            if err != nil { continue }
            for _, item := range items {
                var msg Message
                json.Unmarshal([]byte(item), &msg)
                msg.Delay = 0                     // 清除延迟标记
                producer.Publish(ctx, &msg)       // 重新投递
                c.client.ZRem(ctx, "mq:delay:"+msg.Topic, item)
            }
        case <-ctx.Done():
            return
        }
    }
}
```

---

## 4. MQ 模块 7 项优化方案

### 4.1 优化一览

| # | 优化项 | 解决的问题 | 优先级 | 改动范围 |
|---|--------|-----------|--------|---------|
| 1 | 扩展 Message 结构 | 缺少 Delay/IdempKey/TTL 字段 | **P0** | `mq/mq.go` |
| 2 | 语义化错误处理 | Handler 无法区分永久/可重试失败 | **P0** | `mq/mq.go` |
| 3 | 能力探测接口 | 后端差异无法感知 | **P1** | `mq/capability.go` |
| 4 | Redis 补偿层 | Kafka/RabbitMQ 无延迟能力 | **P1** | `mq/redis/` |
| 5 | Astra 生命周期集成 | Consumer 独立进程运维复杂 | **P1** | `mq/astra_middleware.go` |
| 6 | 路由层集成 | 减少样板代码 | **P2** | `mq/astra_middleware.go` |
| 7 | 可观测性 | 生产环境缺监控 | **P2** | `mq/observability.go` |

### 4.2 优化一：扩展 Message 结构（P0）

```go
// mq/mq.go — Message 扩展
type Message struct {
    Topic     string
    Key       string
    Payload   []byte
    Headers   map[string]string
    Meta      map[string]any

    // ── v2 新增字段 ──
    Delay     time.Duration    // 延迟投递时长（0=立即）
    IdempKey  string            // 幂等键，框架自动去重
    TTL       time.Duration     // 消息存活时间（超时未消费则丢弃）
    RetryMax  int               // 最大重试次数（0=使用默认值）
    TraceID   string            // 链路追踪 ID（自动注入）
}

// ── 工厂方法 ──
func NewMessage(topic string, payload []byte) *Message {
    return &Message{
        Topic:   topic,
        Payload: payload,
    }
}

func (m *Message) WithDelay(d time.Duration) *Message     { m.Delay = d; return m }
func (m *Message) WithIdempKey(key string) *Message        { m.IdempKey = key; return m }
func (m *Message) WithTTL(ttl time.Duration) *Message      { m.TTL = ttl; return m }
func (m *Message) WithRetryMax(n int) *Message             { m.RetryMax = n; return m }
func (m *Message) WithTraceID(traceID string) *Message     { m.TraceID = traceID; return m }
```

✅ **已完成** - 2026-06-23 17:15 - commit: `0721e2e`
- 在 `mq/mq.go` 的 `Message` struct 中添加了 `Delay`/`IdempKey`/`TTL`/`RetryMax`/`TraceID` 字段
- 添加了 `NewMessage()` 构造函数及 5 个 `With*` 便捷方法
- 编译验证通过（`cd mq && go build ./...`）

### 4.3 优化二：语义化错误处理（P0）

```go
// mq/errors.go — 语义化错误类型
type ErrorKind int

const (
    ErrPermanent  ErrorKind = iota  // 永久失败：不重试，进死信队列
    ErrRetry                        // 可重试：按策略延迟后重新投递
    ErrSkip                         // 跳过：不重试，不进死信（配置缺失等）
    ErrPanic                        // panic：框架兜底，进死信
)

type MQError struct {
    Kind       ErrorKind
    Cause      error
    RetryAfter time.Duration  // ErrRetry 时指定下次重试延迟
}

func (e *MQError) Error() string   { return e.Cause.Error() }
func (e *MQError) Unwrap() error   { return e.Cause }

// 便捷构造
func Permanent(err error) error          { return &MQError{Kind: ErrPermanent, Cause: err} }
func Retry(err error, after time.Duration) error {
    return &MQError{Kind: ErrRetry, Cause: err, RetryAfter: after}
}
func Skip(err error) error                { return &MQError{Kind: ErrSkip, Cause: err} }

// 框架内部识别
func IsPermanent(err error) bool { ... }
func IsRetry(err error) bool     { ... }
func IsSkip(err error) bool      { ... }

// 重试间隔计算（内置指数退避 + 阶梯重试支持）
type RetryPolicy struct {
    MaxRetries int              // 最大重试次数
    Levels     []time.Duration  // 阶梯重试（如 [15s, 30s, 45s, 60s, 75s]）
    Base       time.Duration   // 指数退避基数（Levels 为空时使用）
}

func (p *RetryPolicy) NextDelay(retryCount int) (time.Duration, bool) {
    if retryCount >= p.MaxRetries {
        return 0, false // 超出最大重试次数 → 进死信
    }
    if len(p.Levels) > 0 && retryCount < len(p.Levels) {
        return p.Levels[retryCount], true
    }
    // 指数退避 fallback: Base * 2^retryCount
    return p.Base * (1 << uint(retryCount)), true
}
```

**应用层使用示例**：

```go
consumer.Subscribe(ctx, []string{"purchase.order"}, "group1", func(ctx context.Context, msg *Message) error {
    var order Order
    if err := json.Unmarshal(msg.Payload, &order); err != nil {
        return Permanent(err) // 格式错误，永久失败
    }
    if order.Amount <= 0 {
        return Skip(errors.New("invalid amount")) // 跳过
    }
    if err := paymentService.Charge(ctx, order); err != nil {
        if isNetworkError(err) {
            return Retry(err, 15*time.Second) // 网络错误，15s 后重试
        }
        return Permanent(err) // 非网络错误，永久失败
    }
    return nil
})
```

✅ **已完成** - 2026-06-23 17:20 - commit: `49d5f63`
- 创建 `mq/errors.go`，实现 `MQError` 类型（ErrPermanent/ErrRetry/ErrSkip/ErrPanic）
- 添加 `Permanent()`/`Retry()`/`Skip()` 便捷构造函数
- 添加 `IsPermanent()`/`IsRetry()`/`IsSkip()` 判断函数（使用 `errors.As` 正确支持错误链）
- 添加 `RetryPolicy` 结构体及 `NextDelay()` 方法（支持阶梯重试和指数退避）
- 编译验证通过（`cd mq && go build ./...`）

### 4.4 优化三：能力探测接口（P1）

```go
// mq/capability.go
type Capability int

const (
    CapArbitraryDelay  Capability = 1 << iota  // 任意时间延迟
    CapFixedDelay                                 // 固定级别延迟（18 个 level）
    CapNakDelay                                   // NAK + 可配置延迟
    CapIdempotency                                // 原生幂等（Pulsar seq ID）
    CapPriority                                   // 优先级队列
    CapOrdered                                    // 分区内有序
    CapDLQ                                        // 原生死信队列
    CapRetry                                       // 原生重试
    CapMultiGroup                                 // 多消费者组
    CapTx                                         // 事务消息
    CapBatch                                      // 批量发送
)

type Capabilities map[Capability]bool

type Adapter interface {
    Capabilities() Capabilities
    Publish(ctx context.Context, msg *Message, opts ...Option) error
    Subscribe(ctx context.Context, topics []string, group string, h Handler) error
    Close() error
}

// 各后端能力声明
func (a *RocketMQAdapter) Capabilities() Capabilities {
    return Capabilities{
        CapArbitraryDelay: true,  // v5 支持 SetDelayTimestamp
        CapFixedDelay:     true,  // v4 支持 18 个固定 level
        CapDLQ:            true,
        CapRetry:          true,
        CapMultiGroup:     true,
        CapOrdered:        true,
        CapTx:             true,
        CapBatch:          true,
    }
}

func (a *KafkaAdapter) Capabilities() Capabilities {
    return Capabilities{
        CapOrdered:    true,  // partition 内有序
        CapMultiGroup: true,
        CapBatch:      true,
        // 无延迟、无重试、无死信、无幂等
    }
}

// 框架自动选择补偿策略
func BuildAdapter(backend string, opts Options) (Adapter, error) {
    backend = strings.ToLower(backend)
    var adapter Adapter
    var err error
    switch backend {
    case "rocketmq":
        adapter, err = NewRocketMQAdapter(opts.RocketMQ)
    case "kafka":
        adapter, err = NewKafkaAdapter(opts.Kafka)
    // ... 其他后端
    
    default:
        return nil, fmt.Errorf("unknown mq backend: %s", backend)
    }
    
    // 检查能力缺口，自动包装补偿层
    caps := adapter.Capabilities()
    if !caps[CapArbitraryDelay] && !caps[CapFixedDelay] && opts.EnableRedisCompensator {
        adapter = WrapWithRedisCompensator(adapter, opts.Redis)
    }
    if !caps[CapIdempotency] && opts.EnableRedisCompensator {
        adapter = WrapWithIdempotencyMiddleware(adapter, opts.Redis, opts.IdempotentTTL)
    }
    
    return adapter, nil
}
```

✅ **已完成** - 2026-06-23 17:30 - commit: `b61446a`
- 创建 `mq/capability.go`，实现 `Capability` 类型（12 种能力位掩码）
- 实现 `KafkaCapabilities()`/`RabbitMQCapabilities()`/`RocketMQCapabilities()`/`NatsCapabilities()`/`PulsarCapabilities()`/`MqttCapabilities()` 6 个后端能力集
- 在 `mq/mq.go` 的 `Producer` 和 `Consumer` 接口中添加 `Capabilities() Capabilities` 方法
- 给 6 个后端适配器的 Producer 和 Consumer 添加 `Capabilities()` 方法实现
- 编译验证通过（`cd mq && go build ./...`）

### 4.5 优化四：Redis 补偿层（P1）

```go
// mq/redis/compensator.go
package mqredis

type Compensator struct {
    client *redis.Client
}

// 延迟投递补偿（Sorted Set）
func (c *Compensator) EnqueueDelay(ctx context.Context, msg *Message) error {
    score := float64(time.Now().Add(msg.Delay).UnixMilli())
    data, _ := json.Marshal(msg)
    return c.client.ZAdd(ctx, "mq:delay:"+msg.Topic, redis.Z{
        Score:  score,
        Member: msg.IdempKey,
    }).Err()
}

// 幂等性拦截（SET NX + TTL）
func (c *Compensator) CheckIdempotency(ctx context.Context, idempKey string, ttl time.Duration) (bool, error) {
    if idempKey == "" {
        return true, nil // 不检查
    }
    return c.client.SetNX(ctx, "mq:idemp:"+idempKey, "1", ttl).Result()
}

func (c *Compensator) ReleaseIdempotency(ctx context.Context, idempKey string) {
    if idempKey != "" {
        c.client.Del(ctx, "mq:idemp:"+idempKey)
    }
}

// 延迟扫描器
type DelayScanner struct {
    compensator *Compensator
    producer    Adapter
}

func (s *DelayScanner) Run(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            s.scanAndRequeue(ctx)
        case <-ctx.Done():
            return
        }
    }
}

func (s *DelayScanner) scanAndRequeue(ctx context.Context) {
    now := float64(time.Now().UnixMilli())
    // 扫描所有到期消息
    keys, _, err := s.compensator.client.Scan(ctx, 0, "mq:delay:*", 100).Result()
    if err != nil { return }
    for _, key := range keys {
        items, err := s.compensator.client.ZRangeByScore(ctx, key,
            &redis.ZRangeBy{Min: "0", Max: fmt.Sprintf("%.0f", now), Count: 10}).Result()
        if err != nil { continue }
        for _, item := range items {
            var msg Message
            if err := json.Unmarshal([]byte(item), &msg); err != nil { continue }
            msg.Delay = 0 // 清除延迟
            if cErr := s.producer.Publish(ctx, &msg); cErr == nil {
                s.compensator.client.ZRem(ctx, key, item)
            }
        }
    }
}
```

✅ **已完成** - 2026-06-23 17:40 - commit: `9bd6b9e`
- 创建 `mq/redis/compensator.go`，实现 `Compensator` 结构体（`EnqueueDelay()`/`CheckIdempotency()`/`ReleaseIdempotency()`）
- 实现 `DelayScanner` 结构体（`Run()`/`Stop()`/`scanAndRequeue()`），定时扫描 Redis ZSET 并重新投递过期延迟消息
- 在 `mq/go.mod` 中添加 `github.com/redis/go-redis/v9` 依赖
- 编译验证通过（`cd mq && go build ./...`）

### 4.6 优化五：Astra 生命周期集成（P1）

```go
// mq/astra_integration.go
package mq

import "github.com/astra-go/astra"

type AppHandler func(app *astra.App) Adapter

// 注册 MQ 到 Astra App 生命周期
func Register(app *astra.App, opts Options) (Adapter, error) {
    adapter, err := BuildAdapter(opts.Backend, opts)
    if err != nil {
        return nil, err
    }

    // Consumer 随 App 启动
    app.OnStart(func(ctx context.Context) error {
        for _, topic := range opts.Topics {
            if err := adapter.Subscribe(ctx, []string{topic}, opts.Group, opts.Handler); err != nil {
                return fmt.Errorf("subscribe %s: %w", topic, err)
            }
        }
        return nil
    })

    // Redis 延迟扫描器（可选）
    if opts.EnableRedisCompensator && opts.Backend != "rocketmq" {
        scanner := redis.NewDelayScanner(adapter, opts.Redis)
        app.OnStart(func(ctx context.Context) error {
            go scanner.Run(ctx)
            return nil
        })
    }

    // 优雅关闭
    app.OnStop(func(ctx context.Context) error {
        return adapter.Close()
    })

    return adapter, nil
}
```

✅ **已完成** - 2026-06-23 18:05
- 创建 `mq/astra_integration.go`：`AppRegistrar` 接口 + `Register()` 函数
  - 接口注入方式避免 `mq/` 导入 `astra/` 的循环依赖
  - 主模块 `astra.App` 天然实现 `AppRegistrar`（OnStart/OnStop）
- 创建 `mq/compensator.go`：`Compensator`（延迟入队/幂等检查）+ `DelayScanner`
  - 为避免 `mq/redis/` 子包反向引用 `mq/` 的循环依赖，将补偿层合并到 `mq/` 主包
  - 不创建 `mq/redis/go.mod`（子包共享 `mq/go.mod`）
- 删除废弃的 `mq/redis/` 目录
- `mq/` 和主模块双向编译验证通过（`go build ./...`）

### 4.7 优化六：路由层集成（P2）

```go
// mq/astra_router.go
package mq

type MQRouteConfig struct {
    Topic     string
    Group     string
    Handler   Handler
    Middleware []astra.HandlerFunc
}

// 注册 HTTP → MQ 投递路由（POST /_mq/:topic）
func RegisterMQRoutes(app *astra.App, producer Adapter) {
    app.POST("/_mq/:topic", func(c *astra.Ctx) error {
        topic := c.Param("topic")
        if topic == "" {
            return astra.NewHTTPError(400, "topic required")
        }
        
        msg := &Message{
            Topic:     topic,
            Payload:   c.Body(),
            Key:       c.GetHeader("X-MQ-Key"),
            IdempKey:  c.GetHeader("X-MQ-Idemp-Key"),
            TraceID:   c.GetHeader("X-Trace-ID"),
        }
        
        if delayStr := c.Query("delay"); delayStr != "" {
            if d, err := time.ParseDuration(delayStr); err == nil {
                msg.Delay = d
            }
        }
        
        if err := producer.Publish(c.Request().Context(), msg); err != nil {
            return astra.NewHTTPError(503, "mq publish failed: "+err.Error())
        }
        
        // 同步模式：202 Accepted + 消息 ID
        return c.Status(202).JSON(astra.Map{
            "status": "published",
            "topic":  topic,
            "key":    msg.Key,
        })
    })
}
```

✅ **已完成** - 2026-06-23 18:20
- 创建 `mq/router.go`：`RouteRegistrar` 接口（`GET/POST/PUT/DELETE/Any`）避免导入 `astra/`
  - `RegisterHTTPRoutes()`：注册 `POST /<_prefix>/<topic>` 端点
  - 支持 `?delay=` 查询参数（DelayScanner 入队）
  - 支持 `X-MQ-Idemp-Key` 幂等头
- 创建 `mq_http.go`（主模块）：实现 `RouteRegistrar` + `App.RegisterMQHTTPRoutes()` 扩展方法
  - `*App.RegisterMQHTTPRoutes(opts)` 直接调用，无需单独创建 registrar
- `mq/` 和主模块双向编译通过，测试全部通过（0 errors）

### 4.8 优化七：可观测性（P2）

```go
// mq/observability.go
package mq

import (
    "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
    Published    *prometheus.CounterVec
    PublishedErr *prometheus.CounterVec
    Consumed     *prometheus.CounterVec
    ConsumedErr  *prometheus.CounterVec
    Latency      *prometheus.HistogramVec
    Retried      *prometheus.CounterVec
    Dlq          *prometheus.CounterVec
    QueueDepth   *prometheus.GaugeVec
}

func NewMetrics(namespace string) *Metrics {
    return &Metrics{
        Published: prometheus.NewCounterVec(prometheus.CounterOpts{
            Namespace: namespace,
            Name:      "mq_published_total",
            Help:      "Total number of published messages.",
        }, []string{"topic"}),
        
        Consumed: prometheus.NewCounterVec(prometheus.CounterOpts{
            Namespace: namespace,
            Name:      "mq_consumed_total",
            Help:      "Total number of consumed messages.",
        }, []string{"topic", "status"}),
        
        Latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
            Namespace: namespace,
            Name:      "mq_consume_duration_seconds",
            Help:      "Message consume duration in seconds.",
            Buckets:   prometheus.DefBuckets,
        }, []string{"topic"}),
        
        Dlq: prometheus.NewCounterVec(prometheus.CounterOpts{
            Namespace: namespace,
            Name:      "mq_dlq_total",
            Help:      "Total number of messages sent to DLQ.",
        }, []string{"topic"}),
    }
}

// 包装 Handler 自动注入可观测性
func WrapWithMetrics(h Handler, metrics *Metrics) Handler {
    return func(ctx context.Context, msg *Message) error {
        start := time.Now()
        err := h(ctx, msg)
        dur := time.Since(start)
        
        status := "success"
        if err != nil {
            switch {
            case IsPermanent(err): status = "permanent"
            case IsRetry(err):     status = "retry"
            case IsSkip(err):      status = "skip"
            default:               status = "error"
            }
        }
        
        metrics.Consumed.WithLabelValues(msg.Topic, status).Inc()
        metrics.Latency.WithLabelValues(msg.Topic).Observe(dur.Seconds())
        
        return err
    }
}
```

✅ **已完成** - 2026-06-23 18:35
- 创建 `mq/metrics.go`：基于 Prometheus client_golang（已有间接依赖 v1.23.2）
- **CounterVec**：`mq_publish_total{topic,broker,status}`、`mq_consume_total{topic,broker,status}`、`mq_delay_message_total{topic}`
- **HistogramVec**：`mq_publish_duration_seconds{topic,broker}`、`mq_consume_duration_seconds{topic,broker}`
- **Gauge**：`mq_consumer_lag{topic,partition}`、`mq_memory_queue_depth{topic}`、`mq_delay_queue_size`、`mq_producers_active`、`mq_consumers_active`
- `instrumentedProducer` 装饰器：`RecordPublish` 自动记录成功/失败 + 延迟
- `observabilityCounter` 原子计数器：内存 broker 队列深度等内部指标
- 所有指标通过 `promauto` 自动注册到默认 Prometheus Registerer，`/metrics` 端点自动暴露

---

## 5. 与 asynq 能力对比矩阵

| 能力 | asynq v0.25 | Astra MQ v2（当前设计） | Astra MQ v2（优化后） |
|------|------------|----------------------|---------------------|
| **Publish/Subscribe** | ✅ Client.Enqueue | ✅ Publish/Subscribe | ✅ 不变 |
| **延迟投递** | ✅ ProcessIn(d time.Duration) | ❌ 无 | ✅ Message.Delay |
| **幂等性** | ✅ TaskID + Unique(time.Duration) | ❌ 无 | ✅ Message.IdempKey + Redis |
| **消息 TTL** | ✅ 默认 24h | ❌ 无 | ✅ Message.TTL |
| **重试次数控制** | ✅ MaxRetry(n) | ❌ 无 | ✅ Message.RetryMax |
| **语义化错误** | ✅ 仅 error | ❌ 无 | ✅ MQError（Permanent/Retry/Skip） |
| **阶梯重试** | ✅ 指数退避（可自定义） | ❌ 无 | ✅ RetryPolicy.Levels |
| **死信队列** | ✅ DeadLetterServer | ❌ 无 | ✅ MQError.ErrPermanent → DLQ topic |
| **优先级队列** | ✅ 13 档 | ❌ 无 | ⚠️ 依赖后端 CapPriority |
| **分区有序** | ❌ | ⚠️ 依赖后端 | ⚠️ 依赖后端 CapOrdered |
| **Web UI** | ✅ asynq Inspector | ❌ 无 | ❌ 无 |
| **CLI 管理** | ✅ asynq CLI | ❌ 无 | ❌ 无 |
| **Metrics** | ❌ 手动 | ❌ 无 | ✅ Prometheus Metrics |
| **Astra 生命周期集成** | ❌ 独立库 | ❌ 无 | ✅ Register(app, opts) |
| **App.OnStart/OnStop** | ❌ 独立库 | ❌ 无 | ✅ |
| **路由层集成** | ❌ 手动 | ❌ 无 | ✅ POST /_mq/:topic |
| **分布式追踪** | ❌ 手动 | ❌ 无 | ✅ Message.TraceID |
| **后端切换** | ❌ 仅 Redis | ✅ 多后端 | ✅ 多后端 + 能力探测 |
| **部署复杂度** | 仅 Redis | 新增 RocketMQ/Kafka 等 | 无变化（补偿层用已有 Redis） |

---

## 6. 附录

### 6.1 当前 asynq 任务类型清单

> async-svc 项目 21 类任务，来源：旧 mqueue 业务

| 分类 | 任务类型 | 延迟策略 | 优先级 |
|------|---------|---------|--------|
| 广告归因 | track_adjust | 延迟 15m | 普通 |
| 广告归因 | deep_link_match | 延迟 5m | 普通 |
| 订单处理 | auto_deliver | 阶梯重试（15s/30s/45s/60s/75s） | **高** |
| 订单处理 | order_sync | 立即 | 普通 |
| 账号管理 | delete_account | 延迟 7d | 低 |
| 账号管理 | frozen_account | 立即 | 高 |
| 支付 | payment_timeout | 延迟 30m | 高 |
| 支付 | refund_process | 立即 | 高 |
| 通知 | push_notification | 立即 | 普通 |
| 通知 | sms_send | 立即 | 普通 |
| 数据同步 | data_sync_external | 立即 | 低 |
| 数据同步 | report_generate | 延迟 1h | 低 |
| 安全 | token_refresh | 立即 | 高 |
| 安全 | risk_control_check | 立即 | 高 |
| 营销 | coupon_expire_remind | 延迟 24h | 低 |
| 营销 | activity_end_notify | 延迟（用户指定） | 低 |
| 监控 | health_check | 立即 | 普通 |
| 监控 | alert_push | 立即 | **极高** |
| 系统 | log_clean | 延迟 7d | 低 |
| 系统 | cache_warm | 延迟 30m | 普通 |
| 系统 | retry_dead_message | 立即 | 普通 |

### 6.2 RocketMQ 固定延迟级别

| Level | 时间 | Level | 时间 | Level | 时间 |
|-------|------|-------|------|-------|------|
| 1 | 1s | 7 | 3m | 13 | 9m |
| 2 | 5s | 8 | 4m | 14 | 10m |
| 3 | 10s | 9 | 5m | 15 | 20m |
| 4 | 30s | 10 | 6m | 16 | 30m |
| 5 | 1m | 11 | 7m | 17 | 1h |
| 6 | 2m | 12 | 8m | 18 | 2h |

### 6.3 参考文档

- [github.com/astra-go/astra](https://github.com/astra-go/astra)
- [ADR-001: Core Module Dependency Boundary](https://github.com/astra-go/astra/blob/main/docs/adr/ADR-001-core-dependency-boundary.md)
- [ADR-005: 子模块数量上限策略](https://github.com/astra-go/astra/blob/main/docs/adr/ADR-005-module-count-limit.md)
- [Astra MQ 迁移指南 v1 → v2](https://github.com/astra-go/astra/blob/main/docs/migration-guide-mq-v2.md)
- [hibiken/asynq v0.25](https://github.com/hibiken/asynq)
- [RocketMQ v5 Go Client](https://github.com/apache/rocketmq-clients/tree/master/golang)
- [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream)
- [Apache Pulsar Go Client](https://github.com/apache/pulsar-client-go)
