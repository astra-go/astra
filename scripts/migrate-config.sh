#!/bin/bash
# Config 模块迁移脚本
# 用于将旧版 config/nacos, config/etcd, config/apollo 的代码迁移到统一 config 包
#
# 使用方法:
#   ./migrate-config.sh [options]
#
# 选项:
#   -n, --dry-run     预览模式，不实际修改文件
#   -v, --verbose    详细输出
#   -h, --help        显示帮助信息

set -e

# ─── 配置 ─────────────────────────────────────────────────────

DRY_RUN=false
VERBOSE=false
BACKUP_DIR=".migration-backup-$(date +%Y%m%d-%H%M%S)"

# ─── 颜色输出 ─────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ─── 函数 ─────────────────────────────────────────────────────

print_help() {
    cat <<EOF
Config 模块迁移脚本

使用方法:
    $0 [选项]

选项:
    -n, --dry-run     预览模式，不实际修改文件
    -v, --verbose    详细输出
    -h, --help        显示此帮助信息

示例:
    $0                 # 执行迁移
    $0 -n             # 预览模式
    $0 -v             # 详细输出
    $0 -n -v          # 预览模式 + 详细输出

EOF
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_verbose() {
    if [ "$VERBOSE" = true ]; then
        echo -e "${BLUE}[VERBOSE]${NC} $1"
    fi
}

# ─── 参数解析 ─────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
    case $1 in
        -n|--dry-run)
            DRY_RUN=true
            shift
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            print_help
            exit 0
            ;;
        *)
            log_error "未知选项: $1"
            print_help
            exit 1
            ;;
    esac
done

# ─── 前置检查 ─────────────────────────────────────────────────

check_prerequisites() {
    log_info "检查前置条件..."

    # 检查是否在 Git 仓库中
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        log_error "当前目录不是 Git 仓库"
        exit 1
    fi

    # 检查是否有未提交的更改
    if ! git diff-index --quiet HEAD --; then
        log_warn "检测到未提交的更改"
        read -p "是否继续? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi

    # 检查 Go 是否安装
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装或不在 PATH 中"
        exit 1
    fi

    log_success "前置检查通过"
}

# ─── 备份 ─────────────────────────────────────────────────────

create_backup() {
    log_info "创建备份..."

    if [ "$DRY_RUN" = true ]; then
        log_warn "预览模式: 跳过备份"
        return
    fi

    mkdir -p "$BACKUP_DIR"

    # 备份 Go 文件
    find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" -exec cp --parents {} "$BACKUP_DIR/" \;

    # 备份 go.mod 和 go.sum
    find . -name "go.mod" -o -name "go.sum" -not -path "./vendor/*" -exec cp --parents {} "$BACKUP_DIR/" \;

    log_success "备份已创建: $BACKUP_DIR"
}

restore_backup() {
    if [ ! -d "$BACKUP_DIR" ]; then
        log_error "备份目录不存在: $BACKUP_DIR"
        return
    fi

    log_info "恢复备份..."

    if [ "$DRY_RUN" = true ]; then
        log_warn "预览模式: 跳过恢复"
        return
    fi

    cp -r "$BACKUP_DIR"/* .
    rm -rf "$BACKUP_DIR"

    log_success "备份已恢复"
}

# ─── 迁移规则 ─────────────────────────────────────────────────

apply_migrations() {
    log_info "应用迁移规则..."

    local file_count=0
    local modified_count=0

    # 查找所有 Go 文件
    while IFS= read -r -d '' file; do
        file_count=$((file_count + 1))
        log_verbose "处理文件: $file"

        modified=false

        # 规则 1: 替换 import 路径
        if grep -q "github.com/astra-go/astra/config/nacos" "$file"; then
            log_verbose "  - 替换 config/nacos import"
            if [ "$DRY_RUN" = false ]; then
                sed -i.bak 's|"github.com/astra-go/astra/config/nacos"|"github.com/astra-go/astra/config"|g' "$file"
            fi
            modified=true
        fi

        if grep -q "github.com/astra-go/astra/config/etcd" "$file"; then
            log_verbose "  - 替换 config/etcd import"
            if [ "$DRY_RUN" = false ]; then
                sed -i.bak 's|"github.com/astra-go/astra/config/etcd"|"github.com/astra-go/astra/config"|g' "$file"
            fi
            modified=true
        fi

        if grep -q "github.com/astra-go/astra/config/apollo" "$file"; then
            log_verbose "  - 替换 config/apollo import"
            if [ "$DRY_RUN" = false ]; then
                sed -i.bak 's|"github.com/astra-go/astra/config/apollo"|"github.com/astra-go/astra/config"|g' "$file"
            fi
            modified=true
        fi

        # 规则 2: 替换构造函数
        if grep -q "nacos.NewClient" "$file"; then
            log_verbose "  - 替换 nacos.NewClient"
            if [ "$DRY_RUN" = false ]; then
                sed -i.bak 's|nacos\.NewClient|config.NewNacosClient|g' "$file"
            fi
            modified=true
        fi

        if grep -q "etcd.NewSource" "$file"; then
            log_verbose "  - 替换 etcd.NewSource"
            if [ "$DRY_RUN" = false ]; then
                sed -i.bak 's|etcd\.NewSource|config.NewEtcdClient|g' "$file"
            fi
            modified=true
        fi

        if grep -q "apollo.New" "$file"; then
            log_verbose "  - 替换 apollo.New"
            if [ "$DRY_RUN" = false ]; then
                sed -i.bak 's|apollo\.New|config.NewApolloClient|g' "$file"
            fi
            modified=true
        fi

        # 规则 3: 替换选项结构体
        if grep -q "nacos.Options" "$file"; then
            log_verbose "  - 替换 nacos.Options"
            if [ "$DRY_RUN" = false ]; then
                sed -i.bak 's|nacos\.Options|config.NacosOptions|g' "$file"
            fi
            modified=true
        fi

        if grep -q "etcd.Options" "$file"; then
            log_verbose "  - 替换 etcd.Options"
            if [ "$DRY_RUN" = false ]; then
                sed -i.bak 's|etcd\.Options|config.EtcdOptions|g' "$file"
            fi
            modified=true
        fi

        if grep -q "apollo.Options" "$file"; then
            log_verbose "  - 替换 apollo.Options"
            if [ "$DRY_RUN" = false ]; then
                sed -i.bak 's|apollo\.Options|config.ApolloOptions|g' "$file"
            fi
            modified=true
        fi

        # 清理 .bak 文件
        if [ "$DRY_RUN" = false ] && [ -f "${file}.bak" ]; then
            rm "${file}.bak"
        fi

        if [ "$modified" = true ]; then
            modified_count=$((modified_count + 1))
            log_success "已修改: $file"
        fi
    done < <(find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" -print0)

    log_info "处理完成: $file_count 个文件, $modified_count 个文件已修改"
}

# ─── 验证 ─────────────────────────────────────────────────────

verify_migration() {
    log_info "验证迁移结果..."

    local error_count=0

    # 检查是否有残留的旧 import
    if grep -r "github.com/astra-go/astra/config/nacos" --include="*.go" . 2>/dev/null | grep -v "vendor/"; then
        log_error "发现残留的 config/nacos import"
        error_count=$((error_count + 1))
    fi

    if grep -r "github.com/astra-go/astra/config/etcd" --include="*.go" . 2>/dev/null | grep -v "vendor/"; then
        log_error "发现残留的 config/etcd import"
        error_count=$((error_count + 1))
    fi

    if grep -r "github.com/astra-go/astra/config/apollo" --include="*.go" . 2>/dev/null | grep -v "vendor/"; then
        log_error "发现残留的 config/apollo import"
        error_count=$((error_count + 1))
    fi

    # 尝试编译
    log_info "尝试编译..."
    if [ "$DRY_RUN" = false ]; then
        if ! go build ./... 2>/dev/null; then
            log_warn "编译失败，请手动检查"
            error_count=$((error_count + 1))
        else
            log_success "编译通过"
        fi
    fi

    if [ $error_count -eq 0 ]; then
        log_success "验证通过"
    else
        log_warn "验证失败: $error_count 个错误"
    fi
}

# ─── 生成报告 ─────────────────────────────────────────────────

generate_report() {
    log_info "生成迁移报告..."

    local report_file="migration-report-$(date +%Y%m%d-%H%M%S).md"

    cat > "$report_file" <<EOF
# Config 模块迁移报告

生成时间: $(date)

## 迁移概况

- 模式: $([ "$DRY_RUN" = true ] && echo "预览模式" || echo "执行模式")
- 备份目录: $BACKUP_DIR

## 修改的文件

EOF

    # 列出修改的文件
    if [ -d "$BACKUP_DIR" ]; then
        find "$BACKUP_DIR" -name "*.go" | while read -r backup_file; do
            relative_path="${backup_file#$BACKUP_DIR/}"
            if ! diff -q "$backup_file" "$relative_path" > /dev/null 2>&1; then
                echo "- $relative_path" >> "$report_file"
            fi
        done
    fi

    cat >> "$report_file" <<EOF

## 后续步骤

1. 检查修改是否正确
2. 运行测试: \`go test ./...\`
3. 更新文档
4. 提交更改: \`git add -A && git commit -m "refactor: migrate to unified config package"\`

EOF

    log_success "报告已生成: $report_file"
}

# ─── 主流程 ─────────────────────────────────────────────────

main() {
    echo "╔══════════════════════════════════════════════════════════╗"
    echo "║         Config 模块迁移脚本 v1.0                        ║"
    echo "╚══════════════════════════════════════════════════════════╝"
    echo ""

    # 前置检查
    check_prerequisites

    # 创建备份
    create_backup

    # 应用迁移
    apply_migrations

    # 验证
    verify_migration

    # 生成报告
    generate_report

    # 提示
    if [ "$DRY_RUN" = false ]; then
        echo ""
        log_info "迁移完成！请检查修改并按照报告中的步骤操作。"
        log_info "如需回滚: rm -rf . && cp -r $BACKUP_DIR/* ."
    else
        echo ""
        log_info "预览完成！使用不带 -n 参数的命令执行实际迁移。"
    fi
}

# ─── 入口 ─────────────────────────────────────────────────────

main
