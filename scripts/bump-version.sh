#!/usr/bin/env bash
#
# bump-version.sh — 更新所有 go.mod 中的 astra 内部依赖版本号
#
# 功能:
#   1. 更新主模块 go.mod 中所有 astra-go/astra 相关依赖的版本号
#   2. 更新所有子模块 go.mod 中所有 astra-go/astra 相关依赖的版本号
#   3. 不修改 go.work（保持零版本号用于本地开发）
#
# 用法:
#   ./bump-version.sh <new-version>        # 更新为指定版本
#   ./bump-version.sh <new-version> --dry-run  # 预览，不执行
#   ./bump-version.sh <new-version> --check   # 检查当前版本是否一致
#
# 示例:
#   ./scripts/bump-version.sh v1.0.3
#   ./scripts/bump-version.sh v1.0.3 --dry-run
#   ./scripts/bump-version.sh v1.0.3 --check
#
# 注意:
#   - 只更新 require 行，不更新 replace 行
#   - go.work 的 replace 保持零版本号不变
#   - 需要在 go.mod 文件所在目录执行

set -euo pipefail

# ─── 颜色定义 ───
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# ─── 参数解析 ───
if [[ $# -lt 1 ]]; then
  echo "用法: $0 <new-version> [--dry-run|--check]"
  echo ""
  echo "示例:"
  echo "  $0 v1.0.3"
  echo "  $0 v1.0.3 --dry-run"
  echo "  $0 v1.0.3 --check"
  exit 1
fi

NEW_VERSION="$1"; shift
DRY_RUN=false
CHECK_ONLY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)  DRY_RUN=true; shift ;;
    --check)     CHECK_ONLY=true; shift ;;
    *) echo "未知选项: $1"; exit 1 ;;
  esac
done

# 校验版本号格式
if [[ ! "$NEW_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
  echo -e "${RED}❌ 版本号格式错误: $NEW_VERSION${NC}"
  echo "   期望格式: v1.0.3 或 v1.0.3-rc.1"
  exit 1
fi

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo '.')"
cd "$REPO_ROOT"

echo "📦 更新版本号到 $NEW_VERSION"
echo ""

UPDATED_FILES=()
FAILED_FILES=()

# ─── 函数：更新单个 go.mod 文件 ───
bump_go_mod() {
  local file="$1"
  local updated=false
  
  echo -e "${YELLOW}📝 处理: $file${NC}"
  
  # 查找需要更新的行（require 中的 astra-go/astra 依赖）
  local lines_to_update
  lines_to_update=$(grep "astra-go/astra" "$file" 2>/dev/null | \
    grep -v "=>" | \
    grep -v "^module" | grep -v "^//" | \
    grep -v "v0.0.0-00010101000000-000000000000" || true)
  
  if [[ -z "$lines_to_update" ]]; then
    echo "  ℹ️  无需更新"
    return 0
  fi
  
  echo "  需要更新:"
  echo "$lines_to_update" | while IFS= read -r line; do
    echo "    $line"
  done
  
  if [[ "$CHECK_ONLY" == true ]]; then
    echo -e "${YELLOW}  ⚠️  需要更新${NC}"
    return 1
  fi
  
  if [[ "$DRY_RUN" == true ]]; then
    echo -e "${YELLOW}  🔍 --dry-run 模式，不执行${NC}"
    UPDATED_FILES+=("$file")
    return 0
  fi
  
  # 执行更新：替换所有 astra-go/astra 依赖的版本号
  # 只替换 require 行（不包含 "=>" 的行）
  local temp_file
  temp_file=$(mktemp)
  
  while IFS= read -r line; do
    if [[ "$line" =~ astra-go/astra ]] && [[ ! "$line" =~ "=>" ]] && [[ ! "$line" =~ "^module" ]] && [[ ! "$line" =~ "^//" ]]; then
      # 替换版本号
      echo "$line" | sed "s|\(github.com/astra-go/astra[^ ]*\) v[0-9][^ ]*|\1 $NEW_VERSION|" >> "$temp_file"
    else
      echo "$line" >> "$temp_file"
    fi
  done < "$file"
  
  mv "$temp_file" "$file"
  
  echo -e "${GREEN}  ✅ 已更新${NC}"
  UPDATED_FILES+=("$file")
  
  return 0
}

# ─── 更新所有 go.mod 文件 ───
echo "🔍 查找 go.mod 文件..."
echo ""

# 1. 更新主模块 go.mod
if [[ -f "go.mod" ]]; then
  bump_go_mod "go.mod" || FAILED_FILES+=("go.mod")
fi

echo ""

# 2. 更新所有子模块 go.mod
echo "📦 更新子模块..."
echo ""

while IFS= read -r go_mod; do
  [[ -z "$go_mod" ]] && continue
  [[ "$go_mod" == "./go.mod" ]] && continue
  
  bump_go_mod "$go_mod" || FAILED_FILES+=("$go_mod")
  
done < <(find . -name go.mod -not -path "./.git/*" | sed 's|^\./||' | sort)

echo ""

# ─── 汇总 ───
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [[ ${#UPDATED_FILES[@]} -gt 0 ]]; then
  echo -e "${GREEN}✅ 已更新 ${#UPDATED_FILES[@]} 个文件:${NC}"
  printf '  %s\n' "${UPDATED_FILES[@]}"
  echo ""
fi

if [[ ${#FAILED_FILES[@]} -gt 0 ]]; then
  echo -e "${RED}❌ 失败 ${#FAILED_FILES[@]} 个文件:${NC}"
  printf '  %s\n' "${FAILED_FILES[@]}"
  echo ""
  exit 1
fi

if [[ "$CHECK_ONLY" == true ]]; then
  echo -e "${YELLOW}⚠️  有文件需要更新，请运行: $0 $NEW_VERSION${NC}"
  exit 1
fi

if [[ "$DRY_RUN" == true ]]; then
  echo -e "${YELLOW}🔍 --dry-run 模式，未实际修改文件${NC}"
  echo ""
  exit 0
fi

echo -e "${GREEN}🎉 版本号更新完成！${NC}"
echo ""
echo "📋 建议下一步:"
echo "  1. 检查修改: git diff"
echo "  2. 提交修改: git add -A && git commit -m 'chore: bump version to $NEW_VERSION'"
echo "  3. 运行发版: ./scripts/release-tags.sh $NEW_VERSION"
