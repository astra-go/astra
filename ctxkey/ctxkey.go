// Package ctxkey defines well-known context keys used by Astra for request
// metadata propagation. These are used by middleware.RequestContext and the
// Ctx.RequestContext() method to make per-request values accessible through
// the standard context.Context chain.
//
// Usage:
//
//	requestID, ok := ctx.Value(ctxkey.RequestID).(string)
package ctxkey

// CtxKey is the type used for Astra context keys to prevent collisions with
// keys defined by other packages.
type CtxKey string

func (k CtxKey) String() string { return "astra:" + string(k) }

const (
	// RequestID is the unique request identifier string.
	RequestID CtxKey = "request_id"
	// TraceID is the OpenTelemetry trace ID string (hex-encoded).
	TraceID CtxKey = "trace_id"
	// SpanID is the OpenTelemetry span ID string (hex-encoded).
	SpanID CtxKey = "span_id"
	// RoutePath is the matched route template (e.g. "/users/:id").
	RoutePath CtxKey = "route_path"
	// ClientIP is the resolved client IP address.
	ClientIP CtxKey = "client_ip"
	// UserAgent is the User-Agent header value.
	UserAgent CtxKey = "user_agent"
)
