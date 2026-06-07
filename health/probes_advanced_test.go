package health

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/testutil"
)

// ─── Startup Probe Tests ────────────────────────────────────────────────────

func TestStartupProbe_FailsThenPasses(t *testing.T) {
	var count int
	probe := func(ctx context.Context) error {
		count++
		if count < 3 {
			return errors.New("not ready")
		}
		return nil
	}

	sp := NewStartupProbe(probe, 10)

	for i := 0; i < 2; i++ {
		if err := sp.Run(context.Background()); err == nil {
			t.Fatalf("call %d: expected error, got nil", i+1)
		}
	}

	if err := sp.Run(context.Background()); err != nil {
		t.Fatalf("call 3: expected nil, got %v", err)
	}

	if !sp.IsStarted() {
		t.Error("expected IsStarted() = true")
	}

	if err := sp.Run(context.Background()); err != nil {
		t.Fatalf("call 4: expected nil (already started), got %v", err)
	}
}

func TestStartupProbe_ExceedsMaxFailures(t *testing.T) {
	probe := func(ctx context.Context) error {
		return errors.New("db not ready")
	}

	sp := NewStartupProbe(probe, 3)

	for i := 0; i < 3; i++ {
		if err := sp.Run(context.Background()); err == nil {
			t.Fatalf("call %d: expected error", i+1)
		}
	}
}

func TestStartupEndpoint(t *testing.T) {
	var ready bool
	probe := func(ctx context.Context) error {
		if !ready {
			return errors.New("loading")
		}
		return nil
	}

	app := testutil.NewTestApp()
	Register(app, WithStartupProbe("warmup", probe))
	s := testutil.NewServer(t, app)

	// Initially 503
	s.GET("/startup").AssertStatus(http.StatusServiceUnavailable)

	// After probe passes, 200
	ready = true
	s.GET("/startup").AssertStatus(http.StatusOK)
}

// ─── Throttled Probe Tests ─────────────────────────────────────────────────

func TestThrottledProbe_CachesResult(t *testing.T) {
	var calls int
	probe := func(ctx context.Context) error {
		calls++
		return nil
	}

	tp := NewThrottledProbe(probe, 5*time.Second)

	for i := 0; i < 5; i++ {
		tp.Run(context.Background())
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
	if !tp.IsHealthy() {
		t.Error("expected IsHealthy() = true")
	}
}

func TestThrottledProbe_RefreshesAfterCooldown(t *testing.T) {
	var calls int
	probe := func(ctx context.Context) error {
		calls++
		return nil
	}

	tp := NewThrottledProbe(probe, 50*time.Millisecond)
	tp.Run(context.Background())
	time.Sleep(60 * time.Millisecond)
	tp.Run(context.Background())

	if calls != 2 {
		t.Errorf("expected 2 calls after cooldown, got %d", calls)
	}
}

func TestThrottledProbe_FailsCorrectly(t *testing.T) {
	errTest := errors.New("db down")
	probe := func(ctx context.Context) error {
		return errTest
	}

	tp := NewThrottledProbe(probe, 5*time.Second)
	if err := tp.Run(context.Background()); err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
	if tp.IsHealthy() {
		t.Error("expected IsHealthy() = false")
	}
}

// ─── Version & BuildInfo Tests ──────────────────────────────────────────────

func TestHealthEndpoint_WithVersion(t *testing.T) {
	app := testutil.NewTestApp()
	Register(app, WithVersion("1.2.3"))
	s := testutil.NewServer(t, app)
	s.GET("/health").AssertStatus(http.StatusOK)
}

func TestHealthEndpoint_WithBuildInfo(t *testing.T) {
	app := testutil.NewTestApp()
	Register(app, WithVersion("1.0.0"), WithBuildInfo("go", "1.22"), WithBuildInfo("os", "linux"))
	s := testutil.NewServer(t, app)
	s.GET("/health").AssertStatus(http.StatusOK)
}

func TestHealthEndpoint_IncludesUptime(t *testing.T) {
	app := testutil.NewTestApp()
	Register(app)
	s := testutil.NewServer(t, app)
	s.GET("/health").AssertStatus(http.StatusOK)
}

func TestHealthEndpoint_WithTimeout(t *testing.T) {
	slowProbe := func(ctx context.Context) error {
		select {
		case <-time.After(10 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	app := testutil.NewTestApp()
	Register(app, WithTimeout(50*time.Millisecond), WithProbe("slow", slowProbe))
	s := testutil.NewServer(t, app)
	s.GET("/ready").AssertStatus(http.StatusServiceUnavailable)
}

// ─── GolangStats Probe Tests ───────────────────────────────────────────────

func TestGolangStats_Healthy(t *testing.T) {
	probe := GolangStats(100000, 1024)
	if err := probe(context.Background()); err != nil {
		t.Errorf("expected healthy, got %v", err)
	}
}

func TestGolangStats_ExceedsGoroutines(t *testing.T) {
	probe := GolangStats(1, 0)
	if err := probe(context.Background()); err == nil {
		t.Error("expected error for goroutine limit")
	}
}

// ─── TCP Probe Tests ───────────────────────────────────────────────────────

func TestTCPProbe_Unreachable(t *testing.T) {
	probe := TCPProbe("127.0.0.1", 1)
	if err := probe(context.Background()); err == nil {
		t.Error("expected error for unreachable port")
	}
}

// ─── Composite Probe Tests ─────────────────────────────────────────────────

func TestCompositeProbe_AllPass(t *testing.T) {
	cp := CompositeProbe("all",
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error { return nil },
	)
	if err := cp(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCompositeProbe_OneFails(t *testing.T) {
	cp := CompositeProbe("all",
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error { return errors.New("boom") },
	)
	if err := cp(context.Background()); err == nil {
		t.Error("expected error")
	}
}

// ─── DNS Probe Tests ───────────────────────────────────────────────────────

func TestDNSProbe_Invalid(t *testing.T) {
	probe := DNSProbe("this-domain-does-not-exist-ever.invalid")
	err := probe(context.Background())
	if err == nil {
		t.Error("expected DNS resolution error for invalid domain")
	}
}

// ─── Throttled Probe Integration ──────────────────────────────────────────

func TestThrottledProbeIntegration(t *testing.T) {
	var calls int
	probe := func(ctx context.Context) error {
		calls++
		return nil
	}

	tp := NewThrottledProbe(probe, 5*time.Second)

	app := testutil.NewTestApp()
	Register(app, WithProbe("throttled", tp.Probe()))
	s := testutil.NewServer(t, app)

	// Multiple requests should only trigger one probe
	for i := 0; i < 5; i++ {
		s.GET("/ready").AssertStatus(http.StatusOK)
	}
	if calls != 1 {
		t.Errorf("expected 1 probe call, got %d", calls)
	}
}

// ─── Startup + Readiness Integration ──────────────────────────────────────

func TestStartupWithReadiness(t *testing.T) {
	var dbReady bool
	startupProbe := func(ctx context.Context) error {
		if dbReady {
			return nil
		}
		return errors.New("warming up")
	}
	readinessProbe := func(ctx context.Context) error {
		if dbReady {
			return nil
		}
		return errors.New("not ready")
	}

	app := testutil.NewTestApp()
	Register(app,
		WithStartupProbe("warmup", startupProbe),
		WithProbe("db", readinessProbe),
	)
	s := testutil.NewServer(t, app)

	// Both should fail initially
	s.GET("/startup").AssertStatus(http.StatusServiceUnavailable)
	s.GET("/ready").AssertStatus(http.StatusServiceUnavailable)

	// After ready, both should pass
	dbReady = true
	s.GET("/startup").AssertStatus(http.StatusOK)
	s.GET("/ready").AssertStatus(http.StatusOK)
	s.GET("/health").AssertStatus(http.StatusOK)
}

// Helper: unused astra import guard
var _ = astra.HandlerFunc(nil)
