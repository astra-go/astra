# Astra 本地开发环境 - 快速参考

## 🚀 一分钟快速开始

```bash
# 启动最小环境（PostgreSQL + Redis）
./scripts/dev.sh start

# 查看连接信息
./scripts/dev.sh status
```

---

## 📋 常用命令

| 命令 | 说明 |
|------|------|
| `./scripts/dev.sh start` | 启动最小环境 |
| `./scripts/dev.sh start full` | 启动所有服务 |
| `./scripts/dev.sh status` | 查看服务状态和连接信息 |
| `./scripts/dev.sh health` | 检查服务健康状态 |
| `./scripts/dev.sh logs postgres` | 查看服务日志 |
| `./scripts/dev.sh restart redis` | 重启服务 |
| `./scripts/dev.sh stop` | 停止（保留数据） |
| `./scripts/dev.sh reset` | 完全重置（删除所有数据） |

---

## 🔌 服务连接

### 核心服务（Minimal）
```bash
# PostgreSQL
postgresql://astra_dev:dev123@localhost:5432/astra_dev

# Redis
redis://:dev123@localhost:6379
```

### Web UI（Observability / Full）
- **Grafana**: http://localhost:3000 (admin/admin)
- **Jaeger**: http://localhost:16686
- **Prometheus**: http://localhost:9090
- **RabbitMQ**: http://localhost:15672 (astra_dev/dev123)
- **Consul**: http://localhost:8500

---

## 🎛️ 配置档次

| 档次 | 服务 | 启动命令 |
|------|------|----------|
| **Minimal** | PostgreSQL + Redis | `./scripts/dev.sh start` |
| **Observability** | +Prometheus+Grafana+Jaeger | `./scripts/dev.sh start observability` |
| **Full** | 所有服务（15+） | `./scripts/dev.sh start full` |

---

## 🔧 常见问题

### 端口冲突
```bash
# 检查端口占用
lsof -i :5432
./scripts/dev.sh status
```

### 服务启动失败
```bash
# 查看日志
./scripts/dev.sh logs [服务名]

# 重启服务
./scripts/dev.sh restart [服务名]

# 完全重置
./scripts/dev.sh reset
```

### 数据重置
```bash
# 删除所有数据并重新初始化
./scripts/dev.sh reset
./scripts/dev.sh start
```

---

## 📚 详细文档

完整文档请参考: `deploy/README.md`

---

**提示**: 所有服务使用统一的开发凭证  
**用户名**: `astra_dev` | **密码**: `dev123`
