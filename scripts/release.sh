#!/bin/bash
set -e

# ============================================
# Astra 一键发布脚本
# Usage: bash scripts/release.sh <version>
# Example: bash scripts/release.sh v1.0.1
# ============================================

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 使用方式
usage() {
    echo "Usage: $0 <version>"
    echo ""
    echo "Arguments:"
    echo "  <version>  Version tag (e.g., v1.0.1, v2.0.0)"
    echo ""
    echo "Example:"
    echo "  $0 v1.0.1"
    echo ""
    echo "Environment Variables:"
    echo "  DRY_RUN=1    Dry-run mode (no actual changes)"
    echo "  AUTO_CONFIRM=1  Skip all confirmations"
    exit 1
}

# 检查依赖
check_dependencies() {
    local deps=("git" "go" "mage" "sed" "find")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            echo -e "${RED}Error: $dep is not installed${NC}"
            exit 1
        fi
    done
}

# 检查参数
if [ $# -lt 1 ]; then
    usage
fi

VERSION=$1
DRY_RUN=${DRY_RUN:-0}
AUTO_CONFIRM=${AUTO_CONFIRM:-0}

# 检查 VERSION 格式
if [[ ! $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo -e "${RED}Error: VERSION must be vMAJOR.MINOR.PATCH format (e.g., v1.0.1)${NC}"
    exit 1
fi

# 检查依赖
check_dependencies

echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}🚀 Astra Release Script${NC}"
echo -e "${GREEN}   Version: $VERSION${NC}"
echo -e "${GREEN}   Dry Run: $([ $DRY_RUN -eq 1 ] && echo "YES" || echo "NO")${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# 检查当前目录
if [ ! -f "go.work" ]; then
    echo -e "${RED}Error: Must run from astra root directory${NC}"
    echo "Hint: cd ~/data/project/gotest/astra"
    exit 1
fi

# 检查 git 状态
echo -e "${BLUE}Step 0: Checking git status...${NC}"
if [ -n "$(git status --porcelain)" ]; then
    echo -e "${RED}Error: Working tree has uncommitted changes${NC}"
    git status --short
    echo ""
    echo "Please commit or stash changes first."
    exit 1
fi
echo -e "${GREEN}✓ Working tree is clean${NC}"
echo ""

# Step 1: 切换到 main 分支
echo -e "${BLUE}Step 1: Switching to main branch...${NC}"
git checkout main 2>&1 || true
git pull origin main
echo -e "${GREEN}✓ Now on main branch${NC}"
echo ""

# Step 2: 合并 xiaolin 分支
echo -e "${BLUE}Step 2: Merging xiaolin branch...${NC}"
if git branch --list | grep -q "xiaolin"; then
    git merge xiaolin --no-edit 2>&1 || {
        echo -e "${RED}Error: Merge conflict${NC}"
        echo "Please resolve conflicts manually."
        exit 1
    }
    echo -e "${GREEN}✓ xiaolin merged into main${NC}"
else
    echo -e "${YELLOW}⚠ xiaolin branch not found, skipping merge${NC}"
fi
echo ""

# Step 3: 清理 replace 指令
echo -e "${BLUE}Step 3: Cleaning replace directives...${NC}"

# 方法1: 使用 drop-intra-replaces.sh
if [ -f "scripts/drop-intra-replaces.sh" ]; then
    bash scripts/drop-intra-replaces.sh 2>&1 || true
fi

# 方法2: 强制用 sed 清理（double guarantee）
echo "  Force clean with sed..."
find . -name "go.mod" -not -path "./.git/*" -exec sed -i '' '/^[[:space:]]*replace/d' {} + 2>/dev/null || true

# 验证清理结果
echo -e "${BLUE}  Verifying clean state...${NC}"
if grep -r "replace" --include="go.mod" . 2>/dev/null; then
    echo -e "${RED}Error: Still have replace directives${NC}"
    echo "Please clean manually:"
    echo "  find . -name 'go.mod' -exec sed -i '' '/^[[:space:]]*replace/d' {} +"
    exit 1
fi
echo -e "${GREEN}✓ All replace directives removed${NC}"
echo ""

# Step 4: 提交干净状态
echo -e "${BLUE}Step 4: Committing clean state...${NC}"
git add -A
git commit --no-verify -m "chore: clean go.mod for $VERSION release" 2>&1 || {
    echo -e "${YELLOW}⚠ Nothing to commit (already clean)${NC}"
}
echo -e "${GREEN}✓ Clean state committed${NC}"
echo ""

# Step 5: 执行发布
echo -e "${BLUE}Step 5: Running mage release...${NC}"
if [ $DRY_RUN -eq 1 ]; then
    echo -e "${YELLOW}[DRY RUN] Would execute: AUTO_CONFIRM=1 VERSION=$VERSION make release${NC}"
else
    AUTO_CONFIRM=1 VERSION=$VERSION make release 2>&1 || {
        echo -e "${RED}Error: make release failed${NC}"
        echo ""
        echo "Troubleshooting:"
        echo "  1. Check magefiles/release.go exists"
        echo "  2. Run: mage -d magefiles -l"
        echo "  3. Check go.mod files still have replace?"
        exit 1
    }
fi
echo -e "${GREEN}✓ Release tags created${NC}"
echo ""

# Step 6: 推送 tag
echo -e "${BLUE}Step 6: Pushing tags to remote...${NC}"
if [ $DRY_RUN -eq 1 ]; then
    echo -e "${YELLOW}[DRY RUN] Would push tags:${NC}"
    git tag -l "*$VERSION" | while read tag; do
        echo "  git push origin $tag"
    done
else
    # 先删除远程已存在的 tag（如果有）
    for tag in $(git tag -l "*$VERSION"); do
        echo "  Deleting remote tag: $tag"
        git push origin :refs/tags/$tag 2>/dev/null || true
    done
    
    # 推送新 tag
    echo "  Pushing tags..."
    git push origin $(git tag -l "*$VERSION" | tr '\n' ' ') 2>&1 || {
        echo -e "${RED}Error: Failed to push tags${NC}"
        exit 1
    }
fi
echo -e "${GREEN}✓ Tags pushed to remote${NC}"
echo ""

# Step 7: 恢复 replace 指令
echo -e "${BLUE}Step 7: Restoring replace directives...${NC}"
if [ -f "scripts/sync-intra-replaces.sh" ]; then
    bash scripts/sync-intra-replaces.sh 2>&1 || {
        echo -e "${RED}Error: Failed to restore replace directives${NC}"
        echo "Please run manually: bash scripts/sync-intra-replaces.sh"
        exit 1
    }
    echo -e "${GREEN}✓ Replace directives restored${NC}"
else
    echo -e "${YELLOW}⚠ sync-intra-replaces.sh not found, skipping${NC}"
fi
echo ""

# Step 8: 提交恢复后的状态
echo -e "${BLUE}Step 8: Committing restored state...${NC}"
git add -A
git commit --no-verify -m "chore: restore replace after $VERSION release" 2>&1 || {
    echo -e "${YELLOW}⚠ Nothing to commit (already restored)${NC}"
}
echo -e "${GREEN}✓ Restored state committed${NC}"
echo ""

# Step 9: 推送到远程
echo -e "${BLUE}Step 9: Pushing to remote...${NC}"
if [ $DRY_RUN -eq 1 ]; then
    echo -e "${YELLOW}[DRY RUN] Would push main branch${NC}"
else
    git push origin main 2>&1 || {
        echo -e "${RED}Error: Failed to push main${NC}"
        exit 1
    }
fi
echo -e "${GREEN}✓ Main branch pushed${NC}"
echo ""

# Step 10: 切换回 xiaolin 分支
echo -e "${BLUE}Step 10: Switching back to xiaolin branch...${NC}"
if git branch --list | grep -q "xiaolin"; then
    git checkout xiaolin 2>&1 || true
    git rebase main 2>&1 || {
        echo -e "${YELLOW}⚠ Rebase failed, please resolve conflicts manually${NC}"
    }
    git push origin xiaolin --force-with-lease 2>&1 || true
    echo -e "${GREEN}✓ xiaolin branch updated${NC}"
else
    echo -e "${YELLOW}⚠ xiaolin branch not found, staying on main${NC}"
fi
echo ""

# 完成
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✅ Release $VERSION completed successfully!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "Next steps:"
echo "  1. Verify: go list -m github.com/astra-go/astra@$VERSION"
echo "  2. Test: go get github.com/astra-go/astra@$VERSION"
echo "  3. Check: https://github.com/astra-go/astra/tags"
echo ""
