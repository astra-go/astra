#!/usr/bin/env bash
# check-intra-replaces.sh — CI guard: fail if any go.mod is missing required
# intra-workspace replace directives (or has incorrect ones).
#
# New logic: only require replace directives for astra modules that are actually
# listed in the go.mod's require block (direct or indirect). Extra replaces are
# allowed (they don't hurt, just add noise). Missing required replaces break the
# build and must be fixed.
#
# This replaces the old "full-sync" check which required every go.mod to have
# replace entries for ALL other workspace modules (regardless of actual deps).
#
# Usage:
#   bash scripts/check-intra-replaces.sh          # exits 1 on drift
#   bash scripts/check-intra-replaces.sh --fix    # auto-fix by adding missing replaces
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ZERO="v0.0.0-00010101000000-000000000000"

FIX=false
for arg in "$@"; do
    case "$arg" in
        --fix) FIX=true ;;
    esac
done

# ─── Build module-path → workspace-dir table (from go.work) ──────────────────
MOD_TABLE=$(mktemp)
trap "rm -f $MOD_TABLE" EXIT

awk '/^use[[:space:]]*\(/{p=1;next} /^\)/{p=0} p{print}' "$ROOT/go.work" \
    | sed 's|^[[:space:]]*||;s|^./||' \
    | grep -v '^$' \
    | while IFS= read -r mod_dir; do
    [ -z "$mod_dir" ] && continue
    local_dir="$ROOT/$mod_dir"
    [ "$mod_dir" = "." ] && local_dir="$ROOT"
    mod_path=$(grep "^module " "$local_dir/go.mod" 2>/dev/null | awk '{print $2}' || echo "")
    [ -z "$mod_path" ] && continue
    [[ "$mod_path" == github.com/astra-go/astra/example/* ]] && continue
    if [ "$mod_dir" = "." ]; then
        echo "$mod_path ."
    else
        echo "$mod_path $mod_dir"
    fi
done > "$MOD_TABLE"

# ─── Helper: get workspace-local path for a module ────────────────────────────
gomod_to_local_path() {
    local gomod="$1" dep_path="$2"
    local gomod_dir dep_local_dir dep_abs_dir rel_path
    # Strip ROOT prefix to get relative path (gomod may be absolute or relative)
    local rel_gomod="${gomod#$ROOT/}"
    # Compute absolute path of the go.mod directory
    if [ "$(dirname "$rel_gomod")" = "." ]; then
        gomod_dir="$ROOT"
    else
        gomod_dir="$ROOT/$(dirname "$rel_gomod")"
    fi
    # Look up the workspace-local dir for this dep
    dep_local_dir=$(grep "^${dep_path} " "$MOD_TABLE" 2>/dev/null | awk '{print $2}')
    [ -z "$dep_local_dir" ] && return 1
    # Compute absolute path of the dependency
    if [ "$dep_local_dir" = "." ]; then
        dep_abs_dir="$ROOT"
    else
        dep_abs_dir="$ROOT/$dep_local_dir"
    fi
    # Compute relative path from go.mod dir to dep dir
    rel_path=$(python3 -c "import os; print(os.path.relpath('$dep_abs_dir', '$gomod_dir'))" 2>/dev/null)
    [ -z "$rel_path" ] && return 1
    echo "$rel_path"
}

# ─── Helper: extract required astra modules from a go.mod ────────────────────
# Handles both single-line and multi-line require(...) blocks.
# Returns module paths WITHOUT versions.
extract_required_astra() {
    local gomod="$1"
    # Multi-line require (...): lines that START with astra-go/astra (skip the require ( line)
    awk '/^require[[:space:]]*\(/,/^\)/' "$gomod" \
        | grep '^[[:space:]]*github\.com/astra-go/astra' \
        | awk '{print $1}' || true
}

# ─── Helper: extract existing replace directives (astra modules only) ──────────
# Parses go.mod to find replace directives for astra modules.
# Two formats supported:
#   replace github.com/astra-go/astra v0.0.0-... => ..
#   replace (
#       github.com/astra-go/astra => ..
#       github.com/astra-go/astra v0.0.0-... => ..
#   )
# Returns lines like: "github.com/astra-go/astra => .."
extract_replaces() {
    local gomod="$1"
    local in_replace_block=false
    local line
    while IFS= read -r line; do
        # Track replace block boundaries
        if echo "$line" | grep -qE '^[[:space:]]*replace[[:space:]]*\('; then
            in_replace_block=true
            continue
        fi
        if echo "$line" | grep -qE '^[[:space:]]*\)'; then
            in_replace_block=false
            continue
        fi

        # Skip non-astra lines
        if ! echo "$line" | grep -q 'github\.com/astra-go/astra'; then
            continue
        fi

        # Only process lines inside a replace block, or standalone replace lines
        if ! $in_replace_block; then
            # Check if this is a standalone replace line (starts with "replace" keyword)
            if ! echo "$line" | grep -qE '^[[:space:]]*replace[[:space:]]'; then
                continue
            fi
        fi

        # Extract module path: first github.com/astra-go/astra token on the line
        # Must be at start of line (possibly indented) for replace block entries,
        # or right after "replace " for standalone lines.
        local mod
        mod=$(echo "$line" | grep -oE 'github\.com/astra-go/astra[^[:space:]]*' | head -1)
        [ -z "$mod" ] && continue

        # Extract local path: everything after "=> "
        local path_part
        path_part=$(echo "$line" | sed 's/.*[[:space:]]=>[[:space:]]*//')
        [ -z "$path_part" ] && continue

        printf '%s => %s\n' "$mod" "$path_part"
    done < "$gomod" | sort -u
}

# ─── Check each go.mod ────────────────────────────────────────────────────────
error_count=0
while IFS= read -r gomod; do
    [ ! -f "$gomod" ] && continue

    # Skip example modules
    if grep -q "^module github.com/astra-go/astra/example/" "$gomod" 2>/dev/null; then
        continue
    fi

    own_module=$(grep "^module " "$gomod" | awk '{print $2}')
    [ -z "$own_module" ] && continue

    # Get required astra modules (strip versions)
    required=$(extract_required_astra "$gomod" | sort -u)
    [ -z "$required" ] && continue

    # Get existing replaces
    existing=$(extract_replaces "$gomod")

    # For each required module, check it has a replace
    missing_count=0
    wrong_count=0

    while IFS= read -r dep_path; do
        [ -z "$dep_path" ] && continue
        [ "$dep_path" = "$own_module" ] && continue

        # Strip version from dep_path (require block may have version)
        dep_base=$(echo "$dep_path" | awk '{print $1}')

        # Check if there's a replace for this dep
        replace_line=$(echo "$existing" | grep "^${dep_base} => " || true)

        if [ -z "$replace_line" ]; then
            # No replace found
            missing_count=$((missing_count + 1))
            if [ "$FIX" = true ]; then
                local_path=$(gomod_to_local_path "$gomod" "$dep_base")
                if [ -n "$local_path" ]; then
                    # go mod edit requires ./ prefix for root-relative paths (no ../ or leading /)
                    case "$local_path" in
                        ../* | /*) edit_path="$local_path" ;;
                        *) edit_path="./$local_path" ;;
                    esac
                    if (cd "$(dirname "$gomod")" && go mod edit -replace="${dep_base}@${ZERO}=${edit_path}" 2>/dev/null); then
                        echo "  ✓ added: $dep_base => $local_path"
                    fi
                fi
            fi
        else
            # Replace found — verify path
            expected_path=$(gomod_to_local_path "$gomod" "$dep_base")
            # Strip leading ./ from actual path for comparison (go mod edit adds ./ but gomod_to_local_path does not)
            actual_path=$(echo "$replace_line" | sed 's/.*=> //' | sed 's|^\./||')
            if [ "$expected_path" != "$actual_path" ]; then
                wrong_count=$((wrong_count + 1))
            fi
        fi
    done <<< "$required"

    # Report issues
    if [ "$missing_count" -gt 0 ] || [ "$wrong_count" -gt 0 ]; then
        rel="${gomod#$ROOT/}"
        echo "✗ $rel (missing: $missing_count, wrong: $wrong_count)"
        error_count=$((error_count + missing_count + wrong_count))
    fi

done < <(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*" | sort)

# ─── Report ───────────────────────────────────────────────────────────────────
if [ "$error_count" -eq 0 ]; then
    echo "✓ all go.mod intra-workspace replace directives are in sync (only required ones)"
    exit 0
fi

echo ""
echo "✗ $error_count issue(s) found"

if [ "$FIX" = true ]; then
    echo ""
    echo "Re-checking after fixes ..."
    if bash "$SCRIPT_DIR/check-intra-replaces.sh" > /dev/null 2>&1; then
        echo "✓ all fixed — check passed"
        exit 0
    else
        echo "✗ some issues remain"
        exit 1
    fi
fi

echo ""
echo "Fix: bash scripts/check-intra-replaces.sh --fix"
exit 1
