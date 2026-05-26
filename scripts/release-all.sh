#!/usr/bin/env bash
# release-all.sh — Lockstep 发版：为所有 workspace 模块一次性打同一个 vX.Y.Z tag。
#
# 设计原则：所有模块共享同一版本号，consumers 不需要关心兼容性矩阵
# （cache@v1.2.0 永远匹配 orm@v1.2.0）。借鉴 Kubernetes staging modules 模式。
#
# 用法：
#   bash scripts/release-all.sh v1.0.0                # 实际打 tag
#   bash scripts/release-all.sh v1.0.0 --dry-run      # 仅展示要打哪些 tag
#   bash scripts/release-all.sh v1.0.0 --push         # 打 tag 并推送
#
# 标签命名约定：
#   - 根模块: vX.Y.Z              (e.g. v1.0.0)
#   - 子模块: <dir>/vX.Y.Z        (e.g. cache/v1.0.0, orm/v1.0.0)
#
# 前置检查：
#   1. 工作区干净（无 uncommitted 改动）
#   2. 当前在 main 分支
#   3. 子模块 go.mod 不能包含 intra-workspace replace 指令
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if [ $# -lt 1 ]; then
    echo "Usage: $0 <version> [--dry-run|--push]"
    echo "Example: $0 v1.0.0 --push"
    exit 1
fi

VERSION="$1"
MODE="${2:-confirm}"  # 默认需要交互确认，--dry-run 不动作，--push 直接打 tag 并推送

# 校验版本号格式
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.+-]+)?$ ]]; then
    echo "Error: 版本号格式不合法（需要 vMAJOR.MINOR.PATCH[-suffix]）: $VERSION" >&2
    exit 1
fi

# ─── 前置检查 ──────────────────────────────────────────────────────────────
echo "── 前置检查 ──"

# 1. 工作区干净
if [ "$MODE" != "--dry-run" ] && [ -n "$(git -C "$ROOT" status --porcelain)" ]; then
    echo "✗ 工作区有未提交的改动，请先 commit 或 stash" >&2
    git -C "$ROOT" status --short >&2
    exit 1
fi

# 2. 当前分支
current_branch=$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)
if [ "$current_branch" != "main" ] && [ "$MODE" != "--dry-run" ]; then
    echo "⚠️  当前不在 main 分支（在 $current_branch）"
    read -r -p "继续吗？[y/N] " yn
    [[ "$yn" =~ ^[Yy]$ ]] || exit 1
fi

# 3. 子模块 go.mod 不能含 intra-workspace replace
found_replace=0
while IFS= read -r gomod; do
    if grep -q "astra-go/astra.*=>" "$gomod" 2>/dev/null; then
        echo "✗ 含有 intra-workspace replace 指令，不能发布: ${gomod#$ROOT/}" >&2
        grep "astra-go/astra.*=>" "$gomod" >&2
        found_replace=1
    fi
done < <(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*")
[ "$found_replace" -eq 1 ] && {
    echo "" >&2
    echo "提示：运行 bash scripts/drop-intra-replaces.sh 自动清理" >&2
    exit 1
}

# 4. 检查这些 tag 是否已存在
echo ""
echo "── 计算 tag 列表 ──"
IFS=$'\n' read -r -d '' -a ALL_MODULES \
    < <(bash "$SCRIPT_DIR/list-modules.sh" --no-examples && printf '\0') || true

TAGS=()
for mod in "${ALL_MODULES[@]}"; do
    if [ "$mod" = "." ]; then
        TAGS+=("$VERSION")
    else
        TAGS+=("${mod}/${VERSION}")
    fi
done

exists=0
for tag in "${TAGS[@]}"; do
    if git -C "$ROOT" rev-parse "$tag" >/dev/null 2>&1; then
        echo "✗ tag 已存在: $tag" >&2
        exists=1
    fi
done
[ "$exists" -eq 1 ] && exit 1

# ─── 展示计划 ──────────────────────────────────────────────────────────────
echo ""
echo "── 将创建 ${#TAGS[@]} 个 tag ──"
for tag in "${TAGS[@]}"; do echo "  • $tag"; done

if [ "$MODE" = "--dry-run" ]; then
    echo ""
    echo "✓ dry-run 完成，未做任何改动"
    exit 0
fi

if [ "$MODE" = "confirm" ]; then
    echo ""
    read -r -p "确认创建以上 ${#TAGS[@]} 个 tag？[y/N] " yn
    [[ "$yn" =~ ^[Yy]$ ]] || { echo "已取消"; exit 0; }
fi

# ─── 创建 tag ──────────────────────────────────────────────────────────────
echo ""
echo "── 创建 tag ──"
for tag in "${TAGS[@]}"; do
    git -C "$ROOT" tag -a "$tag" -m "Release $tag"
    echo "  ✓ created: $tag"
done

# ─── 推送 ───────────────────────────────────────────────────────────────────
if [ "$MODE" = "--push" ]; then
    echo ""
    echo "── 推送 tag 到 origin ──"
    git -C "$ROOT" push origin "${TAGS[@]}"
    echo "✓ 推送完成"
else
    echo ""
    echo "✓ tag 已本地创建。手动推送命令："
    echo "  git push origin ${TAGS[*]}"
fi
