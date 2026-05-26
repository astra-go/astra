package astra

// Plugin is implemented by third-party packages that integrate an external
// library (Prometheus, tracing, Swagger, OAuth2, etc.) into an App.
//
// Use Plugin when you are writing a reusable, library-facing adapter that
// other applications will pull in as a dependency. For organising your own
// application's business logic into units, use Module instead — the two
// interfaces are intentionally symmetric so that either can be registered
// with the same App.Register call via PluginAsModule.
//
// Drop-in integration example:
//
//	type PrometheusPlugin struct { ... }
//	func (p *PrometheusPlugin) Name() string { return "prometheus" }
//	func (p *PrometheusPlugin) Init(app *astra.App) error {
//	    app.GET("/metrics", middleware.MetricsHandler())
//	    app.OnStop(p.shutdown)
//	    return nil
//	}
//
//	// Option A — dedicated helper (duplicate-safe, preferred):
//	app.RegisterPlugin(&PrometheusPlugin{})
//
//	// Option B — wrap as Module and use the unified Register path:
//	app.Register(astra.PluginAsModule(&PrometheusPlugin{}))
//
// Decision guide:
//   - Third-party library adapter   → Plugin  (Init — "initialise the library")
//   - Application business unit     → Module  (Install — "install into the app")
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
//	app.Register(
//	    astra.PluginAsModule(swagger.New(swagger.Config{})),
//	    myBusinessModule,
//	)
func PluginAsModule(p Plugin) Module {
	return pluginAdapter{p}
}

type pluginAdapter struct{ p Plugin }

func (a pluginAdapter) Name() string        { return a.p.Name() }
func (a pluginAdapter) Install(app *App) error { return a.p.Init(app) }
