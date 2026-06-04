#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# release.sh — astra 多模块发版脚本
# 支持主模块 + 子模块独立或统一发版
# ============================================================

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

MODULE_PREFIX="github.com/astra-go/astra"
ZERO_VERSION="v0.0.0-00010101000000-000000000000"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# ---- 用法 ----
usage() {
    cat <<EOF
用法: $0 [命令] [选项]

命令:
  bump <版本号>            更新所有模块版本号
  tag   <版本号>           为所有模块打 tag
  push  <版本号>           推送 commit + tag
  bump-pkg <子模块> <版本号>  更新指定子模块版本号
  tag-pkg  <子模块> <版本号>  为指定子模块打 tag

选项:
  --main <版本号>          主模块版本号（默认与子模块相同）
  --pkg  <子模块,子模块>   指定子模块（逗号分隔）
  --dry-run                预览模式，不实际执行
  --skip-bump-fix          跳过零版本号修复
  --no-verify              git commit 使用 --no-verify

子模块列表:
$(list_submodules)

示例:
  # 全量发版 v1.0.5
  $0 bump v1.0.5 && $0 tag v1.0.5 && $0 push v1.0.5

  # 主模块 v2.0.0 + 子模块 v1.0.5
  $0 bump v1.0.5 --main v2.0.0

  # 只发布 cache 子模块
  $0 bump-pkg cache v1.0.5
  $0 tag-pkg cache v1.0.5
  $0 push v1.0.5

  # 只发布多个指定子模块
  $0 bump v1.0.5 --pkg cache,grpc,testutil

  # 预览
  $0 tag v1.0.5 --dry-run
EOF
}

# ---- 子模块列表 ----
list_submodules() {
    find . -name "go.mod" -not -path "./.git/*" | while read -r f; do
        dir=$(dirname "$f")
        mod=$(grep "^module " "$f" | sed "s|module ||")
        if [[ "$mod" == "$MODULE_PREFIX" ]]; then
            echo "  (主模块)  $dir"
        elif [[ "$mod" == "$MODULE_PREFIX/"* ]]; then
            sub=${mod#$MODULE_PREFIX/}
            echo "  $sub"
        fi
    done | sort
}

# ---- 获取所有子模块路径 ----
get_all_submodules() {
    find . -name "go.mod" -not -path "./.git/*" | while read -r f; do
        dir=$(dirname "$f")
        mod=$(grep "^module " "$f" | sed "s|module ||")
        if [[ "$mod" == "$MODULE_PREFIX/"* ]]; then
            echo "${mod#$MODULE_PREFIX/}"
        fi
    done | sort
}

# ---- 收集所有子模块 go.mod 文件 ----
collect_go_mods() {
    find . -name "go.mod" -not -path "./.git/*"
}

# ---- 修复零版本号：将 require 中的零版本改为上一个真实版本 ----
fix_zero_versions() {
    local prev_version="$1"
    echo -e "${CYAN}🔧 修复零版本号 → ${prev_version}${NC}"
    collect_go_mods | while read -r f; do
        # 只替换 require 行（不含 =>）
        if grep -q "$ZERO_VERSION" "$f" && ! grep -q "=>" "$f"; then
            sed -i '' "/=>/!s|\\(${MODULE_PREFIX}[^ ]*\\) ${ZERO_VERSION}|\\1 ${prev_version}|g" "$f" 2>/dev/null || true
        fi
        # 通用替换：非 replace 行中的零版本
        sed -i '' "/=>/!s|\\(${MODULE_PREFIX}[^ ]*\\) ${ZERO_VERSION}|\\1 ${prev_version}|g" "$f" 2>/dev/null || true
    done
    local remaining=$(grep -r "$ZERO_VERSION" --include="go.mod" . 2>/dev/null | grep -v "=>" | wc -l | tr -d ' ')
    echo "  剩余零版本引用: ${remaining}"
}

# ---- 同步 replace ----
sync_replaces() {
    if [[ -x "scripts/check-intra-replaces.sh" ]]; then
        echo -e "${CYAN}🔧 同步 replace 指令${NC}"
        bash scripts/check-intra-replaces.sh --fix 2>&1 | tail -3
    fi
}

# ---- bump 命令 ----
do_bump() {
    local version="$1"
    local main_version="${2:-$version}"
    local dry_run="${3:-false}"
    local skip_fix="${4:-false}"
    local pkgs="${5:-}"

    echo -e "${GREEN}📦 Bump → 子模块: ${version}, 主模块: ${main_version}${NC}"

    if [[ "$dry_run" == "true" ]]; then
        echo -e "${YELLOW}[DRY RUN] 将更新以下 go.mod 中的版本号:${NC}"
        if [[ -n "$pkgs" ]]; then
            echo "  指定子模块: $pkgs"
        else
            echo "  所有模块"
        fi
        return 0
    fi

    # 使用 bump-version.sh 如果存在
    if [[ -x "scripts/bump-version.sh" ]]; then
        echo "y" | ./scripts/bump-version.sh "$version" 2>&1 | tail -3
    fi

    # 修复零版本号
    if [[ "$skip_fix" != "true" ]]; then
        # 推断上一个版本号
        local prev=$(echo "$version" | awk -F. '{printf("v%d.%d.%d", $1, $2, $3-1)}')
        [[ "$prev" == "v0.0.-1" ]] && prev="v0.0.0"
        fix_zero_versions "$prev"
    fi

    sync_replaces
}

# ---- tag 命令 ----
do_tag() {
    local version="$1"
    local main_version="${2:-$version}"
    local dry_run="${3:-false}"
    local pkgs="${4:-}"

    echo -e "${GREEN}🏷️  Tag → 子模块: ${version}, 主模块: ${main_version}${NC}"

    # 收集要打 tag 的子模块
    local targets=()
    if [[ -n "$pkgs" ]]; then
        IFS=',' read -ra targets <<< "$pkgs"
    else
        while IFS= read -r sub; do
            targets+=("$sub")
        done < <(get_all_submodules)
    fi

    # 打子模块 tag
    for sub in "${targets[@]}"; do
        local tag="${sub}/${version}"
        if git rev-parse "$tag" >/dev/null 2>&1; then
            echo -e "${YELLOW}⚠️  tag ${tag} 已存在，跳过${NC}"
        else
            if [[ "$dry_run" == "true" ]]; then
                echo -e "${YELLOW}[DRY RUN] git tag ${tag}${NC}"
            else
                git tag "$tag"
                echo -e "  ✅ ${tag}"
            fi
        fi
    done

    # 打主模块 tag
    if git rev-parse "$main_version" >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  tag ${main_version} 已存在，跳过${NC}"
    else
        if [[ "$dry_run" == "true" ]]; then
            echo -e "${YELLOW}[DRY RUN] git tag ${main_version}${NC}"
        else
            git tag "$main_version"
            echo -e "  ✅ ${main_version}"
        fi
    fi
}

# ---- push 命令 ----
do_push() {
    local version="$1"
    local main_version="${2:-$version}"
    local no_verify="${3:-false}"

    echo -e "${GREEN}🚀 推送 main + 所有 tag${NC}"

    local verify_flag=""
    [[ "$no_verify" == "true" ]] && verify_flag="--no-verify"

    # 先提交未暂存的修改
    if ! git diff --quiet || ! git diff --cached --quiet; then
        echo -e "${CYAN}📝 提交变更${NC}"
        git add -A
        git commit $verify_flag -m "chore: release ${main_version}" 2>&1 | tail -3
    fi

    git push origin main --tags 2>&1 | tail -10
}

# ---- bump-pkg 命令：只更新指定子模块 ----
do_bump_pkg() {
    local pkg="$1"
    local version="$2"
    local dry_run="${3:-false}"

    echo -e "${GREEN}📦 Bump 单包 → ${pkg}: ${version}${NC}"

    if [[ "$dry_run" == "true" ]]; then
        echo -e "${YELLOW}[DRY RUN] 将更新 ${pkg} 相关版本号${NC}"
        return 0
    fi

    # 在所有 go.mod 中查找并替换该子包的版本
    local pkg_path="${MODULE_PREFIX}/${pkg}"
    collect_go_mods | while read -r f; do
        sed -i '' "/=>/!s|\\(${pkg_path}\\) v[0-9][^ ]*|\\1 ${version}|g" "$f" 2>/dev/null || true
    done

    # 修复零版本号
    local prev=$(echo "$version" | awk -F. '{printf("v%d.%d.%d", $1, $2, $3-1)}')
    [[ "$prev" == "v0.0.-1" ]] && prev="v0.0.0"
    fix_zero_versions "$prev"

    sync_replaces
    echo -e "  ✅ ${pkg} 版本号已更新为 ${version}"
}

# ---- tag-pkg 命令：只为指定子模块打 tag ----
do_tag_pkg() {
    local pkg="$1"
    local version="$2"
    local dry_run="${3:-false}"

    echo -e "${GREEN}🏷️  Tag 单包 → ${pkg}/${version}${NC}"

    if [[ "$dry_run" == "true" ]]; then
        echo -e "${YELLOW}[DRY RUN] git tag ${pkg}/${version}${NC}"
        return 0
    fi

    local tag="${pkg}/${version}"
    if git rev-parse "$tag" >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  tag ${tag} 已存在，跳过${NC}"
    else
        git tag "$tag"
        echo -e "  ✅ ${tag}"
    fi
}

# ---- 解析参数 ----
CMD=""
VERSION=""
MAIN_VERSION=""
PKGS=""
DRY_RUN=false
SKIP_FIX=false
NO_VERIFY=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        bump|tag|push|bump-pkg|tag-pkg)
            CMD="$1"
            shift
            ;;
        --main)
            MAIN_VERSION="$2"
            shift 2
            ;;
        --pkg)
            PKGS="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --skip-bump-fix)
            SKIP_FIX=true
            shift
            ;;
        --no-verify)
            NO_VERIFY=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            if [[ -z "$VERSION" ]]; then
                VERSION="$1"
            fi
            shift
            ;;
    esac
done

if [[ -z "$CMD" ]] || [[ -z "$VERSION" ]]; then
    usage
    exit 1
fi

[[ -z "$MAIN_VERSION" ]] && MAIN_VERSION="$VERSION"

# ---- 执行 ----
case "$CMD" in
    bump)       do_bump "$VERSION" "$MAIN_VERSION" "$DRY_RUN" "$SKIP_FIX" "$PKGS" ;;
    tag)        do_tag "$VERSION" "$MAIN_VERSION" "$DRY_RUN" "$PKGS" ;;
    push)       do_push "$VERSION" "$MAIN_VERSION" "$NO_VERIFY" ;;
    bump-pkg)   do_bump_pkg "$VERSION" "$MAIN_VERSION" "$DRY_RUN" ;;
    tag-pkg)    do_tag_pkg "$VERSION" "$MAIN_VERSION" "$DRY_RUN" ;;
    *)          usage; exit 1 ;;
esac



# 全量发版 v1.0.5（一条龙）
# ./scripts/release.sh bump v1.0.5 && ./scripts/release.sh tag v1.0.5 && ./scripts/release.sh push v1.0.5 --no-verify

# 主模块 v2.0.0 + 子模块 v1.0.5
# ./scripts/release.sh bump v1.0.5 --main v2.0.0

# 只发布单个子模块（如 cache）
# ./scripts/release.sh bump-pkg cache v1.0.5
# ./scripts/release.sh tag-pkg cache v1.0.5
# ./scripts/release.sh push v1.0.5 --no-verify

# 只发布指定多个子模块
# ./scripts/release.sh bump v1.0.5 --pkg cache,grpc,testutil
# ./scripts/release.sh tag v1.0.5 --pkg cache,grpc,testutil

# 预览（不实际执行）
# ./scripts/release.sh tag v1.0.5 --dry-run
