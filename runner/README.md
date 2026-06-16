# Runner — Background Task Scheduler

Background task runner supporting Cron, GoCron, DAG workflow, and task queue scheduling engines.

## Features

- **Cron Engine**: Standard Cron expression scheduling based on `robfig/cron/v3`
- **GoCron Engine**: More flexible Cron expression support
- **DAG Engine (Dagu)**: Directed acyclic graph workflow, supports task dependencies
- **TaskQueue Engine**: Task queue consumption based on `taskqueue` module

## Quick Start

### Cron Engine (Recommended)

```go
import "github.com/astra-go/astra/runner"

r := runner.New(runner.Config{Engine: runner.EngineCron})

// Cron expression scheduling
r.Add("*/5 * * * *", "cache-warmup", func(ctx context.Context) error {
    return warmCache(ctx)
})

// Fixed interval
r.Add("@every 5m", "heartbeat", func(ctx context.Context) error {
    return sendHeartbeat(ctx)
})

r.Start(ctx)
```

### DAG Workflow

```go
import "github.com/astra-go/astra/runner"

r := runner.New(runner.Config{Engine: runner.EngineDAG})

r.AddTask("fetch-data", func(ctx context.Context) error {
    return fetchData(ctx)
}).Then("process-data", func(ctx context.Context) error {
    return processData(ctx)
}).Then("notify", func(ctx context.Context) error {
    return notifyUser(ctx)
})

r.Start(ctx)
```

## API

### New

```go
func New(cfg Config) *Runner

type Config struct {
    Engine string // "cron" | "gocron" | "dag" | "taskqueue"
}
```

### runner.Add

```go
func (r *Runner) Add(schedule, name string, fn func(context.Context) error) *Runner
```

`schedule` supports Cron expressions or predefined values (`@every 5m`).

### DAG Task Chain

```go
r.AddTask("step1", fn1).Then("step2", fn2).Then("step3", fn3)
// step2 depends on step1; step3 depends on step2 (sequential)

// Parallel execution
r.AddTask("job1", fn1).And("job2", fn2).Then("job3", fn3)
// job1 and job2 run in parallel; job3 runs after both complete
```

### runner.Start / runner.Shutdown

```go
r.Start(ctx)
r.Shutdown(ctx) // Graceful stop, wait for running tasks to complete
```

## Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Engine` | `string` | `"cron"` | Scheduler engine type |
| `Concurrency` | `int` | `1` | Max concurrent tasks |

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/astra-go/astra/runner"
    "time"
)

func main() {
    r := runner.New(runner.Config{Engine: runner.EngineCron})

    r.Add("*/5 * * * *", "heartbeat", func(ctx context.Context) error {
        fmt.Println("heartbeat at", time.Now().Format(time.DateTime))
        return nil
    })

    r.Add("@every 1m", "report", func(ctx context.Context) error {
        fmt.Println("generating report...")
        return nil
    })

    ctx := context.Background()
    r.Start(ctx)

    // Simulate running for 3 minutes
    time.Sleep(3 * time.Minute)
    r.Shutdown(ctx)
}
```

## Module Dependencies

| Sub-package | Dependency |
|-------------|-----------|
| `EngineCron` | `github.com/robfig/cron/v3` |
| `EngineDAG` | `github.com/yohamta/dagu` |

## Notes

- DAG workflow tasks don't auto-retry on failure; implement retry logic in handler function
- When task function returns error in `runner.Add`, it's treated as failure; can pair with logging/alerting
- On `Shutdown` graceful stop, running tasks receive ctx cancel signal
