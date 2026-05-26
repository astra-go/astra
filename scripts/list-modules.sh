#!/usr/bin/env bash
# list-modules.sh — 从 go.work 动态枚举所有 workspace 模块目录。
#
# 输出：换行分隔的模块目录路径（相对于仓库根），根模块输出为 "."
#
# 用法：
#   bash scripts/list-modules.sh          # 所有模块
#   bash scripts/list-modules.sh --no-examples  # 排除 examples/ 下的模块
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
GOWORK="$ROOT/go.work"

if [ ! -f "$GOWORK" ]; then
    echo "Error: go.work not found at $GOWORK" >&2
    exit 1
fi

EXCLUDE_EXAMPLES=false
for arg in "$@"; do
    case "$arg" in
        --no-examples) EXCLUDE_EXAMPLES=true ;;
    esac
done

# 从 go.work 的 use 块中提取路径，规范化为相对目录名
# "." 保持为 "."；"./foo" 转为 "foo"
while IFS= read -r line; do
    # 去掉前后空白和注释
    line="${line%%//*}"
    line="${line//[$'\t' ]/}"
    [ -z "$line" ] && continue

    # 规范化：./foo → foo，. → .
    mod="${line#./}"
    [ -z "$mod" ] && mod="."

    if [ "$EXCLUDE_EXAMPLES" = true ] && [[ "$mod" == examples/* ]]; then
        continue
    fi

    echo "$mod"
done < <(awk '/^use[[:space:]]*\(/{p=1;next} /^\)/{p=0} p{print}' "$GOWORK")
