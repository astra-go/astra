// Package astra - HTTP route integration for mq module.
//
// This file implements mq.HTTPRouteRegistrar using *App and wires up the
// HTTP → MQ publishing routes.
package astra

import (
	"context"
	"net/http"

	"github.com/astra-go/astra/mq"
)

// astraMQRouteRegistrar implements mq.HTTPRouteRegistrar using *App.
// Named with MQ prefix to distinguish it from astra.RouteRegistrar (app.go).
type astraMQRouteRegistrar struct {
	app *App
}

// RegisterMQHTTPRoutes registers HTTP routes for publishing messages via MQ.
//
// Usage:
//
//	producer := ... // create mq.Producer
//	opts := mq.HTTPRouteOptions{
//	    Producer: producer,
//	    Prefix:   "/_mq",
//	    EnableDelay: true,
//	}
//	app.RegisterMQHTTPRoutes(opts)
func (app *App) RegisterMQHTTPRoutes(opts mq.HTTPRouteOptions) {
	registrar := &astraMQRouteRegistrar{app: app}
	mq.RegisterHTTPRoutes(registrar, opts)
}

// GET implements mq.HTTPRouteRegistrar.
func (r *astraMQRouteRegistrar) GET(path string, handler func(ctx context.Context, r *http.Request) error) {
	r.app.GET(path, func(c *Ctx) error {
		return handler(c.Request().Context(), c.Request())
	})
}

// POST implements mq.HTTPRouteRegistrar.
func (r *astraMQRouteRegistrar) POST(path string, handler func(ctx context.Context, r *http.Request) error) {
	r.app.POST(path, func(c *Ctx) error {
		return handler(c.Request().Context(), c.Request())
	})
}

// PUT implements mq.HTTPRouteRegistrar.
func (r *astraMQRouteRegistrar) PUT(path string, handler func(ctx context.Context, r *http.Request) error) {
	r.app.PUT(path, func(c *Ctx) error {
		return handler(c.Request().Context(), c.Request())
	})
}

// DELETE implements mq.HTTPRouteRegistrar.
func (r *astraMQRouteRegistrar) DELETE(path string, handler func(ctx context.Context, r *http.Request) error) {
	r.app.DELETE(path, func(c *Ctx) error {
		return handler(c.Request().Context(), c.Request())
	})
}

// Any implements mq.HTTPRouteRegistrar.
func (r *astraMQRouteRegistrar) Any(path string, handler func(ctx context.Context, r *http.Request) error) {
	r.app.Any(path, func(c *Ctx) error {
		return handler(c.Request().Context(), c.Request())
	})
}
