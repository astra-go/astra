#!/usr/bin/env bash
# check-intra-replaces.sh — CI guard: fail if any go.mod is missing intra-workspace replace directives.
#
# Inlines the same logic as sync-intra-replaces.sh but operates on a scratch
# copy of the go.mod files, then diffs the replace blocks against the originals.
# Path comparison is done by resolving both sides to root-relative canonical paths.
#
# Usage:
#   bash scripts/check-intra-replaces.sh          # exits 1 on drift
#   bash scripts/check-intra-replaces.sh --fix    # auto-fix by running sync
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

# ─── Build scratch tree ───────────────────────────────────────────────────────
SCRATCH=$(mktemp -d)
trap "rm -rf $SCRATCH" EXIT

cp "$ROOT/go.work" "$SCRATCH/go.work"
while IFS= read -r gomod; do
    rel="${gomod#$ROOT/}"
    scratch_gomod="$SCRATCH/$rel"
    mkdir -p "$(dirname "$scratch_gomod")"
    cp "$gomod" "$scratch_gomod"
done < <(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*" | sort)

# ─── Build module-path → workspace-dir table (from go.work) ──────────────────
MOD_TABLE=$(mktemp)
trap "rm -f $MOD_TABLE; rm -rf $SCRATCH" EXIT

awk '/^use[[:space:]]*\(/{p=1;next} /^\)/{p=0} p{print}' "$SCRATCH/go.work" \
    | sed 's|^[[:space:]]*||;s|^./||' \
    | while IFS= read -r mod_dir; do
        [ -z "$mod_dir" ] && continue
        local_dir="$SCRATCH/$mod_dir"
        [ "$mod_dir" = "." ] && local_dir="$SCRATCH"
        mod_path=$(grep "^module " "$local_dir/go.mod" 2>/dev/null | awk '{print $2}' || echo "")
        [ -z "$mod_path" ] && continue
        [[ "$mod_path" == example/* ]] && continue
        if [ "$mod_dir" = "." ]; then
            echo "$mod_path ."
        else
            echo "$mod_path $mod_dir"
        fi
    done > "$MOD_TABLE"

# ─── Apply sync logic to scratch tree ────────────────────────────────────────
while IFS= read -r gomod; do
    dir="$(dirname "$gomod")"
    own_module=$(grep "^module " "$gomod" | awk '{print $2}')

    if [ "$dir" = "$SCRATCH" ]; then
        prefix=""
    else
        rel_to_root="${dir#$SCRATCH/}"
        depth=$(awk -F'/' '{print NF}' <<< "$rel_to_root")
        prefix=""
        for _ in $(seq 1 "$depth"); do prefix="${prefix}../"; done
    fi

    while IFS= read -r line; do
        dep_path=$(echo "$line" | awk '{print $1}')
        dep_local_dir=$(echo "$line" | awk '{print $2}')
        [ "$dep_path" = "$own_module" ] && continue
        if [ "$dep_local_dir" = "." ]; then
            rel_path="${prefix%/}"
            [ -z "$rel_path" ] && rel_path="."
        else
            rel_path="${prefix}${dep_local_dir}"
        fi
        (cd "$dir" && go mod edit -replace="${dep_path}@${ZERO}=${rel_path}" 2>/dev/null) || true
    done < "$MOD_TABLE"
done < <(find "$SCRATCH" -name "go.mod" | sort)

# ─── Compare replace blocks (resolve paths to root-relative canonical form) ───
# Resolves "=> ../foo" to "foo", "=> .." to ".", etc., relative to the repo root.
extract_canonical_replaces() {
    local gomod="$1" base_dir="$2"
    local gomod_dir
    gomod_dir="$(dirname "$gomod")"
    grep -E "^(\t|replace )(github\.com/astra-go/astra[^ ]* ${ZERO} =>)" "$gomod" \
        | while IFS= read -r line; do
            rel_path="${line##*=> }"
            abs_path="$(cd "$gomod_dir" && cd "$rel_path" 2>/dev/null && pwd)" || abs_path="$rel_path"
            root_rel="${abs_path#$base_dir}"
            root_rel="${root_rel#/}"
            [ -z "$root_rel" ] && root_rel="."
            mod_part="${line%% =>*}"
            # strip leading tab or "replace " prefix for uniform comparison
            mod_part="${mod_part#	}"
            mod_part="${mod_part#replace }"
            printf '%s => %s\n' "$mod_part" "$root_rel"
        done \
        | sort || true
}

drift_files=()
while IFS= read -r gomod; do
    rel="${gomod#$ROOT/}"
    scratch_gomod="$SCRATCH/$rel"
    [ -f "$scratch_gomod" ] || continue

    actual=$(extract_canonical_replaces "$gomod" "$ROOT")
    expected=$(extract_canonical_replaces "$scratch_gomod" "$SCRATCH")

    if [ "$actual" != "$expected" ]; then
        drift_files+=("$rel")
        if [ "${VERBOSE:-}" = "1" ]; then
            echo "=== $rel ==="
            diff <(echo "$actual") <(echo "$expected") || true
            echo ""
        fi
    fi
done < <(find "$ROOT" -name "go.mod" -not -path "$ROOT/.git/*" | sort)

# ─── Report ───────────────────────────────────────────────────────────────────
if [ ${#drift_files[@]} -eq 0 ]; then
    echo "✓ all go.mod intra-workspace replace directives are in sync"
    exit 0
fi

echo "✗ intra-workspace replace drift detected in ${#drift_files[@]} file(s):"
for f in "${drift_files[@]}"; do
    echo "  $f"
done
echo ""
echo "Fix: bash scripts/sync-intra-replaces.sh"

if [ "$FIX" = true ]; then
    echo ""
    echo "Running sync-intra-replaces.sh ..."
    bash "$SCRIPT_DIR/sync-intra-replaces.sh"
    echo "✓ fixed"
    exit 0
fi

exit 1
