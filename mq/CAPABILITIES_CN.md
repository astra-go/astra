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
|------|:--------:|:--------:|:-----:|:------:|:----:|:----:|:------:|
| **CapArbitraryDelay** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapFixedDelay** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapNakDelay** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapIdempotency** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapPriority** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| **CapOrdered** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapDLQ** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapRetry** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapMultiGroup** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **CapTx** | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| **CapBatch** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **总分** | **11/11** | **11/11** | **11/11** | **11/11** | **10/11** | **10/11** | **9/11** | **9/11** |

**图例**：✅ = 支持，❌ = 不支持

---

## 适配器详情

### 1. RabbitMQ（11/11）

**Broker**: RabbitMQ + `rabbitmq_delayed_message_exchange` 插件

**已支持能力**：
- ✅ 任意延迟：通过 `x-delayed-message` 插件 + `x-delay` 头实现
- ✅ NAK 延迟：通过 republish + `x-delay` 实现（语义等价）
- ✅ 幂等去重：通过 `InMemoryIdempCache` 或 `RedisIdempCache`
- ✅ 优先级队列：通过队列 `x-max-priority` 参数实现
- ✅ 有序投递：单队列 FIFO + 单消费者
- ✅ 死信队列：通过 DLX（Dead Letter Exchange）
- ✅ 重试策略：阶梯重试 + `RetryPolicy`
- ✅ 多消费组：通过多队列绑定
- ✅ 事务消息：通过 AMQP `TxSelect()` / `TxCommit()` / `TxRollback()`
- ✅ 固定延迟：通过每级延迟队列（TTL → DLX → 真实主题）
- ✅ 批量发送：客户端聚合

**实现说明**：
- 任意延迟需安装 `rabbitmq_delayed_message_exchange` 插件
- 幂等去重为客户端实现（KV 缓存），非 broker 原生
- 事务消息为 AMQP 级别（非分布式事务）

---

### 2. RocketMQ（11/11）🏆

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
- 唯一满分适配器（11/11）
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
- 任意延迟需自定义实现（非原生）

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
- ✅ NAK 延迟：通过 `Nak()` + 可选延迟
- ✅ 多消费组：队列组机制
- ✅ 批量发送：客户端聚合
- ✅ 死信队列：通过 `MaxDeliver` + `DeliverSubject`
- ✅ 重试策略：通过 `MaxDeliver` 重投递
- ✅ 有序投递：通过 `OrderedConsumer()`
- ✅ 幂等去重：JetStream KV bucket 去重
- ✅ 固定延迟：通过 JetStream 延迟消息头 + re-publisher goroutine
- ✅ 任意延迟：通过 `ArbitraryDelay()` + 客户端 re-publisher goroutine
- ✅ 优先级队列：通过多主题路由 (`topic.p0..pN`) + 消费者端堆

**缺失能力**：
- ❌ **事务消息** — NATS 协议和 JetStream 均无事务语义（无 Begin/Commit/Rollback），KV bucket 仅能模拟状态追踪但无法保证跨消息原子性；协议层面不支持，强行实现仅为仿真级别，无实际生产价值



**实现说明**：
- 必须启用 JetStream 才能支持持久化和死信队列
- 幂等去重使用 JetStream KV bucket 存储去重键
- 有序消费者保证 FIFO，但遇到消息缺失时会重置
- 固定/任意延迟：需启动 `StartRePublisher()` / `StartArbitraryDelayPublisher()` goroutine（客户端实现，非崩溃安全）
- 优先级：生产者路由到 `topic.pN`，消费者使用内部堆优先消费高优先级消息
- **CapTx 保持 false** — JetStream KV 可模拟事务状态跟踪，但无原子性保证

---

### 7. MQTT（9/11）

**Broker**: EMQX / Mosquitto / NanoMQ（MQTT v5.0）

**已支持能力**：
- ✅ 多消费组：共享订阅（$share/group/topic）
- ✅ 批量发送：客户端聚合
- ✅ 重试策略：MQTT v5.0 Retry 标志
- ✅ 死信队列：DLQTopic 转发（MaxRetries 次后）
- ✅ 幂等去重：客户端 IdempKey 去重
- ✅ 有序投递：单 Topic 保序（QoS 1/2）
- ✅ NAK 延迟：重试主题 $retry/<count>/<topic> + NakDelay
- ✅ 固定延迟：$delay/<level>/<original_topic> 主题路由
- ✅ 任意延迟：$arb/<delay_ms>/<original_topic> + 消费者定时器

**缺失能力**：
- ❌ **事务消息** — MQTT 是轻量级 Pub/Sub 协议（即使是 v5），协议层无事务语义（无 Begin/Commit/Rollback），PUBLISH 命令是原子操作但无法跨消息组事务
- ❌ **优先级队列** — MQTT broker 按订阅投递，不区分消息优先级；客户端堆排序仅在消费端生效，broker 投递顺序不受控制，属于伪优先级实现

**实现说明**：
- 共享订阅需 MQTT v5.0 broker
- 重试/DLQ 使用主题编码：$retry/<count>/<topic>、$delay/<level>/<topic>、$arb/<ms>/<topic>
- 固定/任意延迟使用消费者端 time.After（非 broker 原生）
- 幂等去重需设置 EnableIdempotency=true

### 8. Memory（9/11）

**Broker**: 进程内 Go channel（仅测试）

**已支持能力**：
- ✅ 任意延迟：通过 `time.Timer` 延迟投递
- ✅ 固定延迟级别：通过 `MemoryBrokerConfig.FixedDelayLevels` 配置级别，`FixedDelay()` 按级别投递
- ✅ NAK 延迟：通过 `MemoryConsumerConfig.NakDelay` 配置，handler 返回 error 后延迟重投递
- ✅ 有序投递：FIFO channel 缓冲
- ✅ 批量发送：通过 `PublishBatch` 聚合发送
- ✅ 幂等去重：通过 `IdempKey` 去重（`MemoryConsumerConfig.Idempotent`）
- ✅ 重试策略：指数退避重试（`MemoryConsumerConfig.MaxRetries`）
- ✅ 死信队列：DLQ channel（`MemoryConsumerConfig.DLQBuffer`）
- ✅ 多消费组：命名消费组 fan-out

**缺失能力**：
- ❌ **事务消息** — Memory broker 是单进程 channel 模型，无跨消息原子性需求；单进程内不需要分布式事务语义，强行实现无实际测试价值
- ❌ **优先级队列** — 需将 channel 替换为 `container/heap`，破坏 FIFO 语义且增加复杂度；对于测试场景，可通过多个 topic 模拟优先级效果

**实现说明**：
- **仅用于测试** — 不适用于生产环境
- 无持久化，进程重启消息丢失
- 无网络开销，单元测试最快
- 使用 `NewMemoryConsumerWithBroker` 共享同一个 broker 连接生产者和消费者
- 建议配合 `NewMemoryProducerWithBroker` 使用

---

## 排名

| 排名 | 适配器 | 分数 | 覆盖率 |
|------|--------|------|--------|
| 🥇 | **RabbitMQ** | 11/11 | 100% |
| 🥇 | **RocketMQ** | 11/11 | 100% |
| 🥇 | **Kafka** | 11/11 | 100% |
| 🥇 | **Pulsar** | 11/11 | 100% |
| 5 | NATS | 10/11 | 91% |
| 6 | Redis | 10/11 | 91% |
| 7 | MQTT | 9/11 | 82% |
| 8 | Memory | 9/11 | 82% |

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
| **固定延迟** | RabbitMQ、RocketMQ、Kafka、Pulsar、NATS、Redis |
| **NAK 延迟** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT |
| **幂等去重** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT |
| **优先级队列** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS |
| **有序投递** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **死信队列** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT |
| **重试策略** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT |
| **多消费组** | 除 Memory 和 Redis 外所有适配器 |
| **事务消息** | RabbitMQ, RocketMQ, Kafka, Pulsar |
| **批量发送** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT |

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

---

## 更新日志

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v1.0.6 | 2026-06-23 | 新增 RabbitMQ NAK 延迟、RocketMQ 优先级 + NAK 延迟 |
| v1.0.5 | 2026-06-23 | MQ 模块初始发布，所有适配器就位 |

---

## 参考链接

- [RabbitMQ 延迟消息插件](https://github.com/rabbitmq/rabbitmq-delayed-message-exchange)
- [RocketMQ v5 文档](https://rocketmq.apache.org/docs/)
- [Kafka KIP-447（精确一次语义）](https://cwiki.apache.org/confluence/display/KAFKA/KIP-447%3A+Producer+scalability+for+exactly+once+semantics)
- [Pulsar 事务消息](https://pulsar.apache.org/docs/next/txn-why/)
- [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream)
- [MQTT v5.0 规范](https://mqtt.org/mqtt-specification/)
