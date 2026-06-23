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
// batch sending. It does NOT support: arbitrary delay, fixed delay,
// NAK delay, native idempotent delivery, priority, DLQ, retry, tx.
func KafkaCapabilities() Capabilities {
	return Capabilities{
		CapOrdered:    true,
		CapMultiGroup: true,
		CapBatch:      true,
	}
}

// RabbitMQCapabilities returns the capabilities of RabbitMQ.
// RabbitMQ supports: priority queues (CapPriority), DLQ via DLX (CapDLQ),
// multi consumer group (CapMultiGroup), arbitrary delay via the
// x-delayed-message plugin (CapArbitraryDelay), idempotent
// deduplication via the IdempCache interface (CapIdempotency),
// staircase retry via RetryPolicy (CapRetry), ordered delivery
// within a single queue (CapOrdered), AMQP transactions (CapTx),
// and client-side batch publishing (CapBatch).
func RabbitMQCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,
		CapIdempotency:    true,
		CapPriority:       true,
		CapOrdered:        true,
		CapDLQ:            true,
		CapRetry:          true,
		CapMultiGroup:     true,
		CapTx:             true,
		CapBatch:          true,
		CapNakDelay:       true, // republish + x-delay is semantically equivalent
	}
}

// RocketMQCapabilities returns the capabilities of Apache RocketMQ v5.
// RocketMQ supports: arbitrary delay (v5), fixed delay (v4), DLQ,
// retry, ordered delivery, multi consumer group, tx, batch.
func RocketMQCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,
		CapFixedDelay:     true,
		CapDLQ:            true,
		CapRetry:          true,
		CapOrdered:        true,
		CapMultiGroup:     true,
		CapTx:             true,
		CapBatch:          true,
	}
}

// NatsCapabilities returns the capabilities of NATS JetStream.
// NATS supports: NAK delay, multi consumer group.
// It does NOT support: arbitrary delay, fixed delay, priority,
// ordered delivery, DLQ, retry, tx, batch.
func NatsCapabilities() Capabilities {
	return Capabilities{
		CapNakDelay:   true,
		CapMultiGroup: true,
	}
}

// PulsarCapabilities returns the capabilities of Apache Pulsar.
// Pulsar supports: arbitrary delay (DeliverAfter), priority,
// ordered delivery (partition), multi consumer group, tx, batch, DLQ.
func PulsarCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,
		CapPriority:       true,
		CapOrdered:        true,
		CapMultiGroup:     true,
		CapTx:             true,
		CapBatch:          true,
		CapDLQ:            true,
	}
}

// MqttCapabilities returns the capabilities of MQTT.
// MQTT has very limited capabilities (no delay, no priority, no order,
// no DLQ, no retry, no tx, no batch).
func MqttCapabilities() Capabilities {
	return Capabilities{}
}

// MemoryCapabilities returns the capability set for the in-process memory broker.
// Supports: arbitrary delay (timer), ordered delivery (FIFO channel).
// Does not support: idempotency, priority, DLQ, retry, transactions, or batching.
// Suitable for testing and local development only.
func MemoryCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,
		CapIdempotency:    false,
		CapPriority:       false,
		CapOrdered:        true,
		CapDLQ:            false,
		CapRetry:          false,
		CapMultiGroup:     false,
		CapTx:             false,
		CapBatch:          false,
	}
}
