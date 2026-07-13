package astra

import (
	"fmt"
)

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

// HasComponent reports whether a component with the given name has been installed.
func (a *App) HasComponent(name string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.components[name]
	return ok
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
