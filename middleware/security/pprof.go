// Package middleware provides HTTP middleware for the Astra web framework.
//
// Pprof registers the standard net/http/pprof debug endpoints on the given Router.
//
// # Usage (recommended: behind an IP-filter middleware)
//
//	middleware.RegisterPprof(app)
//	// endpoints: GET /debug/pprof/, /debug/pprof/cmdline, /debug/pprof/profile,
//	//            /debug/pprof/symbol, /debug/pprof/trace, /debug/pprof/{name}
//
// # Custom prefix
//
//	middleware.RegisterPprof(app, middleware.PprofWithPrefix("/internal/debug"))
//
// # Production hardening (restrict to internal IPs)
//
//	middleware.RegisterPprof(app,
//	    middleware.PprofWithPrefix("/debug"),
//	    middleware.PprofWithMiddleware(middleware.IPAllowList("127.0.0.1/8", "10.0.0.0/8")),
//	)

package security

import (
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/astra-go/astra"
)

// pprofRouter is the minimal router interface needed by RegisterPprof.
type pprofRouter interface {
	GET(path string, handlers ...astra.HandlerFunc)
	POST(path string, handlers ...astra.HandlerFunc)
}

// pprofOptions configures Pprof registration.
type pprofOptions struct {
	prefix     string
	middleware []astra.HandlerFunc
}

// PprofOption configures RegisterPprof.
type PprofOption func(*pprofOptions)

// PprofWithPrefix overrides the URL prefix (default: "/debug/pprof").
func PprofWithPrefix(prefix string) PprofOption {
	return func(o *pprofOptions) {
		o.prefix = strings.TrimRight(prefix, "/")
	}
}

// PprofWithMiddleware adds middleware that is applied to every pprof route.
// Useful for IP allow-listing in production.
func PprofWithMiddleware(mw ...astra.HandlerFunc) PprofOption {
	return func(o *pprofOptions) {
		o.middleware = append(o.middleware, mw...)
	}
}

// RegisterPprof mounts the standard net/http/pprof endpoints on the given Router.
// Defaults to the "/debug/pprof" prefix.
// The router parameter is typically *astra.App.
func RegisterPprof(r pprofRouter, opts ...PprofOption) {
	o := &pprofOptions{prefix: "/debug/pprof"}
	for _, opt := range opts {
		opt(o)
	}

	// chain prepends the configured middleware before each pprof handler so that
	// e.g. an IP allow-list is enforced on every route without needing Group.
	chain := func(h astra.HandlerFunc) []astra.HandlerFunc {
		handlers := make([]astra.HandlerFunc, len(o.middleware)+1)
		copy(handlers, o.middleware)
		handlers[len(o.middleware)] = h
		return handlers
	}

	wrap := func(h http.HandlerFunc) astra.HandlerFunc {
		return func(c *astra.Ctx) error {
			h(c.Writer(), c.Request())
			return nil
		}
	}

	p := o.prefix

	r.GET(p+"/", chain(wrap(pprof.Index))...)
	r.GET(p, chain(wrap(pprof.Index))...)
	r.GET(p+"/cmdline", chain(wrap(pprof.Cmdline))...)
	r.GET(p+"/profile", chain(wrap(pprof.Profile))...)
	r.GET(p+"/symbol", chain(wrap(pprof.Symbol))...)
	r.POST(p+"/symbol", chain(wrap(pprof.Symbol))...)
	r.GET(p+"/trace", chain(wrap(pprof.Trace))...)

	r.GET(p+"/:name", chain(func(c *astra.Ctx) error {
		pprof.Handler(c.Param("name")).ServeHTTP(c.Writer(), c.Request())
		return nil
	})...)
}
