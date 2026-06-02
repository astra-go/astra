package mq

import (
	"fmt"
)

// ProducerOptions contains common configuration for creating a producer.
// Each MQ implementation uses relevant fields and ignores others.
type ProducerOptions struct {
	// Brokers is a list of broker addresses (Kafka, RocketMQ, NATS, MQTT).
	Brokers []string

	// URL is a single connection URL (RabbitMQ: "amqp://user:pass@host:port/vhost").
	URL string

	// MaxMessageBytes is the maximum message size (Kafka, RocketMQ).
	MaxMessageBytes int

	// Namespace is the tenant/namespace (Pulsar).
	Namespace string

	// Topic is the default topic (Pulsar - for producer initialization).
	Topic string
}

// ConsumerOptions contains common configuration for creating a consumer.
type ConsumerOptions struct {
	// Brokers is a list of broker addresses.
	Brokers []string

	// URL is a single connection URL (RabbitMQ).
	URL string

	// Group is the consumer group ID.
	Group string

	// MaxPollRecords is the maximum records per poll (Kafka).
	MaxPollRecords int

	// Namespace is the tenant/namespace (Pulsar).
	Namespace string

	// Subscription is the subscription name (Pulsar).
	Subscription string
}

// NewProducer creates a producer for the specified MQ type.
// Supported types: "kafka", "rabbitmq", "nats", "mqtt", "pulsar", "rocketmq".
//
// Example:
//
//	p, err := mq.NewProducer("kafka", mq.ProducerOptions{
//	    Brokers: []string{"localhost:9092"},
//	})
func NewProducer(typ string, opts ProducerOptions) (Producer, error) {
	switch typ {
	case "kafka":
		return newKafkaProducerFromOptions(opts)
	case "rabbitmq":
		return newRabbitMQProducerFromOptions(opts)
	case "nats":
		return newNATSProducerFromOptions(opts)
	case "mqtt":
		return newMQTTProducerFromOptions(opts)
	case "pulsar":
		return newPulsarProducerFromOptions(opts)
	case "rocketmq":
		return newRocketMQProducerFromOptions(opts)
	default:
		return nil, fmt.Errorf("mq: unsupported producer type %q (supported: kafka, rabbitmq, nats, mqtt, pulsar, rocketmq)", typ)
	}
}

// NewConsumer creates a consumer for the specified MQ type.
// Supported types: "kafka", "rabbitmq", "nats", "mqtt", "pulsar", "rocketmq".
//
// Example:
//
//	c, err := mq.NewConsumer("kafka", mq.ConsumerOptions{
//	    Brokers: []string{"localhost:9092"},
//	    Group:   "my-service",
//	})
func NewConsumer(typ string, opts ConsumerOptions) (Consumer, error) {
	switch typ {
	case "kafka":
		return newKafkaConsumerFromOptions(opts)
	case "rabbitmq":
		return newRabbitMQConsumerFromOptions(opts)
	case "nats":
		return newNATSConsumerFromOptions(opts)
	case "mqtt":
		return newMQTTConsumerFromOptions(opts)
	case "pulsar":
		return newPulsarConsumerFromOptions(opts)
	case "rocketmq":
		return newRocketMQConsumerFromOptions(opts)
	default:
		return nil, fmt.Errorf("mq: unsupported consumer type %q", typ)
	}
}

// ─── Placeholder implementations (to be filled in Phase 2) ─────────────────────
// These functions will import from the sub-modules (mq/kafka, mq/rabbitmq, etc.)
// and convert ProducerOptions/ConsumerOptions to the specific config structs.

func newKafkaProducerFromOptions(_ ProducerOptions) (Producer, error) {
	// TODO: import "github.com/astra-go/astra/mq/kafka"
	// return kafka.NewProducer(kafka.ProducerConfig{
	//     Brokers: opts.Brokers,
	//     MaxMessageBytes: opts.MaxMessageBytes,
	// })
	return nil, fmt.Errorf("kafka producer: not yet implemented in unified API (use github.com/astra-go/astra/mq/kafka directly for now)")
}

func newKafkaConsumerFromOptions(_ ConsumerOptions) (Consumer, error) {
	return nil, fmt.Errorf("kafka consumer: not yet implemented in unified API")
}

func newRabbitMQProducerFromOptions(_ ProducerOptions) (Producer, error) {
	return nil, fmt.Errorf("rabbitmq producer: not yet implemented in unified API (use github.com/astra-go/astra/mq/rabbitmq directly for now)")
}

func newRabbitMQConsumerFromOptions(_ ConsumerOptions) (Consumer, error) {
	return nil, fmt.Errorf("rabbitmq consumer: not yet implemented in unified API")
}

func newNATSProducerFromOptions(_ ProducerOptions) (Producer, error) {
	return nil, fmt.Errorf("nats producer: not yet implemented in unified API")
}

func newNATSConsumerFromOptions(_ ConsumerOptions) (Consumer, error) {
	return nil, fmt.Errorf("nats consumer: not yet implemented in unified API")
}

func newMQTTProducerFromOptions(_ ProducerOptions) (Producer, error) {
	return nil, fmt.Errorf("mqtt producer: not yet implemented in unified API")
}

func newMQTTConsumerFromOptions(_ ConsumerOptions) (Consumer, error) {
	return nil, fmt.Errorf("mqtt consumer: not yet implemented in unified API")
}

func newPulsarProducerFromOptions(_ ProducerOptions) (Producer, error) {
	return nil, fmt.Errorf("pulsar producer: not yet implemented in unified API")
}

func newPulsarConsumerFromOptions(_ ConsumerOptions) (Consumer, error) {
	return nil, fmt.Errorf("pulsar consumer: not yet implemented in unified API")
}

func newRocketMQProducerFromOptions(_ ProducerOptions) (Producer, error) {
	return nil, fmt.Errorf("rocketmq producer: not yet implemented in unified API")
}

func newRocketMQConsumerFromOptions(_ ConsumerOptions) (Consumer, error) {
	return nil, fmt.Errorf("rocketmq consumer: not yet implemented in unified API")
}
