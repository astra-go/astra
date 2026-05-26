// logger_metrics_tracing_ratelimit_test.go — additional middleware tests:
// Logger, Metrics, Tracing, SlidingWindow, RouteQuotaMiddleware.
// These are separate from middleware_test.go to keep file size manageable.
package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	mwobs "github.com/astra-go/astra/middleware/observability"
	sec "github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace/noop"
)

// ─── Logger ───────────────────────────────────────────────────────────────────

// bufHandler is a minimal slog.Handler that writes structured key=value to a buffer.
type bufHandler struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

func newBufLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), buf
}

func TestLogger_PassesRequestThrough(t *testing.T) {
	app := testutil.NewTestApp()
	logger, _ := newBufLogger()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Logger: logger,
	}))
	app.GET("/ping", func(c *astra.Ctx) error { return c.String(http.StatusOK, "pong") })
	s := testutil.NewServer(t, app)

	s.GET("/ping").AssertStatus(http.StatusOK).AssertBodyContains("pong")
}

func TestLogger_SkipsConfiguredPaths(t *testing.T) {
	app := testutil.NewTestApp()
	logger, buf := newBufLogger()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Logger:    logger,
		SkipPaths: []string{"/health"},
	}))
	app.GET("/health", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	app.GET("/info", func(c *astra.Ctx) error { return c.String(http.StatusOK, "info") })
	s := testutil.NewServer(t, app)

	s.GET("/health")
	healthLog := buf.String()

	s.GET("/info")
	infoLog := buf.String()

	if strings.Contains(healthLog, "/health") {
		t.Error("Logger should not log /health (in SkipPaths)")
	}
	if !strings.Contains(infoLog, "/info") {
		t.Error("Logger should log /info (not in SkipPaths)")
	}
}

func TestLogger_SanitizesQueryParams(t *testing.T) {
	app := testutil.NewTestApp()
	logger, buf := newBufLogger()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Logger:          logger,
		SensitiveParams: []string{"token"},
	}))
	app.GET("/search", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/search?q=hello&token=supersecret")

	logged := buf.String()
	if strings.Contains(logged, "supersecret") {
		t.Errorf("sensitive param 'token' value should be redacted in log, got: %s", logged)
	}
	if !strings.Contains(logged, "REDACTED") {
		t.Errorf("expected REDACTED in log, got: %s", logged)
	}
}

func TestLogger_LogsStatusAndMethod(t *testing.T) {
	app := testutil.NewTestApp()
	logger, buf := newBufLogger()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{Logger: logger}))
	app.GET("/data", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/data")

	logged := buf.String()
	if !strings.Contains(logged, "GET") {
		t.Errorf("expected method GET in log, got: %s", logged)
	}
	if !strings.Contains(logged, "/data") {
		t.Errorf("expected path /data in log, got: %s", logged)
	}
}

// ─── Metrics ──────────────────────────────────────────────────────────────────

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

// ─── Tracing ──────────────────────────────────────────────────────────────────

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

	// Just verify no crash and correct response.
	s.GET("/thing").AssertStatus(http.StatusOK)
}

// ─── SlidingWindow ────────────────────────────────────────────────────────────

func TestSlidingWindow_AllowsWithinLimit(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   5,
		Window:  time.Second,
		KeyFunc: func(_ *astra.Ctx) string { return "fixed" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	for i := 0; i < 5; i++ {
		s.GET("/").AssertStatus(http.StatusOK)
	}
}

func TestSlidingWindow_RejectsWhenExceeded(t *testing.T) {
	app := testutil.NewTestApp()
	// Limit of 1 request per very long window.
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   1,
		Window:  time.Hour,
		KeyFunc: func(_ *astra.Ctx) string { return "fixed" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
	s.GET("/").AssertStatus(http.StatusTooManyRequests)
}

func TestSlidingWindow_IndependentKeys(t *testing.T) {
	key := "key-a"
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   1,
		Window:  time.Hour,
		KeyFunc: func(_ *astra.Ctx) string { return key },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	// Switch to a different key — new sliding window, should allow.
	key = "key-b"
	s.GET("/").AssertStatus(http.StatusOK)
}

func TestSlidingWindow_PerKeyLimits(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:  10,
		Window: time.Hour,
		KeyFunc: func(c *astra.Ctx) string {
			return c.Header("X-API-Key")
		},
		PerKeyLimits: map[string]int64{
			"free-key": 1,
		},
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	// "free-key" hits its 1-request limit.
	s.GET("/", map[string]string{"X-API-Key": "free-key"}).AssertStatus(http.StatusOK)
	s.GET("/", map[string]string{"X-API-Key": "free-key"}).AssertStatus(http.StatusTooManyRequests)

	// "premium-key" falls back to default limit (10) and should pass.
	s.GET("/", map[string]string{"X-API-Key": "premium-key"}).AssertStatus(http.StatusOK)
}

// ─── RouteQuotaMiddleware ─────────────────────────────────────────────────────

func TestRouteQuota_AppliesPerRouteLimit(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.RouteQuotaMiddleware(sec.RouteQuotaConfig{
		Routes: []sec.RouteQuota{
			{Prefix: "/api/upload", Limit: 1, Window: time.Hour},
		},
		DefaultLimit:  100,
		DefaultWindow: time.Hour,
		KeyFunc:       func(_ *astra.Ctx) string { return "user" },
	}))
	app.POST("/api/upload", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.POST("/api/upload", nil).AssertStatus(http.StatusOK)
	s.POST("/api/upload", nil).AssertStatus(http.StatusTooManyRequests)
}

func TestRouteQuota_DefaultLimitAppliesWhenNoMatch(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.RouteQuotaMiddleware(sec.RouteQuotaConfig{
		Routes: []sec.RouteQuota{
			{Prefix: "/slow", Limit: 1, Window: time.Hour},
		},
		DefaultLimit:  1,
		DefaultWindow: time.Hour,
		KeyFunc:       func(_ *astra.Ctx) string { return "user" },
	}))
	app.GET("/other", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/other").AssertStatus(http.StatusOK)
	s.GET("/other").AssertStatus(http.StatusTooManyRequests)
}

func TestRouteQuota_PrefixBoundaryMatching(t *testing.T) {
	// "/api" prefix should NOT match "/apiv2".
	app := testutil.NewTestApp()
	app.Use(sec.RouteQuotaMiddleware(sec.RouteQuotaConfig{
		Routes: []sec.RouteQuota{
			{Prefix: "/api", Limit: 1, Window: time.Hour},
		},
		DefaultLimit:  100,
		DefaultWindow: time.Hour,
		KeyFunc:       func(_ *astra.Ctx) string { return "user" },
	}))
	app.GET("/apiv2/resource", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	// "/apiv2/resource" does NOT start with "/api/" — should use default limit.
	s.GET("/apiv2/resource").AssertStatus(http.StatusOK)
	s.GET("/apiv2/resource").AssertStatus(http.StatusOK)
}
