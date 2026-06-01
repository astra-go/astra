package router

import "net/http"

// This file defines type aliases that bridge the router package to the parent
// astra package types. The router operates on these aliases internally, while
// the astra.Router adapter converts between astra types and router types at the
// boundary.

// HandlersChain is a slice of HandlerFunc.
type HandlersChain []HandlerFunc

// HandlerFunc is the function signature for HTTP handlers.
// The concrete *Ctx type is defined in the parent astra package.
type HandlerFunc func(c Ctx) error

// Ctx is the request context interface used by the router.
// The router package operates on this interface; the concrete implementation
// lives in the parent astra package.
type Ctx interface {
	// Request returns the underlying *http.Request.
	Request() *http.Request
	// SetHandlers sets the handler chain for this request.
	SetHandlers(handlers HandlersChain)
	// SetParams sets the path parameters for this request.
	SetParams(params Params)
	// SetRouteKey sets the matched route template.
	SetRouteKey(fullPath string)
	// SetAllowedMethods sets the allowed HTTP methods for 405 responses.
	SetAllowedMethods(methods string)
	// Next advances to the next handler in the chain.
	Next() error
}

// Param represents a single URL parameter.
type Param struct {
	Key   string
	Value string
}

// Params is a slice of URL parameters.
type Params []Param

// RouteInfo contains information about a registered route.
type RouteInfo struct {
	Method   string
	Path     string
	FullPath string
}
