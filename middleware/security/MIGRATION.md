# middleware/security 重构路线图

> 状态：规划中，暂不执行（破坏性变更，需 major 版本发布）

## 背景

`middleware/security` 当前包含 31 个文件，涵盖多个职责域。拆分有助于：
- 缩短 `go get` 传输体积（按需引入子包）
- 明确边界，降低认知负担
- 独立版本演进

## 目标目录结构

```
middleware/
  security/          ← JWT 核心（middleware.JWT）
  security/auth/     ← JWT 多级缓存、Redis 缓存
  security/ratelimit/ ← 令牌桶 / 滑动窗口（内存 + Redis）
  security/tenant/   ← 多租户隔离（quota、metrics）
  security/network/  ← IP 黑名单、CIDR 过滤、灰度/金丝雀
  security/crypto/   ← HMAC/RSA 签名、API Key、账户锁
  misc/              ← 长轮询、pprof 等杂项
```

**迁移影响预估：**

| 子包 | 文件数 | 主要文件 | import 路径变更 |
|------|--------|----------|----------------|
| `security` | ~8 | jwt.go, jwt_generate.go, options.go | 最小影响（保留入口） |
| `security/auth` | ~4 | jwt_cache.go, jwt_cache_redis.go, jwt_cache_multilevel.go | 新路径 |
| `security/ratelimit` | ~3 | ratelimit.go, ratelimit_redis.go, ratelimit_advanced.go | 新路径 |
| `security/tenant` | ~4 | tenant.go, tenant_quota.go, tenant_quota_json.go, tenant_metrics.go | 新路径 |
| `security/network` | ~4 | ipfilter.go, ipblacklist.go, canary.go | 新路径 |
| `security/crypto` | ~3 | signature.go, apikey.go, accountlock.go | 新路径 |

## 迁移策略

### Phase 1：新增子包（向后兼容）

1. 在各子目录创建 `doc.go`，将原文件内容移入
2. 保留原文件，改为 `//go:build forward` + `//go:build ignore` 代理到子包
3. 标注 `deprecated` 注释，引导用户切换 import 路径
4. 更新 `middleware/security/doc.go` 为迁移指南

**关键：此阶段不改变用户可见 API，仅改变 import 路径。**

### Phase 2：v3.0 移除代理文件

1. 删除所有 `forward` 代理文件
2. `middleware/security` 仅保留 JWT 核心（保留原路径）
3. 发布 v3.0.0，release notes 附完整迁移指引

## 暂不分拆原因

- 破坏性 API 变更，需要 major 版本
- 框架当前处于 v2 阶段，稳定优先
- doc.go 已提供足够透明度（见 `middleware/security/doc.go`）
- 用户量和使用方式未知，拆分前建议收集反馈

## 当前缓解措施

`middleware/security/doc.go` 已提供完整的文件分组说明 + 职责边界说明，满足代码可读性需求。
