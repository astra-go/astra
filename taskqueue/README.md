# TaskQueue Module

分布式任务队列模块，支持多种消息中间件后端。

## 特性

- 🎯 **统一接口**：所有后端实现相同的 `Broker` 接口
- 🔌 **可插拔后端**：支持 Kafka、MongoDB、RabbitMQ、Redis、RocketMQ
- 🏷️ **按需编译**：使用 build tags 控制编译哪些后端
- 🔒 **并发安全**：所有实现都是线程安全的

## 支持的后端

| 后端      | Build Tag   | 特点                              |
|----------|------------|----------------------------------|
| Kafka    | `kafka`    | 高吞吐，持久化，支持消费者组       |
| MongoDB  | `mongo`    | 文档存储，适合灵活的任务元数据     |
| RabbitMQ | `rabbitmq` | 成熟稳定，支持延迟队列            |
| Redis    | `redis`    | 轻量级，低延迟，基于 Streams      |
| RocketMQ | `rocketmq` | 高可靠，支持事务消息              |

## 快速开始

### 安装

```bash
go get github.com/astra-go/astra/taskqueue@v2.0.0
```

### 基本用法

所有后端都实现了相同的接口：

```go
type Broker interface {
    Publish(ctx context.Context, task *Task) error
    Subscribe(ctx context.Context, queue string, handler Handler) error
    Close() error
}
```

### Kafka 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/taskqueue"
)

func main() {
    broker := taskqueue.NewKafkaBroker(taskqueue.KafkaConfig{
        Brokers: []string{"localhost:9092"},
        Topic:   "tasks",
    })
    defer broker.Close()
    
    ctx := context.Background()
    
    // 发布任务
    err := broker.Publish(ctx, &taskqueue.Task{
        ID:   "task-1",
        Type: "email",
        Payload: map[string]any{
            "to":      "user@example.com",
            "subject": "Welcome",
        },
    })
}
```

### Redis 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/taskqueue"
)

func main() {
    broker := taskqueue.NewRedisBroker(taskqueue.RedisConfig{
        Addr: "localhost:6379",
    })
    defer broker.Close()
    
    ctx := context.Background()
    
    // 订阅任务
    err := broker.Subscribe(ctx, "tasks", func(ctx context.Context, task *taskqueue.Task) error {
        // 处理任务
        return nil
    })
}
```

### RabbitMQ 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/taskqueue"
)

func main() {
    broker := taskqueue.NewRabbitmqBroker(taskqueue.RabbitmqConfig{
        URL: "amqp://guest:guest@localhost:5672/",
    })
    defer broker.Close()
    
    ctx := context.Background()
    
    err := broker.Publish(ctx, &taskqueue.Task{
        ID:   "task-2",
        Type: "report",
    })
}
```

### MongoDB 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/taskqueue"
)

func main() {
    broker := taskqueue.NewMongoBroker(taskqueue.MongoConfig{
        URI:          "mongodb://localhost:27017",
        Database:     "myapp",
        Collection:   "tasks",
    })
    defer broker.Close()
    
    ctx := context.Background()
    err := broker.Subscribe(ctx, "tasks", func(ctx context.Context, task *taskqueue.Task) error {
        return nil
    })
}
```

### RocketMQ 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/taskqueue"
)

func main() {
    broker := taskqueue.NewRocketmqBroker(taskqueue.RocketmqConfig{
        NameServer: "localhost:9876",
        Topic:      "tasks",
    })
    defer broker.Close()
    
    ctx := context.Background()
    
    err := broker.Publish(ctx, &taskqueue.Task{
        ID:   "task-3",
        Type: "sync",
    })
}
```

## 编译标签

使用 build tags 控制编译哪些后端，减少二进制体积：

```bash
# 编译所有后端
go build -tags=alltags

# 仅编译 Redis
go build -tags=redis

# 编译多个后端
go build -tags="kafka,rabbitmq"
```

## 测试

```bash
# 测试所有后端
go test -tags=alltags ./...

# 测试特定后端
go test -tags=redis ./...
```

## 构造器列表

| 构造器 | Build Tag | 说明 |
|--------|-----------|------|
| `NewKafkaBroker(cfg)` | `kafka` | Kafka 消息队列 |
| `NewMongoBroker(cfg)` | `mongo` | MongoDB 任务队列 |
| `NewRabbitmqBroker(cfg)` | `rabbitmq` | RabbitMQ 消息队列 |
| `NewRedisBroker(cfg)` | `redis` | Redis Streams 任务队列 |
| `NewRocketmqBroker(cfg)` | `rocketmq` | RocketMQ 消息队列 |

## 相关文档

- [ADR-005: 子模块数量上限策略](../docs/adr/ADR-005-module-count-limit.md)
- [架构优化路线图](../docs/architecture-optimization-roadmap.md)

## 许可证

MIT
