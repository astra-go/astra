// Package astra — module system
//
// A Module is an independent building block that wires itself into an *App
// through a single Install call.  Modules compose naturally: pass any number
// of them to App.Register and the framework installs them in order.
//
//	app := astra.New()
//	must(app.Register(
//	    health.NewModule(health.WithProbe("db", dbProbe)),
//	    ormModule,       // *orm.Module — DB lifecycle + middleware
//	    cacheModule,     // closes Redis on shutdown
//	    alert.NewModule(engine),
//	    graphql.NewModule(schema),
//	))
//	app.Run(":8080")
//
// # What a module can do inside Install
//
//   - Register routes:      app.GET / POST / PUT / DELETE / PATCH / Any
//   - Register middleware:  app.Use(...)
//   - Mount route groups:   app.Group(prefix).GET(...)
//   - Lifecycle hooks:      app.OnStart(...) / app.OnStop(...)
//   - Install sub-modules:  app.Register(innerModule)
//
// # Writing your own module
//
//	type APIModule struct{ db *gorm.DB }
//
//	func (m *APIModule) Name() string { return "api" }
//
//	func (m *APIModule) Install(app *astra.App) error {
//	    g := app.Group("/api/v1")
//	    g.GET("/users",  m.listUsers)
//	    g.POST("/users", m.createUser)
//	    app.OnStop(func(ctx context.Context) error {
//	        return m.db.WithContext(ctx).Exec("SELECT 1").Error // flush
//	    })
//	    return nil
//	}

package astra

import "fmt"

// Module is the plug-and-play building-block interface for organising your
// application's own business logic into self-contained units.
//
// Use Module when you are structuring the code you own: API layers, domain
// services, feature flags, or any reusable internal component. For integrating
// a third-party library (Prometheus, tracing, Swagger, …) use Plugin instead —
// the two interfaces are symmetric by design, and PluginAsModule bridges them
// when you need to mix both in a single Register call.
//
// A single Install call is the only contract: the module receives the full
// *App reference and is free to register routes, middleware, lifecycle hooks,
// or nested modules. Install is called exactly once, before the server starts.
type Module interface {
	// Name returns a short, unique identifier for this module.
	// It is used in error messages, logs, and duplicate-detection.
	Name() string

	// Install wires the module into app.
	// A non-nil error aborts registration; the module name is prepended to the
	// returned error automatically.
	Install(app *App) error
}

// ModuleFunc is a lightweight adapter that turns a plain function into a
// Module. Use it for one-off inline setup that does not warrant a full type.
//
//	app.Register(astra.NewModuleFunc("cors-setup", func(app *astra.App) error {
//	    app.Use(middleware.CORS("https://app.example.com"))
//	    return nil
//	}))
type ModuleFunc struct {
	name string
	fn   func(*App) error
}

// NewModuleFunc creates a Module from a name and an install function.
func NewModuleFunc(name string, fn func(*App) error) Module {
	return ModuleFunc{name: name, fn: fn}
}

func (m ModuleFunc) Name() string          { return m.name }
func (m ModuleFunc) Install(app *App) error { return m.fn(app) }

// Register installs one or more modules onto the application in order.
//
// Duplicate module names are rejected — each name may be installed at most
// once. If Install returns an error, the module name is prepended and the
// error is returned immediately; subsequent modules in the same call are not
// installed.
//
// Register returns ErrSlimMode when called on an App created by NewSlim().
//
// Register is safe to call concurrently with other route registrations but is
// typically called during application setup before Run.
func (a *App) Register(modules ...Module) error {
	if a.slim {
		return ErrSlimMode
	}
	for _, m := range modules {
		if err := a.registerOne(m); err != nil {
			return err
		}
	}
	return nil
}

// Modules returns a snapshot of all successfully installed modules keyed by
// name. The returned map is a copy — mutating it has no effect on the App.
func (a *App) Modules() map[string]Module {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]Module, len(a.modules))
	for k, v := range a.modules {
		out[k] = v
	}
	return out
}

// HasModule reports whether a module with the given name has been installed.
func (a *App) HasModule(name string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.modules[name]
	return ok
}

// registerOne installs a single module with duplicate detection.
// The name slot is reserved before Install is called so that concurrent
// Register calls for the same name are serialised correctly.
func (a *App) registerOne(m Module) error {
	name := m.Name()

	// Reserve the name slot before calling Install so concurrent calls for
	// the same module name are rejected even while Install is running.
	a.mu.Lock()
	if a.modules == nil {
		a.modules = make(map[string]Module)
	}
	if _, exists := a.modules[name]; exists {
		a.mu.Unlock()
		return fmt.Errorf("astra: module %q already registered", name)
	}
	a.modules[name] = nil // sentinel — slot is reserved
	a.mu.Unlock()

	// Call Install without holding the lock: Install may call app.Use,
	// app.GET, app.OnStart etc., all of which acquire the same lock.
	if err := m.Install(a); err != nil {
		// Roll back the reservation so the caller can retry or use a
		// different module with the same name.
		a.mu.Lock()
		delete(a.modules, name)
		a.mu.Unlock()
		return fmt.Errorf("astra: module %q: %w", name, err)
	}

	a.mu.Lock()
	a.modules[name] = m
	a.mu.Unlock()
	return nil
}
