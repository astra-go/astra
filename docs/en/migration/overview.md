# Migration Guide Overview

This section documents incompatible changes and migration steps between major versions of Astra.

## Guide List

| Upgrade Path | Description | Scope |
|-------------|-------------|-------|
| [v0.x → v1.0](v0-to-v1.md) | First stable release, includes multiple API consolidations | Moderate |
| [v1.x → v2.0](v1-to-v2.md) | Next major version (planned) | TBD |

---

## Migration Principles

1. **Upgrade incrementally**: when crossing multiple major versions, upgrade one at a time (0.x → 1.0 → 2.0) — do not skip versions.
2. **Read the CHANGELOG first**: before each upgrade, read [CHANGELOG](../changelog.md) and review all `### Changed` and `### Removed` entries.
3. **Test coverage**: ensure sufficient unit and integration tests before upgrading; use `go vet ./...` and `go test ./...` to quickly catch compile-time and runtime issues.
4. **Deprecation warnings**: the Go compiler does not directly show deprecation warnings, but `gopls` and `staticcheck` will flag symbols annotated with `// Deprecated:`. Run this before upgrading:
   ```bash
   go install honnef.co/go/tools/cmd/staticcheck@latest
   staticcheck ./...
   ```

---

## Quick Version Check Script

```bash
#!/usr/bin/env bash
# check-astra-version.sh
set -e

CURRENT=$(go list -m -json github.com/astra-go/astra | jq -r .Version)
LATEST=$(go list -m -versions github.com/astra-go/astra | tr ' ' '\n' | tail -1)

echo "Current version: $CURRENT"
echo "Latest version:  $LATEST"

if [ "$CURRENT" != "$LATEST" ]; then
    echo "Please refer to the migration guide to upgrade: https://astra-go.github.io/astra/migration/"
fi
```

---

## Getting Help

- GitHub Issues: [astra-go/astra/issues](https://github.com/astra-go/astra/issues)
- Filter by the `migration` label for migration-related issues
