package router

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
)

// Router is the Astra HTTP router backed by method-keyed radix tries.
//
// The router holds its own copies of the 404/405 handlers rather than reaching
// back into App.options, so it can be exercised and replaced independently.
//
// mu guards trees, methodRoots, and all node mutations.  Add acquires the
// write lock; Handle, Routes, and maxParamDepth acquire the read lock.
// Routes are typically registered at startup (before the first request),
// so the read path is always uncontended in production — the lock is free.
type Router struct {
	mu sync.RWMutex
	trees                   map[string]*node
	// methodRoots is a fixed-order slice of (method, root) pairs used by
	// allowedMethods() to iterate trees in a deterministic sequence (GET first,
	// then POST/PUT/PATCH/DELETE, then the rest).  Deterministic order produces
	// stable Allow header values and provides better CPU branch-prediction during
	// the 405 traversal compared to random map iteration.
	methodRoots             []methodRoot
	notFoundHandler         HandlerFunc
	methodNotAllowedHandler HandlerFunc
	// Pre-allocated chains avoid a []HandlerFunc literal allocation per 404/405.
	notFoundChain         HandlersChain
	methodNotAllowedChain HandlersChain
	logger                *slog.Logger
	// strictConflict causes Add to panic on duplicate route registration.
	// Enabled in ModeTest or via WithStrictConflict.
	strictConflict        bool

	// maxParamValueLen, when > 0, rejects param segments longer than this many bytes.
	maxParamValueLen int
	// allowedCache caches the Allow header string produced by allowedMethods for
	// each raw request path. Populated lazily on the first 405 hit per path.
	// sync.Map is appropriate here: write-once-per-path / read-many pattern.
	allowedCache sync.Map // key: string path → value: string Allow header
}

// methodRoot pairs an HTTP method string with its radix-tree root node.
// Stored in methodRoots for ordered iteration during 405 Allow-header construction.
type methodRoot struct {
	method string
	root   *node
}

// methodOrder defines the stable iteration order for Allow-header construction.
// Methods that appear first get checked first; unlisted methods are appended
// in registration order at the end of methodRoots.
var methodOrder = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodHead,
	http.MethodOptions,
}

// NewRouter creates a new Router with the given configuration.
func NewRouter(notFoundHandler, methodNotAllowedHandler HandlerFunc, logger *slog.Logger, strictConflict bool, maxParamValueLen int) *Router {
	r := &Router{
		trees:                   make(map[string]*node),
		notFoundHandler:         notFoundHandler,
		methodNotAllowedHandler: methodNotAllowedHandler,
		logger:                  logger,
		strictConflict:          strictConflict,
		maxParamValueLen:        maxParamValueLen,
	}
	r.notFoundChain = HandlersChain{r.notFoundHandler}
	r.methodNotAllowedChain = HandlersChain{r.methodNotAllowedHandler}
	return r
}

// methodOrderRank returns the sort key for a method in the Allow-header output.
// Methods in methodOrder get rank 0..N-1; others get N (appended last).
func methodOrderRank(method string) int {
	for i, m := range methodOrder {
		if m == method {
			return i
		}
	}
	return len(methodOrder)
}

// Add registers a new route in the radix tree.
// If a handler is already registered for the same method and path, the new
// handler replaces it and a warning is emitted via slog so the conflict is
// visible in logs instead of silently discarded.
func (r *Router) Add(method, path string, handlers HandlersChain) {
	if path == "" {
		panic(fmt.Sprintf("astra: path cannot be empty (method=%q)", method))
	}
	// Detect paths that are pure whitespace (e.g. " ", "\t", "  /  ").
	// After trimming the leading '/' the path becomes " " or "\t", which
	// would panic in the next guard as path[0] != '/'.  We give a clear
	// message here so the root cause is obvious.
	if len(path) > 0 && path[0] != '/' {
		panic(fmt.Sprintf("astra: path must start with '/' (method=%q, path=%q)", method, path))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	root, ok := r.trees[method]
	if !ok {
		root = &node{path: "/"}
		r.trees[method] = root
		// Insert into methodRoots at the position that maintains methodOrder.
		// Add() is a startup-only path so the O(N) insertion is acceptable.
		rank := methodOrderRank(method)
		pos := len(r.methodRoots)
		for i, mr := range r.methodRoots {
			if methodOrderRank(mr.method) > rank {
				pos = i
				break
			}
		}
		r.methodRoots = append(r.methodRoots, methodRoot{})
		copy(r.methodRoots[pos+1:], r.methodRoots[pos:])
		r.methodRoots[pos] = methodRoot{method: method, root: root}
	}
	if overwritten := insertNode(root, path, handlers); overwritten {
		msg := fmt.Sprintf("astra: route conflict: handler overwritten for %s %s", method, path)
		if r.strictConflict {
			panic(msg)
		}
		r.logger.Warn("astra: route conflict: handler overwritten",
			"method", method,
			"path", path,
		)
	}
	// Invalidate the allowed-methods cache: a new route may change which methods
	// are valid for a given path. Routes are startup-only in practice, so this
	// Range+Delete is called at most once per registered route.
	r.allowedCache.Range(func(k, _ any) bool {
		r.allowedCache.Delete(k)
		return true
	})
}

// Handle dispatches an HTTP request to the matching handler chain.
// This method is called by the App to route incoming requests.
func (r *Router) Handle(c Ctx) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	req := c.Request()
	method := req.Method
	path := req.URL.Path

	root, ok := r.trees[method]
	if !ok {
		// No tree for this method: check if other methods match (405) or not (404).
		allow := r.cachedAllowedMethods(path)
		if allow != "" {
			c.SetAllowedMethods(allow)
			c.SetHandlers(r.methodNotAllowedChain)
			c.SetParams(nil)
			c.SetRouteKey("")
		} else {
			c.SetHandlers(r.notFoundChain)
			c.SetParams(nil)
			c.SetRouteKey("")
		}
		c.Next()
		return
	}

	// Match the route in the method tree.
	handlers, params, fullPath, found := matchRoute(root, path, nil, r.maxParamValueLen)
	if !found {
		// No route matched: check if other methods match (405) or not (404).
		allow := r.cachedAllowedMethods(path)
		if allow != "" {
			c.SetAllowedMethods(allow)
			c.SetHandlers(r.methodNotAllowedChain)
			c.SetParams(nil)
			c.SetRouteKey("")
		} else {
			c.SetHandlers(r.notFoundChain)
			c.SetParams(nil)
			c.SetRouteKey("")
		}
		c.Next()
		return
	}

	// Route matched: set handlers, params, and route key, then execute.
	c.SetHandlers(handlers)
	c.SetParams(params)
	c.SetRouteKey(fullPath)
	c.Next()
}

// cachedAllowedMethods returns the Allow header string for path, using
// allowedCache to avoid repeated trie traversals for the same path.
// The cache is populated on first miss; subsequent hits are a sync.Map load.
// mu must be held (read) by the caller.
func (r *Router) cachedAllowedMethods(path string) string {
	if v, ok := r.allowedCache.Load(path); ok {
		return v.(string)
	}
	allow := r.allowedMethods(path)
	r.allowedCache.Store(path, allow)
	return allow
}

// allowedMethods traverses all registered method trees for the given path and
// returns a comma-separated list of matching HTTP methods, suitable for the
// Allow response header (RFC 9110 §15.5.6).  Returns "" if no tree matches
// (the caller should respond 404, not 405).
//
// Traversal uses the ordered methodRoots slice (GET → POST → PUT → PATCH →
// DELETE → HEAD → OPTIONS → ...) to produce a deterministic, stable header
// value across requests.
//
// Performance note: this function is only called on the 405 slow path (the
// requested method has no tree at all).  Each matchRoute call is ~15–25 ns on
// a shallow tree; with the typical 4–7 registered methods the total cost is
// ~100–175 ns, which is acceptable for an error path.
func (r *Router) allowedMethods(path string) string {
	// Stack-allocate a fixed buffer large enough for all standard methods plus
	// separators: "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS" = 47 chars;
	// with a few custom methods we use 128 bytes to be safe.
	var buf [128]byte
	n := 0
	for _, mr := range r.methodRoots {
		_, _, _, found := matchRoute(mr.root, path, nil, 0)
		if !found {
			continue
		}
		if n > 0 {
			buf[n] = ','
			buf[n+1] = ' '
			n += 2
		}
		n += copy(buf[n:], mr.method)
	}
	if n == 0 {
		return ""
	}
	return string(buf[:n])
}

// Routes returns all registered routes for introspection.
func (r *Router) Routes() []RouteInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var routes []RouteInfo
	for method, root := range r.trees {
		collectRoutes(root, "", method, &routes)
	}
	return routes
}

// MaxParamDepth returns the maximum number of path parameters across all
// registered routes in all method trees.  Called once at App startup to size
// the params slice in sync.Pool so deep-param routes never trigger a
// mid-request heap allocation.
func (r *Router) MaxParamDepth() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	max := 0
	for _, mr := range r.methodRoots {
		if d := nodeParamDepth(mr.root, 0); d > max {
			max = d
		}
	}
	return max
}
