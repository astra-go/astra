# MQ — 消息队列抽象层

统一 Producer/Consumer 接口，支持多种消息队列后端，实现业务代码与具体 Broker 解耦。

---

## 特性

- **统一接口**：Producer 和 Consumer 接口与 Broker 无关
- **8 个后端**：RabbitMQ、Kafka、RocketMQ、MQTT、NATS、Pulsar、Redis、内存 Broker
- **11 项能力**：任意延迟、固定延迟、NAK 延迟、幂等去重、优先级、有序投递、死信队列、重试、多消费组、事务消息、批量发送
- **运行时能力探测**：运行时查询后端能力，支持优雅降级
- **跨后端补偿层**：Redis 实现的延迟投递和幂等去重可跨后端复用

---

## 支持的后端

| # | Broker | 导入路径 | 分数 | 特点 |
|---|--------|----------|------|------|
| 1 | RabbitMQ | `github.com/astra-go/astra/mq/rabbitmq` | 11/11 | 全能力覆盖：延迟/幂等/事务/批量/优先级/DLQ/有序 |
| 2 | RocketMQ | `github.com/astra-go/astra/mq/rocketmq` | 11/11 | 全能力覆盖：延迟/幂等/事务/批量/有序/DLQ |
| 3 | Kafka | `github.com/astra-go/astra/mq/kafka` | 11/11 | 全能力覆盖：延迟/幂等/事务/批量/优先级/有序 |
| 4 | Pulsar | `github.com/astra-go/astra/mq/pulsar` | 11/11 | 全能力覆盖：延迟/幂等/事务/批量/多租户 |
| 5 | NATS | `github.com/astra-go/astra/mq/nats` | 10/11 | 轻量，JetStream 持久化，缺事务 |
| 6 | Redis | `github.com/astra-go/astra/mq/redis` | 10/11 | 基于 Streams，无外部依赖，缺事务 |
| 7 | MQTT | `github.com/astra-go/astra/mq/mqtt` | 10/11 | IoT 优化，共享订阅，缺优先级/事务 |
| 8 | Memory | `github.com/astra-go/astra/mq/memory` | 9/11 | 仅测试用，零外部依赖 |

---

## 快速开始

### Producer

```go
import (
    "github.com/astra-go/astra/mq"
    "github.com/astra-go/astra/mq/rabbitmq"
)

producer, _ := rabbitmq.NewProducer(rabbitmq.RabbitMQConfig{
    URL:      "amqp://guest:guest@localhost:5672/",
    Exchange: "my-exchange",
})
defer producer.Close()

producer.Publish(ctx, &mq.Message{
    Topic:   "user.created",
    Payload: []byte(`{"user_id": 42}`),
})
```

### Consumer

```go
consumer, _ := rabbitmq.NewConsumer(rabbitmq.RabbitMQConfig{
    URL:     "amqp://guest:guest@localhost:5672/",
    Queue:   "notifications",
    AutoAck: false, // 手动 ack（生产环境建议 false）
})
defer consumer.Close()

consumer.Consume(func(ctx context.Context, msg *mq.Message) error {
    fmt.Printf("Received: %s\n", string(msg.Payload))
    return nil // nil = ACK，error = NACK + 重试
})
```

---

## 核心接口

### Producer 接口

```go
type Producer interface {
    Publish(ctx context.Context, msg *Message) error
    Close() error
    Capabilities() Capabilities  // 运行时能力探测
}
```

### Consumer 接口

```go
type Consumer interface {
    Consume(ctx context.Context, handler MessageHandler) error
    Close() error
    Capabilities() Capabilities
}
```

### Message 结构体

```go
type Message struct {
    Topic      string            // 交换机/路由键
    Payload    []byte            // 消息体
    Key        []byte            // 消息 ID（用于幂等）

    // 投递控制
    Delay      time.Duration     // 延迟投递（取决于后端能力）
    IdempKey   string            // 幂等去重键（已处理则跳过）
    Priority   uint8             // 消息优先级（0 = 最低，取决于后端能力）
    RetryCount int               // 当前重试次数（消费者设置）

    // 投递结果元数据
    Headers    map[string]string // 自定义头
}
```

---

## 能力定义

| 能力 | 常量 | 说明 |
|------|------|------|
| **任意延迟** | `CapArbitraryDelay` | 支持任意精确延迟（非固定级别） |
| **固定延迟** | `CapFixedDelay` | 支持固定延迟级别（如 RocketMQ v4 的 18 个级别） |
| **NAK 延迟** | `CapNakDelay` | 支持 NAK + 可配置延迟后再重投递 |
| **幂等去重** | `CapIdempotency` | 原生幂等投递（去重） |
| **优先级队列** | `CapPriority` | 支持优先级队列 |
| **有序投递** | `CapOrdered` | 分区内/队列内保序投递 |
| **死信队列** | `CapDLQ` | 原生死信队列支持 |
| **重试策略** | `CapRetry` | 原生重试（可配置策略） |
| **多消费组** | `CapMultiGroup` | 支持多消费组 |
| **事务消息** | `CapTx` | 支持事务消息 |
| **批量发送** | `CapBatch` | 支持批量发送 |

---

## 能力矩阵

| 能力 | RabbitMQ | RocketMQ | Kafka | Pulsar | NATS | MQTT | Memory | Redis |
|------|:--------:|:--------:|:-----:|:------:|:----:|:----:|:------:|:-----:|
| 任意延迟 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 固定延迟 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| NAK 延迟 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 幂等去重 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 优先级队列 | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ |
| 有序投递 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 死信队列 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 重试策略 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 多消费组 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ |
| 事务消息 | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| 批量发送 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **总分** | **11/11** | **11/11** | **11/11** | **11/11** | **10/11** | **10/11** | **9/11** | **10/11** |

---

## 各后端速查

### RabbitMQ（`mq/rabbitmq`）

- **要求**：需安装 `rabbitmq_delayed_message_exchange` 插件才能支持任意延迟
- **优先级**：队列需声明 `x-max-priority`
- **事务**：AMQP `TxSelect()`/`TxCommit()`/`TxRollback()`
- **幂等**：`InMemoryIdempCache`（进程内）或 `RedisIdempCache`（分布式）

### RocketMQ（`mq/rocketmq`）

- **要求**：RocketMQ v5（v4 仅支持固定延迟）
- **任意延迟**：原生 `SetDelayTimestamp()`，无需插件
- **事务**：`TransactionProducer` + `TransactionChecker` 回调
- **有序**：通过 `MessageGroup`（分片键）保证同组有序

### Kafka（`mq/kafka`）

- **要求**：Kafka 2.8+（推荐 KRaft 模式）
- **任意延迟**：republish 机制（消费者侧定时器）
- **幂等**：broker 原生（`EnableIdempotent`：producer ID + 序列号）
- **事务**：Kafka Transactions API，消费者需配置 `isolation.level=read_committed`

### Pulsar（`mq/pulsar`）

- **要求**：Pulsar 2.10+
- **任意延迟**：原生 `DeliverAfter()` / `DeliverAt()`
- **事务**：Pulsar Transactions API
- **多租户**：内置 namespace 隔离

### NATS（`mq/nats`）

- **要求**：NATS 2.9+ 且启用 JetStream
- **延迟**：`PublishAsync` + re-publisher goroutine（客户端实现，非崩溃安全）
- **优先级**：多主题路由（`topic.p0..pN`）+ 消费者端堆排序
- **限制**：不支持事务

### Redis（`mq/redis`）

- **要求**：Redis 6.2+ Streams
- **延迟**：`ZADD` 到延迟 Sorted Set + `delayPump` 后台轮询（不保证崩溃安全）
- **优先级**：`PriorityStreams` 多 Stream 路由配置
- **限制**：`MULTI/EXEC` 非分布式事务

### MQTT（`mq/mqtt`）

- **要求**：MQTT v5.0 broker
- **延迟**：主题编码（`$arb/<ms>/<topic>`、`$delay/<level>/<topic>`）+ 消费者定时器
- **多消费组**：共享订阅（`$share/group/topic`）
- **限制**：不支持原生优先级，不支持事务

### Memory（`mq/memory`）

- **无外部依赖**
- **适用场景**：单元测试、本地开发
- **限制**：无持久化、无多消费组、无事务、无优先级

---

## IdempCache 幂等接口

幂等去重通过 `IdempCache` 接口实现：

```go
type IdempCache interface {
    IsProcessed(key string) bool
    MarkProcessed(key string, ttl time.Duration)
}
```

内置两种实现：
- `NewInMemoryIdempCache(ttl time.Duration)` — 进程内，进程重启后失效
- `NewRedisIdempCache(client *redis.Client)` — 分布式，生产可用

---

## RetryPolicy 重试策略

阶梯退避重试，可配置级别：

```go
type RetryPolicy struct {
    MaxRetries int             // 最大重试次数，0 = 默认 3
    Levels     []time.Duration // 阶梯延迟，如 {15s, 30s, 45s, 60s, 75s}
    Base       time.Duration   // 指数退避基础（Levels 为空时使用）
}
```

若 `Levels` 为空，使用指数退避：`delay = retryCount² × Base`。

---

## 能力探测

运行时查询后端能力，避免使用不支持的功能：

```go
caps := producer.Capabilities()

// 检查单项能力
if caps.Has(mq.CapArbitraryDelay) {
    // 后端支持延迟投递
}

// 检查多项能力
required := []mq.Capability{mq.CapDLQ, mq.CapRetry}
for _, cap := range required {
    if !caps.Has(cap) {
        log.Printf("后端不支持: %s", cap)
    }
}
```

---

## 优雅降级

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

## 注意事项

- Producer 和 Consumer 都必须调用 `Close()` 关闭连接释放资源
- 生产环境建议使用 `AutoAck: false` 并手动 ack
- Consumer 处理失败时根据 RetryPolicy 重试，超过最大次数则发往 DLQ
- RabbitMQ 任意延迟需安装 `rabbitmq_delayed_message_exchange` 插件
- RocketMQ v5 原生支持任意延迟，无需插件
- Memory broker（`mq/memory`）仅用于测试，无持久化、无分布式保证
