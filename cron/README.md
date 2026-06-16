# Cron — Scheduled Task Scheduler

Scheduled task scheduler based on `robfig/cron/v3`, supports Cron expressions and fixed interval scheduling.

## Features

- **Cron Expression Scheduling**: 6-field format (second precision), including `*/5 * * * * *`
- **Fixed Interval Scheduling**: `Every(d, name, fn)` directly accepts `time.Duration`
- **Predefined Expressions**: `@yearly`, `@monthly`, `@weekly`, `@daily`, `@hourly`
- **App Lifecycle Integration**: Integrates with Astra app lifecycle via `OnStart`/`OnStop`
- **Context Support**: Task functions receive `context.Context`; scheduler stop cancels it automatically

## Quick Start

### Fixed Interval

```go
s := cron.NewScheduler()
s.Every(5*time.Minute, "cache-warmup", func(ctx context.Context) {
    warmCache(ctx)
})
s.Start()
defer s.Shutdown(context.Background())
```

### Cron Expression

```go
s.Cron("0 30 2 * * *", "nightly-report", func(ctx context.Context) {
    generateReport(ctx)
})
s.Start()
```

### App Lifecycle Integration (Recommended)

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

Executes once every `d` interval. `name` is unique task identifier.

### scheduler.Cron

```go
func (s *Scheduler) Cron(expr string, name string, fn func(context.Context)) *Scheduler
```

Schedules using Cron expression.

### scheduler.JobFunc

```go
type JobFunc func(ctx context.Context)

// Implement Job interface, receives scheduler lifecycle management
func (JobFunc) Run(ctx context.Context)
```

### Cron Expression Format (6 fields, second precision)

```
┌───────────── second       (0–59)
│ ┌─────────── minute       (0–59)
│ │ ┌───────── hour         (0–23)
│ │ │ ┌─────── day-of-month (1–31)
│ │ │ │ ┌───── month        (1–12 or JAN–DEC)
│ │ │ │ │ ┌─── day-of-week  (0–6 or SUN–SAT)
│ │ │ │ │ │
* * * * * *
```

### Predefined Expressions

| Expression | Description |
|------------|-------------|
| `@yearly` | Every January 1st at 00:00:00 |
| `@monthly` | 1st of every month at 00:00:00 |
| `@weekly` | Every Sunday at 00:00:00 |
| `@daily` | Every day at 00:00:00 |
| `@hourly` | Every hour at :00:00 |

## Complete Example

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

    // Print every 30 seconds
    s.Every(30*time.Second, "heartbeat", func(ctx context.Context) {
        fmt.Println("heartbeat at", time.Now().Format(time.DateTime))
    })

    // Run at 3 AM daily
    s.Cron("0 0 3 * * *", "daily-cleanup", func(ctx context.Context) {
        fmt.Println("running daily cleanup...")
    })

    s.Start()

    // Simulate running
    time.Sleep(2 * time.Minute)
    s.Shutdown(context.Background())
}
```

## Module Dependencies

- `github.com/robfig/cron/v3` — Cron expression parsing and scheduling

## Notes

- When both month and day-of-week are specified in Cron expression (Astra: both must match to trigger); use `?` in the month field if either should satisfy
- Task functions should check `ctx.Done()` to respond to scheduler stop signal and avoid long blocking
- Scheduler stop doesn't force interrupt running tasks, but no new task cycles start
