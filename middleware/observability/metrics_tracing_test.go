package observability_test

import (
	"net/http"
	"testing"

	"github.com/astra-go/astra"
	mwobs "github.com/astra-go/astra/middleware/observability"
	"github.com/astra-go/astra/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace/noop"
)

func newIsolatedRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}

func gatherCounter(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			for _, m := range mf.GetMetric() {
				if c := m.GetCounter(); c != nil {
					return c.GetValue()
				}
			}
		}
	}
	return 0
}

func gatherCounterWithLabels(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), labels) {
				if c := m.GetCounter(); c != nil {
					return c.GetValue()
				}
			}
		}
	}
	return 0
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	got := make(map[string]string, len(pairs))
	for _, p := range pairs {
		got[p.GetName()] = p.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

func TestMetrics_IncrementsRequestsTotal(t *testing.T) {
	reg := newIsolatedRegistry()
	app := testutil.NewTestApp()
	app.Use(mwobs.Metrics(
		mwobs.WithMetricsRegisterer(reg),
	))
	app.GET("/users", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/users")
	s.GET("/users")

	count := gatherCounterWithLabels(t, reg, "astra_http_requests_total", map[string]string{
		"method": "GET",
		"status": "200",
	})
	if count < 2 {
		t.Errorf("expected requests_total ≥ 2, got %.0f", count)
	}
}

func TestMetrics_CountsErrors(t *testing.T) {
	reg := newIsolatedRegistry()
	app := testutil.NewTestApp()
	app.Use(mwobs.Metrics(
		mwobs.WithMetricsRegisterer(reg),
	))
	app.GET("/fail", func(c *astra.Ctx) error {
		return astra.NewHTTPError(http.StatusBadRequest, "bad")
	})
	s := testutil.NewServer(t, app)

	s.GET("/fail")

	errCount := gatherCounterWithLabels(t, reg, "astra_http_errors_total", map[string]string{
		"status": "400",
	})
	if errCount < 1 {
		t.Errorf("expected errors_total ≥ 1 for 400 response, got %.0f", errCount)
	}
}

func TestMetrics_SkipsPaths(t *testing.T) {
	reg := newIsolatedRegistry()
	app := testutil.NewTestApp()
	app.Use(mwobs.Metrics(
		mwobs.WithMetricsRegisterer(reg),
		mwobs.WithMetricsSkipPaths("/health"),
	))
	app.GET("/health", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/health")

	count := gatherCounter(t, reg, "astra_http_requests_total")
	if count != 0 {
		t.Errorf("expected 0 requests_total for skipped path, got %.0f", count)
	}
}

func TestTracing_StoresSpanInContext(t *testing.T) {
	tp := noop.NewTracerProvider()

	app := testutil.NewTestApp()
	app.Use(mwobs.Tracing(
		mwobs.WithTracerName("test-tracer"),
		func(cfg *mwobs.TracingConfig) { cfg.TracerProvider = tp },
	))
	var spanStored bool
	app.GET("/", func(c *astra.Ctx) error {
		_, ok := c.Get("otel.span")
		spanStored = ok
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	if !spanStored {
		t.Error("Tracing middleware should store span in context with key 'otel.span'")
	}
}

func TestTracing_SkipsPaths(t *testing.T) {
	tp := noop.NewTracerProvider()

	app := testutil.NewTestApp()
	app.Use(mwobs.Tracing(
		func(cfg *mwobs.TracingConfig) { cfg.TracerProvider = tp },
		mwobs.WithTracingSkipPaths("/healthz"),
	))
	var spanStored bool
	app.GET("/healthz", func(c *astra.Ctx) error {
		_, spanStored = c.Get("otel.span")
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/healthz")

	if spanStored {
		t.Error("Tracing middleware should not store span for skipped path")
	}
}

func TestTracing_PassesRequestThrough(t *testing.T) {
	tp := noop.NewTracerProvider()

	app := testutil.NewTestApp()
	app.Use(mwobs.Tracing(func(cfg *mwobs.TracingConfig) { cfg.TracerProvider = tp }))
	app.GET("/api", func(c *astra.Ctx) error { return c.String(http.StatusOK, "data") })
	s := testutil.NewServer(t, app)

	s.GET("/api").AssertStatus(http.StatusOK).AssertBodyContains("data")
}

func TestTracing_CustomSpanName(t *testing.T) {
	tp := noop.NewTracerProvider()

	app := testutil.NewTestApp()
	app.Use(mwobs.Tracing(
		func(cfg *mwobs.TracingConfig) { cfg.TracerProvider = tp },
		mwobs.WithSpanNameFormatter(func(method, path string) string {
			return "custom:" + method + ":" + path
		}),
	))
	app.GET("/thing", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/thing").AssertStatus(http.StatusOK)
}
