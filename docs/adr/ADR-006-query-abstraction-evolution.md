# ADR-006: Query Abstraction Evolution

## Status
Accepted — 2026-06-03

## Date
2026-06-03

## Context

Astra 的数据访问层通过 `contract.Repository[T]` 接口抽象 ORM 实现，当前设计存在以下问题：

### 1. Query 表达能力受限

`FindWhere(ctx, query any, args ...any)` 使用 `any` 类型接受查询条件，缺乏类型安全：

```go
// 当前：弱类型，编译期无法检查
repo.FindWhere(ctx, "status = ? AND age > ?", 1, 18)
repo.FindWhere(ctx, map[string]any{"status": 1})  // 另一种写法，无统一规范
```

问题：
- 字段名拼写错误只能在运行时发现
- 不同 ORM 实现对 `query` 参数的解析方式不一致（GORM 字符串 vs ent predicate）
- 无法在 IDE 中获得自动补全
- 复杂查询（OR、子查询、JOIN）无法表达，只能回退到原生 SQL

### 2. 非关系型数据源适配困难

当前 Repository 接口隐含关系型数据库假设（FindByID、Updates by ID、Delete by ID）。
但 Astra 还支持：
- `search/elastic` — Elasticsearch 全文检索
- `orm/clickhouse` — 列式分析存储
- `mongodb` — 文档数据库

这些数据源的访问模式与关系型 Repository 差异较大，强行套用同一接口导致：
- 方法语义不一致（ClickHouse 的 "Update" 实际是 ALTER）
- 空实现过多（Search 不需要 Delete by ID）
- 无法暴露数据源特有能力（ES 的聚合、ClickHouse 的物化视图）

### 3. 跨数据源查询无统一入口

业务层同时操作 MySQL（事务数据）+ ES（搜索）+ ClickHouse（分析）时，需要分别获取三个 Repository 实例，缺乏统一查询协调机制。

## Decision

采用 **分层查询抽象** 策略，不急于一步到位，分阶段演进：

### Phase 1: 类型安全查询构建器（当前阶段）

为 `contract.Repository[T]` 增加类型安全的查询构建器，保持接口兼容：

```go
// 新增 QuerySpec 类型，替代 any query 参数
type QuerySpec[T any] struct {
    predicates []Predicate
    orders    []Order
    limit     int
    offset    int
}

// Predicate 支持编译期字段名检查（go generate）
type Predicate struct {
    Field    string
    Operator Op    // Eq, Ne, Gt, Lt, In, Between, Like...
    Value    any
}

// 使用方式
specs := QuerySpec[User]{
    Where: Field("status").Eq(1).And(Field("age").Gt(18)),
    Order: By("created_at", Desc),
    Limit: 20,
}

// Repository 接口扩展（向后兼容）
type Repository[T any] interface {
    // ... 现有方法保留 ...
    
    // 新增：类型安全查询
    Query(ctx context.Context, spec QuerySpec[T]) ([]T, error)
}
```

**约束**：
- 现有 `FindWhere` 方法标记 `Deprecated`，3 个版本后移除
- `QuerySpec` 不支持 JOIN 和子查询（属于跨实体操作，归入 Service 层）
- 生成的字段名常量由 `go generate` 保证编译期安全

### Phase 2: 多数据源查询接口（未来）

为非关系型数据源定义专用接口，替代强行套用 Repository：

```go
// 搜索专用接口
type SearchIndex[T any] interface {
    Search(ctx context.Context, query SearchQuery) ([]T, int64, error)
    Index(ctx context.Context, doc *T) error
    BulkIndex(ctx context.Context, docs []T) error
}

// 分析专用接口
type AnalyticsStore[T any] interface {
    Query(ctx context.Context, spec AnalyticSpec) ([]T, error)
    Aggregate(ctx context.Context, spec AggSpec) ([]AggResult, error)
}
```

**约束**：
- 不再强制所有数据源实现 Repository 接口
- 各接口方法语义明确，无空实现
- Phase 2 仅在出现第三个非关系型数据源需求时启动

### Phase 3: 统一查询协调器（远期）

```go
// QueryCoordinator 跨数据源查询路由
type QueryCoordinator interface {
    // Route 根据查询特征选择最优数据源
    Route(ctx context.Context, query QuerySpec) (DataSource, error)
    // FanOut 并行查询多个数据源并合并结果
    FanOut(ctx context.Context, queries []QuerySpec) ([]Result, error)
}
```

**约束**：
- Phase 3 仅在业务层确实需要跨数据源查询时启动
- 避免过早抽象，先让 Phase 1/2 在实际使用中验证

## Options Considered

### Option A: 保持现状（any query 参数）
- **Pros**: 零迁移成本，GORM 完美兼容
- **Cons**: 类型不安全，非关系型数据源适配困难，长期技术债

### Option B: 全面重写为 CQRS 模式
- **Pros**: 读写分离，查询优化空间大
- **Cons**: 改动面太大，当前团队规模不匹配，过度设计

### Option C: 分层演进（选定方案）
- **Pros**: 渐进改进，向后兼容，每阶段可独立交付
- **Cons**: 过渡期存在两套查询 API（Deprecated + 新 API）

## Consequences

### 得到
- **类型安全**: 编译期检查字段名和操作符，减少运行时错误
- **可扩展**: 非关系型数据源有专用接口，不再强行适配 Repository
- **向后兼容**: 现有代码无需修改，新旧 API 并存 3 个版本

### 放弃
- **单一查询接口**: 不同数据源使用不同接口，业务层需感知数据源类型
- **任何 query any 的灵活性**: 新 API 更严格，复杂场景需回退到原生查询

### 风险与缓解
| 风险 | 缓解措施 |
|------|----------|
| go generate 增加构建步骤 | Makefile 中集成，CI 门禁检查 |
| 两套 API 并存期认知负担 | Deprecated 注解 + lint 规则 |
| Phase 2/3 可能永远不会启动 | 这是优点——不过度设计 |

## References

- ADR-001: Core Module Dependency Boundary
- ADR-005: 子模块数量上限策略
- `contract/orm.go` — 当前 Repository[T] 定义
- `orm/gorm.go` — GORM 实现
- `orm/clickhouse/clickhouse.go` — ClickHouse 实现
