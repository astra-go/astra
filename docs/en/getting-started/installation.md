# Installation

## Requirements

- Go **1.22** or higher

## Core Module

```bash
go get github.com/astra-go/astra@latest
```

## Extension Modules (install as needed)

```bash
# OpenTelemetry observability
go get github.com/astra-go/astra/otel@latest

# gRPC dual-stack
go get github.com/astra-go/astra/grpc@latest

# GORM ORM
go get github.com/astra-go/astra/orm@latest

# Redis
go get github.com/astra-go/astra/redis@latest

# Message queue (choose the backend you need)
go get github.com/astra-go/astra/mq/nats@latest
go get github.com/astra-go/astra/mq/kafka@latest

# Service discovery (choose the backend you need)
go get github.com/astra-go/astra/discovery/nacos@latest
go get github.com/astra-go/astra/discovery/k8s@latest

# HTTP/3 dependency (quic-go is already bundled in the core module)
# RunQUIC requires no additional installation
```

## Verify Installation

```bash
go run . -version
# Astra v1.0.0 (go1.25.9, linux/amd64)
```
