package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/astra-go/astra"
)

// Timeout returns a middleware that sets a deadline on the request context.
// Downstream handlers that perform context-aware I/O (database queries, HTTP
// calls, etc.) will be cancelled automatically when the deadline is exceeded.
//
// SAFETY NOTE: Running c.Next() inside a goroutine is a data race — the *Context
// may be recycled to the sync.Pool before the goroutine finishes. We therefore
// execute the handler chain synchronously and rely on context propagation to
// cancel blocking I/O. Use context-aware libraries for full effect.
func Timeout(duration time.Duration) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), duration)
		defer cancel()

		// Propagate the deadline so downstream context-aware handlers cancel.
		c.SetRequest(c.Request().WithContext(ctx))

		// Execute handler chain synchronously (no goroutine).
		c.Next()

		// After the chain returns, check whether the timeout was exceeded.
		if ctx.Err() == context.DeadlineExceeded {
			if !c.Writer().Written() {
				return astra.NewHTTPError(http.StatusGatewayTimeout, "request timeout")
			}
			// Response already started; abort to prevent further writes.
			c.Abort()
		}

		return nil
	}
}
