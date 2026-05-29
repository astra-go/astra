# 贡献指南

感谢你考虑为 Astra 做贡献！

## 开发环境

```bash
git clone https://github.com/astra-go/astra.git
cd astra
go mod download

# 运行所有测试
go test ./... -race

# 运行 vet + staticcheck
go vet ./...
staticcheck ./...
```

## 提交规范

提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
feat(middleware): add CSRF double-submit cookie support
fix(netengine): drain addCh on shutdown to avoid conn leak
docs(migration): add v0-to-v1 migration guide
test(alert): add For duration delay test
```

| 前缀 | 用途 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档变更 |
| `test` | 测试相关 |
| `refactor` | 重构（无行为变更） |
| `perf` | 性能优化 |
| `chore` | 构建/CI 相关 |

## Pull Request 流程

1. Fork 仓库，创建特性分支 `feat/your-feature`
2. 编写代码 + 测试（覆盖率不低于现有水平）
3. 确认 `go test ./... -race` 通过
4. 更新 `CHANGELOG.md` 的 `[Unreleased]` 区块
5. 提交 PR，填写模板描述变更原因

## 文档更新

```bash
pip install mkdocs-material

# 本地预览
mkdocs serve

# 构建静态站点
mkdocs build
```

## 代码审查规范

### Reviewer 职责

- 在 PR 提交后 **2 个工作日内** 完成首轮审查
- 审查重点：正确性、安全性、与现有架构的一致性
- 涉及 `middleware/security/`、`auth/`、`jwt/`、`binding/` 的变更，需安全负责人（`@astra-go/security`）额外审查

### 审查 Checklist

**功能正确性**
- [ ] 逻辑是否符合需求，边界条件是否处理
- [ ] 错误路径是否正确返回，错误信息是否合适

**安全性**
- [ ] 输入校验是否完整（特别是 binding 层）
- [ ] 是否存在 SQL 注入、XSS、SSRF 等风险
- [ ] 权限控制是否正确，数据隔离是否到位
- [ ] 敏感数据（密钥、token）是否通过 `SecretString` 保护

**代码质量**
- [ ] 命名是否清晰，无需注释即可理解
- [ ] 是否引入了不必要的抽象或依赖
- [ ] 是否与现有代码风格一致

**测试**
- [ ] 新增逻辑是否有对应测试
- [ ] 测试是否覆盖了正常路径和异常路径
- [ ] Codecov 报告中 patch 覆盖率是否 ≥ 60%

**破坏性变更**
- [ ] 公开 API 是否有变更，是否更新了 `.api/next.txt`
- [ ] CHANGELOG.md 是否已更新

### 合并条件

PR 满足以下全部条件才可合并：

1. CI 所有 job 通过（lint、test、api-compat、security、tidy、replaces）
2. 至少 1 名 Reviewer 批准（安全模块需安全负责人批准）
3. Codecov patch 覆盖率 ≥ 60%
4. 无未解决的 Review 评论

### 本地验证命令

```bash
# 格式检查
gofmt -l ./...

# 静态分析（仅新增代码）
make lint

# 全量静态分析
make lint-all

# 安全漏洞扫描
make vuln

# 测试（含 race detector）
go test -race ./...

# API 兼容性检查
bash scripts/apicheck.sh --check
```
