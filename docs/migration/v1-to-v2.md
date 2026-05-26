# 迁移指南：v1.x → v2.0

!!! info "状态：规划中"
    v2.0 尚未发布。本文档描述**计划中的**破坏性变更，帮助你提前评估迁移成本。
    最终版本可能与此文档有所差异，请以正式发布的 CHANGELOG 为准。

---

## 计划变更概览

| 变更 | 影响 | 提前弃用版本 |
|------|------|-------------|
| `Context` 改为接口类型 | 中等 | v1.2 |
| 移除 `astra.H` 类型别名 | 低 | v1.1 |
| `MiddlewareFunc` 参数类型收窄 | 低 | v1.3 |
| 错误处理器签名统一 | 低 | v1.2 |
| 配置选项移至 `astra.Options` 结构体 | 低 | v1.1 |
| `Group` 返回类型变更 | 低 | v1.3 |

---

## 详细变更

### 1. `Context` 改为接口类型

v2.0 将 `*astra.Ctx`（当前具体类型）隐藏为私有实现，
公共 API 统一使用 `astra.Context`（接口）。

**v1.x（当前）**：

```go
func handler(c *astra.Context) error { ... }   // *Ctx 具体类型
```

**v2.0（计划）**：

```go
func handler(c astra.Context) error { ... }   // 接口类型
```

**影响**：函数签名需全局替换 `*astra.Context` → `astra.Context`。
`contract.Context` 接口已经存在，v2.0 将合并两者。

**提前适配（v1.x 下）**：使用 `contract.Context` 代替 `*astra.Context`：

```go
// 现在就可以这样写，v2.0 下无需改动
func handler(c contract.Context) error { ... }
```

---

### 2. 移除 `astra.H` 类型别名

`astra.H` 是 `map[string]any` 的类型别名，v2.0 移除此别名以减少重导出。

**v1.x**：

```go
c.JSON(200, astra.H{"key": "value"})
```

**v2.0**：

```go
c.JSON(200, map[string]any{"key": "value"})
// 或定义自己的 type H = map[string]any
```

**批量迁移**（v1.x → v2.0）：

```bash
sed -i 's/astra\.H{/map[string]any{/g' **/*.go
```

---

### 3. 配置选项结构化

v2.0 将零散的 `With*` option 函数替换为直接传入 `astra.Options` 结构体，
减少导入路径依赖、提升 IDE 自动补全体验。

**v1.x**：

```go
app := astra.New(
    astra.WithLogger(l),
    astra.WithTrustedProxies("10.0.0.0/8"),
    astra.WithMaxMultipartMemory(32 << 20),
)
```

**v2.0（计划）**：

```go
app := astra.New(astra.Options{
    Logger:              l,
    TrustedProxies:      []string{"10.0.0.0/8"},
    MaxMultipartMemory:  32 << 20,
})
```

`With*` 函数将在 v1.1 标记为 Deprecated，在 v2.0 移除。

---

### 4. 错误处理器签名统一

v2.0 的全局错误处理器接收 `astra.Context`（接口），与第 1 条配套。

**v1.x**：

```go
astra.New(astra.WithErrorHandler(func(c *astra.Context, err error) {
    c.JSON(500, astra.H{"error": err.Error()})
}))
```

**v2.0**：

```go
astra.New(astra.Options{
    ErrorHandler: func(c astra.Context, err error) {
        c.JSON(500, map[string]any{"error": err.Error()})
    },
})
```

---

## 提前适配建议

即使还未升级 v2.0，以下做法可以减少未来的迁移工作量：

1. **用 `contract.Context` 代替 `*astra.Context`** 作为内部 handler 参数类型
2. **避免使用 `astra.H`**，直接用 `map[string]any`
3. **用 `middleware.NewRateLimiter`** 代替 `middleware.RateLimit`（已在 v1.0 推荐）
4. **保持 handler 函数无状态**，依赖通过 `c.Get(key)` 注入，而非闭包捕获

---

## 反馈

v2.0 设计仍在讨论中，欢迎参与：

- [GitHub Discussion: v2.0 API Design](https://github.com/astra-go/astra/discussions)
- [RFC: Context as Interface](https://github.com/astra-go/astra/issues)
