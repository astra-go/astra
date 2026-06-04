#!/usr/bin/env bash
#
# release-tags.sh — 批量为 astra 主模块和子模块打 tag 并推送
#
# 用法:
#   ./release-tags.sh <version>           # 升级所有子模块到指定版本
#   ./release-tags.sh <version> --dry-run # 只看会打哪些 tag，不执行
#   ./release-tags.sh <version> --main    # 只打主模块 tag
#   ./release-tags.sh <version> --pkg rule,cache,mq  # 只打指定子模块
#
# 示例:
#   ./release-tags.sh v1.0.3
#   ./release-tags.sh v1.0.3 --dry-run
#   ./release-tags.sh v1.0.3 --pkg rule,cache,mq,orm
#   ./release-tags.sh v1.0.3 --main

# 升级主模块 + 所有子模块到 v1.0.3
# ./scripts/release-tags.sh v1.0.3

# 先预览，不实际执行
# ./scripts/release-tags.sh v1.0.3 --dry-run

# 只打主模块 tag
# ./scripts/release-tags.sh v1.0.3 --main

# 只打指定子模块 tag
# ./scripts/release-tags.sh v1.0.3 --pkg rule,cache,mq

# 组合使用
# ./scripts/release-tags.sh v1.0.3 --pkg rule,cache --dry-run

set -euo pipefail

# ─── 参数校验 ───
if [[ $# -lt 1 ]]; then
  echo "用法: $0 <version> [--dry-run] [--main] [--pkg pkg1,pkg2,...]"
  echo "示例: $0 v1.0.3"
  echo "      $0 v1.0.3 --dry-run"
  echo "      $0 v1.0.3 --main"
  echo "      $0 v1.0.3 --pkg rule,cache,mq"
  exit 1
fi

VERSION="$1"; shift
DRY_RUN=false
MAIN_ONLY=false
PKG_FILTER=""

# 解析选项
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --main)    MAIN_ONLY=true; shift ;;
    --pkg)     PKG_FILTER="$2"; shift 2 ;;
    *) echo "未知选项: $1"; exit 1 ;;
  esac
done

# 校验版本号格式 (vX.Y.Z 或 vX.Y.Z-pre.n)
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
  echo "❌ 版本号格式错误: $VERSION"
  echo "   期望格式: v1.0.3 或 v1.0.3-rc.1"
  exit 1
fi

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo '.')"
cd "$REPO_ROOT"

# 检查工作区是否干净
if ! git diff --quiet HEAD 2>/dev/null; then
  echo "⚠️  工作区有未提交的变更，建议先 commit 或 stash"
  git status --short
  read -rp "继续吗？[y/N] " confirm
  [[ "$confirm" =~ ^[yY]$ ]] || exit 1
fi

# 确保在 main 分支
CURRENT_BRANCH="$(git branch --show-current)"
if [[ "$CURRENT_BRANCH" != "main" ]]; then
  echo "⚠️  当前分支: $CURRENT_BRANCH (不是 main)"
  read -rp "继续在此分支打 tag？[y/N] " confirm
  [[ "$confirm" =~ ^[yY]$ ]] || exit 1
fi

TAGS_TO_CREATE=()

# ─── 主模块 tag ───
if [[ "$MAIN_ONLY" == true || -z "$PKG_FILTER" ]]; then
  if git tag -l "$VERSION" | grep -q .; then
    echo "⚠️  主模块 tag $VERSION 已存在，跳过"
  else
    TAGS_TO_CREATE+=("$VERSION")
  fi
fi

# ─── 子模块 tags ───
if [[ "$MAIN_ONLY" == false ]]; then
  # 获取已有子模块的路径列表
  EXISTING_PKGS=()
  while IFS= read -r existing_tag; do
    pkg="${existing_tag%/*}"
    [[ -z "$pkg" || "$pkg" == "$existing_tag" ]] && continue  # 跳过主模块 tag
    EXISTING_PKGS+=("$pkg")
  done < <(git tag -l '*/v*' | sed 's|/v[0-9].*||' | sort -u)

  # 如果指定了 --pkg，过滤子模块
  if [[ -n "$PKG_FILTER" ]]; then
    IFS=',' read -ra FILTER_ARRAY <<< "$PKG_FILTER"
    FILTERED_PKGS=()
    for pkg in "${EXISTING_PKGS[@]}"; do
      for f in "${FILTER_ARRAY[@]}"; do
        if [[ "$pkg" == "$f" || "$pkg" == *"/$f" ]]; then
          FILTERED_PKGS+=("$pkg")
          break
        fi
      done
    done
    EXISTING_PKGS=("${FILTERED_PKGS[@]}")
  fi

  for pkg in "${EXISTING_PKGS[@]}"; do
    new_tag="${pkg}/${VERSION}"
    if git tag -l "$new_tag" | grep -q .; then
      echo "⚠️  子模块 tag $new_tag 已存在，跳过"
    else
      TAGS_TO_CREATE+=("$new_tag")
    fi
  done
fi

# ─── 汇总 ───
if [[ ${#TAGS_TO_CREATE[@]} -eq 0 ]]; then
  echo "✅ 没有需要创建的 tag"
  exit 0
fi

echo ""
echo "📦 将创建以下 tag:"
printf '  %s\n' "${TAGS_TO_CREATE[@]}"
echo ""

if [[ "$DRY_RUN" == true ]]; then
  echo "🔍 --dry-run 模式，不执行"
  exit 0
fi

# ─── 确认 ───
read -rp "确认打 tag 并推送？[y/N] " confirm
[[ "$confirm" =~ ^[yY]$ ]] || { echo "已取消"; exit 0; }

# ─── 打 tag ───
echo ""
echo "🏷️  创建 tag..."
for tag in "${TAGS_TO_CREATE[@]}"; do
  git tag "$tag"
  echo "  ✅ $tag"
done

# ─── 推送 ───
echo ""
echo "🚀 推送 tag 到 origin..."
for tag in "${TAGS_TO_CREATE[@]}"; do
  git push origin "$tag"
  echo "  ✅ $tag"
done

echo ""
echo "🎉 完成！共推送 ${#TAGS_TO_CREATE[@]} 个 tag"
