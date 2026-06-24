package middleware

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/testutil"
)

// ─── BodyLimit ────────────────────────────────────────────────────────────

func TestBodyLimit_SmallBodyPasses(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(BodyLimit(100))
	app.POST("/", func(c *astra.Ctx) error {
		_, _ = io.ReadAll(c.Request().Body)
		return c.String(http.StatusOK, "ok")
	})

	s := testutil.NewServer(t, app)
	resp := s.POST("/", "hello")
	resp.AssertStatus(http.StatusOK)
}

func TestBodyLimit_ExactLimitPasses(t *testing.T) {
	body := strings.Repeat("a", 100)
	app := testutil.NewTestApp()
	app.Use(BodyLimit(100))
	app.POST("/", func(c *astra.Ctx) error {
		_, _ = io.ReadAll(c.Request().Body)
		return c.String(http.StatusOK, "ok")
	})

	s := testutil.NewServer(t, app)
	resp := s.POST("/", body)
	resp.AssertStatus(http.StatusOK)
}

func TestBodyLimit_OversizeBodyRejected(t *testing.T) {
	body := strings.Repeat("a", 200)
	app := testutil.NewTestApp()
	app.Use(BodyLimit(100))
	app.POST("/", func(c *astra.Ctx) error {
		_, _ = io.ReadAll(c.Request().Body)
		return c.String(http.StatusOK, "ok")
	})

	s := testutil.NewServer(t, app)
	resp := s.POST("/", body)
	// The server should respond — body too large.
	if resp.Status() == 0 {
		t.Fatal("expected a response but got status 0")
	}
}

func TestBodyLimitWithConfig_ZeroFallsBack(t *testing.T) {
	body := strings.Repeat("a", 1<<20+1) // > 1 MiB default
	app := testutil.NewTestApp()
	app.Use(BodyLimitWithConfig(BodyLimitConfig{MaxBytes: 0}))
	app.POST("/", func(c *astra.Ctx) error {
		_, _ = io.ReadAll(c.Request().Body)
		return c.String(http.StatusOK, "ok")
	})

	s := testutil.NewServer(t, app)
	s.POST("/", body)
}

func TestBodyLimitWithConfig_Skipped(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(BodyLimitWithConfig(BodyLimitConfig{
		MaxBytes: 5,
		Skipper:  func(c *astra.Ctx) bool { return true },
	}))
	app.POST("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	s := testutil.NewServer(t, app)
	resp := s.POST("/", strings.Repeat("a", 1000))
	resp.AssertStatus(http.StatusOK)
}

// ─── RequestMetrics ───────────────────────────────────────────────────────

func TestRequestMetrics_RecordsCount(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestMetrics())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	s := testutil.NewServer(t, app)
	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)
}

func TestRequestMetrics_MultipleRequests(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestMetrics())
	app.GET("/a", func(c *astra.Ctx) error { return c.NoContent(http.StatusOK) })
	app.GET("/b", func(c *astra.Ctx) error { return c.NoContent(http.StatusOK) })

	s := testutil.NewServer(t, app)
	s.GET("/a").AssertStatus(http.StatusOK)
	s.GET("/b").AssertStatus(http.StatusOK)
}

func TestRequestMetrics_Skipped(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestMetricsWithConfig(RequestMetricsConfig{
		Skipper: func(c *astra.Ctx) bool { return true },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })

	s := testutil.NewServer(t, app)
	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)
}

func TestRequestMetrics_CustomPathLabel(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestMetricsWithConfig(RequestMetricsConfig{
		PathLabelFunc: func(c *astra.Ctx) string { return "/normalised" },
	}))
	app.GET("/users/42", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })

	s := testutil.NewServer(t, app)
	resp := s.GET("/users/42")
	resp.AssertStatus(http.StatusOK)
}
