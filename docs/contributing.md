# 贡献指南

感谢你考虑为 Astra 做贡献！

## 开发环境

```bash
git clone https://github.com/astra-go/astra.git
cd astra
go mod download

# 运行所有测试
go test ./... -race

# 运行 vet + staticcheck
go vet ./...
staticcheck ./...
```

### Build Tags

部分模块使用 build tags 控制后端编译，减少二进制体积和依赖。不指定 tag 时只编译默认后端。

| 模块 | 可用 Build Tags | 说明 |
|------|----------------|------|
| `mq` | `kafka`, `rabbitmq`, `nats`, `mqtt`, `pulsar`, `rocketmq` | 消息队列后端 |
| `discovery` | `consul`, `etcd`, `k8s`, `nacos` | 服务发现后端 |
| `config` | `nacos`, `etcd`, `apollo` | 配置中心后端 |
| `runner` | `cron`, `dagu`, `gocron`, `tqrunner` | 任务运行后端 |
| `taskqueue` | `kafka`, `mongo`, `rabbitmq`, `redis`, `rocketmq` | 任务队列后端 |
| `notify` | `email`, `push`, `sms` | 通知通道后端 |

```bash
# 仅编译特定后端
go build -tags=kafka,rabbitmq ./mq/...
go test -tags=cron ./runner/...

# 编译所有后端
go build -tags=alltags ./...

# 测试所有后端
go test -tags=alltags ./...
```

> **提示**: 使用 `-tags=alltags` 可一次性编译所有后端实现，适合 CI 全量测试。本地开发建议只指定需要的后端以加快编译速度。

## 提交规范

提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
feat(middleware): add CSRF double-submit cookie support
fix(netengine): drain addCh on shutdown to avoid conn leak
docs(migration): add v0-to-v1 migration guide
test(alert): add For duration delay test
```

| 前缀 | 用途 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档变更 |
| `test` | 测试相关 |
| `refactor` | 重构（无行为变更） |
| `perf` | 性能优化 |
| `chore` | 构建/CI 相关 |

## Pull Request 流程

1. Fork 仓库，创建特性分支 `feat/your-feature`
2. 编写代码 + 测试（覆盖率不低于现有水平）
3. 确认 `go test ./... -race` 通过
4. 更新 `CHANGELOG.md` 的 `[Unreleased]` 区块
5. 提交 PR，填写模板描述变更原因

## 文档更新

```bash
pip install mkdocs-material

# 本地预览
mkdocs serve

# 构建静态站点
mkdocs build
```

## 代码审查规范

### Reviewer 职责

- 在 PR 提交后 **2 个工作日内** 完成首轮审查
- 审查重点：正确性、安全性、与现有架构的一致性
- 涉及 `middleware/security/`、`auth/`、`jwt/`、`binding/` 的变更，需安全负责人（`@astra-go/security`）额外审查

### 审查 Checklist

**功能正确性**
- [ ] 逻辑是否符合需求，边界条件是否处理
- [ ] 错误路径是否正确返回，错误信息是否合适

**安全性**
- [ ] 输入校验是否完整（特别是 binding 层）
- [ ] 是否存在 SQL 注入、XSS、SSRF 等风险
- [ ] 权限控制是否正确，数据隔离是否到位
- [ ] 敏感数据（密钥、token）是否通过 `SecretString` 保护

**代码质量**
- [ ] 命名是否清晰，无需注释即可理解
- [ ] 是否引入了不必要的抽象或依赖
- [ ] 是否与现有代码风格一致

**测试**
- [ ] 新增逻辑是否有对应测试
- [ ] 测试是否覆盖了正常路径和异常路径
- [ ] Codecov 报告中 patch 覆盖率是否 ≥ 60%

**破坏性变更**
- [ ] 公开 API 是否有变更，是否更新了 `.api/next.txt`
- [ ] CHANGELOG.md 是否已更新

### 合并条件

PR 满足以下全部条件才可合并：

1. CI 所有 job 通过（lint、test、api-compat、security、tidy、replaces）
2. 至少 1 名 Reviewer 批准（安全模块需安全负责人批准）
3. Codecov patch 覆盖率 ≥ 60%
4. 无未解决的 Review 评论

### 本地验证命令

```bash
# 格式检查
gofmt -l ./...

# 静态分析（仅新增代码）
make lint

# 全量静态分析
make lint-all

# 安全漏洞扫描
make vuln

# 测试（含 race detector）
go test -race ./...

# API 兼容性检查
bash scripts/apicheck.sh --check

# 架构适应度检查（Architecture Fitness Function）
make check-arch
```

---

## 架构约束

### 核心依赖边界（ADR-001）

**原则**: 核心模块 `github.com/astra-go/astra` **禁止**直接依赖重型基础设施包。

**背景**: 保持框架核心轻量化，允许用户按需选择具体实现（GORM vs sqlx、Redis vs Memcached 等）。

**禁止的依赖类别**:

| 类别 | 禁止包示例 | 应使用 |
|------|-----------|--------|
| **ORM 库** | `gorm.io/gorm`<br>`gorm.io/driver/*` | `github.com/astra-go/astra/orm`<br>或 `contract.Repository[T]` 接口 |
| **缓存客户端** | `github.com/redis/go-redis`<br>`github.com/bradfitz/gomemcache` | `github.com/astra-go/astra/cache` |
| **消息队列** | `github.com/segmentio/kafka-go`<br>`github.com/rabbitmq/amqp091-go`<br>`github.com/nats-io/nats.go` | `github.com/astra-go/astra/mq` |
| **数据库驱动** | `github.com/lib/pq`<br>`github.com/go-sql-driver/mysql` | `github.com/astra-go/astra/orm` |
| **NoSQL** | `go.mongodb.org/mongo-driver`<br>`github.com/elastic/go-elasticsearch` | `github.com/astra-go/astra/mongodb`<br>`github.com/astra-go/astra/search` |
| **可观测性** | `go.opentelemetry.io/otel/**`<br>`github.com/prometheus/client_golang` | `github.com/astra-go/astra/otel`<br>`github.com/astra-go/astra/observability` |
| **服务发现** | `github.com/hashicorp/consul/api`<br>`go.etcd.io/etcd/client/v3` | `github.com/astra-go/astra/discovery`<br>`github.com/astra-go/astra/config` |
| **JWT 库** | `github.com/golang-jwt/jwt/**` | `github.com/astra-go/astra/auth` |

**例外情况**: 
- `go.opentelemetry.io/otel/trace/noop` - noop tracer 允许（零依赖）
- 标准库和 `golang.org/x/*` 官方扩展包
- 轻量工具库（如 `github.com/goccy/go-json`, `github.com/go-playground/validator`）

**自动检查**: 

CI 会自动检测核心模块的依赖边界违规：

```bash
# 本地运行架构检查
make check-arch

# 输出示例（违规时）：
# ❌ Architecture violation detected (1 issues):
#
# 1. Core module depends on: gorm.io/gorm
#    Reason: ORM libraries must be in orm/ sub-module (ADR-001)
#    Fix: Use contract.Repository[T] interface or import github.com/astra-go/astra/orm
#    Documentation: docs/adr/ADR-001-core-dependency-boundary.md
```

**违规修复示例**:

```go
// ❌ 错误：核心模块直接依赖 GORM
package astra

import "gorm.io/gorm"

func (app *App) ConnectDB() *gorm.DB {
    return gorm.Open(...)
}

// ✅ 正确：通过接口解耦
package astra

type Repository[T any] interface {
    FindByID(id string) (T, error)
    Save(entity T) error
}

func (app *App) UseRepository[T any](repo Repository[T]) {
    // 用户自行决定使用 GORM、sqlx 或其他实现
}
```

**详细文档**: 
- [ADR-001: 核心依赖边界](./adr/ADR-001-core-dependency-boundary.md)
- [架构适应度函数实现](./analysis-p0-1-architecture-fitness-function.md)

---

### 循环依赖检测

**原则**: 子模块间不允许循环依赖。

**检测方法**: `make check-arch` 会自动执行 `CheckCircularDeps()` 检测。

**常见问题**:
- `cache/` 依赖 `orm/`，同时 `orm/` 又依赖 `cache/` → ❌ 循环
- 解决方案：提取公共接口到 `contract/` 包，或反转依赖方向

---

### 子模块数量上限（ADR-005）

**原则**: 子模块总数（不含 examples/e2e/tools）不超过 **40 个**。

**背景**: 控制模块数量可以：
- 降低版本发布复杂度（减少 Git tag 数量）
- 缩短 CI 全量测试时间
- 降低新贡献者的认知负担

**检测方法**: `make check-arch` 会自动执行 `CheckModuleCount()` 检测。

**当前状态**:
```bash
$ make check-arch
🔍 Checking sub-module count limit (ADR-005)...
✅ Module count check passed (34/40 modules)
```

**合并策略**:
当模块数量接近上限时，优先合并同类功能模块：
- **MQ 模块**: `mq/{kafka,rabbitmq,nats,mqtt,pulsar,rocketmq}` → `mq/`
- **Config 模块**: `config/{nacos,etcd,apollo}` → `config/`
- **Discovery 模块**: `discovery/{consul,etcd,k8s,nacos}` → `discovery/`

**新增模块时**:
1. 确认没有现有模块可以扩展
2. 评估是否会导致超过上限
3. 如果接近上限，需要先合并低价值模块
4. 在 PR 中说明新增模块的必要性

**详细文档**: 
- [ADR-005: 子模块数量上限策略](./adr/ADR-005-module-count-limit.md)
- [ADR-005 需求分析报告](./analysis-adr-005-module-count-limit.md)

---
