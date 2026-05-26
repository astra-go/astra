package health_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/astra-go/astra/health"
	"github.com/astra-go/astra/testutil"
)

// ─── RegisterIstioProbes ──────────────────────────────────────────────────────

func TestRegisterIstioProbes_LivenessPath(t *testing.T) {
	app := testutil.NewTestApp()
	health.RegisterIstioProbes(app)
	s := testutil.NewServer(t, app)

	s.GET("/healthz/live").AssertStatus(http.StatusOK)
}

func TestRegisterIstioProbes_ReadinessPath(t *testing.T) {
	app := testutil.NewTestApp()
	health.RegisterIstioProbes(app)
	s := testutil.NewServer(t, app)

	s.GET("/healthz/ready").AssertStatus(http.StatusOK)
}

func TestRegisterIstioProbes_WithProbe_ReadyReturns200_WhenHealthy(t *testing.T) {
	app := testutil.NewTestApp()
	health.RegisterIstioProbes(app,
		health.WithProbe("db", func(_ context.Context) error { return nil }),
	)
	s := testutil.NewServer(t, app)
	s.GET("/healthz/ready").AssertStatus(http.StatusOK)
}

func TestRegisterIstioProbes_WithPrefix(t *testing.T) {
	app := testutil.NewTestApp()
	health.RegisterIstioProbes(app, health.WithPrefix("/internal"))
	s := testutil.NewServer(t, app)

	s.GET("/internal/healthz/live").AssertStatus(http.StatusOK)
	s.GET("/internal/healthz/ready").AssertStatus(http.StatusOK)
}

func TestRegisterIstioProbes_DoesNotOverrideStandardPaths(t *testing.T) {
	app := testutil.NewTestApp()
	// Register standard K8s paths first.
	health.Register(app)
	// Then register Istio paths — must not conflict.
	health.RegisterIstioProbes(app)
	s := testutil.NewServer(t, app)

	// Both sets of paths must respond successfully.
	s.GET("/live").AssertStatus(http.StatusOK)
	s.GET("/ready").AssertStatus(http.StatusOK)
	s.GET("/healthz/live").AssertStatus(http.StatusOK)
	s.GET("/healthz/ready").AssertStatus(http.StatusOK)
}

// ─── WithIstioHeaders ─────────────────────────────────────────────────────────

func TestRegisterIstioProbes_WithIstioHeaders_InjectsHeaders(t *testing.T) {
	app := testutil.NewTestApp()
	health.RegisterIstioProbes(app, health.WithIstioHeaders())
	s := testutil.NewServer(t, app)

	resp := s.GET("/healthz/live")
	resp.AssertStatus(http.StatusOK)

	if resp.Header("x-content-type-options") == "" {
		t.Error("expected x-content-type-options header from WithIstioHeaders")
	}
	if resp.Header("x-envoy-upstream-service-time") == "" {
		t.Error("expected x-envoy-upstream-service-time header from WithIstioHeaders")
	}
}

func TestRegisterIstioProbes_WithoutIstioHeaders_NoIstioHeaders(t *testing.T) {
	app := testutil.NewTestApp()
	health.RegisterIstioProbes(app) // no WithIstioHeaders
	s := testutil.NewServer(t, app)

	resp := s.GET("/healthz/live")
	resp.AssertStatus(http.StatusOK)
	// Standard registration must NOT inject Istio headers.
	if resp.Header("x-envoy-upstream-service-time") != "" {
		t.Error("unexpected x-envoy-upstream-service-time header without WithIstioHeaders")
	}
}

func TestRegisterIstioProbes_BothHeaders_OnReady(t *testing.T) {
	app := testutil.NewTestApp()
	health.RegisterIstioProbes(app, health.WithIstioHeaders())
	s := testutil.NewServer(t, app)

	resp := s.GET("/healthz/ready")
	resp.AssertStatus(http.StatusOK)
	if resp.Header("x-content-type-options") == "" {
		t.Error("expected x-content-type-options on /healthz/ready")
	}
}
