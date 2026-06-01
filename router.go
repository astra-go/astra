package astra

import (
	"log/slog"

	"github.com/astra-go/astra/router"
)

// HttpRouter is the interface that App depends on for route registration and
// request dispatching.  The default implementation is the built-in radix-tree
// Router.  Replace it via astra.WithRouter for testing or to plug in an
// alternative routing algorithm.
type HttpRouter interface {
	// Add registers a handler chain for the given HTTP method and path pattern.
	Add(method, path string, handlers HandlersChain)
	// Handle dispatches an incoming request by populating the context's handler
	// chain and path parameters, then calling c.Next().
	Handle(c *Ctx)
	// Routes returns a snapshot of all registered routes for introspection.
	Routes() []RouteInfo
}

// RouteInfo contains information about a registered route.
type RouteInfo struct {
	Method   string
	Path     string
	FullPath string
}

// Router is the Astra HTTP router backed by method-keyed radix tries.
// This is a thin adapter around the router package implementation.
type Router struct {
	impl *router.Router
}

func newRouter(app *App) *Router {
	impl := router.NewRouter(
		wrapHandlerFunc(app.options.NotFoundHandler),
		wrapHandlerFunc(app.options.MethodNotAllowedHandler),
		slog.Default(),
		app.options.StrictConflict || app.options.Mode == ModeTest,
		app.options.MaxParamValueLen,
	)
	return &Router{impl: impl}
}

// wrapHandlerFunc converts an astra.HandlerFunc to router.HandlerFunc.
func wrapHandlerFunc(h HandlerFunc) router.HandlerFunc {
	return func(c router.Ctx) error {
		return h(c.(*Ctx))
	}
}

// wrapHandlersChain converts an astra.HandlersChain to router.HandlersChain.
func wrapHandlersChain(handlers HandlersChain) router.HandlersChain {
	wrapped := make(router.HandlersChain, len(handlers))
	for i, h := range handlers {
		wrapped[i] = wrapHandlerFunc(h)
	}
	return wrapped
}

// Add registers a new route in the radix tree.
func (r *Router) Add(method, path string, handlers HandlersChain) {
	r.impl.Add(method, path, wrapHandlersChain(handlers))
}

// Handle dispatches an HTTP request to the matching handler chain.
func (r *Router) Handle(c *Ctx) {
	r.impl.Handle(c)
}

// Routes returns all registered routes for introspection.
func (r *Router) Routes() []RouteInfo {
	routes := r.impl.Routes()
	result := make([]RouteInfo, len(routes))
	for i, route := range routes {
		result[i] = RouteInfo{
			Method:   route.Method,
			Path:     route.Path,
			FullPath: route.FullPath,
		}
	}
	return result
}

// maxParamDepth returns the maximum number of path parameters across all
// registered routes in all method trees.
func (r *Router) maxParamDepth() int {
	return r.impl.MaxParamDepth()
}
