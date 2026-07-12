# Git 提交邮箱修改指南

> 将所有提交历史中的 `<xiaolin@boomgames.top>` 替换为指定目标邮箱。
> **仅修改作者身份信息，不改动提交时间、内容和时间戳。**

---

## 第一步：安装 git-filter-repo

```bash
# macOS — 使用 pip
pip3 install git-filter-repo

# 或使用 Homebrew
brew install git-filter-repo
```

---

## 第二步：备份仓库（强烈建议）

```bash
# 在 astra 同级目录下创建备份副本
cp -r /Users/huangxiaolin/data/project/gotest/astra /Users/huangxiaolin/data/project/gotest/astra-backup
```

> 保留备份，直到确认新历史正确无误。

---

## 第三步：确认目标邮箱

在执行重写之前，**替换 `<目标邮箱>` 为你的真实目标邮箱**，例如：

```
your-github@email.com
```

---

## 第四步：创建邮件映射文件

```bash
cd /Users/huangxiaolin/data/project/gotest/astra

# 格式：旧身份 <旧邮箱> <新邮箱>
# 将 <目标邮箱> 替换为真实邮箱，例如 your@email.com
echo "xiaolin@boomgames.top <目标邮箱>" > .mailmap
```

---

## 第五步：重写历史

```bash
git filter-repo --mailmap .mailmap --force
```

**执行效果：**
- 将所有 `xiaolin@boomgames.top` 的作者邮箱改为 `<目标邮箱>`
- 提交时间（Author Date / Committer Date）保持不变
- 提交内容（diff）完全不变
- 时间顺序不变

---

## 第六步：验证结果

```bash
# 查看所有不重复的作者邮箱
git log --format="%ae | %an" | sort -u

# 查看所有不重复的提交者邮箱
git log --format="%ce | %cn" | sort -u
```

确认输出中**不再包含** `xiaolin@boomgames.top`。

---

## 第七步：强制推送

```bash
# 推送所有分支（历史已改变，需要 --force）
git push --force --all

# 推送所有标签
git push --force --tags
```

---

## 注意事项

| 项目 | 说明 |
|------|------|
| 仓库权限 | 确保你对仓库有写权限并可以 force push |
| 协作者影响 | 如果其他人也在协作此仓库，他们需要重新 clone 或 rebase |
| 备份保留 | 保留 `astra-backup` 直到确认新历史正确 |
| 不可逆操作 | 重写历史后无法恢复，备份是唯一保障 |

---

## 如需回滚

```bash
# 删除修改后的仓库
rm -rf /Users/huangxiaolin/data/project/gotest/astra

# 恢复备份
cp -r /Users/huangxiaolin/data/project/gotest/astra-backup /Users/huangxiaolin/data/project/gotest/astra
```
