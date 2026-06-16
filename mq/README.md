# MQ — Message Queue Abstraction

Unified Producer/Consumer interface supporting multiple message queue backends, decoupling business code from specific Brokers.

## Features

- **Unified Interface**: Producer and Consumer interfaces are broker-agnostic
- **Multi-Backend**: RabbitMQ, Kafka, RocketMQ, MQTT, NATS, Pulsar
- **Consumer Groups**: Multi-consumer load-balanced consumption
- **Message Retry**: Failed messages auto-retry or are sent to dead-letter queue
- **Message Serialization**: Supports JSON, Protobuf, and other formats

## Supported Backends

| Broker | Import Path |
|--------|------------|
| RabbitMQ | `github.com/astra-go/astra/mq/rabbitmq` |
| Apache Kafka | `github.com/astra-go/astra/mq/kafka` |
| Apache RocketMQ | `github.com/astra-go/astra/mq/rocketmq` |
| MQTT | `github.com/astra-go/astra/mq/mqtt` |
| NATS | `github.com/astra-go/astra/mq/nats` |
| Apache Pulsar | `github.com/astra-go/astra/mq/pulsar` |

## Quick Start

### Producer

```go
import "github.com/astra-go/astra/mq/rabbitmq"

producer, _ := rabbitmq.NewProducer(rabbitmq.Config{
    URL: "amqp://guest:guest@localhost:5672/",
})

producer.Publish(ctx, mq.Topic{
    Name: "user.created",
    Body: []byte(`{"user_id": 42}`),
    Headers: map[string]string{"source": "api"},
})
```

### Consumer

```go
consumer, _ := rabbitmq.NewConsumer(rabbitmq.Config{
    URL: "amqp://guest:guest@localhost:5672/",
    Queue: rabbitmq.QueueConfig{Name: "notifications"},
    AutoAck: false, // Manual ack
})

consumer.Consume(func(msg *mq.Message) error {
    fmt.Printf("Received: %s\n", string(msg.Body))
    return nil // nil = ACK, error = NACK and possibly retry
})
```

## API

### Producer Interface

```go
type Producer interface {
    Publish(ctx context.Context, topic Topic) error
    PublishAsync(ctx context.Context, topic Topic) error
    Close() error
}

type Topic struct {
    Name    string            // Topic/Exchange name
    Body    []byte            // Message body
    Key    string             // Routing key (optional)
    Headers map[string]string  // Custom headers
}
```

### Consumer Interface

```go
type Consumer interface {
    Consume(handler MessageHandler) error
    Close() error
}

type MessageHandler func(*Message) error

type Message struct {
    Body     []byte
    Headers  map[string]string
    Topic    string
    Key      string
    Offset   int64  // Message offset
    Timestamp time.Time
}
```

## Config

### RabbitMQ

| Config | Type | Default | Description |
|--------|------|---------|-------------|
| `URL` | `string` | — | AMQP connection URL |
| `Queue.Name` | `string` | — | Queue name |
| `Exchange.Name` | `string` | — | Exchange name |
| `AutoAck` | `bool` | `false` | Auto-ack after consumption |

### Kafka

```go
producer, _ := kafka.NewProducer(kafka.Config{
    Brokers: []string{"localhost:9092"},
    Topic:  "user-events",
})

consumer, _ := kafka.NewConsumer(kafka.Config{
    Brokers:       []string{"localhost:9092"},
    GroupID:       "my-consumer-group",
    Topic:         "user-events",
    AutoOffsetReset: kafka.Earliest,
})
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/astra-go/astra/mq/rabbitmq"
    "github.com/astra-go/astra/mq"
)

func main() {
    ctx := context.Background()

    // Producer
    producer, _ := rabbitmq.NewProducer(rabbitmq.Config{
        URL: "amqp://guest:guest@localhost:5672/",
    })

    producer.Publish(ctx, mq.Topic{
        Name: "order.created",
        Body: []byte(`{"order_id": "123", "amount": 299}`),
    })
    producer.Close()

    // Consumer
    consumer, _ := rabbitmq.NewConsumer(rabbitmq.Config{
        URL:  "amqp://guest:guest@localhost:5672/",
        Queue: rabbitmq.QueueConfig{Name: "order-handler"},
        AutoAck: false,
    })

    consumer.Consume(func(msg *mq.Message) error {
        fmt.Printf("Consumed: %s\n", string(msg.Body))
        return nil
    })
}
```

## Module Dependencies

Each sub-package depends on the corresponding message queue client library.

## Notes

- Both producer and consumer must call `Close()` to close connections
- AutoAck mode strongly relates to business reliability; in production use `AutoAck: false` with manual ack
- When consumer processing fails, message is NACKed; depending on Broker config it may enter dead-letter queue
