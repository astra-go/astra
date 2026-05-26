package astra

// Plugin is implemented by third-party packages that integrate an external
// library (Prometheus, tracing, Swagger, OAuth2, etc.) into an App.
//
// Deprecated: Plugin is superseded by Component in v2. Implement Component
// (with Init) instead. Existing Plugin implementations continue to work via
// RegisterPlugin, which wraps them as Components internally.
//
// Migration:
//
//	// Before (v1)
//	func (p *MyPlugin) Init(app *astra.App) error { ... }
//
//	// After (v2) — no change needed; Init is already the Component method name.
//	// Just implement Component directly:
//	var _ astra.Component = (*MyPlugin)(nil)
type Plugin interface {
	// Name returns a unique identifier used in error messages and logs.
	Name() string
	// Init is called once during plugin registration. Register routes,
	// middleware, and lifecycle hooks here.
	Init(app *App) error
}

// PluginAsModule wraps a Plugin so it can be passed to App.Register alongside
// Modules. The adapter forwards Name() and delegates Init to Install, giving
// Plugins the same duplicate-detection and error-wrapping behaviour that
// Modules receive through Register.
//
// Deprecated: Use App.Register with a Component directly. If your Plugin
// already implements Init(*App) error, it satisfies Component with a one-line
// type assertion change.
//
//	app.Register(
//	    astra.PluginAsModule(swagger.New(swagger.Config{})),
//	    myBizModule,
//	)
func PluginAsModule(p Plugin) Module {
	return legacyPluginAsModule{p}
}

// legacyPluginAsModule bridges a v1 Plugin into the v1 Module interface so
// that PluginAsModule keeps working without change.
type legacyPluginAsModule struct{ p Plugin }

func (a legacyPluginAsModule) Name() string        { return a.p.Name() }
func (a legacyPluginAsModule) Install(app *App) error { return a.p.Init(app) }
