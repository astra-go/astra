# TaskQueue — Distributed Task Queue

Distributed task queue supporting multiple backends, delayed tasks, task deduplication, priority queues, and DAG workflows.

## Features

- **Multi-Backend**: Redis (recommended), RabbitMQ, Kafka, RocketMQ, MongoDB
- **Delayed Tasks**: `WithProcessIn`/`WithProcessAt` for delayed execution
- **Task Deduplication**: `WithUnique` prevents duplicate enqueuing
- **Priority Queues**: Multi-queue weighted round-robin, `critical` has higher weight than `default`
- **Auto Retry**: Failed tasks retry with exponential backoff
- **Cron Tasks**: `RegisterCron` for scheduled tasks, multi-instance deduplication
- **Dead-Letter Queue**: Tasks exceeding max retries enter dead-letter queue

## Quick Start

```go
import "github.com/astra-go/astra/taskqueue"

// Create Broker (Redis)
broker, _ := taskqueue.NewRedisBroker(taskqueue.RedisConfig{Addr: "localhost:6379"})
defer broker.Close()

// Create client
client := taskqueue.NewClient(broker)

// Enqueue task
client.EnqueueTask(ctx, "email:welcome", payload,
    taskqueue.WithQueue("critical"),
    taskqueue.WithMaxRetries(3),
)

// Delayed task
client.EnqueueTask(ctx, "report:generate", payload,
    taskqueue.WithProcessIn(10*time.Minute),
)
```

### Worker Server

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

// Common options:
WithQueue(name string)          // Specify queue name (default "default")
WithMaxRetries(n int)           // Max retries (default 0)
WithProcessIn(d time.Duration)  // Delayed execution
WithProcessAt(t time.Time)      // Scheduled execution at specific time
WithUnique(key string, ttl time.Duration) // Task deduplication
```

### Task Structure

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

### Task Retry Config

```go
// Exponential backoff retry
client.EnqueueTask(ctx, "job", payload,
    taskqueue.WithMaxRetries(5),
    taskqueue.WithBackoff(taskqueue.BackoffExponential, 30*time.Second),
)
```

### Cron Tasks

```go
srv.RegisterCron("0 9 * * *", "report:daily", nil,
    taskqueue.WithUnique("report:daily", 23*time.Hour),
)
```

## Config

### Redis Broker

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Addr` | `string` | — | Redis address |
| `DB` | `int` | `0` | Redis DB |
| `Password` | `string` | `""` | Password |

### ServerConfig

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Concurrency` | `int` | `10` | Number of concurrent workers |
| `Queues` | `map[string]int` | `{"default": 1}` | Queue weights |
| `PollingInterval` | `time.Duration` | `1s` | Queue polling interval |

## Complete Example

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

    // Enqueue task
    payload, _ := json.Marshal(map[string]any{"user_id": 42})
    client.EnqueueTask(ctx, "send-welcome-email", payload,
        taskqueue.WithQueue("critical"),
        taskqueue.WithMaxRetries(3),
    )

    // Delayed task
    client.EnqueueTask(ctx, "send-reminder", payload,
        taskqueue.WithProcessIn(1*time.Hour),
    )

    fmt.Println("Task enqueued")
}
```

## Module Dependencies

Backend sub-packages depend on corresponding message queue client libraries:
- Redis: `github.com/redis/go-redis/v9`
- RabbitMQ: `github.com/rabbitmq/amqp091-go`
- Kafka: `github.com/twmb/franz-go/pkg/kgo`
- RocketMQ: `github.com/apache/rocketmq-client-go/v2`
- MongoDB: `go.mongodb.org/mongo-driver`

## Notes

- `WithUnique` key with same key within ttl won't be re-enqueued, preventing duplicate submissions
- `RegisterCron` with `WithUnique` ensures single execution in multi-instance scenarios
- Task Payload recommends JSON or Protobuf serialization
