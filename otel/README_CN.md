# OTel — OpenTelemetry 集成

OpenTelemetry 分布式追踪和指标集成，支持 OTLP 导出。

## 特性

- **自动追踪**：HTTP/gRPC 请求自动创建 Span
- **OTLP 导出**：支持将 Trace 和 Metrics 导出到 OTel Collector
- **日志集成**：`SpanExtractor` 将 Trace ID 注入日志
- **传播协议**：自动注入和提取 W3C TraceContext 和 B3 格式
- **指标导出**：Prometheus 格式和 OTLP 双导出

## 快速开始

```go
import (
    "context"
    astraotel "github.com/astra-go/astra/otel"
    "github.com/astra-go/astra/log"
)

ctx := context.Background()

shutdown, err := astraotel.Setup(ctx, astraotel.Config{
    ServiceName:  "my-service",
    ServiceVersion: "1.0.0",
    Exporter:    astraotel.ExporterOTLP,
    Endpoint:    "http://otel-collector:4318",
})
defer shutdown()

// 启用日志集成
log.SetSpanExtractor(astraotel.SpanExtractor)
```

## API

### Setup

```go
func Setup(ctx context.Context, cfg Config) (func(), error)
```

初始化 OpenTelemetry，返回关闭函数（App 退出时调用）。

### Config

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `ServiceName` | `string` | — | 服务名（必填） |
| `ServiceVersion` | `string` | `""` | 服务版本 |
| `Exporter` | `string` | `ExporterOTLP` | 导出方式 |
| `Endpoint` | `string` | — | Collector 地址 |
| `Insecure` | `bool` | `true` | 是否使用 TLS |

### Exporter 类型

| 常量 | 说明 |
|----------|-------------|
| `ExporterOTLP` | OTLP over HTTP（推荐） |
| `ExporterStdout` | 标准输出（用于调试） |
| `ExporterJaeger` | Jaeger 原生协议 |

### SpanExtractor

```go
// 将 Trace/Span ID 注入日志
func SpanExtractor(ctx context.Context) (traceID, spanID string)
```

## 完整示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/otel"
    "github.com/astra-go/astra/log"
)

func main() {
    ctx := context.Background()

    // 初始化 OTel
    shutdown, _ := otel.Setup(ctx, otel.Config{
        ServiceName:  "order-service",
        ServiceVersion: "1.0.0",
        Exporter:    otel.ExporterOTLP,
        Endpoint:    "http://localhost:4318",
        Insecure:    true,
    })
    defer shutdown()

    // 启用日志 Trace ID 注入
    log.SetSpanExtractor(otel.SpanExtractor)

    log.Info("application started")
}
```

## 模块依赖

- `go.opentelemetry.io/otel` — OTel 核心
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` — OTLP HTTP 导出器

## 注意事项

- `Setup` 必须在创建 HTTP/gRPC 服务器之前调用
- App 退出时调用 `shutdown` 函数确保数据刷新
- 生产环境 `Insecure` 应为 `false` 并配置 TLS 证书