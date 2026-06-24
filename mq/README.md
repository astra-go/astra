# MQ — Message Queue Abstraction

Unified Producer/Consumer interface supporting multiple message queue backends, decoupling business code from specific Brokers.

---

## Features

- **Unified Interface**: `Producer` and `Consumer` interfaces are broker-agnostic
- **8 Backends**: RabbitMQ, Kafka, RocketMQ, MQTT, NATS, Pulsar, Redis, In-Memory
- **11 Capabilities**: Arbitrary delay, fixed delay, NAK delay, idempotency, priority, ordered, DLQ, retry, multi-group, transaction, batch
- **Runtime Capability Detection**: Query backend capabilities at runtime, graceful degradation
- **Multi-Backend Compensation**: Redis-based delay and idempotency layer works across all backends

---

## Supported Backends

| # | Broker | Import Path | Score | Highlights |
|---|--------|-------------|-------|------------|
| 1 | RabbitMQ | `github.com/astra-go/astra/mq/rabbitmq` | 11/11 | Full coverage: delay/idempotency/tx/batch/priority/DLQ/ordered |
| 2 | RocketMQ | `github.com/astra-go/astra/mq/rocketmq` | 11/11 | Full coverage: delay/idempotency/tx/batch/ordered/DLQ |
| 3 | Kafka | `github.com/astra-go/astra/mq/kafka` | 11/11 | Full coverage: delay/idempotency/tx/batch/priority/ordered |
| 4 | Pulsar | `github.com/astra-go/astra/mq/pulsar` | 11/11 | Full coverage: delay/idempotency/tx/batch/multi-tenant |
| 5 | NATS | `github.com/astra-go/astra/mq/nats` | 10/11 | Lightweight, JetStream persistence, missing transactions |
| 6 | Redis | `github.com/astra-go/astra/mq/redis` | 10/11 | Streams-based, no external dependency, missing transactions |
| 7 | MQTT | `github.com/astra-go/astra/mq/mqtt` | 10/11 | IoT-optimized, shared subscriptions, missing priority/transactions |
| 8 | Memory | `github.com/astra-go/astra/mq/memory` | 9/11 | Testing only, zero external dependency |

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
    AutoAck: false, // Manual ack (recommended for production)
})
defer consumer.Close()

consumer.Consume(func(ctx context.Context, msg *mq.Message) error {
    fmt.Printf("Received: %s\n", string(msg.Payload))
    return nil // nil = ACK, error = NACK + retry
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
```

### Message Struct

```go
type Message struct {
    Topic      string            // Exchange/routing key
    Payload    []byte            // Message body
    Key        []byte            // Message ID (used for idempotency)

    // Delivery control
    Delay      time.Duration     // Delayed delivery (backend-dependent support)
    IdempKey   string            // Idempotency key (skip if already processed)
    Priority   uint8             // Message priority (0 = lowest, backend-dependent)
    RetryCount int               // Current retry count (set by consumer on retry)

    // Delivery result metadata (set by consumer)
    Headers    map[string]string // Custom headers
}
```

---

## Capability Definitions

| Capability | Flag | Description |
|------------|------|-------------|
| **Arbitrary Delay** | `CapArbitraryDelay` | Support for arbitrary precise delay (not limited to fixed levels) |
| **Fixed Delay** | `CapFixedDelay` | Support for fixed-level delay (e.g., 18 delay levels in RocketMQ v4) |
| **NAK Delay** | `CapNakDelay` | Support for NAK + configurable delay before redelivery |
| **Idempotency** | `CapIdempotency` | Native idempotent deduplication |
| **Priority** | `CapPriority` | Support for priority queues |
| **Ordered** | `CapOrdered` | Guaranteed in-order delivery within a partition/queue |
| **DLQ** | `CapDLQ` | Native dead-letter queue support |
| **Retry** | `CapRetry` | Native retry with configurable policy |
| **Multi Group** | `CapMultiGroup` | Support for multiple consumer groups |
| **Transaction** | `CapTx` | Support for transactional messages |
| **Batch** | `CapBatch` | Support for batch sending |

---

## Capability Matrix

| Capability | RabbitMQ | RocketMQ | Kafka | Pulsar | NATS | MQTT | Memory | Redis |
|:----------:|:--------:|:--------:|:-----:|:------:|:----:|:----:|:------:|:-----:|
| Arbitrary Delay | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Fixed Delay | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| NAK Delay | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Idempotency | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Priority | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ |
| Ordered | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| DLQ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Retry | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Multi Group | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ |
| Transaction | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Batch | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Score** | **11/11** | **11/11** | **11/11** | **11/11** | **10/11** | **10/11** | **9/11** | **10/11** |

---

## Per-Backend Quick Reference

### RabbitMQ (`mq/rabbitmq`)

- **Requires**: `rabbitmq_delayed_message_exchange` plugin for arbitrary delay
- **Priority**: via queue `x-max-priority` declaration
- **Transaction**: AMQP `TxSelect()`/`TxCommit()`/`TxRollback()`
- **Idempotency**: `InMemoryIdempCache` (in-process) or `RedisIdempCache` (distributed)

### RocketMQ (`mq/rocketmq`)

- **Requires**: RocketMQ v5 (v4 supports fixed delay only)
- **Arbitrary delay**: native `SetDelayTimestamp()`, no plugin needed
- **Transaction**: `TransactionProducer` + `TransactionChecker` callback
- **Ordered**: via `MessageGroup` (sharding key)

### Kafka (`mq/kafka`)

- **Requires**: Kafka 2.8+ (KRaft mode recommended)
- **Arbitrary delay**: republish mechanism (consumer-side timer)
- **Idempotency**: broker-native via `EnableIdempotent` (producer ID + sequence number)
- **Transaction**: Kafka Transactions API, requires `isolation.level=read_committed`

### Pulsar (`mq/pulsar`)

- **Requires**: Pulsar 2.10+
- **Arbitrary delay**: native `DeliverAfter()` / `DeliverAt()`
- **Transaction**: Pulsar Transactions API
- **Multi-tenant**: built-in namespace isolation

### NATS (`mq/nats`)

- **Requires**: NATS 2.9+ with JetStream enabled
- **Delay**: `PublishAsync` + re-publisher goroutine (client-side, not crash-safe)
- **Priority**: multi-subject routing (`topic.p0..pN`) + consumer-side heap
- **Limitation**: no transaction support

### Redis (`mq/redis`)

- **Requires**: Redis 6.2+ Streams
- **Delay**: `ZADD` to delay sorted set + `delayPump` background goroutine (not crash-safe)
- **Priority**: multi-stream routing via `PriorityStreams` config
- **Limitation**: `MULTI/EXEC` is not a distributed transaction

### MQTT (`mq/mqtt`)

- **Requires**: MQTT v5.0 broker
- **Delay**: topic encoding (`$arb/<ms>/<topic>`, `$delay/<level>/<topic>`) + consumer timer
- **Multi Group**: shared subscriptions (`$share/group/topic`)
- **Limitation**: no native priority, no transaction support

### Memory (`mq/memory`)

- **No external dependency**
- **Best for**: unit tests, local development
- **Limitation**: no persistence, no multi-group, no transaction, no priority

---

## IdempCache Interface

Idempotency is implemented via the `IdempCache` interface:

```go
type IdempCache interface {
    IsProcessed(key string) bool
    MarkProcessed(key string, ttl time.Duration)
}
```

Two built-in implementations:
- `NewInMemoryIdempCache(ttl time.Duration)` — in-process, lost on restart
- `NewRedisIdempCache(client *redis.Client)` — distributed, production-ready

---

## RetryPolicy

Staircase backoff with configurable levels:

```go
type RetryPolicy struct {
    MaxRetries int             // Max retries; 0 = default 3
    Levels     []time.Duration // Staircase delays, e.g. {15s, 30s, 45s, 60s, 75s}
    Base       time.Duration   // Exponential base (used when Levels is empty)
}
```

Exponential backoff when `Levels` is empty: `delay = retryCount² × Base`.

---

## Capability Detection

Query capabilities at runtime to avoid unsupported features:

```go
caps := producer.Capabilities()

// Check single capability
if caps.Has(mq.CapArbitraryDelay) {
    // Backend supports delayed delivery
}

// Check multiple capabilities
required := []mq.Capability{mq.CapDLQ, mq.CapRetry}
for _, cap := range required {
    if !caps.Has(cap) {
        log.Printf("backend missing: %s", cap)
    }
}
```

---

## Graceful Degradation

When a capability is not supported, the framework falls back:

| Missing Capability | Fallback Behavior |
|--------------------|-------------------|
| `CapArbitraryDelay` | Immediate delivery (no delay) |
| `CapFixedDelay` | Use `CapArbitraryDelay` if available |
| `CapNakDelay` | Immediate NACK (no delay before redelivery) |
| `CapIdempotency` | Application must handle duplicates |
| `CapPriority` | FIFO delivery (no priority ordering) |
| `CapOrdered` | No ordering guarantee |
| `CapDLQ` | Messages dropped after max retries |
| `CapRetry` | No automatic retry |
| `CapTx` | Non-transactional publish |
| `CapBatch` | Sequential publish |

---

## Notes

- Both producer and consumer must call `Close()` to release connections
- Production environments should use `AutoAck: false` with manual ack
- Consumer errors trigger retry per `RetryPolicy`; exceeding `MaxRetries` forwards to DLQ
- RabbitMQ arbitrary delay requires `rabbitmq_delayed_message_exchange` plugin
- RocketMQ v5 has native arbitrary delay support (no plugin needed)
- Memory broker (`mq/memory`) is for testing only — no persistence, no distributed guarantees
