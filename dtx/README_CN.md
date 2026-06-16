# DTX — 分布式事务

提供 Saga 和 TCC 分布式事务模式，解决跨服务数据一致性。

## 特性

- **Saga 模式**：正向补偿模式 — 按顺序执行步骤；任意步骤失败则逆序执行补偿
- **TCC 模式**：Try-Confirm-Cancel 三阶段 — Try 预留资源、Confirm 提交、Cancel 回滚
- **自动补偿**：失败时自动执行已注册的补偿逻辑；无需手动回滚
- **事务链传播**：通过 Context 在服务间传递事务上下文

## 快速开始

### Saga 模式

适用于长业务流，有明确可定义的补偿操作（如订单创建 + 库存扣减 + 支付）：

```go
saga := dtx.NewSaga("create-order").
    AddStep(&dtx.SagaStep{
        Name:       "deduct-inventory",
        Action:     func(ctx context.Context) error {
            return inventory.Deduct(ctx, 1)
        },
        Compensate: func(ctx context.Context) error {
            return inventory.Restore(ctx, 1)
        },
    }).
    AddStep(&dtx.SagaStep{
        Name:       "create-payment",
        Action:     func(ctx context.Context) error {
            return payment.Create(ctx, amount)
        },
        Compensate: func(ctx context.Context) error {
            return payment.Refund(ctx, amount)
        },
    })

if err := saga.Execute(ctx); err != nil {
    // 已执行步骤的补偿已自动触发
    log.Printf("saga failed: %v", err)
}
```

### TCC 模式

适用于资源预留场景（如账户冻结、库存冻结），正向/反向操作确定性高：

```go
tcc := dtx.NewTCC("transfer").
    AddStep(&dtx.TCCStep{
        Name:   "freeze-account",
        Try:    func(ctx context.Context) error {
            return account.Freeze(ctx, fromUser, 100)
        },
        Confirm: func(ctx context.Context) error {
            return account.Deduct(ctx, fromUser, 100)
        },
        Cancel: func(ctx context.Context) error {
            return account.Unfreeze(ctx, fromUser, 100)
        },
    })

if err := tcc.Execute(ctx); err != nil {
    // Try 失败，Cancel 已自动调用
}
```

## API

### Saga

```go
func NewSaga(name string) *Saga

func (s *Saga) AddStep(step *SagaStep) *Saga

type SagaStep struct {
    Name       string
    Action     func(ctx context.Context) error // 正向操作
    Compensate func(ctx context.Context) error // 补偿操作
}

func (s *Saga) Execute(ctx context.Context) error
```

### TCC

```go
func NewTCC(name string) *TCC

func (t *TCC) AddStep(step *TCCStep) *TCC

type TCCStep struct {
    Name   string
    Try    func(ctx context.Context) error    // 预留资源
    Confirm func(ctx context.Context) error  // 确认（Try 成功后执行）
    Cancel  func(ctx context.Context) error   // 取消（Try 失败或超时时执行）
}

func (t *TCC) Execute(ctx context.Context) error
```

### DtxOption（高级选项）

```go
// 设置最大并发步骤数
dtx.WithConcurrency(5)

// 设置每步骤超时
dtx.WithStepTimeout(10 * time.Second)

// 记录事务日志（用于崩溃恢复）
dtx.WithPersistence(store)
```

## 配置

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `Concurrency` | `int` | `1` | 最大并发步骤数 |
| `StepTimeout` | `time.Duration` | `30s` | 每步骤超时 |

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "github.com/astra-go/astra/dtx"
)

type Order struct{ Amount float64 }

func main() {
    ctx := context.Background()

    // Saga 示例：创建订单
    saga := dtx.NewSaga("create-order").
        AddStep(&dtx.SagaStep{
            Name: "validate-inventory",
            Action: func(ctx context.Context) error {
                fmt.Println("Validating inventory")
                return nil
            },
            Compensate: func(ctx context.Context) error {
                fmt.Println("Releasing inventory validation")
                return nil
            },
        }).
        AddStep(&dtx.SagaStep{
            Name: "create-order",
            Action: func(ctx context.Context) error {
                fmt.Println("Creating order")
                return nil
            },
            Compensate: func(ctx context.Context) error {
                fmt.Println("Canceling order")
                return nil
            },
        })

    if err := saga.Execute(ctx); err != nil {
        fmt.Printf("Transaction failed: %v\n", err)
    } else {
        fmt.Println("Transaction succeeded")
    }
}
```

## 模块依赖

- `github.com/astra-go/astra/discovery` — 服务发现
- `github.com/astra-go/astra/lock` — 分布式锁（用于 TCC 并发控制）

## 注意事项

- **Saga 补偿幂等性**：补偿操作应幂等（多次执行结果相同），避免网络重试导致问题
- **TCC 空回滚**：Try 未执行但调用了 Cancel 时应检测并跳过；实现 `TCCStep.Cancel` 时检查资源状态
- **悬挂问题**：Try 超时未返回时，Confirm 和 Cancel 可能都被调用；使用状态机避免此问题
- 不建议在 Saga/TCC 步骤中包含人工审批等长耗时操作