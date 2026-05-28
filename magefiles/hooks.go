//go:build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const hookScript = `#!/usr/bin/env bash
# pre-commit: run go mod tidy on changed modules and fail if it produces a diff.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)

ALL_MODULES=(
    otel mq taskqueue storage discovery config cache lock search notify mongodb lua
    orm grpc session auth
    runner client testutil
)

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

FAILED=()
for mod in "${ALL_MODULES[@]}"; do
    has_module "$mod" || continue
    dir="$ROOT"; [ "$mod" != "." ] && dir="$ROOT/$mod"
    echo "▶  go mod tidy — $mod"
    (cd "$dir" && go mod tidy)
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

echo "▶  check-replaces"
if ! bash "$ROOT/scripts/check-intra-replaces.sh"; then
    echo ""
    echo "✗ intra-workspace replace directives are out of sync."
    echo "  Run 'make sync-replaces', then 'git add' the updated go.mod files and retry."
    exit 1
fi
echo "✓ check-replaces — all clean"

# ── gofmt 格式检查 ────────────────────────────────────────────────────────────
echo "▶  gofmt check"
GO_FILES=$(git diff --cached --name-only 2>/dev/null | grep '\.go$' || true)
if [ -n "$GO_FILES" ]; then
    UNFORMATTED=$(gofmt -l $GO_FILES 2>/dev/null || true)
    if [ -n "$UNFORMATTED" ]; then
        echo ""
        echo "✗ unformatted files (run: gofmt -w <file>):"
        echo "$UNFORMATTED"
        exit 1
    fi
fi
echo "✓ gofmt — all clean"

# ── go vet ────────────────────────────────────────────────────────────────────
echo "▶  go vet"
for mod in $dirty_modules; do
    dir="$ROOT"; [ "$mod" != "." ] && dir="$ROOT/$mod"
    (cd "$dir" && go vet ./...) || { echo "✗ go vet failed in $mod"; exit 1; }
done
echo "✓ go vet — all clean"
`

// InstallHooks installs the pre-commit git hook.
func InstallHooks() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
		return fmt.Errorf("write hook: %w", err)
	}
	fmt.Printf("✓ pre-commit hook installed: %s\n", hookPath)
	fmt.Println()
	fmt.Printf("To uninstall: rm %s\n", hookPath)
	fmt.Println("To skip once:  git commit --no-verify")
	return nil
}
