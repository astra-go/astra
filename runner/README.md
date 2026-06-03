# Runner Module

任务运行模块，支持多种任务调度后端。

## 特性

- 🎯 **统一接口**：所有后端实现相同的 `Runner` 接口
- 🔌 **可插拔后端**：支持 Cron、Dagu、Gocron、TaskQueue
- 🏷️ **按需编译**：使用 build tags 控制编译哪些后端
- 🔒 **并发安全**：所有实现都是线程安全的

## 支持的后端

| 后端     | Build Tag  | 特点                           |
|---------|-----------|-------------------------------|
| Cron    | `cron`    | 标准 crontab 表达式，轻量级     |
| Dagu    | `dagu`    | DAG 工作流引擎，支持复杂依赖   |
| Gocron  | `gocron`  | Go 原生调度器，无需外部依赖    |
| TaskQueue| `tqrunner` | 基于消息队列的分布式任务执行  |

## 快速开始

### 安装

```bash
go get github.com/astra-go/astra/runner@v2.0.0
```

### 基本用法

所有后端都实现了相同的接口：

```go
type Runner interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Schedule(job *Job) error
    Remove(jobID string) error
}
```

### Cron 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/runner"
)

func main() {
    r := runner.NewCronRunner()
    
    ctx := context.Background()
    
    // 注册定时任务
    err := r.Schedule(&runner.Job{
        ID:      "cleanup",
        Spec:    "0 3 * * *", // 每天凌晨 3 点
        Handler: func(ctx context.Context) error {
            // 清理过期数据
            return nil
        },
    })
    
    r.Start(ctx)
}
```

### Dagu 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/runner"
)

func main() {
    r := runner.NewDaguRunner(runner.DaguConfig{
        DAGsDir: "./dags",
    })
    
    ctx := context.Background()
    r.Start(ctx)
}
```

### Gocron 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/runner"
)

func main() {
    r := runner.NewGocronRunner()
    
    ctx := context.Background()
    
    err := r.Schedule(&runner.Job{
        ID:      "report",
        Spec:    "@hourly",
        Handler: func(ctx context.Context) error {
            // 生成报表
            return nil
        },
    })
    
    r.Start(ctx)
}
```

### TaskQueue 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/runner"
)

func main() {
    r := runner.NewTaskqueueRunner(runner.TaskqueueConfig{
        BrokerURL: "redis://localhost:6379",
    })
    
    ctx := context.Background()
    r.Start(ctx)
}
```

## 编译标签

使用 build tags 控制编译哪些后端，减少二进制体积：

```bash
# 编译所有后端
go build -tags=alltags

# 仅编译 Cron
go build -tags=cron

# 编译多个后端
go build -tags="cron,gocron"
```

## 测试

```bash
# 测试所有后端
go test -tags=alltags ./...

# 测试特定后端
go test -tags=cron ./...
```

## 构造器列表

| 构造器 | Build Tag | 说明 |
|--------|-----------|------|
| `NewCronRunner()` | `cron` | 标准 crontab 调度器 |
| `NewDaguRunner(cfg)` | `dagu` | DAG 工作流引擎 |
| `NewGocronRunner()` | `gocron` | Go 原生调度器 |
| `NewTaskqueueRunner(cfg)` | `tqrunner` | 分布式任务队列 |

## 相关文档

- [ADR-005: 子模块数量上限策略](../docs/adr/ADR-005-module-count-limit.md)
- [架构优化路线图](../docs/architecture-optimization-roadmap.md)

## 许可证

MIT
