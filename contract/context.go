// Package contract defines the core interfaces that decouple Astra middleware
// and sub-packages from the concrete astra.App / astra.Context types.
//
// Middleware and reusable components should depend on contract.Context and
// contract.HandlerFunc rather than on the concrete *astra.Context type.
// This allows them to be unit-tested with a lightweight mock and reused
// outside the Astra framework.
//
// Quick summary of types:
//
//   - ResponseWriter  — http.ResponseWriter + Status/Size/Written helpers
//   - Context         — per-request state: request, response, params, store, chain
//   - HandlerFunc     — func(Context) error
//   - MiddlewareFunc  — alias for HandlerFunc
//   - Router          — minimal route-registration interface (used by health package)
package contract

import (
	"mime/multipart"
	"net/http"
)

// ResponseWriter is an enhanced http.ResponseWriter that tracks the response
// status code, body size, and whether WriteHeader has been called.
type ResponseWriter interface {
	http.ResponseWriter
	// Status returns the HTTP status code that was (or will be) written.
	Status() int
	// Size returns the number of bytes written to the response body so far.
	Size() int
	// Written reports whether WriteHeader has already been called.
	Written() bool
}

// Context is the per-request interface passed to every handler and middleware.
//
// The concrete implementation lives in the astra package (*astra.Ctx).
// Middleware and sub-packages depend only on this interface so they can be
// tested with a lightweight mock without instantiating a full astra.App.
type Context interface {
	// ─── Request / Response ──────────────────────────────────────────────

	// Request returns the underlying *http.Request.
	Request() *http.Request
	// SetRequest replaces the underlying request (used e.g. by tracing middleware
	// to attach a span context).
	SetRequest(r *http.Request)
	// Writer returns the enhanced response writer.
	Writer() ResponseWriter
	// SetWriter replaces the response writer (used e.g. by compress middleware
	// to wrap the writer with a gzip layer).
	SetWriter(w ResponseWriter)

	// ─── Handler chain ───────────────────────────────────────────────────

	// Next executes the next handler in the chain.
	Next()
	// Abort prevents remaining handlers from running.
	Abort()
	// AbortWithStatus calls Abort and writes the given HTTP status code.
	AbortWithStatus(code int)
	// AbortWithError calls Abort, writes the status code, and invokes ErrorHandler.
	AbortWithError(code int, err error)
	// IsAborted reports whether Abort has been called.
	IsAborted() bool

	// ─── Path / Query / Form ─────────────────────────────────────────────

	// Param returns the URL path parameter by name.
	Param(key string) string
	// Query returns the URL query parameter by name.
	Query(key string) string
	// DefaultQuery returns the query parameter or defaultValue if missing.
	DefaultQuery(key, defaultValue string) string
	// QueryMap returns all query parameters as a map.
	QueryMap() map[string]string
	// PostForm returns the form value for the given key.
	PostForm(key string) string
	// DefaultPostForm returns the form value or defaultValue if missing.
	DefaultPostForm(key, defaultValue string) string
	// FormFile returns the file header for the given multipart form key.
	FormFile(key string) (*multipart.FileHeader, error)

	// ─── Binding ─────────────────────────────────────────────────────────

	Bind(obj any) error
	BindJSON(obj any) error
	BindXML(obj any) error
	BindForm(obj any) error
	BindQuery(obj any) error
	BindPath(obj any) error

	ShouldBind(obj any) error
	ShouldBindJSON(obj any) error
	ShouldBindXML(obj any) error
	ShouldBindForm(obj any) error
	ShouldBindQuery(obj any) error
	ShouldBindPath(obj any) error

	MustBind(obj any) error
	MustBindJSON(obj any) error

	Validate(obj any) error

	// ─── Response helpers ─────────────────────────────────────────────────

	JSON(code int, obj any) error
	// JSONStream encodes obj directly into the ResponseWriter without an
	// intermediate buffer.  Content-Length is not set (chunked on HTTP/1.1).
	// Prefer for bulk list responses to eliminate the ~13 KB buffer allocation.
	JSONStream(code int, obj any) error
	XML(code int, obj any) error
	String(code int, format string, values ...any) error
	HTML(code int, html string) error
	Render(code int, name string, data any) error
	Blob(code int, contentType string, data []byte) error
	NoContent(code int) error
	Redirect(code int, location string) error
	File(filepath string) error

	// ─── Headers ─────────────────────────────────────────────────────────

	// SetHeader sets a response header.
	SetHeader(key, value string)
	// Header returns a request header value.
	Header(key string) string
	// ContentType returns the Content-Type of the request.
	ContentType() string

	// ─── Context store ────────────────────────────────────────────────────

	Set(key string, value any)
	Get(key string) (any, bool)
	MustGet(key string) any
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool

	// ─── Client info ──────────────────────────────────────────────────────

	ClientIP() string
	UserAgent() string
	IsWebsocket() bool

	// ─── SSE ──────────────────────────────────────────────────────────────

	SSEvent(event, data string) error

	// ─── HTTP/2 Server Push ───────────────────────────────────────────────

	// Push initiates an HTTP/2 server push for the given target path.
	// Returns http.ErrNotSupported when the underlying connection does not
	// support push (HTTP/1.1, or push disabled by the client).
	//
	// Deprecated: HTTP/2 Server Push is no longer supported by major browsers.
	// Use EarlyHints instead.
	Push(target string, opts *http.PushOptions) error

	// EarlyHints sends a 103 Early Hints interim response that instructs the
	// client to preload the given resource paths.  opts may contain per-resource
	// link attributes such as "as" or "crossorigin".  No-op if headers have
	// already been written.
	EarlyHints(targets []string, opts map[string]string) error
}

// HandlerFunc is the core handler / middleware signature.
// It receives a Context and returns an error (nil = success).
type HandlerFunc func(Context) error

// MiddlewareFunc is an alias for HandlerFunc — middleware is just a handler.
type MiddlewareFunc = HandlerFunc

// Router is the minimal route-registration interface used by sub-packages
// (e.g. health, pprof) that need to register routes on an App without
// importing the concrete *astra.App type.
type Router interface {
	GET(path string, handlers ...HandlerFunc)
	POST(path string, handlers ...HandlerFunc)
}
