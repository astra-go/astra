# MQ Adapters Capabilities Matrix

This document provides a comprehensive overview of all MQ adapter capabilities in the astra framework.

## Overview

The astra MQ module provides 8 message queue adapters with varying levels of feature support. Each adapter implements the `Producer` and `Consumer` interfaces with capabilities declared via the `Capabilities()` method.

---

## Capability Definitions

| Capability | Constant | Description |
|------------|----------|-------------|
| **Arbitrary Delay** | `CapArbitraryDelay` | Support for arbitrary-delay message delivery (not just fixed delay levels) |
| **Fixed Delay** | `CapFixedDelay` | Support for fixed-level delay (e.g., RocketMQ v4's 18 delay levels) |
| **NAK Delay** | `CapNakDelay` | Support for NAK + configurable delay (re-delivery after failure) |
| **Idempotency** | `CapIdempotency` | Native idempotent delivery (deduplication) |
| **Priority** | `CapPriority` | Support for priority queues |
| **Ordered** | `CapOrdered` | Guaranteed in-order delivery within a partition/queue |
| **DLQ** | `CapDLQ` | Native dead-letter queue support |
| **Retry** | `CapRetry` | Native retry support with configurable policy |
| **Multi Group** | `CapMultiGroup` | Support for multiple consumer groups |
| **Transaction** | `CapTx` | Support for transactional messages |
| **Batch** | `CapBatch` | Support for batch sending |

---

## Capability Matrix

| Capability | RabbitMQ | RocketMQ | Kafka | Pulsar | NATS | MQTT | Memory | Redis |
|:----------:|:--------:|:--------:|:-----:|:------:|:----:|:----:|:------:|:-----:|
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
| **Score** | **11/11** | **11/11** | **11/11** | **11/11** | **10/11** | **10/11** | **9/11** | **10/11** |

**Legend**: ✅ = Supported, ❌ = Not Supported

---

## Adapter Details

### 1. RabbitMQ (11/11)

**Broker**: RabbitMQ with `rabbitmq_delayed_message_exchange` plugin

**Supported Capabilities**:
- ✅ Arbitrary Delay: via `x-delayed-message` plugin with `x-delay` header
- ✅ Fixed Delay: via per-level delay queues (`x-message-ttl` → `x-dead-letter-exchange` → real topic)
- ✅ NAK Delay: via republish + `x-delay` (semantic equivalent)
- ✅ Idempotency: via `InMemoryIdempCache` or `RedisIdempCache`
- ✅ Priority: via queue `x-max-priority` argument
- ✅ Ordered: single-queue FIFO with single consumer
- ✅ DLQ: via Dead Letter Exchange (DLX)
- ✅ Retry: staircase retry via `RetryPolicy`
- ✅ Multi Group: via multiple queue bindings
- ✅ Transaction: via AMQP `TxSelect()` / `TxCommit()` / `TxRollback()`
- ✅ Batch: client-side aggregation

**Implementation Notes**:
- Requires `rabbitmq_delayed_message_exchange` plugin for arbitrary delay
- Idempotency is client-side (KV cache), not broker-native
- Transaction support is AMQP-level (not distributed transactions)

---

### 2. RocketMQ (11/11)

**Broker**: Apache RocketMQ v5

**Supported Capabilities**:
- ✅ Arbitrary Delay: via `SetDelayTimestamp()` (RocketMQ v5)
- ✅ Fixed Delay: via 18 predefined delay levels (RocketMQ v4 compatible)
- ✅ NAK Delay: via `ChangeInvisibleDuration()` API
- ✅ Idempotency: via client-side `IdempCache` + broker keys
- ✅ Priority: via multi-topic routing + consumer-side `container/heap` sorting
- ✅ Ordered: via `MessageGroup` (sharding key)
- ✅ DLQ: native `%DLQ%` topic
- ✅ Retry: native retry queue with staircase delays
- ✅ Multi Group: via consumer groups
- ✅ Transaction: via `TransactionProducer` with `TransactionChecker`
- ✅ Batch: via `Batch()` API

**Implementation Notes**:
- Priority uses multi-topic pattern (no native priority queues in RocketMQ)
- Transaction messages require `TransactionChecker` callback implementation

---

### 3. Kafka (11/11)

**Broker**: Apache Kafka (KRaft mode recommended)

**Supported Capabilities**:
- ✅ Arbitrary Delay: via republish mechanism
- ✅ Fixed Delay: via per-level delay topics + forwarding consumer (background goroutine required)
- ✅ NAK Delay: via `NakWithDelay()` (pause/resume consumer)
- ✅ Idempotency: via `EnableIdempotent` producer config
- ✅ Priority: via topic partitioning + consumer-side sorting
- ✅ Ordered: via partition ordering guarantee
- ✅ DLQ: via `DLQTopic` config
- ✅ Retry: via `RetryPolicy` with backoff
- ✅ Multi Group: via consumer groups
- ✅ Transaction: via Kafka Transactions API (exactly-once semantics)
- ✅ Batch: via producer batching

**Implementation Notes**:
- Idempotency is broker-native (producer ID + sequence number)
- Transaction support requires `isolation.level=read_committed` for consumers
- Fixed delay publishes to per-level delay topics; background consumer forwards after TTL

---

### 4. Pulsar (11/11)

**Broker**: Apache Pulsar

**Supported Capabilities**:
- ✅ Arbitrary Delay: via `DeliverAfter()` API
- ✅ Fixed Delay: via `DeliverAfter()` (maps to nearest configured delay level)
- ✅ NAK Delay: via republish with delay
- ✅ Idempotency: via `IdempCache` + sequence ID
- ✅ Priority: via message priority metadata
- ✅ Ordered: via partitioned topics + key-based routing
- ✅ DLQ: via `DeadLetterPolicy` config
- ✅ Retry: via `RetryPolicy` with backoff
- ✅ Multi Group: via subscription names
- ✅ Transaction: via Pulsar Transactions API
- ✅ Batch: via producer batching

**Implementation Notes**:
- Arbitrary delay is native via `DeliverAfter()` or `DeliverAt()`
- Idempotency combines client-side cache + broker sequence ID
- Transaction support requires broker configuration

---

### 5. NATS (10/11)

**Broker**: NATS with JetStream enabled

**Supported Capabilities**:
- ✅ Arbitrary Delay: via `ArbitraryDelay()` + client-side re-publisher goroutine
- ✅ Fixed Delay: via `PublishAsync` delay headers + re-publisher goroutine
- ✅ NAK Delay: via `Nak()` with optional delay
- ✅ Idempotency: via JetStream KV bucket deduplication
- ✅ Priority: via multi-subject routing (`topic.p0..pN`) + consumer-side heap
- ✅ Ordered: via `OrderedConsumer()`
- ✅ DLQ: via `MaxDeliver` + `DeliverSubject`
- ✅ Retry: via `MaxDeliver` redelivery
- ✅ Multi Group: via queue groups
- ✅ Batch: client-side aggregation

**Missing Capabilities**:
- ❌ **Transaction** — Neither NATS protocol nor JetStream provide transaction semantics (no Begin/Commit/Rollback); KV buckets can only simulate state tracking without cross-message atomicity guarantees

**Implementation Notes**:
- JetStream must be enabled for persistence and DLQ
- Fixed/Arbitrary delay: requires `StartRePublisher()` / `StartArbitraryDelayPublisher()` goroutine (client-side, not crash-safe)
- Priority: producer routes to `topic.pN`, consumer drains high-priority messages first using internal heap

---

### 6. Redis Streams (10/11)

**Broker**: Redis 6.2+ Streams

**Supported Capabilities**:
- ✅ Arbitrary Delay: via `ZADD` to delay sorted set, `delayPump` goroutine polls and `XADD`s expired messages
- ✅ Fixed Delay: via `RedisConfig.FixedDelayLevels`, `FixedDelay()` maps level index to delay duration
- ✅ NAK Delay: via `XCLAIM` min-idle-time, message is redelivered after configured delay
- ✅ Idempotency: Redis SET stores `msg.IdempKey`, written on success with TTL
- ✅ Priority: via `PriorityStreams` multi-stream routing (`topic.p0/p1/p2`) + consumer priority ordering
- ✅ Ordered: Redis Stream message IDs (`timestamp-sequence`) are monotonically increasing
- ✅ DLQ: on retry exhaustion, `XADD` to configured `DLQStream`
- ✅ Retry: exponential backoff (`retryCount² × base`) or level-based, via `XCLAIM`
- ✅ Multi Group: `XGROUP CREATE` supports multiple consumer groups
- ✅ Batch: Redis pipeline for batch `XADD`

**Missing Capabilities**:
- ❌ **Transaction** — Redis `MULTI/EXEC` is optimistic concurrency control, not a distributed transaction; `WATCH` is unsuitable for MQ use cases

**Configuration Example**:

```go
// Producer
p := mq.NewRedisProducer(mq.RedisProducerConfig{
    Addr:             "localhost:6379",
    Stream:           "orders",
    FixedDelayLevels: []int64{1000, 5000, 10000, 30000, 60000},
    PriorityStreams:  map[int]string{3: "orders:high", 2: "orders:normal", 1: "orders:low"},
})

// Consumer
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

**Implementation Notes**:
- Delay delivery relies on `delayPump` background goroutine (`ZADD` + `ZPOPMIN`); not crash-safe — process restart loses in-flight delay entries
- Recommended for Redis primary-replica mode; primary failure preserves Stream availability but delay sorted set may be lost
- Best for: testing, small-to-medium projects, lightweight async tasks

---

### 7. MQTT (10/11)

**Broker**: EMQX / Mosquitto / NanoMQ (MQTT v5.0)

**Supported Capabilities**:
- ✅ Arbitrary Delay: via `$arb/<delay_ms>/<original_topic>` + consumer timer
- ✅ Fixed Delay: via `$delay/<level>/<original_topic>` topic routing
- ✅ NAK Delay: via retry topic `$retry/<count>/<original_topic>` + NakDelay
- ✅ Idempotency: client-side IdempKey deduplication
- ✅ Ordered: single-topic ordering naturally preserved (QoS 1/2)
- ✅ DLQ: via DLQTopic forwarding after MaxRetries
- ✅ Retry: via MQTT v5.0 Retry flag
- ✅ Multi Group: via shared subscriptions (`$share/group/topic`)
- ✅ Batch: client-side aggregation

**Missing Capabilities**:
- ❌ **Transaction** — MQTT is a lightweight Pub/Sub protocol with no transaction semantics; PUBLISH is atomic per-message but cross-message grouping is unsupported
- ❌ **Priority** — MQTT brokers deliver by subscription without message priority; client-side sorting only affects consumption order, not broker delivery order

**Implementation Notes**:
- Shared subscriptions require MQTT v5.0 broker
- Retry/DLQ use topic encoding: `$retry/<count>/<topic>`, `$delay/<level>/<topic>`, `$arb/<ms>/<topic>`
- Fixed/arbitrary delay uses consumer-side `time.After` timer (not broker-native)

---

### 8. Memory (9/11)

**Broker**: In-process Go channels (testing only)

**Supported Capabilities**:
- ✅ Arbitrary Delay: via `time.Timer` delayed delivery
- ✅ Fixed Delay: via `MemoryBrokerConfig.FixedDelayLevels`, `FixedDelay()` maps level index
- ✅ NAK Delay: via `MemoryConsumerConfig.NakDelay`, handler error triggers delayed redelivery
- ✅ Idempotency: via `IdempKey` deduplication (`MemoryConsumerConfig.Idempotent`)
- ✅ Ordered: FIFO channel buffering
- ✅ DLQ: DLQ channel (`MemoryConsumerConfig.DLQBuffer`)
- ✅ Retry: exponential backoff (`MemoryConsumerConfig.MaxRetries`)
- ✅ Multi Group: named consumer group fan-out
- ✅ Batch: via `PublishBatch` aggregation

**Missing Capabilities**:
- ❌ **Transaction** — Single-process channel model with no cross-message atomicity requirements; distributed transaction semantics are meaningless in a test broker
- ❌ **Priority** — Would require replacing channels with `container/heap`, breaking FIFO semantics; for testing, multi-topic simulation is simpler

**Implementation Notes**:
- **Testing only** — not suitable for production
- No persistence; messages lost on process restart
- Use `NewMemoryConsumerWithBroker` / `NewMemoryProducerWithBroker` to share the same broker instance

---

## Ranking

| Rank | Adapter | Score | Coverage |
|:----:|---------|-------|----------|
| 🥇 | **RabbitMQ** | 11/11 | 100% |
| 🥇 | **RocketMQ** | 11/11 | 100% |
| 🥇 | **Kafka** | 11/11 | 100% |
| 🥇 | **Pulsar** | 11/11 | 100% |
| 5 | **NATS** | 10/11 | 91% |
| 5 | **Redis** | 10/11 | 91% |
| 7 | **MQTT** | 10/11 | 91% |
| 8 | **Memory** | 9/11 | 82% |

---

## Choosing an Adapter

### Production Recommendations

| Scenario | Recommended Adapter | Reason |
|----------|---------------------|--------|
| **Financial/Transaction** | RocketMQ | Full capability coverage (11/11) |
| **High Throughput** | Kafka | Excellent batching + partition scaling |
| **Low Latency** | NATS | Minimal overhead, cloud-native |
| **IoT/Edge** | MQTT | Lightweight protocol, broad device support |
| **Event Streaming** | Pulsar | Tiered storage + geo-replication |
| **Testing/Development** | Memory, Redis | Zero external dependencies |

### Capability-Based Selection

| Required Capability | Supported Adapters |
|---------------------|-------------------|
| **Arbitrary Delay** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **Fixed Delay** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **NAK Delay** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **Idempotency** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **Priority** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, Redis |
| **Ordered** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **DLQ** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **Retry** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |
| **Multi Group** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Redis |
| **Transaction** | RabbitMQ, RocketMQ, Kafka, Pulsar |
| **Batch** | RabbitMQ, RocketMQ, Kafka, Pulsar, NATS, MQTT, Memory, Redis |

---

## Implementation Details

### Capability Detection

```go
// Check if a producer supports arbitrary delay
caps := producer.Capabilities()
if caps.Has(mq.CapArbitraryDelay) {
    // Can use msg.Delay for arbitrary delay
}

// Check multiple capabilities at once
required := []mq.Capability{mq.CapDLQ, mq.CapRetry}
for _, cap := range required {
    if !caps.Has(cap) {
        // Handle missing capability
    }
}
```

### Graceful Degradation

When a capability is not supported, the framework provides fallback behavior:

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

## Version Compatibility

| Adapter | Minimum Broker Version | Go Client Library |
|---------|-----------------------|-------------------|
| RabbitMQ | 3.8+ (delay plugin required) | `github.com/rabbitmq/amqp091-go` |
| RocketMQ | 5.0+ | `github.com/apache/rocketmq-clients/golang` |
| Kafka | 2.8+ (KRaft) | `github.com/IBM/sarama` |
| Pulsar | 2.10+ | `github.com/apache/pulsar-client-go` |
| NATS | 2.9+ (JetStream) | `github.com/nats-io/nats.go` |
| MQTT | 5.0+ | `github.com/eclipse/paho.mqtt.golang` |
| Redis | 6.2+ | `github.com/redis/go-redis/v9` |
| Memory | N/A | N/A (in-process) |

---

## Future Enhancements

| Proposed Capability | Description |
|---------------------|-------------|
| `CapCompression` | Native message compression support |
| `CapEncryption` | End-to-end encryption support |
| `CapFiltering` | Server-side message filtering |
| `CapTTL` | Message TTL configuration |
| `CapReplication` | Geo-replication support |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| v1.0.5 | 2026-06-24 | MQ module initial release |

---

## References

- [RabbitMQ Delayed Message Plugin](https://github.com/rabbitmq/rabbitmq-delayed-message-exchange)
- [RocketMQ v5 Documentation](https://rocketmq.apache.org/docs/)
- [Kafka KIP-447](https://cwiki.apache.org/confluence/display/KAFKA/KIP-447%3A+Producer+scalability+for+exactly+once+semantics)
- [Pulsar Transactions](https://pulsar.apache.org/docs/next/txn-why/)
- [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream)
- [MQTT v5.0 Specification](https://mqtt.org/mqtt-specification/)
- [Redis Streams](https://redis.io/docs/data-types/streams/)

---

## Task Queue (`astra/taskqueue`)

A typed dispatch layer built on top of `mq.Consumer`. Adds application-level task semantics to any broker adapter.

### Features

| Feature | Description |
|---------|-------------|
| **Type-based routing** | Messages are dispatched to typed `TaskHandler` functions by `TaskType` |
| **Exponential backoff** | Configurable base delay × multiplier with jitter and max cap |
| **DLQ envelope** | `DLQPayload` records full task context (attempts, error, timestamps) on final failure |
| **Task store interface** | `TaskStore` interface allows pluggable persistence (Postgres, Redis) |
| **Broker agnostic** | Works with any `mq.Consumer` (NATS, RabbitMQ, Kafka, RocketMQ, etc.) |

### Usage

```go
import "github.com/astra-go/astra/taskqueue"

// 1. Define handlers
r := taskqueue.NewRouter(nil)
r.Register("send_email", func(ctx context.Context, data json.RawMessage) error {
    var email EmailPayload
    json.Unmarshal(data, &email)
    return smtp.Send(email)
})

// 2. Wrap the mq.Consumer with a Dispatcher
d := taskqueue.NewDispatcher(natsConsumer, r,
    taskqueue.WithRetryPolicy(taskqueue.DefaultRetryPolicy),
)

// 3. Start (blocks)
d.Start(ctx)
```

### RetryPolicy defaults

| Field | Default | Description |
|-------|---------|-------------|
| `MaxRetries` | 3 | Attempts after initial delivery |
| `BackoffBase` | 5s | Base delay |
| `BackoffMultiplier` | 3 | Multiply delay per attempt |
| `BackoffMax` | 300s | Hard cap on delay |
| `BackoffJitter` | true | ±25% randomization |

### Compared to raw `mq.Consumer` retry

| Aspect | `mq.Consumer` (MaxDeliver) | `taskqueue.RetryPolicy` |
|--------|---------------------------|------------------------|
| Retries | Count only | Count + exponential delay |
| Backoff | None (fixed interval) | Configurable base × multiplier |
| Max cap | None | `BackoffMax` |
| Jitter | None | ±25% optional |
| DLQ envelope | Raw message | `DLQPayload` with full context |
| Task routing | Topic-based | `TaskType`-based |

