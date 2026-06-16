# Alert — 基于规则的告警引擎

基于指标采样的规则告警引擎，支持表达式条件和多渠道通知。

## 特性

- **指标驱动**：注册任意函数作为指标来源（CPU、错误率、延迟等）
- **表达式告警**：使用 `github.com/expr-lang/expr` 表达式语言定义阈值规则
- **持续时间告警**：`For` 字段防止抖动 — 指标必须持续触发指定时间后才告警
- **多渠道通知**：支持 Webhook 等渠道；可为每条告警规则选择渠道
- **告警恢复**：规则恢复正常时自动触发恢复通知

## 快速开始

```go
engine := alert.NewEngine(alert.EngineConfig{EvalInterval: 30 * time.Second})

// 注册指标
engine.RegisterMetric("error_rate", func() float64 {
    return metrics.ErrorRate()
})
engine.RegisterMetric("cpu_usage", func() float64 {
    return sys.CPUPercent()
})

// 添加告警规则
engine.AddRule(alert.Rule{
    Name:     "high-error-rate",
    Expr:     "error_rate >= 0.05",
    For:      2 * time.Minute,
    Labels:   map[string]string{"severity": "critical"},
    Channels: []string{"webhook"},
})

// 添加通知渠道
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

创建告警引擎；`EngineConfig.EvalInterval` 控制评估间隔（默认 30 秒）。

### engine.RegisterMetric

```go
func (e *Engine) RegisterMetric(name string, fn MetricFunc) *Engine
```

注册指标函数。`MetricFunc` 返回当前指标 float64 值。可链式调用。

### engine.AddRule

```go
func (e *Engine) AddRule(r Rule) *Engine
```

添加告警规则：

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `Name` | `string` | 唯一规则名 |
| `Expr` | `string` | expr-lang 表达式，如 `"error_rate >= 0.05"` |
| `For` | `time.Duration` | 触发前持续时间（0=立即） |
| `Labels` | `map[string]string` | 额外标签 |
| `Channels` | `[]string` | 通知渠道名称列表 |

### engine.AddChannel

```go
func (e *Engine) AddChannel(ch Channel) *Engine
```

注册通知渠道。`Channel` 接口必须实现 `Send(ctx context.Context, alert Alert) error`。

### WebhookChannel

```go
&alert.WebhookChannel{
    ChannelName: "webhook",
    URL:         "https://hooks.example.com/alerts",
}
```

通过 HTTP POST 将告警 JSON 发送到指定 URL。

### Alert 结构体

```go
type Alert struct {
    Rule     *Rule              // 触发的规则
    Metrics  map[string]float64 // 指标快照
    FiredAt  time.Time          // 首次触发时间
    Resolved bool               // 是否已恢复
}
```

## 配置

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `EvalInterval` | `time.Duration` | `30s` | 指标采样和规则评估间隔 |

## 模块依赖

- `github.com/expr-lang/expr` — 规则表达式求值
- `github.com/astra-go/astra/rule` — Lua 规则引擎（内部使用）

## 注意事项

- `MetricFunc` 必须快速返回，避免阻塞评估循环
- `For` = 0 意味着指标首次达到阈值时立即触发告警，可能导致抖动
- 通过 App 生命周期管理告警引擎：`app.OnStart`/`app.OnStop`