# ADR-002: Module and Plugin Interface Unification

## Status
Accepted — Implemented in v2 (2026-05-26)

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
**Merge Module and Plugin into a single `Component` interface.**

```go
// New unified interface (component.go)
type Component interface {
    Name() string
    Init(app *App) error
}

// Deprecated: Use Component instead
type Module interface {
    Name() string
    Install(app *App) error  // Deprecated: implement Component.Init instead
}

// Deprecated: Use Component instead
type Plugin interface {
    Name() string
    Init(app *App) error
}
```

### What changed

| Symbol | Before | After |
|--------|--------|-------|
| `Component` | did not exist | new unified interface |
| `ComponentFunc` / `NewComponentFunc` | did not exist | new functional adapter |
| `App.Register` | accepts `...Module` | accepts `...Component` |
| `App.Components()` | did not exist | returns `map[string]Component` |
| `App.HasComponent()` | did not exist | new method |
| `App.RegisterModule()` | did not exist | backward-compat shim for `...Module` |
| `ModuleAsComponent()` | did not exist | wraps v1 Module as Component |
| `App.Modules()` | returns `map[string]Module` | returns `map[string]Component` (deprecated alias for Components) |
| `App.HasModule()` | existed | deprecated alias for HasComponent |
| `Module` / `ModuleFunc` / `NewModuleFunc` | primary API | deprecated, kept for compat |
| `Plugin` / `PluginAsModule` | primary API | deprecated, kept for compat |
| `App.RegisterPlugin()` | wraps via PluginAsModule | wraps via pluginAdapter directly |
| `observability.Module.Install` | primary method | renamed to `Init` |
| `swagger.New()` | returns `astra.Plugin` | returns `astra.Component` |

### Migration Path

**Minimal migration (zero code change):** existing `Module` implementations continue to work via `RegisterModule` or `ModuleAsComponent`. Existing `Plugin` implementations continue to work via `RegisterPlugin`.

**Full migration (recommended):**

```go
// Before (v1)
type MyModule struct{}
func (m *MyModule) Name() string { return "mymodule" }
func (m *MyModule) Install(app *astra.App) error { ... }

app.Register(astra.PluginAsModule(&MyPlugin{}), &MyModule{})
```

```go
// After (v2)
type MyComponent struct{}
func (c *MyComponent) Name() string { return "mycomponent" }
func (c *MyComponent) Init(app *astra.App) error { ... }

app.Register(&MyPlugin{}, &MyComponent{})  // Plugin.Init already matches Component.Init
```

For `Plugin` implementors: since `Plugin.Init` and `Component.Init` have identical signatures, the only change needed is updating the interface assertion comment.

## Consequences

### Positive
- Simpler mental model: one concept, one interface
- Reduced confusion for extension authors
- `PluginAsModule` bridge pattern eliminated from the hot path
- `swagger.New()` now returns `Component` directly — no wrapping needed

### Negative
- `App.Register` signature changed from `...Module` to `...Component` — callers passing `Module` values directly must use `ModuleAsComponent()` or `RegisterModule()`
- `App.Modules()` return type changed from `map[string]Module` to `map[string]Component`

### Not changed (backward compat preserved)
- `Module`, `Plugin`, `ModuleFunc`, `NewModuleFunc`, `PluginAsModule` — all kept, marked deprecated
- `App.RegisterPlugin()` — kept, marked deprecated
- `App.HasModule()`, `App.Modules()` — kept as deprecated aliases

## References
- ADR-001: Core Dependency Boundary
- ADR-003: Rule Package Ownership
