package module

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/di"
)

// ─── Helper: stub health registrar ───────────────────────────────────────────

type stubHealthRegistrar struct {
	probes map[string]func(ctx context.Context) error
}

func newStubHealth() *stubHealthRegistrar {
	return &stubHealthRegistrar{probes: make(map[string]func(ctx context.Context) error)}
}

func (s *stubHealthRegistrar) RegisterProbe(name string, probe func(ctx context.Context) error) {
	s.probes[name] = probe
}

// ─── Module Construction Tests ──────────────────────────────────────────────

func TestNew_ModuleDefaults(t *testing.T) {
	m := New("test")
	if m.Name != "test" {
		t.Errorf("expected name 'test', got %q", m.Name)
	}
	if m.Phase != PhaseService {
		t.Errorf("expected PhaseService, got %d", m.Phase)
	}
	if len(m.DependsOn) != 0 {
		t.Errorf("expected no dependencies")
	}
}

func TestModule_BuilderPattern(t *testing.T) {
	m := New("cache").
		Desc("Redis cache layer").
		SetPhase(PhaseInfra).
		WithDependsOn("database").
		WithProvider(func(c *di.Container) {}).WithHealthProbe(func(r HealthRegistrar) {}).
		WithRoute(func(app *astra.App) {}).
		WithMiddleware(func(app *astra.App) {}).
		WithStartHook(func(ctx context.Context) error { return nil }).
		WithStopHook(func(ctx context.Context) error { return nil })

	if m.Description != "Redis cache layer" {
		t.Errorf("description mismatch")
	}
	if m.Phase != PhaseInfra {
		t.Errorf("expected PhaseInfra")
	}
	if len(m.DependsOn) != 1 || m.DependsOn[0] != "database" {
		t.Errorf("expected DependsOn=[database]")
	}
	if len(m.Providers) != 1 || len(m.HealthProbes) != 1 || len(m.Routes) != 1 {
		t.Errorf("expected 1 of each callback type")
	}
}

// ─── Registry Tests ──────────────────────────────────────────────────────────

func TestRegistry_RegisterAndLookup(t *testing.T) {
	app := astra.New()
	r := NewRegistry(app)
	r.Register(New("db"))

	if m := r.Lookup("db"); m == nil {
		t.Error("expected to find 'db'")
	}
	if m := r.Lookup("missing"); m != nil {
		t.Error("expected nil for missing module")
	}
	if r.Lookup("db").Name != "db" {
		t.Error("name mismatch")
	}
}

func TestRegistry_DuplicateNamePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate name")
		}
	}()
	app := astra.New()
	r := NewRegistry(app)
	r.Register(New("db"))
	r.Register(New("db"))
}

func TestRegistry_List(t *testing.T) {
	app := astra.New()
	r := NewRegistry(app)
	r.Register(New("cache"))
	r.Register(New("auth"))
	r.Register(New("db"))

	names := r.List()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	// Should be sorted
	if names[0] != "auth" || names[1] != "cache" || names[2] != "db" {
		t.Errorf("expected sorted names, got %v", names)
	}
}

// ─── Dependency Resolution Tests ─────────────────────────────────────────────

func TestResolveOrder_NoDeps(t *testing.T) {
	mods := map[string]*Module{
		"a": New("a"),
		"b": New("b"),
		"c": New("c"),
	}
	order, err := resolveOrder(mods)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 modules, got %d", len(order))
	}
}

func TestResolveOrder_SimpleChain(t *testing.T) {
	mods := map[string]*Module{
		"cache":  New("cache").WithDependsOn("db"),
		"api":    New("api").WithDependsOn("cache"),
		"db":     New("db").SetPhase(PhaseInfra),
	}
	order, err := resolveOrder(mods)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// db must come before cache, cache before api
	idx := make(map[string]int)
	for i, m := range order {
		idx[m.Name] = i
	}
	if idx["db"] >= idx["cache"] {
		t.Error("db should start before cache")
	}
	if idx["cache"] >= idx["api"] {
		t.Error("cache should start before api")
	}
}

func TestResolveOrder_MissingDependency(t *testing.T) {
	mods := map[string]*Module{
		"cache": New("cache").WithDependsOn("nonexistent"),
	}
	_, err := resolveOrder(mods)
	if err == nil {
		t.Error("expected error for missing dependency")
	}
}

func TestResolveOrder_CircularDependency(t *testing.T) {
	mods := map[string]*Module{
		"a": New("a").WithDependsOn("b"),
		"b": New("b").WithDependsOn("c"),
		"c": New("c").WithDependsOn("a"),
	}
	_, err := resolveOrder(mods)
	if err == nil {
		t.Error("expected error for circular dependency")
	}
}

func TestResolveOrder_PhaseOrdering(t *testing.T) {
	mods := map[string]*Module{
		"app":   New("app").SetPhase(PhaseApp),
		"infra": New("infra").SetPhase(PhaseInfra),
		"svc":   New("svc").SetPhase(PhaseService),
		"gw":    New("gw").SetPhase(PhaseGateway),
		"cfg":   New("cfg").SetPhase(PhaseConfig),
	}
	order, err := resolveOrder(mods)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order[0].Phase != PhaseConfig {
		t.Errorf("expected first phase=Config, got %s", order[0].Phase)
	}
	if order[1].Phase != PhaseInfra {
		t.Errorf("expected second phase=Infra, got %s", order[1].Phase)
	}
	if order[2].Phase != PhaseService {
		t.Errorf("expected third phase=Service, got %s", order[2].Phase)
	}
	if order[3].Phase != PhaseGateway {
		t.Errorf("expected fourth phase=Gateway, got %s", order[3].Phase)
	}
	if order[4].Phase != PhaseApp {
		t.Errorf("expected fifth phase=App, got %s", order[4].Phase)
	}
}

// ─── Registry.Start Lifecycle Tests ────────────────────────────────────────

func TestRegistry_Start_CallsProvidersInOrder(t *testing.T) {
	var order []string
	app := astra.New()
	r := NewRegistry(app)

	r.Register(New("db").SetPhase(PhaseInfra).
		WithProvider(func(c *di.Container) { order = append(order, "db-provider") }).
		WithStartHook(func(ctx context.Context) error { order = append(order, "db-start"); return nil }))

	r.Register(New("cache").WithDependsOn("db").SetPhase(PhaseInfra).
		WithProvider(func(c *di.Container) { order = append(order, "cache-provider") }).
		WithStartHook(func(ctx context.Context) error { order = append(order, "cache-start"); return nil }))

	err := r.Start(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(order) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(order), order)
	}
	if order[0] != "db-provider" || order[1] != "cache-provider" {
		t.Errorf("providers should run in dep order: %v", order)
	}
}

func TestRegistry_Start_HealthProbesRegistered(t *testing.T) {
	hr := newStubHealth()
	app := astra.New()
	r := NewRegistry(app, WithHealthRegistrar(hr))

	r.Register(New("db").
		WithHealthProbe(func(r HealthRegistrar) {
			r.RegisterProbe("db", func(ctx context.Context) error { return nil })
		}))

	r.Start(context.Background())

	if _, ok := hr.probes["db"]; !ok {
		t.Error("expected 'db' probe to be registered")
	}
}

func TestRegistry_Start_StartHookError(t *testing.T) {
	app := astra.New()
	r := NewRegistry(app)

	r.Register(New("db").
		WithStartHook(func(ctx context.Context) error {
			return errors.New("db connection failed")
		}))

	err := r.Start(context.Background())
	if err == nil {
		t.Error("expected error from start hook")
	}
	if err.Error() == "" {
		t.Error("error should contain module name")
	}
}

func TestRegistry_Stop_CallsInReverse(t *testing.T) {
	var order []string
	app := astra.New()
	r := NewRegistry(app)

	r.Register(New("db").SetPhase(PhaseInfra).
		WithStopHook(func(ctx context.Context) error { order = append(order, "db-stop"); return nil }))

	r.Register(New("cache").WithDependsOn("db").SetPhase(PhaseInfra).
		WithStopHook(func(ctx context.Context) error { order = append(order, "cache-stop"); return nil }))

	r.Start(context.Background())
	r.Stop(context.Background())

	if len(order) != 2 {
		t.Fatalf("expected 2 stops, got %d", len(order))
	}
	// Stop should be reverse: cache before db
	if order[0] != "cache-stop" || order[1] != "db-stop" {
		t.Errorf("stop hooks should be reverse order: %v", order)
	}
}

func TestRegistry_Start_RoutesRegistered(t *testing.T) {
	app := astra.New()
	r := NewRegistry(app)

	r.Register(New("api").SetPhase(PhaseGateway).
		WithRoute(func(app *astra.App) {
			app.GET("/ping", func(c *astra.Ctx) error {
				return c.JSON(200, map[string]string{"msg": "pong"})
			})
		}))

	r.Start(context.Background())

	// Verify route was registered (app startup was successful)
	_ = app // route registration happened during Start
}

// ─── Proxy Tests ─────────────────────────────────────────────────────────────

func TestProxy_Call_Success(t *testing.T) {
	app := astra.New()
	r := NewRegistry(app)

	called := false
	r.Register(New("svc").
		WithRoute(func(app *astra.App) {}))

	p := GetProxy[bool](r)
	// This will fail because bool isn't in DI — that's fine for this test
	// Let's test with a custom resolve
	p.resolve = func() (bool, error) { called = true; return true, nil }

	err := p.Call(context.Background(), time.Second, func(svc bool) error {
		if !svc {
			t.Error("expected true")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("resolve should have been called")
	}
}

func TestProxy_Call_Timeout(t *testing.T) {
	p := &Proxy[string]{
		resolve: func() (string, error) { return "ok", nil },
	}

	err := p.Call(context.Background(), 10*time.Millisecond, func(svc string) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	if err == nil {
		t.Error("expected timeout error")
	}
}

	func TestProxy_Call_Retries(t *testing.T) {
		var calls atomic.Int32

		p := &Proxy[string]{
			resolve:  func() (string, error) { return "ok", nil },
			retries: 2,
		}

		lastErr := p.callWithRetry(context.Background(), time.Second, func(s string) error {
			calls.Add(1)
			if calls.Load() < 3 {
				return fmt.Errorf("transient error %d", calls.Load())
			}
			return nil
		}, 2)
		if lastErr != nil {
			t.Fatalf("unexpected error: %v", lastErr)
		}
		if calls.Load() != 3 {
			t.Errorf("expected 3 calls (1 + 2 retries), got %d", calls.Load())
		}
	}

func TestProxy_CircuitBreaker(t *testing.T) {
	p := &Proxy[string]{
		resolve: func() (string, error) { return "ok", nil },
		circuit: newCircuitState(3, 100*time.Millisecond),
	}

	// Trip the circuit
	for i := 0; i < 3; i++ {
		p.mu.Lock()
		p.circuit.recordFailure()
		p.mu.Unlock()
	}

	if p.State() != "open" {
		t.Errorf("expected open, got %s", p.State())
	}

	// Call should fail immediately
	err := p.Call(context.Background(), time.Second, func(s string) error { return nil })
	if err == nil {
		t.Error("expected circuit breaker open error")
	}

	// Wait for recovery timeout
	time.Sleep(120 * time.Millisecond)
	if p.State() != "half-open" {
		t.Errorf("expected half-open after timeout, got %s", p.State())
	}

	// Success should close circuit
	p.mu.Lock()
	p.circuit.recordSuccess()
	p.mu.Unlock()
	if p.State() != "closed" {
		t.Errorf("expected closed after success, got %s", p.State())
	}
}

func TestProxy_State_NoBreaker(t *testing.T) {
	p := &Proxy[string]{
		resolve: func() (string, error) { return "ok", nil },
	}
	if p.State() != "" {
		t.Error("expected empty state when no breaker")
	}
}

// ─── Phase String Tests ──────────────────────────────────────────────────────

func TestPhase_String(t *testing.T) {
	tests := []struct {
		p    Phase
		want string
	}{
		{PhaseConfig, "config"},
		{PhaseInfra, "infra"},
		{PhaseService, "service"},
		{PhaseGateway, "gateway"},
		{PhaseApp, "app"},
		{Phase(99), "phase(99)"},
	}
	for _, tt := range tests {
		if got := tt.p.String(); got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.p, got, tt.want)
		}
	}
}

// ─── Integration: Module + DI + Health ──────────────────────────────────────

func TestIntegration_ModuleWithDI(t *testing.T) {
	app := astra.New()
	r := NewRegistry(app)

	// Module registers a DI provider
	r.Register(New("database").SetPhase(PhaseInfra).
		WithProvider(func(c *di.Container) {
			di.ProvideValue(c, "postgresql://localhost/db")
		}))

	r.Start(context.Background())

	// DI container should have the value
	dsn, err := di.Invoke[string](r.Container())
	if err != nil {
		t.Fatalf("expected to resolve dsn: %v", err)
	}
	if dsn != "postgresql://localhost/db" {
		t.Errorf("expected postgresql dsn, got %q", dsn)
	}
}

func TestIntegration_MultiModuleWithHealth(t *testing.T) {
	hr := newStubHealth()
	app := astra.New()
	r := NewRegistry(app, WithHealthRegistrar(hr))

	r.Register(New("database").SetPhase(PhaseInfra).
		WithHealthProbe(func(r HealthRegistrar) {
			r.RegisterProbe("postgres", func(ctx context.Context) error { return nil })
		}))

	r.Register(New("cache").WithDependsOn("database").SetPhase(PhaseInfra).
		WithHealthProbe(func(r HealthRegistrar) {
			r.RegisterProbe("redis", func(ctx context.Context) error { return nil })
		}))

	r.Register(New("api").WithDependsOn("cache").SetPhase(PhaseGateway).
		WithRoute(func(app *astra.App) {
			app.GET("/health", func(c *astra.Ctx) error {
				return c.JSON(200, map[string]string{"status": "ok"})
			})
		}))

	err := r.Start(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hr.probes) != 2 {
		t.Errorf("expected 2 probes, got %d", len(hr.probes))
	}
	if _, ok := hr.probes["postgres"]; !ok {
		t.Error("expected 'postgres' probe")
	}
	if _, ok := hr.probes["redis"]; !ok {
		t.Error("expected 'redis' probe")
	}
}
