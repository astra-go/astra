# MQ 适配器能力矩阵

本文档提供 astra 框架中所有 MQ 适配器能力的完整概览。

## 概述

astra MQ 模块提供 8 个消息队列适配器，具有不同级别的功能支持。每个适配器实现 `Producer` 和 `Consumer` 接口，并通过 `Capabilities()` 方法声明能力。

---

## 能力定义

| 能力 | 常量 | 说明 |
|------|------|------|
| **任意延迟** | `CapArbitraryDelay` | 支持任意延迟投递（非固定延迟级别） |
| **固定延迟** | `CapFixedDelay` | 支持固定延迟级别（如 RocketMQ v4 的 18 个级别） |
| **NAK 延迟** | `CapNakDelay` | 支持 NAK + 可配置延迟（失败后延迟重投递） |
| **幂等去重** | `CapIdempotency` | 原生幂等投递（去重） |
| **优先级队列** | `CapPriority` | 支持优先级队列 |
| **有序投递** | `CapOrdered` | 分区内保序投递 |
| **死信队列** | `CapDLQ` | 原生死信队列支持 |
| **重试策略** | `CapRetry` | 原生重试支持（可配置策略） |
| **多消费组** | `CapMultiGroup` | 支持多消费组 |
| **事务消息** | `CapTx` | 支持事务消息 |
| **批量发送** | `CapBatch` | 支持批量发送 |

---

## 能力矩阵

| 能力 | RabbitMQ | RocketMQ | Kafka | Pulsar | NATS | MQTT | Memory | Redis |
|------|:--------:|:--------:|:-----:|:------:|:----:|:----:|:------:|:-----:|
| **CapArbitraryDelay** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapFixedDelay** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapNakDelay** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapIdempotency** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapPriority** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ |
| **CapOrdered** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapDLQ** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapRetry** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapMultiGroup** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapTx** | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| **CapBatch** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **总分** | **11/11** | **11/11** | **11/11** | **11/11** | **10/11** | **10/11** | **9/11** | **10/11** |

**图例**：✅ = 支持，❌ = 不支持

---

## 适配器详情

### 1. RabbitMQ（11/11）

**Broker**: RabbitMQ + `rabbitmq_delayed_message_exchange` 插件

**已支持能力**：
- ✅ 任意延迟：通过 `x-delayed-message` 插件 + `x-delay` 头实现
- ✅ 固定延迟：通过每级延迟队列（TTL → DLX → 真实主题）
- ✅ NAK 延迟：通过 republish + `x-delay` 实现（语义等价）
- ✅ 幂等去重：通过 `InMemoryIdempCache` 或 `RedisIdempCache`
- ✅ 优先级队列：通过队列 `x-max-priority` 参数实现
- ✅ 有序投递：单队列 FIFO + 单消费者
- ✅ 死信队列：通过 DLX（Dead Letter Exchange）
- ✅ 重试策略：阶梯重试 + `RetryPolicy`
- ✅ 多消费组：通过多队列绑定
- ✅ 事务消息：通过 AMQP `TxSelect()` / `TxCommit()` / `TxRollback()`
- ✅ 批量发送：客户端聚合

**实现说明**：
- 任意延迟需安装 `rabbitmq_delayed_message_exchange` 插件
- 幂等去重为客户端实现（KV 缓存），非 broker 原生
- 事务消息为 AMQP 级别（非分布式事务）

---

### 2. RocketMQ（11/11）

**Broker**: Apache RocketMQ v5

**已支持能力**：
- ✅ 任意延迟：通过 `SetDelayTimestamp()`（RocketMQ v5）
- ✅ 固定延迟：通过 18 个预定义延迟级别（兼容 v4）
- ✅ NAK 延迟：通过 `ChangeInvisibleDuration()` API
- ✅ 幂等去重：客户端 `IdempCache` + broker keys
- ✅ 优先级队列：多 Topic 路由 + 消费者端 `container/heap` 排序
- ✅ 有序投递：通过 `MessageGroup`（分片键）
- ✅ 死信队列：原生 `%DLQ%` Topic
- ✅ 重试策略：原生重试队列 + 阶梯延迟
- ✅ 多消费组：消费组机制
- ✅ 事务消息：通过 `TransactionProducer` + `TransactionChecker`
- ✅ 批量发送：通过 `Batch()` API

**实现说明**：
- 优先级采用多 Topic 模式（RocketMQ 无原生优先级队列）
- 事务消息需实现 `TransactionChecker` 回调

---

### 3. Kafka（11/11）

**Broker**: Apache Kafka（推荐 KRaft 模式）

**已支持能力**：
- ✅ 任意延迟：通过 republish 机制实现
- ✅ 固定延迟：通过每级延迟主题 + 转发消费者（需后台 goroutine）
- ✅ NAK 延迟：通过 `NakWithDelay()`（pause/resume consumer）
- ✅ 幂等去重：通过 `EnableIdempotent` 生产者配置
- ✅ 优先级队列：Topic 分区 + 消费者端排序
- ✅ 有序投递：分区顺序保证
- ✅ 死信队列：通过 `DLQTopic` 配置
- ✅ 重试策略：通过 `RetryPolicy` + 退避
- ✅ 多消费组：消费组机制
- ✅ 事务消息：Kafka Transactions API（精确一次语义）
- ✅ 批量发送：生产者批量机制

**实现说明**：
- 幂等去重为 broker 原生（生产者 ID + 序列号）
- 事务消息需消费者配置 `isolation.level=read_committed`
- 固定延迟发布到分层延迟主题，后台消费者到期后转发

---

### 4. Pulsar（11/11）

**Broker**: Apache Pulsar

**已支持能力**：
- ✅ 任意延迟：通过 `DeliverAfter()` API
- ✅ 固定延迟：通过 `DeliverAfter()`（映射到最近延迟级别）
- ✅ NAK 延迟：通过带延迟的 republish
- ✅ 幂等去重：`IdempCache` + 序列 ID
- ✅ 优先级队列：消息优先级元数据
- ✅ 有序投递：分区 Topic + 键路由
- ✅ 死信队列：通过 `DeadLetterPolicy` 配置
- ✅ 重试策略：通过 `RetryPolicy` + 退避
- ✅ 多消费组：订阅名机制
- ✅ 事务消息：Pulsar Transactions API
- ✅ 批量发送：生产者批量机制

**实现说明**：
- 任意延迟为原生支持（`DeliverAfter()` 或 `DeliverAt()`）
- 幂等去重结合客户端缓存 + broker 序列 ID
- 事务消息需 broker 配置启用

---

### 5. NATS（10/11）

**Broker**: NATS + JetStream

**已支持能力**：
- ✅ 任意延迟：通过 `ArbitraryDelay()` + 客户端 re-publisher goroutine
- ✅ 固定延迟：通过 JetStream 延迟消息头 + re-publisher goroutine
- ✅ NAK 延迟：通过 `Nak()` + 可选延迟
- ✅ 幂等去重：JetStream KV bucket 去重
- ✅ 优先级队列：通过多主题路由 (`topic.p0..pN`) + 消费者端堆
- ✅ 有序投递：通过 `OrderedConsumer()`
- ✅ 死信队列：通过 `MaxDeliver` + `DeliverSubject`
- ✅ 重试策略：通过 `MaxDeliver` 重投递
- ✅ 多消费组：队列组机制
- ✅ 批量发送：客户端聚合

**缺失能力**：
- ❌ **事务消息** — NATS 协议和 JetStream 均无事务语义（无 Begin/Commit/Rollback），KV bucket 仅能模拟状态追踪但无法保证跨消息原子性

**实现说明**：
- 必须启用 JetStream 才能支持持久化和死信队列
- 固定/任意延迟：需启动 `StartRePublisher()` / `StartArbitraryDelayPublisher()` goroutine（客户端实现，非崩溃安全）
- 优先级：生产者路由到 `topic.pN`，消费者使用内部堆优先消费高优先级消息

---

### 6. Redis Streams（10/11）

**Broker**: Redis 6.2+ Streams

**已支持能力**：
- ✅ 任意延迟：通过 `ZADD` 到延迟 Sorted Set，`delayPump` 后台轮询到期后 `XADD`
- ✅ 固定延迟：通过 `RedisConfig.FixedDelayLevels`，`FixedDelay()` 按级别投递
- ✅ NAK 延迟：通过 `XCLAIM` min-idle-time，消息延迟后重投递
- ✅ 幂等去重：Redis SET 存储 `msg.IdempKey`，成功后写入 TTL
- ✅ 优先级队列：通过 `PriorityStreams` 多 Stream 路由（`topic.p0/p1/p2`）+ 消费者优先级排序
- ✅ 有序投递：Redis Stream ID（`timestamp-sequence`）单调递增
- ✅ 死信队列：重试耗尽后 `XADD` 到配置的 `DLQStream`
- ✅ 重试策略：指数退避（`retryCount² × base`）或级别制，通过 `XCLAIM`
- ✅ 多消费组：`XGROUP CREATE` 支持多消费组
- ✅ 批量发送：Redis pipeline 批量 `XADD`

**缺失能力**：
- ❌ **事务消息** — Redis `MULTI/EXEC` 是乐观并发控制，非分布式事务；`WATCH` 不适用于 MQ 场景

**配置示例**：

```go
// 生产者
p := mq.NewRedisProducer(mq.RedisProducerConfig{
    Addr:             "localhost:6379",
    Stream:           "orders",
    FixedDelayLevels: []int64{1000, 5000, 10000, 30000, 60000},
    PriorityStreams:  map[int]string{3: "orders:high", 2: "orders:normal", 1: "orders:low"},
})

// 消费者
c, _ := mq.NewRedisConsumer(mq.RedisConsumerConfig{
    Addr:          "localhost:6379",
    Stream:        "orders",
    ConsumerGroup: "order-service",
    ConsumerName:  "instance-1",
    DLQStream:     "orders.dlq",
    RetryPolicy: mq.RetryPolicy{
        MaxRetries: 3,
        Base:       1 * time.Second,
    },
    Idempotent:     true,
    IdempotencyTTL: 24 * time.Hour,
    PriorityStreams: map[int]string{3: "orders:high", 2: "orders:normal", 1: "orders:low"},
})
```

**实现说明**：
- 延迟投递依赖 `delayPump` 后台 goroutine（`ZADD` + `ZPOPMIN`）；非崩溃安全——进程重启会丢失在途延迟条目
- 推荐 Redis 主从模式；主节点故障时 Stream 可用但延迟 Sorted Set 可能丢失
- 适用场景：测试、中小项目、轻量级异步任务

---

### 7. MQTT（10/11）

**Broker**: EMQX / Mosquitto / NanoMQ（MQTT v5.0）

**已支持能力**：
- ✅ 任意延迟：`$arb/<delay_ms>/<original_topic>` + 消费者定时器
- ✅ 固定延迟：`$delay/<level>/<original_topic>` 主题路由
- ✅ NAK 延迟：重试主题 `$retry/<count>/<original_topic>` + NakDelay
- ✅ 幂等去重：客户端 IdempKey 去重
- ✅ 有序投递：单 Topic 保序（QoS 1/2）
- ✅ 死信队列：DLQTopic 转发（MaxRetries 次后）
- ✅ 重试策略：MQTT v5.0 Retry 标志
- ✅ 多消费组：共享订阅（`$share/group/topic`）
- ✅ 批量发送：客户端聚合

**缺失能力**：
- ❌ **事务消息** — MQTT 是轻量级 Pub/Sub 协议，无事务语义；PUBLISH 单消息原子但无法跨消息组事务
- ❌ **优先级队列** — MQTT broker 按订阅投递不区分优先级；客户端排序仅影响消费端，broker 投递顺序不受控

**实现说明**：
- 共享订阅需 MQTT v5.0 broker
- 重试/DLQ 使用主题编码：`$retry/<count>/<topic>`、`$delay/<level>/<topic>`、`$arb/<ms>/<topic>`
- 固定/任意延迟使用消费者端 `time.After`（非 broker 原生）

---

### 8. Memory（9/11）

**Broker**: 进程内 Go channel（仅测试）

**已支持能力**：
- ✅ 任意延迟：通过 `time.Timer` 延迟投递
- ✅ 固定延迟：通过 `MemoryBrokerConfig.FixedDelayLevels`，`FixedDelay()` 按级别投递
- ✅ NAK 延迟：通过 `MemoryConsumerConfig.NakDelay`，handler 错误触发延迟重投递
- ✅ 幂等去重：通过 `IdempKey` 去重（`MemoryConsumerConfig.Idempotent`）
- ✅ 有序投递：FIFO channel 缓冲
- ✅ 死信队列：DLQ channel（`MemoryConsumerConfig.DLQBuffer`）
- ✅ 重试策略：指数退避（`MemoryConsumerConfig.MaxRetries`）
- ✅ 多消费组：命名消费组 fan-out
- ✅ 批量发送：通过 `PublishBatch` 聚合发送

**缺失能力**：
- ❌ **事务消息** — 单进程 channel 模型无需跨消息原子性；分布式事务语义在测试 broker 中无意义
- ❌ **优先级队列** — 需将 channel 替换为 `container/heap`，破坏 FIFO 语义；多 topic 模拟更简单

**实现说明**：
- **仅用于测试** — 不适用于生产环境
- 无持久化，进程重启消息丢失
- 使用 `NewMemoryConsumerWithBroker` / `NewMemoryProducerWithBroker` 共享同一 broker 实例

---

## 排名

| 排名 | 适配器 | 分数 | 覆盖率 |
|:----:|--------|------|--------|
| 🥇 | **RabbitMQ** | 11/11 | 100% |
| 🥇 | **RocketMQ** | 11/11 | 100% |
| 🥇 | **Kafka** | 11/11 | 100% |
| 🥇 | **Pulsar** | 11/11 | 100% |
| 5 | **NATS** | 10/11 | 91% |
| 5 | **Redis** | 10/11 | 91% |
| 7 | **MQTT** | 10/11 | 91% |
| 8 | **Memory** | 9/11 | 82% |

---

## 适配器选型指南

### 生产环境推荐

| 场景 | 推荐适配器 | 理由 |
|------|------------|------|
| **金融/交易** | RocketMQ | 全能力覆盖（11/11） |
| **高吞吐** | Kafka | 优秀批量 + 分区扩展 |
| **低延迟** | NATS | 极低开销，云原生 |
| **IoT/边缘** | MQTT | 轻量协议，设备支持广泛 |
| **事件流** | Pulsar | 分层存储 + 地理复制 |
| **测试/开发** | Memory、Redis | 无外部依赖 |

### 按能力选型

| 必需能力 | 支持的适配器 |
|----------|--------------|
| **任意延迟** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **固定延迟** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **NAK 延迟** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **幂等去重** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **优先级队列** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, Redis |
| **有序投递** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **死信队列** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **重试策略** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **多消费组** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Redis |
| **事务消息** | RabbitMQ, RocketMQ, Kafka, Pulsar |
| **批量发送** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |

---

## 能力降级策略

当某项能力不支持时，框架提供以下降级行为：

| 缺失能力 | 降级行为 |
|----------|----------|
| `CapArbitraryDelay` | 立即投递（无延迟） |
| `CapFixedDelay` | 若支持 `CapArbitraryDelay` 则使用 |
| `CapNakDelay` | 立即 NACK（无延迟重投递） |
| `CapIdempotency` | 应用需自行处理重复消息 |
| `CapPriority` | FIFO 投递（无优先级排序） |
| `CapOrdered` | 无顺序保证 |
| `CapDLQ` | 达到最大重试次数后丢弃消息 |
| `CapRetry` | 无自动重试 |
| `CapTx` | 非事务性发布 |
| `CapBatch` | 顺序发布 |

---

## 版本兼容性

| 适配器 | 最低 Broker 版本 | Go 客户端库 |
|--------|------------------|-------------|
| RabbitMQ | 3.8+（需延迟插件） | `github.com/rabbitmq/amqp091-go` |
| RocketMQ | 5.0+ | `github.com/apache/rocketmq-clients/golang` |
| Kafka | 2.8+ (KRaft) | `github.com/IBM/sarama` |
| Pulsar | 2.10+ | `github.com/apache/pulsar-client-go` |
| NATS | 2.9+ (JetStream) | `github.com/nats-io/nats.go` |
| MQTT | 5.0+ | `github.com/eclipse/paho.mqtt.golang` |
| Redis | 6.2+ | `github.com/redis/go-redis/v9` |
| Memory | 无 | 无（进程内） |

---

## 更新日志

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v1.0.5 | 2026-06-24 | MQ 模块初始发布 含所有适配器|

---

## 参考链接

- [RabbitMQ 延迟消息插件](https://github.com/rabbitmq/rabbitmq-delayed-message-exchange)
- [RocketMQ v5 文档](https://rocketmq.apache.org/docs/)
- [Kafka KIP-447（精确一次语义）](https://cwiki.apache.org/confluence/display/KAFKA/KIP-447%3A+Producer+scalability+for+exactly+once+semantics)
- [Pulsar 事务消息](https://pulsar.apache.org/docs/next/txn-why/)
- [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream)
- [MQTT v5.0 规范](https://mqtt.org/mqtt-specification/)
- [Redis Streams](https://redis.io/docs/data-types/streams/)
