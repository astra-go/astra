# 迁移指南总览

本节记录 Astra 各主要版本之间的不兼容变更及其迁移步骤。

## 指南列表

| 升级路径 | 说明 | 影响范围 |
|----------|------|----------|
| [v0.x → v1.0](v0-to-v1.md) | 首个稳定版，含多项 API 整合 | 中等 |
| [v1.x → v2.0](v1-to-v2.md) | 下一主要版本（规划中） | 待定 |

---

## 迁移原则

1. **逐步升级**：跨越多个 major 版本时，请依次升级（0.x → 1.0 → 2.0），
   不要跨版本跳跃。
2. **先读 CHANGELOG**：每次升级前阅读 [CHANGELOG](../changelog.md)，
   确认所有 `### Changed` 和 `### Removed` 条目。
3. **测试覆盖**：升级前确保有足够的单元测试和集成测试，利用
   `go vet ./...` 和 `go test ./...` 快速发现编译期和运行期问题。
4. **弃用警告**：Go 编译器不直接显示弃用警告，但 `gopls` 和
   `staticcheck` 会标注 `// Deprecated:` 注释的符号。
   建议在升级前先运行：
   ```bash
   go install honnef.co/go/tools/cmd/staticcheck@latest
   staticcheck ./...
   ```

---

## 快速版本检查脚本

```bash
#!/usr/bin/env bash
# check-astra-version.sh
set -e

CURRENT=$(go list -m -json github.com/astra-go/astra | jq -r .Version)
LATEST=$(go list -m -versions github.com/astra-go/astra | tr ' ' '\n' | tail -1)

echo "当前版本：$CURRENT"
echo "最新版本：$LATEST"

if [ "$CURRENT" != "$LATEST" ]; then
    echo "请参考迁移指南升级：https://astra-go.github.io/astra/migration/"
fi
```

---

## 获取帮助

- GitHub Issues：[astra-go/astra/issues](https://github.com/astra-go/astra/issues)
- 标签 `migration` 过滤迁移相关问题
