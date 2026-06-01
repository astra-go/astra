package astra

import (
	"context"
	"net/http"
	"net/url"
)

// maxRouteParams is the capacity of the inline params backing array.
// Routes with more than 8 path parameters will fall back to a heap allocation
// on the first append that exceeds this limit (rare in practice).
const maxRouteParams = 8

// kvPair holds one entry in the per-request context's key-value store.
type kvPair struct {
	key   string
	value any
}

// Ctx is the concrete per-request context.
// Handlers and middleware receive *Ctx directly — no interface boxing,
// all method calls are inlinable.
//
// Allocation strategy:
//   - rw (responseWriter) is embedded as a value field so reset() can update it
//     in-place via field assignments — no heap allocation per request.
//   - writer (ResponseWriter interface) points to &c.rw; set once in
//     allocateContext, reset() restores it after any middleware SetWriter call.
//   - paramsArr is an inline [maxRouteParams]Param backing array; params is a
//     slice over it.  append() stays within this array for ≤8 path params,
//     avoiding any heap allocation during route matching.
//   - routeKey holds the matched route template directly as a string field,
//     bypassing the key-value store to eliminate string→any boxing (1 alloc/req).
//   - kvStore is a []kvPair slice for per-request key-value data. reset() resets
//     it to [:0] to retain the backing array across requests (zero allocation on
//     reuse). Linear scan over a small slice is faster than map hashing and needs
//     no mutex — the context belongs to a single goroutine throughout the request.
//
// Concurrency: Ctx is NOT goroutine-safe. c.Set / c.Get must not be called
// concurrently from goroutines spawned within a handler. Copy needed values into
// local variables before launching goroutines (same contract as Gin / Echo).
//
// # Method index
//
// context_request.go — HTTP request/response accessors
//
//	Request()  SetRequest()  Writer()  SetWriter()
//	Param(key)
//	Query(key)  DefaultQuery(key, def)  QueryMap()
//	PostForm(key)  DefaultPostForm(key, def)  FormFile(key)
//	Header(key)  SetHeader(key, val)  ContentType()
//	ClientIP()  UserAgent()  IsWebsocket()
//
// context_response.go — Response serialisation
//
//	JSON(code, obj)  JSONStream(code, obj)
//	XML(code, obj)  String(code, fmt, …)  HTML(code, html)
//	Render(code, name, data)  Blob(code, ct, data)
//	NoContent(code)  Redirect(code, url)  File(path)
//	SSEvent(event, data)  EarlyHints(targets, opts)
//	Push(target, opts)  [deprecated — use EarlyHints]
//
// context_bind.go — Request binding and validation
//
//	Bind(obj)  BindJSON(obj)  BindXML(obj)  BindForm(obj)
//	BindQuery(obj)  BindPath(obj)
//	ShouldBind(obj)  ShouldBindJSON(obj)  ShouldBindXML(obj)
//	ShouldBindForm(obj)  ShouldBindQuery(obj)  ShouldBindPath(obj)
//	MustBind(obj)  MustBindJSON(obj)
//	Validate(obj)
//
// context_store.go — Per-request key-value store
//
//	Set(key, val)  Get(key)  MustGet(key)
//	GetString(key)  GetInt(key)  GetInt64(key)  GetFloat64(key)  GetBool(key)
//	TryGetString(key)  TryGetInt(key)  TryGetBool(key)
//
// context_flow.go — Handler-chain flow control
//
//	Next()  Abort()  AbortWithStatus(code)
//	AbortWithError(code, err)  IsAborted()
type Ctx struct {
	req *http.Request

	// debugFields is embedded to add goroutine tracking in debug builds.
	// In production builds (without astra_debug tag), this is an empty struct.
	debugFields

	// rw is the embedded response writer value.  Its fields are updated in-place
	// by reset() so that c.writer (the interface) never needs to point at a freshly
	// allocated object.
	rw     responseWriter
	writer ResponseWriter // always &c.rw unless middleware called SetWriter

	// paramsArr is the inline backing array for URL path parameters.
	// params is a re-slice of paramsArr reset to [:0] on each request.
	// When the application registers routes with more than maxRouteParams path
	// parameters, allocateContext pre-allocates an overflowParams slice sized to
	// the actual maximum depth; reset() switches params to that slice instead,
	// keeping all route matching allocation-free regardless of param count.
	paramsArr      [maxRouteParams]Param
	params         Params
	overflowParams Params // non-nil when max route depth > maxRouteParams

	// handler chain and current index.
	// index advances through handlers in Next(); Abort() sets it to abortIndex
	// (math.MaxInt16) to stop the chain. int16 supports up to 32 767 handlers
	// per chain — far beyond any practical limit.
	handlers HandlersChain
	index    int16

	// routeKey is the matched route template (e.g. "/users/:id"), set directly
	// by the router to avoid the string→any interface boxing that c.Set would incur.
	// Exposed via Get/GetString(contract.RouteKey) through a special-case fast path.
	routeKey string

	// kvStore is the per-request key-value store. It grows on demand via append
	// and is reset to [:0] on each request to retain the backing array.
	// No mutex: the context is single-goroutine (see concurrency note above).
	// When len(kvStore) exceeds kvStoreMapThreshold, kvMap is populated and
	// subsequent Set/Get operations use the map for O(1) access.
	kvStore []kvPair
	kvMap   map[string]any

	// queryCache holds the parsed URL query parameters for the current request.
	// Lazily initialized on the first Query/QueryMap call; cleared by reset().
	// Avoids repeated url.ParseQuery calls when a handler reads multiple params.
	queryCache url.Values

	// reference to the app
	app *App

	// pooled is set to true the first time this Ctx is returned to the pool.
	// Used by App.ServeHTTP to distinguish pool hits from fresh allocations.
	pooled bool

	// isClone is true when this Ctx was created by Clone or CloneWithContext.
	isClone bool
}

// reset recycles the context for a new request (used with sync.Pool).
//
// Key allocation avoidances:
//  1. c.rw fields are mutated in-place — no &responseWriter{} heap allocation.
//  2. c.writer is restored to &c.rw — undoes any middleware SetWriter wrapping.
//  3. c.params is re-sliced from the embedded paramsArr — no backing array alloc.
//  4. c.routeKey is cleared with a simple string assignment — no map operation.
//  5. c.kvStore is reset to [:0] — retains the backing array for the next request.
func (c *Ctx) reset(w http.ResponseWriter, r *http.Request) {
	c.req = r

	// Clear debug fields (goroutine ID tracking in debug builds).
	c.debugReset()

	// Update the embedded responseWriter in-place.
	// c.writer already points to &c.rw (set in allocateContext); restoring it here
	// undoes any SetWriter(wrappedWriter) call from the previous request.
	c.rw.ResponseWriter = w
	c.rw.status = http.StatusOK
	c.rw.size = 0
	c.rw.written = false
	c.writer = &c.rw

	// Reset params to the correct backing array:
	// - overflowParams is non-nil when the app has routes with > maxRouteParams
	//   parameters; it was pre-allocated at pool-init time with cap=maxDepth.
	// - Otherwise fall back to the inline paramsArr — zero extra allocation.
	if c.overflowParams != nil {
		c.params = c.overflowParams[:0]
	} else {
		c.params = c.paramsArr[:0]
	}

	c.handlers = nil
	c.index = -1

	// Clear the direct routeKey field.
	c.routeKey = ""

	// Invalidate the query parameter cache so the next Query() call re-parses
	// the new request's URL (url.Values map is GC'd; we don't retain it).
	c.queryCache = nil

	// Retain the kvStore backing array; clear entries to release value references
	// so GC can collect them, then reset length to zero.
	kv := c.kvStore
	for i := range kv {
		kv[i].key = ""
		kv[i].value = nil
	}
	c.kvStore = kv[:0]
	// Clear the map if it was promoted; retain the allocation for reuse.
	for k := range c.kvMap {
		delete(c.kvMap, k)
	}
}



// nopResponseWriter discards all writes. Used by Clone() so that response
// methods called on a cloned Ctx do not affect the real response.
type nopResponseWriter struct{ header http.Header }

func (n *nopResponseWriter) Header() http.Header         { return n.header }
func (n *nopResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (n *nopResponseWriter) WriteHeader(int)             {}
func (n *nopResponseWriter) Status() int                 { return http.StatusOK }
func (n *nopResponseWriter) Size() int                   { return 0 }
func (n *nopResponseWriter) Written() bool               { return false }
func (n *nopResponseWriter) WriteString(s string) (int, error) { return len(s), nil }

// Clone returns a shallow copy of c with an isolated KV store and a nop
// ResponseWriter. The clone shares the same request, params, and route key as
// the original but writes to /dev/null, making it safe to pass to goroutines
// or background tasks that must not touch the live response.
func (c *Ctx) Clone() *Ctx {
	clone := &Ctx{
		req:      c.req,
		app:      c.app,
		routeKey: c.routeKey,
		params:   c.params,
		isClone:  true,
	}
	nop := &nopResponseWriter{header: make(http.Header)}
	clone.rw = responseWriter{}
	clone.writer = nop
	// Deep-copy the KV store so mutations on either side are isolated.
	if len(c.kvStore) > 0 {
		clone.kvStore = make([]kvPair, len(c.kvStore))
		copy(clone.kvStore, c.kvStore)
	}
	if len(c.kvMap) > 0 {
		clone.kvMap = make(map[string]any, len(c.kvMap))
		for k, v := range c.kvMap {
			clone.kvMap[k] = v
		}
	}
	return clone
}

// CloneWithContext is like Clone but replaces the request's context with ctx.
// Use this when passing the clone to a goroutine that needs a different
// cancellation scope.
func (c *Ctx) CloneWithContext(ctx context.Context) *Ctx {
	clone := c.Clone()
	clone.req = clone.req.WithContext(ctx)
	return clone
}

// IsClone reports whether this Ctx was created by Clone or CloneWithContext.
func (c *Ctx) IsClone() bool { return c.isClone }
