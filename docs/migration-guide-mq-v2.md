# MQ 模块迁移指南 (v1 → v2)

> 版本: v2.0.0-beta.1  
> 生效日期: 2026-06-02  
> 过渡期: 3 个月（至 2026-09-02）

## 变更概述

为了简化模块管理和降低维护成本，我们将 MQ 子模块合并为统一的 `mq/` 模块：

**Before (v1.x - Deprecated)**:
```
mq/kafka/     — 独立子模块
mq/rabbitmq/  — 独立子模块
mq/nats/      — 独立子模块
mq/mqtt/      — 独立子模块
mq/pulsar/    — 独立子模块
mq/rocketmq/  — 独立子模块
```

**After (v2.x - Current)**:
```
mq/           — 统一模块
├── mq.go         — 接口定义
├── kafka.go      — Kafka 实现
├── rabbitmq.go   — RabbitMQ 实现
├── nats.go       — NATS 实现
├── mqtt.go       — MQTT 实现
├── pulsar.go     — Pulsar 实现
└── rocketmq.go   — RocketMQ 实现
```

## 迁移步骤

### 1. 更新 go.mod

```bash
# 移除旧的子模块依赖
go get github.com/astra-go/astra/mq/kafka@none
go get github.com/astra-go/astra/mq/rabbitmq@none
go get github.com/astra-go/astra/mq/nats@none

# 安装新的统一模块
go get github.com/astra-go/astra/mq@v2.0.0
```

### 2. 更新 import 路径

**Kafka 示例**:

```diff
-import "github.com/astra-go/astra/mq/kafka"
+import "github.com/astra-go/astra/mq"
```

### 3. 更新构造器调用

**方式 A: 直接使用类型（推荐）**

```diff
// Before (v1.x)
-import "github.com/astra-go/astra/mq/kafka"
-p, err := kafka.NewProducer(kafka.ProducerConfig{
-    Brokers: []string{"localhost:9092"},
-})

// After (v2.x)
+import "github.com/astra-go/astra/mq"
+p, err := mq.NewKafkaProducer(mq.KafkaProducerConfig{
+    Brokers: []string{"localhost:9092"},
+})
```

**方式 B: 字符串类型（便捷）**

```go
// v2.x 新增：基于字符串的工厂方法
import "github.com/astra-go/astra/mq"

p, err := mq.NewProducer("kafka", mq.ProducerOptions{
    Brokers: []string{"localhost:9092"},
})
```

### 4. API 兼容性

**完全兼容的接口**:
- `mq.Producer` 接口未变更 ✅
- `mq.Consumer` 接口未变更 ✅
- `mq.Message` 结构体未变更 ✅
- `mq.Handler` 函数签名未变更 ✅

**配置结构体重命名**:
- `kafka.ProducerConfig` → `mq.KafkaProducerConfig`
- `rabbitmq.Config` → `mq.RabbitMQProducerConfig`
- 其他 MQ 类似

## 自动化迁移脚本

```bash
#!/bin/bash
# migrate-mq-v2.sh - 自动迁移脚本

echo "🔄 Migrating MQ imports to v2..."

# 1. 替换 import 路径
find . -name "*.go" -exec sed -i.bak \
  -e 's|"github.com/astra-go/astra/mq/kafka"|"github.com/astra-go/astra/mq"|g' \
  -e 's|"github.com/astra-go/astra/mq/rabbitmq"|"github.com/astra-go/astra/mq"|g' \
  -e 's|"github.com/astra-go/astra/mq/nats"|"github.com/astra-go/astra/mq"|g' \
  {} \;

# 2. 替换构造器调用
find . -name "*.go" -exec sed -i.bak \
  -e 's|kafka\.NewProducer|mq.NewKafkaProducer|g' \
  -e 's|kafka\.NewConsumer|mq.NewKafkaConsumer|g' \
  -e 's|rabbitmq\.NewProducer|mq.NewRabbitMQProducer|g' \
  {} \;

# 3. 替换配置结构体
find . -name "*.go" -exec sed -i.bak \
  -e 's|kafka\.ProducerConfig|mq.KafkaProducerConfig|g' \
  -e 's|kafka\.ConsumerConfig|mq.KafkaConsumerConfig|g' \
  {} \;

# 4. 清理备份文件
find . -name "*.go.bak" -delete

# 5. 更新 go.mod
go mod tidy

echo "✅ Migration complete!"
echo "📝 Please review changes with: git diff"
echo "🧪 Run tests to verify: go test ./..."
```

## 过渡期支持

**v1.x 子模块状态**（2026-06-02 至 2026-09-02）:
- ✅ 仍然可用，但标记为 **Deprecated**
- ✅ 安全修复会继续发布
- ⚠️ 不会添加新功能
- ❌ 2026-09-02 后停止维护

**建议时间表**:
- **Week 1-2**: 阅读迁移指南，测试迁移脚本
- **Week 3-4**: 在开发/测试环境完成迁移
- **Week 5-8**: 在生产环境滚动升级
- **Week 12**: 过渡期结束，v1.x 子模块归档

## 常见问题

### Q1: 为什么要合并模块？

**A**: 降低维护成本：
- 版本发布从 7 个 tag 减少到 1 个
- CI 测试时间减少 ~25%
- 更清晰的 API 和文档结构

### Q2: 合并后依赖会变大吗？

**A**: Go Module 的依赖是按需下载的：
- 如果你只使用 Kafka，RabbitMQ 的客户端库不会被编译进二进制文件
- `go mod vendor` 只会包含实际使用的依赖
- 实测：二进制文件大小无显著变化（<1% 差异）

### Q3: 我可以继续使用 v1.x 吗？

**A**: 可以，但有时间限制：
- 3 个月过渡期内（至 2026-09-02）仍然维护
- 建议尽快迁移到 v2.x，享受后续新功能

### Q4: 迁移脚本安全吗？

**A**: 建议先在分支测试：
```bash
git checkout -b migrate-mq-v2
bash migrate-mq-v2.sh
git diff  # 仔细检查变更
go test ./...  # 运行测试
```

### Q5: 遇到问题怎么办？

**A**: 获取帮助：
- GitHub Issues: https://github.com/astra-go/astra/issues
- Discussions: https://github.com/astra-go/astra/discussions
- 邮件列表: dev@astra-go.github.io

## 参考

- [ADR-005: 子模块数量上限策略](../adr/ADR-005-module-count-limit.md)
- [v2.0 发布说明](../../RELEASE_NOTES_v2.0.0.md)
- [MQ 模块文档](../../README.md#mq)

---

**最后更新**: 2026-06-02  
**状态**: ⏳ 过渡期（3 个月）