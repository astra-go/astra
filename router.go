package astra

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
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

// nodeType categorizes each node in the radix tree.
type nodeType uint8

const (
	staticNode   nodeType = iota
	regexNode              // {name:pattern} — regex-constrained param
	paramNode              // :name — unconstrained param
	catchAllNode           // *name — catch-all
)

// ── Regexp pre-compilation cache ─────────────────────────────────────────────
//
// regexpCache maps anchored pattern strings ("^(?:<pattern>)$") to their
// compiled *regexp.Regexp.  Using sync.Map because the access pattern is
// write-once-at-startup / read-many-at-runtime: sync.Map avoids the global
// mutex contention that a plain map+RWMutex would impose under high load.
var regexpCache sync.Map // key: string → value: *regexp.Regexp

// getOrCompileRegexp returns a shared *regexp.Regexp for the anchored pattern
// "^(?:<pattern>)$", compiling and caching it on first use.  All routes with
// the same pattern share one Regexp instance and therefore one internal machine
// pool, which reduces pool fragmentation and GC pressure under concurrency.
func getOrCompileRegexp(pattern string) (*regexp.Regexp, error) {
	anchored := "^(?:" + pattern + ")$"
	if v, ok := regexpCache.Load(anchored); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(anchored)
	if err != nil {
		return nil, err
	}
	// LoadOrStore is safe under concurrent insertNode calls at startup.
	// If another goroutine stored first, we use its instance and let ours be GC'd.
	actual, _ := regexpCache.LoadOrStore(anchored, re)
	return actual.(*regexp.Regexp), nil
}

// ── Fast-path matchers for common patterns ────────────────────────────────────
//
// For the most frequent regex patterns, a direct byte-scan is orders of
// magnitude faster than the regexp engine: no automaton construction, no pool
// get/put, no allocation.  The fast matcher is stored alongside the compiled
// Regexp and used instead when non-nil.

// fastMatcher is a zero-allocation string predicate for common patterns.
type fastMatcher func(s string) bool

// wellKnownMatchers maps common regexp patterns to their fast-path equivalents.
// Checked at route-registration time; any pattern listed here bypasses the
// regexp engine on every request.
var wellKnownMatchers = map[string]fastMatcher{
	`[0-9]+`:          fastDigits,
	`\d+`:             fastDigits,
	`[a-z]+`:          fastLower,
	`[A-Z]+`:          fastUpper,
	`[a-zA-Z]+`:       fastAlpha,
	`[a-z0-9]+`:       fastAlphanumLower,
	`[A-Z0-9]+`:       fastAlphanumUpper,
	`[a-zA-Z0-9]+`:    fastAlphanum,
	`[a-z0-9\-]+`:     fastSlugLower,
	`[a-z0-9-]+`:      fastSlugLower,
	`[a-zA-Z0-9_\-]+`: fastSlug,
	`[a-zA-Z0-9_-]+`:  fastSlug,
	`[a-zA-Z0-9_]+`:   fastIdentifier,
	`\w+`:             fastIdentifier,
}

func fastDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func fastLower(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

func fastUpper(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

func fastAlpha(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

func fastAlphanumLower(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func fastAlphanumUpper(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func fastAlphanum(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func fastSlugLower(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func fastSlug(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func fastIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// childIndexAbsent and childIndexCollision are sentinel values for node.childIndex.
// Using int16 supports up to 32 765 static siblings before the index saturates.
const (
	childIndexAbsent    int16 = -1 // no child has this first byte
	childIndexCollision int16 = -2 // two or more children share this first byte → linear scan
)

// node is a node in the radix tree.
type node struct {
	path          string
	nType         nodeType
	handlers      HandlersChain
	paramKey      string
	regex         *regexp.Regexp // non-nil for regexNode; shared via regexpCache
	fastMatch     fastMatcher    // non-nil when a well-known pattern fast-path is available
	children      []*node        // static children (kept for ordered iteration)
	childIndex    *[256]int16    // first-byte dispatch: childIndex[b] = index into children, or sentinel
	childMap      map[string]*node // non-nil when childIndex has collision buckets; O(1) full-path lookup
	regexChildren []*node        // regex-constrained children (multiple patterns allowed)
	param         *node          // :name child
	catchAll      *node          // *name child
	fullPath      string
}

// newChildIndex allocates and initialises a first-byte dispatch table.
// All 256 entries are set to childIndexAbsent (-1).
func newChildIndex() *[256]int16 {
	idx := new([256]int16)
	for i := range idx {
		idx[i] = childIndexAbsent
	}
	return idx
}

// recordChildIndex updates n.childIndex after a new child was appended to
// n.children at position childPos.  Must be called exactly once per append.
// When two children collide on the same first byte, childMap is populated so
// that matchSegments can resolve the collision in O(1) via a map lookup instead
// of a linear scan over all children.
func recordChildIndex(n *node, childPos int) {
	if len(n.children[childPos].path) == 0 {
		return
	}
	if n.childIndex == nil {
		n.childIndex = newChildIndex()
	}
	b := n.children[childPos].path[0]
	switch n.childIndex[b] {
	case childIndexAbsent:
		n.childIndex[b] = int16(childPos) // sole occupant of this bucket
	default:
		if n.childIndex[b] != childIndexCollision {
			// First collision for this byte: migrate the previously indexed child
			// into childMap so we have O(1) lookup for both children.
			if n.childMap == nil {
				n.childMap = make(map[string]*node, 4)
			}
			n.childMap[n.children[n.childIndex[b]].path] = n.children[n.childIndex[b]]
			n.childIndex[b] = childIndexCollision
		}
		// Add the newly appended child to childMap.
		if n.childMap == nil {
			n.childMap = make(map[string]*node, 4)
		}
		n.childMap[n.children[childPos].path] = n.children[childPos]
	}
}

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

func newRouter(app *App) *Router {
	r := &Router{
		trees:                   make(map[string]*node),
		notFoundHandler:         app.options.NotFoundHandler,
		methodNotAllowedHandler: app.options.MethodNotAllowedHandler,
		logger:                  slog.Default(),
		strictConflict:          app.options.StrictConflict || app.options.Mode == ModeTest,
		maxParamValueLen:        app.options.MaxParamValueLen,
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
		panic("astra: path cannot be empty")
	}
	if path[0] != '/' {
		panic("astra: path must start with '/'")
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

func insertNode(root *node, path string, handlers HandlersChain) (overwritten bool) {
	parts := splitPath(path)
	current := root

	// Root path "/"
	if len(parts) == 0 {
		overwritten = root.handlers != nil
		root.handlers = handlers
		root.fullPath = path
		return
	}

	for i, part := range parts {
		isLast := i == len(parts)-1

		if strings.HasPrefix(part, "*") {
			key := part[1:]
			if current.catchAll == nil {
				current.catchAll = &node{path: part, nType: catchAllNode, paramKey: key, fullPath: path}
			}
			if isLast {
				overwritten = current.catchAll.handlers != nil
				current.catchAll.handlers = handlers
				current.catchAll.fullPath = path
			}
			break
		}

		// {name:pattern} — regex-constrained parameter
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") && strings.Contains(part, ":") {
			inner := part[1 : len(part)-1] // strip { }
			colonIdx := strings.Index(inner, ":")
			key := inner[:colonIdx]
			pattern := inner[colonIdx+1:]
			if key == "" || pattern == "" {
				panic(fmt.Sprintf("astra: invalid regex segment %q in path %q", part, path))
			}
			re, err := getOrCompileRegexp(pattern)
			if err != nil {
				panic(fmt.Sprintf("astra: invalid regex %q in path %q: %v", pattern, path, err))
			}
			child := findRegexChild(current, re)
			if child == nil {
				child = &node{
					path:      part,
					nType:     regexNode,
					paramKey:  key,
					regex:     re,
					fastMatch: wellKnownMatchers[pattern],
				}
				current.regexChildren = append(current.regexChildren, child)
			}
			if isLast {
				overwritten = child.handlers != nil
				child.handlers = handlers
				child.fullPath = path
			}
			current = child
			continue
		}

		if strings.HasPrefix(part, ":") {
			key := part[1:]
			if current.param == nil {
				current.param = &node{path: part, nType: paramNode, paramKey: key, fullPath: path}
			}
			if isLast {
				overwritten = current.param.handlers != nil
				current.param.handlers = handlers
				current.param.fullPath = path
			}
			current = current.param
		} else {
			child := findStaticChild(current, part)
			if child == nil {
				child = &node{path: part, nType: staticNode, fullPath: path}
				current.children = append(current.children, child)
				recordChildIndex(current, len(current.children)-1)
			}
			if isLast {
				overwritten = child.handlers != nil
				child.handlers = handlers
				child.fullPath = path
			}
			current = child
		}
	}
	return
}

// findRegexChild finds an existing regex child that shares the same compiled
// Regexp instance (pointer equality after cache lookup).
func findRegexChild(n *node, re *regexp.Regexp) *node {
	for _, child := range n.regexChildren {
		if child.regex == re {
			return child
		}
	}
	return nil
}

func findStaticChild(n *node, segment string) *node {
	for _, child := range n.children {
		if child.path == segment {
			return child
		}
	}
	return nil
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// Handle dispatches an HTTP request to the matching handler chain.
func (r *Router) Handle(c *Ctx) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	method := c.req.Method
	path := c.req.URL.Path

	root, ok := r.trees[method]
	if !ok {
		// Build Allow header by traversing all method trees (RFC 9110 §15.5.6
		// requires the Allow header in every 405 response).  allowedMethods uses
		// the ordered methodRoots slice so the header value is deterministic.
		// Results are cached in allowedCache (sync.Map) so repeated 405 hits on
		// the same path skip the trie traversal entirely.
		if allow := r.cachedAllowedMethods(path); allow != "" {
			c.rw.ResponseWriter.Header().Set("Allow", allow)
			c.handlers = r.methodNotAllowedChain
		} else {
			c.handlers = r.notFoundChain
		}
		c.Next()
		return
	}

	// Pass c.params (backed by c.paramsArr) into matchRoute so that param
	// extraction appends into the pre-allocated inline array — no heap alloc
	// for routes with ≤maxRouteParams path parameters.
	handlers, params, fullPath, found := matchRoute(root, path, c.params, r.maxParamValueLen)
	if !found {
		c.handlers = r.notFoundChain
		c.Next()
		return
	}

	c.handlers = handlers
	c.params = params // updated slice header (length may have grown)
	// Store the matched route template so middleware (e.g. metrics, tracing) can
	// use a low-cardinality label instead of the raw request path.
	// Written directly to c.routeKey to avoid the string→any boxing alloc that
	// c.Set(contract.RouteKey, fullPath) would incur on every matched request.
	// Readers: c.GetString(contract.RouteKey) has a matching lock-free fast path.
	if fullPath != "" {
		c.routeKey = fullPath
	}
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

// matchRoute traverses the trie to find handlers and extract params.
//
// params is passed in as the initial (pre-allocated) slice; it is typically
// c.params from Handle(), backed by c.paramsArr.  Passing a non-nil slice with
// sufficient capacity (≥ number of path parameters) means all append() calls
// inside matchSegments stay within the existing backing array — zero heap alloc.
//
// Path parsing is done inline (no splitPath / strings.Split call), eliminating
// two allocations on every request compared to the previous design.
func matchRoute(root *node, path string, params Params, maxParamValueLen int) (HandlersChain, Params, string, bool) {
	pos := 0
	if len(path) > 0 && path[0] == '/' {
		pos = 1
	}
	// Root path "/" or empty: check root node directly.
	if pos >= len(path) {
		if root.handlers != nil {
			return root.handlers, params, root.fullPath, true
		}
		return nil, nil, "", false
	}
	return matchSegments(root, path, pos, params, maxParamValueLen)
}

// matchSegments is the recursive heart of the router.
//
// It operates directly on the original path string using byte-offset slicing
// (path[a:b]) rather than a pre-split []string, so each call allocates nothing
// for segment extraction.  param values stored in Params are sub-slices of the
// input path string — valid for the lifetime of the http.Request.
func matchSegments(current *node, path string, pos int, params Params, maxParamValueLen int) (HandlersChain, Params, string, bool) {
	// Extract the current segment without allocating.
	// strings.IndexByte is O(k) on path[pos:] — no slice header escapes.
	end := strings.IndexByte(path[pos:], '/')
	var part string
	var nextPos int
	if end < 0 {
		part = path[pos:]       // sub-slice of original string: no alloc
		nextPos = len(path)
	} else {
		part = path[pos : pos+end] // sub-slice: no alloc
		nextPos = pos + end + 1    // skip the '/'
	}
	isLast := nextPos >= len(path)

	// Catch-all consumes the current segment and everything after it.
	// path[pos-1:] is a sub-slice of the original string — no alloc.
	// pos is always ≥1 here (guaranteed by matchRoute stripping the leading '/').
	if current.catchAll != nil && current.catchAll.handlers != nil {
		remaining := path[pos-1:] // "/segment/rest..." — no alloc
		if maxParamValueLen > 0 && len(remaining) > maxParamValueLen {
			return nil, nil, "", false
		}
		p := append(params, Param{Key: current.catchAll.paramKey, Value: remaining})
		return current.catchAll.handlers, p, current.catchAll.fullPath, true
	}

	// Try static children first (most specific match).
	// Fast path: use childIndex to skip to the right bucket in O(1).
	var matchedStatic *node
	if current.childIndex != nil && len(part) > 0 {
		switch idx := current.childIndex[part[0]]; {
		case idx >= 0:
			if current.children[idx].path == part {
				matchedStatic = current.children[idx]
			}
			// idx >= 0 but path mismatch: no other child can have this first byte.
		case idx == childIndexCollision:
			// Two or more children share this first byte; use childMap for O(1)
			// lookup when available, otherwise fall back to linear scan.
			if current.childMap != nil {
				matchedStatic = current.childMap[part]
			} else {
				for _, child := range current.children {
					if child.path == part {
						matchedStatic = child
						break
					}
				}
			}
		}
		// childIndexAbsent (-1): no child has this first byte; matchedStatic stays nil.
	} else {
		for _, child := range current.children {
			if child.path == part {
				matchedStatic = child
				break
			}
		}
	}
	if matchedStatic != nil {
		if isLast {
			if matchedStatic.handlers != nil {
				return matchedStatic.handlers, params, matchedStatic.fullPath, true
			}
			if matchedStatic.catchAll != nil && matchedStatic.catchAll.handlers != nil {
				p := append(params, Param{Key: matchedStatic.catchAll.paramKey, Value: "/"})
				return matchedStatic.catchAll.handlers, p, matchedStatic.catchAll.fullPath, true
			}
			return nil, nil, "", false
		}
		if h, p, fp, ok := matchSegments(matchedStatic, path, nextPos, params, maxParamValueLen); ok {
			return h, p, fp, ok
		}
	}

	// Try regex children (more specific than bare :param).
	// Fast-path matchers bypass the regexp engine for well-known patterns.
	for _, child := range current.regexChildren {
		var matched bool
		if child.fastMatch != nil {
			matched = child.fastMatch(part)
		} else {
			matched = child.regex != nil && child.regex.MatchString(part)
		}
		if matched {
			if maxParamValueLen > 0 && len(part) > maxParamValueLen {
				continue
			}
			newParams := append(params, Param{Key: child.paramKey, Value: part})
			if isLast {
				if child.handlers != nil {
					return child.handlers, newParams, child.fullPath, true
				}
				if child.catchAll != nil && child.catchAll.handlers != nil {
					p := append(newParams, Param{Key: child.catchAll.paramKey, Value: "/"})
					return child.catchAll.handlers, p, child.catchAll.fullPath, true
				}
				return nil, nil, "", false
			}
			if h, p, fp, ok := matchSegments(child, path, nextPos, newParams, maxParamValueLen); ok {
				return h, p, fp, ok
			}
		}
	}

	// Try param child.
	if current.param != nil {
		if maxParamValueLen == 0 || len(part) <= maxParamValueLen {
			newParams := append(params, Param{Key: current.param.paramKey, Value: part})
			if isLast {
				if current.param.handlers != nil {
					return current.param.handlers, newParams, current.param.fullPath, true
				}
				if current.param.catchAll != nil && current.param.catchAll.handlers != nil {
					p := append(newParams, Param{Key: current.param.catchAll.paramKey, Value: "/"})
					return current.param.catchAll.handlers, p, current.param.catchAll.fullPath, true
				}
				return nil, nil, "", false
			}
			if h, p, fp, ok := matchSegments(current.param, path, nextPos, newParams, maxParamValueLen); ok {
				return h, p, fp, ok
			}
		}
	}

	return nil, nil, "", false
}

// Routes returns all registered routes for introspection.
type RouteInfo struct {
	Method   string
	Path     string
	FullPath string
}

func (r *Router) Routes() []RouteInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var routes []RouteInfo
	for method, root := range r.trees {
		collectRoutes(root, "", method, &routes)
	}
	return routes
}

func collectRoutes(n *node, prefix string, method string, routes *[]RouteInfo) {
	var path string
	if n.path == "/" {
		path = "/"
	} else {
		path = prefix + "/" + n.path
	}
	if n.handlers != nil && n.path != "/" {
		*routes = append(*routes, RouteInfo{Method: method, Path: path, FullPath: n.fullPath})
	} else if n.handlers != nil && n.path == "/" {
		*routes = append(*routes, RouteInfo{Method: method, Path: "/", FullPath: n.fullPath})
	}
	for _, child := range n.children {
		collectRoutes(child, path, method, routes)
	}
	for _, child := range n.regexChildren {
		collectRoutes(child, path, method, routes)
	}
	if n.param != nil {
		collectRoutes(n.param, path, method, routes)
	}
	if n.catchAll != nil {
		collectRoutes(n.catchAll, path, method, routes)
	}
}

// Ensure net/http is used.
var _ http.Handler = (*App)(nil)

// Ensure *Router satisfies HttpRouter at compile time.
var _ HttpRouter = (*Router)(nil)

// maxParamDepth returns the maximum number of path parameters across all
// registered routes in all method trees.  Called once at App startup to size
// the params slice in sync.Pool so deep-param routes never trigger a
// mid-request heap allocation.
func (r *Router) maxParamDepth() int {
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

// nodeParamDepth recursively walks the radix tree and returns the maximum
// accumulated param count along any root-to-leaf path.
func nodeParamDepth(n *node, depth int) int {
	if n.nType == paramNode || n.nType == regexNode {
		depth++
	}
	max := depth
	for _, child := range n.children {
		if d := nodeParamDepth(child, depth); d > max {
			max = d
		}
	}
	for _, child := range n.regexChildren {
		if d := nodeParamDepth(child, depth); d > max {
			max = d
		}
	}
	if n.param != nil {
		if d := nodeParamDepth(n.param, depth); d > max {
			max = d
		}
	}
	if n.catchAll != nil {
		if d := nodeParamDepth(n.catchAll, depth+1); d > max {
			max = d
		}
	}
	return max
}

