package astra

import (
	"strings"
)

// routeMetaEntry stores metadata and tags associated with a registered route.
type routeMetaEntry struct {
	Metadata map[string]string
	Tags     []string
}

// metadataKey builds the lookup key for the App.routeMeta registry.
func metadataKey(method, fullPath string) string {
	return method + ":" + fullPath
}

// registerRouteMeta stores metadata and tags for a route in the shared registry.
// Safe for concurrent access once routes are fully registered (before ServeHTTP).
// During registration it is called under the App.mu write lock.
func (a *App) registerRouteMeta(method, fullPath string, meta map[string]string, tags []string) {
	if len(meta) == 0 && len(tags) == 0 {
		return
	}
	a.mu.Lock()
	if a.routeMeta == nil {
		a.routeMeta = make(map[string]routeMetaEntry, 64)
	}
	key := metadataKey(method, fullPath)
	entry, exists := a.routeMeta[key]
	if !exists {
		entry = routeMetaEntry{
			Metadata: make(map[string]string, len(meta)),
			Tags:     make([]string, 0, len(tags)),
		}
	}
	for k, v := range meta {
		entry.Metadata[k] = v
	}
	entry.Tags = append(entry.Tags, tags...)
	a.routeMeta[key] = entry
	a.mu.Unlock()
}

// getRouteMeta returns a copy of the metadata and tags for the given route.
// Returns nil if no metadata is registered.
func (a *App) getRouteMeta(method, fullPath string) (map[string]string, []string) {
	a.mu.RLock()
	entry, ok := a.routeMeta[metadataKey(method, fullPath)]
	a.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	// Return shallow copies to avoid mutation races.
	meta := make(map[string]string, len(entry.Metadata))
	for k, v := range entry.Metadata {
		meta[k] = v
	}
	tags := make([]string, len(entry.Tags))
	copy(tags, entry.Tags)
	return meta, tags
}

// RouteMeta returns the value of the named metadata key for the matched route.
// For example:
//
//	group.Metadata("rate_limit", "100/min")
//	// later, in middleware or handler:
//	rate, ok := c.RouteMeta("rate_limit")
//
// Returns ("", false) when the key is not found or no metadata is registered
// for the current route.
func (c *Ctx) RouteMeta(key string) (string, bool) {
	meta, _ := c.app.getRouteMeta(c.Request().Method, c.routeKey)
	if meta == nil {
		return "", false
	}
	val, ok := meta[key]
	return val, ok
}

// RouteTags returns the tags associated with the matched route.
// Returns nil if the route has no tags.
func (c *Ctx) RouteTags() []string {
	_, tags := c.app.getRouteMeta(c.Request().Method, c.routeKey)
	return tags
}

// RouteHasTag returns true if the matched route has the specified tag.
func (c *Ctx) RouteHasTag(tag string) bool {
	_, tags := c.app.getRouteMeta(c.Request().Method, c.routeKey)
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// EnrichedRouteInfo extends RouteInfo with metadata and tags.
// Used by the framework internally; the `Routes()` method on App and Group
// returns this type so callers can access per-route metadata.
type EnrichedRouteInfo struct {
	RouteInfo
	Metadata map[string]string
	Tags     []string
}

// enrichRoutes enriches a slice of RouteInfo with metadata and tags from the
// App registry.
func (a *App) enrichRoutes(routes []RouteInfo) []EnrichedRouteInfo {
	result := make([]EnrichedRouteInfo, len(routes))
	for i, r := range routes {
		fullPath := r.FullPath
		if fullPath == "" {
			fullPath = r.Path
		}
		meta, tags := a.getRouteMeta(r.Method, fullPath)
		result[i] = EnrichedRouteInfo{
			RouteInfo: r,
			Metadata:  meta,
			Tags:      tags,
		}
	}
	return result
}

// Routes returns all registered routes with metadata and tags.
// This overrides the old-style plain RouteInfo list for introspection tools.
func (a *App) Routes() []EnrichedRouteInfo {
	return a.enrichRoutes(a.router.Routes())
}

// GetRouteMeta returns a copy of the metadata registered for the given method/path
// on the App level. Returns nil when no metadata is found.
func (a *App) GetRouteMeta(method, path string) map[string]string {
	meta, _ := a.getRouteMeta(method, path)
	return meta
}

// GetRouteTags returns a copy of the tags registered for the given method/path
// on the App level. Returns nil when no tags are found.
func (a *App) GetRouteTags(method, path string) []string {
	_, tags := a.getRouteMeta(method, path)
	return tags
}

// ─── App-level Metadata/Tag helpers ──────────────────────────────────────────

// SetRouteMeta registers metadata for a specific route on the App level.
// This is useful for programmatic route registration:
//
//	app.POST("/api/users", createUser)
//	app.SetRouteMeta("POST", "/api/users", "rate_limit", "100/min")
func (a *App) SetRouteMeta(method, path, key, value string) {
	a.registerRouteMeta(method, path, map[string]string{key: value}, nil)
}

// SetRouteTags registers tags for a specific route on the App level.
func (a *App) SetRouteTags(method, path string, tags ...string) {
	a.registerRouteMeta(method, path, nil, tags)
}

// ─── Group chaining helpers ──────────────────────────────────────────────────

// Metadata sets a key-value metadata entry on this Group.
// Metadata is inherited by subgroups and attached to every route registered
// via this group. Chainable.
//
//	app.Group("/api").Metadata("version", "v1").GET("/users", listUsers)
//	// later, in middleware:
//	ver, ok := c.RouteMeta("version")  // "v1", true
func (g *Group) Metadata(key, value string) *Group {
	if g.metadata == nil {
		g.metadata = make(map[string]string)
	}
	g.metadata[key] = value
	return g
}

// Tag adds tags to this Group. Tags are inherited by subgroups and attached to
// every route registered via this group. Chainable.
//
//	app.Group("/admin").Tag("auth", "admin").GET("/dashboard", adminDash)
//	// later, in middleware:
//	c.RouteHasTag("auth")  // true
func (g *Group) Tag(tags ...string) *Group {
	g.tags = append(g.tags, tags...)
	return g
}

// FullPath returns the complete prefix path of this Group, useful for debugging
// and metrics labels.
func (g *Group) FullPath() string {
	return g.prefix
}

// SplitGroup splits the current prefix into segments and returns them.
// Each segment is the part between slashes (empty segments are skipped).
// Example: "/api/v1/users" → ["api", "v1", "users"]
func (g *Group) SplitGroup() []string {
	parts := strings.Split(g.prefix, "/")
	var result []string
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// Routes returns all routes registered under this Group's prefix, enriched
// with metadata and tags.
func (g *Group) Routes() []EnrichedRouteInfo {
	routes := g.app.Routes()
	var result []EnrichedRouteInfo
	for _, r := range routes {
		if strings.HasPrefix(r.Path, g.prefix) || strings.HasPrefix(r.FullPath, g.prefix) {
			result = append(result, r)
		}
	}
	return result
}
