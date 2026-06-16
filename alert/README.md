# Alert — Rule-Based Alerting Engine

Rule-based alerting engine based on metric sampling, supports expression conditions and multi-channel notifications.

## Features

- **Metric-Driven**: Register arbitrary functions as metric sources (CPU, error rate, latency, etc.)
- **Expression Alerts**: Define threshold rules using `github.com/expr-lang/expr` expression language
- **Duration Alerts**: `For` field prevents flapping — metrics must trigger continuously for specified duration before alerting
- **Multi-Channel Notifications**: Supports Webhook and other channels; can select channels per alert rule
- **Alert Recovery**: Auto-triggers recovery notification when rule returns to normal

## Quick Start

```go
engine := alert.NewEngine(alert.EngineConfig{EvalInterval: 30 * time.Second})

// Register metrics
engine.RegisterMetric("error_rate", func() float64 {
    return metrics.ErrorRate()
})
engine.RegisterMetric("cpu_usage", func() float64 {
    return sys.CPUPercent()
})

// Add alert rules
engine.AddRule(alert.Rule{
    Name:     "high-error-rate",
    Expr:     "error_rate >= 0.05",
    For:      2 * time.Minute,
    Labels:   map[string]string{"severity": "critical"},
    Channels: []string{"webhook"},
})

// Add notification channels
engine.AddChannel(&alert.WebhookChannel{
    ChannelName: "webhook",
    URL:         "https://hooks.example.com/alerts",
})

engine.Start(ctx)
defer engine.Stop()
```

## API

### alert.NewEngine

```go
func NewEngine(cfg EngineConfig) *Engine
```

Creates alerting engine; `EngineConfig.EvalInterval` controls evaluation interval (default 30 seconds).

### engine.RegisterMetric

```go
func (e *Engine) RegisterMetric(name string, fn MetricFunc) *Engine
```

Registers metric function. `MetricFunc` returns current metric float64 value. Chainable.

### engine.AddRule

```go
func (e *Engine) AddRule(r Rule) *Engine
```

Adds alert rule:

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` | Unique rule name |
| `Expr` | `string` | expr-lang expression, e.g., `"error_rate >= 0.05"` |
| `For` | `time.Duration` | Trigger duration before alerting (0=immediate) |
| `Labels` | `map[string]string` | Additional labels |
| `Channels` | `[]string` | Notification channel name list |

### engine.AddChannel

```go
func (e *Engine) AddChannel(ch Channel) *Engine
```

Registers notification channel. `Channel` interface must implement `Send(ctx context.Context, alert Alert) error`.

### WebhookChannel

```go
&alert.WebhookChannel{
    ChannelName: "webhook",
    URL:         "https://hooks.example.com/alerts",
}
```

Sends alert JSON via HTTP POST to specified URL.

### Alert Structure

```go
type Alert struct {
    Rule     *Rule              // Triggering rule
    Metrics  map[string]float64 // Metric snapshot
    FiredAt  time.Time          // First trigger time
    Resolved bool               // Whether recovered
}
```

## Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `EvalInterval` | `time.Duration` | `30s` | Metric sampling and rule evaluation interval |

## Module Dependencies

- `github.com/expr-lang/expr` — Rule expression evaluation
- `github.com/astra-go/astra/rule` — Lua rule engine (internal use)

## Notes

- `MetricFunc` must return quickly to avoid blocking the evaluation loop
- `For` = 0 means alert triggers immediately when metric first reaches threshold, may cause flapping
- Manage alert engine with App lifecycle via `app.OnStart`/`app.OnStop`
