# Astra 项目架构审查报告

> 审查日期：2026-07-15  
> 版本范围：v1.0.5 → v1.0.6  
> 审查范围：全项目（50+ 模块）

---

## 🐛 Bug

### 1. ✅ `middleware/security/doc.go` 文档过时 — **已修复** (commit `d58d2d8`)

引用了已删除的 `auth/` 子包。`doc.go` 中写道：

> "The auth/ subpackage is a standalone authentication system... Use auth for login/logout flows; use middleware/security/jwt.go to protect routes."

`middleware/security/auth/` 子目录已在 v1.0.6 清理中删除，但文档未更新。

**文件：** `middleware/security/doc.go`  
**修复：** 删除对 `auth/` 子包的引用，或更新为指向根目录的 `auth/` 独立模块。

---

### 2. ✅ `ErrorHandler` 签名不统一 — **已修复** (`security/options.go` 改为 `middleware.ErrorHandler` 类型别名, types.go 保留+注释)

| 定义位置 | 签名 | 外部引用 | 状态 |
|----------|------|----------|------|
| `types.go:23` | `func(*Ctx, error)` | 0 | **死代码** |
| `middleware/options.go:33` | `astra.HandlerFunc` = `func(*Ctx) error` | cors/timeout/errorhandler | 活跃 |
| `middleware/security/options.go:18` | `astra.HandlerFunc` = `func(*Ctx) error` | apikey/ratelimit/ipblacklist/tenant | 活跃 |
| `middleware/security/jwt.go:221` | `func(*Ctx, error) error` | JWT 专用 | 活跃（例外） |

`types.go` 的 `ErrorHandler` **完全未被任何代码引用**，可以安全删除。

**文件：** `types.go:22-23`  
**修复：** 删除死代码（仅需确保无外部依赖引用）。后续可考虑在 `middleware/` 下统一定义一个 `ErrorHandler`，让 `security/` 通过类型别名引用。

---

### 3. ✅ `Skipper` 类型重复定义 — **已修复** (`security/options.go` 改为 `= middleware.Skipper`, commit `d58d2d8`)

两个包各自定义了完全相同的类型：

```go
// middleware/options.go:9
type Skipper func(c *astra.Ctx) bool

// middleware/security/options.go:9
type Skipper func(c *astra.Ctx) bool
```

`shouldSkip` 函数也重复了两份：

```go
// middleware/options.go:23
func shouldSkip(skipper Skipper, c *astra.Ctx) bool { ... }

// middleware/security/options.go:12
func shouldSkip(skipper Skipper, c *astra.Ctx) bool { ... }
```

`middleware/security/` 不 import `middleware/` 包，所以不存在类型别名复用问题——但将来如果签名变化，两处都需要修改。

**文件：** `middleware/options.go:9-12`、`middleware/security/options.go:9-15`  
**修复：** 让 `middleware/security/` import `middleware/` 并统一使用 `middleware.Skipper`，或至少保留清晰的注释说明双向同步要求。

---

### 4. ✅ `middleware/ratelimit_redis_test.go` 位置错误 — **已修复** (移至 `middleware/security/`, commit `d58d2d8`)

该文件在 `middleware/` 包下（`package middleware_test`），但实际测试的是 `middleware/security` 包的 ratelimit 功能：

```go
import (
    sec "github.com/astra-go/astra/middleware/security"
    ...
)
```

**文件：** `middleware/ratelimit_redis_test.go`  
**修复：** 移至 `middleware/security/ratelimit_redis_test.go`。

---

## 🗑 冗余与清理

### 5. ✅ `types.go` 的 `ErrorHandler` — **已处理** (分析后非死代码，框架内部多处引用，已添加注释说明与 middleware 级的区别)

`astra.ErrorHandler`（`func(*Ctx, error)`）在 50+ 个模块中没有引用。`middleware.ErrorHandler` 和 `security.ErrorHandler` 都是 `astra.HandlerFunc` 别名。

**文件：** `types.go:22-23`  
**修复：** 删除。如果未来有框架级 ErrorHandler 需求，可定义为 `astra.HandlerFunc` 别名而不是新签名。

---

### 6. ✅ `tools/modproxy/cache/` — **已清理** (目录已删除，已在 `.gitignore`)

该目录通过 `.gitignore`（第 8-9 行）排除在 git 追踪之外，但磁盘上仍保留 178MB 的 Go module proxy 缓存。

**文件：** `tools/modproxy/cache/`  
**修复：** 添加 `make clean` 或 `go work sync` 钩子清理缓存，或在 README 中说明如何手动清理。

---

### 7. ✅ `.astractl/deps-cache.json` 未 gitignore — **已修复** (添加 `.astractl/` 到 `.gitignore`, commit `d58d2d8`)

该文件 2.4MB，不在 `.gitignore` 中。目前不在 git 追踪中，但随时可能被误 `git add`。

**文件：** `.astractl/deps-cache.json`  
**修复：** 添加 `.astractl/` 到 `.gitignore`。

---

### 8. ✅ `examples/` 断裂模块 — **已修复** (hello/mq 补 go.mod, wasm 修正, http3/orm-readwrite 空目录删除, 全量 build/vet 通过)

| 模块 | go.mod | go.work | 状态 |
|------|--------|---------|------|
| `examples/wasm` | ✅ (依赖 astra v1.0.5 旧版本) | ❌ | 版本不对齐、无法构建 |
| `examples/hello` | ❌ | ❌ | 缺少 go.mod |
| `examples/mq` | ❌ | ❌ | 缺少 go.mod |
| `examples/http3` | ❌ | ❌ | 空目录 |
| `examples/orm-readwrite` | ❌ | ❌ | 缺少 go.mod |

**修复：** 补全 go.mod、更新版本引用，并加入 go.work，或删除不再维护的示例。

---

### 9. ✅ `tools/modproxy/private/` — **已清除** (目录已不存在)

`private/github.com/` 三层目录结构存在但无文件。

**修复：** 删除或添加 `.gitkeep` 并添加 README 说明用途。

---

## ⚠️ 设计问题

### 10. ⏸️ `auth/` 包定位模糊 — **暂缓** (独立 Go 模块, auth/rbac + auth/oauth2, 与 middleware/security 零代码交互, 等待产品决策)

`auth/` 是根级独立 Go 模块（含 `rbac/` 和 `oauth2/` 子包），在 go.work 中声明为 `./auth`。实际情况：
- `rbac/` — 仅 1 个 `.go` 文件
- `oauth2/` — 仅 2 个 `.go` 文件
- 与 `middleware/security` **零代码级交互**（security 包自己管理 JWT 相关代码）
- `middleware/security/doc.go` 把 `auth/` 描述为"独立的认证系统"

**建议：** 明确 `auth/` 的定位——是公共认证工具库还是服务端 SDK？如果代码量小且无活跃使用者，考虑合并到 `middleware/security` 或删除。

---

### 11. ✅ `provider/` → `di/` 迁移残留 — **已处理** (`registry.go` 降级为 deprecated alias, CHANGELOG 更新, 计划 v2 移除, commit `bb431ad`)

`provider/registry.go` 已降级为 deprecated type alias（指向 `di.ProviderRegistry`）：

```go
// Deprecated: Package provider is superseded by di.ProviderRegistry...
```

**问题：** `provider/` 没有自己的 `go.mod`，随主模块 `github.com/astra-go/astra` 一起发布。未来删除时会 break 所有 `import "...astra/provider"` 的外部使用者。

**建议：** 在下一个主版本（v2）中移除前，先在 CHANGELOG 中明确弃用时间表。

---

### 12. ⚠️ `middleware/cors.go` panic 过多 — **已修改未提交** (新增 `validateOrigins` 合并校验, panic 8→5, 3处 validateOrigins + 1处 env + 1处 creds+wildcard)

CORS 配置错误全部使用 `panic` 表达。虽然是 "fail fast" 模式，但在以下场景可能危险：

```go
// CORSProduction 从环境变量读取配置
func CORSProduction() astra.HandlerFunc {
    if origins == "" {
        panic("middleware.CORSProduction: CORS_ALLOWED_ORIGINS environment variable is required")
    }
```

如果 `CORS_ALLOWED_ORIGINS` 配置错误，整个进程会崩溃。

**建议：** `CORSProduction` 等从外部读取配置的函数返回 `(astra.HandlerFunc, error)` 而不是 panic。

---

### 13. ⚠️ `taskqueue/task.go` 的 `must()` — **已修改未提交** (删除未使用的 `must()` 死代码)

```go
func must(v []byte, err error) []byte {
    if err != nil { panic(err) }
    return v
}
```

在序列化失败时（如 JSON/MsgPack encode 返回 error）会导致进程崩溃。序列化通常是确定性操作，但极端情况（OOM、类型错误）仍可能触发。

**建议：** 将 error 向上传递而不是 panic，或仅在 `init()` 阶段使用 `must`。

---

### 14. ⚠️ `context_response.go` CL 缓存 — **已修改未提交** (添加注释说明 1024/52KB/降级, `clCacheSize` 更正为 1024)

```go
const clCacheSize = 2048

func init() {
    for i := range clStrings {
        clStrings[i] = strconv.Itoa(i) // 预缓存 "0".."2047"
    }
}
```

`clCacheSize` 硬编码了 2048，超出上限的 Content-Length 会静默降级为分配新字符串（0 allocs → 1 alloc），功能上无 bug，但缺少文档说明。

**建议：** 添加注释说明为什么选择 2048（覆盖大多数响应的 Content-Length 范围），以及超出上限时的行为。

---

## 总结

| 严重度 | 数量 | 条目 |
|--------|------|------|
| 🐛 Bug | 4 (全部已修复) | doc.go 过时、ErrorHandler 签名混乱、Skipper 重复、测试文件错位 |
| 🗑 冗余 | 5 (全部已处理) | 死代码 ErrorHandler、178MB 缓存、deps-cache 未 gitignore、断裂 examples、残留目录 |
| ⚠️ 设计 | 5 (1暂缓, 3修改未提交, 1已处理) | auth 定位模糊、provider 迁移残留、CORS panic、must() panic、缓存文档不足 |

### 验证通过项

- ✅ 全项目 build 通过（50+ 模块）
- ✅ `go vet` 零警告
- ✅ Saga 补偿逻辑完整（dtx/saga.go）
- ✅ `Ctx.reset()` pool 安全（100% 重置敏感字段）
- ✅ 优雅关闭信号处理完整（SIGTERM/SIGINT）
- ✅ RocketMQ 事务集成测试通过
- ✅ gRPC-Web 集成完成（24 PASS）

---

## 快速修复建议

所有 14 项已处理:
- 第 1-9, 11 项: ✅ 已修复/已处理
- 第 10 项: ⏸️ 暂缓 (等待产品决策)
- 第 12-14 项: ⚠️ 代码已修改，等待提交

---