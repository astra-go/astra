# Performance Baseline Report

> Astra 框架性能基线报告
> 生成时间: 2026-06-04
> 版本: v1.1.1-beta.1

---

## 📋 基准测试场景

### 核心框架 (`bench_test.go`)

| 场景 | 描述 |
|------|------|
| Router_Static | 静态路由匹配 (`/ping`) |
| Router_Static_REST | REST 风格路由 (`/api/v1/users`) |
| Router_Static_100 | 100 条静态路由匹配 |
| Router_Param | 参数路由 (`/users/:id`) |
| Router_Param_Deep | 多层参数路由 (`/a/:b/c/:d`) |
| Router_Regex | 正则路由匹配 |
| Router_Wildcard | 通配符路由 (`/files/*path`) |
| Router_NotFound | 未匹配路由 (404) |

### 全栈集成 (`benchmarks/suite_test.go`)

| 场景 | 描述 |
|------|------|
| Baseline | 空请求 → 空响应 |
| StaticRoute_JSON | 静态路由 + JSON 响应 |
| ParamRoute_JSON | 参数路由 + JSON 响应 |
| POST_BindJSON_Response | POST 请求 + JSON 绑定 + JSON 响应 |
| NotFound | 404 响应 |
| Middleware3_JSON | 3 个中间件 + JSON 响应 |
| Middleware5_JWT_JSON | 5 个中间件 (含 JWT) + JSON 响应 |
| GroupedAPI | 分组路由 + JSON 响应 |
| Parallel_Static | 并发静态路由请求 |
| Parallel_Static_WarmPool | 并发请求 (预热连接池) |

### 参考应用 (`examples/reference-blog/tests/benchmark/`)

| 场景 | 描述 |
|------|------|
| ArticleCreate | 文章创建 (SQLite 内存) |
| ArticlePublish | 文章发布流程 |
| ArticleGetByID | 按 ID 获取文章 (缓存未命中) |
| ArticleListPublished | 分页列表查询 |
| ArticleListPublished_DeepPage | 深度分页 (offset=100) |
| ArticleUpdate | 乐观锁更新 |
| ArticleLike | 原子点赞计数 |
| ArticleDelete | 软删除 |
| AuthRegister | 用户注册 (含 bcrypt) |
| AuthLogin | 用户登录 (bcrypt 校验) |
| AuthValidateToken | JWT Token 验证 |
| ConcurrentReads | 并发文章读取 |
| ConcurrentLikes | 并发点赞操作 |

---

## 🎯 性能目标

| 场景类别 | 目标 QPS | P99 延迟 | 内存占用 |
|---------|---------|---------|---------|
| 空路由 (Baseline) | 50,000+ | < 1ms | < 50MB |
| 静态路由 + JSON | 30,000+ | < 2ms | < 100MB |
| 参数路由 + JSON | 25,000+ | < 3ms | < 100MB |
| 中间件链 (3-5) | 15,000+ | < 5ms | < 150MB |
| POST + JSON 绑定 | 10,000+ | < 10ms | < 150MB |
| ORM 查询 (SQLite 内存) | 5,000+ | < 20ms | < 200MB |
| ORM 写入 (SQLite 内存) | 3,000+ | < 20ms | < 200MB |
| 认证 (bcrypt) | 500+ | < 50ms | < 200MB |
| JWT 验证 | 50,000+ | < 1ms | < 50MB |

---

## 🚦 CI 性能门禁

### 触发条件

- **PR 到 main**: 自动运行基准测试对比
- **push 到 main**: 自动记录基线数据

### 退化检测规则

- 使用 `benchstat` 对比当前分支 vs 基线分支
- **退化阈值**: ns/op 或 allocs 增长 >10% → CI 失败
- **PR 评论**: 自动在 PR 中发布基准测试对比结果

### CI Workflow

```
benchmark.yml
├── benchmark-core          # 核心框架基准测试 + 退化检测
├── benchmark-reference-app # 参考应用基准测试 + 退化检测
└── benchmark-report        # 合并报告生成 (仅 main push)
```

### 本地运行

```bash
# 核心框架
make bench

# 参考应用
cd examples/reference-blog && make benchmark

# 对比基线
make bench-compare

# 快速验证
make bench-quick
```

---

## 📊 基线数据 (待首次 CI 运行后填充)

> 首次 push 到 main 后，CI 将自动生成基线数据并存储到 `benchmarks/` 目录。
> 后续 PR 将自动与基线对比，退化 >10% 则阻止合并。

**状态**: ⏳ 等待 v1.1.1-beta.1 release CI 自动生成基线

---

## 🔧 维护说明

1. **添加新基准**: 在 `bench_test.go` 或 `benchmarks/suite_test.go` 中添加 `func Benchmark*`
2. **更新基线**: 大版本发布时，更新 `benchmarks/core-latest.txt`
3. **调整阈值**: 修改 `benchmark.yml` 中的退化检测逻辑
4. **本地对比**: 使用 `golang.org/x/perf/cmd/benchstat` 对比两次运行结果

```bash
go install golang.org/x/perf/cmd/benchstat@latest
benchstat old.txt new.txt
```
