#!/usr/bin/env bash
# drop-intra-replaces.sh — 真正删除所有 go.mod 中的 intra-workspace replace 指令
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
count=0

echo "🔍 开始清理 intra-workspace replace 指令..."

while IFS= read -r gomod; do
    dir="$(dirname "$gomod")"
    
    # 检查是否有 => ../ 的 replace
    if grep -q "=>.*\.\./" "$gomod" 2>/dev/null; then
        cd "$dir"
        
        # 提取所有包含 "../" 的 replace 模块路径
        # 处理两种格式：
        #   1. replace github.com/xxx => ../xxx
        #   2. replace ( \n   github.com/xxx => ../xxx \n )
        
        # 方法：用 go mod edit -json 提取，然后用 jq 过滤
        if command -v jq &> /dev/null; then
            go mod edit -json 2>/dev/null | jq -r '.Replace[] | select(.New.Path | test("\\.\\./")) | .Old.Path' | while read -r modpath; do
                [ -n "$modpath" ] && go mod edit -dropreplace="$modpath" 2>/dev/null || true
            done
        else
            # 没有 jq 的备用方案：直接从 go.mod 提取
            grep "=>.*\.\./" go.mod | awk '{print $1}' | while read -r modpath; do
                [ "$modpath" != "replace" ] && [ -n "$modpath" ] && go mod edit -dropreplace="$modpath" 2>/dev/null || true
            done
        fi
        
        # 用 go mod tidy 清理格式（会删除无效的 replace 块）
        go mod tidy 2>/dev/null || true
        
        echo "✓ cleaned: ${dir#$ROOT/}/go.mod"
        count=$((count + 1))
    fi
done < <(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*" | sort)

echo ""
echo "✅ 完成：清理了 $count 个 go.mod"
