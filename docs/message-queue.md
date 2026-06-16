# Message Queue

Astra's MQ module provides unified Producer/Consumer interfaces supporting multiple message brokers.

## Interface Definition

```go
// Producer sends messages
type Producer interface {
    Publish(ctx context.Context, msg *Message) error
    PublishBatch(ctx context.Context, msgs []*Message) error
    Close() error
}

// Consumer consumes messages
type Consumer interface {
    Subscribe(ctx context.Context, topics []string, group string, handler Handler) error
    Close() error
}

// Message structure
type Message struct {
    Topic   string            // Topic/queue name
    Key     []byte            // Partition key
    Payload []byte            // Message body
    Headers map[string]string // Metadata headers
    Meta    map[string]any    // Broker metadata (read-only)
}
```

## RabbitMQ

```go
import (
    "github.com/astra-go/astra/mq"
    "github.com/astra-go/astra/mq/rabbitmq"
)

// Create Producer
producer, err := rabbitmq.NewProducer(rabbitmq.Config{
    URL:      "amqp://guest:guest@localhost:5672/",
    Exchange: "orders",           // Optional, defaults to direct
    Kind:     "direct",           // Exchange type
})

// Send message
err = producer.Publish(ctx, &mq.Message{
    Topic:   "order.created",
    Payload: []byte(`{"order_id": 123, "amount": 99.99}`),
    Headers: map[string]string{"source": "web"},
})

// Batch send
err = producer.PublishBatch(ctx, []*mq.Message{msg1, msg2, msg3})

// Create Consumer
consumer, err := rabbitmq.NewConsumer(rabbitmq.ConsumerConfig{
    URL:        "amqp://guest:guest@localhost:5672/",
    Exchange:   "orders",
    Queue:      "order.created.queue",
    RoutingKey: "order.created",
    Kind:       "direct",
})

// Subscribe to messages
err = consumer.Subscribe(ctx, []string{"order.created"}, "order-service",
    func(ctx context.Context, msg *mq.Message) error {
        log.Printf("Received: %s", string(msg.Payload))
        // nil = ack, error = nack/requeue
        return nil
    })
```

## Apache Kafka

```go
import (
    "github.com/astra-go/astra/mq"
    "github.com/astra-go/astra/mq/kafka"
)

// Producer
producer, err := kafka.NewProducer(kafka.Config{
    Brokers: []string{"localhost:9092", "localhost:9093"},
})

err = producer.Publish(ctx, &mq.Message{
    Topic:   "user.events",
    Key:     []byte("user_42"),          // Partition key
    Payload: []byte(`{"type": "signup", "user_id": 42}`),
})

// Consumer
consumer, err := kafka.NewConsumer(kafka.ConsumerConfig{
    Brokers: []string{"localhost:9092"},
    GroupID: "user-service",
})

err = consumer.Subscribe(ctx, []string{"user.events"}, "user-service",
    func(ctx context.Context, msg *mq.Message) error {
        log.Printf("partition=%s offset=%s msg=%s",
            msg.Meta["partition"], msg.Meta["offset"], string(msg.Payload))
        return nil
    })
```

## RocketMQ

```go
import "github.com/astra-go/astra/mq/rocketmq"

// Producer
producer, err := rocketmq.NewProducer(rocketmq.Config{
    Endpoint: "localhost:9876",
    // Supports 5.x gRPC protocol
})

// Consumer
consumer, err := rocketmq.NewConsumer(rocketmq.ConsumerConfig{
    Endpoint: "localhost:9876",
    GroupID:  "my-group",
})
```

## MQTT

```go
import "github.com/astra-go/astra/mq/mqtt"

// Suitable for IoT, mobile push scenarios
producer, err := mqtt.NewProducer(mqtt.Config{
    Broker:   "tcp://localhost:1883",
    ClientID: "publisher-1",
    QoS:      1, // At least once delivery
})
```

## NATS

```go
import "github.com/astra-go/astra/mq/nats"

producer, err := nats.NewProducer(nats.Config{
    URL: "nats://localhost:4222",
})
```

## Pulsar

```go
import "github.com/astra-go/astra/mq/pulsar"

producer, err := pulsar.NewProducer(pulsar.Config{
    URL: "pulsar://localhost:6650",
})
```

## Switching Between Brokers

The biggest advantage of the MQ module: switching brokers only requires changing imports and initialization — business logic stays the same:

```go
// Switching RabbitMQ → Kafka only changes these two lines
import "github.com/astra-go/astra/mq/kafka"
// import "github.com/astra-go/astra/mq/rabbitmq"

producer, _ := kafka.NewProducer(kafka.Config{...})
// producer, _ := rabbitmq.NewProducer(rabbitmq.Config{...})

// Below code doesn't change at all
producer.Publish(ctx, &mq.Message{
    Topic:   "order.created",
    Payload: payload,
})
```

## Best Practices

1. **Message Structure**: Use Protobuf or MessagePack instead of JSON to reduce serialization overhead
2. **Retry Mechanism**: return error on consumption failure for auto-retry; use dead-letter queue for failed messages
3. **Idempotent Consumption**: consumption processing should be idempotent to prevent data inconsistency from duplicate delivery
4. **Batch Publishing**: use `PublishBatch` to reduce network round-trips
5. **Monitoring and Alerting**: monitor consumption latency, backlog, and retry count
