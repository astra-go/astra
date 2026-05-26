// Package observability provides a unified facade that wires together
// OpenTelemetry tracing, Prometheus metrics, and structured logging for
// Astra applications in a single app.Register call.
//
// # Quick start
//
//	app := astra.New()
//	must(app.Register(observability.NewModule(observability.Config{
//	    ServiceName:  "payment-svc",
//	    OTLPEndpoint: "localhost:4317",
//	    OTLPInsecure: true,
//	})))
//	app.Run(":8080")
//
// The module installs, in order:
//
//  1. OTel TracerProvider + MeterProvider (global) via otel.Setup.
//  2. Tracing middleware — creates a server span for every request.
//  3. Logger middleware — structured access log, automatically enriched with
//     trace_id and span_id from the active span.
//  4. Prometheus metrics middleware — records request counters, latency, size.
//  5. GET /metrics endpoint serving the Prometheus text format.
//
// A graceful-shutdown hook flushes and shuts down the OTel exporters.
package observability

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/astra-go/astra"
	astralog "github.com/astra-go/astra/log"
	"github.com/astra-go/astra/middleware"
	mwobs "github.com/astra-go/astra/middleware/observability"
	astraotel "github.com/astra-go/astra/otel"
	"github.com/prometheus/client_golang/prometheus"
)

// Config holds the observability module configuration.
type Config struct {
	// ServiceName is the OTel service.name resource attribute (required).
	ServiceName string

	// OTLPEndpoint is the gRPC OTLP receiver address (e.g. "localhost:4317").
	// Leave empty to run without a remote trace exporter (useful in dev/test).
	OTLPEndpoint string
	// OTLPInsecure disables TLS on the OTLP gRPC connection.
	OTLPInsecure bool

	// SampleRatio controls head-based trace sampling: 0.0 = never, 1.0 = always.
	// Default: 1.0.
	SampleRatio float64

	// EnableStdout additionally exports traces to stdout (pretty-printed JSON).
	// Useful during local development.
	EnableStdout bool

	// MetricsPath is the Prometheus scrape endpoint.  Default: "/metrics".
	MetricsPath string

	// MetricsNamespace / MetricsSubsystem prefix Prometheus metric names.
	// Defaults: "astra" / "http".
	MetricsNamespace string
	MetricsSubsystem string

	// LogFormat is "json" or "text".  Default: "json" (structured, machine-parseable).
	LogFormat string
	// LogLevel is the minimum log level.  Default: slog.LevelInfo.
	LogLevel slog.Level

	// SkipPaths are paths excluded from tracing, metrics, and access logging
	// (e.g. liveness probes).  MetricsPath is always skipped automatically.
	SkipPaths []string

	// PrometheusRegisterer is the Prometheus registry to use for OTel metrics.
	// If nil, uses prometheus.DefaultRegisterer.  Pass prometheus.NewRegistry()
	// in tests to avoid cross-test metric conflicts.
	PrometheusRegisterer prometheus.Registerer
}

// Module implements astra.Component and installs the full observability stack.
type Module struct {
	cfg      Config
	shutdown func(context.Context) error
}

// NewModule returns a Module that will wire metrics + tracing + logging when
// installed.  cfg.ServiceName is required; all other fields have sensible defaults.
func NewModule(cfg Config) *Module {
	if cfg.MetricsPath == "" {
		cfg.MetricsPath = "/metrics"
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = "json"
	}
	return &Module{cfg: cfg}
}

// Name satisfies astra.Component.
func (m *Module) Name() string { return "observability" }

// Init wires the observability stack into app.
//
// Call order within Init:
//  1. otel.Setup — initialises global TracerProvider and MeterProvider.
//  2. log.SetDefault — replaces the global logger with a JSON/text logger.
//  3. app.Use(Tracing) — must be first so the span is in the context.
//  4. app.Use(Logger with WithTraceContext) — reads trace_id/span_id.
//  5. app.Use(Metrics) — records request counters, latency, error rate.
//  6. app.GET(MetricsPath) — exposes the Prometheus scrape endpoint.
//  7. app.OnStop — flushes and shuts down OTel exporters.
func (m *Module) Init(app *astra.App) error {
	// ── 1. OTel SDK setup ─────────────────────────────────────────────────────
	// Must run before middleware registration so Tracing captures the real
	// TracerProvider rather than the pre-init no-op global.
	if m.cfg.OTLPInsecure {
		slog.Warn("OTLP connection is insecure; do not use in production",
			"endpoint", m.cfg.OTLPEndpoint)
	}
	shutdown, err := astraotel.Setup(context.Background(), astraotel.Config{
		ServiceName:          m.cfg.ServiceName,
		OTLPEndpoint:         m.cfg.OTLPEndpoint,
		Insecure:             m.cfg.OTLPInsecure,
		SampleRatio:          m.cfg.SampleRatio,
		EnableStdout:         m.cfg.EnableStdout,
		PrometheusRegisterer: m.cfg.PrometheusRegisterer,
	})
	if err != nil {
		return fmt.Errorf("observability: otel setup: %w", err)
	}
	m.shutdown = shutdown

	// ── 2. Global logger ──────────────────────────────────────────────────────
	// Replace log/slog default with a properly levelled logger.
	// Per-request trace correlation happens in the Logger middleware (step 4);
	// application code using log.GetDefault().WithContext(ctx) gets it for free
	// because the OTel TracerProvider is now initialised.
	astralog.SetDefault(astralog.New(astralog.Config{
		Format: m.cfg.LogFormat,
		Level:  m.cfg.LogLevel,
	}))

	// ── 3-5. Middleware chain ───────────────��─────────────────────────────────
	// Order is critical: Tracing sets the span, Logger reads it, Metrics records
	// the final status.  MetricsPath is always skipped to avoid self-observation.
	skipPaths := buildSkipPaths(m.cfg.MetricsPath, m.cfg.SkipPaths)

	app.Use(mwobs.Tracing(
		mwobs.WithTracingSkipPaths(skipPaths...),
	))
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format:        m.cfg.LogFormat,
		SkipPaths:     skipPaths,
		SpanExtractor: mwobs.OTelSpanExtractor(),
	}))
	app.Use(mwobs.Metrics(
		mwobs.WithMetricsSkipPaths(skipPaths...),
		optScopeName(m.cfg.MetricsNamespace, m.cfg.MetricsSubsystem),
	))

	// ── 6. Prometheus scrape endpoint ─────────────────────────────────────────
	app.GET(m.cfg.MetricsPath, mwobs.MetricsHandler())

	// ── 7. Lifecycle: flush OTel exporters on shutdown ────────────────────────
	return app.OnStop(func(ctx context.Context) error {
		return m.shutdown(ctx)
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func buildSkipPaths(metricsPath string, extra []string) []string {
	paths := make([]string, 0, len(extra)+1)
	paths = append(paths, metricsPath)
	paths = append(paths, extra...)
	return paths
}

// optScopeName maps legacy MetricsNamespace / MetricsSubsystem fields to an
// OTel scope name. Returns nil (use default "astra.http") when both are empty.
func optScopeName(ns, sub string) func(*mwobs.MetricsConfig) {
	if ns == "" && sub == "" {
		return func(*mwobs.MetricsConfig) {}
	}
	var scope string
	switch {
	case ns != "" && sub != "":
		scope = ns + "." + sub
	case ns != "":
		scope = ns
	default:
		scope = sub
	}
	return mwobs.WithMetricsScopeName(scope)
}
