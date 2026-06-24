package astra

import (
	"context"

	"github.com/astra-go/astra/ctxkey"
)

// RequestContext returns a context.Context that inherits from the underlying
// HTTP request's context and includes Astra per-request metadata stored in the
// Ctx key-value store for the well-known keys defined in ctxkey.
//
// Use this when you need to pass a context.Context to a downstream call
// (database query, gRPC call, etc.) and want the Astra metadata (request ID,
// trace ID, client IP, etc.) to propagate automatically.
//
// Compare with c.Request().Context(), which returns the raw HTTP request
// context without Astra metadata.
//
// Example:
//
//	func MyHandler(c *astra.Ctx) error {
//	    result, err := db.Query(c.RequestContext(), "SELECT ...")
//	    //                                    ^^^^^^^^^^^^^^^^
//	    // The database call receives the ASTRA-enhanced context that
//	    // includes request_id, trace_id, client_ip, etc.
//	}
func (c *Ctx) RequestContext() context.Context {
	ctx := c.Request().Context()

	// Propagate well-known keys from the Ctx store into the context chain.
	for _, kv := range c.kvStore {
		ck := contextKeyFromStoreKey(kv.key)
		if ck != nil {
			ctx = context.WithValue(ctx, *ck, kv.value)
		}
	}

	// Also propagate routeKey if set.
	if c.routeKey != "" {
		ctx = context.WithValue(ctx, ctxkey.RoutePath, c.routeKey)
	}

	return ctx
}

// contextKeyFromStoreKey maps a Ctx kvStore key name to the corresponding
// ctxkey.CtxKey. Returns nil if the key is not in the well-known set.
func contextKeyFromStoreKey(key string) *ctxkey.CtxKey {
	switch key {
	case "requestID", "X-Request-ID":
		return ptr(ctxkey.RequestID)
	case "traceID":
		return ptr(ctxkey.TraceID)
	case "spanID":
		return ptr(ctxkey.SpanID)
	case "clientIP":
		return ptr(ctxkey.ClientIP)
	case "userAgent":
		return ptr(ctxkey.UserAgent)
	case RouteKey: // "astra.route"
		return ptr(ctxkey.RoutePath)
	default:
		return nil
	}
}

func ptr[T any](v T) *T { return &v }
