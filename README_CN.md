# ⭐ Astra — 面向星辰的 Go Web 框架

> **Astra** — 面向星辰的新一代 Go Web 框架。

[![Go Reference](https://pkg.go.dev/badge/github.com/astra-go/astra.svg)](https://pkg.go.dev/github.com/astra-go/astra)
[![Go Report Card](https://goreportcard.com/badge/github.com/astra-go/astra)](https://goreportcard.com/report/github.com/astra-go/astra)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/astra-go/astra)](go.mod)
[![Version: v1.0.5](https://img.shields.io/badge/version-v1.0.5-blue.svg)](https://github.com/astra-go/astra/releases/tag/v1.0.5)

Astra 是一个**现代化、高性能**的 Go Web 框架，凝聚了 Gin、Echo、go-zero、Beego 和 Kratos 的最佳实践，具有轻量核心和丰富的扩展生态。

---

## ✨ 核心特性

- **🚀 高性能** — radix-tree 路由、零分配 Context 池、优化 JSON 序列化（Sonic）
- **🧩 组件架构** — 统一 `Component` 接口，模块即插即用
- **🔧 丰富的内置中间件** — CORS、CSRF、Recovery、Logger、Compress、RateLimit、JWT、API Key、IP Filter 等
- **📦 全功能扩展** — ORM、缓存、消息队列、配置中心、服务发现、gRPC、Session、分布式锁、对象存储等
- **🔌 插件生态** — Prometheus 指标、OpenTelemetry、健康检查、分布式事务（Saga/TCC）
- **⚙️ 灵活配置** — 多源配置（文件、环境变量、etcd、Nacos、Apollo），支持热更新
- **🔒 企业级安全** — JWT 认证（含撤销）、API Key、签名校验、多级缓存
- **🌐 多协议支持** — HTTP/1.1、HTTP/2、HTTP/3（QUIC）、WebSocket、gRPC
- **📝 约定优于配置 → 可扩展** — 零配置默认，开箱即用，按需开启高级特性

---

## 📦 安装指南

### 快速安装（推荐）

```bash
# 安装最新版本
go get github.com/astra-go/astra@v1.0.5

# 或安装指定模块
go get github.com/astra-go/astra/cache@v1.0.5
go get github.com/astra-go/astra/orm@v1.0.5
```

### 版本信息

| 组件 | 版本 | 安装命令 |
|--------|------|----------|
| **核心** | `v1.0.5` | `go get github.com/astra-go/astra@v1.0.5` |
| **缓存** | `v1.0.5` | `go get github.com/astra-go/astra/cache@v1.0.5` |
| **ORM** | `v1.0.5` | `go get github.com/astra-go/astra/orm@v1.0.5` |
| **所有模块** | `v1.0.5` | 参见 [docs/installation.md](docs/installation.md) |

> 💡 **提示：** 所有子模块独立版本管理。使用 `@v1.0.5` 确保 monorepo 版本一致。

---

## 🚀 快速开始

## 🏗️ 项目结构

```
astra/
├── 📄 app.go           # 核心 App — 路由、中间件、生命周期
├── 📄 router.go         # radix-tree 路由
├── 📄 context.go        # 请求上下文（零分配设计）
├── 📄 group.go          # 路由分组
├── 📄 module.go         # 组件注册系统（v1 兼容）
├── 📄 lifecycle.go      # 启动/停止钩子
├── 📄 options.go        # App 配置选项
├── 📄 errors.go         # HTTP 错误和业务错误
│
├── 🧩 middleware/       # 内置中间件
│   ├── cors.go          # 跨域
│   ├── csrf.go          # CSRF 防护
│   ├── recovery.go      # Panic 恢复
│   ├── logger.go        # 访问日志
│   ├── compress.go      # Gzip 压缩
│   ├── secure.go        # 安全响应头
│   ├── csp.go           # Content Security Policy
│   ├── timeout.go       # 请求超时
│   ├── requestid.go     # 请求 ID
│   ├── sanitize.go      # 输入清理
│   ├── marketplace/     # 中间件市场
│   └── security/        # 安全中间件（JWT/APIKey/RateLimit/Tenant 等）
│
├── 📦 cache/            # 缓存抽象（memory/redis/memcached）
├── 📦 orm/              # GORM 集成（读写分离、分库分表、事务传播）
├── 📦 mq/               # 消息队列（Kafka/RabbitMQ/RocketMQ/MQTT/NATS/Pulsar）
├── 📦 config/           # 配置管理（yaml/env/etcd/nacos/apollo）
├── 📦 session/          # Session 管理（Redis）
├── 📦 auth/             # 认证（OAuth2、RBAC）
├── 📦 storage/          # 对象存储（S3/OSS/COS）
├── 📦 discovery/        # 服务发现（consul/etcd/k8s/nacos）
├── 📦 grpc/             # gRPC 集成
├── 📦 notify/           # 通知（邮件/短信/推送）
├── 📦 search/           # 搜索（Elasticsearch）
├── 📦 client/           # HTTP/gRPC 客户端
├── 📦 taskqueue/        # 任务队列
├── 📦 dtx/              # 分布式事务（Saga/TCC）
├── 📦 lock/             # 分布式锁（etcd/redis）
├── 📦 loadbalance/      # 负载均衡
├── 📦 runner/           # 后台任务调度（cron/dagu/gocron）
├── 📦 stream/           # 流式处理
├── 📦 rule/             # 规则引擎（Lua）
├── 📦 health/           # 健康检查
├── 📦 metrics/          # Prometheus 指标
├── 📦 otel/             # OpenTelemetry
├── 📦 cron/             # 定时任务
├── 📦 di/               # 依赖注入
├── 📦 alert/            # 告警系统
├── 📦 contract/         # 接口契约
├── 📦 binding/          # 请求绑定
├── 📦 pagination/       # 分页
├── 📦 render/           # 模板渲染
├── 📦 validate/         # 数据校验
├── 📦 websocket/        # WebSocket
├── 📦 quic/             # QUIC/HTTP3
├── 📦 log/              # 日志
├── 📦 i18n/             # 国际化
├── 📦 upload/           # 文件上传
├── 📦 mongodb/          # MongoDB 集成
├── 📦 netengine/        # 网络引擎
│
├── 📂 examples/         # 示例项目
├── 📂 deploy/           # 部署配置（Docker/Helm/Kustomize）
├── 📂 docs/             # 文档
└── 📂 scripts/          # 构建脚本
```

## 🚀 快速开始

### 安装

```bash
go get github.com/astra-go/astra@latest
```

### 最小示例

```go
package main

import (
    "log"
    "github.com/astra-go/astra"
)

func main() {
    app := astra.New()

    app.GET("/hello/:name", func(c *astra.Ctx) error {
        return c.JSON(200, astra.Map{
            "message": "Hello, " + c.Param("name"),
        })
    })

    log.Fatal(app.Run(":8080"))
}
```

运行：

```bash
go run main.go
# 访问 http://localhost:8080/hello/Astra
# 输出: {"message":"Hello, Astra"}
```

### 完整应用

```go
package main

import (
    "log"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
)

func main() {
    // 创建 App
    app := astra.New()

    // 全局中间件
    app.Use(middleware.Logger())
    app.Use(middleware.Recovery())
    app.Use(middleware.CORS("https://example.com"))

    // 路由分组
    api := app.Group("/api/v1")
    {
        api.GET("/users", listUsers)
        api.POST("/users", createUser)
        api.GET("/users/:id", getUser)
    }

    // 静态文件
    app.Static("/static", "./public")

    // 启动服务（支持优雅关闭）
    log.Fatal(app.Run(":8080"))
}

func listUsers(c *astra.Ctx) error {
    return c.JSON(200, astra.Map{"users": []string{"Alice", "Bob"}})
}

func createUser(c *astra.Ctx) error {
    var user struct {
        Name  string `json:"name" validate:"required"`
        Email string `json:"email" validate:"required,email"`
    }
    if err := c.BindJSON(&user); err != nil {
        return err
    }
    return c.JSON(201, astra.Map{"id": 1, "name": user.Name})
}

func getUser(c *astra.Ctx) error {
    id := c.Param("id")
    return c.JSON(200, astra.Map{"id": id, "name": "Alice"})
}
```

## 📘 文档

完整文档和教程在 [`docs/`](docs/) 目录下：

| 文档 | 说明 |
|----------|-------------|
| [安装指南](docs/getting-started.md) | 安装和版本选择 |
| [快速开始](docs/quick-start.md) | 三步快速上手 |
| [核心概念](docs/core-concepts.md) | App、Context、路由、中间件 |
| [配置管理](docs/configuration.md) | 配置管理和热更新 |
| [中间件](docs/middleware.md) | 内置和自定义中间件 |
| [数据库 ORM](docs/database-orm.md) | GORM 集成、读写分离、事务 |
| [缓存](docs/caching.md) | 统一多后端缓存接口 |
| [消息队列](docs/message-queue.md) | 多 broker 消息中间件 |
| [微服务](docs/microservices.md) | 服务发现、gRPC、分布式事务 |
| [部署](docs/deployment.md) | Docker、K8s、Helm |
| [最佳实践](docs/best-practices.md) | 工程建议 |

## 📦 子模块概览

每个子模块是独立的 Go module，有自己的版本：

| 模块 | 路径 | 说明 |
|--------|------|-------------|
| **cache** | `astra/cache` | 缓存抽象：memory/redis/memcached |
| **orm** | `astra/orm` | GORM 集成：读写分离、分库分表、事务 |
| **mq** | `astra/mq` | 消息队列：Kafka/RabbitMQ/RocketMQ/MQTT/NATS/Pulsar |
| **config** | `astra/config` | 配置管理：file/env/etcd/Nacos/Apollo |
| **session** | `astra/session` | Session 管理（Redis 后端） |
| **auth** | `astra/auth` | 认证：OAuth2、RBAC |
| **storage** | `astra/storage` | 对象存储：S3/OSS/COS |
| **discovery** | `astra/discovery` | 服务发现：Consul/etcd/K8s/Nacos |
| **grpc** | `astra/grpc` | gRPC 服务端/客户端 |
| **notify** | `astra/notify` | 通知：邮件/短信/推送 |
| **search** | `astra/search` | 搜索：Elasticsearch |
| **client** | `astra/client` | HTTP/gRPC 客户端 |
| **taskqueue** | `astra/taskqueue` | 任务队列 |
| **dtx** | `astra/dtx` | 分布式事务：Saga/TCC |
| **lock** | `astra/lock` | 分布式锁：etcd/redis |
| **loadbalance** | `astra/loadbalance` | 负载均衡 |
| **runner** | `astra/runner` | 后台任务调度 |
| **stream** | `astra/stream` | 流式处理 |
| **rule** | `astra/rule` | 规则引擎（Lua） |
| **health** | `astra/health` | 健康检查 |
| **metrics** | `astra/metrics` | Prometheus 指标 |
| **otel** | `astra/otel` | OpenTelemetry |
| **cron** | `astra/cron` | 定时任务 |
| **di** | `astra/di` | 依赖注入 |
| **alert** | `astra/alert` | 告警系统 |
| **mongodb** | `astra/mongodb` | MongoDB 集成 |
| **netengine** | `astra/netengine` | 网络引擎 |

## 📄 许可证

[MIT 许可证](LICENSE)

---

**Astra** — 面向星辰。⭐