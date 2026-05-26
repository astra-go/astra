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
