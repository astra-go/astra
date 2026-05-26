#!/usr/bin/env bash
# check-dep-versions.sh — detect version splits: same dependency at different
# versions across workspace modules.
#
# Exit 0  = all external deps use a single version across the workspace.
# Exit 1  = at least one dependency has conflicting versions.
#
# Usage:  bash scripts/check-dep-versions.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Collect all (dep, version, module) triples, then detect conflicts with awk.
find "$ROOT" -name "go.mod" -not -path "*/.git/*" -print0 | sort -z \
  | while IFS= read -r -d '' gomod; do
      mod_dir="${gomod#"$ROOT/"}"
      mod_dir="${mod_dir%/go.mod}"
      go mod edit -json "$gomod" 2>/dev/null \
        | jq -r --arg mod "$mod_dir" \
          '.Require[]? | select(.Version | test("^v0\\.0\\.0-000101") | not) | [$mod, .Path, .Version] | @tsv'
    done \
  | awk -F'\t' '
    {
      mod=$1; dep=$2; ver=$3
      if (!(dep in first_ver)) {
        first_ver[dep] = ver
        first_mod[dep] = mod
      } else if (first_ver[dep] != ver) {
        if (!printed[dep]++) {
          printf "✗ %s\n", dep
          printf "    %s  (%s)\n", first_ver[dep], first_mod[dep]
        }
        printf "    %s  (%s)\n", ver, mod
        fail = 1
      }
    }
    END {
      if (!fail) print "✓ All external dependencies use consistent versions across the workspace"
      exit fail ? 1 : 0
    }
  '
