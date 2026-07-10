// Package otel provides a deep, zero-allocation OpenTelemetry integration for
// Astra applications.
//
// It wires together a TracerProvider (traces) and a MeterProvider (metrics),
// supports multiple trace backends — OTLP/gRPC, OTLP/HTTP, Jaeger (reached
// through its OTLP receiver), Zipkin, and stdout — and pluggable head-based
// sampling (Always / Never / TraceIDRatio / ParentBased). On top of the SDK it
// ships first-class integrations:
//
//   - app.go       — application-level configuration (TracerProvider, MeterProvider)
//   - middleware.go — net/http middleware that auto-traces every request
//   - gorm.go      — a GORM plugin that auto-traces every SQL statement
//   - grpc.go      — gRPC server & client interceptors with W3C context propagation
//
// # Quick start (explicit App)
//
//	app, err := otel.NewApp(ctx, otel.Config{
//	    ServiceName:  "my-svc",
//	    ServiceVersion: "1.2.3",
//	    Exporter:    otel.ExporterOTLPGRPC,
//	    Endpoint:    "localhost:4317",
//	    Insecure:    true,
//	})
//	if err != nil { log.Fatal(err) }
//	defer app.Shutdown(context.Background())
//
// # Quick start (global registration, used by the Showcase app)
//
//	shutdown, err := otel.Setup(ctx, otel.Config{ServiceName: "my-svc"})
//	if err != nil { log.Fatal(err) }
//	defer shutdown(context.Background())
package otel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	gotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otlptrace "go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Exporter selects the trace backend.
type Exporter int

const (
	// ExporterOTLPGRPC exports spans via OTLP/gRPC (default). Jaeger ≥ 1.35,
	// Grafana Tempo and the OpenTelemetry Collector all accept OTLP/gRPC.
	ExporterOTLPGRPC Exporter = iota
	// ExporterOTLPHTTP exports spans via OTLP/HTTP (Collector port 4318).
	ExporterOTLPHTTP
	// ExporterJaeger routes traces to Jaeger's OTLP/gRPC receiver (port 4317).
	// The legacy Jaeger Thrift exporter is deprecated and is no longer built
	// against current OTel SDKs, so Jaeger is reached through OTLP — the
	// approach recommended by the Jaeger project itself.
	ExporterJaeger
	// ExporterZipkin exports spans to a Zipkin collector
	// (e.g. "http://localhost:9411/api/v2/spans").
	ExporterZipkin
	// ExporterStdout prints spans to stdout as pretty-printed JSON. Useful in
	// development and as an additive exporter.
	ExporterStdout
	// ExporterNone disables trace export. Spans are still created locally and
	// are available to in-process processors (e.g. the log bridge).
	ExporterNone
)

// Sampler selects the head-based sampling strategy applied at span creation.
type Sampler int

const (
	// SamplerParentBased is the default: a span is sampled when its parent was
	// sampled, otherwise a TraceIDRatio(determined by SampleRatio) decision is
	// made for root spans. This keeps a whole trace together while letting you
	// throttle root-span creation under load.
	SamplerParentBased Sampler = iota
	// SamplerAlways samples every span.
	SamplerAlways
	// SamplerNever drops every span (tracing effectively off).
	SamplerNever
	// SamplerTraceIDRatio samples a fixed fraction (SampleRatio) of traces
	// regardless of the parent's sampling decision.
	SamplerTraceIDRatio
)

// Config holds the OpenTelemetry initialisation options.
type Config struct {
	// ServiceName is the logical name of this service (required).
	// Mapped to the "service.name" resource attribute.
	ServiceName string
	// ServiceVersion is the semantic version of this service (e.g. "1.2.3").
	// Mapped to "service.version".
	ServiceVersion string
	// ServiceNamespace groups related services (e.g. "payments").
	// Mapped to "service.namespace".
	ServiceNamespace string
	// ServiceEnvironment is the deployment environment (e.g. "production").
	// Mapped to "deployment.environment.name".
	ServiceEnvironment string

	// Exporter selects the trace backend. Default: ExporterOTLPGRPC.
	Exporter Exporter
	// Endpoint is the backend address. Interpretation depends on Exporter:
	//   - OTLPGRPC: "host:port"            (e.g. "localhost:4317")
	//   - OTLPHTTP : "http(s)://host:port" (e.g. "http://localhost:4318")
	//   - Jaeger   : OTLP/gRPC "host:port" (defaults to "localhost:4317")
	//   - Zipkin   : full collector URL    (e.g. "http://localhost:9411/api/v2/spans")
	Endpoint string
	// OTLPEndpoint is a deprecated alias for Endpoint used by older callers
	// (Showcase). It is consulted only when Endpoint is empty and Exporter is
	// OTLP gRPC / Jaeger.
	OTLPEndpoint string
	// Insecure disables TLS for OTLP/gRPC and OTLP/HTTP connections.
	Insecure bool
	// DialTimeout bounds the OTLP exporter connection. Default: 5s.
	DialTimeout time.Duration

	// Sampler selects the head-based sampling strategy. Default: ParentBased.
	Sampler Sampler
	// SampleRatio is the sampling fraction in [0,1] used by TraceIDRatio and
	// ParentBased. Default: 1.0 (sample everything).
	SampleRatio float64

	// EnableStdout additionally exports spans to stdout, on top of any backend
	// selected by Exporter.
	EnableStdout bool

	// EnablePrometheus installs the Prometheus metrics bridge so the
	// MeterProvider is scrapeable via promhttp. Default: true.
	EnablePrometheus bool
	// PrometheusRegisterer is the Prometheus registry for the metrics bridge.
	// If nil, prometheus.DefaultRegisterer is used (shared global registry —
	// prefer a dedicated registry in tests to avoid cross-test conflicts).
	PrometheusRegisterer promclient.Registerer

	// ResourceAttributes are extra resource attributes merged into the
	// auto-detected resource (host, process, telemetry SDK).
	ResourceAttributes []attribute.KeyValue
	// ResourceDetectors are extra resource detectors appended to the defaults.
	ResourceDetectors []resource.Detector
}

// App bundles the configured TracerProvider, MeterProvider, propagator and
// Resource. It is the single object you pass to the HTTP/GORM/gRPC integrations
// so they all share one pipeline.
type App struct {
	cfg     Config
	res     *resource.Resource
	tp      *sdktrace.TracerProvider
	mp      *sdkmetric.MeterProvider
	prop    propagation.TextMapPropagator
	tracer  trace.Tracer
	meter   otelmetric.Meter
	closers []func(context.Context) error

	shutdownOnce sync.Once
	shutdownErr  error
}

// TracerProvider returns the underlying SDK TracerProvider.
func (a *App) TracerProvider() *sdktrace.TracerProvider { return a.tp }

// MeterProvider returns the underlying SDK MeterProvider.
func (a *App) MeterProvider() *sdkmetric.MeterProvider { return a.mp }

// Tracer returns an instrumented tracer from this App's provider.
// An empty name defaults to "astra/otel".
func (a *App) Tracer(name string) trace.Tracer {
	if name == "" {
		name = "astra/otel"
	}
	return a.tp.Tracer(name)
}

// Meter returns an instrumented meter from this App's provider.
// An empty name defaults to "astra/otel".
func (a *App) Meter(name string) otelmetric.Meter {
	if name == "" {
		name = "astra/otel"
	}
	return a.mp.Meter(name)
}

// Resource returns the merged resource describing this service.
func (a *App) Resource() *resource.Resource { return a.res }

// Propagator returns the composite W3C TraceContext + Baggage propagator.
func (a *App) Propagator() propagation.TextMapPropagator { return a.prop }

// Shutdown flushes and closes every exporter/provider. It is safe to call more
// than once; only the first call has an effect.
func (a *App) Shutdown(ctx context.Context) error {
	a.shutdownOnce.Do(func() {
		a.shutdownErr = chainShutdown(ctx, a.closers)
	})
	return a.shutdownErr
}

// NewApp builds the TracerProvider, MeterProvider and propagator described by
// cfg. It does NOT register them globally; call Setup if you want the classic
// global-registration behaviour.
func NewApp(ctx context.Context, cfg Config) (*App, error) {
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("otel: ServiceName is required")
	}
	if cfg.SampleRatio <= 0 {
		cfg.SampleRatio = 1.0
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("otel: build resource: %w", err)
	}

	exporters, expShutdown, err := newTraceExporters(ctx, cfg)
	if err != nil {
		return nil, err
	}

	tp := buildTracerProvider(res, exporters, newSampler(cfg))

	mp, err := buildMeterProvider(res, cfg)
	if err != nil {
		_ = chainShutdown(ctx, expShutdown)
		return nil, fmt.Errorf("otel: meter provider: %w", err)
	}

	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	app := &App{
		cfg:    cfg,
		res:    res,
		tp:     tp,
		mp:     mp,
		prop:   prop,
		tracer: tp.Tracer("astra/otel"),
		meter:  mp.Meter("astra/otel"),
		closers: append(expShutdown,
			tp.Shutdown,
			mp.Shutdown,
		),
	}
	return app, nil
}

// Setup initialises the global OTel TracerProvider, MeterProvider and
// TextMapPropagator and returns a shutdown function that MUST be called before
// the process exits so pending spans/metrics are flushed.
//
//	shutdown, err := otel.Setup(ctx, otel.Config{ServiceName: "my-svc"})
//	if err != nil { log.Fatal(err) }
//	defer shutdown(context.Background())
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	app, err := NewApp(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// Register globals so code that relies on otel.GetTracerProvider() /
	// otel.GetTextMapPropagator() (including the framework's own interceptors)
	// transparently uses this pipeline.
	gotel.SetTracerProvider(app.tp)
	gotel.SetMeterProvider(app.mp)
	gotel.SetTextMapPropagator(app.prop)
	return app.Shutdown, nil
}

// ─── builders ────────────────────────────────────────────────────────────────

func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attrs := make([]attribute.KeyValue, 0, 4+len(cfg.ResourceAttributes))
	attrs = append(attrs,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
	)
	if cfg.ServiceNamespace != "" {
		attrs = append(attrs, semconv.ServiceNamespace(cfg.ServiceNamespace))
	}
	if cfg.ServiceEnvironment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentKey.String(cfg.ServiceEnvironment))
	}
	attrs = append(attrs, cfg.ResourceAttributes...)

	opts := []resource.Option{
		resource.WithAttributes(attrs...),
		resource.WithProcessPID(),
		resource.WithHost(),
		resource.WithTelemetrySDK(),
	}
	if len(cfg.ResourceDetectors) > 0 {
		opts = append(opts, resource.WithDetectors(cfg.ResourceDetectors...))
	}

	// Do not set an explicit schema URL at the resource level: WithTelemetrySDK
	// already carries the SDK's schema URL, and mixing an explicit
	// semconv-versioned URL with it triggers resource.ErrSchemaURLConflict on
	// SDK upgrades. Each attribute's semantic-convention identity is implied by
	// its key name.
	r, err := resource.New(ctx, opts...)
	if err != nil {
		// ErrPartialResource: some detectors partially failed (e.g. host
		// detection in a sandbox). The partial resource is still usable.
		if errors.Is(err, resource.ErrPartialResource) {
			return r, nil
		}
		return nil, err
	}
	return r, nil
}

// newTraceExporters builds the span exporters selected by cfg and returns the
// matching shutdown functions.
func newTraceExporters(ctx context.Context, cfg Config) ([]sdktrace.SpanExporter, []func(context.Context) error, error) {
	var exporters []sdktrace.SpanExporter
	var shutdowns []func(context.Context) error

	add := func(exp sdktrace.SpanExporter, shut func(context.Context) error) {
		exporters = append(exporters, exp)
		shutdowns = append(shutdowns, shut)
	}

	endpoint := resolveEndpoint(cfg)

	switch cfg.Exporter {
	case ExporterOTLPGRPC:
		if endpoint == "" {
			break // no endpoint → no exporter (matches legacy OTLPEndpoint=="" behaviour)
		}
		exp, err := newOTLPGRPCExporter(ctx, endpoint, cfg.Insecure, cfg.DialTimeout)
		if err != nil {
			return nil, nil, err
		}
		add(exp, exp.Shutdown)
	case ExporterOTLPHTTP:
		if endpoint == "" {
			return nil, nil, fmt.Errorf("otel: OTLP/HTTP exporter requires Endpoint")
		}
		exp, err := newOTLPHTTPExporter(ctx, endpoint, cfg.Insecure)
		if err != nil {
			return nil, nil, err
		}
		add(exp, exp.Shutdown)
	case ExporterJaeger:
		je := endpoint
		if je == "" {
			je = "localhost:4317"
		}
		exp, err := newOTLPGRPCExporter(ctx, je, cfg.Insecure, cfg.DialTimeout)
		if err != nil {
			return nil, nil, err
		}
		add(exp, exp.Shutdown)
	case ExporterZipkin:
		if endpoint == "" {
			return nil, nil, fmt.Errorf("otel: Zipkin exporter requires Endpoint")
		}
		exp, err := zipkin.New(endpoint)
		if err != nil {
			return nil, nil, fmt.Errorf("otel: zipkin exporter: %w", err)
		}
		add(exp, exp.Shutdown)
	case ExporterStdout:
		exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, nil, fmt.Errorf("otel: stdout exporter: %w", err)
		}
		add(exp, exp.Shutdown)
	case ExporterNone:
		// intentionally empty
	}

	if cfg.EnableStdout {
		exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, nil, fmt.Errorf("otel: stdout exporter: %w", err)
		}
		add(exp, exp.Shutdown)
	}

	return exporters, shutdowns, nil
}

func resolveEndpoint(cfg Config) string {
	if cfg.Endpoint != "" {
		return cfg.Endpoint
	}
	return cfg.OTLPEndpoint
}

func newOTLPGRPCExporter(ctx context.Context, endpoint string, skipTLS bool, dialTimeout time.Duration) (*otlptrace.Exporter, error) {
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithDialOption(grpc.WithBlock()),
	}
	if skipTLS {
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	exp, err := otlptracegrpc.New(dialCtx, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", endpoint, err)
	}
	return exp, nil
}

func newOTLPHTTPExporter(ctx context.Context, endpoint string, skipTLS bool) (*otlptrace.Exporter, error) {
	e := endpoint
	if strings.HasPrefix(e, "http://") {
		e = strings.TrimPrefix(e, "http://")
		skipTLS = true
	} else if strings.HasPrefix(e, "https://") {
		e = strings.TrimPrefix(e, "https://")
	}
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(e)}
	if skipTLS {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otel: otlp http exporter: %w", err)
	}
	return exp, nil
}

func newSampler(cfg Config) sdktrace.Sampler {
	switch cfg.Sampler {
	case SamplerAlways:
		return sdktrace.AlwaysSample()
	case SamplerNever:
		return sdktrace.NeverSample()
	case SamplerTraceIDRatio:
		return sdktrace.TraceIDRatioBased(cfg.SampleRatio)
	default: // SamplerParentBased
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))
	}
}

func buildTracerProvider(res *resource.Resource, exporters []sdktrace.SpanExporter, sampler sdktrace.Sampler) *sdktrace.TracerProvider {
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}
	for _, exp := range exporters {
		opts = append(opts, sdktrace.WithBatcher(exp))
	}
	return sdktrace.NewTracerProvider(opts...)
}

func buildMeterProvider(res *resource.Resource, cfg Config) (*sdkmetric.MeterProvider, error) {
	if !cfg.EnablePrometheus && cfg.PrometheusRegisterer == nil {
		// Default-on for backwards compatibility with the legacy Setup.
		cfg.EnablePrometheus = true
	}
	if !cfg.EnablePrometheus {
		return sdkmetric.NewMeterProvider(sdkmetric.WithResource(res)), nil
	}

	opts := []otelprom.Option{}
	if cfg.PrometheusRegisterer != nil {
		opts = append(opts, otelprom.WithRegisterer(cfg.PrometheusRegisterer))
	}
	promExp, err := otelprom.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("prometheus exporter: %w", err)
	}
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExp),
	), nil
}

// chainShutdown calls every closer and returns the first error encountered.
func chainShutdown(ctx context.Context, fns []func(context.Context) error) error {
	var firstErr error
	for _, fn := range fns {
		if err := fn(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ─── zero-allocation helpers ──────────────────────────────────────────────────
//
// The hot paths (HTTP middleware, GORM plugin, gRPC interceptors) build a small
// []attribute.KeyValue per call. The OTel SDK copies attribute values into the
// span at Start/SetAttributes time, so the slice can be safely recycled
// afterwards. A shared pool keeps per-request allocations off the heap.

// attrSlice wraps a []attribute.KeyValue so it can be pooled via sync.Pool
// (Go does not allow *[]T as a slice operand, so wrapping is required).
type attrSlice struct{ v []attribute.KeyValue }

var attrPool = sync.Pool{
	New: func() any {
		return &attrSlice{v: make([]attribute.KeyValue, 0, 8)}
	},
}

// getAttrs returns a pooled *attrSlice reset to length 0.
func getAttrs() *attrSlice {
	s := attrPool.Get().(*attrSlice)
	s.v = s.v[:0]
	return s
}

// putAttrs returns a slice to the pool. Call it only after the SDK has consumed
// the values (i.e. after tracer.Start / span.SetAttributes has returned).
func putAttrs(s *attrSlice) {
	s.v = s.v[:0]
	attrPool.Put(s)
}
