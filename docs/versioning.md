# 版本策略

Astra 遵循 [Semantic Versioning 2.0](https://semver.org/spec/v2.0.0.html)。

---

## 版本号规则

```
v MAJOR . MINOR . PATCH
   │        │       └─ 向后兼容的 Bug 修复
   │        └──────── 向后兼容的新功能
   └───────────────── 不兼容的 API 变更
```

| 版本类型 | 触发条件 | 是否需要迁移 |
|----------|----------|-------------|
| `PATCH`  | Bug 修复、文档更新、内部重构 | 否 |
| `MINOR`  | 新功能、弃用标记（deprecated） | 通常不需要 |
| `MAJOR`  | 删除已弃用 API、破坏性接口变更 | 是，参见迁移指南 |

---

## 稳定性等级

每个导出符号（函数、类型、方法、常量）在文档中标注稳定性级别：

| 标签 | 含义 |
|------|------|
| **Stable** | 在当前 major 版本内永不破坏 |
| **Beta** | 可能在 minor 版本中调整，但会提供迁移路径 |
| **Experimental** | 随时可能变更或移除，不建议生产使用 |

---

## 支持周期

| 类型 | 积极维护 | 安全修复 |
|------|----------|----------|
| 当前 major（v1） | 持续 | 持续 |
| 上一 minor（v1.N-1） | 新 minor 发布后 3 个月 | 新 minor 发布后 12 个月 |
| 旧 major（v0） | 已结束 | v1.0 发布后 12 个月 |

> **安全修复优先**：在任何受支持版本中发现的 CVE 将以 `PATCH` 版本形式发布，
> 通知周期不超过 14 天。

---

## 弃用流程

1. 新 `MINOR` 版本中，在文档和 `godoc` 注释中标记 `// Deprecated: 使用 Foo 代替`
2. 弃用标记后至少 **一个 MINOR 版本**（约 3 个月）内保持可用
3. 在下一 `MAJOR` 版本中删除

---

## Go 最低版本

Astra 始终支持最新两个 Go stable 版本（当前 Go 1.25 / 1.24）。
Go 最低版本的提升属于 **MINOR** 变更（有充分提前通知）。

---

## 版本化文档站点

本站使用 [mike](https://github.com/jimporter/mike) 部署多版本文档。
URL 格式为：

```
https://astra-go.github.io/astra/{version}/
```

示例：

```
https://astra-go.github.io/astra/latest/   # 最新稳定版
https://astra-go.github.io/astra/1.0/
https://astra-go.github.io/astra/0.10/
```

### 本地运行所有版本

```bash
pip install mkdocs-material mike

# 构建并部署版本到本地 gh-pages 分支
mike deploy --push 1.0 latest
mike deploy --push 0.10

# 查看
mike serve
# 浏览器访问 http://localhost:8000
```

---

## 发布节奏

| 类型 | 目标周期 |
|------|----------|
| PATCH | 按需（Bug / 安全修复） |
| MINOR | 每季度（约 3 个月） |
| MAJOR | 仅在积累足够多破坏性变更后 |
