#!/usr/bin/env bash
# fix-require-versions.sh — 将所有 intra-workspace 依赖的伪版本改为指定版本
#
# 用法：bash scripts/fix-require-versions.sh <version>
# 示例：bash scripts/fix-require-versions.sh v0.1.0

set -euo pipefail

VERSION="${1:-v0.1.0}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ZERO="v0.0.0-00010101000000-000000000000"

echo ":: 将 intra-workspace 依赖从 $ZERO 更新为 $VERSION"

# 获取所有 workspace 模块的 module path
INTRA_MODS=()
while IFS= read -r mod_dir; do
    local_dir="$ROOT/$mod_dir"
    [ "$mod_dir" = "." ] && local_dir="$ROOT"
    mod_path=$(grep "^module " "$local_dir/go.mod" 2>/dev/null | awk '{print $2}' || echo "")
    [ -n "$mod_path" ] && INTRA_MODS+=("$mod_path")
done < <(bash "$ROOT/scripts/list-modules.sh")

updated=0
skipped=0

for gomod in $(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*" | sort); do
    modified=false
    
    for mod in "${INTRA_MODS[@]}"; do
        # 检查是否有对这个模块的伪版本依赖
        if grep -q "$mod $ZERO" "$gomod" 2>/dev/null; then
            # 替换为真实版本
            sed -i.bak "s|$mod $ZERO|$mod $VERSION|g" "$gomod"
            modified=true
            echo "  ✓ $(basename $(dirname $gomod))/go.mod: $mod -> $VERSION"
        fi
    done
    
    if [ "$modified" = true ]; then
        rm -f "$gomod.bak"
        updated=$((updated + 1))
    else
        skipped=$((skipped + 1))
    fi
done

echo ""
echo "✓ 完成：更新 $updated 个 go.mod，跳过 $skipped 个"
echo ""
echo ":: 下一步："
echo "  1. 运行 'bash scripts/tidy-all.sh' 验证依赖"
echo "  2. 提交更改：git add -A && git commit -m 'chore: update dependencies to $VERSION'"
echo "  3. 发布：VERSION=$VERSION make release"
