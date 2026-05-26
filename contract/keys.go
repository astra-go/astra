package contract

// Context store keys used by the framework core.  Using named constants
// instead of raw string literals eliminates the implicit coupling between the
// router, middleware, and any code that reads these values from the context.

const (
	// RouteKey is the context store key under which the Router records the
	// matched route template (e.g. "/users/:id").
	//
	// Middleware that needs a low-cardinality path label — such as Prometheus
	// metrics or OpenTelemetry tracing — should read this key rather than the
	// raw request path to avoid label-cardinality explosion.
	//
	// Written by: router.go (astra package)
	// Read by:    middleware/metrics.go, middleware/tracing.go
	RouteKey = "astra.route"
)
