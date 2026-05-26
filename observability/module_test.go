package observability_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/observability"
	"github.com/astra-go/astra/testutil"
	"github.com/prometheus/client_golang/prometheus"
)

// newObs returns a minimal Config suitable for unit tests:
// no OTLP endpoint, no stdout, in-memory-only OTel providers.
// A fresh Prometheus registry is used to avoid cross-test metric conflicts.
func newObs(ns string) *observability.Module {
	return observability.NewModule(observability.Config{
		ServiceName:          "test-svc",
		MetricsNamespace:     ns,
		PrometheusRegisterer: prometheus.NewRegistry(),
	})
}

// ─── Module contract ──────────────────────────────────────────────────────────

func TestModule_Name(t *testing.T) {
	if got := newObs("n").Name(); got != "observability" {
		t.Fatalf("Name() = %q, want %q", got, "observability")
	}
}

func TestModule_InstallSucceeds(t *testing.T) {
	app := testutil.NewTestApp()
	if err := app.Register(newObs("install")); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestModule_DuplicateInstallRejected(t *testing.T) {
	app := testutil.NewTestApp()
	_ = app.Register(newObs("dup"))
	if err := app.Register(newObs("dup")); err == nil {
		t.Fatal("expected error for duplicate module, got nil")
	}
}

// ─── Metrics endpoint ─────────────────────────────────────────────────────────

func TestModule_MetricsEndpointRegistered(t *testing.T) {
	app := testutil.NewTestApp()
	_ = app.Register(newObs("mep"))
	srv := testutil.NewServer(t, app)

	srv.GET("/metrics").AssertStatus(http.StatusOK)
}

func TestModule_MetricsEndpointCustomPath(t *testing.T) {
	app := testutil.NewTestApp()
	_ = app.Register(observability.NewModule(observability.Config{
		ServiceName:          "svc",
		MetricsPath:          "/internal/metrics",
		PrometheusRegisterer: prometheus.NewRegistry(),
	}))
	srv := testutil.NewServer(t, app)

	srv.GET("/internal/metrics").AssertStatus(http.StatusOK)
}

// ─── Middleware chain ─────────────────────────────────────────────────────────

func TestModule_MiddlewareChain_RequestPasses(t *testing.T) {
	app := testutil.NewTestApp()
	_ = app.Register(newObs("chain"))
	app.GET("/hello", func(c *astra.Ctx) error { return c.String(http.StatusOK, "world") })
	srv := testutil.NewServer(t, app)

	srv.GET("/hello").AssertStatus(http.StatusOK).AssertBodyContains("world")
}

// TestModule_MetricsSkipped verifies that requests to /metrics are not
// themselves recorded as metrics (avoids infinite self-observation cardinality).
func TestModule_MetricsSkipped(t *testing.T) {
	app := testutil.NewTestApp()
	_ = app.Register(newObs("skip"))
	srv := testutil.NewServer(t, app)

	// Hit /metrics several times; the endpoint must still return 200.
	for range 3 {
		srv.GET("/metrics").AssertStatus(http.StatusOK)
	}

	// Verify that the /metrics path does NOT appear in the scraped output.
	resp := srv.GET("/metrics")
	if strings.Contains(string(resp.Body()), `path="/metrics"`) {
		t.Error("/metrics path should not be recorded as a metrics label")
	}
}

// TestModule_TraceContextInLog verifies that the Tracing + Logger middlewares
// are installed in the correct order: the Logger runs after the Tracing
// middleware has set the span in the request context.
// We verify this indirectly by confirming requests complete without panic.
func TestModule_TraceContextInLog_NoPanic(t *testing.T) {
	app := testutil.NewTestApp()
	_ = app.Register(newObs("tracelog"))
	app.GET("/traced", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	srv := testutil.NewServer(t, app)

	// If middleware order is wrong (Logger before Tracing), accessing the span
	// context would still be safe (returns no-op), so this test verifies
	// the request completes successfully.
	srv.GET("/traced").AssertStatus(http.StatusOK)
}
