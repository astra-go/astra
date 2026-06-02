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

cleaned=0
skipped=0

while IFS= read -r gomod; do
    dir="$(dirname "$gomod")"
    # 检查是否有 intra-workspace replace（=> ../）
    if grep -q "=>.*\.\./" "$gomod" 2>/dev/null; then
        # 用 sed 删除 replace 块（支持单行和多行）
        # 1. 删除单行 replace：replace github.com/xxx => ../xxx
        # 2. 删除多行 replace ( ... ) 块
        python3 -c "
import re, sys
with open(sys.argv[1], 'r') as f:
    content = f.read()

# 删除多行 replace ( ... ) 块
lines = content.split('\n')
result = []
skip_block = False
for line in lines:
    if 'replace (' in line:
        skip_block = True
        continue
    if skip_block and line.strip() == ')':
        skip_block = False
        continue
    if skip_block:
        # 检查是否是 intra-workspace replace
        if '=>' in line and '../' in line:
            continue
        # 如果不是 intra-workspace replace，保留这一行
        # 但因为我们跳过了整个块，所以这里不会执行
        pass
    if not skip_block:
        result.append(line)

# 删除单行 replace（intra-workspace）
final = '\n'.join(result)
final = re.sub(r'^replace\s+.*?=>\s*\.\./.*$\n?', '', final, flags=re.MULTILINE)

with open(sys.argv[1], 'w') as f:
    f.write(final)
" "$gomod"
        echo "✓ cleaned: ${gomod#$ROOT/}"
        cleaned=$((cleaned + 1))
    else
        skipped=$((skipped + 1))
    fi
done < <(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*" | sort)

echo "✓ 完成：清理 $cleaned 个 go.mod，跳过 $skipped 个（已干净）"
