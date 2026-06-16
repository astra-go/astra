# Runner — 后台任务调度器

后台任务运行器，支持 Cron、GoCron、DAG 工作流和任务队列调度引擎。

## 特性

- **Cron 引擎**：基于 `robfig/cron/v3` 的标准 Cron 表达式调度
- **GoCron 引擎**：更灵活的 Cron 表达式支持
- **DAG 引擎（Dagu）**：有向无环图工作流，支持任务依赖
- **TaskQueue 引擎**：基于 `taskqueue` 模块的任务队列消费

## 快速开始

### Cron 引擎（推荐）

```go
import "github.com/astra-go/astra/runner"

r := runner.New(runner.Config{Engine: runner.EngineCron})

// Cron 表达式调度
r.Add("*/5 * * * *", "cache-warmup", func(ctx context.Context) error {
    return warmCache(ctx)
})

// 固定间隔
r.Add("@every 5m", "heartbeat", func(ctx context.Context) error {
    return sendHeartbeat(ctx)
})

r.Start(ctx)
```

### DAG 工作流

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

`schedule` 支持 Cron 表达式或预定义值（`@every 5m`）。

### DAG 任务链

```go
r.AddTask("step1", fn1).Then("step2", fn2).Then("step3", fn3)
// step2 依赖 step1；step3 依赖 step2（顺序执行）

// 并行执行
r.AddTask("job1", fn1).And("job2", fn2).Then("job3", fn3)
// job1 和 job2 并行执行；job3 等两者完成后运行
```

### runner.Start / runner.Shutdown

```go
r.Start(ctx)
r.Shutdown(ctx) // 优雅停止，等待正在运行的任务完成
```

## 配置

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `Engine` | `string` | `"cron"` | 调度引擎类型 |
| `Concurrency` | `int` | `1` | 最大并发任务数 |

## 完整示例

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

    // 模拟运行 3 分钟
    time.Sleep(3 * time.Minute)
    r.Shutdown(ctx)
}
```

## 模块依赖

| 子包 | 依赖 |
|-------------|-----------|
| `EngineCron` | `github.com/robfig/cron/v3` |
| `EngineDAG` | `github.com/yohamta/dagu` |

## 注意事项

- DAG 工作流任务失败时不自动重试；需在处理器函数中实现重试逻辑
- `runner.Add` 中任务函数返回错误时视为失败；可配合日志/告警使用
- `Shutdown` 优雅停止时，正在运行的任务收到 ctx 取消信号