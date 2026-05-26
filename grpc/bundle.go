// bundle.go — opinionated interceptor bundles for Astra gRPC servers.
//
// ServerBundle eliminates the boilerplate of wiring individual interceptors in
// the correct order. Instead of registering 6–8 interceptors by hand (and
// forgetting the stream-side counterparts), callers configure a single struct
// and call Apply():
//
//	bundle := grpcserver.MeshServerBundle()
//	bundle.Auth = &grpcserver.AuthBundle{
//	    Extractor: grpcserver.BearerTokenExtractor(),
//	    Validator: myJWT.Validate,
//	    Skippers:  []grpcserver.AuthSkipper{grpcserver.SkipHealthCheck()},
//	}
//	s := grpcserver.New(app, bundle.Apply()...)
//
// # Interceptor execution order (fixed)
//
//	Recovery → Tracing → Metrics → Logger → Auth → RateLimit → Propagation
//
// Recovery is always outermost so panics in every subsequent interceptor are
// caught. Tracing wraps Metrics so the span covers the full measured window.
// Auth runs before RateLimit so unauthenticated requests are rejected cheaply.
// Propagation is innermost so forwarded headers are available to the handler.
//
// Both unary and stream interceptors are registered in the same order, so
// Apply() always returns a matched pair — callers cannot accidentally omit the
// stream side.
package grpcserver

import (
	"time"

	"google.golang.org/grpc"
)

// ─── AuthBundle ───────────────────────────────────────────────────────────────

// AuthBundle configures the authentication interceptors inside a ServerBundle.
type AuthBundle struct {
	// Extractor pulls the bearer token (or other credential) from the context.
	// Use BearerTokenExtractor() for standard Authorization: Bearer <token> headers.
	Extractor TokenExtractor
	// Validator verifies the token and returns an enriched context.
	// Inject auth claims via context.WithValue so handlers can read them.
	Validator TokenValidator
	// Skippers bypass authentication for specific methods.
	// Use SkipHealthCheck() to allow unauthenticated liveness/readiness probes.
	Skippers []AuthSkipper
}

// ─── RateLimitBundle ─────────────────────────────────────────────────────────

// RateLimitBundle configures the per-client rate-limit interceptors inside a
// ServerBundle. Clients are identified by their peer IP address.
type RateLimitBundle struct {
	// Rate is the steady-state token refill rate (tokens per second).
	Rate float64
	// Burst is the maximum burst size (tokens available at start).
	Burst int
}

// ─── ServerBundle ─────────────────────────────────────────────────────────────

// ServerBundle is an opinionated set of server-side interceptors.
// Zero values are safe: every field is opt-in except Recovery and Tracing,
// which default to true in the preset constructors.
//
// Modify the returned bundle before calling Apply():
//
//	bundle := grpcserver.DefaultServerBundle()
//	bundle.RateLimit = &grpcserver.RateLimitBundle{Rate: 200, Burst: 50}
//	s := grpcserver.New(app, bundle.Apply()...)
type ServerBundle struct {
	// Recovery wraps every interceptor with panic recovery.
	// Strongly recommended in production. Default: true in preset constructors.
	Recovery bool
	// Tracing injects OTel server-side span creation and W3C TraceContext
	// extraction. Requires a global TracerProvider to be configured.
	// Default: true in preset constructors.
	Tracing bool
	// Metrics records OTel request counters, latency histograms, and in-flight
	// gauges. Zero-value GRPCMetricsConfig uses the global MeterProvider.
	Metrics *GRPCMetricsConfig
	// Logger emits a structured slog line per RPC with method and latency.
	Logger bool
	// Auth configures token-based authentication. nil = no authentication.
	Auth *AuthBundle
	// RateLimit configures per-client IP token-bucket rate limiting.
	// nil = no rate limiting.
	RateLimit *RateLimitBundle
	// Propagation lists metadata keys to forward from incoming to outgoing
	// context (service-mesh header forwarding). nil = no forwarding.
	// Use IstioHeaders, W3CHeaders, or EnvoyHeaders as starting points.
	Propagation []string
	// Timeout enforces a per-call server-side deadline.
	// 0 = no timeout (default). Mirrors WithTimeout on the server.
	Timeout time.Duration
}

// DefaultServerBundle returns a bundle suitable for most services:
// recovery + tracing + metrics + W3C header propagation.
// No authentication or rate limiting — add those fields as needed.
func DefaultServerBundle() ServerBundle {
	cfg := GRPCMetricsConfig{}
	return ServerBundle{
		Recovery:    true,
		Tracing:     true,
		Metrics:     &cfg,
		Propagation: W3CHeaders,
	}
}

// MeshServerBundle returns a bundle tuned for Istio / Envoy service mesh
// deployments: recovery + tracing + metrics + Istio B3 + W3C header forwarding.
// Add Auth and RateLimit fields for services that need them.
func MeshServerBundle() ServerBundle {
	cfg := GRPCMetricsConfig{}
	keys := make([]string, 0, len(IstioHeaders)+len(W3CHeaders))
	keys = append(keys, IstioHeaders...)
	keys = append(keys, W3CHeaders...)
	return ServerBundle{
		Recovery:    true,
		Tracing:     true,
		Metrics:     &cfg,
		Propagation: keys,
	}
}

// Apply converts the bundle into a slice of server Options ready to pass to New.
//
// Interceptors are registered in the canonical order:
//
//	Recovery → Tracing → Metrics → Logger → Auth → RateLimit → Propagation
//
// Both unary and stream interceptors are always registered together, so the
// stream side can never be accidentally omitted.
func (b ServerBundle) Apply() []Option {
	var unary []grpc.UnaryServerInterceptor
	var stream []grpc.StreamServerInterceptor

	if b.Recovery {
		unary = append(unary, UnaryInterceptorRecovery())
		stream = append(stream, StreamInterceptorRecovery())
	}

	if b.Tracing {
		unary = append(unary, UnaryInterceptorTracing())
		stream = append(stream, StreamInterceptorTracing())
	}

	if b.Metrics != nil {
		unary = append(unary, UnaryInterceptorMetrics(*b.Metrics))
		stream = append(stream, StreamInterceptorMetrics(*b.Metrics))
	}

	if b.Logger {
		unary = append(unary, UnaryInterceptorLogger())
		stream = append(stream, StreamInterceptorLogger())
	}

	if b.Auth != nil {
		unary = append(unary, UnaryInterceptorAuth(b.Auth.Extractor, b.Auth.Validator, b.Auth.Skippers...))
		stream = append(stream, StreamInterceptorAuth(b.Auth.Extractor, b.Auth.Validator, b.Auth.Skippers...))
	}

	if b.RateLimit != nil {
		unary = append(unary, UnaryInterceptorRateLimit(b.RateLimit.Rate, b.RateLimit.Burst))
		stream = append(stream, StreamInterceptorRateLimit(b.RateLimit.Rate, b.RateLimit.Burst))
	}

	if len(b.Propagation) > 0 {
		unary = append(unary, UnaryInterceptorMetadataForwarding(b.Propagation...))
		stream = append(stream, StreamInterceptorMetadataForwarding(b.Propagation...))
	}

	var opts []Option
	if len(unary) > 0 {
		opts = append(opts, WithUnaryInterceptors(unary...))
	}
	if len(stream) > 0 {
		opts = append(opts, WithStreamInterceptors(stream...))
	}
	if b.Timeout > 0 {
		opts = append(opts, WithTimeout(b.Timeout))
	}
	return opts
}
