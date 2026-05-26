package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/contract"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// MetricsConfig configures the OTel metrics middleware.
type MetricsConfig struct {
	// ScopeName is the OTel instrumentation scope name. Default: "astra.http".
	ScopeName string
	// MeterProvider to use. Default: otel.GetMeterProvider() (global).
	// Setting PrometheusRegisterer overrides this field.
	MeterProvider metric.MeterProvider
	// prometheusReg, when set, creates an isolated OTel meter provider backed
	// by a Prometheus exporter writing to this registry. Intended for testing.
	prometheusReg prometheus.Registerer
	// SkipPaths are paths that should not be recorded.
	SkipPaths []string
	// DurationBuckets for the request duration histogram (seconds).
	// Default: {.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	DurationBuckets []float64
	// SizeBuckets for the response size histogram (bytes).
	// Default: {100, 1000, 10000, 100000, 1e6, 1e7, 1e8}
	SizeBuckets []float64
}

var (
	defaultDurationBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	defaultSizeBuckets     = []float64{100, 1000, 10000, 100000, 1e6, 1e7, 1e8}
)

type astraOTelMetrics struct {
	requestsTotal   metric.Int64Counter
	requestDuration metric.Float64Histogram
	requestsActive  metric.Int64UpDownCounter
	responseSize    metric.Int64Histogram
	errorsTotal     metric.Int64Counter
}

func newOTelMetrics(mp metric.MeterProvider, scope string, durationBuckets, sizeBuckets []float64) (*astraOTelMetrics, error) {
	m := mp.Meter(scope)

	requestsTotal, err := m.Int64Counter("astra.http.requests",
		metric.WithDescription("Total number of HTTP requests processed, partitioned by method, route and status."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	requestDuration, err := m.Float64Histogram("astra.http.request.duration",
		metric.WithDescription("HTTP request latency distribution."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	if err != nil {
		return nil, err
	}

	requestsActive, err := m.Int64UpDownCounter("astra.http.requests.active",
		metric.WithDescription("Number of HTTP requests currently being served."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	responseSize, err := m.Int64Histogram("astra.http.response.size",
		metric.WithDescription("HTTP response body size distribution."),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(sizeBuckets...),
	)
	if err != nil {
		return nil, err
	}

	errorsTotal, err := m.Int64Counter("astra.http.errors",
		metric.WithDescription("Total number of HTTP error responses (4xx + 5xx), partitioned by method, route and status."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	return &astraOTelMetrics{
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
		requestsActive:  requestsActive,
		responseSize:    responseSize,
		errorsTotal:     errorsTotal,
	}, nil
}

// Metrics returns a middleware that records OTel metrics for each request.
//
// Instruments (exported via Prometheus bridge as astra_http_* after otel.Setup):
//   - astra.http.requests{method,path,status}         — counter
//   - astra.http.request.duration{method,path}         — histogram (seconds)
//   - astra.http.requests.active                       — updowncounter
//   - astra.http.response.size{method,path}            — histogram (bytes)
//   - astra.http.errors{method,path,status}            — counter (4xx + 5xx)
//
// Prerequisites: call otel.Setup (or otel.SetMeterProvider) before using this
// middleware so instruments are wired to a real MeterProvider.
func Metrics(opts ...func(*MetricsConfig)) astra.HandlerFunc {
	cfg := MetricsConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.ScopeName == "" {
		cfg.ScopeName = "astra.http"
	}
	if cfg.prometheusReg != nil {
		exp, err := promexporter.New(promexporter.WithRegisterer(cfg.prometheusReg))
		if err != nil {
			panic("astra/middleware/observability: failed to create Prometheus exporter: " + err.Error())
		}
		cfg.MeterProvider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(exp))
	}
	if cfg.MeterProvider == nil {
		cfg.MeterProvider = otel.GetMeterProvider()
	}
	if len(cfg.DurationBuckets) == 0 {
		cfg.DurationBuckets = defaultDurationBuckets
	}
	if len(cfg.SizeBuckets) == 0 {
		cfg.SizeBuckets = defaultSizeBuckets
	}

	m, err := newOTelMetrics(cfg.MeterProvider, cfg.ScopeName, cfg.DurationBuckets, cfg.SizeBuckets)
	if err != nil {
		panic("astra/middleware/observability: failed to create OTel metrics instruments: " + err.Error())
	}

	skip := make(map[string]bool, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skip[p] = true
	}

	return func(c *astra.Ctx) error {
		path := c.Request().URL.Path
		if skip[path] {
			return nil
		}

		routeLabel := c.GetString(contract.RouteKey)
		if routeLabel == "" {
			routeLabel = path
		}

		method := c.Request().Method
		ctx := c.Request().Context()
		start := time.Now()

		m.requestsActive.Add(ctx, 1)
		defer m.requestsActive.Add(ctx, -1)

		c.Next()

		duration := time.Since(start).Seconds()
		status := c.Writer().Status()
		statusLabel := strconv.Itoa(status)
		size := c.Writer().Size()

		attrs := []attribute.KeyValue{
			attribute.String("method", method),
			attribute.String("path", routeLabel),
			attribute.String("status", statusLabel),
		}
		routeAttrs := []attribute.KeyValue{
			attribute.String("method", method),
			attribute.String("path", routeLabel),
		}

		m.requestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
		m.requestDuration.Record(ctx, duration, metric.WithAttributes(routeAttrs...))
		m.responseSize.Record(ctx, int64(size), metric.WithAttributes(routeAttrs...))

		if status >= http.StatusBadRequest {
			m.errorsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
		}

		return nil
	}
}

// MetricsHandler returns an Astra handler that serves the /metrics endpoint.
// Register it as: app.GET("/metrics", obs.MetricsHandler())
func MetricsHandler() astra.HandlerFunc {
	h := promhttp.Handler()
	return func(c *astra.Ctx) error {
		h.ServeHTTP(c.Writer(), c.Request())
		return nil
	}
}

// MetricsHandlerFor returns a handler using a custom Prometheus gatherer.
func MetricsHandlerFor(g prometheus.Gatherer) astra.HandlerFunc {
	h := promhttp.HandlerFor(g, promhttp.HandlerOpts{})
	return func(c *astra.Ctx) error {
		h.ServeHTTP(c.Writer(), c.Request())
		return nil
	}
}

// Option helpers

func WithMetricsScopeName(name string) func(*MetricsConfig) {
	return func(c *MetricsConfig) { c.ScopeName = name }
}

func WithMetricsMeterProvider(mp metric.MeterProvider) func(*MetricsConfig) {
	return func(c *MetricsConfig) { c.MeterProvider = mp }
}

func WithMetricsSkipPaths(paths ...string) func(*MetricsConfig) {
	return func(c *MetricsConfig) { c.SkipPaths = paths }
}

func WithMetricsDurationBuckets(b []float64) func(*MetricsConfig) {
	return func(c *MetricsConfig) { c.DurationBuckets = b }
}

func WithMetricsSizeBuckets(b []float64) func(*MetricsConfig) {
	return func(c *MetricsConfig) { c.SizeBuckets = b }
}

// WithMetricsRegisterer sets a custom Prometheus registry for this middleware.
// It creates an isolated OTel meter provider backed by a Prometheus exporter
// writing to reg. Intended for test isolation: pass prometheus.NewRegistry()
// to prevent metric conflicts between test cases.
func WithMetricsRegisterer(reg prometheus.Registerer) func(*MetricsConfig) {
	return func(c *MetricsConfig) { c.prometheusReg = reg }
}
