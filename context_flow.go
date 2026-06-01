package astra

import "github.com/astra-go/astra/router"

// context_flow.go — handler-chain flow control methods for Ctx.
//
// These methods let middleware and handlers cooperate within the chain:
//   - Next() advances to the next handler (call this at the end of a middleware).
//   - Abort() and its variants stop the chain immediately, skipping all remaining
//     handlers. Middleware that called Next() before the abort will still run its
//     deferred post-handler code (like timing or logging finalization).

import "math"

// abortIndex is the sentinel value assigned to Ctx.index when Abort is called.
// Any value ≥ abortIndex means the chain was stopped.  int16 max = 32767.
const abortIndex int16 = math.MaxInt16

// Next executes the next handler in the chain.
// It is safe to call on an already-aborted context — the call is a no-op.
// Returns the first non-nil error returned by a handler, or nil if all
// handlers completed without error.  The error is also passed to
// ErrorHandler so the framework always records the error even if the
// caller does not check the return value.
func (c *Ctx) Next() (err error) {
	c.index++
	for c.index < int16(len(c.handlers)) {
		err = c.handlers[c.index](c)
		if err != nil {
			c.app.options.ErrorHandler(c, err)
			return
		}
		// Guard against int16 overflow when a handler called Abort() (sets
		// c.index = abortIndex = math.MaxInt16). Incrementing MaxInt16 wraps
		// to MinInt16 which would satisfy the loop condition and cause a panic.
		if c.IsAborted() {
			return
		}
		c.index++
	}
	return nil
}

// Abort prevents remaining handlers from being called.
func (c *Ctx) Abort() {
	c.index = abortIndex
}

// AbortWithStatus calls Abort and writes the given status code.
func (c *Ctx) AbortWithStatus(code int) {
	c.writer.WriteHeader(code)
	c.Abort()
}

// AbortWithError calls Abort, writes the status code, and delegates to ErrorHandler.
func (c *Ctx) AbortWithError(code int, err error) {
	c.index = abortIndex
	c.app.options.ErrorHandler(c, NewHTTPError(code, err.Error()).WithInternal(err))
}

// IsAborted returns true if the current context has been aborted.
func (c *Ctx) IsAborted() bool {
	return c.index >= abortIndex
}

// SetHandlers sets the handler chain for this request.
// Called by the router during request dispatch.
func (c *Ctx) SetHandlers(handlers router.HandlersChain) {
	// Convert router.HandlersChain to astra.HandlersChain
	c.handlers = make(HandlersChain, len(handlers))
	for i, h := range handlers {
		c.handlers[i] = func(ctx *Ctx) error {
			return h(ctx)
		}
	}
	c.index = -1
}

// SetParams sets the path parameters for this request.
// Called by the router during request dispatch.
func (c *Ctx) SetParams(params router.Params) {
	// Convert router.Params to astra.Params
	c.params = make(Params, len(params))
	for i, p := range params {
		c.params[i] = Param{Key: p.Key, Value: p.Value}
	}
}

// SetRouteKey sets the matched route template.
// Called by the router during request dispatch.
func (c *Ctx) SetRouteKey(fullPath string) {
	c.routeKey = fullPath
}
