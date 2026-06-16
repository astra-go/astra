# LoadBalance — Load Balancing

Multiple load balancing strategy implementations, all implement `Balancer` interface and can be directly swapped.

## Features

- **RoundRobin**: Weighted round-robin, O(1) time complexity
- **Random / WeightedRandom**: Random selection, supports weights
- **LeastConnections**: Least connections
- **P2C (Power of Two Choices)**: Two random + EWMA latency — most commonly used adaptive load balancing strategy
- **ConsistentHash**: Consistent hashing with virtual nodes
- **Health Filtering**: `Filter` function filters instances by condition

## Quick Start

```go
import "github.com/astra-go/astra/loadbalance"

instances := []*discovery.ServiceInstance{
    {Name: "user-svc", Address: "10.0.0.1", Port: 8080, Weight: 100},
    {Name: "user-svc", Address: "10.0.0.2", Port: 8080, Weight: 100},
}

// RoundRobin
lb := loadbalance.NewRoundRobin()
inst, _ := lb.Pick(instances, "")

// P2C (recommended for production)
lb := loadbalance.NewP2C()
lb.RecordSuccess(inst, 50*time.Millisecond)
lb.RecordError(inst, 50*time.Millisecond)
inst, _ := lb.Pick(instances, "")

// Consistent hashing (for session persistence)
lb := loadbalance.NewConsistentHash()
inst, _ := lb.Pick(instances, "user-42") // Same user always hits same instance
```

## API

### Balancer Interface

```go
type Balancer interface {
    Pick(instances []*discovery.ServiceInstance, key string) (*discovery.ServiceInstance, error)
}
// key param: ignored by RoundRobin/Random/P2C/LeastConn; used by ConsistentHash as hash input
```

### Factory Functions

| Function | Description |
|----------|-------------|
| `NewRoundRobin()` | Weighted round-robin |
| `NewRandom()` | Random selection |
| `NewLeastConn()` | Least connections |
| `NewP2C()` | Power of Two Choices + EWMA |
| `NewConsistentHash()` | Consistent hashing |
| `NewWeighted(weights map[string]int)` | Specified weight round-robin |

### Filter

```go
// Filter instances by condition
healthy := loadbalance.Filter(instances, func(i *discovery.ServiceInstance) bool {
    return i.Metadata["status"] != "draining"
})
inst, _ := lb.Pick(healthy, "")
```

### Reporter Interface (P2C/LeastConn Implementation)

```go
// Record success/failure, used for adaptive weight adjustment
lb := loadbalance.NewP2C()
lb.RecordSuccess(inst, elapsed)
lb.RecordError(inst, elapsed)
```

## Config

### ConsistentHash Virtual Node Count

```go
lb := loadbalance.NewConsistentHash(
    loadbalance.WithVirtualNodes(150), // Default 150, recommended range 100-500
)
```

### P2C Parameters

```go
lb := loadbalance.NewP2C(
    loadbalance.WithP2CMinSampleSize(5),     // Min sample size (default 5)
    loadbalance.WithP2CEWMAAlpha(0.3),       // EWMA smoothing factor (default 0.3)
)
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/astra-go/astra/discovery"
    "github.com/astra-go/astra/loadbalance"
)

func main() {
    instances := []*discovery.ServiceInstance{
        {Name: "user-svc", Address: "10.0.0.1", Port: 8080, Weight: 100},
        {Name: "user-svc", Address: "10.0.0.2", Port: 8080, Weight: 100},
        {Name: "user-svc", Address: "10.0.0.3", Port: 8080, Weight: 50},
    }

    // P2C load balancing
    lb := loadbalance.NewP2C()

    // Simulate requests: record success
    for i := 0; i < 10; i++ {
        inst, _ := lb.Pick(instances, "")
        fmt.Printf("Request %d → %s:%d\n", i, inst.Address, inst.Port)
        lb.RecordSuccess(inst, 30*time.Millisecond)
    }

    // Consistent hashing (session persistence)
    ch := loadbalance.NewConsistentHash()
    inst, _ := ch.Pick(instances, "user-42")
    fmt.Printf("user-42 → %s:%d (stable)\n", inst.Address, inst.Port)
}
```

## Module Dependencies

- `github.com/astra-go/astra/discovery` — `ServiceInstance` type definition

## Notes

- P2C's `RecordSuccess`/`RecordError` affect EWMA latency sampling, not actual weights
- ConsistentHash may cause minor request drift when nodes change; this is by design
- `Pick` returns `ErrNoInstances` when `Filter` results in empty list
