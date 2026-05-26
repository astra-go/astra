## 依赖说明

Astra 采用 **多模块 monorepo 架构**（`go.work` + 19 个独立子模块）。根模块仅有 8 个直接依赖；OTel / GORM / MQ / Redis 等重量级集成各自独立声明版本，用户按需 `go get` 对应子模块，未使用的模块不进入 `vendor`，二进制体积可控，升级互不干扰。

---

### 根模块 — `go get github.com/astra-go/astra`（8 个直接依赖）

| 包 / 路径 | 说明 | 外部依赖 |
|-----------|------|---------|
| `app.go` / `router.go` / `context.go` / `group.go` | 核心路由框架（基数树 O(k)，优雅停机） | 无 |
| `middleware/`（logger / recovery / cors / ratelimit / ratelimit_advanced / timeout / requestid / compress / csrf / secure / pprof / apikey / audit / tenant / canary / signature / csp / ipfilter / longpoll） | 轻量中间件全家桶 | 无（纯 stdlib） |
| `middleware/jwt.go` | JWT 认证（HS256/RS256/ES256） | `github.com/golang-jwt/jwt/v5` |
| `middleware/metrics.go` | Prometheus HTTP 指标采集 | `github.com/prometheus/client_golang` |
| `websocket/` | Hub/Client WebSocket（广播 + 心跳 + 并发安全） | `github.com/gorilla/websocket` |
| `app_quic.go` | HTTP/3 RunQUIC（Alt-Svc 自动升级，TLS + QUIC 双栈） | `github.com/quic-go/quic-go` |
| `cron/` | 定时任务调度器（interval + cron 表达式，Panic 恢复） | `github.com/robfig/cron/v3` |
| `alert/` | 告警规则引擎（`expr` 表达式求值 + `For` 持续窗口 + Channel 通知） | `github.com/expr-lang/expr` |
| `validate/` | 请求参数验证 | `github.com/go-playground/validator/v10` |
| `circuit/` | 三态熔断器 + 自适应熔断器（错误率/P99 延迟） | 无 |
| `health/` + `health/istio.go` | 健康检查三端点 + Istio `/healthz/*` probe | 无 |
| `di/` | 轻量依赖注入容器（`Provide[T]` / `Invoke[T]` / 命名实例 / 生命周期 `OnStart`/`OnStop` + `BindApp`） | 无 |
| `dtx/saga.go` | Saga 分布式事务（正向步骤 + 逆序补偿） | 无 |
| `graphql/` | GraphQL 挂载助手（`Mount()` + Playground HTML） | 无 |
| `pagination/` | offset / cursor 双模式分页（`Page[T]` / `CursorPage[T]`，纯 stdlib，无 ORM 依赖） | 无 |
| `render/` | HTML 模板引擎（布局继承 + `embed.FS` + 热重载） | 无 |
| `swagger/` | Swagger UI + OpenAPI JSON 端点（CDN / 自托管） | 无 |
| `config/config.go` + `config/remote.go` | 多源配置（YAML/JSON/ENV + fsnotify 热重载 + etcd/Consul 远程） | `gopkg.in/yaml.v3` |
| `log/` / `binding/` / `errors.go` / `retry/` / `loadbalance/` / `timeutil/` / `migrate/` | 工具包（stdlib） | 无 |

---

### 子模块 — 按需 `go get`

各子模块独立版本演进；本地开发通过 `go.work` 自动解析，IDE 跳转无感知。

#### `go get github.com/astra-go/astra/otel`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `otel/provider.go` | OTel SDK 初始化（OTLP gRPC / stdout exporter，Prometheus exporter，日志关联 helper，gRPC 客户端拦截器） | `go.opentelemetry.io/otel` + SDK + exporters |

#### `go get github.com/astra-go/astra/orm`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `orm/gorm.go` | GORM 适配（DB 注入 / 自动事务 / 分页 / 泛型 Repository） | `gorm.io/gorm` |
| `orm/dialect.go` | MySQL / PostgreSQL 方言快速构造 | `gorm.io/driver/mysql`, `gorm.io/driver/postgres` |
| `orm/clickhouse/clickhouse.go` | ClickHouse GORM 方言适配（连接池，`Open(Config)`） | `gorm.io/driver/clickhouse` |

#### `go get github.com/astra-go/astra/mq`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `mq/rabbitmq/` | RabbitMQ（AMQP 0-9-1，amqp091-go） | `github.com/rabbitmq/amqp091-go` |
| `mq/kafka/` | Apache Kafka（franz-go，ProduceSync + 消费组） | `github.com/twmb/franz-go/pkg/kgo` |
| `mq/nats/` | NATS Core QueueSubscribe + JetStream Durable push | `github.com/nats-io/nats.go` |
| `mq/mqtt/` | MQTT 3.1.1/5.0（EMQX / Mosquitto / NanoMQ） | `github.com/eclipse/paho.mqtt.golang` |
| `mq/pulsar/` | Apache Pulsar（Exclusive/Shared/Failover/KeyShared，Token/TLS 认证） | `github.com/apache/pulsar-client-go/pulsar` |
| `mq/rocketmq/` | RocketMQ 5.x gRPC SimpleConsumer | `github.com/apache/rocketmq-clients/golang/v5` |

#### `go get github.com/astra-go/astra/taskqueue`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `taskqueue/redis/broker.go` | Redis 后端（6 个 Lua 原子脚本，ZSET 延迟队列） | `github.com/redis/go-redis/v9` |
| `taskqueue/mongo/broker.go` | MongoDB 后端（FindOneAndUpdate + TTL 去重集合） | `go.mongodb.org/mongo-driver/v2` |
| `taskqueue/rabbitmq/broker.go` | RabbitMQ 后端（x-delayed-message 延迟交换机） | `github.com/rabbitmq/amqp091-go` |
| `taskqueue/kafka/broker.go` | Kafka 后端（三客户端模型 + retry topic） | `github.com/twmb/franz-go/pkg/kgo` |
| `taskqueue/rocketmq/broker.go` | RocketMQ 5.x 后端（原生延迟重投） | `github.com/apache/rocketmq-clients/golang/v5` |

#### `go get github.com/astra-go/astra/storage`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `storage/s3/` | AWS S3（兼容 MinIO / Cloudflare R2 / Backblaze B2） | `github.com/aws/aws-sdk-go-v2/service/s3` |
| `storage/oss/` | 阿里云 OSS | `github.com/aliyun/aliyun-oss-go-sdk` |
| `storage/cos/` | 腾讯云 COS | `github.com/tencentyun/cos-go-sdk-v5` |

#### `go get github.com/astra-go/astra/grpc`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `grpc/server.go` | HTTP + gRPC 双栈（健康检查 / OTel 追踪 / WithTimeout/TLS / ChainInterceptors） | `google.golang.org/grpc` |
| `grpc/errors.go` | Kratos 风格结构化错误（`BadRequest` / `NotFound` / `FromError` 解包） | — |
| `grpc/middleware.go` | Kratos 中间件抽象（`Handler` / `Chain` / `UnaryInterceptorMiddleware`） | — |

#### `go get github.com/astra-go/astra/discovery`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `discovery/etcd/` | etcd 服务发现（租约注册 + Watch） | `go.etcd.io/etcd/client/v3` |
| `discovery/consul/` | Consul 服务发现（Health API + Watch） | `github.com/hashicorp/consul/api` |
| `discovery/nacos/` | Nacos 服务发现（Ephemeral 实例 + Subscribe 推送） | `github.com/nacos-group/nacos-sdk-go/v2` |
| `discovery/k8s/` | Kubernetes 服务发现（Endpoints API + Informer Watch） | `k8s.io/client-go` |

#### `go get github.com/astra-go/astra/config`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `config/nacos/` | Nacos 配置中心（DataID/Group，长轮询热重载，JSON/YAML） | `github.com/nacos-group/nacos-sdk-go/v2` |
| `config/apollo/` | Apollo 配置中心（agollo，长轮询 + `AddChangeListener`） | `github.com/apolloconfig/agollo/v4` |

#### `go get github.com/astra-go/astra/cache`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `cache/memory/` | LRU 内存缓存（容量上限 + TTL，懒过期 + 后台清理） | 无（stdlib `container/list`） |
| `cache/redis/` | Redis 缓存（go-redis/v9，连接池） | `github.com/redis/go-redis/v9` |
| `cache/memcached/` | Memcached 缓存 | `github.com/bradfitz/gomemcache` |

#### `go get github.com/astra-go/astra/lock`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `lock/redis/` | Redis 分布式锁（`SET NX EX` + Lua CAS 释放 + 自动续期） | `github.com/redis/go-redis/v9` |
| `lock/etcd/` | etcd 分布式锁（租约 + `concurrency.Mutex`） | `go.etcd.io/etcd/client/v3` |

#### `go get github.com/astra-go/astra/session`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `session/redis/` | Redis-backed Session（JSON 序列化 + HMAC 签名 + TTL） | `github.com/redis/go-redis/v9` |

#### `go get github.com/astra-go/astra/auth`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `auth/rbac/` | Casbin RBAC 中间件（可插拔 subject/object/action 提取器） | `github.com/casbin/casbin/v2` |
| `auth/oauth2/` | OAuth2/OIDC 授权码流 + PKCE S256 + UserInfo + Cookie StateStore | `golang.org/x/oauth2` |

#### `go get github.com/astra-go/astra/search`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `search/elastic/` | Elasticsearch / OpenSearch（Index / BulkIndex / Search / Delete / CreateIndex） | `github.com/elastic/go-elasticsearch/v8` |

#### `go get github.com/astra-go/astra/notify`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `notify/email/smtp/` | SMTP 邮件（STARTTLS/ImplicitTLS，multipart/alternative + 附件） | 无（stdlib `net/smtp`） |
| `notify/sms/aliyun/` | 阿里云 SMS（HMAC-SHA1 V1 签名，纯 HTTP，无 SDK） | 无 |
| `notify/sms/tencent/` | 腾讯云 SMS（TC3-HMAC-SHA256 签名，纯 HTTP，无 SDK） | 无 |
| `notify/push/fcm/` | FCM HTTP v1（服务账号 JWT + RSA 签名，纯 HTTP，无 SDK） | 无 |

#### `go get github.com/astra-go/astra/mongodb`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `mongodb/` | mongo-driver/v2 泛型封装（`TypedCollection[T]`） | `go.mongodb.org/mongo-driver/v2` |

#### `go get github.com/astra-go/astra/runner`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `runner/cron/` | CronRunner — 包装 `astra/cron`（robfig/cron，进程内调度） | 复用根模块 `cron/` 依赖，无新增 |
| `runner/gocron/` | GocronRunner — go-co-op/gocron/v2（可接分布式锁） | `github.com/go-co-op/gocron/v2` |
| `runner/taskqueue/` | TaskQueueRunner — 包装 `taskqueue` 子模块（分布式 + 持久化） | 复用 `taskqueue/` 依赖，无新增 |
| `runner/dagu/` | DaguRunner — Dagu DAG 编排（HTTP 回调 + YAML 生成） | 无（stdlib `net/http` + `text/template`） |

#### `go get github.com/astra-go/astra/lua`

| 包 | 说明 | 主要外部依赖 |
|----|------|------------|
| `lua/` | gopher-lua 脚本引擎 + Redis EVAL 封装 | `github.com/yuin/gopher-lua` |

---

> **CLI 工具**：`cmd/astractl/`（`astractl new` / `gen`）仅依赖 `gopkg.in/yaml.v3`，作为开发工具独立运行，不影响任何运行时依赖。

---

