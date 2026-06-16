# Showcase — Astra 生产级参考应用

功能完整的电商 / SaaS 演示项目，展示如何在单一生产级应用中组合使用所有主要 Astra 子模块。

## 功能概览

| 功能 | 模块 | 位置 |
|---------|--------|-------|
| 多租户 ORM + AutoMigrate | `astra/orm` | `internal/db`, `internal/repository` |
| 泛型 `Repository[T]` + 租户隔离 | `astra/orm` | `internal/repository/tenant_repo.go` |
| 原子库存扣减（无 TOCTOU） | `astra/orm` | `internal/repository/repos.go` |
| 读穿透 Redis 缓存 | `astra/cache` | `internal/service/cached_product_svc.go` |
| 异步任务队列（订单邮件、报表） | `astra/taskqueue` | `internal/service/order_svc.go`, `cmd/worker` |
| Casbin RBAC（admin / seller / buyer） | `astra/auth/rbac` | `config/rbac_*.{conf,csv}`, `cmd/api/main.go` |
| OAuth2 登录（Google + GitHub） | `astra/auth/oauth2` | `internal/handler/auth_handler.go` |
| JWT 颁发 + 中间件 | `astra/middleware` | `internal/service/user_svc.go` |
| gRPC 双栈（HTTP :8081 + gRPC :9091） | `astra/grpc` | `cmd/grpc`, `internal/grpc` |
| OTel 追踪 + Prometheus 指标 | `astra/otel` | `cmd/api/main.go`, `cmd/grpc/main.go` |
| 金丝雀 / 特性开关中间件 | `astra/middleware` | `cmd/api/main.go` |
| `TxMiddleware` 原子订单创建 | `astra/orm` | `cmd/api/main.go` |

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│  cmd/api  (HTTP :8080)          cmd/grpc  (HTTP :8081 + gRPC :9091)
│     │                                │
│  middleware chain                middleware chain
│  RequestID → Tracing → Logger    RequestID → Tracing → Logger
│  Recovery → CORS → JWT → RBAC    Recovery
│  Canary                               │
│     │                           grpcserver.New
│  handlers                       InventoryService (gRPC)
│  ProductHandler                       │
│  OrderHandler ──TxMiddleware──►  productRepo
│  AdminHandler                         │
│  AuthHandler (OAuth2)                 │
│     │                           ▼
│     ▼                           PostgreSQL
│  services
│  CachedProductSvc ──► Redis
│  OrderSvc ──────────► TaskQueue ──► cmd/worker
│  UserSvc ───────────► JWT
│     │
│     ▼
│  repositories (TenantRepository[T])
│     │
│     ▼
│  PostgreSQL
└─────────────────────────────────────────────────────────────┘

可观测性: Jaeger (追踪) + Prometheus + Grafana
```

## 快速开始

```bash
# 1. 启动基础设施
docker compose up -d

# 2. 运行 API 服务
go run ./cmd/api

# 3. （可选）运行 gRPC 双栈服务
go run ./cmd/grpc

# 4. （可选）运行后台 Worker
go run ./cmd/worker
```

Jaeger UI: http://localhost:16686  
Grafana:   http://localhost:3000 (admin/admin)  
Prometheus: http://localhost:9090

## API 参考

### 公开端点

| 方法 | 路径 | 说明 |
|--------|------|-------------|
| GET | `/health` | 健康检查 |
| GET | `/auth/google/login` | 重定向到 Google OAuth2 |
| GET | `/auth/google/callback` | Google OAuth2 回调 → JWT |
| GET | `/auth/github/login` | 重定向到 GitHub OAuth2 |
| GET | `/auth/github/callback` | GitHub OAuth2 回调 → JWT |

### 受保护端点（需要 Bearer JWT）

#### 产品

| 方法 | 路径 | 角色 | 说明 |
|--------|------|------|-------------|
| GET | `/api/v1/products` | buyer, seller, admin | 产品列表（第一页缓存） |
| POST | `/api/v1/products` | seller, admin | 创建产品 |
| GET | `/api/v1/products/:id` | buyer, seller, admin | 获取产品 |
| PUT | `/api/v1/products/:id` | seller, admin | 更新产品 |
| DELETE | `/api/v1/products/:id` | seller, admin | 删除产品 |

#### 订单

| 方法 | 路径 | 角色 | 说明 |
|--------|------|------|-------------|
| GET | `/api/v1/orders` | buyer, seller, admin | 订单列表 |
| GET | `/api/v1/orders/:id` | buyer, seller, admin | 获取订单及明细 |
| POST | `/api/v1/orders` | buyer, admin | 创建订单（原子库存扣减） |

#### 管理

| 方法 | 路径 | 角色 | 说明 |
|--------|------|------|-------------|
| GET | `/api/v1/admin/users/:id` | admin | 获取用户 |
| PUT | `/api/v1/admin/users/:id/role` | admin | 更新用户角色 |

### gRPC 服务（`:9091`）

```protobuf
service InventoryService {
  rpc GetStock(GetStockRequest) returns (StockResponse);
  rpc BatchGetStock(BatchGetStockRequest) returns (BatchStockResponse);
  rpc DecrStock(DecrStockRequest) returns (StockResponse);
  rpc ListLowStock(ListLowStockRequest) returns (stream StockItem);
}
```

使用 `grpcurl` 或 Evans 探索：

```bash
grpcurl -plaintext localhost:9091 list
grpcurl -plaintext -d '{"product_id":1,"tenant_id":1}' \
  localhost:9091 showcase.inventory.v1.InventoryService/GetStock
```

## 配置

| 变量 | 默认值 | 说明 |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://showcase:showcase@localhost:5432/showcase?sslmode=disable` | PostgreSQL DSN |
| `REDIS_ADDR` | `localhost:6379` | Redis 地址 |
| `JWT_SECRET` | `change-me-in-production` | HS256 签名密钥 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | _(空，禁用)_ | OTLP gRPC 端点 |
| `HTTP_ADDR` | `:8081` | HTTP 监听地址（cmd/grpc 专用） |
| `GRPC_ADDR` | `:9091` | gRPC 监听地址（cmd/grpc 专用） |

## 运行测试

```bash
# 单元 + repository 测试（SQLite 内存，无需外部依赖）
go test ./...

# 带竞态检测
go test -race ./...

# 集成测试（需要 Docker）
go test -tags integration -v ./internal/integration/...
```

## 项目布局

```
examples/showcase/
├── cmd/
│   ├── api/        # HTTP 入口（REST + OAuth2 + RBAC）
│   ├── grpc/       # 双栈 HTTP+gRPC 入口
│   └── worker/     # 后台任务 Worker
├── config/
│   ├── rbac_model.conf
│   └── rbac_policy.csv
├── internal/
│   ├── db/         # Open, Migrate, Seed
│   ├── domain/     # 实体 + DTO
│   ├── grpc/       # gRPC 服务实现
│   ├── handler/    # HTTP 处理器 + auth handler
│   ├── integration/# Testcontainers 集成测试
│   ├── pb/         # 生成的 protobuf 代码
│   ├── repository/ # TenantRepository[T] + 具体 repos
│   └── service/    # 业务逻辑 + 缓存封装
├── proto/          # .proto 源文件
├── docker-compose.yml
└── Makefile
```

## RBAC 角色

| 角色 | 产品 | 订单 | 管理 |
|------|----------|--------|-------|
| `admin` | 完整 CRUD | 完整 CRUD | 是 |
| `seller` | GET + POST + PUT + DELETE | GET 仅 | 否 |
| `buyer` | GET 仅 | GET + POST | 否 |

新 OAuth2 用户自动分配 `buyer` 角色。管理员可通过 `PUT /api/v1/admin/users/:id/role` 晋升用户。

## 金丝雀部署

10% 的用户（user_id % 10 == 0）和带 `X-Canary: true` 的请求路由到 `v2` 结账流程。金丝雀版本存储在请求上下文中，处理器可读取以提供不同行为。

---

## 架构决策与最佳实践

### 1. 泛型 Repository 模式：`TenantRepository[T]`

**决策**：使用泛型 Repository 包装器，自动应用租户过滤。

**理由**：
- **类型安全**：编译期保证实体类型
- **DRY**：消除跨 Repository 的重复租户过滤代码
- **安全**：不可能忘记租户隔离（类型级别强制）
- **可测试**：易于用接口 mock

### 2. 原子库存扣减（行级锁）

**决策**：使用 `UPDATE ... WHERE stock >= qty` 加行锁，而非 SELECT + UPDATE。

**理由**：
- **防止超卖**：原子操作防止 TOCTOU 竞态
- **数据库级保证**：即使高并发，库存永不为负
- **无应用锁**：数据库处理并发，代码更简洁

### 3. 事务中间件用于订单创建

**决策**：使用中间件将订单创建包装在数据库事务中。

**理由**：
- **原子性**：订单创建 + 库存扣减必须一起成功或失败
- **关注点分离**：业务逻辑不管理事务
- **一致错误处理**：Panic 或错误时自动回滚

### 4. 读穿透缓存模式

**决策**：缓存产品列表并在更新时自动失效。

**理由**：
- **性能**：缓存产品列表 5000+ QPS vs 无缓存 500 QPS
- **防脏数据**：CUD 操作时失效缓存
- **简单**：Service 层处理缓存，Repository 保持干净

### 5. gRPC 和 HTTP 分离

**决策**：gRPC 服务独立于 HTTP API 运行（不同进程）。

**理由**：
- **独立扩展**：根据 RPC 负载而非 HTTP 负载扩展 gRPC Worker
- **避免端口冲突**：HTTP :8080，gRPC :9091
- **协议优化**：不同中间件链（gRPC 不需要 CORS、JWT）
- **部署灵活性**：可在内部负载均衡器后部署 gRPC-only 实例

---

## 常见陷阱及避免方法

### ❌ 陷阱 1：N+1 查询问题

**差**：逐条加载订单
```go
orders := repo.FindAll()
for _, order := range orders {
    items := itemRepo.FindByOrderID(order.ID) // N 条查询！
}
```

**好**：用 GORM 预加载
```go
db.Preload("Items").Find(&orders)
```

### ❌ 陷阱 2：忘记租户隔离

**差**：直接 GORM 查询，无租户过滤！
```go
db.First(&product, productID)
```

**好**：使用 TenantRepository
```go
productRepo := NewProductRepo(db, tenantID)
product, err := productRepo.FindByID(ctx, productID)
```

### ❌ 陷阱 3：缓存穿透

**问题**：反复请求不存在的 key 导致打到数据库。

**解决方案**：缓存负面结果并设置短 TTL。

```go
if err == gorm.ErrRecordNotFound {
    s.cache.Set(ctx, key, nil, 1*time.Minute)
    return nil, err
}
```

### ❌ 陷阱 4：无界分页

**差**：无 LIMIT，可能加载数百万行
```go
db.Find(&products)
```

**好**：始终分页
```go
db.Limit(pageSize).Offset((page-1)*pageSize).Find(&products)
```

---

## 生产就绪清单

- [x] 多租户隔离在 Repository 层强制执行
- [x] 库存管理的原子操作（无竞态）
- [x] Casbin RBAC（admin/seller/buyer 角色）
- [x] OAuth2 认证（Google + GitHub）
- [x] JWT Token 颁发和校验
- [x] OpenTelemetry 请求追踪
- [x] Prometheus 指标暴露
- [x] 存活/就绪健康检查
- [x] golang-migrate 数据库迁移
- [x] Redis 缓存（带失效）
- [x] 服务间通信 gRPC
- [x] Worker 异步任务处理
- [x] 集成测试（20+ 场景）
- [x] 性能基准测试（目标已定义）
- [x] Kubernetes 部署清单
- [x] Horizontal Pod Autoscaling (HPA)
- [x] 完整文档

**生产环境仍需**：
- [ ] 按租户限流
- [ ] 敏感操作审计日志
- [ ] 备份和灾难恢复
- [ ] CI/CD 流水线
- [ ] 安全扫描（SAST/DAST）
- [ ] 预发布环境负载测试
- [ ] 事件响应 Runbook

---

## 延伸阅读

- [Astra 框架文档](https://github.com/astra-go/astra)
- [Twelve-Factor App](https://12factor.net/)
- [Database Reliability Engineering](https://www.oreilly.com/library/view/database-reliability-engineering/9781491925935/)
- [Building Microservices (2nd Edition)](https://www.oreilly.com/library/view/building-microservices-2nd/9781492034018/)
- [Site Reliability Engineering](https://sre.google/books/)