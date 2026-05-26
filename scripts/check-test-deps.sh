#!/usr/bin/env bash
# check-test-deps.sh — detect test-only packages incorrectly declared as
# direct (production) dependencies in go.mod.
#
# A "test-only" dep is one that appears as a direct require (not // indirect)
# but is imported exclusively by *_test.go files within that module.
#
# Detection uses `go list -json` for precise import analysis, avoiding the
# false positives that grep-based prefix matching can produce.
#
# Packages annotated with "// test-only" in go.mod are explicitly acknowledged
# as intentional test-only direct deps and are skipped by this check.
#
# Exit 0  = no violations found.
# Exit 1  = at least one violation found.
#
# Usage:
#   bash scripts/check-test-deps.sh           # default: heavy packages only
#   bash scripts/check-test-deps.sh --strict  # also flag lightweight test helpers
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

STRICT=false
for arg in "$@"; do
    case "$arg" in
        --strict) STRICT=true ;;
    esac
done

# ── Exempt modules ────────────────────────────────────────────────────────────
# These modules are test/benchmark/example modules by design; all their direct
# deps are intentional even when only used in _test.go files.
EXEMPT_MODULES=(
    "benchmarks"
    "e2e"
    "e2e/orm"
    "e2e/search"
    "examples/crud"
    "examples/orm"
    "examples/showcase"
    "examples/wasm"
)

# ── Exempt packages ───────────────────────────────────────────────────────────
# These packages are intentionally test-only direct deps.
# - testutil: internal test helper designed to be imported only from _test.go
# - otel/sdk/metric: used in middleware tests for ManualReader-backed MeterProvider
#   (in-process metric collection for test assertions); not needed in production builds
# - SQLite drivers: CGo-free in-memory drivers used as test databases; they
#   register via database/sql side-effects and cannot be marked // indirect
#   without breaking `go mod tidy` (tidy re-promotes them to direct).
EXEMPT_PKGS=(
    "github.com/astra-go/astra/testutil"
    "go.opentelemetry.io/otel/sdk/metric"
    "github.com/glebarez/sqlite"
    "modernc.org/sqlite"
    "github.com/mattn/go-sqlite3"
)

# ── Heavy test-infrastructure packages (always an error when test-only) ───────
# Matched as exact module path prefix (not substring) to avoid false positives.
HEAVY_PATTERNS=(
    "github.com/testcontainers/"
    "github.com/testcontainers/testcontainers-go"
    "github.com/stretchr/testify"
    "github.com/golang/mock"
    "go.uber.org/mock"
    "github.com/vektra/mockery"
    "github.com/DATA-DOG/go-sqlmock"
    "github.com/jarcoal/httpmock"
    "github.com/h2non/gock"
    "github.com/onsi/ginkgo"
    "github.com/onsi/gomega"
)

# ── Lightweight test helpers (only flagged in --strict mode) ──────────────────
LIGHT_PATTERNS=(
    "github.com/glebarez/sqlite"
    "modernc.org/sqlite"
    "github.com/mattn/go-sqlite3"
)

# ─────────────────────────────────────────────────────────────────────────────

_is_exempt_module() {
    local mod="$1"
    for e in "${EXEMPT_MODULES[@]}"; do
        [ "$mod" = "$e" ] && return 0
    done
    return 1
}

_is_exempt_pkg() {
    local pkg="$1"
    for e in "${EXEMPT_PKGS[@]}"; do
        [ "$pkg" = "$e" ] && return 0
    done
    return 1
}

# Returns 0 if the package is annotated with "// test-only" in the given go.mod.
# This is the explicit opt-in mechanism: maintainers annotate intentional
# test-only direct deps so the checker skips them without a global exemption.
_is_annotated_test_only() {
    local pkg="$1" gomod="$2"
    grep -qE "^\s+${pkg//./\\.}\s+[^ ]+ // test-only" "$gomod" 2>/dev/null
}

# Returns "heavy", "light", or "unknown" on stdout.
# Matches against exact module path prefix to avoid false positives from
# substring matching (e.g. "foo/bar" matching "foo/bar/baz").
_classify_pkg() {
    local pkg="$1"
    for p in "${HEAVY_PATTERNS[@]}"; do
        # Match exact path or path prefix (pkg == p or pkg starts with p/)
        if [ "$pkg" = "$p" ] || [[ "$pkg" == "${p}/"* ]] || [[ "$pkg" == "${p%/}"* ]]; then
            echo "heavy" && return
        fi
    done
    for p in "${LIGHT_PATTERNS[@]}"; do
        if [ "$pkg" = "$p" ] || [[ "$pkg" == "${p}/"* ]]; then
            echo "light" && return
        fi
    done
    echo "unknown"
}

# Collect production and test import paths for a module directory using
# `go list -json`, which is precise and handles build tags correctly.
# Outputs two newline-separated lists to stdout, separated by "---".
# $1 = module directory
_get_imports() {
    local dir="$1"
    # Run go list in the module directory with workspace disabled so that
    # each sub-module is analysed in isolation (workspace mode would merge
    # import graphs across modules).
    (
        cd "$dir"
        GOWORK=off go list -json ./... 2>/dev/null \
            | python3 -c "
import json, sys

data = sys.stdin.read()
prod = set()
test = set()

dec = json.JSONDecoder()
pos = 0
while pos < len(data):
    try:
        obj, pos = dec.raw_decode(data, pos)
    except json.JSONDecodeError:
        pos += 1
        continue
    prod.update(obj.get('Imports', []))
    test.update(obj.get('TestImports', []))
    test.update(obj.get('XTestImports', []))

print('\n'.join(sorted(prod)))
print('---')
print('\n'.join(sorted(test)))
" 2>/dev/null || true
    )
}

# ─────────────────────────────────────────────────────────────────────────────

fail=0
warn_count=0
error_count=0

# Enumerate all workspace modules from go.work
while IFS= read -r mod; do
    # Skip exempt modules
    _is_exempt_module "$mod" && continue

    mod_dir="$ROOT"
    [ "$mod" != "." ] && mod_dir="$ROOT/$mod"
    gomod="$mod_dir/go.mod"
    [ -f "$gomod" ] || continue

    # Collect production and test imports via go list
    imports_output=$(_get_imports "$mod_dir")
    prod_imports=$(echo "$imports_output" | awk '/^---$/{exit} {print}')
    test_imports=$(echo "$imports_output" | awk 'found{print} /^---$/{found=1}')

    # Parse direct (non-indirect) requires via go mod edit -json
    while IFS=$'\t' read -r pkg _version; do
        [ -z "$pkg" ] && continue

        # Skip intra-workspace pseudo-versions
        [[ "$_version" == "v0.0.0-00010101000000-000000000000" ]] && continue

        # Skip exempt packages (global list)
        _is_exempt_pkg "$pkg" && continue

        # Skip packages explicitly annotated "// test-only" in this go.mod
        _is_annotated_test_only "$pkg" "$gomod" && continue

        # Check if this package (or any sub-package) is used in production code.
        # A direct dep may expose multiple importable sub-packages; we check
        # whether any of them appear in the production import set.
        prod_match=$(echo "$prod_imports" | grep -E "^${pkg}(/|$)" || true)
        [ -n "$prod_match" ] && continue

        # Check if it's used at all (in tests)
        test_match=$(echo "$test_imports" | grep -E "^${pkg}(/|$)" || true)
        [ -z "$test_match" ] && continue

        # Classify severity
        severity=$(_classify_pkg "$pkg")

        if [ "$severity" = "heavy" ]; then
            echo "✗ [$mod] $pkg"
            echo "      declared as direct dep but only imported in _test.go files"
            echo "      fix: move to a dedicated e2e sub-module, or mark as '// indirect'"
            error_count=$(( error_count + 1 ))
            fail=1
        elif [ "$severity" = "light" ] && [ "$STRICT" = true ]; then
            echo "⚠ [$mod] $pkg"
            echo "      declared as direct dep but only imported in _test.go files"
            echo "      consider: mark as '// indirect' or move to a test sub-module"
            warn_count=$(( warn_count + 1 ))
            fail=1
        elif [ "$severity" = "unknown" ] && [ "$STRICT" = true ]; then
            echo "⚠ [$mod] $pkg"
            echo "      declared as direct dep but only imported in _test.go files"
            warn_count=$(( warn_count + 1 ))
            fail=1
        fi
    done < <(
        go mod edit -json "$gomod" 2>/dev/null \
            | jq -r '.Require[]? | select(.Indirect != true) | [.Path, .Version] | @tsv'
    )
done < <(bash "$SCRIPT_DIR/list-modules.sh")

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
if [ $fail -eq 0 ]; then
    echo "✓ All modules pass dependency health check"
else
    total=$(( error_count + warn_count ))
    echo "Found $total issue(s): $error_count error(s), $warn_count warning(s)"
    echo ""
    echo "How to fix:"
    echo "  Option A — annotate as intentional in go.mod (preferred for SDK test helpers):"
    echo "    require github.com/some/pkg v1.2.3 // test-only"
    echo "  Option B — isolate into a dedicated e2e sub-module (preferred for heavy deps like testcontainers):"
    echo "    mkdir -p e2e/<name> && cd e2e/<name> && go mod init github.com/astra-go/astra/e2e/<name>"
    echo "    move the *_test.go files there, add the dep to that module's go.mod"
    echo "  Option C — if the package is truly needed at build time, verify it's not test-only"
    echo "  Option D — add the module to EXEMPT_MODULES in scripts/check-test-deps.sh if intentional"
fi

exit $fail
