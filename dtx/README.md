# DTX — Distributed Transactions

Provides Saga and TCC distributed transaction patterns for solving cross-service data consistency.

## Features

- **Saga Pattern**: Forward compensation pattern — executes steps in order; on any failure, executes compensation in reverse order
- **TCC Pattern**: Try-Confirm-Cancel three-phase — Try reserves resources, Confirm commits, Cancel rolls back
- **Auto Compensation**: On failure, auto-executes registered compensation logic; no manual rollback needed
- **Transaction Chain Propagation**: Passes transaction context through Context between services

## Quick Start

### Saga Pattern

Suitable for long business flows with definable compensation operations (e.g., order creation + inventory deduction + payment):

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
    // Compensation for all executed steps has been triggered automatically
    log.Printf("saga failed: %v", err)
}
```

### TCC Pattern

Suitable for resource reservation scenarios (e.g., account freeze, inventory freeze), where forward/reverse operations are deterministic:

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
    // Try failed, Cancel was called automatically
}
```

## API

### Saga

```go
func NewSaga(name string) *Saga

func (s *Saga) AddStep(step *SagaStep) *Saga

type SagaStep struct {
    Name       string
    Action     func(ctx context.Context) error // Forward operation
    Compensate func(ctx context.Context) error // Compensation operation
}

func (s *Saga) Execute(ctx context.Context) error
```

### TCC

```go
func NewTCC(name string) *TCC

func (t *TCC) AddStep(step *TCCStep) *TCC

type TCCStep struct {
    Name   string
    Try    func(ctx context.Context) error    // Reserve resources
    Confirm func(ctx context.Context) error  // Confirm (executes after Try succeeds)
    Cancel  func(ctx context.Context) error   // Cancel (executes when Try fails or times out)
}

func (t *TCC) Execute(ctx context.Context) error
```

### DtxOption (Advanced Options)

```go
// Set max concurrent step execution
dtx.WithConcurrency(5)

// Set per-step timeout
dtx.WithStepTimeout(10 * time.Second)

// Record transaction log (for crash recovery)
dtx.WithPersistence(store)
```

## Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Concurrency` | `int` | `1` | Max concurrent step execution |
| `StepTimeout` | `time.Duration` | `30s` | Per-step timeout |

## Complete Example

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

    // Saga example: create order
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

## Module Dependencies

- `github.com/astra-go/astra/discovery` — Service discovery
- `github.com/astra-go/astra/lock` — Distributed lock (for TCC concurrency control)

## Notes

- **Saga Compensation Idempotency**: compensation operations should be idempotent (same result when executed multiple times) to avoid issues from network retries
- **TCC Empty Rollback**: when Try hasn't executed but Cancel is called, should detect and skip; when implementing `TCCStep.Cancel`, check resource state
- **悬挂问题 (Suspension)**: when Try times out without returning, both Confirm and Cancel may be called; use a state machine to avoid this
- Not recommended to include long-duration operations like human approval in Saga/TCC steps
