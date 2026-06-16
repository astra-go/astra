# Astra Examples

通过以下学习路径逐步学习；每个示例均可通过 `go run` 直接运行。

## 学习路径

```
① hello      — 18 行代码，验证安装（5 分钟）
      ↓
② basic      — 路由分组、中间件、请求绑定和校验（15 分钟）
      ↓
③ jwt        — JWT 认证保护路由（10 分钟）
      ↓
④ crud       — 完整 CRUD + 数据库（30 分钟）
      ↓
⑤ orm        — GORM 集成、Repository 模式（20 分钟）
      ↓
⑥ mq         — 消息队列、生产者 + 消费者（20 分钟）
      ↓
⑦ showcase   — 生产级完整示例（参考）
```

---

## 示例说明

### ① hello — 最小模板

```bash
cd hello && go run main.go
curl http://localhost:8080/hello/world
```

展示：`astra.New()` + 路由 + `app.Run()`。Gin/Echo 兼容 API，无需额外概念。

---

### ② basic — 核心功能

```bash
cd basic && go run main.go
```

展示：
- 全局中间件（Recovery / Logger / RequestID / CORS / Timeout）
- 路由分组（`app.Group`）
- 路径参数、查询参数
- JSON 请求绑定和校验（`c.BindJSON` + `validate` tag）
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

展示：
- `middleware.JWT` 保护路由分组
- 登录处理器生成 JWT
- 公开和受保护的路由分组

---

### ④ crud — 完整 CRUD

```bash
cd crud && go run main.go
curl -X POST http://localhost:8080/api/v1/items \
  -H "Content-Type: application/json" \
  -d '{"name":"Apple","price":10}'
curl http://localhost:8080/api/v1/items
```

展示：
- RESTful CRUD 接口（GET / POST / PUT / DELETE）
- 请求绑定和校验
- 统一错误处理
- 内存存储（可替换为真实数据库）

---

### ⑤ orm — GORM 集成

```bash
cd orm && go run main.go
```

展示：
- `astra/orm` 模块集成 GORM
- 泛型 `Repository[T]` 模式
- 事务辅助 `RunTx`
- 模块系统组织代码

> 需要 PostgreSQL；连接字符串通过 `DATABASE_URL` 环境变量配置。

---

### ⑥ mq — 消息队列

```bash
cd mq && go run main.go
```

展示：
- `astra/mq` 统一接口
- 生产者发送消息
- 消费者处理消息
- 优雅关闭

> 默认使用 NATS；需要本地 `docker run -p 4222:4222 nats`。

---

### ⑦ showcase — 生产级示例

完整的生产级应用，包含：
- 模块 + DI 容器组织依赖
- OTel 分布式追踪
- Prometheus 指标
- gRPC 双栈
- 自适应熔断器
- 健康检查（K8s / Istio）

适合作为新项目的参考模板。

---

## 快速生成项目脚手架

使用 `astractl` CLI 快速生成可运行的项目脚手架：

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