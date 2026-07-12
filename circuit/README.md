# Circuit — Circuit Breaker

Thread-safe circuit breaker with count-based and adaptive (statistical) modes for fault tolerance.

## Features

- **Two Breaker Types**:
  - `Breaker` — count-based: opens after N consecutive failures
  - `AdaptiveBreaker` — statistical: trips on error rate or P99 latency thresholds
- **Three States**: Closed → Open → HalfOpen → Closed (standard circuit breaker pattern)
- **Fluent Builders**: `BreakerBuilder` and `AdaptiveBreakerBuilder` for clean configuration
- **State Callbacks**: `OnStateChange` hook for logging/metrics/alerting
- **Thread-Safe**: Safe for concurrent use across goroutines

## Quick Start

### Count-Based Breaker

```go
import "github.com/astra-go/astra/circuit"

// Create with fluent builder
cb := circuit.NewBreakerBuilder("payment-svc").
    WithFailureThreshold(10).
    WithSuccessThreshold(3).
    WithTimeout(30 * time.Second).
    WithOnStateChange(func(name string, from, to circuit.State) {
        log.Printf("circuit %s: %v → %v", name, from, to)
    }).
    Build()

// Use in request
err := cb.Do(func() error {
    return callExternalService()
})
if errors.Is(err, circuit.ErrOpen) {
    // Circuit is open, fail fast
    return fmt.Errorf("service unavailable")
}
```

### Adaptive Breaker (Error Rate + Latency)

```go
import "github.com/astra-go/astra/circuit"

ab := circuit.NewAdaptiveBreakerBuilder("order-svc").
    WithErrorRateThreshold(0.5).    // Trip at 50% error rate
    WithMinRequests(10).             // Min 10 requests before evaluating
    WithLatencyThreshold(2 * time.Second). // Trip at P99 > 2s
    WithWindow(10 * time.Second).    // Rolling window
    Build()

err := ab.Do(func() error {
    return processOrder()
})
```

## State Transitions

```
Closed  → Open      when consecutive failures ≥ Threshold (Breaker)
                    or error rate ≥ ErrorRateThreshold (AdaptiveBreaker)
                    or P99 latency ≥ LatencyThreshold (AdaptiveBreaker)

Open    → HalfOpen  after Timeout elapses

HalfOpen → Closed   when HalfOpenSuccesses succeed
HalfOpen → Open     on any failure
```

## API

### Breaker (Count-Based)

| Method | Description |
|--------|-------------|
| `New(cfg Config) *Breaker` | Create with Config struct |
| `NewBreakerBuilder(name).Build()` | Create with fluent builder |
| `Do(fn func() error) error` | Execute function through breaker |
| `State() State` | Get current state |
| `Counts() (requests, failures, successes int64)` | Get counters |
| `Reset()` | Force reset to Closed state |

### Breaker Config

| Field | Default | Description |
|-------|---------|-------------|
| `Name` | — | Identifier for logs/metrics |
| `Threshold` | `5` | Consecutive failures before opening |
| `Timeout` | `30s` | Time in Open state before HalfOpen |
| `HalfOpenSuccesses` | `2` | Successes to close from HalfOpen |
| `HalfOpenMaxRequests` | `1` | Concurrent probes in HalfOpen |
| `OnStateChange` | `nil` | Callback on state transition |

### AdaptiveBreaker (Statistical)

| Method | Description |
|--------|-------------|
| `NewAdaptive(cfg AdaptiveConfig) *AdaptiveBreaker` | Create with Config struct |
| `NewAdaptiveBreakerBuilder(name).Build()` | Create with fluent builder |
| `Do(fn func() error) error` | Execute function through breaker |
| `State() State` | Get current state |
| `Stats() (requests, errors int64, errorRate float64)` | Get rolling stats |
| `LatencyP99() time.Duration` | Get P99 latency (if LatencyThreshold enabled) |
| `Reset()` | Force reset to Closed state |

### Adaptive Config

| Field | Default | Description |
|-------|---------|-------------|
| `Name` | — | Identifier for logs/metrics |
| `ErrorRateThreshold` | `0.5` | Error rate (0.0-1.0) that trips circuit |
| `MinRequests` | `10` | Min requests in window before evaluating |
| `LatencyThreshold` | `0` (disabled) | P99 latency threshold to trip circuit |
| `Window` | `10s` | Rolling stats window duration |
| `BucketCount` | `10` | Time buckets within window |
| `LatencySampleSize` | `256` | Ring buffer size for P99 computation |
| `Timeout` | `30s` | Time in Open state before HalfOpen |
| `HalfOpenSuccesses` | `2` | Successes to close from HalfOpen |
| `HalfOpenMaxRequests` | `1` | Concurrent probes in HalfOpen |
| `OnStateChange` | `nil` | Callback on state transition |

## Complete Example

```go
package main

import (
    "errors"
    "fmt"
    "log"
    "time"

    "github.com/astra-go/astra/circuit"
)

func main() {
    // Create breaker with monitoring
    cb := circuit.NewBreakerBuilder("external-api").
        WithFailureThreshold(5).
        WithTimeout(20 * time.Second).
        WithSuccessThreshold(2).
        WithOnStateChange(func(name string, from, to circuit.State) {
            log.Printf("[ALERT] circuit %s: %s → %s", name, from, to)
        }).
        Build()

    // Simulate requests
    for i := 0; i < 20; i++ {
        err := cb.Do(func() error {
            // Simulate failing external call
            if i < 10 {
                return errors.New("connection refused")
            }
            return nil
        })

        if errors.Is(err, circuit.ErrOpen) {
            fmt.Printf("Request %d: circuit open, fail fast\n", i)
        } else if err != nil {
            fmt.Printf("Request %d: error: %v\n", i, err)
        } else {
            fmt.Printf("Request %d: success\n", i)
        }

        time.Sleep(100 * time.Millisecond)
    }

    // Check final state
    fmt.Printf("Final state: %s\n", cb.State())
}
```

## When to Use Which

| Scenario | Recommended |
|----------|-------------|
| Simple threshold, low traffic | `Breaker` |
| High traffic, need statistical trip | `AdaptiveBreaker` |
| Latency-sensitive service | `AdaptiveBreaker` with `LatencyThreshold` |
| External API with known failure patterns | `Breaker` with `Threshold=5-10` |

## Notes

- `ErrOpen` is returned when circuit is open — callers should fail fast
- `OnStateChange` is called in a goroutine to avoid blocking state transitions
- Adaptive breaker uses a rolling window with time buckets for stats
- P99 latency is sampled via a ring buffer (configurable size)
- Both breakers are thread-safe; safe to share across goroutines
