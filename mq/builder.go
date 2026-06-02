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
// These functions convert ProducerOptions/ConsumerOptions to the specific config structs.

func newKafkaProducerFromOptions(opts ProducerOptions) (Producer, error) {
	return NewKafkaProducer(KafkaProducerConfig{
		Brokers:         opts.Brokers,
		MaxMessageBytes: opts.MaxMessageBytes,
	})
}

func newKafkaConsumerFromOptions(opts ConsumerOptions) (Consumer, error) {
	return NewKafkaConsumer(KafkaConsumerConfig{
		Brokers:        opts.Brokers,
		Group:          opts.Group,
		MaxPollRecords: opts.MaxPollRecords,
	})
}

func newRabbitMQProducerFromOptions(opts ProducerOptions) (Producer, error) {
	return NewRabbitMQProducer(RabbitMQConfig{
		URL: opts.URL,
	})
}

func newRabbitMQConsumerFromOptions(opts ConsumerOptions) (Consumer, error) {
	return NewRabbitMQConsumer(RabbitMQConsumerConfig{
		URL: opts.URL,
	})
}

func newNATSProducerFromOptions(opts ProducerOptions) (Producer, error) {
	url := "nats://localhost:4222"
	if len(opts.Brokers) > 0 {
		url = opts.Brokers[0]
	}
	return NewNATSProducer(NATSConfig{
		URL: url,
	})
}

func newNATSConsumerFromOptions(opts ConsumerOptions) (Consumer, error) {
	url := "nats://localhost:4222"
	if len(opts.Brokers) > 0 {
		url = opts.Brokers[0]
	}
	return NewNATSConsumer(NATSConsumerConfig{
		NATSConfig: NATSConfig{URL: url},
	})
}

func newMQTTProducerFromOptions(opts ProducerOptions) (Producer, error) {
	broker := "tcp://localhost:1883"
	if len(opts.Brokers) > 0 {
		broker = opts.Brokers[0]
	}
	return NewMQTTProducer(MQTTConfig{
		Broker:   broker,
		ClientID: "astra-producer",
	})
}

func newMQTTConsumerFromOptions(opts ConsumerOptions) (Consumer, error) {
	broker := "tcp://localhost:1883"
	if len(opts.Brokers) > 0 {
		broker = opts.Brokers[0]
	}
	return NewMQTTConsumer(MQTTConfig{
		Broker:   broker,
		ClientID: "astra-consumer",
	})
}

func newPulsarProducerFromOptions(opts ProducerOptions) (Producer, error) {
	url := "pulsar://localhost:6650"
	if len(opts.Brokers) > 0 {
		url = opts.Brokers[0]
	}
	return NewPulsarProducer(PulsarConfig{
		URL: url,
	})
}

func newPulsarConsumerFromOptions(opts ConsumerOptions) (Consumer, error) {
	url := "pulsar://localhost:6650"
	if len(opts.Brokers) > 0 {
		url = opts.Brokers[0]
	}
	return NewPulsarConsumer(PulsarConsumerConfig{
		PulsarConfig: PulsarConfig{URL: url},
		Subscription: opts.Subscription,
	})
}

func newRocketMQProducerFromOptions(opts ProducerOptions) (Producer, error) {
	endpoint := "localhost:8081"
	if len(opts.Brokers) > 0 {
		endpoint = opts.Brokers[0]
	}
	return NewRocketMQProducer(RocketMQConfig{
		Endpoint: endpoint,
		Topic:    opts.Topic,
	})
}

func newRocketMQConsumerFromOptions(opts ConsumerOptions) (Consumer, error) {
	endpoint := "localhost:8081"
	if len(opts.Brokers) > 0 {
		endpoint = opts.Brokers[0]
	}
	return NewRocketMQConsumer(RocketMQConfig{
		Endpoint:      endpoint,
		ConsumerGroup: opts.Group,
	})
}
