#!/usr/bin/env bash
# sync-intra-replaces.sh — 同步所有 go.mod 的 intra-workspace replace 指令。
#
# 策略：让每个 go.mod 都拥有 go.work 中所有 workspace 模块的 replace 指令，
# 使 `go mod tidy` 在添加新的 workspace require 时能正确解析为零版本伪依赖
# （go.work 的 replace 不会被 tidy 用于版本决策）。
#
# 这些 replace 是本地开发态的副产物，发布前由 release-all.sh 自动剥离。
#
# 用法：bash scripts/sync-intra-replaces.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ZERO="v0.0.0-00010101000000-000000000000"

# ─── 1. 收集所有 workspace 模块的 module path 和本地路径 ─────────────────
ALL_MODULES=()
while IFS= read -r line; do
    ALL_MODULES+=("$line")
done < <(bash "$SCRIPT_DIR/list-modules.sh")

# 写 module_path → local_dir 映射到临时文件
TMPFILE=$(mktemp)
trap "rm -f $TMPFILE" EXIT

for mod_dir in "${ALL_MODULES[@]}"; do
    local_dir="$ROOT/$mod_dir"
    [ "$mod_dir" = "." ] && local_dir="$ROOT"
    mod_path=$(grep "^module " "$local_dir/go.mod" 2>/dev/null | awk '{print $2}' || echo "")
    [ -z "$mod_path" ] && continue
    # 跳过 example/* 模块（这些是 examples 的不规范命名，不应作为别人 replace 目标）
    [[ "$mod_path" == example/* ]] && continue
    if [ "$mod_dir" = "." ]; then
        echo "$mod_path ."
    else
        echo "$mod_path $mod_dir"
    fi
done > "$TMPFILE"

# ─── 2. 为每个 go.mod 同步 replace 指令 ─────────────────────────────────
synced=0
while IFS= read -r gomod; do
    dir="$(dirname "$gomod")"
    own_module=$(grep "^module " "$gomod" | awk '{print $2}')

    # 计算从该 go.mod 所在目录到 ROOT 的相对前缀（用于 ../ 跳出）
    if [ "$dir" = "$ROOT" ]; then
        prefix=""
    else
        # 例如 examples/crud → depth=2 → ../../
        rel_to_root="${dir#$ROOT/}"
        depth=$(awk -F'/' '{print NF}' <<< "$rel_to_root")
        prefix=""
        for _ in $(seq 1 "$depth"); do prefix="${prefix}../"; done
    fi

    # 为每个 workspace 模块 upsert replace 指令
    n=0
    while IFS= read -r line; do
        dep_path=$(echo "$line" | awk '{print $1}')
        dep_local_dir=$(echo "$line" | awk '{print $2}')

        # 跳过对自己的 replace
        [ "$dep_path" = "$own_module" ] && continue

        # 拼接相对路径
        if [ "$dep_local_dir" = "." ]; then
            rel_path="${prefix%/}"
            [ -z "$rel_path" ] && rel_path="."
        else
            rel_path="${prefix}${dep_local_dir}"
        fi

        (cd "$dir" && go mod edit -replace="${dep_path}@${ZERO}=${rel_path}" 2>/dev/null) || true
        n=$((n + 1))
    done < "$TMPFILE"

    echo "✓ synced: ${gomod#$ROOT/} ($n replace 指令)"
    synced=$((synced + 1))
done < <(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*" | sort)

echo ""
echo "✓ 完成：同步 $synced 个 go.mod"
