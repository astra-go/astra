# ADR-002: Module and Plugin Interface Unification

## Status
Proposed (Target: v2)

## Date
2026-05-26

## Context
Astra currently defines two functionally identical interfaces:

```go
// Module: business module with Install method
type Module interface {
    Name() string
    Install(app *App) error
}

// Plugin: third-party adapter with Init method  
type Plugin interface {
    Name() string
    Init(app *App) error
}
```

`PluginAsModule` exists as a bridge between them, proving the distinction is artificial. Users face a confusing choice when writing extensions: "Am I writing a Module or a Plugin?" This distinction becomes meaningless as projects evolve.

## Decision
**Merge Module and Plugin into a single `Component` interface in v2.**

```go
// Deprecated: Use Component instead
type Module interface {
    Name() string
    Install(app *App) error
}

// Deprecated: Use Component instead
type Plugin interface {
    Name() string
    Init(app *App) error
}

// New unified interface
type Component interface {
    Name() string
    Init(app *App) error  // Install was renamed to Init for consistency
}
```

### Migration Path
1. Introduce `Component` as the new recommended interface (v2)
2. Mark `Module.Install` and `Plugin.Init` as deprecated with clear migration instructions
3. `PluginAsModule` remains as a backward-compatible bridge
4. `App.Register()` accepts `Component` interface
5. Maintain binary compatibility: old `Module` and `Plugin` implementations still work via `PluginAsModule` internally

### Timeline
- v1.x: `Component` introduced as alias to `Module`, `Plugin` deprecated
- v2.0: `Module.Install` removed, `Plugin` removed, only `Component` remains
- Provide `migrate-module-to-component` tool in `astractl`

## Consequences

### Positive
- Simpler mental model: one concept, one interface
- Reduced confusion for extension authors
- Cleaner codebase without bridge patterns

### Negative
- Breaking change in v2: all `Module` and `Plugin` implementations must migrate
- Estimated migration effort: 15-30 minutes per extension library
- `astractl migrate-module-to-component` tool needs to be implemented

### Migration Example
Before:
```go
type MyModule struct{}
func (m *MyModule) Name() string { return "mymodule" }
func (m *MyModule) Install(app *astra.App) error { ... }
```

After:
```go
type MyModule struct{}
func (m *MyModule) Name() string { return "mymodule" }
func (m *MyModule) Init(app *astra.App) error { ... }  // Install → Init
```

## References
- ADR-001: Core Dependency Boundary
- ADR-003: Rule Package Ownership
