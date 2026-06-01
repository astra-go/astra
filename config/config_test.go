package config

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── Concurrent Operations Tests ──────────────────────────────────────────────

func TestConfig_ConcurrentWatch(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := New(&YAMLFile{Path: cfgPath})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var callCount atomic.Int32

	// Register 100 hooks concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg.Watch(func() {
				callCount.Add(1)
			})
		}()
	}
	wg.Wait()

	// Trigger a reload
	if err := cfg.Load(); err != nil {
		t.Fatal(err)
	}

	// Wait for all hooks to execute
	time.Sleep(100 * time.Millisecond)

	// All 100 hooks should have been called
	if count := callCount.Load(); count != 100 {
		t.Errorf("expected 100 hook calls, got %d", count)
	}
}

func TestConfig_ConcurrentLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := New(&YAMLFile{Path: cfgPath})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var errCount atomic.Int32

	// 50 goroutines calling Load() simultaneously
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := cfg.Load(); err != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if count := errCount.Load(); count > 0 {
		t.Errorf("expected 0 errors, got %d", count)
	}
}

func TestConfig_ConcurrentGetAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 8080\ndebug: true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := New(&YAMLFile{Path: cfgPath})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// 20 readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_ = cfg.GetInt("port")
					_ = cfg.GetBool("debug")
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}

	// 5 writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_ = cfg.Load()
					time.Sleep(50 * time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()
}

// ─── Panic Recovery Tests ─────────────────────────────────────────────────────

func TestConfig_WatchHookPanic(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := New(&YAMLFile{Path: cfgPath})
	if err != nil {
		t.Fatal(err)
	}

	var goodHookCalled atomic.Bool

	// Register a hook that panics
	cfg.Watch(func() {
		panic("intentional panic")
	})

	// Register a good hook
	cfg.Watch(func() {
		goodHookCalled.Store(true)
	})

	// Trigger reload
	if err := cfg.Load(); err != nil {
		t.Fatal(err)
	}

	// Wait for hooks to execute
	time.Sleep(100 * time.Millisecond)

	// The good hook should still have been called despite the panic
	if !goodHookCalled.Load() {
		t.Error("good hook was not called after another hook panicked")
	}
}

// ─── File Watching Tests ──────────────────────────────────────────────────────

func TestConfig_FileWatch_ConcurrentModifications(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := New(&YAMLFile{Path: cfgPath})
	if err != nil {
		t.Fatal(err)
	}

	var reloadCount atomic.Int32
	cfg.Watch(func() {
		reloadCount.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cfg.StartWatch(ctx); err != nil {
		t.Fatal(err)
	}
	defer cfg.StopWatch()

	// Rapidly modify the file 10 times
	for i := 0; i < 10; i++ {
		content := []byte("port: " + string(rune('0'+i)) + "\n")
		if err := os.WriteFile(cfgPath, content, 0644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for debounce and final reload
	time.Sleep(300 * time.Millisecond)

	// Due to debouncing, we should have fewer reloads than modifications
	count := reloadCount.Load()
	if count == 0 {
		t.Error("expected at least one reload, got 0")
	}
	if count >= 10 {
		t.Errorf("debouncing failed: expected fewer than 10 reloads, got %d", count)
	}
}

func TestConfig_StartWatch_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := New(&YAMLFile{Path: cfgPath})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Call StartWatch multiple times
	for i := 0; i < 5; i++ {
		if err := cfg.StartWatch(ctx); err != nil {
			t.Fatal(err)
		}
	}

	var reloadCount atomic.Int32
	cfg.Watch(func() {
		reloadCount.Add(1)
	})

	// Modify the file once
	if err := os.WriteFile(cfgPath, []byte("port: 9090\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for reload
	time.Sleep(300 * time.Millisecond)

	// Should only reload once despite multiple StartWatch calls
	if count := reloadCount.Load(); count != 1 {
		t.Errorf("expected 1 reload, got %d (StartWatch not idempotent)", count)
	}

	cfg.StopWatch()
}

func TestConfig_StopWatch_Safe(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := New(&YAMLFile{Path: cfgPath})
	if err != nil {
		t.Fatal(err)
	}

	// StopWatch before StartWatch should not panic
	cfg.StopWatch()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cfg.StartWatch(ctx); err != nil {
		t.Fatal(err)
	}

	var reloadCount atomic.Int32
	cfg.Watch(func() {
		reloadCount.Add(1)
	})

	// Stop watching
	cfg.StopWatch()

	// Modify the file after stopping
	if err := os.WriteFile(cfgPath, []byte("port: 9090\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait to ensure no reload happens
	time.Sleep(300 * time.Millisecond)

	// Should not have reloaded
	if count := reloadCount.Load(); count != 0 {
		t.Errorf("expected 0 reloads after StopWatch, got %d", count)
	}
}

// ─── Basic Functionality Tests ────────────────────────────────────────────────

func TestConfig_GetString(t *testing.T) {
	cfg, err := New(&Memory{Data: map[string]any{"name": "astra"}})
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.GetString("name"); got != "astra" {
		t.Errorf("expected 'astra', got %q", got)
	}
	if got := cfg.GetString("missing"); got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
}

func TestConfig_GetInt(t *testing.T) {
	cfg, err := New(&Memory{Data: map[string]any{"port": 8080}})
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.GetInt("port"); got != 8080 {
		t.Errorf("expected 8080, got %d", got)
	}
	if got := cfg.GetInt("missing"); got != 0 {
		t.Errorf("expected 0 for missing key, got %d", got)
	}
}

func TestConfig_GetBool(t *testing.T) {
	cfg, err := New(&Memory{Data: map[string]any{"debug": true}})
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.GetBool("debug"); !got {
		t.Error("expected true, got false")
	}
	if got := cfg.GetBool("missing"); got {
		t.Error("expected false for missing key, got true")
	}
}

func TestConfig_GetDuration(t *testing.T) {
	cfg, err := New(&Memory{Data: map[string]any{"timeout": "30s"}})
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.GetDuration("timeout"); got != 30*time.Second {
		t.Errorf("expected 30s, got %v", got)
	}
	if got := cfg.GetDuration("missing"); got != 0 {
		t.Errorf("expected 0 for missing key, got %v", got)
	}
}

func TestConfig_Scan(t *testing.T) {
	type AppCfg struct {
		Port    int    `yaml:"port" default:"8080"`
		Debug   bool   `yaml:"debug" default:"false"`
		Timeout string `yaml:"timeout" default:"30s"`
	}

	cfg, err := New(&Memory{Data: map[string]any{"port": 9090, "debug": true}})
	if err != nil {
		t.Fatal(err)
	}

	var app AppCfg
	if err := cfg.Scan(&app); err != nil {
		t.Fatal(err)
	}

	if app.Port != 9090 {
		t.Errorf("expected port 9090, got %d", app.Port)
	}
	if !app.Debug {
		t.Error("expected debug true, got false")
	}
	// timeout should use default since it's not in the data
	if app.Timeout != "30s" {
		t.Errorf("expected timeout '30s', got %q", app.Timeout)
	}
}

func TestConfig_ScanKey(t *testing.T) {
	type DBCfg struct {
		Host string `yaml:"host" default:"localhost"`
		Port int    `yaml:"port" default:"5432"`
	}

	cfg, err := New(&Memory{Data: map[string]any{
		"db": map[string]any{"host": "db.example.com"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	var db DBCfg
	if err := cfg.ScanKey("db", &db); err != nil {
		t.Fatal(err)
	}

	if db.Host != "db.example.com" {
		t.Errorf("expected host 'db.example.com', got %q", db.Host)
	}
	// port should use default
	if db.Port != 5432 {
		t.Errorf("expected port 5432, got %d", db.Port)
	}
}

func TestConfig_SourceMerging(t *testing.T) {
	src1 := &Memory{Data: map[string]any{"port": 8080, "debug": false}}
	src2 := &Memory{Data: map[string]any{"debug": true, "name": "astra"}}

	cfg, err := New(src1, src2)
	if err != nil {
		t.Fatal(err)
	}

	// src2 should override src1 for 'debug'
	if got := cfg.GetBool("debug"); !got {
		t.Error("expected debug true (from src2), got false")
	}
	// src1's 'port' should remain
	if got := cfg.GetInt("port"); got != 8080 {
		t.Errorf("expected port 8080, got %d", got)
	}
	// src2's 'name' should be present
	if got := cfg.GetString("name"); got != "astra" {
		t.Errorf("expected name 'astra', got %q", got)
	}
}

func TestConfig_NestedKeys(t *testing.T) {
	cfg, err := New(&Memory{Data: map[string]any{
		"db": map[string]any{
			"host": "localhost",
			"port": 5432,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.GetString("db.host"); got != "localhost" {
		t.Errorf("expected 'localhost', got %q", got)
	}
	if got := cfg.GetInt("db.port"); got != 5432 {
		t.Errorf("expected 5432, got %d", got)
	}
}
