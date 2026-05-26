// Package observability provides OpenTelemetry tracing and Prometheus metrics
// middleware for the Astra web framework.
//
// Import this sub-module only when you need distributed tracing or metrics
// instrumentation. For basic middleware (Recovery, CORS, Logger, etc.) use
// github.com/astra-go/astra/middleware directly — it has no heavy dependencies.
//
// # Tracing
//
//	import obs "github.com/astra-go/astra/middleware/observability"
//
//	app.Use(obs.Tracing())
//	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
//	    SpanExtractor: obs.OTelSpanExtractor(),
//	}))
//
// # Metrics
//
//	app.Use(obs.Metrics())
//	app.GET("/metrics", obs.MetricsHandler())
package observability

import (
	"net/http"
	"net/url"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/contract"
	isani "github.com/astra-go/astra/internal/sanitize"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// SpanExtractorIface extracts distributed-tracing identifiers from an HTTP
// request. It has the same method set as middleware.SpanExtractor so that
// values returned by OTelSpanExtractor() satisfy that interface without
// creating an import cycle between the two packages.
type SpanExtractorIface interface {
	TraceID(r *http.Request) string
	SpanID(r *http.Request) string
}

// TracingConfig configures the OpenTelemetry tracing middleware.
type TracingConfig struct {
	// TracerName is the instrumentation library name. Default: "astra".
	TracerName string
	// TracerProvider to use. Defaults to otel.GetTracerProvider().
	TracerProvider trace.TracerProvider
	// Propagator extracts/injects trace context. Defaults to otel.GetTextMapPropagator().
	Propagator propagation.TextMapPropagator
	// SkipPaths are paths that should not be traced (e.g. "/healthz", "/metrics").
	SkipPaths []string
	// SpanNameFormatter returns the span name for a given method and path.
	SpanNameFormatter func(method, path string) string
	// RedactQueryParams lists query-parameter names whose values are replaced with
	// "REDACTED" in span attributes. Protects tokens, passwords, API keys, etc.
	// Default: see isani.DefaultSensitiveParams.
	RedactQueryParams []string
	// IncludeQuery controls whether query parameters appear in spans at all.
	// When false (default), the raw query string is omitted from span attributes,
	// preventing accidental leakage of credentials in URL query strings.
	IncludeQuery bool
}

// SpanKey is the context store key under which the Tracing middleware stores the
// active OpenTelemetry span. Downstream handlers retrieve it via SpanFromContext.
const SpanKey = "otel.span"

// Tracing returns a middleware that creates an OpenTelemetry span per request.
//
// Security — URL sanitization:
// Query parameters are excluded from span attributes by default (IncludeQuery:false)
// to prevent credentials (e.g. ?token=…, ?password=…) from appearing in trace
// backends. Enable IncludeQuery:true only when you have audited your query params
// and added sensitive names to RedactQueryParams.
//
// Prerequisites: call otel.SetTracerProvider and otel.SetTextMapPropagator before
// using this middleware (or use the otel.Setup helper from this module).
func Tracing(opts ...func(*TracingConfig)) astra.HandlerFunc {
	cfg := &TracingConfig{
		TracerName: "astra",
		SpanNameFormatter: func(method, path string) string {
			return method + " " + path
		},
		IncludeQuery:      false,
		RedactQueryParams: isani.DefaultSensitiveParams,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	tp := cfg.TracerProvider
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	tracer := tp.Tracer(cfg.TracerName)

	prop := cfg.Propagator
	if prop == nil {
		prop = otel.GetTextMapPropagator()
	}

	skip := make(map[string]bool, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skip[p] = true
	}

	sensitiveSet := isani.BuildSet(cfg.RedactQueryParams)

	return func(c *astra.Ctx) error {
		path := c.Request().URL.Path
		if skip[path] {
			return nil
		}

		ctx := prop.Extract(c.Request().Context(), propagation.HeaderCarrier(c.Request().Header))

		attrs := []attribute.KeyValue{
			semconv.HTTPMethodKey.String(c.Request().Method),
			semconv.HTTPTargetKey.String(sanitizedTarget(c.Request(), cfg, sensitiveSet)),
			semconv.HTTPSchemeKey.String(scheme(c.Request())),
			attribute.String("http.user_agent", c.UserAgent()),
			semconv.NetHostNameKey.String(c.Request().Host),
			attribute.String("http.client_ip", c.ClientIP()),
		}

		spanName := cfg.SpanNameFormatter(c.Request().Method, path)
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		c.SetRequest(c.Request().WithContext(ctx))
		c.Set(SpanKey, span)

		prop.Inject(ctx, propagation.HeaderCarrier(c.Writer().Header()))

		c.Next()

		if routeTemplate := c.GetString(contract.RouteKey); routeTemplate != "" {
			span.SetName(cfg.SpanNameFormatter(c.Request().Method, routeTemplate))
		}

		statusCode := c.Writer().Status()
		span.SetAttributes(
			semconv.HTTPStatusCodeKey.Int(statusCode),
			attribute.Int("http.response_content_length", c.Writer().Size()),
		)

		switch {
		case statusCode >= 500:
			span.SetStatus(codes.Error, http.StatusText(statusCode))
		default:
			span.SetStatus(codes.Unset, "")
		}

		return nil
	}
}

// sanitizedTarget returns the request target with sensitive query-parameter
// values redacted.
func sanitizedTarget(r *http.Request, cfg *TracingConfig, sensitiveSet map[string]bool) string {
	if !cfg.IncludeQuery || r.URL.RawQuery == "" {
		return r.URL.Path
	}
	q := r.URL.Query()
	sanitized := url.URL{Path: r.URL.Path, RawQuery: isani.RedactQuery(q, sensitiveSet).Encode()}
	return sanitized.RequestURI()
}

// SpanFromContext retrieves the OTel span stored by the Tracing middleware.
// Falls back to the span from the request context if none was stored explicitly.
func SpanFromContext(c *astra.Ctx) trace.Span {
	if v, ok := c.Get(SpanKey); ok {
		if span, ok := v.(trace.Span); ok {
			return span
		}
	}
	return trace.SpanFromContext(c.Request().Context())
}

// ─── OTel → SpanExtractor bridge ─────────────────────────────────────────────

type otelSpanExtractor struct{}

func (e *otelSpanExtractor) TraceID(r *http.Request) string {
	if sc := trace.SpanFromContext(r.Context()).SpanContext(); sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}

func (e *otelSpanExtractor) SpanID(r *http.Request) string {
	if sc := trace.SpanFromContext(r.Context()).SpanContext(); sc.IsValid() {
		return sc.SpanID().String()
	}
	return ""
}

// OTelSpanExtractor returns a SpanExtractorIface that reads trace_id and
// span_id from the OpenTelemetry span in the request context.
//
// SpanExtractorIface has the same method set as middleware.SpanExtractor, so
// the return value can be assigned directly to LoggerConfig.SpanExtractor:
//
//	app.Use(obs.Tracing())
//	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
//	    SpanExtractor: obs.OTelSpanExtractor(),
//	}))
func OTelSpanExtractor() SpanExtractorIface {
	return &otelSpanExtractor{}
}

// ─── Option helpers ───────────────────────────────────────────────────────────

func WithTracerName(name string) func(*TracingConfig) {
	return func(c *TracingConfig) { c.TracerName = name }
}

func WithTracingSkipPaths(paths ...string) func(*TracingConfig) {
	return func(c *TracingConfig) { c.SkipPaths = paths }
}

func WithSpanNameFormatter(fn func(method, path string) string) func(*TracingConfig) {
	return func(c *TracingConfig) { c.SpanNameFormatter = fn }
}

func WithTracingIncludeQuery(include bool) func(*TracingConfig) {
	return func(c *TracingConfig) { c.IncludeQuery = include }
}

func WithTracingRedactParams(params ...string) func(*TracingConfig) {
	return func(c *TracingConfig) { c.RedactQueryParams = params }
}

func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}
