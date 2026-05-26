## 完整示例

> 所有示例位于 `examples/` 目录，无外部服务依赖（SQLite 内存数据库、in-process broker），直接 `go run main.go` 即可运行。

| 示例 | 目录 | 演示内容 |
|------|------|---------|
| 基础功能 | `examples/basic` | 中间件、路径/Query 参数、JSON 绑定、JWT 路由、限流、SSE、生命周期钩子 |
| REST CRUD | `examples/crud` | 内存 Repository、分页、输入验证、`v1`（限流）路由 |
| JWT 认证 | `examples/jwt` | 注册 / 登录、Access + Refresh Token、`/auth/refresh`、受保护路由 `/api/me` |
| WebSocket | `examples/websocket` | 多房间 Hub-per-room、广播、join/leave 事件、`/api/rooms` 统计 |
| 消息队列 | `examples/mq` | Producer / Consumer 接口模式（in-process broker，可一行切换 RabbitMQ / Kafka） |
| ORM | `examples/orm` | GORM + SQLite 内存库、软删除、恢复、分页 |
| 缓存 | `examples/cache` | Read-through `GetOrSet`、更新时缓存失效、手动驱逐、命中率统计 |

---

### 基础示例（`examples/basic`）

```bash
cd examples/basic && go run main.go
```

演示：全局中间件（RequestID / Logger / Recovery / CORS / Timeout）、路径参数、Query 参数、JSON 绑定、SSE 推送、JWT 保护路由、限流路由组、生命周期钩子。

```bash
curl http://localhost:8080/ping
curl http://localhost:8080/hello/Astra
curl "http://localhost:8080/search?q=golang&page=2"
curl -X POST http://localhost:8080/echo \
  -H "Content-Type: application/json" \
  -d '{"framework":"astra","stars":9999}'
curl -N http://localhost:8080/events
```

---

### CRUD 示例（`examples/crud`）

```bash
cd examples/crud && go run main.go
```

用户 REST API，内存 Repository，分页（`?page=1&size=20`），输入验证，限流路由组。

```bash
curl http://localhost:8080/api/v1/users
curl -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{"name":"Charlie","email":"charlie@example.com"}'
curl http://localhost:8080/api/v1/users/1
curl -X PUT  http://localhost:8080/api/v1/users/1 \
  -H "Content-Type: application/json" -d '{"name":"Charles"}'
curl -X DELETE http://localhost:8080/api/v1/users/2
```

---

### JWT 认证示例（`examples/jwt`）

```bash
cd examples/jwt && go run main.go
```

完整的 JWT 认证流程：注册 → 登录 → 刷新令牌 → 访问受保护资源。
- Access Token 有效期 15 分钟，Refresh Token 有效期 7 天
- 使用 `middleware.GenerateJWT` 签发，`middleware.JWT` 中间件验证

```bash
# 注册
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice","email":"alice@example.com","password":"secret"}'

# 登录（返回 access_token + refresh_token）
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"demo@example.com","password":"password123"}' \
  | jq -r .access_token)

# 访问受保护路由
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/me

# 刷新 Access Token
curl -X POST http://localhost:8080/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<your_refresh_token>"}'
```

---

### WebSocket 示例（`examples/websocket`）

```bash
cd examples/websocket && go run main.go
```

多房间实时聊天：每个房间独享一个 `Hub`，支持 join/leave 事件广播，HTTP 端点查看房间统计。

```bash
# 查看活跃房间
curl http://localhost:8080/api/rooms

# 用 websocat 连接（两个终端模拟两个客户端）
websocat ws://localhost:8080/ws?room=general
# 发送消息（支持纯文本或 JSON）
# {"text":"hello room!"}
```

---

### 消息队列示例（`examples/mq`）

```bash
cd examples/mq && go run main.go
```

演示 Producer / Consumer 接口模式。内置 in-process broker，**替换 broker 只需一行**：

```go
// 切换为 RabbitMQ（需引入 mq 子模块）
// import "github.com/astra-go/astra/mq/rabbitmq"
// broker := rabbitmq.NewProducer(rabbitmq.Config{URL: "amqp://guest:guest@localhost/"})
```

```bash
# 发布 order.created 事件
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"item":"widget","qty":3}'

# 发布 order.shipped 事件
curl -X POST http://localhost:8080/orders/42/ship

# 查看所有已处理事件
curl http://localhost:8080/events
```

---

### ORM 示例（`examples/orm`）

```bash
cd examples/orm && go run main.go
```

GORM + SQLite 内存数据库，演示：软删除（`gorm.Model`）、恢复（`Unscoped().Update`）、分页查询。

```bash
# 列表（分页）
curl "http://localhost:8080/api/v1/products?page=1&size=10"

# 创建
curl -X POST http://localhost:8080/api/v1/products \
  -H "Content-Type: application/json" \
  -d '{"name":"Widget Z","price":29.99,"stock":50,"category":"widgets"}'

# 软删除
curl -X DELETE http://localhost:8080/api/v1/products/1

# 恢复
curl -X POST http://localhost:8080/api/v1/products/1/restore
```

---

### 缓存示例（`examples/cache`）

```bash
cd examples/cache && go run main.go
```

Read-through 缓存模式：首次请求触发 DB 查询（模拟 20ms 延迟），后续命中缓存（< 1ms）；更新时自动失效。

```bash
# 第一次：~20ms（DB 查询）
curl http://localhost:8080/users/1

# 第二次：< 1ms（cache hit）
curl http://localhost:8080/users/1

# 更新并自动失效缓存
curl -X PUT http://localhost:8080/users/1 \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice Updated"}'

# 查看命中率统计
curl http://localhost:8080/cache/stats

# 手动驱逐
curl -X DELETE http://localhost:8080/cache/1
```

---

### 可观测微服务示例（代码见「快速开始」第 4 节）

演示：HTTP + gRPC 双栈、OTel OTLP 上报、HTTP 和 gRPC 分布式追踪、日志 × Trace 关联、滑动窗口限流、自适应熔断器、Prometheus 指标。

启动 Jaeger（本地 all-in-one）后运行：

```bash
# 启动 Jaeger（OTLP receiver 默认开启）
docker run -d --name jaeger \
  -p 4317:4317 -p 16686:16686 \
  jaegertracing/all-in-one:latest

# 运行服务
go run main.go

# 发送请求并在 Jaeger UI 查看 trace
curl http://localhost:8080/health
open http://localhost:16686
```

---

