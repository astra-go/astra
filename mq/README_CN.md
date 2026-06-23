# MQ — 消息队列抽象

统一 Producer/Consumer 接口，支持多种消息队列后端，实现业务代码与具体 Broker 解耦。

## 特性

- **统一接口**：Producer 和 Consumer 接口与 Broker 无关
- **多后端**：RabbitMQ、Kafka、RocketMQ、MQTT、NATS、Pulsar、内存 Broker（开发/测试用）
- **能力探测**：运行时查询后端能力（延迟投递、幂等、重试、DLQ 等）
- **Consumer Group**：多消费者负载均衡消费
- **消息重试**：失败消息自动阶梯重试或进入死信队列
- **Redis 补偿层**：延迟投递和幂等去重的跨后端统一实现

## 支持的后端

| Broker | 导入路径 | 特点 |
|--------|----------|------|
| RabbitMQ | `github.com/astra-go/astra/mq/rabbitmq` | 功能最全：延迟/幂等/事务/批量/优先级/DLQ |
| Apache Kafka | `github.com/astra-go/astra/mq/kafka` | 高吞吐，事件驱动 |
| Apache RocketMQ | `github.com/astra-go/astra/mq/rocketmq` | 功能对齐 RabbitMQ：延迟/幂等/重试/DLQ/事务/批量/有序 |
| MQTT | `github.com/astra-go/astra/mq/mqtt` | IoT 场景 |
| NATS | `github.com/astra-go/astra/mq/nats` | 轻量，JetStream 可持久化 |
| Apache Pulsar | `github.com/astra-go/astra/mq/pulsar` | 多租户，分层存储 |
| 内存 Broker | `github.com/astra-go/astra/mq/memory` | 本地开发/测试，无持久化 |

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
    URL:    "amqp://guest:guest@localhost:5672/",
    Queue:  "notifications",
    AutoAck: false, // 手动 ack
})
defer consumer.Close()

consumer.Consume(func(ctx context.Context, msg *mq.Message) error {
    fmt.Printf("Received: %s\n", string(msg.Payload))
    return nil // nil = ACK，error = NACK 并可能重试
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

type MessageHandler func(ctx context.Context, msg *Message) error
```

### Message 结构体（增强版）

```go
type Message struct {
    Topic      string            // 交换机/路由键
    Payload    []byte            // 消息体
    Key        []byte            // 消息 ID（用于幂等）
    Headers    map[string]string // 自定义头

    // 增强字段
    Delay      time.Duration     // 延迟投递（x-delay header）
    IdempKey   string            // 幂等去重键（已处理则跳过）
    RetryCount int               // 当前重试次数（消费者写入）
    Priority   uint8             // AMQP 优先级（0-9）
}
```

---

## RabbitMQ 能力清单

### 能力矩阵

| 能力 | 标志 | 实现状态 | 说明 |
|------|------|----------|------|
| 任意延迟投递 | `CapArbitraryDelay` | ✅ 需插件 | 依赖 `rabbitmq-delayed-message-exchange` |
| 幂等去重 | `CapIdempotency` | ⚠️ 接口完整 | `IdempCache` 接口已实现；`RedisIdempCache` 为 TODO |
| 阶梯重试 | `CapRetry` | ✅ 已实现 | `RetryPolicy.Levels` + `NextDelay()`，消费者自动 republish |
| 死信队列 | `CapDLQ` | ✅ 已实现 | 配置 `DLQExchange` + `DLQQueue`，失败消息自动转发 |
| 事务 | `CapTx` | ✅ 已实现 | 配置 `EnableTx: true`，`Tx()` / `TxCommit()` / `TxRollback()` |
| 批量发送 | `CapBatch` | ✅ 已实现 | `BatchSize` + 后台 `batchFlusher` goroutine |
| 优先级队列 | `CapPriority` | ✅ 已实现 | `Priority` 字段；队列需声明 `x-max-priority` |
| 多消费组 | `CapMultiGroup` | ✅ 已实现 | vhost + 独立 queue 绑定 |
| 有序投递 | `CapOrdered` | ❌ 未实现 | 声明为 true 但无特殊保证（建议 QoS prefetch=1） |

### RabbitMQConfig 完整配置

```go
type RabbitMQConfig struct {
    // 连接
    URL string  // amqp://user:pass@host:5672/

    // 交换机
    Exchange  string  // 默认交换机名
    ExType   string  // 类型：direct/topic/fanout/headers/x-delayed-message

    // 延迟投递（需启用 rabbitmq_delayed_message_exchange 插件）
    DelayExchange string  // 延迟交换机名（如 "my-delay-exchange"）

    // 队列
    Queue    string  // 队列名
    Durable  bool    // 持久化，默认 true

    // 手动 ack
    AutoAck  bool    // 默认 false（生产环境建议 false）

    // 死信队列
    DLQExchange string  // 死信交换机名
    DLQQueue    string  // 死信队列名

    // 幂等去重（内存实现，生产建议换 RedisIdempCache）
    IdempCache IdempCache  // nil = 不开启幂等

    // 重试策略（阶梯退避）
    RetryPolicy *RetryPolicy  // nil = 默认 3 次重试

    // 事务模式
    EnableTx bool  // true = 每条消息开启 AMQP 事务

    // 批量发送
    BatchSize    int           // 批量阈值，默认 0 = 不批量
    BatchTimeout time.Duration // flush 间隔，默认 0 = 立即

    // QoS（有序消费建议 prefetch=1）
    Prefetch int
}
```

### RetryPolicy 重试策略

```go
// 阶梯退避策略（优先使用 Levels 指定）
type RetryPolicy struct {
    MaxRetries int             // 最大重试次数，0 = 默认 3
    Levels     []time.Duration // 阶梯延迟，如 []time.Duration{15s, 30s, 45s}
    Base       time.Duration   // 指数退避基础（Levels 为空时使用）
}

// 默认阶梯延迟
var DefaultRetryDelays = []time.Duration{
    15 * time.Second,
    30 * time.Second,
    45 * time.Second,
    60 * time.Second,
    75 * time.Second,
}
```

### IdempCache 幂等接口

```go
type IdempCache interface {
    IsProcessed(key string) bool   // 检查是否已处理
    MarkProcessed(key string, ttl time.Duration)
}
```

提供两种实现：
- `NewInMemoryIdempCache(ttl time.Duration)`：内存实现，进程重启后失效
- `NewRedisIdempCache(client *redis.Client)`：Redis 分布式实现（TODO）

---

## 完整使用示例

### 延迟投递 + 幂等 + 阶梯重试

```go
producer, _ := rabbitmq.NewProducer(rabbitmq.RabbitMQConfig{
    URL:           "amqp://guest:guest@localhost:5672/",
    Exchange:      "order.events",
    DelayExchange: "order.delay", // x-delayed-message 类型
})
defer producer.Close()

// 发送延迟消息（5 分钟后送达）
producer.Publish(ctx, &mq.Message{
    Topic:    "order.timeout",
    Payload:  []byte(`{"order_id": "12345"}`),
    Key:      []byte("order:12345"), // 幂等键
    Delay:    5 * time.Minute,
})
```

### Consumer：幂等 + 重试 + DLQ

```go
consumer, _ := rabbitmq.NewConsumer(rabbitmq.RabbitMQConfig{
    URL:           "amqp://guest:guest@localhost:5672/",
    Exchange:      "order.events",
    Queue:         "order-processor",
    AutoAck:       false,
    DelayExchange: "order.delay",
    DLQExchange:   "order.dlq",
    DLQQueue:      "order.dlq",
    IdempCache:    rabbitmq.NewInMemoryIdempCache(24 * time.Hour),
    RetryPolicy: &RetryPolicy{
        MaxRetries: 5,
        Levels:     []time.Duration{15, 30, 45, 60, 75} * time.Second,
    },
})
defer consumer.Close()

consumer.Consume(func(ctx context.Context, msg *mq.Message) error {
    // 幂等键相同则自动跳过（IdempCache 已处理）
    return processOrder(ctx, msg)
})
```

### 事务模式批量发送

```go
producer, _ := rabbitmq.NewProducer(rabbitmq.RabbitMQConfig{
    URL:           "amqp://guest:guest@localhost:5672/",
    Exchange:      "payment.events",
    EnableTx:      true,
    BatchSize:     100,
    BatchTimeout:  500 * time.Millisecond,
})
defer producer.Close()

// 批量发送，500ms 或累计 100 条后自动 flush
for _, event := range events {
    producer.Publish(ctx, &mq.Message{
        Topic:   "payment.processed",
        Payload: event,
    })
}
producer.Flush(ctx) // 阻塞等待所有消息确认
```

---

## RabbitMQ 插件安装

延迟投递需要安装延迟插件：

```bash
# 启用延迟消息交换机插件
rabbitmq-plugins enable rabbitmq_delayed_message_exchange

# 验证插件状态
rabbitmq-plugins list | grep delayed
```

---

## RocketMQ 能力清单

### 能力矩阵

| 能力 | 标志 | 实现状态 | 说明 |
|------|------|----------|------|
| 任意延迟投递 | `CapArbitraryDelay` | ✅ 已实现 | v5 原生 `SetDelayTimestamp()`，无需插件 |
| 幂等去重 | `CapIdempotency` | ⚠️ 接口完整 | `IdempCache` 接口已实现；`RedisIdempCache` 为 TODO |
| 阶梯重试 | `CapRetry` | ✅ 已实现 | `RetryPolicy.Levels` + `NextDelay()`，消费者自动 republish |
| 死信队列 | `CapDLQ` | ✅ 已实现 | 配置 `DLQTopic`，失败消息转发 |
| 事务 | `CapTx` | ⚠️ 接口预留 | `EnableTx: true` 预留，TransactionProducer 为 TODO |
| 批量发送 | `CapBatch` | ✅ 已实现 | 客户端缓冲 + 后台 flusher |
| 有序投递 | `CapOrdered` | ✅ 已实现 | `MessageGroup` 分区有序投递 |
| 多消费组 | `CapMultiGroup` | ✅ 已实现 | `ConsumerGroup` 配置 |
| 优先级队列 | `CapPriority` | ❌ 不支持 | RocketMQ 不支持原生优先级 |

### RocketMQConfig 完整配置

```go
type RocketMQConfig struct {
    // 连接
    Endpoint      string  // NameServer 地址，如 "localhost:8081"
    Topic         string  // 默认 topic（Producer 预热路由需要）

    // 认证
    AccessKey     string  // ACL AccessKey
    SecretKey     string  // ACL SecretKey
    NameSpace     string  // 多租户命名空间

    // TLS
    EnableSSL     bool    // 启用 TLS，默认 false

    // 消费者组
    ConsumerGroup string  // 消费者组名

    // 延迟投递（v5 原生，无需插件）
    EnableDelay   bool    // 启用延迟投递，默认 true

    // 幂等去重（内存实现，生产建议换 RedisIdempCache）
    IdempCache    IdempCache  // nil = 不开启

    // 重试策略（阶梯退避）
    RetryPolicy   *RetryPolicy  // nil = 默认 3 次重试

    // 死信队列（消费端失败消息转发 topic）
    DLQTopic      string  // 空 = 不开启 DLQ

    // 事务消息（TODO）
    EnableTx      bool    // true = 使用 TransactionProducer（TODO）

    // 批量发送
    BatchSize     int           // 批量缓冲大小，0 = 不批量
    BatchTimeout  time.Duration // flush 间隔

    // 有序投递（同一 MessageGroup 内有序）
    MessageGroup  string  // 设置后同一 group 消息有序

    // 消费配置
    ReceiveBatchSize   int32          // Receive 批量大小，默认 16
    InvisibleDuration  time.Duration  // 可见性超时，默认 30s（需 > 20s）
    MaxAttempts        int32          // 生产者重试次数，默认 3
}
```

### RocketMQ 完整使用示例

#### 延迟投递（v5 原生，无需插件）

```go
producer, _ := rocketmq.NewProducer(rocketmq.RocketMQConfig{
    Endpoint:  "localhost:8081",
    Topic:     "order.events",
    AccessKey: "ak",
    SecretKey: "sk",
    EnableSSL: false,
})
defer producer.Close()

// 发送延迟消息（5 分钟后送达）
producer.Publish(ctx, &mq.Message{
    Topic:   "order.timeout",
    Payload: []byte(`{"order_id": "12345"}`),
    Key:     []byte("order:12345"),
    Delay:   5 * time.Minute,
})
```

#### Consumer：幂等 + 重试 + DLQ

```go
consumer, _ := rocketmq.NewConsumer(rocketmq.RocketMQConsumerConfig{
    Endpoint:       "localhost:8081",
    Topic:          "order.events",
    ConsumerGroup:  "order-processor",
    AccessKey:      "ak",
    SecretKey:      "sk",
    EnableSSL:      false,
    IdempCache:     mq.NewInMemoryIdempCache(24 * time.Hour),
    RetryPolicy: &mq.RetryPolicy{
        MaxRetries: 5,
        Levels:     []time.Duration{15, 30, 45, 60, 75} * time.Second,
    },
    DLQTopic: "order.dlq",
})
defer consumer.Close()

consumer.Subscribe(ctx, []string{"order.events"}, "order-processor",
    func(ctx context.Context, msg *mq.Message) error {
        return processOrder(ctx, msg)
    })
```

#### 分区有序投递

```go
producer, _ := rocketmq.NewProducer(rocketmq.RocketMQConfig{
    Endpoint:     "localhost:8081",
    Topic:        "payment",
    AccessKey:    "ak",
    SecretKey:    "sk",
    MessageGroup: "payment-sequence-001", // 同一 group 内有序
})
defer producer.Close()

// 同一 MessageGroup 的消息按发送顺序被同一 consumer 消费
producer.Publish(ctx, &mq.Message{
    Topic:   "payment",
    Payload: []byte(`{"step": 1}`),
})
```

---

## 能力探测

运行时查询后端能力，避免使用不支持的功能：

```go
caps := producer.Capabilities()
if caps.Has(mq.CapArbitraryDelay) {
    // 后端支持延迟投递
}
if caps.Has(mq.CapIdempotency) {
    // 后端支持幂等
}
```

所有后端能力标志（`mq/capability.go`）：

| 标志 | 说明 |
|------|------|
| `CapArbitraryDelay` | 任意精确延迟 |
| `CapIdempotency` | 幂等去重 |
| `CapPriority` | 优先级队列 |
| `CapOrdered` | 有序投递 |
| `CapDLQ` | 死信队列 |
| `CapRetry` | 阶梯重试 |
| `CapMultiGroup` | 多消费组 |
| `CapTx` | 事务 |
| `CapBatch` | 批量发送 |

---

## 注意事项

- Producer 和 Consumer 都必须调用 `Close()` 关闭连接
- 生产环境使用 `AutoAck: false` 并手动 ack
- Consumer 处理失败时根据 RetryPolicy 重试，超过最大次数则发往 DLQ
- 延迟投递：RabbitMQ 需要 `rabbitmq-delayed-message-exchange` 插件；RocketMQ v5 原生支持，无需插件
- 幂等去重：当前只有内存实现，生产环境需实现 `RedisIdempCache`
- RocketMQ 事务消息（`EnableTx`）接口已预留，TransactionProducer 为 TODO
- RocketMQ 不支持优先级队列
- 内存 Broker（`mq/memory`）仅适用于本地开发/测试，无持久化、无分布式保证
