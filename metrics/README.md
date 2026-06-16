# Metrics — Prometheus Metrics

Built-in Prometheus metrics registry and export, supporting Counter, Histogram, Gauge, and Summary types.

## Features

- **Four Metric Types**: Counter (increment-only), Histogram (distribution), Gauge (current value), Summary (quantiles)
- **HTTP Export**: Built-in `Handler()` exposes Prometheus scrape endpoint
- **Auto Aggregation**: Histogram/Summary auto-calculate quantiles (p50/p90/p99, etc.)
- **Label Support**: Each metric type supports multi-dimensional labels
- **Out of the Box**: Astra's built-in HTTP request metrics auto-recorded

## Quick Start

```go
import "github.com/astra-go/astra/metrics"

// Create metrics (globally registered)
counter := metrics.NewCounter("http_requests_total", "Total HTTP requests",
    "method", "handler", // Label names
)
histogram := metrics.NewHistogram("request_duration_seconds", "Request duration (seconds)",
    []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1}, // buckets
    "method", "path",
)

app.GET("/metrics", metrics.Handler()) // Prometheus scrape endpoint
```

## API

### Counter

```go
// Create
counter := metrics.NewCounter(name, help, labelNames...)

// Methods
counter.Add(1)
counter.With("method", "GET", "path", "/users").Add(1) // With labels
```

### Histogram

```go
// Create
histogram := metrics.NewHistogram(name, help, buckets []float64, labelNames...)

// Methods
histogram.Observe(0.042) // In seconds
histogram.With("method", "POST").Observe(0.15)
```

### Gauge

```go
// Create
gauge := metrics.NewGauge(name, help, labelNames...)

// Methods
gauge.Set(42)
gauge.Inc()
gauge.Dec()
gauge.Add(10)
gauge.With("instance", "node-1").Set(100)
```

### Summary

```go
// Create (server-side quantile calculation, not recommended for multi-instance)
summary := metrics.NewSummary(name, help, objectives map[float64]float64, labelNames...)
// objectives: map of target quantiles and error, e.g., 0.5: 0.05 (p50 ± 5%)

Summary.Observe(0.042)
```

### Handler

```go
func Handler() astra.HandlerFunc
```

Registers Prometheus scrape endpoint, returns all registered metric data.

## Config

### Common Histogram Buckets

```go
// HTTP latency
[]float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// Request body size (bytes)
[]float64{100, 500, 1024, 5*1024, 10*1024, 100*1024, 1*1024*1024}

// Database query time
[]float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5}
```

## Complete Example

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/metrics"
)

func main() {
    // Register Prometheus scrape endpoint
    app := astra.New()
    app.GET("/metrics", metrics.Handler())

    // Custom business metrics
    ordersCounter := metrics.NewCounter("orders_total", "Total orders",
        "status", "payment_method",
    )
    orderAmountHist := metrics.NewHistogram("order_amount_total", "Order amount",
        []float64{10, 50, 100, 500, 1000, 5000},
        "currency",
    )

    app.POST("/orders", func(c *astra.Ctx) error {
        // Business logic
        ordersCounter.With("status", "success", "payment_method", "alipay").Add(1)
        orderAmountHist.With("currency", "CNY").Observe(299.00)
        return c.JSON(200, astra.Map{"ok": true})
    })

    app.Run(":8080")
}
```

## Module Dependencies

- `github.com/prometheus/client_golang` — Prometheus Go client

## Notes

- Prometheus endpoint `/metrics` should be restricted in production (intranet or auth)
- Label cardinality should not be too high (e.g., user ID) to avoid metric dimension explosion
- Histogram aggregates client-side; multi-instance Prometheus should use `rate()` or `histogram_quantile()` functions
