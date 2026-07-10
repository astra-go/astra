package otel

import (
	"net/http"

	gotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// MiddlewareOption configures the HTTP tracing middleware.
type MiddlewareOption func(*mwConfig)

type mwConfig struct {
	tp         trace.TracerProvider
	prop       propagation.TextMapPropagator
	tracerName string
	filter     func(r *http.Request) bool
}

// WithMiddlewareApp binds the middleware to a specific *App, so spans are
// emitted through that App's TracerProvider and propagator.
func WithMiddlewareApp(a *App) MiddlewareOption {
	return func(c *mwConfig) {
		if a != nil {
			c.tp = a.tp
			c.prop = a.prop
		}
	}
}

// WithMiddlewareTracerProvider overrides the TracerProvider (defaults to the
// global one).
func WithMiddlewareTracerProvider(tp trace.TracerProvider) MiddlewareOption {
	return func(c *mwConfig) { c.tp = tp }
}

// WithMiddlewarePropagator overrides the propagator used to extract the inbound
// W3C trace context (defaults to the global one).
func WithMiddlewarePropagator(p propagation.TextMapPropagator) MiddlewareOption {
	return func(c *mwConfig) { c.prop = p }
}

// WithMiddlewareFilter skips tracing for requests where filter(r) is true
// (e.g. health checks, static assets).
func WithMiddlewareFilter(f func(r *http.Request) bool) MiddlewareOption {
	return func(c *mwConfig) { c.filter = f }
}

func newMWConfig(opts []MiddlewareOption) *mwConfig {
	c := &mwConfig{
		tp:         gotel.GetTracerProvider(),
		prop:       gotel.GetTextMapPropagator(),
		tracerName: "astra/otel/http",
	}
	for _, o := range opts {
		o(c)
	}
	if c.prop == nil {
		c.prop = propagation.TraceContext{}
	}
	return c
}

// HTTPMiddleware returns a net/http middleware that creates a server span for
// every request, records the standard HTTP semantic-convention attributes,
// extracts any inbound W3C trace context so the request continues an existing
// trace, and sets the HTTP status (and error status for >= 400).
//
// Bound version — share one *App across your HTTP/GORM/gRPC integrations:
//
//	app.Use(otelApp.HTTPMiddleware())
//
// Standalone version — uses the global TracerProvider / propagator:
//
//	handler = otel.HTTPMiddleware()(handler)
func (a *App) HTTPMiddleware(opts ...MiddlewareOption) func(http.Handler) http.Handler {
	opts = append([]MiddlewareOption{WithMiddlewareApp(a)}, opts...)
	return HTTPMiddleware(opts...)
}

// HTTPMiddleware is the package-level, App-agnostic counterpart of
// App.HTTPMiddleware.
func HTTPMiddleware(opts ...MiddlewareOption) func(http.Handler) http.Handler {
	c := newMWConfig(opts)
	tracer := c.tp.Tracer(c.tracerName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c.filter != nil && c.filter(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Continue an existing trace if the caller sent W3C headers.
			ctx := c.prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Span name follows the OTel HTTP convention: use the matched
			// route pattern when available (Go 1.23+), otherwise the method.
			spanName := r.Method
			if pattern := r.Pattern; pattern != "" {
				spanName = pattern
			}

			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
			)
			defer span.End()

			// Zero-alloc: borrow a slice from the pool, recycle after the SDK
			// has copied the values into the span.
			attrs := getAttrs()
			attrs.v = append(attrs.v,
				semconv.HTTPRequestMethodKey.String(r.Method),
			)
			if pattern := r.Pattern; pattern != "" {
				attrs.v = append(attrs.v, semconv.HTTPRouteKey.String(pattern))
			}
			attrs.v = append(attrs.v, attribute.String("url.full", r.URL.String()))
			if r.ContentLength > 0 {
				attrs.v = append(attrs.v, semconv.HTTPRequestBodySizeKey.Int64(r.ContentLength))
			}
			span.SetAttributes(attrs.v...)
			putAttrs(attrs)

			rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			r = r.WithContext(ctx)

			next.ServeHTTP(rw, r)

			span.SetAttributes(semconv.HTTPResponseStatusCodeKey.Int(rw.status))
			if rw.size > 0 {
				span.SetAttributes(semconv.HTTPResponseBodySizeKey.Int64(rw.size))
			}
			if rw.status >= http.StatusBadRequest {
				span.SetStatus(codes.Error, http.StatusText(rw.status))
			}
		})
	}
}

// statusWriter captures the response status code and body size without
// allocating on the request path beyond the wrapper itself.
type statusWriter struct {
	http.ResponseWriter
	status int
	size   int64
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.size += int64(n)
	return n, err
}

// Unwrap exposes the underlying ResponseWriter (e.g. for http.Flusher).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
