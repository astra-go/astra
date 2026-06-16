# MQ — 消息队列抽象

统一 Producer/Consumer 接口，支持多种消息队列后端，实现业务代码与具体 Broker 解耦。

## 特性

- **统一接口**：Producer 和 Consumer 接口与 Broker 无关
- **多后端**：RabbitMQ、Kafka、RocketMQ、MQTT、NATS、Pulsar
- **Consumer Group**：多消费者负载均衡消费
- **消息重试**：失败消息自动重试或进入死信队列
- **消息序列化**：支持 JSON、Protobuf 等格式

## 支持的后端

| Broker | 导入路径 |
|--------|------------|
| RabbitMQ | `github.com/astra-go/astra/mq/rabbitmq` |
| Apache Kafka | `github.com/astra-go/astra/mq/kafka` |
| Apache RocketMQ | `github.com/astra-go/astra/mq/rocketmq` |
| MQTT | `github.com/astra-go/astra/mq/mqtt` |
| NATS | `github.com/astra-go/astra/mq/nats` |
| Apache Pulsar | `github.com/astra-go/astra/mq/pulsar` |

## 快速开始

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
    AutoAck: false, // 手动 ack
})

consumer.Consume(func(msg *mq.Message) error {
    fmt.Printf("Received: %s\n", string(msg.Body))
    return nil // nil = ACK，error = NACK 并可能重试
})
```

## API

### Producer 接口

```go
type Producer interface {
    Publish(ctx context.Context, topic Topic) error
    PublishAsync(ctx context.Context, topic Topic) error
    Close() error
}

type Topic struct {
    Name    string            // Topic/Exchange 名
    Body    []byte            // 消息体
    Key    string             // 路由键（可选）
    Headers map[string]string  // 自定义头信息
}
```

### Consumer 接口

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
    Offset   int64  // 消息偏移量
    Timestamp time.Time
}
```

## 配置

### RabbitMQ

| 配置 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `URL` | `string` | — | AMQP 连接 URL |
| `Queue.Name` | `string` | — | 队列名 |
| `Exchange.Name` | `string` | — | Exchange 名 |
| `AutoAck` | `bool` | `false` | 消费后自动 ack |

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

## 完整示例

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

## 模块依赖

各子包依赖对应的消息队列客户端库。

## 注意事项

- Producer 和 Consumer 都必须调用 `Close()` 关闭连接
- AutoAck 模式与业务可靠性密切相关；生产环境使用 `AutoAck: false` 并手动 ack
- 消费者处理失败时消息被 NACK；根据 Broker 配置可能进入死信队列