# TaskQueue — 分布式任务队列

分布式任务队列，支持多后端、延迟任务、任务去重、优先级队列和 DAG 工作流。

## 特性

- **多后端**：Redis（推荐）、RabbitMQ、Kafka、RocketMQ、MongoDB
- **延迟任务**：`WithProcessIn`/`WithProcessAt` 实现延迟执行
- **任务去重**：`WithUnique` 防止重复入队
- **优先级队列**：多队列加权轮询，`critical` 权重高于 `default`
- **自动重试**：失败任务指数退避重试
- **Cron 任务**：`RegisterCron` 注册定时任务，多实例自动去重
- **死信队列**：超过最大重试次数的任务进入死信队列

## 快速开始

```go
import "github.com/astra-go/astra/taskqueue"

// 创建 Broker（Redis）
broker, _ := taskqueue.NewRedisBroker(taskqueue.RedisConfig{Addr: "localhost:6379"})
defer broker.Close()

// 创建客户端
client := taskqueue.NewClient(broker)

// 入队任务
client.EnqueueTask(ctx, "email:welcome", payload,
    taskqueue.WithQueue("critical"),
    taskqueue.WithMaxRetries(3),
)

// 延迟任务
client.EnqueueTask(ctx, "report:generate", payload,
    taskqueue.WithProcessIn(10*time.Minute),
)
```

### Worker 服务端

```go
mux := taskqueue.NewServeMux()
mux.HandleFunc("email:welcome", func(ctx context.Context, t *taskqueue.Task) error {
    return sendWelcomeEmail(t.Payload)
})

srv := taskqueue.NewServer(taskqueue.ServerConfig{
    Broker:      broker,
    Queues:      map[string]int{"critical": 6, "default": 3, "low": 1},
    Concurrency: 20,
})
srv.Run(ctx, mux)
```

## API

### EnqueueTask

```go
func (c *Client) EnqueueTask(ctx context.Context, name string, payload []byte, opts ...Option) error

// 常用选项：
WithQueue(name string)          // 指定队列名（默认 "default"）
WithMaxRetries(n int)           // 最大重试次数（默认 0）
WithProcessIn(d time.Duration)  // 延迟执行
WithProcessAt(t time.Time)      // 指定时间执行
WithUnique(key string, ttl time.Duration) // 任务去重
```

### Task 结构体

```go
type Task struct {
    ID        string
    Name      string
    Payload   []byte
    Retries   int
    Queue     string
    ProcessAt time.Time
}
```

### 任务重试配置

```go
// 指数退避重试
client.EnqueueTask(ctx, "job", payload,
    taskqueue.WithMaxRetries(5),
    taskqueue.WithBackoff(taskqueue.BackoffExponential, 30*time.Second),
)
```

### Cron 任务

```go
srv.RegisterCron("0 9 * * *", "report:daily", nil,
    taskqueue.WithUnique("report:daily", 23*time.Hour),
)
```

## 配置

### Redis Broker

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `Addr` | `string` | — | Redis 地址 |
| `DB` | `int` | `0` | Redis DB |
| `Password` | `string` | `""` | 密码 |

### ServerConfig

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `Concurrency` | `int` | `10` | 并发 Worker 数量 |
| `Queues` | `map[string]int` | `{"default": 1}` | 队列权重 |
| `PollingInterval` | `time.Duration` | `1s` | 队列轮询间隔 |

## 完整示例

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "github.com/astra-go/astra/taskqueue"
    "time"
)

func main() {
    ctx := context.Background()
    broker, _ := taskqueue.NewRedisBroker(taskqueue.RedisConfig{Addr: "localhost:6379"})
    defer broker.Close()

    client := taskqueue.NewClient(broker)

    // 入队
    payload, _ := json.Marshal(map[string]any{"user_id": 42})
    client.EnqueueTask(ctx, "send-welcome-email", payload,
        taskqueue.WithQueue("critical"),
        taskqueue.WithMaxRetries(3),
    )

    // 延迟任务
    client.EnqueueTask(ctx, "send-reminder", payload,
        taskqueue.WithProcessIn(1*time.Hour),
    )

    fmt.Println("Task enqueued")
}
```

## 模块依赖

各后端子包依赖对应的消息队列客户端库：
- Redis: `github.com/redis/go-redis/v9`
- RabbitMQ: `github.com/rabbitmq/amqp091-go`
- Kafka: `github.com/twmb/franz-go/pkg/kgo`
- RocketMQ: `github.com/apache/rocketmq-client-go/v2`
- MongoDB: `go.mongodb.org/mongo-driver`

## 注意事项

- `WithUnique` 相同 key 在 ttl 有效期内不会重复入队，防止重复提交
- `RegisterCron` 配合 `WithUnique` 确保多实例场景下单次执行
- Task Payload 推荐使用 JSON 或 Protobuf 序列化