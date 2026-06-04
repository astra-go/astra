package security_test

import (
	"net/http"
	"testing"

	"github.com/astra-go/astra"
	sec "github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
	"github.com/prometheus/client_golang/prometheus"
)

func TestTenantMetrics_RecordsRequests(t *testing.T) {
	// Use a custom registry to avoid polluting the default one.
	reg := prometheus.NewRegistry()

	app := testutil.NewTestApp()
	app.Use(sec.TenantMetricsWithConfig(sec.TenantMetricsConfig{
		KeyFunc:     func(_ *astra.Ctx) string { return "acme" },
		Registerer:  reg,
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	// Verify the requests_total counter was incremented.
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "astra_tenant_requests_total" {
			for _, m := range mf.GetMetric() {
				for _, label := range m.GetLabel() {
					if label.GetName() == "tenant" && label.GetValue() == "acme" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("astra_tenant_requests_total metric not found for tenant=acme")
	}
}

func TestTenantMetrics_UnknownTenant(t *testing.T) {
	reg := prometheus.NewRegistry()

	app := testutil.NewTestApp()
	app.Use(sec.TenantMetricsWithConfig(sec.TenantMetricsConfig{
		KeyFunc:    func(_ *astra.Ctx) string { return "" }, // empty → "unknown"
		Registerer: reg,
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "astra_tenant_requests_total" {
			for _, m := range mf.GetMetric() {
				for _, label := range m.GetLabel() {
					if label.GetName() == "tenant" && label.GetValue() == "unknown" {
						return // found
					}
				}
			}
		}
	}
	t.Error("expected tenant=unknown label in metrics")
}

func TestTenantMetrics_Skipper(t *testing.T) {
	reg := prometheus.NewRegistry()

	app := testutil.NewTestApp()
	app.Use(sec.TenantMetricsWithConfig(sec.TenantMetricsConfig{
		KeyFunc:    func(_ *astra.Ctx) string { return "acme" },
		Registerer: reg,
		Skipper:    func(c *astra.Ctx) bool { return c.Request().URL.Path == "/health" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	app.GET("/health", func(c *astra.Ctx) error { return c.String(http.StatusOK, "healthy") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
	s.GET("/health").AssertStatus(http.StatusOK)

	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "astra_tenant_requests_total" {
			// Only one metric should exist (for "/", not "/health").
			if len(mf.GetMetric()) != 1 {
				t.Errorf("expected 1 metric, got %d", len(mf.GetMetric()))
			}
		}
	}
}

func TestTenantMetrics_ActiveRequestsGauge(t *testing.T) {
	reg := prometheus.NewRegistry()

	app := testutil.NewTestApp()
	app.Use(sec.TenantMetricsWithConfig(sec.TenantMetricsConfig{
		KeyFunc:    func(_ *astra.Ctx) string { return "acme" },
		Registerer: reg,
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "astra_tenant_active_requests" {
			for _, m := range mf.GetMetric() {
				// After request completes, gauge should be 0.
				if m.GetGauge().GetValue() != 0 {
					t.Errorf("active_requests should be 0 after request, got %v", m.GetGauge().GetValue())
				}
			}
		}
	}
}