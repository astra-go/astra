#!/usr/bin/env bash
# merge-cover.sh — 合并多个 Go coverprofile 文件为一个。
#
# Go coverprofile 格式：第一行为 "mode: <mode>"，后续每行为覆盖数据。
# 多文件合并时保留第一个文件的 mode 行，其余文件跳过 mode 行直接追加。
#
# 用法：
#   bash scripts/merge-cover.sh -o coverage/merged.out coverage/*.out
#   bash scripts/merge-cover.sh coverage/merged.out coverage/a.out coverage/b.out
#
set -euo pipefail

OUTPUT=""
INPUTS=()

# 解析参数
while [[ $# -gt 0 ]]; do
    case "$1" in
        -o)
            OUTPUT="$2"
            shift 2
            ;;
        *)
            INPUTS+=("$1")
            shift
            ;;
    esac
done

# 第一个位置参数也可作为 output
if [[ -z "$OUTPUT" && ${#INPUTS[@]} -gt 0 ]]; then
    OUTPUT="${INPUTS[0]}"
    INPUTS=("${INPUTS[@]:1}")
fi

if [[ -z "$OUTPUT" ]]; then
    echo "usage: merge-cover.sh -o <output> <input1> [input2 ...]" >&2
    exit 1
fi

# 展开 glob（调用方传入字符串 glob 时处理）
expanded=()
for pattern in "${INPUTS[@]}"; do
    for f in $pattern; do
        [[ -f "$f" ]] && expanded+=("$f")
    done
done

if [[ ${#expanded[@]} -eq 0 ]]; then
    echo "merge-cover: no input files found" >&2
    exit 1
fi

mkdir -p "$(dirname "$OUTPUT")"

first=1
for f in "${expanded[@]}"; do
    if [[ $first -eq 1 ]]; then
        # 保留第一个文件的 mode 行
        cat "$f" > "$OUTPUT"
        first=0
    else
        # 跳过 mode 行，追加覆盖数据
        tail -n +2 "$f" >> "$OUTPUT"
    fi
done

total=$(grep -c "^[^m]" "$OUTPUT" 2>/dev/null || echo 0)
echo "merged ${#expanded[@]} coverprofile(s) → $OUTPUT  ($total coverage lines)"
