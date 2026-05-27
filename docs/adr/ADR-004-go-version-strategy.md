# ADR-004: Go Version Strategy

## Status
Accepted

## Date
2026-05-26 (updated 2026-05-27)

## Context
The Astra framework targets modern Go versions. We need a clear strategy for Go version support to ensure compatibility, predictability for users, and manageable upgrade paths for both the core framework and its 30+ sub-modules.

## Decision

### 1. Minimum Supported Go Version
- **Core module**: Go 1.25.1 minimum (downgraded from 1.25.8 on 2026-05-26)
- **All sub-modules**: Must declare the same minimum Go version as the core, or higher
- Sub-modules may declare a **higher** minimum if they require specific Go features unavailable in the core minimum

### 2. Version Bumping Policy
When a new Go minor version is released:
1. Test the current codebase with the new Go version
2. Update all `go.mod` files to the new version via automated tooling
3. Run the full test suite and benchmarks
4. Update this ADR with the new version
5. Announce the change in the CHANGELOG

### 3. Deprecation Policy
- When a Go version reaches EOL (as defined by the Go team), it becomes **deprecated**
- Deprecated versions remain buildable but may have reduced test coverage
- Deprecated versions are removed from CI after 2 subsequent minor releases
- Example: If Go 1.24 reaches EOL when Go 1.27 is released, Go 1.24 is deprecated in Astra

### 4. Sub-Module Version Coordination
- All `go.mod` files in the monorepo are updated **together** in a single PR
- Script: `mage checkGoVersions` validates all sub-modules meet the core minimum version
- CI step enforces version consistency across all workspace modules
- **Exempt modules**: `examples/showcase` and `examples/wasm` are intentionally excluded
  from the workspace (`go.work`) and may target older Go versions for compatibility demos

### 5. Version Display
- `astra version` command reports the **minimum supported Go version**, not the build-time Go version
- Documentation clearly states the minimum Go version requirement

## Consequences

### Positive
- Clear, predictable upgrade path for users
- Easier maintenance with automated tooling
- Consistent versioning across the monorepo
- Clear communication of support lifecycle

### Negative
- Users on older Go versions must upgrade to use new Astra releases
- All sub-modules must keep pace with core version updates

## References
- Go Release Policy: https://go.dev/doc/devel/release
- Go EOL Timeline: https://endoflife.date/go
