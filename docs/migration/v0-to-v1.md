# 迁移指南：v0.x → v1.0

本指南涵盖从任意 v0.x 版本升级到 **v1.0.0** 所需的全部变更。

!!! warning "生产环境升级建议"
    在测试环境验证通过后再升级生产环境。建议先升到 v0.10.x（最终 0.x 版本）
    再升到 v1.0，以便利用弃用警告发现问题。

---

## 一键检查脚本

在项目根目录运行以下脚本，自动扫描需要手动处理的代码模式：

```bash
#!/usr/bin/env bash
echo "=== 检查 v0→v1 迁移问题 ==="

# 1. 旧 SetLogger 调用
grep -rn "\.SetLogger(" --include="*.go" . && echo "⚠️  请改用 astra.WithLogger()"

# 2. c.JSON(data) 两参数缺失
grep -rn 'c\.JSON([^2]' --include="*.go" . | grep -v 'c\.JSON(2[0-9][0-9]' && \
    echo "⚠️  c.JSON 需要状态码作为第一参数"

# 3. 旧 http.HandlerFunc 未包装
grep -rn 'app\.\(GET\|POST\|PUT\|DELETE\|PATCH\)\(.*http\.HandlerFunc' --include="*.go" . && \
    echo "⚠️  请用 astra.WrapH() 包装 http.HandlerFunc"

# 4. TrustedProxies 字符串 setter（v0.9 已移除）
grep -rn '\.SetTrustedProxies(' --include="*.go" . && \
    echo "⚠️  请改用 astra.WithTrustedProxies() Option"

echo "=== 扫描完成 ==="
```

---

## 变更详情

### 1. Handler 签名（v0.2.0 引入，v1.0 最终确认）

**v0.1.x（已移除）**：

```go
// 旧：标准库签名
app.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("hello"))
})
```

**v1.0（当前）**：

```go
// 新：Astra Context 签名
app.GET("/hello", func(c *astra.Context) error {
    return c.String(200, "hello")
})

// 迁移已有 http.HandlerFunc
app.GET("/legacy", astra.WrapH(myOldHandler))
```

**迁移步骤**：

```bash
# 大量文件时可用 sed 辅助（需人工审查）
# 将 func(w http.ResponseWriter, r *http.Request) 改为 func(c *astra.Context) error
```

---

### 2. `c.JSON` / `c.XML` 需要状态码（v0.3.0 引入，v0.x 旧 API 在 v1.0 移除）

**旧（v0.2.x）**：

```go
c.JSON(data)        // 隐式 200
c.XML(data)         // 隐式 200
```

**新（v1.0）**：

```go
c.JSON(200, data)   // 显式状态码
c.XML(200, data)
c.JSON(201, created)
```

**批量迁移**：

```bash
# 注意：仅作参考，需人工确认每处的期望状态码
sed -i 's/c\.JSON(\([^,0-9]\)/c.JSON(200, \1/g' **/*.go
sed -i 's/c\.XML(\([^,0-9]\)/c.XML(200, \1/g'  **/*.go
```

---

### 3. `App.SetLogger` 已移除（v0.6.0 弃用，v1.0 移除）

**旧**：

```go
app := astra.New()
app.SetLogger(myLogger)
```

**新**：

```go
app := astra.New(astra.WithLogger(myLogger))
```

---

### 4. `App.SetTrustedProxies` 已移除（v0.9.0 弃用，v1.0 移除）

**旧**：

```go
app.SetTrustedProxies("10.0.0.0/8")
```

**新**：

```go
app := astra.New(astra.WithTrustedProxies("10.0.0.0/8", "172.16.0.0/12"))
```

!!! note "行为变更"
    v0.9+ 起，受信代理 CIDR 在 `New()` 时一次性编译为 `*net.IPNet`；
    `ClientIP()` 执行右到左 XFF 遍历。旧版本的左到右遍历**不再支持**。

---

### 5. `OnStart` / `OnStop` 钩子串行化（v0.8.0）

v0.7.x 及之前，多个钩子并发执行（这是一个 bug）。
v0.8.0+ 改为**按注册顺序串行执行**。

**需要并发执行的场景**：

```go
// 旧：依赖并发副作用（不推荐）
app.OnStart(startDB)
app.OnStart(startCache)  // v0.7.x 中与 startDB 并发运行

// 新：在单个钩子内自行并发
app.OnStart(func(ctx context.Context) error {
    var eg errgroup.Group
    eg.Go(func() error { return startDB(ctx) })
    eg.Go(func() error { return startCache(ctx) })
    return eg.Wait()
})
```

---

### 6. `middleware.RateLimit` cleanup goroutine 控制（v0.9.0 / v1.0）

`RateLimit(rate, burst)` 内部启动的 cleanup goroutine 不再永久运行于
`context.Background()`——当 `App` 停止时，钩子应取消它。

**推荐用法（v1.0）**：

```go
// 方式 A：NewRateLimiter（最简洁，测试友好）
mw, stop := middleware.NewRateLimiter(100, 20)
app.OnStop(func(_ context.Context) error { stop(); return nil })
app.Use(mw)

// 方式 B：通过 Context 字段控制
ctx, cancel := context.WithCancel(context.Background())
app.OnStop(func(_ context.Context) error { cancel(); return nil })
app.Use(middleware.RateLimitWithConfig(middleware.RateLimitConfig{
    Rate: 100, Burst: 20, Context: ctx,
}))
```

**如不控制**（顶层应用可接受）：

```go
app.Use(middleware.RateLimit(100, 20))
// goroutine 随进程退出，不泄漏
```

---

### 7. 路由重复注册行为变更（v0.9.0 / v1.0）

v0.9+ 起，重复注册同一 method+path 会输出 `slog.Warn`（之前静默覆盖）。
**行为不变**（新 handler 覆盖旧 handler），但日志会暴露潜在的注册错误。

如果你的代码中有**故意**的覆盖逻辑（测试用 mock），建议改用路由组或
单独的 `App` 实例，避免混淆。

---

### 8. `binding.MaxSliceParams` 默认限制（v0.9.0 / v1.0）

绑定时 slice 字段最多填充 **1000** 个元素（`MaxSliceParams = 1000`）。
超出的元素静默截断。

如果业务确实需要更大的 slice 输入，请改用 JSON body：

```go
// 不推荐：query string 传大数组
GET /api?ids=1&ids=2&...&ids=5000   // 超出 1000 后截断

// 推荐：JSON body
POST /api
{"ids": [1, 2, ..., 5000]}
```

---

## 升级步骤汇总

```bash
# 1. 更新依赖
go get github.com/astra-go/astra@v1.0.0

# 2. 运行迁移检查脚本（见上方）

# 3. 修复编译错误
go build ./...

# 4. 静态分析
staticcheck ./...

# 5. 运行测试
go test ./... -race

# 6. 验证启动
go run ./cmd/server -addr :8080
```

---

## 常见问题

**Q: v0.9 的代码能直接升到 v1.0 吗？**

A: 通常可以，v0.9 → v1.0 基本没有新的破坏性变更（v1.0 是 v0.10 的稳定版）。
主要变更都在 v0.2~v0.9 阶段引入，检查 CHANGELOG 确认你的升级路径覆盖的版本范围。

**Q: 我的项目还在用 `net/http` handler，必须全部改掉吗？**

A: 不需要。使用 `astra.WrapH(h http.Handler)` 包装即可继续使用。
推荐逐步迁移，优先迁移新功能代码。

**Q: 迁移后性能是否有变化？**

A: v1.0 的标准 HTTP 服务器性能与 v0.x 相当。
如果想使用 Reactor 引擎（epoll/kqueue）获得更高并发：
```go
app.RunReactor(":8080")   // 替换 app.Run(":8080")
```
