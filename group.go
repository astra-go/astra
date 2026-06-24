package astra

// Group represents a collection of routes sharing a common prefix and middleware.
// Inspired by gin's RouterGroup and echo's Group.
type Group struct {
	app        *App
	prefix     string
	middleware HandlersChain
	metadata   map[string]string
	tags       []string
}

func newGroup(app *App, prefix string, middleware ...MiddlewareFunc) *Group {
	return &Group{
		app:        app,
		prefix:     prefix,
		middleware: append(HandlersChain{}, middleware...),
	}
}

// Use adds middleware to this group.
func (g *Group) Use(middleware ...MiddlewareFunc) {
	g.middleware = append(g.middleware, middleware...)
}

// Group creates a nested group under this group, inheriting metadata and tags.
func (g *Group) Group(prefix string, middleware ...MiddlewareFunc) *Group {
	combined := make(HandlersChain, len(g.middleware)+len(middleware))
	copy(combined, g.middleware)
	copy(combined[len(g.middleware):], middleware)
	ng := &Group{
		app:        g.app,
		prefix:     g.prefix + prefix,
		middleware: combined,
		tags:       make([]string, len(g.tags)),
	}
	copy(ng.tags, g.tags)
	if len(g.metadata) > 0 {
		ng.metadata = make(map[string]string, len(g.metadata))
		for k, v := range g.metadata {
			ng.metadata[k] = v
		}
	}
	return ng
}

// GET registers a GET route on the group.
func (g *Group) GET(path string, handlers ...HandlerFunc) {
	g.handle(MethodGET, path, handlers)
}

// POST registers a POST route on the group.
func (g *Group) POST(path string, handlers ...HandlerFunc) {
	g.handle(MethodPOST, path, handlers)
}

// PUT registers a PUT route on the group.
func (g *Group) PUT(path string, handlers ...HandlerFunc) {
	g.handle(MethodPUT, path, handlers)
}

// DELETE registers a DELETE route on the group.
func (g *Group) DELETE(path string, handlers ...HandlerFunc) {
	g.handle(MethodDELETE, path, handlers)
}

// PATCH registers a PATCH route on the group.
func (g *Group) PATCH(path string, handlers ...HandlerFunc) {
	g.handle(MethodPATCH, path, handlers)
}

// HEAD registers a HEAD route on the group.
func (g *Group) HEAD(path string, handlers ...HandlerFunc) {
	g.handle(MethodHEAD, path, handlers)
}

// OPTIONS registers an OPTIONS route on the group.
func (g *Group) OPTIONS(path string, handlers ...HandlerFunc) {
	g.handle(MethodOPTIONS, path, handlers)
}

// Any registers a route for all HTTP methods on the group.
func (g *Group) Any(path string, handlers ...HandlerFunc) {
	methods := []string{
		MethodGET, MethodPOST, MethodPUT, MethodDELETE,
		MethodPATCH, MethodHEAD, MethodOPTIONS,
	}
	for _, m := range methods {
		g.handle(m, path, handlers)
	}
}

func (g *Group) handle(method, path string, handlers HandlersChain) {
	// Combine group middleware with route handlers; global app middleware is
	// merged safely inside app.handle() under the app's read lock.
	fullPath := g.prefix + path
	combined := make(HandlersChain, len(g.middleware)+len(handlers))
	copy(combined, g.middleware)
	copy(combined[len(g.middleware):], handlers)
	g.app.handle(method, fullPath, combined)
	// Register metadata and tags for runtime introspection.
	if len(g.metadata) > 0 || len(g.tags) > 0 {
		g.app.registerRouteMeta(method, fullPath, g.metadata, g.tags)
	}
}
