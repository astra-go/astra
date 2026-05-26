// Package otel provides one-call OpenTelemetry SDK initialisation for Astra
// applications, wiring together:
//
//   - A TracerProvider that exports spans via OTLP/gRPC (Jaeger ≥ 1.35,
//     Grafana Tempo, OpenTelemetry Collector, …)
//   - A MeterProvider that exposes metrics via the Prometheus pull model
//   - W3C TraceContext + Baggage propagators registered globally
//
// # Quick start
//
//	shutdown, err := otel.Setup(ctx, otel.Config{
//	    ServiceName: "my-svc",
//	    OTLPEndpoint: "localhost:4317",  // Jaeger / Collector gRPC port
//	})
//	if err != nil { log.Fatal(err) }
//	defer shutdown(context.Background())
//
//	// Prometheus metrics are served on the existing /metrics endpoint;
//	// no extra configuration needed.
//
// # Jaeger
//
// Configure Jaeger ≥ 1.35 with its OTLP receiver (enabled by default) and
// point OTLPEndpoint at port 4317.  The deprecated Jaeger Thrift exporter is
// NOT used here; OTLP is the current recommendation from the Jaeger project.
//
// # Development / stdout
//
//	otel.Setup(ctx, otel.Config{
//	    ServiceName: "dev",
//	    EnableStdout: true,
//	})
package otel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	gotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Config holds the OTel initialisation options.
type Config struct {
	// ServiceName is the logical name of this service (required).
	// Used as the "service.name" resource attribute.
	ServiceName string
	// ServiceVersion is the semantic version of this service (e.g. "1.2.3").
	ServiceVersion string
	// ServiceNamespace groups related services (e.g. "payments").
	ServiceNamespace string

	// OTLPEndpoint is the gRPC endpoint of the OTLP receiver, e.g. "localhost:4317".
	// Leave empty to disable OTLP export (use EnableStdout for dev).
	OTLPEndpoint string
	// Insecure disables TLS for the OTLP gRPC connection (useful in dev/test).
	Insecure bool
	// DialTimeout is the connection timeout for the OTLP exporter. Default: 5s.
	DialTimeout time.Duration

	// SampleRatio controls head-based sampling: 0.0 = never, 1.0 = always.
	// Default: 1.0 (sample all spans).
	SampleRatio float64

	// EnableStdout additionally exports spans to stdout (pretty-printed JSON).
	// Useful during development.
	EnableStdout bool

	// PrometheusRegisterer is the Prometheus registry to register the OTel
	// metrics bridge with. If nil, uses prometheus.DefaultRegisterer.
	// Pass prometheus.NewRegistry() in tests to avoid cross-test metric conflicts.
	PrometheusRegisterer prometheus.Registerer
}

// Setup initialises the global OTel TracerProvider and MeterProvider, registers
// W3C propagators, and returns a shutdown function.
//
// The shutdown function MUST be called before the process exits to ensure all
// pending spans and metrics are flushed.
//
//	shutdown, err := otel.Setup(ctx, cfg)
//	if err != nil { ... }
//	defer shutdown(context.Background())
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("otel: ServiceName is required")
	}
	if cfg.SampleRatio == 0 && !cfg.EnableStdout && cfg.OTLPEndpoint == "" {
		// All exporters disabled — nothing to do but set up the no-op provider.
		cfg.SampleRatio = 1.0
	}
	if cfg.SampleRatio <= 0 {
		cfg.SampleRatio = 1.0
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}

	// ── Resource ─────────────────────────────────────────────────────────────
	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("otel: build resource: %w", err)
	}

	// ── Trace exporters ───────────────────────────────────────────────────────
	var shutdowns []func(context.Context) error
	var spanExporters []sdktrace.SpanExporter

	if cfg.OTLPEndpoint != "" {
		exp, err := newOTLPExporter(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("otel: OTLP exporter: %w", err)
		}
		spanExporters = append(spanExporters, exp)
		shutdowns = append(shutdowns, exp.Shutdown)
	}

	if cfg.EnableStdout {
		exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("otel: stdout exporter: %w", err)
		}
		spanExporters = append(spanExporters, exp)
		shutdowns = append(shutdowns, exp.Shutdown)
	}

	// ── TracerProvider ────────────────────────────────────────────────────────
	tp := buildTracerProvider(res, spanExporters, cfg.SampleRatio)
	gotel.SetTracerProvider(tp)
	shutdowns = append(shutdowns, tp.Shutdown)

	// ── MeterProvider + Prometheus bridge ─────────────────────────────────────
	mp, err := buildMeterProvider(res, cfg.PrometheusRegisterer)
	if err != nil {
		_ = chainShutdown(ctx, shutdowns)
		return nil, fmt.Errorf("otel: meter provider: %w", err)
	}
	gotel.SetMeterProvider(mp)
	shutdowns = append(shutdowns, mp.Shutdown)

	// ── Propagators ───────────────────────────────────────────────────────────
	// Register W3C TraceContext and Baggage propagators.
	gotel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(ctx context.Context) error {
		return chainShutdown(ctx, shutdowns)
	}, nil
}

// ─── Internal builders ────────────────────────────────────────────────────────

func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	// Do not set an explicit SchemaURL at the resource level: resource.WithTelemetrySDK()
	// returns a resource whose schema URL matches the current SDK version, and mixing
	// an explicit semconv-versioned URL with it causes resource.ErrSchemaURLConflict
	// when the SDK is upgraded by workspace MVS resolution.  Each attribute already
	// carries its semantic-convention identity via its key name.
	r, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
		resource.WithProcessPID(),
		resource.WithHost(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		// ErrPartialResource is returned when some detectors succeed and others
		// partially fail (e.g. host detection on a sandboxed environment).
		// The partial resource is still usable.
		if errors.Is(err, resource.ErrPartialResource) {
			return r, nil
		}
		return nil, err
	}
	return r, nil
}

func newOTLPExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	dialCtx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithDialOption(grpc.WithBlock()),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	exp, err := otlptracegrpc.New(dialCtx, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", cfg.OTLPEndpoint, err)
	}
	return exp, nil
}

func buildTracerProvider(res *resource.Resource, exporters []sdktrace.SpanExporter, sampleRatio float64) *sdktrace.TracerProvider {
	var sampler sdktrace.Sampler
	switch {
	case sampleRatio >= 1.0:
		sampler = sdktrace.AlwaysSample()
	case sampleRatio <= 0.0:
		sampler = sdktrace.NeverSample()
	default:
		sampler = sdktrace.TraceIDRatioBased(sampleRatio)
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sampler)),
	}
	for _, exp := range exporters {
		opts = append(opts, sdktrace.WithBatcher(exp))
	}
	return sdktrace.NewTracerProvider(opts...)
}

func buildMeterProvider(res *resource.Resource, reg prometheus.Registerer) (*sdkmetric.MeterProvider, error) {
	// The Prometheus bridge registers itself with the provided registry
	// (or prometheus.DefaultRegisterer if nil) and serves metrics via promhttp.Handler.
	opts := []promexporter.Option{}
	if reg != nil {
		opts = append(opts, promexporter.WithRegisterer(reg))
	}
	promExp, err := promexporter.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("prometheus exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExp),
	)
	return mp, nil
}

// chainShutdown calls all shutdown functions and returns the first error.
func chainShutdown(ctx context.Context, fns []func(context.Context) error) error {
	var firstErr error
	for _, fn := range fns {
		if err := fn(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ─── Log-correlation helpers ──────────────────────────────────────────────────

// TraceIDFromContext returns the trace ID of the active span in ctx as a
// 32-character lowercase hex string, or "" if there is no active sampled span.
//
// Inject this into log records to correlate logs with distributed traces:
//
//	slog.Info("payment processed",
//	    slog.String("trace_id", otel.TraceIDFromContext(ctx)),
//	    slog.String("span_id",  otel.SpanIDFromContext(ctx)),
//	)
func TraceIDFromContext(ctx context.Context) string {
	sc := oteltrace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}

// SpanIDFromContext returns the span ID of the active span in ctx as a
// 16-character lowercase hex string, or "" if there is no active sampled span.
func SpanIDFromContext(ctx context.Context) string {
	sc := oteltrace.SpanFromContext(ctx).SpanContext()
	if sc.HasSpanID() {
		return sc.SpanID().String()
	}
	return ""
}

// SpanContext returns the full SpanContext for the active span in ctx.
// Prefer TraceIDFromContext / SpanIDFromContext for simple log injection.
func SpanContext(ctx context.Context) oteltrace.SpanContext {
	return oteltrace.SpanFromContext(ctx).SpanContext()
}

// ─── gRPC client dial helpers ─────────────────────────────────────────────────

// GRPCClientUnaryInterceptor returns a gRPC UnaryClientInterceptor that injects
// the current W3C trace context into outgoing gRPC metadata.
//
// Add it to grpc.Dial (or grpc.NewClient) to propagate traces to downstream services:
//
//	conn, err := grpc.NewClient(addr,
//	    grpc.WithTransportCredentials(insecure.NewCredentials()),
//	    grpc.WithChainUnaryInterceptor(otel.GRPCClientUnaryInterceptor()),
//	    grpc.WithChainStreamInterceptor(otel.GRPCClientStreamInterceptor()),
//	)
func GRPCClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	prop := gotel.GetTextMapPropagator()
	tracer := gotel.GetTracerProvider().Tracer("grpc-client")

	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx, span := tracer.Start(ctx, method,
			oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		)
		defer span.End()

		// Inject trace context into outgoing metadata.
		md := &metadataSetter{}
		prop.Inject(ctx, md)
		ctx = metadata.NewOutgoingContext(ctx, md.md)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// GRPCClientStreamInterceptor returns a gRPC StreamClientInterceptor that
// injects the current W3C trace context into outgoing gRPC metadata.
func GRPCClientStreamInterceptor() grpc.StreamClientInterceptor {
	prop := gotel.GetTextMapPropagator()
	tracer := gotel.GetTracerProvider().Tracer("grpc-client")

	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx, span := tracer.Start(ctx, method,
			oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		)
		defer span.End()

		md := &metadataSetter{}
		prop.Inject(ctx, md)
		ctx = metadata.NewOutgoingContext(ctx, md.md)

		return streamer(ctx, desc, cc, method, opts...)
	}
}

// metadataSetter is a write-only propagation.TextMapCarrier backed by gRPC metadata.
type metadataSetter struct {
	md metadata.MD
}

func (m *metadataSetter) Set(key, value string) {
	if m.md == nil {
		m.md = metadata.MD{}
	}
	m.md.Append(key, value)
}

func (m *metadataSetter) Get(_ string) string { return "" } // write-only
func (m *metadataSetter) Keys() []string       { return nil } // write-only
