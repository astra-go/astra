// Package security provides tenant-level Prometheus metrics middleware.
//
// TenantMetrics records per-tenant request counts, latency, active requests,
// and quota exceeded events as Prometheus metrics. Each metric is labelled
// with the tenant identifier so dashboards and alerts can be scoped to
// individual tenants.
//
// # Metrics emitted
//
//   astra_tenant_requests_total{tenant,method,path,status}
//   astra_tenant_request_duration_seconds{tenant}  (histogram)
//   astra_tenant_active_requests{tenant}            (gauge)
//   astra_tenant_quota_exceeded_total{tenant,type}
//
// # Usage
//
//	app.Use(security.TenantMetrics())
//
//	// Custom buckets
//	app.Use(security.TenantMetricsWithConfig(security.TenantMetricsConfig{
//	    DurationBuckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
//	}))
package security

import (
	"strconv"
	"time"

	"github.com/astra-go/astra"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricNamespace = "astra"
	metricSubsystem = "tenant"
)

// TenantMetricsConfig configures the TenantMetrics middleware.
type TenantMetricsConfig struct {
	// DurationBuckets defines the histogram buckets for request latency.
	// Defaults to prometheus.DefBuckets if nil.
	DurationBuckets []float64

	// KeyFunc extracts the tenant identifier from the context.
	// Defaults to TenantID.
	KeyFunc func(*astra.Ctx) string

	// Skipper skips metrics recording for matching requests.
	Skipper Skipper

	// Registerer is the Prometheus registerer to use.
	// Defaults to prometheus.DefaultRegisterer.
	Registerer prometheus.Registerer
}

// defaultTenantMetricsBuckets are the default histogram buckets for request duration.
var defaultTenantMetricsBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// TenantMetrics returns a middleware that records per-tenant Prometheus metrics
// using default configuration.
func TenantMetrics() astra.HandlerFunc {
	return TenantMetricsWithConfig(TenantMetricsConfig{})
}

// TenantMetricsWithConfig returns a TenantMetrics middleware with custom config.
func TenantMetricsWithConfig(cfg TenantMetricsConfig) astra.HandlerFunc {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = TenantID
	}
	if cfg.DurationBuckets == nil {
		cfg.DurationBuckets = defaultTenantMetricsBuckets
	}
	if cfg.Registerer == nil {
		cfg.Registerer = prometheus.DefaultRegisterer
	}

	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
		Subsystem: metricSubsystem,
			Name:      "requests_total",
			Help:      "Total number of requests per tenant.",
		},
		[]string{"tenant", "method", "path", "status"},
	)
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Subsystem: metricSubsystem,
			Name:      "request_duration_seconds",
			Help:      "Request duration per tenant in seconds.",
			Buckets:   cfg.DurationBuckets,
		},
		[]string{"tenant"},
	)
	activeRequests := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Subsystem: metricSubsystem,
			Name:      "active_requests",
			Help:      "Number of currently active requests per tenant.",
		},
		[]string{"tenant"},
	)
	quotaExceeded := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Subsystem: metricSubsystem,
			Name:      "quota_exceeded_total",
			Help:      "Total number of quota exceeded events per tenant.",
		},
		[]string{"tenant", "type"},
	)

	cfg.Registerer.MustRegister(requestsTotal, requestDuration, activeRequests, quotaExceeded)

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			c.Next()
			return nil
		}

		tenantID := cfg.KeyFunc(c)
		if tenantID == "" {
			tenantID = "unknown"
		}

		activeRequests.WithLabelValues(tenantID).Inc()
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer().Status())

		requestsTotal.WithLabelValues(tenantID, c.Request().Method, c.Request().URL.Path, status).Inc()
		requestDuration.WithLabelValues(tenantID).Observe(duration)
		activeRequests.WithLabelValues(tenantID).Dec()

		// Track quota exceeded events (429 status).
		if c.Writer().Status() == 429 {
			quotaType := "unknown"
			if v, ok := c.Get("quota_exceeded_type"); ok {
				quotaType, _ = v.(string)
			}
			quotaExceeded.WithLabelValues(tenantID, quotaType).Inc()
		}

		return nil
	}
}