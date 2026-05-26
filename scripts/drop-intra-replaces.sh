#!/usr/bin/env bash
# drop-intra-replaces.sh — 删除所有 go.mod 中的 intra-workspace replace 指令。
#
# 发布前执行：将所有 go.mod 恢复为纯净状态，使 GOPROXY 消费者能正确解析依赖。
# 发布后可再次运行 sync-intra-replaces.sh 恢复本地开发态。
#
# 用法：bash scripts/drop-intra-replaces.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ZERO="v0.0.0-00010101000000-000000000000"

# ─── 动态收集所有 workspace 模块的 module path（从 go.work 派生）──────────
INTRA_MODS=()
while IFS= read -r mod_dir; do
    local_dir="$ROOT/$mod_dir"
    [ "$mod_dir" = "." ] && local_dir="$ROOT"
    mod_path=$(grep "^module " "$local_dir/go.mod" 2>/dev/null | awk '{print $2}' || echo "")
    [ -n "$mod_path" ] && INTRA_MODS+=("$mod_path")
done < <(bash "$SCRIPT_DIR/list-modules.sh")

# 构建 dropreplace 参数列表
DROP_ARGS=()
for mod in "${INTRA_MODS[@]}"; do
    DROP_ARGS+=("-dropreplace=${mod}@${ZERO}")
done

cleaned=0
skipped=0

while IFS= read -r gomod; do
    dir="$(dirname "$gomod")"
    if grep -q "astra-go/astra.*=>" "$gomod" 2>/dev/null; then
        (cd "$dir" && go mod edit "${DROP_ARGS[@]}" 2>/dev/null) || true
        echo "✓ cleaned: ${gomod#$ROOT/}"
        cleaned=$((cleaned + 1))
    else
        skipped=$((skipped + 1))
    fi
done < <(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*" | sort)

echo "✓ 完成：清理 $cleaned 个 go.mod，跳过 $skipped 个（已干净）"

