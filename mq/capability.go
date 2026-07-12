// Package mq - capability detection for backend adapters.
//
// Each backend has different capabilities (arbitrary delay, idempotency,
// priority queue, etc.). The Capability type and Capabilities map
// allow the framework to detect and compensate for missing capabilities.
package mq

// Capability represents a feature that a backend may or may not support.
type Capability int

const (
	// CapArbitraryDelay means the backend supports arbitrary-delay
	// message delivery (not just fixed delay levels).
	// Supported by: RocketMQ v5 (SetDelayTimestamp), Pulsar (DeliverAfter).
	CapArbitraryDelay Capability = 1 << iota

	// CapFixedDelay means the backend supports fixed-level delay
	// (e.g. RocketMQ v4 has 18 fixed delay levels).
	CapFixedDelay

	// CapNakDelay means the backend supports NAK + configurable delay
	// (e.g. NATS JetStream NakWithDelay).
	CapNakDelay

	// CapIdempotency means the backend has native idempotent delivery
	// (e.g. Pulsar sequence ID).
	CapIdempotency

	// CapPriority means the backend supports priority queues.
	// Supported by: RabbitMQ (queue priority), Pulsar (priority metadata).
	CapPriority

	// CapOrdered means the backend guarantees in-order delivery within a partition/queue.
	// Supported by: Kafka (partition), Pulsar (partition), RocketMQ (queue).
	CapOrdered

	// CapDLQ means the backend has native dead-letter queue support.
	// Supported by: RocketMQ, RabbitMQ (DLX), Pulsar, NATS (max deliver)
	CapDLQ

	// CapRetry means the backend has native retry support.
	// Supported by: RocketMQ (retry queue), Pulsar (dead letter policy).
	CapRetry

	// CapMultiGroup means the backend supports multiple consumer groups.
	// Supported by: Kafka, RocketMQ, Pulsar, RabbitMQ, NATS.
	CapMultiGroup

	// CapTx means the backend supports transactional messages.
	// Supported by: RocketMQ, Pulsar, Kafka (exactly-once v2).
	CapTx

	// CapBatch means the backend supports batch sending.
	// Supported by: Kafka (ProducerBatch), Pulsar, RocketMQ.
	CapBatch
)

// Capabilities is a set of capabilities that a backend supports.
// Keys are Capability values; values are true if supported.
type Capabilities map[Capability]bool

// Has returns true if the capability is supported.
func (c Capabilities) Has(cap Capability) bool {
	if c == nil {
		return false
	}
	return c[cap]
}

// ── Default capability sets for each backend ──────────────────────────────

// KafkaCapabilities returns the capabilities of Apache Kafka.
// Kafka supports: ordered delivery (partition), multi consumer group,
// batch sending, idempotent delivery (via EnableIdempotent),
// transactional messages (via EnableTx + kgo.Transactional), arbitrary delay
// (via republish), NAK delay (via NakWithDelay), DLQ (via DLQTopic), retry
// (via RetryPolicy), priority (via priority sorting within Subscribe).
// It does NOT support: fixed delay levels (Kafka has no native concept of
// fixed delay; CapFixedDelay is implemented via per-level delay topics +
// a forwarding consumer — a framework-layer workaround, not broker-native).
func KafkaCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,
		CapFixedDelay:     true, // via per-level delay topics + forwarding consumer (framework workaround)
		CapNakDelay:       true,
		CapIdempotency:    true,
		CapPriority:       true,
		CapOrdered:        true,
		CapDLQ:            true,
		CapRetry:          true,
		CapMultiGroup:     true,
		CapTx:             true, // requires EnableTx=true in KafkaProducerConfig
		CapBatch:          true,
	}
}

// RabbitMQCapabilities returns the capabilities of RabbitMQ.
// RabbitMQ supports: priority queues (CapPriority), DLQ via DLX (CapDLQ),
// multi consumer group (CapMultiGroup), arbitrary delay via the
// x-delayed-message plugin (CapArbitraryDelay), fixed delay via per-level
// delay queues (CapFixedDelay), idempotent deduplication via the
// IdempCache interface (CapIdempotency), staircase retry via RetryPolicy
// (CapRetry), ordered delivery within a single queue (CapOrdered),
// AMQP transactions (CapTx), and client-side batch publishing (CapBatch).
func RabbitMQCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,
		CapFixedDelay:     true, // via per-level delay queues (TTL → DLX)
		CapIdempotency:   true,
		CapPriority:      true,
		CapOrdered:       true,
		CapDLQ:           true,
		CapRetry:         true,
		CapMultiGroup:    true,
		CapTx:            true,
		CapBatch:         true,
		CapNakDelay:      true, // republish + x-delay is semantically equivalent
	}
}

// RocketMQCapabilities returns the capabilities of Apache RocketMQ v5.
// RocketMQ supports: arbitrary delay (v5), fixed delay (v4), DLQ,
// retry, ordered delivery, multi consumer group, batch,
// priority queues (via PriorityTopics + sortViewsByPriority),
// and NAK delay (via ChangeInvisibleDuration).
// CapTx is conditional — EnableTx config exists. TransactionProducer
// is implemented (rocketmq.go) but requires broker-side transaction
// support to be enabled; CapTx=false by default for safety.
func RocketMQCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,
		CapFixedDelay:     true,
		CapNakDelay:       true,
		CapIdempotency:    true,
		CapPriority:       true,
		CapOrdered:        true,
		CapDLQ:            true,
		CapRetry:          true,
		CapMultiGroup:     true,
		CapTx:             false, // requires broker-side transaction support; set true if RocketMQ broker has TransactionEnabled=true
		CapBatch:          true,
	}
}

// NatsCapabilities returns the capabilities of NATS JetStream.
// NATS (JetStream) supports: NAK delay (CapNakDelay), multi consumer group
// (CapMultiGroup), batch (CapBatch), DLQ via DLQSubject (CapDLQ), retry via
// MaxDeliver (CapRetry), ordered delivery (CapOrdered), idempotency via
// KV bucket (CapIdempotency), fixed delay via JetStream scheduled delivery
// (CapFixedDelay), arbitrary delay via client-side re-publisher (CapArbitraryDelay),
// and priority via multi-subject routing (CapPriority).
// It does NOT support native transactions (CapTx).
func NatsCapabilities() Capabilities {
	return Capabilities{
		CapFixedDelay:     true, // via JetStream scheduled delivery (PublishAsync + WithNextTime)
		CapArbitraryDelay: true, // via client-side re-publisher goroutine
		CapNakDelay:       true,
		CapMultiGroup:     true,
		CapBatch:          true,
		CapDLQ:            true,
		CapRetry:          true,
		CapOrdered:        true,
		CapIdempotency:    true,
		CapPriority:       true, // via multi-subject routing (topic.p0..pN) + heap
	}
}

// PulsarCapabilities returns the capabilities of Apache Pulsar.
// Pulsar supports: arbitrary delay (DeliverAfter), priority,
// ordered delivery (partition), multi consumer group, tx, batch, DLQ,
// idempotent delivery (via IdempCache), retry (via RetryPolicy),
// fixed delay (via DeliverAfter mapped to nearest level),
// and NAK delay (via republish with delay).
func PulsarCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,
		CapFixedDelay:     true, // via DeliverAfter() (maps to nearest level)
		CapNakDelay:       true, // via republish with delay
		CapIdempotency:    true, // via IdempCache + SequenceID
		CapPriority:       true,
		CapOrdered:        true,
		CapDLQ:            true,
		CapRetry:          true, // via RetryPolicy
		CapMultiGroup:     true,
		CapTx:             true,
		CapBatch:          true,
	}
}

// MqttCapabilities returns the capabilities of MQTT.
// MQTT (v5.0) supports: multi consumer group via shared subscriptions
// (CapMultiGroup), batch via client-side aggregation (CapBatch), retry via
// MQTT v5.0 Retry flag (CapRetry), DLQ via DLQTopic forwarding (CapDLQ),
// idempotency via client-side IdempKey tracking (CapIdempotency), ordered
// delivery within a single topic (CapOrdered), NAK delay via retry topic
// + Message Expiry Interval (CapNakDelay), fixed delay via topic-level routing
// (CapFixedDelay), and arbitrary delay via expiry interval + re-publisher
// (CapArbitraryDelay).
func MqttCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true, // via Message Expiry Interval + re-publisher goroutine
		CapFixedDelay:     true, // via topic-level routing (topic.level.N) + expiry interval
		CapNakDelay:       true, // via retry topic + expiry interval + MaxRetries
		CapMultiGroup:     true, // via shared subscriptions ($share/group/topic)
		CapBatch:          true, // client-side aggregation
		CapRetry:          true, // via Retry flag (MQTT v5.0)
		CapDLQ:            true, // via DLQTopic forwarding after MaxRetries
		CapIdempotency:    true, // client-side IdempKey deduplication
		CapOrdered:        true, // single-topic ordering is naturally preserved
	}
}



