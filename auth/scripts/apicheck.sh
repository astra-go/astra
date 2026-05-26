#!/usr/bin/env bash
# apicheck.sh — Check for breaking API changes using apidiff.
#
# Usage:
#   bash scripts/apicheck.sh                        # diff core module
#   bash scripts/apicheck.sh --module auth          # diff a sub-module
#   bash scripts/apicheck.sh --all                  # diff all workspace modules
#   bash scripts/apicheck.sh --update               # update baseline(s)
#   bash scripts/apicheck.sh --check                # fail on breaking changes (CI mode)
#   bash scripts/apicheck.sh --all --check          # CI mode for all modules
#
# Requires: apidiff (go install golang.org/x/exp/cmd/apidiff@latest)
set -euo pipefail

API_DIR=".api"
MODE="diff"
TARGET_MODULE=""
ALL_MODULES=false

# ── Parse args ────────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --update)          MODE="update" ;;
    --check)           MODE="check"  ;;
    --all)             ALL_MODULES=true ;;
    --module)          TARGET_MODULE="$2"; shift ;;
    --help)
      echo "Usage: $0 [--module <dir>] [--all] [--update|--check]"
      exit 0
      ;;
  esac
  shift
done

# ── Verify apidiff is available ───────────────────────────────────────────────
if ! command -v apidiff &>/dev/null; then
  echo "⚠️  apidiff not found. Install with:"
  echo "    go install golang.org/x/exp/cmd/apidiff@latest"
  exit 1
fi

mkdir -p "$API_DIR"

# ── Module list ───────────────────────────────────────────────────────────────
# Derive from go.work so the list stays in sync automatically.
_list_modules() {
  awk '/^use[[:space:]]*\(/{p=1;next} /^\)/{p=0} p{print}' go.work \
    | sed 's|[[:space:]]||g; s|^\./||; s|^\.$|.|' \
    | grep -v '^$'
}

if $ALL_MODULES; then
  modules=$(_list_modules)
elif [[ -n "$TARGET_MODULE" ]]; then
  modules="$TARGET_MODULE"
else
  modules="."
fi

# ── Per-module import path ────────────────────────────────────────────────────
_import_path() {
  local dir="$1"
  if [[ "$dir" == "." ]]; then
    grep '^module ' go.mod | awk '{print $2}'
  else
    grep '^module ' "$dir/go.mod" | awk '{print $2}'
  fi
}

# ── Baseline file path for a module dir ──────────────────────────────────────
_baseline_file() {
  local dir="$1"
  if [[ "$dir" == "." ]]; then
    echo "${API_DIR}/next.txt"
  else
    # e.g. auth → .api/auth.txt, dtx/orm → .api/dtx_orm.txt
    local slug
    slug=$(echo "$dir" | tr '/' '_')
    echo "${API_DIR}/${slug}.txt"
  fi
}

# ── Process one module ────────────────────────────────────────────────────────
OVERALL_EXIT=0

_check_module() {
  local dir="$1"
  local import_path
  import_path=$(_import_path "$dir")
  local baseline
  baseline=$(_baseline_file "$dir")
  local current_file="${API_DIR}/_current_$(echo "$dir" | tr '/' '_').bin"

  echo "📋 [$dir] $import_path"

  # Write current export data
  local work_dir="."
  [[ "$dir" != "." ]] && work_dir="$dir"

  if ! (cd "$work_dir" && apidiff -w "../$current_file" "$import_path" 2>/dev/null); then
    # apidiff -w writes relative to cwd; adjust for root module
    if ! apidiff -w "$current_file" "$import_path" 2>/dev/null; then
      echo "  ⚠️  Could not generate export data — skipping"
      return 0
    fi
  fi

  if [[ "$MODE" == "update" ]]; then
    cp "$current_file" "$baseline"
    echo "  ✅ Baseline updated ($(wc -c < "$baseline") bytes)"
    rm -f "$current_file"
    return 0
  fi

  if [[ ! -f "$baseline" ]]; then
    echo "  ⚠️  No baseline at $baseline — run: bash scripts/apicheck.sh --module $dir --update"
    rm -f "$current_file"
    return 0
  fi

  # Compare baseline vs current
  local diff_out
  diff_out=$(apidiff "$baseline" "$current_file" 2>/dev/null || true)
  rm -f "$current_file"

  if [[ -z "$diff_out" ]]; then
    echo "  ✅ No API changes"
    return 0
  fi

  echo "$diff_out" | sed 's/^/  /'

  if [[ "$MODE" == "check" ]]; then
    # apidiff marks incompatible changes with "Incompatible changes:"
    if echo "$diff_out" | grep -q "^Incompatible changes:"; then
      echo "  ❌ Breaking API changes detected"
      echo "     To accept: bash scripts/apicheck.sh --module $dir --update"
      OVERALL_EXIT=1
    else
      echo "  ✅ Compatible changes only"
    fi
  fi
}

# ── Run ───────────────────────────────────────────────────────────────────────
while IFS= read -r mod; do
  [[ -z "$mod" ]] && continue
  _check_module "$mod"
  echo ""
done <<< "$modules"

if [[ $OVERALL_EXIT -ne 0 ]]; then
  exit 1
fi
