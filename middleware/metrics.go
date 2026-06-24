// Package middleware provides HTTP middleware for the Astra web framework.
//
// Metrics middleware records Prometheus request metrics: request count, latency
// histogram, and in-flight gauge, labelled by method, path, and status code.
//
// # Quick start
//
//	app.Use(middleware.RequestMetrics())
//
// Custom buckets:
//
//	app.Use(middleware.RequestMetricsWithConfig(middleware.RequestMetricsConfig{
//	    Namespace: "mysvc",
//	    Subsystem: "http",
//	    Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
//	}))
//
// # Metrics emitted
//
//	astra_http_requests_total{method,path,status}
//	astra_http_request_duration_seconds{method,path}  (histogram)
//	astra_http_active_requests{method}                 (gauge)
//
// # Integration with otel.Setup
//
// The `otel` package sets up a Prometheus bridge on the default registerer.
// When using `otel.Setup()`, the metrics registered by this middleware are
// automatically included in the `/metrics` endpoint — no extra wiring needed.
package middleware

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/astra-go/astra"
	"github.com/prometheus/client_golang/prometheus"
)

// RequestMetrics returns a middleware that records general HTTP request
// metrics (count, latency histogram, active requests) using default labels
// and buckets.
//
// Paths with variable segments (e.g. /api/users/:id) are recorded as-is,
// so high-cardinality path parameters should be normalised before reaching
// this middleware, or use a custom PathLabelFunc to sanitise.
func RequestMetrics() astra.HandlerFunc {
	return RequestMetricsWithConfig(RequestMetricsConfig{})
}

// RequestMetricsConfig configures the RequestMetrics middleware.
type RequestMetricsConfig struct {
	// Namespace for all metric names. Default: "astra".
	Namespace string
	// Subsystem for all metric names. Default: "http".
	Subsystem string

	// Buckets for request duration histogram.
	// Default: prometheus.DefBuckets.
	Buckets []float64

	// PathLabelFunc extracts or normalises the HTTP path label.
	// Default: c.Request().URL.Path (raw request path).
	// Use this to collapse /api/users/42 into /api/users/:id.
	PathLabelFunc func(*astra.Ctx) string

	// Skipper skips metrics recording for matching requests.
	Skipper func(*astra.Ctx) bool

	// Registerer is the Prometheus registerer to use.
	// Defaults to prometheus.DefaultRegisterer.
	Registerer prometheus.Registerer
}

// RequestMetricsWithConfig returns a RequestMetrics middleware with custom config.
func RequestMetricsWithConfig(cfg RequestMetricsConfig) astra.HandlerFunc {
	if cfg.Namespace == "" {
		cfg.Namespace = "astra"
	}
	if cfg.Subsystem == "" {
		cfg.Subsystem = "http"
	}
	if cfg.Buckets == nil {
		cfg.Buckets = prometheus.DefBuckets
	}
	if cfg.PathLabelFunc == nil {
		cfg.PathLabelFunc = func(c *astra.Ctx) string {
			return c.Request().URL.Path
		}
	}
	if cfg.Registerer == nil {
		cfg.Registerer = prometheus.DefaultRegisterer
	}

	// Collectors are lazily initialised and registered.
	var (
		once            sync.Once
		requestsTotal   *prometheus.CounterVec
		requestDuration *prometheus.HistogramVec
		activeRequests  *prometheus.GaugeVec
	)

	initMetrics := func() {
		requestsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "requests_total",
				Help:      "Total number of HTTP requests.",
			},
			[]string{"method", "path", "status"},
		)
		requestDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds.",
				Buckets:   cfg.Buckets,
			},
			[]string{"method", "path"},
		)
		activeRequests = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "active_requests",
				Help:      "Number of currently active HTTP requests.",
			},
			[]string{"method"},
		)

		// Attempt to register; silently skip if already registered by another
		// call to RequestMetricsWithConfig with the same namespace/subsystem.
		err := cfg.Registerer.Register(requestsTotal)
		if err != nil {
			var are prometheus.AlreadyRegisteredError
			if errors.As(err, &are) {
				requestsTotal = are.ExistingCollector.(*prometheus.CounterVec)
			}
		}

		err = cfg.Registerer.Register(requestDuration)
		if err != nil {
			var are prometheus.AlreadyRegisteredError
			if errors.As(err, &are) {
				requestDuration = are.ExistingCollector.(*prometheus.HistogramVec)
			}
		}

		err = cfg.Registerer.Register(activeRequests)
		if err != nil {
			var are prometheus.AlreadyRegisteredError
			if errors.As(err, &are) {
				activeRequests = are.ExistingCollector.(*prometheus.GaugeVec)
			}
		}
	}

	return func(c *astra.Ctx) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return nil
		}

		once.Do(initMetrics)

		path := cfg.PathLabelFunc(c)
		method := c.Request().Method

		activeRequests.WithLabelValues(method).Inc()
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer().Status())

		requestsTotal.WithLabelValues(method, path, status).Inc()
		requestDuration.WithLabelValues(method, path).Observe(duration)
		activeRequests.WithLabelValues(method).Dec()

		return nil
	}
}
