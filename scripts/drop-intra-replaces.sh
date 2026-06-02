#!/usr/bin/env bash
# drop-intra-replaces.sh — 简化版：用 sed 删除所有 intra-workspace replace
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
count=0

echo "🔍 清理 intra-workspace replace 指令..."

for gomod in $(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*" | sort); do
    # 检查是否有 => ../ 的 replace
    if grep -q "=>.*\.\./" "$gomod" 2>/dev/null; then
        echo "Cleaning: $gomod"
        
        # 用 sed 删除所有包含 "=> ../" 的行（单行 replace）
        # macOS sed 需要用 -i.bak 创建备份文件
        sed -i.bak '/=>\.\.\//d' "$gomod"
        rm -f "${gomod}.bak"
        
        # 用 go mod tidy 清理格式（会删除无效的 replace 块）
        dir="$(dirname "$gomod")"
        (cd "$dir" && go mod tidy 2>/dev/null || true)
        
        count=$((count + 1))
    fi
done

echo ""
echo "✅ 完成：清理了 $count 个 go.mod"
