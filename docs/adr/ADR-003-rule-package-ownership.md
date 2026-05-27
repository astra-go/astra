# ADR-003: Rule Package Ownership

## Status
Accepted — implemented 2026-05-27

## Date
2026-05-26

## Context
The `rule/` directory is part of the core module, importing `github.com/expr-lang/expr` as a direct dependency. This forces ALL users of `github.com/astra-go/astra` to download `expr-lang/expr` (~2MB) even though:
- The rule engine is only used by the `alert/` sub-module
- Most users never use `alert/`
- `expr-lang/expr` is a complex dependency with its own ecosystem

Current dependency chain:
```
core (github.com/astra-go/astra)
  └── rule/ (github.com/astra-go/astra/rule) [no independent go.mod]
        └── expr-lang/expr v1.17.8

alert/ (github.com/astra-go/astra/alert) [independent sub-module]
  └── ??? (should depend on rule/, but rule/ is part of core)
```

## Decision
**Extract `rule/` into an independent sub-module with its own `go.mod`.**

### New Dependency Chain
```
core (github.com/astra-go/astra)
  └── (no dependency on rule/)

alert/ (github.com/astra-go/astra/alert)
  └── github.com/astra-go/astra/rule v0.x.x
        └── github.com/expr-lang/expr v1.17.8
```

### Migration Steps
1. Create `rule/go.mod` with `module github.com/astra-go/astra/rule`
2. Add `require github.com/expr-lang/expr v1.17.8` to `rule/go.mod`
3. Run `go mod tidy` in `rule/` directory
4. Add `./rule` to `go.work` use block
5. Add `replace github.com/astra-go/astra/rule v0.0.0 => ./rule` to `go.work`
6. Update `alert/go.mod` to `require github.com/astra-go/astra/rule v0.0.0`
7. Remove `github.com/expr-lang/expr` from core `go.mod`
8. Update `go.mod tidy` in root directory

### Versioning
- `rule` sub-module uses independent versioning from core
- Initial version: `v0.0.0` (zero-version for workspace development)
- First release: `v0.1.0` when published to registry

## Consequences

### Positive
- Core module users no longer download `expr-lang/expr` by default
- Alert users explicitly choose to install rule support
- Smaller core dependency tree (~2MB savings)
- Rule engine can evolve independently

### Negative
- Users of `alert/` must explicitly `go get github.com/astra-go/astra/rule`
- Breaking change: alert users must update their `go.mod`

### Mitigation
- Document clearly in alert/ README
- Provide migration guide with exact commands

## References
- ADR-001: Core Dependency Boundary
- ADR-002: Module/Plugin Interface Unification
