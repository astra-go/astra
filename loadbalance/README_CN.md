# LoadBalance — 负载均衡

多种负载均衡策略实现，全部实现 `Balancer` 接口，可直接替换。

## 特性

- **RoundRobin**：加权轮询，O(1) 时间复杂度
- **Random / WeightedRandom**：随机选择，支持权重
- **LeastConnections**：最少连接
- **P2C（Power of Two Choices）**：两个随机 + EWMA 延迟 — 最常用的自适应负载均衡策略
- **ConsistentHash**：一致性哈希，支持虚拟节点
- **健康过滤**：`Filter` 函数按条件过滤实例

## 快速开始

```go
import "github.com/astra-go/astra/loadbalance"

instances := []*discovery.ServiceInstance{
    {Name: "user-svc", Address: "10.0.0.1", Port: 8080, Weight: 100},
    {Name: "user-svc", Address: "10.0.0.2", Port: 8080, Weight: 100},
}

// 轮询
lb := loadbalance.NewRoundRobin()
inst, _ := lb.Pick(instances, "")

// P2C（生产环境推荐）
lb := loadbalance.NewP2C()
lb.RecordSuccess(inst, 50*time.Millisecond)
lb.RecordError(inst, 50*time.Millisecond)
inst, _ := lb.Pick(instances, "")

// 一致性哈希（用于 Session 粘性）
lb := loadbalance.NewConsistentHash()
inst, _ := lb.Pick(instances, "user-42") // 同一用户始终命中同一实例
```

## API

### Balancer 接口

```go
type Balancer interface {
    Pick(instances []*discovery.ServiceInstance, key string) (*discovery.ServiceInstance, error)
}
// key 参数：RoundRobin/Random/P2C/LeastConn 下忽略；ConsistentHash 用作哈希输入
```

### 工厂函数

| 函数 | 说明 |
|----------|-------------|
| `NewRoundRobin()` | 加权轮询 |
| `NewRandom()` | 随机选择 |
| `NewLeastConn()` | 最少连接 |
| `NewP2C()` | Power of Two Choices + EWMA |
| `NewConsistentHash()` | 一致性哈希 |
| `NewWeighted(weights map[string]int)` | 指定权重轮询 |

### Filter

```go
// 按条件过滤实例
healthy := loadbalance.Filter(instances, func(i *discovery.ServiceInstance) bool {
    return i.Metadata["status"] != "draining"
})
inst, _ := lb.Pick(healthy, "")
```

### Reporter 接口（P2C/LeastConn 实现）

```go
// 记录成功/失败，用于自适应权重调整
lb := loadbalance.NewP2C()
lb.RecordSuccess(inst, elapsed)
lb.RecordError(inst, elapsed)
```

## 配置

### 一致性哈希虚拟节点数

```go
lb := loadbalance.NewConsistentHash(
    loadbalance.WithVirtualNodes(150), // 默认 150，推荐范围 100-500
)
```

### P2C 参数

```go
lb := loadbalance.NewP2C(
    loadbalance.WithP2CMinSampleSize(5),     // 最小采样数（默认 5）
    loadbalance.WithP2CEWMAAlpha(0.3),       // EWMA 平滑因子（默认 0.3）
)
```

## 完整示例

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

    // P2C 负载均衡
    lb := loadbalance.NewP2C()

    // 模拟请求：记录成功
    for i := 0; i < 10; i++ {
        inst, _ := lb.Pick(instances, "")
        fmt.Printf("Request %d → %s:%d\n", i, inst.Address, inst.Port)
        lb.RecordSuccess(inst, 30*time.Millisecond)
    }

    // 一致性哈希（Session 粘性）
    ch := loadbalance.NewConsistentHash()
    inst, _ := ch.Pick(instances, "user-42")
    fmt.Printf("user-42 → %s:%d (stable)\n", inst.Address, inst.Port)
}
```

## 模块依赖

- `github.com/astra-go/astra/discovery` — `ServiceInstance` 类型定义

## 注意事项

- P2C 的 `RecordSuccess`/`RecordError` 影响 EWMA 延迟采样，不影响实际权重
- 一致性哈希在节点变化时可能产生少量请求漂移；这是设计如此
- `Pick` 在 `Filter` 结果为空时返回 `ErrNoInstances`