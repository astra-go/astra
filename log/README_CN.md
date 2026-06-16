# Log — 结构化日志

基于 Go 标准库 `log/slog` 的结构化日志，支持 JSON/Text 双格式、多输出和 OpenTelemetry 上下文注入。

## 特性

- **结构化日志**：键值对记录，天然支持 JSON 格式
- **双格式**：`json`（生产环境推荐）或 `text`（开发友好）
- **多输出**：同时写入多个 Writer（stdout + 文件）
- **上下文传播**：支持提取 Trace ID/Span ID 并注入日志
- **OTel 集成**：注册 `SetSpanExtractor` 后自动注入分布式追踪上下文

## 快速开始

```go
import "github.com/astra-go/astra/log"

// 初始化全局日志器
log.SetDefault(log.New(log.Config{
    Level:  log.LevelInfo,
    Format: "json", // "json" 或 "text"
    Output: os.Stdout,
}))

log.Info("server started", "addr", ":8080", "pid", os.Getpid())
log.Warn("slow query", "duration_ms", 1500, "sql", "SELECT ...")
log.Error("request failed", "err", err, "path", "/api/users")
```

## API

### log.New

```go
func New(cfg Config) *slog.Logger
```

### Config

| 字段 | 类型 | 默认值 | 说明 |
|-------|------|---------|-------------|
| `Level` | `Level` | `LevelInfo` | 最小日志级别 |
| `Format` | `string` | `"text"` | `"json"` 或 `"text"` |
| `Output` | `io.Writer` | — | 单个输出（设置了 `Outputs` 时忽略） |
| `Outputs` | `[]io.Writer` | — | 多个输出目标 |
| `AddSource` | `bool` | `false` | 是否记录源码位置 |

### log.SetDefault

```go
func SetDefault(l *slog.Logger)
```

设置全局默认日志器。随后 `log.Info`/`log.Warn` 等使用此日志器。

### log.SetSpanExtractor

```go
func SetSpanExtractor(fn func(ctx context.Context) (traceID, spanID string))
```

注册 OpenTelemetry 追踪上下文提取器。注册后日志自动携带 `trace_id` 和 `span_id`：

```go
import astraotel "github.com/astra-go/astra/otel"
log.SetSpanExtractor(astraotel.SpanExtractor)
```

## 配置

### 多输出示例

```go
f, _ := os.Create("app.log")
log.New(log.Config{
    Level:   log.LevelDebug,
    Format:  "json",
    Outputs: []io.Writer{os.Stdout, f}, // 同时写入控制台和文件
})
```

### 开发/生产配置

```go
// 开发
log.New(log.Config{Level: log.LevelDebug, Format: "text"})

// 生产
log.New(log.Config{Level: log.LevelInfo, Format: "json"})
```

## 完整示例

```go
package main

import (
    "os"
    "github.com/astra-go/astra/log"
)

func main() {
    // 同时写入 stdout 和文件
    f, _ := os.Create("app.log")
    logger := log.New(log.Config{
        Level:   log.LevelDebug,
        Format:  "json",
        Outputs: []io.Writer{os.Stdout, f},
    })

    log.SetDefault(logger)

    log.Info("app starting",
        "version", "1.0.0",
        "env", "production",
    )

    log.With("request_id", "abc123").Warn("rate limit approaching",
        "current_rpm", 980,
    )

    log.Error("database connection lost",
        "err", "connection refused",
        "host", "localhost",
    )
}
```

## 模块依赖

- `go.opentelemetry.io/otel` — 分布式追踪上下文提取（可选）

## 注意事项

- 日志级别遵循 `slog` 规范：Debug < Info < Warn < Error
- 生产环境推荐 `json` 格式，便于日志采集系统（ELK/Loki）解析
- `SetSpanExtractor` 只需在启动时调用一次；确保 OpenTelemetry 已初始化
- `log.With` 返回带额外上下文的派生日志器，不修改原日志器