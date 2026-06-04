#!/usr/bin/env bash
#
# prepare-release.sh — 发版前检查和修复 go.mod 配置
#
# 功能:
#   1. 检查所有子模块 go.mod 中的 astra 内部依赖版本号是否为零版本
#   2. 检查是否有遗留的 astra replace 行（子模块不应有）
#   3. 自动修复这些问题
#   4. 检查 go.work 是否有错误的 replace 行
#
# 用法:
#   ./prepare-release.sh              # 检查并修复
#   ./prepare-release.sh --dry-run   # 只检查，不修复
#   ./prepare-release.sh --check      # 只检查，退出码 0=正常 1=需要修复
#   ./prepare-release.sh --fix       # 强制修复（默认行为）
#
# 示例:
#   ./scripts/prepare-release.sh
#   ./scripts/prepare-release.sh --dry-run
#   ./scripts/prepare-release.sh --check && echo "OK" || echo "Need fix"

set -euo pipefail

# ─── 颜色定义 ───
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# ─── 参数解析 ───
DRY_RUN=false
CHECK_ONLY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)   DRY_RUN=true; shift ;;
    --check)     CHECK_ONLY=true; shift ;;
    --fix)       DRY_RUN=false; CHECK_ONLY=false; shift ;;
    *) echo "未知选项: $1"; exit 1 ;;
  esac
done

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo '.')"
cd "$REPO_ROOT"

echo "🔍 检查 go.mod 配置..."
echo ""

ISSUES_FOUND=0
FIX_COMMANDS=()

# ─── 函数：检查单个 go.mod 文件 ───
check_go_mod() {
  local file="$1"
  local issues=0
  
  # 1. 检查 require 行中的非零版本（排除有 "=>" 的行，那些是 replace）
  local non_zero_requires
  non_zero_requires=$(grep "astra-go/astra" "$file" 2>/dev/null | \
    grep -v "=>" | \
    grep -v "v0.0.0-00010101000000-000000000000" | \
    grep -v "^module" | grep -v "^//" || true)
  
  if [[ -n "$non_zero_requires" ]]; then
    echo -e "${YELLOW}⚠️  $file 中有非零版本的 astra 依赖:${NC}"
    echo "$non_zero_requires" | sed 's/^/    /'
    issues=$((issues + 1))
    # 添加修复命令
    FIX_COMMANDS+=("sed -i '' 's|\\(github.com/astra-go/astra[^ ]*\\) v[0-9][^ ]*|\\1 v0.0.0-00010101000000-000000000000|g' \"$file\"")
  fi
  
  # 2. 检查 astra 的 replace 行（子模块不应有，主模块可以有）
  if [[ "$file" != "./go.mod" ]]; then
    local astra_replaces
    astra_replaces=$(grep "astra-go/astra" "$file" 2>/dev/null | grep "=>" || true)
    
    if [[ -n "$astra_replaces" ]]; then
      echo -e "${YELLOW}⚠️  $file 中有 astra replace 行（应删除）:${NC}"
      echo "$astra_replaces" | sed 's/^/    /'
      issues=$((issues + 1))
      # 添加修复命令：删除包含 astra-go/astra 和 "=>" 的行
      FIX_COMMANDS+=("sed -i '' '/astra-go\\/astra.*=>/d' \"$file\"")
      # 还需要清理空的 replace () 块（可选）
    fi
  fi
  
  return $issues
}

# ─── 检查所有子模块 ───
echo "📦 检查子模块 go.mod..."
echo ""

while IFS= read -r go_mod; do
  [[ -z "$go_mod" ]] && continue
  [[ "$go_mod" == "./go.mod" ]] && continue
  
  if ! check_go_mod "$go_mod" 2>/dev/null; then
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
  else
    echo -e "${GREEN}✅ $go_mod${NC}"
  fi
  
done < <(find . -name go.mod -not -path "./.git/*" | sed 's|^\./||' | sort)

echo ""

# ─── 检查主模块 go.mod ───
echo "📦 检查主模块 go.mod..."
echo ""

if ! check_go_mod "./go.mod" 2>/dev/null; then
  ISSUES_FOUND=$((ISSUES_FOUND + 1))
else
  echo -e "${GREEN}✅ go.mod${NC}"
fi

echo ""

# ─── 检查 go.work ───
echo "📦 检查 go.work..."
echo ""

if [[ -f "go.work" ]]; then
  # 检查是否有非零版本的 astra replace
  non_zero_work=$(grep "astra-go/astra" go.work 2>/dev/null | \
    grep "=>" | \
    grep -v "v0.0.0-00010101000000-000000000000" || true)
  
  if [[ -n "$non_zero_work" ]]; then
    echo -e "${YELLOW}⚠️  go.work 中有非零版本的 replace:${NC}"
    echo "$non_zero_work" | sed 's/^/    /'
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
    FIX_COMMANDS+=("sed -i '' '/astra-go\\/astra.*v[0-9][^ ]* =>/d' go.work")
  else
    echo -e "${GREEN}✅ go.work${NC}"
  fi
else
  echo -e "${YELLOW}⚠️  未找到 go.work${NC}"
fi

echo ""

# ─── 汇总 ───
if [[ $ISSUES_FOUND -eq 0 ]]; then
  echo -e "${GREEN}✅ 所有检查通过，无需修复${NC}"
  
  if [[ "$CHECK_ONLY" == true ]]; then
    exit 0
  fi
  exit 0
fi

echo -e "${YELLOW}⚠️  发现 $ISSUES_FOUND 个问题${NC}"
echo ""

# ─── 执行修复 ───
if [[ "$CHECK_ONLY" == true ]]; then
  echo "🔍 --check 模式，不执行修复"
  exit 1
fi

if [[ "$DRY_RUN" == true ]]; then
  echo "🔍 --dry-run 模式，预览修复命令:"
  printf '  %s\n' "${FIX_COMMANDS[@]}"
  echo ""
  exit 0
fi

# 确认
read -rp "确认执行修复？[y/N] " confirm
if [[ ! "$confirm" =~ ^[yY]$ ]]; then
  echo "已取消"
  exit 0
fi

echo ""
echo "🔧 执行修复..."

# 执行修复命令
for cmd in "${FIX_COMMANDS[@]}"; do
  echo "  $cmd"
  eval "$cmd" || echo "    ⚠️  命令执行失败，继续..."
done

# 清理可能残留的空 replace () 块
echo ""
echo "🧹 清理空的 replace 块..."
for go_mod in $(find . -name go.mod -not -path "./.git/*" | sed 's|^\./||'); do
  [[ "$go_mod" == "go.mod" ]] && continue
  # 删除只有注释或空行的 replace 块（简单处理：删除文件末尾的空 replace 块）
  # 更安全的做法：用 go mod edit，但这里用 sed 简单处理
  python3 -c "
import re, sys
with open('$go_mod', 'r') as f:
    content = f.read()
# 删除空的 replace () 块
content = re.sub(r'\nreplace \(\n\)\n?', '\n', content)
with open('$go_mod', 'w') as f:
    f.write(content)
" 2>/dev/null || true
done

echo ""
echo -e "${GREEN}✅ 修复完成${NC}"
echo ""
echo "📋 建议下一步:"
echo "  1. 检查修改: git diff"
echo "  2. 提交修改: git add -A && git commit -m 'chore: clean go.mod for release'"
echo "  3. 运行发版: ./scripts/release-tags.sh v1.0.3"
