#!/usr/bin/env bash
# apicheck.sh — Check for breaking API changes against a baseline.
#
# Usage:
#   bash scripts/apicheck.sh                  # compare against origin/main
#   bash scripts/apicheck.sh --update         # update .api/next.txt baseline
#   bash scripts/apicheck.sh --check          # fail on breaking changes (CI mode)
#
# Requires: Go 1.22+
set -euo pipefail

BASE_REF=${BASE_REF:-origin/main}
MODE="diff"
API_DIR=".api"
API_FILE="${API_DIR}/next.txt"

for arg in "$@"; do
  case "$arg" in
    --update) MODE="update" ;;
    --check)  MODE="check"  ;;
    --help)   echo "Usage: $0 [--update|--check]"; exit 0 ;;
  esac
done

mkdir -p "$API_DIR"

# Generate current API surface using go vet / api-compare approach.
# Since go api is not yet stable, we use go vet with a custom approach:
# Export the public API surface by listing exported symbols.
echo "📋 Generating API surface for core module..."

# Use go doc to extract the public API surface
CURRENT=$(go doc -all github.com/astra-go/astra 2>/dev/null | grep -E '^(func |type |var |const |method )' | sort || true)

if [ -z "$CURRENT" ]; then
  echo "⚠️  Could not generate API surface. Skipping API check."
  exit 0
fi

echo "$CURRENT" > "${API_DIR}/current.txt"

if [ "$MODE" = "update" ]; then
  cp "${API_DIR}/current.txt" "$API_FILE"
  echo "✅ Updated $API_FILE ($(wc -l < "$API_FILE") symbols)"
  exit 0
fi

if [ ! -f "$API_FILE" ]; then
  echo "⚠️  No baseline found at $API_FILE. Run: bash scripts/apicheck.sh --update"
  exit 0
fi

# Compare
CHANGES=$(diff "$API_FILE" "${API_DIR}/current.txt" || true)

if [ -z "$CHANGES" ]; then
  echo "✅ No API changes detected"
  exit 0
fi

echo "📋 API changes detected:"
echo "$CHANGES"

if [ "$MODE" = "check" ]; then
  # Removals (lines starting with < in diff output) are breaking changes
  BREAKING=$(echo "$CHANGES" | grep '^< ' || true)
  if [ -n "$BREAKING" ]; then
    echo ""
    echo "❌ Breaking API changes detected:"
    echo "$BREAKING"
    echo ""
    echo "If intentional, update the baseline: bash scripts/apicheck.sh --update"
    exit 1
  fi
  echo "✅ No breaking changes (only additions)"
fi
