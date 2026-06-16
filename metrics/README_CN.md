# Metrics — Prometheus 指标

内置 Prometheus 指标注册与导出，支持 Counter、Histogram、Gauge 和 Summary 四种类型。

## 特性

- **四种指标类型**：Counter（只增）、Histogram（分布）、Gauge（当前值）、Summary（分位数）
- **HTTP 导出**：内置 `Handler()` 暴露 Prometheus 抓取端点
- **自动聚合**：Histogram/Summary 自动计算分位数（p50/p90/p99 等）
- **标签支持**：每种指标类型都支持多维标签
- **开箱即用**：Astra 内置的 HTTP 请求指标自动采集

## 快速开始

```go
import "github.com/astra-go/astra/metrics"

// 创建指标（全局注册）
counter := metrics.NewCounter("http_requests_total", "Total HTTP requests",
    "method", "handler", // 标签名
)
histogram := metrics.NewHistogram("request_duration_seconds", "Request duration (seconds)",
    []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1}, // buckets
    "method", "path",
)

app.GET("/metrics", metrics.Handler()) // Prometheus 抓取端点
```

## API

### Counter

```go
// 创建
counter := metrics.NewCounter(name, help, labelNames...)

// 方法
counter.Add(1)
counter.With("method", "GET", "path", "/users").Add(1) // 带标签
```

### Histogram

```go
// 创建
histogram := metrics.NewHistogram(name, help, buckets []float64, labelNames...)

// 方法
histogram.Observe(0.042) // 单位：秒
histogram.With("method", "POST").Observe(0.15)
```

### Gauge

```go
// 创建
gauge := metrics.NewGauge(name, help, labelNames...)

// 方法
gauge.Set(42)
gauge.Inc()
gauge.Dec()
gauge.Add(10)
gauge.With("instance", "node-1").Set(100)
```

### Summary

```go
// 创建（服务端分位数计算，多实例场景不推荐）
summary := metrics.NewSummary(name, help, objectives map[float64]float64, labelNames...)
// objectives: 目标分位数和误差的映射，如 0.5: 0.05 (p50 ± 5%)

Summary.Observe(0.042)
```

### Handler

```go
func Handler() astra.HandlerFunc
```

注册 Prometheus 抓取端点，返回所有已注册的指标数据。

## 配置

### 常用 Histogram Buckets

```go
// HTTP 延迟
[]float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// 请求体大小（字节）
[]float64{100, 500, 1024, 5*1024, 10*1024, 100*1024, 1*1024*1024}

// 数据库查询时间
[]float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5}
```

## 完整示例

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/metrics"
)

func main() {
    // 注册 Prometheus 抓取端点
    app := astra.New()
    app.GET("/metrics", metrics.Handler())

    // 自定义业务指标
    ordersCounter := metrics.NewCounter("orders_total", "Total orders",
        "status", "payment_method",
    )
    orderAmountHist := metrics.NewHistogram("order_amount_total", "Order amount",
        []float64{10, 50, 100, 500, 1000, 5000},
        "currency",
    )

    app.POST("/orders", func(c *astra.Ctx) error {
        // 业务逻辑
        ordersCounter.With("status", "success", "payment_method", "alipay").Add(1)
        orderAmountHist.With("currency", "CNY").Observe(299.00)
        return c.JSON(200, astra.Map{"ok": true})
    })

    app.Run(":8080")
}
```

## 模块依赖

- `github.com/prometheus/client_golang` — Prometheus Go 客户端

## 注意事项

- Prometheus 端点 `/metrics` 在生产环境应限制访问（内网或鉴权）
- 标签基数不宜过高（如 user ID），避免指标维度爆炸
- Histogram 在客户端聚合；多实例 Prometheus 应使用 `rate()` 或 `histogram_quantile()` 函数