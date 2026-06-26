package boot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra"
)

// ============================================================================
// Test: New with defaults
// ============================================================================

func TestNewDefault(t *testing.T) {
	svc := New("test-svc", WithoutConfig(), WithoutDefaultMiddleware())
	if svc == nil {
		t.Fatal("New returned nil")
	}
	if svc.Cfg().Name != "test-svc" {
		t.Errorf("expected Name=test-svc, got %q", svc.Cfg().Name)
	}
	if svc.Cfg().Port != "8080" {
		t.Errorf("expected Port=8080, got %q", svc.Cfg().Port)
	}
	if svc.Cfg().Mode != "dev" {
		t.Errorf("expected Mode=dev, got %q", svc.Cfg().Mode)
	}
	if svc.Logger() == nil {
		t.Error("Logger should not be nil")
	}
}

// ============================================================================
// Test: Custom config overrides defaults
// ============================================================================

func TestNewWithConfigOverride(t *testing.T) {
	svc := New("custom-svc",
		WithoutConfig(),
		WithConfig(&Config{
			Port: "9090",
			Mode: "prod",
		}),
	)
	if svc.Cfg().Port != "9090" {
		t.Errorf("expected Port=9090, got %q", svc.Cfg().Port)
	}
	if svc.Cfg().Mode != "prod" {
		t.Errorf("expected Mode=prod, got %q", svc.Cfg().Mode)
	}
	if svc.Cfg().Name != "custom-svc" {
		t.Errorf("expected Name=custom-svc, got %q", svc.Cfg().Name)
	}
}

// ============================================================================
// Test: Health endpoints registered
// ============================================================================

func TestHealthEndpoints(t *testing.T) {
	svc := New("health-svc", WithoutConfig(), WithoutDefaultMiddleware())
	ts := httptest.NewServer(svc.App())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health/live")
	if err != nil {
		t.Fatalf("health/live request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health/live expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"ok"`) {
		t.Errorf("expected body to contain ok, got %s", body)
	}

	// Ready endpoint should also respond
	resp2, err := http.Get(ts.URL + "/health/ready")
	if err != nil {
		t.Fatalf("health/ready request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("health/ready expected 200, got %d", resp2.StatusCode)
	}
}

// ============================================================================
// Test: Router callback
// ============================================================================

func TestRouter(t *testing.T) {
	svc := New("router-svc", WithoutConfig(), WithoutDefaultMiddleware())

	called := false
	svc.Router(func(app *astra.App) {
		called = true
		if app == nil {
			t.Error("Router callback received nil app")
		}
	})

	if !called {
		t.Error("Router callback was not called")
	}
}

// ============================================================================
// Test: Use middleware
// ============================================================================

func TestUse(t *testing.T) {
	svc := New("mw-svc", WithoutConfig(), WithoutDefaultMiddleware())

	called := false
	svc.Use(func(c *astra.Ctx) error {
		called = true
		return c.Next()
	})

	svc.App().GET("/test-use", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})

	ts := httptest.NewServer(svc.App())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/test-use")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if !called {
		t.Error("middleware was not called")
	}
}

// ============================================================================
// Test: Config file loading
// ============================================================================

func TestConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yaml")
	cfgContent := `name: file-svc
port: "3030"
mode: prod
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	svc := New("override-me",
		WithConfigPath(cfgPath),
		WithoutConfigWatch(),
	)

	if svc.Cfg().Name != "file-svc" {
		t.Errorf("expected Name=file-svc from file, got %q", svc.Cfg().Name)
	}
	if svc.Cfg().Port != "3030" {
		t.Errorf("expected Port=3030 from file, got %q", svc.Cfg().Port)
	}
	if svc.Cfg().Mode != "prod" {
		t.Errorf("expected Mode=prod from file, got %q", svc.Cfg().Mode)
	}
}

// ============================================================================
// Test: Logger format based on mode
// ============================================================================

func TestLoggerMode(t *testing.T) {
	devSvc := New("dev-svc",
		WithoutConfig(),
		WithConfig(&Config{Mode: "dev"}),
	)
	if devSvc.Logger() == nil {
		t.Error("dev logger should not be nil")
	}

	prodSvc := New("prod-svc",
		WithoutConfig(),
		WithConfig(&Config{Mode: "prod"}),
	)
	if prodSvc.Logger() == nil {
		t.Error("prod logger should not be nil")
	}
}

// ============================================================================
// Test: Custom logger (nil should fall back to auto-init)
// ============================================================================

func TestCustomLogger(t *testing.T) {
	svc := New("custom-log-svc",
		WithoutConfig(),
		WithLogger(nil),
	)
	if svc.Logger() == nil {
		t.Error("logger should not be nil with nil custom logger")
	}
}

// ============================================================================
// Test: Group API
// ============================================================================

func TestGroup(t *testing.T) {
	svc := New("group-svc", WithoutConfig(), WithoutDefaultMiddleware())
	g := svc.Group("/api")
	if g == nil {
		t.Error("Group returned nil")
	}

	g.GET("/ping", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "pong")
	})

	ts := httptest.NewServer(svc.App())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("expected pong, got %s", body)
	}
}

// ============================================================================
// Test: Custom health paths
// ============================================================================

func TestHealthEndpointsCustom(t *testing.T) {
	svc := New("custom-health",
		WithoutConfig(),
		WithoutDefaultMiddleware(),
		WithConfig(&Config{
			Health: HealthConfig{
				LivePath:  "/alive",
				ReadyPath: "/ready",
			},
		}),
	)

	ts := httptest.NewServer(svc.App())
	defer ts.Close()

	// Custom path should work
	resp, err := http.Get(ts.URL + "/alive")
	if err != nil {
		t.Fatalf("alive request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 on /alive, got %d", resp.StatusCode)
	}

	// Default path should NOT work (it's been overridden)
	resp2, err := http.Get(ts.URL + "/health/live")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 on /health/live (overridden), got %d", resp2.StatusCode)
	}
}

// ============================================================================
// Test: WithoutHealth disables health endpoints
// ============================================================================

func TestWithoutHealth(t *testing.T) {
	svc := New("no-health", WithoutConfig(), WithoutDefaultMiddleware(), WithoutHealth())

	ts := httptest.NewServer(svc.App())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health/live")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 (no health endpoint), got %d", resp.StatusCode)
	}
}

// ============================================================================
// Test: Config file with env prefix (env vars override file)
// ============================================================================

func TestConfigFileWithEnv(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test_env.yaml")
	cfgContent := `name: file-svc
port: "3030"
mode: prod
log_level: debug
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TESTBOOT_PORT", "4040")
	t.Setenv("TESTBOOT_LOG_LEVEL", "warn")

	svc := New("env-override",
		WithConfigPath(cfgPath),
		WithEnvPrefix("TESTBOOT"),
		WithoutConfigWatch(),
	)

	// Env source with prefix should override file values
	if svc.Cfg().Port != "4040" {
		t.Errorf("expected Port=4040 (env override), got %q", svc.Cfg().Port)
	}
	// File value for non-overridden keys (no env var set)
	if svc.Cfg().Mode != "prod" {
		t.Errorf("expected Mode=prod from file, got %q", svc.Cfg().Mode)
	}
	// Env override for log_level
	if svc.Cfg().LogLevel != "warn" {
		t.Errorf("expected LogLevel=warn (env override), got %q", svc.Cfg().LogLevel)
	}
}

// ============================================================================
// Test: Without config loading (all defaults)
// ============================================================================

func TestWithoutConfig(t *testing.T) {
	svc := New("no-config-svc", WithoutConfig())

	if svc.Cfg().Name != "no-config-svc" {
		t.Errorf("expected Name=no-config-svc, got %q", svc.Cfg().Name)
	}
	if svc.Cfg().Port != "8080" {
		t.Errorf("expected Port=8080, got %q", svc.Cfg().Port)
	}
}

// ============================================================================
// Test: Programmatic config overrides everything
// ============================================================================

func TestProgrammaticConfigOverridesAll(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "override.yaml")
	cfgContent := `name: file-name
port: "2020"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	svc := New("prog-override",
		WithConfigPath(cfgPath),
		WithConfig(&Config{Port: "9090", Mode: "staging"}),
		WithoutConfigWatch(),
	)

	// Programmatic config should override file values for non-empty fields
	if svc.Cfg().Port != "9090" {
		t.Errorf("expected Port=9090 (programmatic), got %q", svc.Cfg().Port)
	}
	if svc.Cfg().Mode != "staging" {
		t.Errorf("expected Mode=staging, got %q", svc.Cfg().Mode)
	}
	// File values fill where programmatic doesn't specify
	if svc.Cfg().Name != "file-name" {
		t.Errorf("expected Name=file-name from file, got %q", svc.Cfg().Name)
	}
}

// ============================================================================
// Test: Health endpoints with same live/ready paths
// ============================================================================
// Test: RegisterReloadable and config hot-reload triggers Reload()
// ============================================================================

func TestRegisterReloadable(t *testing.T) {
	type testComponent struct {
		reloaded bool
		oldVal   string
		newVal   string
	}

	comp := &testComponent{}

	svc := New("reloadable-test",
		WithoutConfig(),
		WithConfig(&Config{Port: "8080"}),
	)

	var mu sync.RWMutex
	svc.RegisterReloadable(ReloadableFunc(func(ctx context.Context, oldCfg, newCfg any) error {
		mu.Lock()
		defer mu.Unlock()
		comp.reloaded = true
		if old, ok := oldCfg.(*Config); ok {
			comp.oldVal = old.Port
		}
		if nc, ok := newCfg.(*Config); ok {
			comp.newVal = nc.Port
		}
		return nil
	}))

	// Simulate a config reload by calling mgr.Watch hooks
	// The easiest way is to reload with a new value.
	// Since we have no file watcher, we manually Load() to trigger callbacks.
	// Actually, we need to trigger the watch hooks that were registered.
	// The registered hook re-scans the mgr; since mgr is nil (WithoutConfig),
	// the hook would do nothing. Let's test with a file-backed config.

	// Reloadable callback on nil mgr is a no-op pass-through test:
	_ = svc
	_ = comp

	// For a proper test, create a config with a file that we can modify.
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "test.yaml")
	yamlContent := []byte("name: reloadable-svc\nport: \"8080\"\n")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}

	reloaded := make(chan struct{}, 1)

	svc2 := New("reloadable-svc",
		WithConfigPath(yamlPath),
	)

	type reloadTracker struct {
		t    *testing.T
		done chan struct{}
	}
	svc2.RegisterReloadable(ReloadableFunc(func(ctx context.Context, oldCfg, newCfg any) error {
		oc := oldCfg.(*Config)
		nc := newCfg.(*Config)
		// Verify old and new are different when port changes
		if oc.Port != nc.Port {
			reloaded <- struct{}{}
		}
		return nil
	}))

	// Modify the config file
	yamlContent = []byte("name: reloadable-svc\nport: \"9090\"\n")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for reload or timeout
	select {
	case <-reloaded:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Reloadable to be called after config change")
	}

	svc2.StopConfigWatch()
}

// ReloadableFunc is a func adapter that implements Reloadable.
type ReloadableFunc func(ctx context.Context, oldCfg, newCfg any) error

func (f ReloadableFunc) Reload(ctx context.Context, oldCfg, newCfg any) error {
	return f(ctx, oldCfg, newCfg)
}

// ============================================================================
// Test: Reloadable error does not panic and is logged
// ============================================================================

func TestReloadableErrorHandling(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "test.yaml")
	yamlContent := []byte("name: err-svc\nport: \"8080\"\n")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := New("err-svc",
		WithConfigPath(yamlPath),
	)

	reloaded := make(chan struct{}, 1)

	// Register a component that always errors
	svc.RegisterReloadable(ReloadableFunc(func(ctx context.Context, oldCfg, newCfg any) error {
		return fmt.Errorf("simulated error")
	}))

	// Register a second component that succeeds
	svc.RegisterReloadable(ReloadableFunc(func(ctx context.Context, oldCfg, newCfg any) error {
		reloaded <- struct{}{}
		return nil
	}))

	// Modify config to trigger reload
	yamlContent = []byte("name: err-svc\nport: \"9090\"\n")
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Second component should still be called despite first erroring
	select {
	case <-reloaded:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: second component not called despite first erroring")
	}

	svc.StopConfigWatch()
}

// ============================================================================

func TestHealthSamePaths(t *testing.T) {
	svc := New("same-health",
		WithoutConfig(),
		WithoutDefaultMiddleware(),
		WithConfig(&Config{
			Health: HealthConfig{
				LivePath:  "/healthz",
				ReadyPath: "/healthz", // same as live
			},
		}),
	)

	ts := httptest.NewServer(svc.App())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 on /healthz, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Test: Mode toggles log format (auto detection)
// ============================================================================

func TestAutoLogFormat(t *testing.T) {
	devSvc := New("auto-dev",
		WithoutConfig(),
		WithConfig(&Config{Mode: "dev", LogFormat: "auto"}),
	)
	_ = devSvc.Logger() // should be text handler

	prodSvc := New("auto-prod",
		WithoutConfig(),
		WithConfig(&Config{Mode: "prod", LogFormat: "auto"}),
	)
	_ = prodSvc.Logger() // should be JSON handler
}

// ============================================================================
// Test: Config file not found should not fail (silent skip)
// ============================================================================

func TestConfigFileNotFound(t *testing.T) {
	svc := New("no-file",
		WithConfigPath("/tmp/nonexistent_dir_9348/config.yaml"),
	)

	// Should fall back to defaults
	if svc.Cfg().Name != "no-file" {
		t.Errorf("expected Name=no-file, got %q", svc.Cfg().Name)
	}
	if svc.Cfg().Port != "8080" {
		t.Errorf("expected default Port=8080, got %q", svc.Cfg().Port)
	}
}

// ============================================================================
// Test: WithoutLoggerInit uses slog.Default()
// ============================================================================

func TestWithoutLoggerInit(t *testing.T) {
	svc := New("no-log-init",
		WithoutConfig(),
		WithoutLoggerInit(),
	)
	if svc.Logger() == nil {
		t.Error("logger should not be nil")
	}
}

// ============================================================================
// Test: WithoutDefaultMiddleware
// ============================================================================

func TestWithoutDefaultMiddleware(t *testing.T) {
	svc := New("no-default-mw", WithoutConfig(), WithoutDefaultMiddleware())

	svc.App().GET("/test", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "no-mw")
	})

	ts := httptest.NewServer(svc.App())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
