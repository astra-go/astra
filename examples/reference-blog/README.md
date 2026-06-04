# Reference Blog System

一个完整的博客系统参考实现，展示 Astra 框架的企业级应用最佳实践。

## 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client (Web/Mobile)                      │
└──────────────────────────┬──────────────────────────────────────┘
                           │ HTTP/REST
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                       API Server (Astra)                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ Auth Handler │  │Article Handler│  │Search Handler│          │
│  └──────┬───────┘  └──────┬────────┘  └──────┬───────┘          │
│         │                 │                   │                   │
│  ┌──────▼──────────────────▼───────────────────▼───────┐        │
│  │              Service Layer (DDD)                     │        │
│  │  AuthService │ ArticleService │ NotificationService  │        │
│  └──────┬──────────────┬─────────────────┬─────────────┘        │
│         │              │                 │                        │
│  ┌──────▼──────┐  ┌───▼────────┐  ┌────▼──────┐                │
│  │ Repository  │  │ Repository │  │   MQ      │                │
│  │   Layer     │  │   Layer    │  │ Producer  │                │
│  └──────┬──────┘  └───┬────────┘  └────┬──────┘                │
└─────────┼─────────────┼────────────────┼────────────────────────┘
          │             │                │
          │             │                │ gRPC
          ▼             ▼                ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────────┐
│  PostgreSQL  │  │    Redis     │  │  Comment Service │
│   (Primary)  │  │   (Cache)    │  │     (gRPC)       │
└──────────────┘  └──────────────┘  └──────────────────┘
                                           │
          ┌────────────────────────────────┤
          │                                │
          ▼                                ▼
┌──────────────────┐              ┌──────────────┐
│ Kafka (Async)    │              │ PostgreSQL   │
│ - notifications  │              │ (Comments)   │
│ - email queue    │              └──────────────┘
└────────┬─────────┘
         │
         ▼
┌──────────────────┐              ┌──────────────┐
│  Worker Service  │◄─────────────│Elasticsearch │
│  (Kafka Consumer)│              │   (Search)   │
└──────────────────┘              └──────────────┘

                    ┌──────────────────┐
                    │  Observability   │
                    │ Prometheus+Jaeger│
                    └──────────────────┘
```

## 项目结构

```
examples/reference-blog/
├── cmd/
│   ├── api-server/          # HTTP API 服务入口
│   │   └── main.go
│   ├── comment-service/     # gRPC 评论服务入口
│   │   └── main.go
│   └── worker/              # Kafka 消费者入口
│       └── main.go
├── internal/
│   ├── domain/              # 领域模型 (DDD)
│   │   ├── user.go
│   │   ├── article.go
│   │   └── comment.go
│   ├── repository/          # 数据访问层
│   │   ├── user_repo.go
│   │   ├── article_repo.go
│   │   └── comment_repo.go
│   ├── service/             # 业务逻辑层
│   │   ├── auth_service.go
│   │   ├── article_service.go
│   │   ├── comment_service.go
│   │   ├── search_service.go
│   │   └── notification_service.go
│   ├── handler/             # HTTP/gRPC 处理器
│   │   ├── auth_handler.go
│   │   ├── article_handler.go
│   │   ├── comment_handler.go
│   │   └── search_handler.go
│   └── proto/               # gRPC 协议定义
│       └── comment.proto
├── deployments/
│   ├── docker/
│   │   └── Dockerfile
│   └── k8s/
│       ├── api-server.yaml
│       ├── comment-service.yaml
│       ├── worker.yaml
│       └── configmap.yaml
├── configs/
│   ├── api-server.yaml
│   ├── comment-service.yaml
│   └── worker.yaml
├── tests/
│   ├── integration/
│   └── benchmark/
├── docker-compose.yml       # 本地开发环境
├── Makefile                 # 构建和运行脚本
├── go.mod
└── README.md
```

## 快速开始

### 前置要求

- Go 1.25.1+
- Docker & Docker Compose
- Make (可选)

### 本地开发

1. **启动依赖服务**

```bash
docker-compose up -d postgres redis kafka elasticsearch
```

2. **数据库迁移**

```bash
make migrate
```

3. **启动服务**

```bash
# Terminal 1: API Server
make run-api

# Terminal 2: Comment Service
make run-comment

# Terminal 3: Worker
make run-worker
```

4. **访问服务**

- API Server: http://localhost:8080
- Comment Service (gRPC): localhost:9090
- Prometheus: http://localhost:9091/metrics
- Jaeger UI: http://localhost:16686

### Docker 部署

```bash
# 构建镜像
make docker-build

# 启动所有服务
docker-compose up -d
```

### Kubernetes 部署

```bash
# 应用配置
kubectl apply -f deployments/k8s/

# 验证部署
kubectl get pods -n reference-blog
```

## 功能模块

### 1. 用户管理 (Auth)

**技术栈**: Astra Auth + JWT + Casbin RBAC

**API 端点**:

```
POST   /api/v1/auth/register    # 用户注册
POST   /api/v1/auth/login       # 用户登录
POST   /api/v1/auth/refresh     # 刷新令牌
GET    /api/v1/users/:id        # 获取用户信息
PUT    /api/v1/users/:id        # 更新用户信息
```

**特性**:
- JWT access token (15分钟) + refresh token (7天)
- Casbin RBAC 权限控制 (admin / author / reader)
- 密码 bcrypt 加密
- 登录限流保护

### 2. 文章 CRUD (Article)

**技术栈**: Astra ORM (GORM + PostgreSQL) + Redis Cache

**API 端点**:

```
POST   /api/v1/articles         # 创建文章 (需登录)
GET    /api/v1/articles         # 文章列表 (分页)
GET    /api/v1/articles/:id     # 文章详情
PUT    /api/v1/articles/:id     # 更新文章 (作者/管理员)
DELETE /api/v1/articles/:id     # 删除文章 (作者/管理员)
POST   /api/v1/articles/:id/publish  # 发布文章
```

**特性**:
- 文章状态机: draft → published → archived
- Redis 缓存策略:
  - 文章详情: TTL 10分钟
  - 文章列表: TTL 5分钟
  - Cache-aside pattern with write-through
- 软删除 (GORM DeletedAt)
- 乐观锁 (Version 字段)

### 3. 评论系统 (Comment)

**技术栈**: Astra gRPC + PostgreSQL

**gRPC 服务定义**:

```protobuf
service CommentService {
  rpc CreateComment(CreateCommentRequest) returns (Comment);
  rpc ListComments(ListCommentsRequest) returns (ListCommentsResponse);
  rpc DeleteComment(DeleteCommentRequest) returns (Empty);
  rpc LikeComment(LikeCommentRequest) returns (Comment);
}
```

**特性**:
- gRPC 服务间通信 (API Server ↔ Comment Service)
- 评论嵌套树形结构 (parent_id)
- 点赞计数器 (Redis increment)
- 敏感词过滤

### 4. 搜索功能 (Search)

**技术栈**: Astra Search (Elasticsearch 8.x)

**API 端点**:

```
GET    /api/v1/search?q=keyword&type=article  # 全文搜索
GET    /api/v1/search/suggest?q=key           # 搜索建议
```

**特性**:
- Elasticsearch IK 中文分词
- 搜索高亮显示
- 模糊匹配 + 拼音搜索
- 异步索引更新 (通过 Kafka)

### 5. 通知系统 (Notification)

**技术栈**: Astra MQ (Kafka + franz-go)

**Kafka Topics**:

```
article.published    # 文章发布事件
comment.created      # 评论创建事件
user.followed        # 用户关注事件
notification.email   # 邮件通知队列
```

**特性**:
- 异步事件驱动架构
- 消息持久化 + 重试机制
- Consumer Group 负载均衡
- Dead Letter Queue (DLQ)

## 配置管理

**环境变量** (`configs/api-server.yaml` 示例):

```yaml
server:
  port: 8080
  mode: debug  # debug | release

database:
  driver: postgres
  dsn: "postgres://user:pass@localhost:5432/blog?sslmode=disable"
  max_open_conns: 50
  max_idle_conns: 10

redis:
  addr: localhost:6379
  password: ""
  db: 0

kafka:
  brokers:
    - localhost:9092
  group_id: blog-api-server

auth:
  jwt_secret: "your-secret-key"
  access_token_ttl: 15m
  refresh_token_ttl: 168h

grpc:
  comment_service_addr: localhost:9090

elasticsearch:
  addresses:
    - http://localhost:9200
  username: elastic
  password: changeme

observability:
  prometheus_port: 9091
  jaeger_endpoint: http://localhost:14268/api/traces
```

## 可观测性 (Observability)

### Metrics (Prometheus)

- HTTP 请求延迟 (histogram)
- 数据库连接池状态
- Redis 缓存命中率
- Kafka 消息处理速率

### Tracing (Jaeger)

- 端到端请求链路追踪
- gRPC 调用追踪
- 数据库查询追踪
- 缓存操作追踪

### Logging

- 结构化日志 (JSON)
- 日志级别: DEBUG | INFO | WARN | ERROR
- 请求 ID 关联

## 测试

### 单元测试

```bash
make test
```

### 集成测试

```bash
make test-integration
```

**覆盖率目标**: > 80%

### 性能基准测试

```bash
make benchmark
```

**性能目标**: 单实例支持 1000 QPS

## 架构决策记录 (ADR)

### ADR-001: 为什么使用 gRPC 作为评论服务通信协议?

**背景**: 评论系统需要高频读写，且未来可能独立扩展。

**决策**: 采用 gRPC 而非 HTTP REST。

**理由**:
1. 性能: Protobuf 序列化比 JSON 快 3-10 倍
2. 类型安全: 编译期类型检查
3. 双向流: 支持实时评论推送 (未来扩展)
4. 服务发现: 易于集成 Consul/Kubernetes Service

**后果**: 需要维护 .proto 文件和代码生成工具链。

### ADR-002: 为什么选择 Kafka 而非 RabbitMQ?

**背景**: 需要高吞吐量的异步消息处理。

**决策**: 使用 Kafka 作为消息中间件。

**理由**:
1. 吞吐量: Kafka 单节点可达 100k+ msg/s
2. 持久化: 基于日志的存储，支持消息回溯
3. 分区: 天然支持水平扩展
4. 生态: 易于集成 Flink/Spark 做流处理

**后果**: 运维复杂度略高于 RabbitMQ。

### ADR-003: 缓存更新策略为什么用 Cache-Aside 而非 Write-Through?

**背景**: 文章更新频率低 (< 1%)，读取频率高 (> 99%)。

**决策**: 采用 Cache-Aside (懒加载) 模式。

**理由**:
1. 简单: 逻辑清晰，易于理解和调试
2. 灵活: 缓存失效策略可独立调整
3. 性能: 避免写操作同步等待缓存更新

**权衡**: 可能出现短暂的缓存不一致 (通过 TTL 自动修复)。

## 最佳实践

1. **领域驱动设计 (DDD)**
   - 清晰的分层架构: domain → repository → service → handler
   - 领域模型与数据模型分离

2. **依赖注入 (DI)**
   - 使用 Astra 的 DI 容器管理依赖
   - 便于单元测试 (mock 注入)

3. **错误处理**
   - 统一错误码定义 (`internal/errors/codes.go`)
   - 错误日志上下文传递

4. **安全实践**
   - SQL 注入防护 (GORM 参数化查询)
   - XSS 防护 (输出 HTML 转义)
   - CSRF 防护 (Token 验证)
   - 敏感数据加密存储

5. **性能优化**
   - 数据库索引优化 (联合索引 + 覆盖索引)
   - Redis 缓存预热
   - gRPC 连接池复用
   - Kafka 批量消费

## License

MIT

## 贡献指南

欢迎提交 Issue 和 Pull Request!

## 联系方式

- GitHub Issues: https://github.com/astra-go/astra/issues
- Discussions: https://github.com/astra-go/astra/discussions
