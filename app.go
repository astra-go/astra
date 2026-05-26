package astra

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// App is the core of the Astra framework.
// It manages routes, middleware, lifecycle hooks, and the HTTP server.
type App struct {
	router     HttpRouter
	options    *Options
	middleware HandlersChain
	pool       sync.Pool
	lifecycle  *Lifecycle
	components map[string]Component
	mu         sync.RWMutex
	slim       bool // true when created by NewSlim(); disables lifecycle/plugin/module subsystems

	// pool telemetry — updated atomically
	poolHit    atomic.Int64
	poolMiss   atomic.Int64
	poolActive atomic.Int64
}

// PoolStat holds a snapshot of the Ctx pool counters.
type PoolStat struct {
	Hit    int64 // number of Get calls that returned a pooled Ctx
	Miss   int64 // number of Get calls that allocated a new Ctx
	Active int64 // number of Ctx objects currently in use
}

// PoolStats returns a snapshot of the Ctx pool counters.
func (a *App) PoolStats() PoolStat {
	return PoolStat{
		Hit:    a.poolHit.Load(),
		Miss:   a.poolMiss.Load(),
		Active: a.poolActive.Load(),
	}
}

// Router returns the underlying HttpRouter.
func (a *App) Router() HttpRouter { return a.router }

// New creates a new Astra application with the given options.
func New(opts ...Option) *App {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	// Validate options — panic early so misconfiguration is caught at startup.
	if options.ShutdownTimeout < 0 {
		panic("astra: WithShutdownTimeout: value must be >= 0")
	}
	if options.MaxJSONBodySize < 0 {
		panic("astra: WithMaxJSONBodySize: value must be >= 0")
	}
	if options.MaxMultipartMemory < 0 {
		panic("astra: WithMaxMultipartMemory: value must be >= 0")
	}
	// Compile TrustedProxies strings into net.IPNet once so that per-request
	// ClientIP lookups never call net.ParseCIDR / net.ParseIP again.
	options.prepareTrustedNets()

	app := &App{
		options:   options,
		lifecycle: &Lifecycle{},
	}
	if options.customRouter != nil {
		app.router = options.customRouter
	} else {
		app.router = newRouter(app)
	}

	app.pool.New = func() any {
		return app.allocateContext()
	}

	return app
}

// NewSlim creates a minimal Astra application suitable for serverless functions
// and lightweight microservices.
//
// Compared with New(), a slim App:
//   - Does not allocate a Lifecycle (OnStart / OnStop return ErrSlimMode).
//   - Does not initialise the Module or Plugin registries (Register /
//     RegisterPlugin return ErrSlimMode).
//   - Sets Binder to nil, so c.Bind / c.ShouldBind are unavailable; use
//     c.BodyParser or manual JSON decoding instead.
//
// Everything else — routing, middleware, ServeHTTP, graceful shutdown — works
// identically to New(). Opts are applied on top of slimDefaultOptions, so you
// can still customise the serialiser, error handler, timeouts, etc.
func NewSlim(opts ...Option) *App {
	options := slimDefaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	options.prepareTrustedNets()

	app := &App{
		options: options,
		slim:    true,
		// lifecycle intentionally nil
	}
	if options.customRouter != nil {
		app.router = options.customRouter
	} else {
		app.router = newRouter(app)
	}

	app.pool.New = func() any {
		return app.allocateContext()
	}

	return app
}

func (a *App) allocateContext() *Ctx {
	c := &Ctx{
		app: a,
		// keys (overflow map) and smallKeys are lazily initialised — allocating
		// them here would waste memory for requests that never call c.Set.
	}
	// Pre-wire the params slice to the embedded backing array.
	// reset() restores this on every request.
	c.params = c.paramsArr[:0]
	// Pre-wire the ResponseWriter interface to the embedded responseWriter value.
	// Because c is already on the heap (sync.Pool), &c.rw is an interior heap
	// pointer — no additional allocation occurs here or in reset().
	c.writer = &c.rw
	return c
}

// sealPool is called once before the server starts listening.  It scans the
// registered route tree to find the deepest param path and, if it exceeds the
// inline paramsArr capacity, pre-allocates an overflow slice in the Pool.New
// closure so that every subsequent Get() returns a context ready to hold all
// parameters without a mid-request heap allocation.
//
// It also pre-warms the pool by placing one Ctx per logical CPU, so that
// the first wave of requests hits warm per-P slots instead of calling New().
func (a *App) sealPool() {
	r, ok := a.router.(*Router)
	if !ok {
		return // custom router — cannot introspect
	}
	depth := r.maxParamDepth()
	if depth > maxRouteParams {
		// Re-wire Pool.New to produce contexts whose overflowParams backing array
		// already has the required capacity.  Existing pooled objects are not
		// affected (they will be returned and their overflowParams left nil until
		// they are GC'd), but the branch in reset() handles both cases correctly.
		a.pool.New = func() any {
			c := a.allocateContext()
			c.overflowParams = make(Params, 0, depth)
			c.params = c.overflowParams
			return c
		}
	}

	// Pre-warm: place one Ctx per logical CPU so each P's local slot is
	// populated before the first request arrives.  Without this, the first
	// GOMAXPROCS concurrent requests all call pool.New, causing a burst of
	// allocations and elevated GC pressure at cold-start.
	n := runtime.GOMAXPROCS(0)
	warmCtxs := make([]*Ctx, n)
	for i := range warmCtxs {
		warmCtxs[i] = a.pool.New().(*Ctx)
	}
	for _, c := range warmCtxs {
		a.pool.Put(c)
	}
}

// Use registers global middleware that is applied to all routes.
//
// IMPORTANT: Use must be called before any route registration (GET, POST,
// PUT, etc.). Middleware registered after a route is added will NOT apply
// to that route, because handlers are baked into the route tree at
// registration time.
//
//	app := astra.New()
//	app.Use(Logger(), Recovery())  // register middleware first
//	app.GET("/users", listUsers)   // then register routes
//
// Safe to call concurrently with other Use calls, but not concurrently
// with route registrations.
func (a *App) Use(middleware ...MiddlewareFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.middleware = append(a.middleware, middleware...)
}

// GET registers a route for GET requests.
func (a *App) GET(path string, handlers ...HandlerFunc) {
	a.handle(MethodGET, path, handlers)
}

// POST registers a route for POST requests.
func (a *App) POST(path string, handlers ...HandlerFunc) {
	a.handle(MethodPOST, path, handlers)
}

// RouteRegistrar is the minimal route-registration interface used by sub-packages
// (health, pprof, etc.) that need to register routes without importing *App.
// *App satisfies this interface.
type RouteRegistrar interface {
	GET(path string, handlers ...HandlerFunc)
	POST(path string, handlers ...HandlerFunc)
}

// PUT registers a route for PUT requests.
func (a *App) PUT(path string, handlers ...HandlerFunc) {
	a.handle(MethodPUT, path, handlers)
}

// DELETE registers a route for DELETE requests.
func (a *App) DELETE(path string, handlers ...HandlerFunc) {
	a.handle(MethodDELETE, path, handlers)
}

// PATCH registers a route for PATCH requests.
func (a *App) PATCH(path string, handlers ...HandlerFunc) {
	a.handle(MethodPATCH, path, handlers)
}

// HEAD registers a route for HEAD requests.
func (a *App) HEAD(path string, handlers ...HandlerFunc) {
	a.handle(MethodHEAD, path, handlers)
}

// OPTIONS registers a route for OPTIONS requests.
func (a *App) OPTIONS(path string, handlers ...HandlerFunc) {
	a.handle(MethodOPTIONS, path, handlers)
}

// Any registers a route for all HTTP methods.
func (a *App) Any(path string, handlers ...HandlerFunc) {
	methods := []string{
		MethodGET, MethodPOST, MethodPUT, MethodDELETE,
		MethodPATCH, MethodHEAD, MethodOPTIONS,
	}
	for _, m := range methods {
		a.handle(m, path, handlers)
	}
}

// Group creates a route group with a common prefix and optional middleware.
func (a *App) Group(prefix string, middleware ...MiddlewareFunc) *Group {
	return newGroup(a, prefix, middleware...)
}

// Static serves static files from the given filesystem root.
func (a *App) Static(prefix, root string) {
	fs := http.FileServer(http.Dir(root))
	handler := func(c *Ctx) error {
		http.StripPrefix(prefix, fs).ServeHTTP(c.Writer(), c.Request())
		return nil
	}
	a.GET(prefix+"/*filepath", handler)
}

func (a *App) handle(method, path string, handlers HandlersChain) {
	if path == "" {
		return
	}
	a.mu.RLock()
	allHandlers := make(HandlersChain, len(a.middleware)+len(handlers))
	copy(allHandlers, a.middleware)
	copy(allHandlers[len(a.middleware):], handlers)
	a.mu.RUnlock()
	a.router.Add(method, path, allHandlers)
}

// ServeHTTP implements http.Handler, making App compatible with the standard library.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw := a.pool.Get()
	c := raw.(*Ctx)
	if c.pooled {
		a.poolHit.Add(1)
	} else {
		a.poolMiss.Add(1)
		c.pooled = true
	}
	a.poolActive.Add(1)
	c.reset(w, r)

	a.router.Handle(c)

	a.poolActive.Add(-1)
	a.pool.Put(c)
}

// newDefaultServer builds an http.Server with Astra's production-safe timeouts.
func newDefaultServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// Run starts the HTTP server on the given address.
// It also listens for OS signals (SIGINT, SIGTERM) for graceful shutdown.
func (a *App) Run(addr string) error {
	return a.RunServer(newDefaultServer(addr, a))
}

// RunTLS starts the HTTPS server.
func (a *App) RunTLS(addr, certFile, keyFile string) error {
	server := newDefaultServer(addr, a)
	return a.runWithGracefulShutdown(server, func() error {
		return server.ListenAndServeTLS(certFile, keyFile)
	})
}

// RunServer starts a custom http.Server with graceful shutdown.
func (a *App) RunServer(server *http.Server) error {
	return a.runWithGracefulShutdown(server, server.ListenAndServe)
}

func (a *App) runWithGracefulShutdown(server *http.Server, listenFn func() error) error {
	// Scan the route tree once to size the params pool for deep-param routes.
	// Must run after all routes are registered, before the first request arrives.
	a.sealPool()

	if a.lifecycle != nil {
		if err := a.lifecycle.RunStartHooks(context.Background()); err != nil {
			return err
		}
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := listenFn(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		signal.Stop(quit)
		return err
	case sig := <-quit:
		_ = sig // captured signal
	}
	signal.Stop(quit)

	// Graceful shutdown
	timeout := time.Duration(a.options.ShutdownTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if a.lifecycle != nil {
		a.lifecycle.RunStopHooks(ctx)
	}

	return server.Shutdown(ctx)
}

// OnStart registers a hook that is called before the server starts.
// Returns ErrSlimMode when called on an App created by NewSlim().
func (a *App) OnStart(fn func(context.Context) error) error {
	if a.slim {
		return ErrSlimMode
	}
	a.lifecycle.OnStart(fn)
	return nil
}

// OnStop registers a hook that is called during graceful shutdown.
// Returns ErrSlimMode when called on an App created by NewSlim().
func (a *App) OnStop(fn func(context.Context) error) error {
	if a.slim {
		return ErrSlimMode
	}
	a.lifecycle.OnStop(fn)
	return nil
}

// RegisterPlugin registers one or more v1 Plugins in order. Each plugin is
// wrapped as a Component and installed through Register, giving it the same
// duplicate-detection and error-wrapping behaviour as any Component.
// Returns the first error encountered.
// Returns ErrSlimMode when called on an App created by NewSlim().
//
//	app.RegisterPlugin(
//	    &prometheus.Plugin{},
//	    &tracing.Plugin{Endpoint: "localhost:4317"},
//	)
//
// Deprecated: Implement Component and use Register directly. Plugin.Init
// already matches the Component.Init signature — only the interface assertion
// needs to change.
func (a *App) RegisterPlugin(plugins ...Plugin) error {
	components := make([]Component, len(plugins))
	for i, p := range plugins {
		components[i] = pluginAdapter{p}
	}
	return a.Register(components...)
}
