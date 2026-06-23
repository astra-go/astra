# MQ — Message Queue Abstraction

Unified Producer/Consumer interface supporting multiple message queue backends, decoupling business code from specific Brokers.

## Features

- **Unified Interface**: Producer and Consumer interfaces are broker-agnostic
- **Multi-Backend**: RabbitMQ, Kafka, RocketMQ, MQTT, NATS, Pulsar, In-Memory Broker (dev/test)
- **Capability Detection**: Runtime probe of backend capabilities (delay, idempotency, retry, DLQ, etc.)
- **Consumer Groups**: Multi-consumer load-balanced consumption
- **Message Retry**: Staircase retry or dead-letter queue for failed messages
- **Redis Compensation Layer**: Cross-backend unified delay and idempotency implementation

## Supported Backends

| Broker | Import Path | Highlights |
|--------|-------------|------------|
| RabbitMQ | `github.com/astra-go/astra/mq/rabbitmq` | Most feature-rich: delay/idempotency/tx/batch/priority/DLQ |
| Apache Kafka | `github.com/astra-go/astra/mq/kafka` | High throughput, event-driven |
| Apache RocketMQ | `github.com/astra-go/astra/mq/rocketmq` | Feature-aligned with RabbitMQ: delay/idempotency/retry/DLQ/tx/batch/ordered |
| MQTT | `github.com/astra-go/astra/mq/mqtt` | IoT scenarios |
| NATS | `github.com/astra-go/astra/mq/nats` | Lightweight, JetStream for persistence |
| Apache Pulsar | `github.com/astra-go/astra/mq/pulsar` | Multi-tenant, tiered storage |
| In-Memory | `github.com/astra-go/astra/mq/memory` | Local dev/testing, no persistence |

---

## Quick Start

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
    AutoAck: false, // Manual ack
})
defer consumer.Close()

consumer.Consume(func(ctx context.Context, msg *mq.Message) error {
    fmt.Printf("Received: %s\n", string(msg.Payload))
    return nil // nil = ACK, error = NACK and may retry
})
```

---

## Core Interfaces

### Producer Interface

```go
type Producer interface {
    Publish(ctx context.Context, msg *Message) error
    Close() error
    Capabilities() Capabilities  // Runtime capability probe
}
```

### Consumer Interface

```go
type Consumer interface {
    Consume(ctx context.Context, handler MessageHandler) error
    Close() error
    Capabilities() Capabilities
}

type MessageHandler func(ctx context.Context, msg *Message) error
```

### Message Struct (Enhanced)

```go
type Message struct {
    Topic      string            // Exchange/routing key
    Payload    []byte            // Message body
    Key        []byte            // Message ID (used for idempotency)
    Headers    map[string]string // Custom headers

    // Enhanced fields
    Delay      time.Duration     // Delayed delivery (x-delay header)
    IdempKey   string            // Idempotency key (skip if already processed)
    RetryCount int               // Current retry count (written by consumer)
    Priority   uint8             // AMQP priority (0–9)
}
```

---

## RabbitMQ Capabilities

### Capability Matrix

| Capability | Flag | Status | Note |
|------------|------|--------|------|
| Arbitrary Delay | `CapArbitraryDelay` | ✅ Plugin required | Needs `rabbitmq-delayed-message-exchange` |
| Idempotency | `CapIdempotency` | ⚠️ Interface complete | `IdempCache` implemented; `RedisIdempCache` is TODO |
| Staircase Retry | `CapRetry` | ✅ Implemented | `RetryPolicy.Levels` + `NextDelay()`, auto republish |
| Dead-Letter Queue | `CapDLQ` | ✅ Implemented | Config `DLQExchange` + `DLQQueue`, auto-forward failed messages |
| Transactions | `CapTx` | ✅ Implemented | `EnableTx: true`, `Tx()` / `TxCommit()` / `TxRollback()` |
| Batching | `CapBatch` | ✅ Implemented | `BatchSize` + background `batchFlusher` goroutine |
| Priority Queues | `CapPriority` | ✅ Implemented | `Priority` field; queue needs `x-max-priority` declaration |
| Multi Consumer Group | `CapMultiGroup` | ✅ Implemented | vhost + independent queue binding |
| Ordered Delivery | `CapOrdered` | ❌ Not implemented | Declared true but no enforcement (recommend QoS prefetch=1) |

### RabbitMQConfig Full Reference

```go
type RabbitMQConfig struct {
    // Connection
    URL string  // amqp://user:pass@host:5672/

    // Exchange
    Exchange  string  // Default exchange name
    ExType   string  // Type: direct/topic/fanout/headers/x-delayed-message

    // Delayed delivery (requires rabbitmq_delayed_message_exchange plugin)
    DelayExchange string  // Delay exchange name (e.g. "my-delay-exchange")

    // Queue
    Queue    string  // Queue name
    Durable  bool    // Durable, default true

    // Manual ack
    AutoAck  bool    // Default false (recommended for production)

    // Dead-letter queue
    DLQExchange string  // DLX exchange name
    DLQQueue    string  // DLQ name

    // Idempotency (memory impl; use RedisIdempCache in production)
    IdempCache IdempCache  // nil = disabled

    // Retry policy (staircase backoff)
    RetryPolicy *RetryPolicy  // nil = default 3 retries

    // Transaction mode
    EnableTx bool  // true = AMQP transaction per message

    // Batching
    BatchSize    int           // Flush threshold, 0 = disabled
    BatchTimeout time.Duration // Flush interval, 0 = immediate

    // QoS (prefetch=1 recommended for ordered delivery)
    Prefetch int
}
```

### RetryPolicy

```go
// Staircase backoff; Levels take priority over exponential Base.
type RetryPolicy struct {
    MaxRetries int             // Max retries; 0 = default 3
    Levels     []time.Duration // Staircase delays, e.g. []time.Duration{15s, 30s, 45s}
    Base       time.Duration   // Exponential base (used when Levels is empty)
}

// Default staircase delays
var DefaultRetryDelays = []time.Duration{
    15 * time.Second,
    30 * time.Second,
    45 * time.Second,
    60 * time.Second,
    75 * time.Second,
}
```

### IdempCache Interface

```go
type IdempCache interface {
    IsProcessed(key string) bool
    MarkProcessed(key string, ttl time.Duration)
}
```

Two implementations available:
- `NewInMemoryIdempCache(ttl time.Duration)`: In-memory, lost on restart
- `NewRedisIdempCache(client *redis.Client)`: Distributed (TODO)

---

## Complete Examples

### Delayed Delivery + Idempotency + Staircase Retry

```go
producer, _ := rabbitmq.NewProducer(rabbitmq.RabbitMQConfig{
    URL:           "amqp://guest:guest@localhost:5672/",
    Exchange:      "order.events",
    DelayExchange: "order.delay", // x-delayed-message type
})
defer producer.Close()

// Send a delayed message (delivered in 5 minutes)
producer.Publish(ctx, &mq.Message{
    Topic:    "order.timeout",
    Payload:  []byte(`{"order_id": "12345"}`),
    Key:      []byte("order:12345"), // Idempotency key
    Delay:    5 * time.Minute,
})
```

### Consumer: Idempotency + Retry + DLQ

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
    // Duplicate messages with same IdempKey are auto-skipped
    return processOrder(ctx, msg)
})
```

### Transactional Batched Publishing

```go
producer, _ := rabbitmq.NewProducer(rabbitmq.RabbitMQConfig{
    URL:           "amqp://guest:guest@localhost:5672/",
    Exchange:      "payment.events",
    EnableTx:      true,
    BatchSize:     100,
    BatchTimeout:  500 * time.Millisecond,
})
defer producer.Close()

for _, event := range events {
    producer.Publish(ctx, &mq.Message{
        Topic:   "payment.processed",
        Payload: event,
    })
}
producer.Flush(ctx) // Block until all messages are committed
```

---

## RabbitMQ Plugin Installation

Delayed delivery requires the delayed-message plugin:

```bash
# Enable the delayed-message exchange plugin
rabbitmq-plugins enable rabbitmq_delayed_message_exchange

# Verify
rabbitmq-plugins list | grep delayed
```

---

## RocketMQ Capabilities

### Capability Matrix

| Capability | Flag | Status | Note |
|------------|------|--------|------|
| Arbitrary Delay | `CapArbitraryDelay` | ✅ Implemented | v5 native `SetDelayTimestamp()`, no plugin needed |
| Idempotency | `CapIdempotency` | ⚠️ Interface complete | `IdempCache` implemented; `RedisIdempCache` is TODO |
| Staircase Retry | `CapRetry` | ✅ Implemented | `RetryPolicy.Levels` + `NextDelay()`, auto republish |
| Dead-Letter Queue | `CapDLQ` | ✅ Implemented | Configure `DLQTopic`, failed messages forwarded |
| Transactions | `CapTx` | ⚠️ Reserved | `EnableTx: true` reserved; TransactionProducer is TODO |
| Batching | `CapBatch` | ✅ Implemented | Client-side buffering + background flusher |
| Ordered Delivery | `CapOrdered` | ✅ Implemented | `MessageGroup` for per-group FIFO |
| Multi Consumer Group | `CapMultiGroup` | ✅ Implemented | `ConsumerGroup` config |
| Priority Queues | `CapPriority` | ❌ Not supported | RocketMQ does not support native priority |

### RocketMQConfig Full Reference

```go
type RocketMQConfig struct {
    // Connection
    Endpoint      string  // NameServer address, e.g. "localhost:8081"
    Topic         string  // Default topic (required for Producer routing)

    // ACL Authentication
    AccessKey     string
    SecretKey     string
    NameSpace     string  // Multi-tenant namespace

    // TLS
    EnableSSL     bool    // Default false

    // Consumer Group
    ConsumerGroup string

    // Delayed Delivery (v5 native, no plugin required)
    EnableDelay   bool    // Default true

    // Idempotency (memory impl; use RedisIdempCache in production)
    IdempCache    IdempCache  // nil = disabled

    // Retry Policy (staircase backoff)
    RetryPolicy   *RetryPolicy  // nil = default 3 retries

    // Dead-Letter Queue (consumer-side failed message topic)
    DLQTopic      string  // empty = disabled

    // Transactional Messages (TODO)
    EnableTx      bool    // true = use TransactionProducer (TODO)

    // Batching
    BatchSize     int           // Flush threshold, 0 = disabled
    BatchTimeout  time.Duration // Flush interval

    // Ordered Delivery (messages within same MessageGroup are ordered)
    MessageGroup  string

    // Consumer Options
    ReceiveBatchSize   int32          // Default 16
    InvisibleDuration  time.Duration  // Default 30s (must be > 20s)
    MaxAttempts        int32          // Producer retries, default 3
}
```

### RocketMQ Complete Examples

#### Delayed Delivery (v5 native, no plugin required)

```go
producer, _ := rocketmq.NewProducer(rocketmq.RocketMQConfig{
    Endpoint:  "localhost:8081",
    Topic:     "order.events",
    AccessKey: "ak",
    SecretKey: "sk",
    EnableSSL: false,
})
defer producer.Close()

producer.Publish(ctx, &mq.Message{
    Topic:   "order.timeout",
    Payload: []byte(`{"order_id": "12345"}`),
    Key:     []byte("order:12345"),
    Delay:   5 * time.Minute,
})
```

#### Consumer: Idempotency + Retry + DLQ

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

#### Ordered Delivery by MessageGroup

```go
producer, _ := rocketmq.NewProducer(rocketmq.RocketMQConfig{
    Endpoint:     "localhost:8081",
    Topic:        "payment",
    AccessKey:    "ak",
    SecretKey:    "sk",
    MessageGroup: "payment-sequence-001", // FIFO within this group
})
defer producer.Close()

// Messages in the same MessageGroup are consumed in send order by the same consumer
producer.Publish(ctx, &mq.Message{
    Topic:   "payment",
    Payload: []byte(`{"step": 1}`),
})
```

---

## Capability Detection

Probe capabilities at runtime to avoid unsupported features:

```go
caps := producer.Capabilities()
if caps.Has(mq.CapArbitraryDelay) {
    // Backend supports delayed delivery
}
if caps.Has(mq.CapIdempotency) {
    // Backend supports idempotent deduplication
}
```

All capability flags (`mq/capability.go`):

| Flag | Description |
|------|-------------|
| `CapArbitraryDelay` | Arbitrary precise delay |
| `CapIdempotency` | Idempotent deduplication |
| `CapPriority` | Priority queues |
| `CapOrdered` | Ordered delivery |
| `CapDLQ` | Dead-letter queue |
| `CapRetry` | Staircase retry |
| `CapMultiGroup` | Multi consumer group |
| `CapTx` | Transactions |
| `CapBatch` | Batched publishing |

---

## Notes

- Both producer and consumer must call `Close()` to release connections
- Production environments should use `AutoAck: false` with manual ack
- Failed consumer handling triggers retry per `RetryPolicy`; exceeding `MaxRetries` forwards to DLQ
- Delayed delivery: RabbitMQ requires `rabbitmq-delayed-message-exchange` plugin; RocketMQ v5 has native support, no plugin needed
- Idempotency currently has only in-memory implementation; production needs `RedisIdempCache`
- RocketMQ transactional messages (`EnableTx`) interface is reserved; TransactionProducer is TODO
- RocketMQ does not support native priority queues
- In-Memory Broker (`mq/memory`) is for local dev/testing only—no persistence, no distributed guarantees
