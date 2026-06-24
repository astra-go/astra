package middleware

import (
	"github.com/astra-go/astra"
)

// RequestContext returns a middleware that enriches the request context chain
// with standard Astra observability metadata (request ID, client IP, user agent,
// and eventually trace context).
//
// This middleware should be one of the first in the chain — before any middleware
// or handler that calls c.RequestContext() and expects metadata to be present.
//
// Effect:
//   - Injects or preserves X-Request-ID header + Ctx store value ("requestID")
//   - Sets "clientIP" and "userAgent" in the Ctx store
//   - After this middleware, c.RequestContext() returns a context that includes
//     all of the above values (request_id, client_ip, user_agent, route_path)
//
// Example:
//
//	app.Use(middleware.RequestContext())
//	app.Use(middleware.RequestID())       // optional: custom ID format
//	app.Use(middleware.RequestMetrics())   // log/record after context is ready
func RequestContext() astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		// Ensure a request ID exists.
		id := c.Header(requestIDKey)
		if id == "" {
			id = generateID()
			c.SetHeader(requestIDKey, id)
		}
		c.Set("requestID", id)

		// Store client IP and user agent for context propagation.
		c.Set("clientIP", c.ClientIP())
		c.Set("userAgent", c.UserAgent())

		// Metadata is now available via c.RequestContext().
		c.Next()
		return nil
	}
}
