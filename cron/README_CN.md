# Cron — 定时任务调度器

基于 `robfig/cron/v3` 的定时任务调度器，支持 Cron 表达式和固定间隔调度。

## 特性

- **Cron 表达式调度**：6 字段格式（秒级精度），支持 `*/5 * * * * *`
- **固定间隔调度**：`Every(d, name, fn)` 直接接受 `time.Duration`
- **预定义表达式**：`@yearly`、`@monthly`、`@weekly`、`@daily`、`@hourly`
- **App 生命周期集成**：通过 `OnStart`/`OnStop` 与 Astra App 生命周期集成
- **Context 支持**：任务函数接收 `context.Context`；调度器停止时自动取消

## 快速开始

### 固定间隔

```go
s := cron.NewScheduler()
s.Every(5*time.Minute, "cache-warmup", func(ctx context.Context) {
    warmCache(ctx)
})
s.Start()
defer s.Shutdown(context.Background())
```

### Cron 表达式

```go
s.Cron("0 30 2 * * *", "nightly-report", func(ctx context.Context) {
    generateReport(ctx)
})
s.Start()
```

### App 生命周期集成（推荐）

```go
s := cron.NewScheduler()
s.Every(time.Minute, "heartbeat", func(ctx context.Context) { ... })

app.OnStart(func(ctx context.Context) error {
    s.Start()
    return nil
})
app.OnStop(func(ctx context.Context) error {
    s.Shutdown(ctx)
    return nil
})
```

## API

### NewScheduler

```go
func NewScheduler() *Scheduler
```

### scheduler.Every

```go
func (s *Scheduler) Every(d time.Duration, name string, fn func(context.Context)) *Scheduler
```

每隔 `d` 执行一次。`name` 是唯一任务标识符。

### scheduler.Cron

```go
func (s *Scheduler) Cron(expr string, name string, fn func(context.Context)) *Scheduler
```

使用 Cron 表达式调度。

### scheduler.JobFunc

```go
type JobFunc func(ctx context.Context)

// 实现 Job 接口，接收调度器生命周期管理
func (JobFunc) Run(ctx context.Context)
```

### Cron 表达式格式（6 字段，秒级精度）

```
┌───────────── 秒         (0–59)
│ ┌─────────── 分钟       (0–59)
│ │ ┌───────── 小时       (0–23)
│ │ │ ┌─────── 日        (1–31)
│ │ │ │ ┌───── 月        (1–12 或 JAN–DEC)
│ │ │ │ │ ┌─── 星期      (0–6 或 SUN–SAT)
│ │ │ │ │ │
* * * * * *
```

### 预定义表达式

| 表达式 | 说明 |
|------------|-------------|
| `@yearly` | 每年 1 月 1 日 00:00:00 |
| `@monthly` | 每月 1 日 00:00:00 |
| `@weekly` | 每周日 00:00:00 |
| `@daily` | 每天 00:00:00 |
| `@hourly` | 每小时 :00:00 |

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/astra-go/astra/cron"
)

func main() {
    s := cron.NewScheduler()

    // 每 30 秒打印一次
    s.Every(30*time.Second, "heartbeat", func(ctx context.Context) {
        fmt.Println("heartbeat at", time.Now().Format(time.DateTime))
    })

    // 每天凌晨 3 点运行
    s.Cron("0 0 3 * * *", "daily-cleanup", func(ctx context.Context) {
        fmt.Println("running daily cleanup...")
    })

    s.Start()

    // 模拟运行
    time.Sleep(2 * time.Minute)
    s.Shutdown(context.Background())
}
```

## 模块依赖

- `github.com/robfig/cron/v3` — Cron 表达式解析和调度

## 注意事项

- Cron 表达式同时指定了月和星期时（Astra：两者都必须满足才触发）；如需两者任一满足使用 `?`
- 任务函数应检查 `ctx.Done()` 以响应调度器停止信号，避免长时间阻塞
- 调度器停止不会强制中断正在运行的任务，但不会再启动新的任务周期