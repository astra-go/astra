# Performance Benchmark Results

性能基准测试脚本和 Astra Showcase 应用的结果。

## 快速开始

### 前置要求

```bash
# macOS
brew install wrk

# Ubuntu/Debian
apt-get install wrk

# 安装 ghz 用于 gRPC 基准测试（可选）
go install github.com/bojand/ghz/cmd/ghz@latest
```

### 运行基准测试

```bash
# 1. 启动服务
cd ../
docker-compose up -d
go run ./cmd/api &
go run ./cmd/grpc &

# 2. 生成 JWT token（用于认证端点）
export JWT_TOKEN=$(go run ./tools/gentoken/main.go)

# 3. 运行基准测试
cd perf/
chmod +x benchmark.sh
./benchmark.sh
```

### 结果

结果保存到 `perf/results/` 并带时间戳：
- `01_health_<timestamp>.txt` - 健康检查（空路由）
- `02_products_<timestamp>.txt` - 产品列表（缓存）
- `03_orders_<timestamp>.txt` - 订单创建（写）
- `04_grpc_stock_<timestamp>.txt` - gRPC 库存查询
- `05_grpc_batch_<timestamp>.txt` - gRPC 批量查询
- `BENCHMARK_<timestamp>.md` - 摘要报告

### 解析结果

```bash
go run parse_results.go results/
```

## 基准测试目标

| 场景 | 目标 QPS | 目标 P99 延迟 | 说明 |
|----------|-----------|-------------------|-------------|
| **健康检查** | 10,000+ | < 1ms | 空路由基准 |
| **产品列表** | 5,000+ | < 5ms | Redis 缓存命中 |
| **订单创建** | 1,000+ | < 20ms | DB 写入 + 库存扣减 |
| **gRPC 库存** | 3,000+ | < 5ms | gRPC unary 调用 |
| **gRPC 批量** | 2,000+ | < 10ms | gRPC 批量查询 |

## k6 负载测试

更全面的负载测试场景：

```bash
# 安装 k6
brew install k6  # macOS
# 或从 https://k6.io/docs/getting-started/installation/ 下载

# 冒烟测试（完整性检查）
k6 run --env SCENARIO=smoke --env JWT=$JWT_TOKEN load_test.js

# 负载测试（默认 - 增加到 50 VUs）
k6 run --env JWT=$JWT_TOKEN load_test.js

# 压力测试（增加到 200 VUs）
k6 run --env SCENARIO=stress --env JWT=$JWT_TOKEN load_test.js
```

### k6 阈值

- HTTP 请求持续时间 P95 < 500ms
- 错误率 < 1%
- 订单创建 P95 < 1000ms

## 持续集成

添加到 `.github/workflows/performance.yml`：

```yaml
name: Performance Regression Test

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.25'
      
      - name: Install wrk
        run: sudo apt-get install -y wrk
      
      - name: Start services
        run: |
          cd examples/showcase
          docker-compose up -d
          go run ./cmd/api &
          sleep 10
      
      - name: Run benchmarks
        run: |
          cd examples/showcase/perf
          export JWT_TOKEN=$(go run ../tools/gentoken/main.go)
          ./benchmark.sh
      
      - name: Parse results
        run: |
          cd examples/showcase/perf
          go run parse_results.go results/
      
      - name: Upload results
        uses: actions/upload-artifact@v3
        with:
          name: benchmark-results
          path: examples/showcase/perf/results/
```

## 解读结果

### wrk 输出

```
Running 30s test @ http://localhost:8080/health
  4 threads and 100 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     1.23ms    2.45ms  50.12ms   89.45%
    Req/Sec     2.50k   450.00    3.10k    75.00%
  Latency Distribution
     50%    0.95ms
     75%    1.20ms
     90%    2.10ms
     99%    5.80ms
  299456 requests in 30.01s, 45.67MB read
Requests/sec:   9978.52
Transfer/sec:      1.52MB
```

**关键指标**：
- **Requests/sec**：总吞吐量（QPS）
- **Latency P99**：99% 请求在此时间内完成
- **Latency Avg**：平均响应时间
- **Transfer/sec**：使用的网络带宽

### ghz 输出（gRPC）

```
Summary:
  Count:        100000
  Total:        33.12 s
  Slowest:      25.34 ms
  Fastest:      0.15 ms
  Average:      1.65 ms
  Requests/sec: 3019.32

Latency distribution:
  10% in 0.89 ms
  25% in 1.12 ms
  50% in 1.45 ms
  75% in 1.89 ms
  90% in 2.45 ms
  95% in 3.12 ms
  99% in 4.89 ms
```

**关键指标**：
- **Requests/sec**：gRPC QPS
- **Latency P99**：第 99 百分位延迟
- **Error rate**：成功率

## 故障排除

### QPS 低

1. **检查 CPU 使用**：`htop` 或 Activity Monitor
2. **检查 DB 连接**：`SELECT count(*) FROM pg_stat_activity;`
3. **检查 Redis**：`redis-cli INFO stats`
4. **启用 pprof**：`go tool pprof http://localhost:8080/debug/pprof/profile`

### P99 延迟高

1. **GC 压力**：检查 `GODEBUG=gctrace=1`
2. **DB 慢查询**：启用 PostgreSQL 慢查询日志
3. **网络延迟**：使用本地 Docker 而非远程
4. **锁竞争**：用 pprof 检查互斥锁热点

### 常见问题

**"Service not available"**
```bash
# 检查服务是否运行
docker-compose ps
curl http://localhost:8080/health

# 检查日志
docker-compose logs postgres
go run ./cmd/api
```

**"ghz: command not found"**
```bash
go install github.com/bojand/ghz/cmd/ghz@latest
export PATH=$PATH:$(go env GOPATH)/bin
```

**"JWT token invalid"**
```bash
# 重新生成 token
export JWT_TOKEN=$(go run ./tools/gentoken/main.go)
echo $JWT_TOKEN  # 验证非空
```

## 最佳实践

1. **预热**：基准测试前运行一些请求以预热缓存
2. **稳定环境**：基准测试期间关闭不必要的应用
3. **多次运行**：运行 3-5 次取中位数
4. **真实数据**：用类生产数据量填充数据库
5. **监控资源**：测试期间观察 CPU、内存和网络