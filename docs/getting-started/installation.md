# 安装

## 要求

- Go **1.22** 或更高版本

## 核心模块

```bash
go get github.com/astra-go/astra@latest
```

## 扩展模块（按需安装）

```bash
# OpenTelemetry 可观测性
go get github.com/astra-go/astra/otel@latest

# gRPC 双栈
go get github.com/astra-go/astra/grpc@latest

# GORM ORM
go get github.com/astra-go/astra/orm@latest

# Redis
go get github.com/astra-go/astra/redis@latest

# 消息队列（选择所需后端）
go get github.com/astra-go/astra/mq/nats@latest
go get github.com/astra-go/astra/mq/kafka@latest

# 服务发现（选择所需后端）
go get github.com/astra-go/astra/discovery/nacos@latest
go get github.com/astra-go/astra/discovery/k8s@latest

# HTTP/3 依赖（quic-go 已内置于核心模块）
# RunQUIC 无需额外安装
```

## 验证安装

```bash
go run . -version
# Astra v1.0.0 (go1.25.9, linux/amd64)
```
