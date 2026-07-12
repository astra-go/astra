// Package astra — legacy Module system (v1 compatibility)
//
// Module is the v1 building-block interface. It is superseded by Component in
// v2. Existing Module implementations continue to work: pass them to
// App.Register via ModuleAsComponent, or use the RegisterModule helper.
//
// Migration guide:
//
//	// Before (v1)
//	type APIModule struct{ db *gorm.DB }
//	func (m *APIModule) Name() string { return "api" }
//	func (m *APIModule) Install(app *astra.App) error { ... }
//
//	// After (v2)
//	type APIComponent struct{ db *gorm.DB }
//	func (c *APIComponent) Name() string { return "api" }
//	func (c *APIComponent) Init(app *astra.App) error { ... }
//
// # v3 Removal Roadmap
//
// Module v1 is scheduled for removal in v3.0. The deprecation timeline:
//
//   - v2.x (current): Module functional; deprecated calls log slog.Warn
//   - v2.5: go.mod toolchain gate — new projects see deprecation notice
//   - v3.0: Module, ModuleFunc, ModuleAsComponent, RegisterModule, Modules() removed
//
// To prepare for v3, migrate all Install → Init and implement Component.
// The ModuleAsComponent adapter will be removed; use Component directly.

package astra

import (
	"fmt"
	"log/slog"
)

// Module is the v1 plug-and-play building-block interface.
//
// Deprecated: Use Component instead. Module will be removed in v3.
// Rename Install to Init and change the interface assertion to Component.
// Existing implementations can be passed to App.Register via ModuleAsComponent.
type Module interface {
	// Name returns a short, unique identifier for this module.
	Name() string
	// Install wires the module into app.
	//
	// Deprecated: Implement Component.Init instead.
	Install(app *App) error
}

// ModuleFunc is a lightweight adapter that turns a plain function into a
// Module.
//
// Deprecated: Use NewComponentFunc instead.
type ModuleFunc struct {
	name string
	fn   func(*App) error
}

// NewModuleFunc creates a Module from a name and an install function.
//
// Deprecated: Use NewComponentFunc instead.
func NewModuleFunc(name string, fn func(*App) error) Module {
	slog.Warn("NewModuleFunc is deprecated — migrate to Component + NewComponentFunc", "module", name)
	return ModuleFunc{name: name, fn: fn}
}

func (m ModuleFunc) Name() string          { return m.name }
func (m ModuleFunc) Install(app *App) error {
	slog.Warn("Module.Install is deprecated — rename to Init and implement Component", "module", m.name)
	return m.fn(app)
}

// ModuleAsComponent wraps a v1 Module so it can be passed to App.Register.
//
//	app.Register(astra.ModuleAsComponent(myLegacyModule))
func ModuleAsComponent(m Module) Component {
	return moduleAdapter{m}
}

// Register installs one or more Components onto the application in order.
//
// Duplicate component names are rejected — each name may be installed at most
// once. If Init returns an error, the component name is prepended and the
// error is returned immediately; subsequent components in the same call are not
// installed.
//
// Register returns ErrSlimMode when called on an App created by NewSlim().
//
// Register is safe to call concurrently with other route registrations but is
// typically called during application setup before Run.
func (a *App) Register(components ...Component) error {
	if a.slim {
		return ErrSlimMode
	}
	for _, c := range components {
		if err := a.registerOne(c); err != nil {
			return err
		}
	}
	return nil
}

// RegisterModule installs one or more v1 Modules for backward compatibility.
// Each Module is wrapped via ModuleAsComponent before registration.
//
// Deprecated: Implement Component and use Register directly.
func (a *App) RegisterModule(modules ...Module) error {
	for _, m := range modules {
		slog.Warn("RegisterModule is deprecated — migrate to Component", "module", m.Name())
	}
	components := make([]Component, len(modules))
	for i, m := range modules {
		components[i] = ModuleAsComponent(m)
	}
	return a.Register(components...)
}

// Components returns a snapshot of all successfully installed components keyed
// by name. The returned map is a copy — mutating it has no effect on the App.
// For single-component lookups prefer GetComponent to avoid the full map copy.
func (a *App) Components() map[string]Component {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]Component, len(a.components))
	for k, v := range a.components {
		out[k] = v
	}
	return out
}

// GetComponent returns the component with the given name and a boolean indicating
// whether it was found. This is O(1) and avoids the full map copy returned by
// Components(). Prefer this for routine single-component access.
func (a *App) GetComponent(name string) (Component, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	c, ok := a.components[name]
	return c, ok
}

// Modules returns a snapshot of all successfully installed components keyed by
// name, for backward compatibility with v1 callers.
//
// Deprecated: Use Components() instead.
func (a *App) Modules() map[string]Component {
	slog.Warn("App.Modules is deprecated — use App.Components() instead")
	return a.Components()
}

// HasModule reports whether a component with the given name has been installed.
func (a *App) HasModule(name string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.components[name]
	return ok
}

// HasComponent reports whether a component with the given name has been installed.
func (a *App) HasComponent(name string) bool {
	return a.HasModule(name)
}

// registerOne installs a single component with duplicate detection.
func (a *App) registerOne(c Component) error {
	name := c.Name()

	a.mu.Lock()
	if a.components == nil {
		a.components = make(map[string]Component)
	}
	if _, exists := a.components[name]; exists {
		a.mu.Unlock()
		return fmt.Errorf("astra: component %q already registered", name)
	}
	a.components[name] = nil // sentinel — slot is reserved
	a.mu.Unlock()

	if err := c.Init(a); err != nil {
		a.mu.Lock()
		delete(a.components, name)
		a.mu.Unlock()
		return fmt.Errorf("astra: component %q: %w", name, err)
	}

	a.mu.Lock()
	a.components[name] = c
	a.mu.Unlock()
	return nil
}
