# Astra 代码安全与完整性审计报告

**项目**: github.com/astra-go/astra  
**审计时间**: 2026-07-11  
**Go 版本**: 1.25.11  
**审计范围**: 575 个 .go 文件

---

## 编译检查

- ✅ `go build ./...` — 无错误
- ✅ `go vet ./...` — 无警告

---

> **⚠️ 修订说明（2026-07-12）**：H-1、H-2、M-1c、M-2、M-3 已于本版本修复并提交（commit `4d3973f`）。
> M-1a、M-1b 暂缓（需等生命周期管理方案统一）。

---

## 高危（必须修复）

### 🔴 H-1: ~~潜在 Panic~~ ✅ 已修复 — N+1 检测器中的不安全类型断言

**文件**: `orm/n_plus_one.go`  
**状态**: ✅ 已修复（commit `4d3973f`）
**修复方式**: `getRequestID` 改用两值形式 `if id, ok := ctx.Value("request-id").(string); ok { return id }`

```go
func (d *NPlusOneDetector) getRequestID(ctx context.Context) string {
    if id := ctx.Value("request-id"); id != nil {
        return id.(string)  // ← 无 ok-check，可能 panic
    }
    if id := ctx.Value("x-request-id"); id != nil {
        return id.(string)  // ← 无 ok-check，可能 panic
    }
    return fmt.Sprintf("%p", ctx)
}
```

**问题**: `ctx.Value()` 返回 `any`，直接做 `id.(string)` 类型断言而未检查 `ok` 值。如果上游将非 string 类型（如 `int`）存入 context，会触发 panic，导致服务崩溃。

---

### 🔴 H-2: ~~账户锁定失败被静默忽略~~ ✅ 已修复 — 安全绕过风险

**文件**: `middleware/security/accountlock.go`  
**状态**: ✅ 已修复（commit `4d3973f`）
**修复方式**: `IncrLoginFail` 中 `Expire`/`LockAccount` 改 fail-closed，错误时返回 error 而非静默忽略。

```go
func (r *RedisAccountLocker) IncrLoginFail(...) (int64, error) {
    ...
    if count == 1 {
        _ = r.client.Expire(ctx, fk, r.window)  // ← 忽略错误
    }
    if count >= int64(r.maxFails) {
        _ = r.LockAccount(ctx, key)  // ← 忽略错误
    }
    return count, nil
}
```

## 中危（建议修复）

### 🟡 M-1: Goroutine 泄漏 — 多处后台清理 goroutine 无法停止

#### M-1a: ⚠️ 暂未修复 — IPFilter 后台刷新 goroutine 无法停止

**文件**: `middleware/security/ipfilter.go`  
**原因**: `reloadCancel` 存入 cfg 但未返回，用户无法主动停止。需等生命周期管理方案统一后再处理。

#### M-1b: ⚠️ 暂未修复 — InMemoryNonceStore 的 reap goroutine 无法停止

**文件**: `middleware/security/signature.go`  
**原因**: 内部启动 goroutine 但未提供停止通路。需等生命周期管理方案统一后再处理。

#### M-1c: ✅ 已修复 — `SlidingWindow` 函数丢弃停止函数

**文件**: `middleware/security/ratelimit_advanced.go`  
**状态**: ✅ 已修复（commit `4d3973f`）
**修复方式**: `SlidingWindow()` 便利函数不再启动 goroutine（lazy eviction 替代）；`SlidingWindowWithConfig` 仅在 `cfg.Context != nil` 时启动后台清理。

---

### 🟡 M-2: ~~OAuth2 StateKey 随机生成~~ ✅ 已修复 — 多实例部署 cookie 失效

**文件**: `auth/oauth2/oauth2.go`  
**状态**: ✅ 已修复（commit `4d3973f`）
**修复方式**: 支持 `ASTRA_OAUTH_STATE_KEY` 环境变量注入固定 32 字节密钥（base64.RawURL 或明文），无配置时警告并 fallback 随机。

---

### 🟡 M-3: ~~N+1 检测器重复编译正则表达式~~ ✅ 已修复 — extractTable 性能优化

**文件**: `orm/n_plus_one.go`  
**状态**: ✅ 已修复（commit `4d3973f`）
**修复方式**: 5 个正则移至 `package init()` 预编译 `tablePatterns []*regexp.Regexp`，消除每次 SQL 查询的 MustCompile 开销。

---

## 低危（可选优化）

### 🟢 L-1: Panic 作为配置验证手段

**文件**: `middleware/security/jwt.go` 436, 456, 459; `signature.go` 105; `cors.go` 多处

启动时 `panic` 会导致整个服务无法启动，如果配置在运行时动态加载会导致服务崩溃。

**建议**: 对配置验证 panic 影响有限（启动失败比错误运行好），但对于可选组件可改为返回错误。

---

### 🟢 L-2: DefaultTenantValidator 仅限 ASCII 范围

**文件**: `middleware/security/tenant.go` 185–193

租户 ID 验证只允许 `[a-zA-Z0-9_-]`，不支持 Unicode租户 ID。如果业务方使用非 ASCII 租户 ID 会无法使用。

**建议**: 如业务需要 Unicode 租户 ID，调整正则：`[\p{L}\p{N}_-]`

---

### 🟢 L-3: Recover 中的 AlertFunc goroutine 无外部取消机制

**文件**: `middleware/recovery.go` 行 68

AlertFunc goroutine 使用 10s timeout，但如果 AlertFunc 内部调用阻塞的外部服务（超过 10s），goroutine 会在 timeout 后被强制取消但 AlertFunc 可能仍在运行。影响极小。

---

## 安全设计亮点（值得肯定）

| 方面 | 评价 |
|------|------|
| JWT 算法白名单 + alg:none 拒绝 | ✅ 完整 |
| JWT HMAC 最小密钥长度检查 | ✅ 32 字节 |
| RSA/EC 最小密钥强度检查 | ✅ 2048-bit RSA, P-256 |
| CORS 默认拒绝所有来源 | ✅ 从 v4.1 改为默认空（拒绝） |
| CORS 凭证+通配符冲突检测 | ✅ 启动时 panic |
| Storage 路径遍历防护 | ✅ `absPath()` 完整实现 |
| SQL Schema 名称白名单验证 | ✅ `isValidSchemaName` + 正则 |
| ORM DeleteWhere SQL 注入防护 | ✅ 拒绝 Raw SQL String |
| Redis Lua 脚本原子操作 | ✅ 所有 broker 操作使用 Lua |
| 分布式锁 CAS 删除 | ✅ token 比对后才 DEL |
| CSRF 恒定时间比较 | ✅ `hmac.Equal` |
| SecretString 防日志泄露 | ✅ MarshalJSON/Stringer 均返回 `[REDACTED]` |

---

## 当前状态汇总（2026-07-12）

| ID | 问题 | 状态 | 原因 |
|----|------|------|------|
| H-1 | getRequestID 类型断言 panic | ✅ 已修复 | commit `4d3973f` |
| H-2 | 账户锁定 fail-closed | ✅ 已修复 | commit `4d3973f` |
| M-1a | IPFilter goroutine 泄漏 | ⚠️ 暂缓 | 需生命周期管理方案 |
| M-1b | InMemoryNonceStore goroutine 泄漏 | ⚠️ 暂缓 | 需生命周期管理方案 |
| M-1c | SlidingWindow goroutine 泄漏 | ✅ 已修复 | commit `4d3973f` |
| M-2 | OAuth2 StateKey 随机 | ✅ 已修复 | commit `4d3973f` |
| M-3 | extractTable 正则重复编译 | ✅ 已修复 | commit `4d3973f` |
| L-1 | Panic 作为配置验证 | ⏸️ 暂不处理 | 启动失败好于错误运行 |
| L-2 | DefaultTenantValidator 仅 ASCII | ⏸️ 暂不处理 | 业务暂无 Unicode 租户 ID |
| L-3 | AlertFunc goroutine 无取消机制 | ⏸️ 暂不处理 | 影响极小 |

**已修复**: 5 项（H-1、H-2、M-1c、M-2、M-3）
**暂缓**: 5 项（M-1a、M-1b 低危；L-1/2/3 极低风险）
**总提交**: commit `4d3973f`（5 个文件）
