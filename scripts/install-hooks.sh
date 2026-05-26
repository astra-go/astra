#!/usr/bin/env bash
# install-hooks.sh — 安装 Git hooks，防止提交未 tidy 的 go.mod/go.sum。
#
# 运行一次即可：
#   bash scripts/install-hooks.sh
#
# Hook 行为：
#   pre-commit  — 检测本次 staged 变动涉及的模块，对每个模块运行
#                 `go mod tidy`；若 go.mod / go.sum 因此产生新的 diff，
#                 打印提示并中止提交（需要开发者 git add 后再次提交）。
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
HOOK="$ROOT/.git/hooks/pre-commit"

cat > "$HOOK" << 'HOOK_SCRIPT'
#!/usr/bin/env bash
# pre-commit: run go mod tidy on changed modules and fail if it produces a diff.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)

# ── 找出本次 staged 改动涉及的模块 ──────────────────────────────────────────
# ALL_MODULES lists sub-modules that may need tidy on commit.
# The root module (.) is intentionally excluded: in workspace mode,
# tidy on the root triggers network resolution of all workspace members
# and is not needed for intra-module changes.
ALL_MODULES=(
    otel mq taskqueue storage discovery config cache lock search notify mongodb lua
    orm grpc session auth
    runner client testutil
)

# dirty_modules: space-separated list of modules that need tidy
dirty_modules=""

has_module() {
    local needle="$1"
    local m
    for m in $dirty_modules; do
        [ "$m" = "$needle" ] && return 0
    done
    return 1
}

staged=$(git diff --cached --name-only 2>/dev/null || true)
[ -z "$staged" ] && exit 0

for f in $staged; do
    best="."
    for mod in "${ALL_MODULES[@]}"; do
        [ "$mod" = "." ] && continue
        if [[ "$f" == "$mod/"* ]] && [ ${#mod} -gt ${#best} ]; then
            best="$mod"
        fi
    done
    has_module "$best" || dirty_modules="$dirty_modules $best"
done

# ── 按拓扑顺序 tidy ──────────────────────────────────────────────────────────
FAILED=()
for mod in "${ALL_MODULES[@]}"; do
    has_module "$mod" || continue
    dir="$ROOT"; [ "$mod" != "." ] && dir="$ROOT/$mod"
    echo "▶  go mod tidy — $mod"
    (cd "$dir" && go mod tidy)
    # 检查 tidy 是否产生了新 diff
    if ! git diff --quiet -- "$dir/go.mod" "$dir/go.sum" 2>/dev/null; then
        FAILED+=("$mod")
    fi
done

if [ ${#FAILED[@]} -gt 0 ]; then
    echo ""
    echo "✗ go mod tidy produced changes in: ${FAILED[*]}"
    echo "  Please 'git add' the updated go.mod/go.sum files and retry."
    exit 1
fi
echo "✓ go mod tidy — all clean"

# ── check intra-workspace replace directives ─────────────────────────────────
echo "▶  check-replaces"
if ! bash "$ROOT/scripts/check-intra-replaces.sh"; then
    echo ""
    echo "✗ intra-workspace replace directives are out of sync."
    echo "  Run 'make sync-replaces', then 'git add' the updated go.mod files and retry."
    exit 1
fi
echo "✓ check-replaces — all clean"
HOOK_SCRIPT

chmod +x "$HOOK"
echo "✓ pre-commit hook installed: $HOOK"
echo ""
echo "To uninstall: rm $HOOK"
echo "To skip once:  git commit --no-verify"
