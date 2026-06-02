# 📦 Astra 发布流程

## 🎯 核心原则

| 状态 | go.mod 状态 | 用途 | 谁需要 |
|------|-------------|------|--------|
| **开发态** | 有 `replace => ../` | 本地实时测试 | 开发者 |
| **发布态** | 无 `replace` | 外部用户 `go get` | 全世界 |

---

## 🔧 日常开发（保持 `replace`）

```bash
# 1. 确保处于开发态（有 replace）
bash scripts/sync-intra-replaces.sh

# 2. 正常开发
# - 修改任何子模块代码
# - 本地测试（go test ./...）
# - replace 确保使用本地代码，无需发布

# 3. 提交代码（保留 replace）
git add -A
git commit -m "feat: add new feature"
git push origin xiaolin
```

---

## 🚀 发布流程（7步）

### 第1步：切换到 `main` 分支
```bash
cd ~/data/project/gotest/astra
git checkout main
git pull origin main
```

### 第2步：合并开发分支
```bash
git merge xiaolin
```

### 第3步：清理 `replace` 指令
```bash
# 删除所有 go.mod 中的 replace 行
bash scripts/drop-intra-replaces.sh

# 验证：确保完全清理
grep -r "replace" --include="go.mod" . && echo "❌ 仍有 replace" || echo "✅ 已清理"
```

### 第4步：提交干净状态
```bash
git add -A
git commit --no-verify -m "chore: clean go.mod for v1.0.0 release"
```

### 第5步：执行发布
```bash
# AUTO_CONFIRM=1 跳过所有交互式确认
AUTO_CONFIRM=1 VERSION=v1.0.0 make release
```

**预期输出**：
```
── 前置检查 ──
⚠  current branch is "main", not main  (如果不在 main)
Continue? [y/N] y (auto-confirmed)

── 计算 tag 列表 ──
── 将创建 34 个 tag ──
  • v1.0.0
  • alert/v1.0.0
  ...

确认创建以上 34 个 tag？ [y/N] y (auto-confirmed)

── 创建 tag ──
  ✓ created: v1.0.0
  ✓ created: alert/v1.0.0
  ...
```

### 第6步：推送 tag 到远程
```bash
# 推送所有新创建的 tag
git push origin $(git tag -l '*v1.0.0' | tr '\n' ' ')

# 验证：检查远程 tag
git ls-remote --tags origin | grep "v1.0.0"
```

### 第7步：恢复本地开发态
```bash
# 恢复 replace 指令
bash scripts/sync-intra-replaces.sh

# 提交恢复后的状态
git add -A
git commit --no-verify -m "chore: restore replace after v1.0.0 release"

# 推送到远程
git push origin main
```

### 第8步：同步到开发分支
```bash
git checkout xiaolin
git rebase main
git push origin xiaolin --force-with-lease
```

---

## ✅ 发布验证

### 1. 检查本地 tag
```bash
git tag -l "*v1.0.0"
```

### 2. 检查远程 tag
```bash
git ls-remote --tags origin | grep "v1.0.0"
```

### 3. 测试外部引用
```bash
# 创建临时目录
mkdir -p /tmp/astra-test && cd /tmp/astra-test

# 初始化测试模块
go mod init astra-test

# 设置代理（国内用 goproxy.cn）
go env -w GOPROXY=https://goproxy.cn,direct

# 拉取 astra
go get github.com/astra-go/astra@v1.0.0

# 验证成功
grep "astra" go.mod
```

**预期输出**：
```
go: downloading github.com/astra-go/astra v1.0.0
go: added github.com/astra-go/astra v1.0.0
```

---

## 🔄 完整示例（发布 `v1.0.1`）

```bash
cd ~/data/project/gotest/astra

# 1. 切换到 main
git checkout main && git pull origin main

# 2. 合并开发分支
git merge xiaolin

# 3. 清理 replace
bash scripts/drop-intra-replaces.sh
grep -r "replace" --include="go.mod" . && echo "❌ 仍有 replace" || echo "✅ 已清理"

# 4. 提交
git add -A && git commit --no-verify -m "chore: clean go.mod for v1.0.1 release"

# 5. 发布
AUTO_CONFIRM=1 VERSION=v1.0.1 make release

# 6. 推送 tag
git push origin $(git tag -l '*v1.0.1' | tr '\n' ' ')

# 7. 恢复 replace
bash scripts/sync-intra-replaces.sh
git add -A && git commit --no-verify -m "chore: restore replace after v1.0.1 release"
git push origin main

# 8. 同步开发分支
git checkout xiaolin && git rebase main && git push origin xiaolin --force-with-lease
```

---

## 🛠️ 故障排查

### 问题1：`mage release` 报错 "intra-workspace replace found"
**原因**：`go.mod` 仍有 `replace` 指令  
**解决**：
```bash
# 强制删除所有 replace 行
find . -name "go.mod" -not -path "./.git/*" -exec sed -i '' '/^[[:space:]]*replace/d' {} +
```

### 问题2：`go mod tidy` 报错 "unknown revision"
**原因**：依赖的版本号不存在（伪版本）  
**解决**：
```bash
# 修复版本号（假设引导版本是 v0.1.0）
bash scripts/fix-require-versions.sh v0.1.0
```

### 问题3：推送 tag 失败 "already exists"
**原因**：远程已有同名 tag  
**解决**：
```bash
# 删除本地 tag
git tag -d v1.0.0 alert/v1.0.0 ...

# 删除远程 tag
git push origin :refs/tags/v1.0.0 :refs/tags/alert/v1.0.0 ...

# 重新创建并推送
git tag v1.0.0 && git push origin v1.0.0
```

---

## 📝 脚本说明

| 脚本 | 用途 | 使用场景 |
|------|------|----------|
| `scripts/sync-intra-replaces.sh` | 添加 `replace` 指令 | 开发前/发布后 |
| `scripts/drop-intra-replaces.sh` | 删除 `replace` 指令 | 发布前 |
| `scripts/fix-require-versions.sh` | 修复伪版本号 | 引导发布时 |

---

## 🎯 快速参考

### 开发循环
```
开发代码 → 测试（replace 生效） → 提交 → 推送
```

### 发布循环
```
合并代码 → 删除 replace → 发布 → 推送 tag → 恢复 replace
```

---

## ✅ 检查清单

发布前确认：
- [ ] 所有代码已合并到 `main`
- [ ] 所有测试通过（`go test ./...`）
- [ ] `go.mod` 无 `replace` 指令
- [ ] `VERSION` 环境变量已设置（如 `v1.0.0`）

发布后确认：
- [ ] 所有 tag 已推送到远程
- [ ] 外部项目能 `go get github.com/astra-go/astra@v1.0.0`
- [ ] `replace` 指令已恢复（本地开发态）
- [ ] 开发分支已同步（`xiaolin` rebased on `main`）

---

**最后更新**：2026-06-02
