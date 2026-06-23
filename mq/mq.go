// Package mq defines a broker-agnostic messaging abstraction for Astra.
//
// The two core interfaces are Producer and Consumer. Each MQ backend
// implements them in its own sub-package:
//
//	mq/rabbitmq  — RabbitMQ (AMQP 0-9-1)
//	mq/kafka     — Apache Kafka
//	mq/rocketmq  — Apache RocketMQ 5.x (gRPC)
//	mq/mqtt      — MQTT 3.1.1/5.0 (EMQX, Mosquitto, NanoMQ)
//
// # Quick start
//
//	p, _ := rabbitmq.NewProducer(rabbitmq.Config{URL: "amqp://guest:guest@localhost:5672/"})
//	defer p.Close()
//
//	p.Publish(ctx, &mq.Message{Topic: "orders", Payload: []byte(`{"id":1}`)})
//
//	c, _ := rabbitmq.NewConsumer(rabbitmq.ConsumerConfig{...})
//	c.Subscribe(ctx, []string{"orders"}, "my-group", func(ctx context.Context, msg *mq.Message) error {
//	    fmt.Println(string(msg.Payload))
//	    return nil  // nil = ack
//	})
package mq

import (
	"context"
	"time"
)

// Message is a broker-agnostic message envelope.
type Message struct {
	// Topic is the destination topic, queue, or routing key.
	Topic string

	// Key is an optional partition/routing key (Kafka partition key,
	// RabbitMQ routing key suffix, etc.)
	Key []byte

	// Payload is the raw message body.
	Payload []byte

	// Headers carries optional metadata.
	Headers map[string]string

	// Meta contains broker-specific read-only fields populated by the consumer
	// (e.g. partition, offset, message ID). It is nil when publishing.
	Meta map[string]any

	// ── v2 新增字段 ──

	// Delay is the delay duration before the message is delivered.
	// Zero means deliver immediately.
	Delay time.Duration

	// IdempKey is the idempotency key; the framework auto-deduplicates
	// messages with the same IdempKey within the TTL window.
	IdempKey string

	// TTL is the time-to-live for the message; if the message is not
	// consumed within TTL, it is discarded.
	TTL time.Duration

	// RetryMax is the maximum number of retries for this message.
	// Zero means use the default retry policy.
	RetryMax int

	// TraceID is the distributed tracing ID, auto-injected if empty.
	TraceID string
}

// Handler processes a received message.
//
//   - Returning nil acknowledges the message (ack/commit).
//   - Returning an error nacks or requeues it (broker-dependent behaviour).
//   - Returning a *MQError with Kind=ErrPermanent means the message
//     should be sent to the dead-letter queue (no retry).
//   - Returning a *MQError with Kind=ErrRetry means the message
//     should be retried after the specified RetryAfter duration.
//   - Returning a *MQError with Kind=ErrSkip means the message
//     should be skipped (no retry, no dead-letter).
type Handler func(ctx context.Context, msg *Message) error

// ── Message builder methods ──

// NewMessage creates a new Message with the given topic and payload.
func NewMessage(topic string, payload []byte) *Message {
	return &Message{
		Topic:   topic,
		Payload: payload,
	}
}

// WithDelay sets the delay duration for the message.
func (m *Message) WithDelay(d time.Duration) *Message {
	m.Delay = d
	return m
}

// WithIdempKey sets the idempotency key for the message.
func (m *Message) WithIdempKey(key string) *Message {
	m.IdempKey = key
	return m
}

// WithTTL sets the time-to-live for the message.
func (m *Message) WithTTL(ttl time.Duration) *Message {
	m.TTL = ttl
	return m
}

// WithRetryMax sets the maximum number of retries for the message.
func (m *Message) WithRetryMax(n int) *Message {
	m.RetryMax = n
	return m
}

// WithTraceID sets the distributed tracing ID for the message.
func (m *Message) WithTraceID(traceID string) *Message {
	m.TraceID = traceID
	return m
}

// Producer publishes messages to a message broker.
type Producer interface {
	// Publish sends a single message synchronously.
	Publish(ctx context.Context, msg *Message) error

	// PublishBatch sends multiple messages. Implementations may send them
	// as a single broker transaction/batch where supported.
	PublishBatch(ctx context.Context, msgs []*Message) error

	// Close flushes pending messages and releases all resources.
	Close() error

	// Capabilities returns the set of capabilities supported by this producer's backend.
	Capabilities() Capabilities
}

// Consumer subscribes to topics and processes messages.
type Consumer interface {
	// Subscribe starts consuming from the given topics.
	// It blocks until ctx is cancelled or a fatal error occurs.
	// The handler is called once per message; returning nil acks the message.
	Subscribe(ctx context.Context, topics []string, group string, handler Handler) error

	// Close stops consuming and releases resources.
	Close() error

	// Capabilities returns the set of capabilities supported by this consumer's backend.
	Capabilities() Capabilities
}
