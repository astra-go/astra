# Astra 示例

按以下路径逐步学习，每个示例都可以直接 `go run` 运行。

## 学习路径

```
① hello      — 18 行，验证安装（5 分钟）
      ↓
② basic      — 路由组、中间件、请求绑定与校验（15 分钟）
      ↓
③ jwt        — JWT 认证保护路由（10 分钟）
      ↓
④ crud       — 完整 CRUD + 数据库（30 分钟）
      ↓
⑤ orm        — GORM 集成，Repository 模式（20 分钟）
      ↓
⑥ mq         — 消息队列，生产者 + 消费者（20 分钟）
      ↓
⑦ showcase   — 生产级完整示例（参考）
```

---

## 各示例说明

### ① hello — 最小模板

```bash
cd hello && go run main.go
curl http://localhost:8080/hello/world
```

演示：`astra.New()` + 路由 + `app.Run()`。和 Gin/Echo 写法完全一致，无任何额外概念。

---

### ② basic — 核心特性

```bash
cd basic && go run main.go
```

演示：
- 全局中间件（Recovery / Logger / RequestID / CORS / Timeout）
- 路由组（`app.Group`）
- 路径参数、查询参数
- JSON 请求绑定与校验（`c.BindJSON` + `validate` 标签）
- 生命周期钩子（`app.OnStart` / `app.OnStop`）

---

### ③ jwt — JWT 认证

```bash
cd jwt && go run main.go
# 登录获取 token
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}'
# 使用 token 访问受保护路由
curl http://localhost:8080/api/v1/profile \
  -H "Authorization: Bearer <token>"
```

演示：
- `middleware.JWT` 保护路由组
- 登录 handler 生成 JWT
- 公开路由与受保护路由分组

---

### ④ crud — 完整 CRUD

```bash
cd crud && go run main.go
curl -X POST http://localhost:8080/api/v1/items \
  -H "Content-Type: application/json" \
  -d '{"name":"Apple","price":10}'
curl http://localhost:8080/api/v1/items
```

演示：
- RESTful CRUD 接口（GET / POST / PUT / DELETE）
- 请求绑定与校验
- 统一错误处理
- 内存 Store（可替换为真实数据库）

---

### ⑤ orm — GORM 集成

```bash
cd orm && go run main.go
```

演示：
- `astra/orm` 模块集成 GORM
- 泛型 `Repository[T]` 模式
- 事务辅助函数 `RunTx`
- Module 系统组织代码

> 需要 PostgreSQL，连接字符串通过环境变量 `DATABASE_URL` 配置。

---

### ⑥ mq — 消息队列

```bash
cd mq && go run main.go
```

演示：
- `astra/mq` 统一接口
- 生产者发送消息
- 消费者处理消息
- 优雅关闭

> 默认使用 NATS，需要本地运行 `docker run -p 4222:4222 nats`。

---

### ⑦ showcase — 生产级示例

完整的生产级应用，包含：
- Module + DI 容器组织依赖
- OTel 分布式追踪
- Prometheus 指标
- gRPC 双栈
- 自适应熔断器
- 健康检查（K8s / Istio）

适合作为新项目的参考模板。

---

## 快速生成项目骨架

使用 `astractl` CLI 可以快速生成可运行的项目骨架：

```bash
# 安装 CLI
go install github.com/astra-go/astra/cmd/astractl@latest

# 生成新项目
astractl new myapp

# 生成 CRUD handler
astractl gen crud User --with-service

# 从 proto 文件生成 handler
astractl gen proto api/user.proto
```

详见 [astractl 文档](../docs/getting-started/quickstart.md#astractl-cli)。
