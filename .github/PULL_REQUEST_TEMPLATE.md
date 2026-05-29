## 变更类型
<!-- 勾选对应的 Conventional Commits 前缀 -->
- [ ] `feat` — 新功能
- [ ] `fix` — Bug 修复
- [ ] `perf` — 性能优化
- [ ] `refactor` — 重构（无行为变更）
- [ ] `docs` — 文档
- [ ] `test` — 测试
- [ ] `chore` — 构建 / CI / 依赖

## 变更描述
<!-- 一句话说明做了什么，以及为什么 -->

## 破坏性变更
- [ ] 无破坏性变更
- [ ] 有破坏性变更（请在下方说明受影响的 API 和迁移路径）

<!-- 如有破坏性变更，在此描述 -->

## 测试
- [ ] 新增 / 更新了单元测试
- [ ] 通过 `go test -race ./...`
- [ ] 覆盖率未下降（Codecov 报告确认）
- [ ] 不涉及可测试的逻辑变更（仅文档 / 注释 / 格式）

## 安全影响
<!-- 变更涉及 middleware/security/、auth/、jwt/、binding/ 等安全相关模块时必填 -->
- [ ] 不涉及安全相关模块
- [ ] 涉及安全模块，已评估（说明：）

## Checklist
- [ ] 提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范
- [ ] 更新了 `CHANGELOG.md` 的 `[Unreleased]` 区块
- [ ] 如有新的公开 API，已更新对应文档
- [ ] 如涉及子模块依赖变更，已运行 `go mod tidy` 并提交 `go.mod` / `go.sum`
