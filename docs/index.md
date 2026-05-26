# Astra — 星辰 Go Web 框架

Astra 是面向 Go 开发者的高性能、全功能 Web 框架，汇聚 Gin、Echo、Kratos、go-zero 等主流框架精华。

## 特色功能

| 分类 | 功能 |
|------|------|
| **网络层** | Reactor 引擎（epoll/kqueue）、HTTP/3 QUIC |
| **认证** | JWT · API Key · RBAC · OAuth2/OIDC |
| **可观测** | OpenTelemetry traces + metrics + logs · Prometheus |
| **数据层** | GORM · Redis · MongoDB · ClickHouse · Elasticsearch |
| **消息队列** | NATS · Kafka · RabbitMQ · Redis Streams · SQS · Pulsar |
| **微服务** | gRPC 双栈 · 服务发现 · 负载均衡 · 熔断 · Saga 事务 |
| **基础设施** | 配置中心 · 分布式锁 · Session · 对象存储 · 告警引擎 |

## 快速开始

```go
package main

import "github.com/astra-go/astra"

func main() {
    app := astra.New()
    app.GET("/ping", func(c *astra.Context) error {
        return c.JSON(200, astra.H{"message": "pong"})
    })
    app.Run(":8080")
}
```

## 文档导航

- [快速开始](getting-started/installation.md) — 安装、第一个应用
- [API 参考](api/core.md) — 完整 API 文档
- [版本策略](versioning.md) — SemVer 规则、支持周期
- [迁移指南](migration/overview.md) — 跨版本升级指南
- [Changelog](changelog.md) — 完整变更历史

## 版本状态

| 版本 | 状态 | Go 要求 | 维护至 |
|------|------|---------|--------|
| **v1.0** (latest) | :white_check_mark: 积极维护 | ≥ 1.22 | 2027-04 |
| v0.10 | :wrench: 安全修复 | ≥ 1.22 | 2026-10 |
| v0.9  | :wrench: 安全修复 | ≥ 1.21 | 2026-08 |
| ≤ v0.8 | :x: 不再维护 | — | — |
