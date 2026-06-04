# 操作方案 — 下一步待办执行指南

> 创建时间: 2026-06-04
> 状态: 待执行
> 阻塞项: `go mod download` / `go build` 因 goproxy.cn 不可达全部超时，需网络恢复后执行

---

## #1 P1-6 覆盖率/QPS 验证（预计 0.5 天）

### 前置条件

- 网络恢复（goproxy.cn 可达）
- PostgreSQL 已通过 Docker 启动（`docker ps` 中 `blog-postgres` 为 healthy）
- docker-compose.yml 中 postgres image 已改为 `postgres:latest`（PG18），volumes mount 已适配 PG18 新目录结构

### 步骤 1 — 验证编译

```bash
cd ~/data/project/gotest/astra/examples/reference-blog
go build ./...
```

### 步骤 2 — 运行数据库迁移

```bash
docker exec blog-postgres pg_isready -U bloguser  # 确认 postgres 在线
go run scripts/migrate.go --db-type postgres --dsn "postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable"
```

### 步骤 3 — 运行集成测试

```bash
TEST_DATABASE_DSN="postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable" \
  go test -v -race -tags=integration -coverprofile=coverage.out ./tests/integration/...
```

### 步骤 4 — 检查覆盖率

```bash
go tool cover -func=coverage.out | tail -1
# 目标：总覆盖率 > 80%
```

### 步骤 5 — 运行性能基准

```bash
go test -bench=. -benchmem -timeout=5m ./tests/benchmark/... 2>&1 | tee benchmark-results.txt
```

### 步骤 6 — 检查 QPS

```bash
# 查看基准结果中吞吐量，目标：单实例 ≥ 1000 QPS
grep -E "op/s|ns/op" benchmark-results.txt
```

### 步骤 7 — 更新路线图文档

根据实际数据更新 `architecture-optimization-roadmap.md`：
- 覆盖率数值写入文档
- QPS 数值写入文档
- 将 4 处 `⏳ 集成测试覆盖率 > 80%` 和 `⏳ 性能基准：单实例支持 1000 QPS` 改为 `✅`

---

## #2 CI MQ 矩阵测试（预计 0.5 天）

### 已完成

- `.github/workflows/mq-test-matrix.yml` — GitHub Actions workflow（Kafka / RabbitMQ / NATS / MQTT 四个独立 job + unit + summary）
- `mq/integration_test.go` — 集成测试骨架（每个 MQ 类型有 PubSub + Batch 测试）

### 步骤 1 — 验证集成测试编译

```bash
cd ~/data/project/gotest/astra/mq
go test -c -tags=integration -o /dev/null ./... 2>&1
```

可能需要调整的项：
- `mq.go` 中的构造函数名（`NewRabbitMQProducer` / `NewRabbitMQConsumer` / `NewNatsProducer` / `NewNatsConsumer` / `NewMqttProducer` / `NewMqttConsumer` / `NewKafkaProducer` / `NewKafkaConsumer`）需与 `kafka.go`、`rabbitmq.go`、`nats.go`、`mqtt.go` 中的实际导出函数名一致
- Config struct 字段名（`KafkaProducerConfig.Brokers`、`RabbitMQProducerConfig.URL` 等）需与源码匹配

### 步骤 2 — 检查构造函数命名一致性

```bash
cd ~/data/project/gotest/astra/mq
grep -E "^func New.*Producer|^func New.*Consumer" *.go
# 对比 integration_test.go 中调用的函数名，修正不匹配的
```

### 步骤 3 — 本地跑单个 MQ 测试（以 NATS 为例，最轻量）

```bash
docker run -d --name test-nats -p 4222:4222 nats:2.10-alpine
sleep 3
cd ~/data/project/gotest/astra/mq
MQ_TEST_TYPE=nats NATS_URL=nats://localhost:4222 \
  go test -v -tags=integration -run 'Test.*Nats.*' ./...
docker rm -f test-nats
```

### 步骤 4 — 更新路线图文档

将文档中 P1-4 相关的：

```
⏳ CI 中测试所有 MQ 类型(矩阵构建) **待完成**
```

改为：

```
✅ CI 中测试所有 MQ 类型(矩阵构建) **已完成** (2026-06-04)
```

---

## #3 P1-8 性能基线建立（预计 2 周）

### 步骤 1 — 创建 benchmark CI pipeline

在 `.github/workflows/benchmark.yml`（已有文件）中补充：

```yaml
on:
  pull_request:
    branches: [main]
  release:
    types: [published]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: Run benchmarks
        run: go test -bench=. -benchmem ./... 2>&1 | tee benchmark.txt
      - name: Check regression (fail if >10% slower)
        run: |
          # 对比基准：从 main 分支拉取 prev 结果
          git fetch origin main --depth=1
          git stash
          git checkout origin/main -- benchmark-prev.txt 2>/dev/null || true
          git stash pop
          # 如果有 prev 结果，对比 ns/op
          if [ -f benchmark-prev.txt ]; then
            echo "Comparing with baseline..."
            # 具体对比逻辑待实现
          fi
      - name: Upload benchmark artifact
        uses: actions/upload-artifact@v4
        with:
          name: benchmark-results
          path: benchmark.txt
      - name: Attach to release
        if: github.event_name == 'release'
        run: gh release upload ${{ github.ref_name }} benchmark.txt
```

### 步骤 2 — 建立基准数据文件

```bash
cd ~/data/project/gotest/astra
go test -bench=. -benchmem -timeout=10m ./... 2>&1 | tee docs/benchmark-baseline.txt
git add docs/benchmark-baseline.txt
git commit -m "chore: add performance baseline (P1-8)"
```

### 步骤 3 — 更新路线图文档 P1-8 相关标记

将 P1-8 下的 ⏳ 项改为 ✅ 或记录完成时间。

---

## #4 P1-9 混沌工程验证（预计 2 周）

### 方案

使用 GitHub Actions + docker-compose 进行依赖注入故障测试。

### 步骤 1 — 创建混沌测试目录

```bash
mkdir -p ~/data/project/gotest/astra/tests/chaos
```

### 步骤 2 — 创建混沌测试脚本

`tests/chaos/dependency-failure_test.go` 需覆盖：

- **Redis 宕机**：停止 redis 容器 → 验证服务降级（不应 crash）
- **Kafka 宕机**：停止 kafka 容器 → 验证消息不丢失（重试/持久化）
- **DB 主从切换**：重建只读副本 → 验证读写分离恢复
- **自动恢复**：重启所有服务 → 验证自动重连

### 步骤 3 — 创建 CI workflow

`.github/workflows/chaos-test.yml`：
- 与 MQ 矩阵类似，用 service containers 启动依赖
- 按步骤杀容器注入故障
- 验证服务恢复后状态正确

### 步骤 4 — 集成到主 CI

在 `security.yml` 的 test job 之后加：

```yaml
chaos:
  needs: test
  uses: ./.github/workflows/chaos-test.yml
  if: github.ref == 'refs/heads/main'
```

---

## #5 P2-10 多租户支持（预计 3 周）

待 #1-#4 完成后启动。主要工作：

- X-Tenant-ID 请求级路由（middleware 注入 tenant_id）
- 数据库分片路由（自动注入 tenant_id 过滤）
- 租户配额限制（QPS、并发数、存储空间）
- 租户级监控指标（独立的 Prometheus metrics）

---

## #6 P2-11 配置热更新（预计 2 周）

待 #1-#4 完成后启动。主要工作：

- 接入配置中心（Nacos/Consul/etcd）
- 配置变更实时推送（Watch 机制）
- 热更新场景：限流阈值、日志级别、feature flags

---

## 执行总览

| # | 任务 | 状态 | 阻塞 |
|---|------|------|------|
| 1 | P1-6 覆盖率/QPS 验证 | ⏳ 待执行 | **网络恢复** |
| 2 | CI MQ 矩阵测试 | 🟢 workflow+测试已写，待编译验证 | **网络恢复** |
| 3 | P1-8 性能基线建立 | 📋 方案已出，待执行 | 无 |
| 4 | P1-9 混沌工程验证 | 📋 方案已出，待执行 | 无 |
| 5 | P2-10 多租户支持 | 📋 排期中 | #3 #4 |
| 6 | P2-11 配置热更新 | 📋 排期中 | #3 #4 |

**建议执行顺序**：1 → 2 → 3 → 4，#5/#6 视社区需求排期。
