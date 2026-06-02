# ADR-005: 子模块数量上限策略

## 状态
已接受 (2026-06-02)

## 日期
2026-06-02

## 背景

Astra 当前采用 Monorepo 架构，包含多个独立版本化的子模块。截至 2026-06-02：

**当前状态**:
- 实际子模块数量: **34 个**（排除 examples/e2e/tools）
- go.work 管理的模块: 40 个（含示例）
- CI 全量测试时间: ~20 分钟
- 版本发布工作量: 需要为每个子模块打 Git tag

**主要问题**:
1. **版本发布复杂**: 每次发版需要打 34+ 个 Git tag，容易出错
2. **CI 时间长**: 全量测试需要覆盖所有子模块，耗时较长
3. **模块碎片化**: 同类功能分散在多个独立模块（如 mq/{kafka,rabbitmq,nats,mqtt,pulsar,rocketmq}）
4. **认知负担**: 新贡献者难以快速理解子模块边界和职责划分

随着项目功能持续增加，如果不加控制，模块数量可能继续增长，维护成本将线性甚至指数级上升。

## 决策

### 1. 设定上限

子模块总数（不含 examples/e2e/tools）**不超过 40 个**。

**计算依据**:
```
目标模块数 = 当前数量 × (目标 CI 时间 / 当前 CI 时间)
           = 47 × (15 / 20)
           = 35 个

上限设为 40（预留 5 个缓冲空间）
```

### 2. 强制执行

在 CI 中自动检测子模块数量，超出上限时构建失败，阻止 PR 合并。

实现方式：
```bash
# .github/workflows/security.yml
- name: Check module count limit (ADR-005)
  run: mage -d magefiles checkModuleCount
```

### 3. 合并策略

当子模块数量接近或超过上限时，按以下优先级合并：

#### 优先级 P0（必须合并）

**同类功能模块合并**：
- `mq/{kafka,rabbitmq,nats,mqtt,pulsar,rocketmq}` → `mq/`
  - 收益：减少 5 个模块
  - 方案：统一接口 + 多实现
  
- `config/{nacos,etcd,apollo}` → `config/`
  - 收益：减少 2 个模块
  - 方案：统一配置源接口

- `discovery/{consul,etcd,k8s,nacos}` → `discovery/`
  - 收益：减少 3 个模块
  - 方案：统一服务发现接口

#### 优先级 P1（建议合并）

**低价值模块合并**：
- `lua/` + `rule/` → `rule/`
  - 理由：Lua 作为规则引擎的一种实现方式
  - 收益：减少 1 个模块

#### 合并实施原则

1. **统一接口优先**: 使用接口抽象 + 多实现方式，而非 Build Tags
2. **渐进式迁移**: 保留旧模块 3 个月过渡期，标记为 Deprecated
3. **工具支持**: 提供自动化迁移脚本降低用户成本
4. **文档完善**: 详细的迁移指南 + 破坏性变更说明

## 实施计划

### Phase 1: 文档与检查（Week 1）
- ✅ 创建 ADR-005 文档
- ✅ 实现 `CheckModuleCount()` 函数
- ✅ 集成到 CI 和 Makefile

### Phase 2: 模块合并 - MQ（Week 2-3）
- 设计统一 `Producer` 和 `Consumer` 接口
- 迁移各 MQ 实现到单一 `mq/` 模块
- 更新测试和文档
- 发布 v2.0-beta.1

### Phase 3: 模块合并 - Config & Discovery（Week 4-5）
- 复用 MQ 的合并模式
- 编写用户迁移指南
- 发布 v2.0-rc.1

### Phase 4: 正式发布（Week 6）
- 发布 v2.0.0
- 更新所有文档和示例
- 发布技术博客和社区公告

## 影响

### 得到什么

**技术收益**:
- CI 时间从 20 分钟降至 15 分钟（-25%）
- 版本发布工作量减少 25%
- go.mod 文件数量减少，依赖关系更清晰

**用户体验改善**:
- 导入路径更简洁：`import "github.com/astra-go/astra/mq"`
- 统一的 API 风格：`mq.NewKafkaProducer()` vs `kafka.NewProducer()`
- 新手更容易理解模块职责

### 放弃什么

**权衡**:
- 按需下载受限：用户需要 Kafka 时也会下载 RabbitMQ/NATS 等依赖
- 向后兼容性：现有用户需要修改 import 路径
- 灵活性降低：新增 MQ 类型需要在统一模块中添加

**缓解措施**:
- 提供 3 个月过渡期和兼容层
- 自动化迁移工具（一键脚本）
- Go Module 缓存机制可复用依赖，实际下载时间影响不大

## 监控指标

### 技术指标
- **子模块数量**: 目标 ≤ 40，当前 34 ✅
- **CI 全量测试时间**: 目标 ≤ 15 分钟
- **版本发布时间**: 目标 ≤ 5 分钟

### 用户指标
- **迁移完成率**: v2.0 发布后 3 个月内，80% 用户完成迁移
- **社区反馈**: GitHub Issues/Discussions 中的迁移问题 < 10 个

### 评审机制
- **季度评审**: 每 3 个月检查一次模块数量和合并效果
- **年度回顾**: 评估 ADR-005 是否需要调整上限或策略

## 参考

- [架构优化路线图](../architecture-optimization-roadmap.md)
- [ADR-001: 核心依赖边界](./ADR-001-core-dependency-boundary.md)
- [ADR-002: 模块与插件接口统一](./ADR-002-module-plugin-interface.md)
- [P0-1: 架构适应度函数实施报告](../analysis-p0-1-architecture-fitness-function.md)
- [ADR-005 需求分析报告](../analysis-adr-005-module-count-limit.md)

## 历史

- 2026-06-02: 创建 ADR-005，设定上限 40 个，实现自动化检查
- 2026-06-02: 当前模块数 34 个，无需立即合并，但建立长期约束机制