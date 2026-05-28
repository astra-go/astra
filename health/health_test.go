package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astra-go/astra/health"
	"github.com/astra-go/astra/testutil"
)

// ─── /health endpoint ─────────────────────────────────────────────────────────

func TestHealth_NoProbes_Returns200(t *testing.T) {
	app := testutil.NewTestApp()
	health.Register(app)
	s := testutil.NewServer(t, app)

	s.GET("/health").AssertStatus(http.StatusOK)
}

func TestHealth_AllProbesPass_Returns200(t *testing.T) {
	app := testutil.NewTestApp()
	health.Register(app,
		health.WithProbe("db", func(_ context.Context) error { return nil }),
		health.WithProbe("cache", func(_ context.Context) error { return nil }),
	)
	s := testutil.NewServer(t, app)

	resp := s.GET("/health")
	resp.AssertStatus(http.StatusOK)
	resp.AssertBodyContains(`"status":"ok"`)
	resp.AssertBodyContains(`"live":true`)
	resp.AssertBodyContains(`"ready":true`)
}

func TestHealth_FailingProbe_Returns503(t *testing.T) {
	app := testutil.NewTestApp()
	health.Register(app,
		health.WithProbe("db", func(_ context.Context) error {
			return errors.New("connection refused")
		}),
	)
	s := testutil.NewServer(t, app)

	resp := s.GET("/health")
	resp.AssertStatus(http.StatusServiceUnavailable)
	resp.AssertBodyContains(`"status":"degraded"`)
	resp.AssertBodyContains(`"ready":false`)
}

// ─── /ready endpoint ──────────────────────────────────────────────────────────

func TestReady_FailingProbe_Returns503(t *testing.T) {
	app := testutil.NewTestApp()
	health.Register(app,
		health.WithProbe("redis", func(_ context.Context) error {
			return errors.New("timeout")
		}),
	)
	s := testutil.NewServer(t, app)

	resp := s.GET("/ready")
	resp.AssertStatus(http.StatusServiceUnavailable)
	resp.AssertBodyContains("degraded")
}

// ─── WithPrefix ───────────────────────────────────────────────────────────────

func TestRegister_WithPrefix(t *testing.T) {
	app := testutil.NewTestApp()
	health.Register(app, health.WithPrefix("/internal"))
	s := testutil.NewServer(t, app)

	s.GET("/internal/live").AssertStatus(http.StatusOK)
	s.GET("/internal/ready").AssertStatus(http.StatusOK)
	s.GET("/internal/health").AssertStatus(http.StatusOK)
}

// ─── NewModule ────────────────────────────────────────────────────────────────

func TestNewModule_RegistersRoutes(t *testing.T) {
	app := testutil.NewTestApp()
	if err := app.RegisterModule(health.NewModule()); err != nil {
		t.Fatalf("RegisterModule: %v", err)
	}
	s := testutil.NewServer(t, app)

	s.GET("/live").AssertStatus(http.StatusOK)
	s.GET("/ready").AssertStatus(http.StatusOK)
	s.GET("/health").AssertStatus(http.StatusOK)
}

func TestNewModule_WithProbe(t *testing.T) {
	app := testutil.NewTestApp()
	if err := app.RegisterModule(health.NewModule(
		health.WithProbe("svc", func(_ context.Context) error { return nil }),
	)); err != nil {
		t.Fatalf("RegisterModule: %v", err)
	}
	s := testutil.NewServer(t, app)
	s.GET("/ready").AssertStatus(http.StatusOK)
}

// ─── NewIstioModule ───────────────────────────────────────────────────────────

func TestNewIstioModule_RegistersAllRoutes(t *testing.T) {
	app := testutil.NewTestApp()
	if err := app.RegisterModule(health.NewIstioModule()); err != nil {
		t.Fatalf("RegisterModule: %v", err)
	}
	s := testutil.NewServer(t, app)

	s.GET("/live").AssertStatus(http.StatusOK)
	s.GET("/ready").AssertStatus(http.StatusOK)
	s.GET("/health").AssertStatus(http.StatusOK)
	s.GET("/healthz/live").AssertStatus(http.StatusOK)
	s.GET("/healthz/ready").AssertStatus(http.StatusOK)
}

// ─── RedisProbe ───────────────────────────────────────────────────────────────

type mockPingResult struct{ err error }

func (m mockPingResult) Err() error { return m.err }

type mockRedisClient struct{ err error }

func (m mockRedisClient) Ping(_ context.Context) interface{ Err() error } {
	return mockPingResult{err: m.err}
}

func TestRedisProbe_Success(t *testing.T) {
	probe := health.RedisProbe(mockRedisClient{err: nil})
	if err := probe(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestRedisProbe_Failure(t *testing.T) {
	probe := health.RedisProbe(mockRedisClient{err: errors.New("dial tcp: refused")})
	if err := probe(context.Background()); err == nil {
		t.Error("expected error, got nil")
	}
}

// ─── HTTPProbe ────────────────────────────────────────────────────────────────

func TestHTTPProbe_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	probe := health.HTTPProbe(ts.URL)
	if err := probe(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestHTTPProbe_ServerError_ReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	probe := health.HTTPProbe(ts.URL)
	if err := probe(context.Background()); err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestHTTPProbe_Unreachable_ReturnsError(t *testing.T) {
	probe := health.HTTPProbe("http://127.0.0.1:1") // nothing listening
	if err := probe(context.Background()); err == nil {
		t.Error("expected error for unreachable host, got nil")
	}
}
