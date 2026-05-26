// Package astra — Component system (v2 unified interface)
//
// Component is the single plug-and-play interface that replaces the separate
// Module and Plugin interfaces introduced in v1. Both Module (Install) and
// Plugin (Init) are now deprecated aliases; migrate to Component.
//
//	app := astra.New()
//	must(app.Register(
//	    health.New(health.WithProbe("db", dbProbe)),
//	    ormComponent,
//	    cacheComponent,
//	))
//	app.Run(":8080")
//
// # Writing a Component
//
//	type APIComponent struct{ db *gorm.DB }
//
//	func (c *APIComponent) Name() string { return "api" }
//
//	func (c *APIComponent) Init(app *astra.App) error {
//	    g := app.Group("/api/v1")
//	    g.GET("/users",  c.listUsers)
//	    g.POST("/users", c.createUser)
//	    app.OnStop(func(ctx context.Context) error {
//	        return c.db.WithContext(ctx).Exec("SELECT 1").Error
//	    })
//	    return nil
//	}

package astra

// Component is the unified plug-and-play interface for wiring any building
// block — whether a first-party business unit or a third-party library adapter
// — into an *App through a single Init call.
//
// Component replaces the separate Module (Install) and Plugin (Init)
// interfaces from v1. Both are deprecated; implement Component instead.
//
// A single Init call is the only contract: the component receives the full
// *App reference and is free to register routes, middleware, lifecycle hooks,
// or nested components. Init is called exactly once, before the server starts.
type Component interface {
	// Name returns a short, unique identifier for this component.
	// It is used in error messages, logs, and duplicate-detection.
	Name() string

	// Init wires the component into app.
	// A non-nil error aborts registration; the component name is prepended to
	// the returned error automatically.
	Init(app *App) error
}

// ComponentFunc is a lightweight adapter that turns a plain function into a
// Component. Use it for one-off inline setup that does not warrant a full type.
//
//	app.Register(astra.NewComponentFunc("cors-setup", func(app *astra.App) error {
//	    app.Use(middleware.CORS("https://app.example.com"))
//	    return nil
//	}))
type ComponentFunc struct {
	name string
	fn   func(*App) error
}

// NewComponentFunc creates a Component from a name and an init function.
func NewComponentFunc(name string, fn func(*App) error) Component {
	return ComponentFunc{name: name, fn: fn}
}

func (c ComponentFunc) Name() string        { return c.name }
func (c ComponentFunc) Init(app *App) error { return c.fn(app) }

// moduleAdapter wraps a v1 Module so it satisfies Component.
type moduleAdapter struct{ m Module }

func (a moduleAdapter) Name() string        { return a.m.Name() }
func (a moduleAdapter) Init(app *App) error { return a.m.Install(app) }

// pluginAdapter wraps a v1 Plugin so it satisfies Component.
type pluginAdapter struct{ p Plugin }

func (a pluginAdapter) Name() string        { return a.p.Name() }
func (a pluginAdapter) Init(app *App) error { return a.p.Init(app) }
