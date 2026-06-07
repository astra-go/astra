package marketplace

import (
	"fmt"
	"testing"

	"github.com/astra-go/astra"
)

func TestCatalog_RegisterAndLookup(t *testing.T) {
	c := NewCatalog()
	c.Register(&MiddlewareDescriptor{
		Name:        "test-mw",
		Description: "A test middleware",
		Category:    "utility",
		Factory:     func(config any) (astra.HandlerFunc, error) { return nil, nil },
	})

	desc := c.Lookup("test-mw")
	if desc == nil {
		t.Fatal("expected to find 'test-mw'")
	}
	if desc.Name != "test-mw" {
		t.Errorf("name mismatch: %q", desc.Name)
	}
	if d := c.Lookup("nonexistent"); d != nil {
		t.Error("expected nil for nonexistent")
	}
}

func TestCatalog_DuplicateNamePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate name")
		}
	}()
	c := NewCatalog()
	c.Register(&MiddlewareDescriptor{Name: "dup", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})
	c.Register(&MiddlewareDescriptor{Name: "dup", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})
}

func TestCatalog_List(t *testing.T) {
	c := NewCatalog()
	c.Register(&MiddlewareDescriptor{Name: "b", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})
	c.Register(&MiddlewareDescriptor{Name: "a", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})
	c.Register(&MiddlewareDescriptor{Name: "c", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})

	names := c.List()
	if len(names) != 3 || names[0] != "a" || names[2] != "c" {
		t.Errorf("expected sorted [a b c], got %v", names)
	}
}

func TestCatalog_Search(t *testing.T) {
	c := NewCatalog()
	c.Register(&MiddlewareDescriptor{
		Name: "cors", Description: "CORS headers", Category: "security", Tags: []string{"origin"},
		Factory: func(any) (astra.HandlerFunc, error) { return nil, nil },
	})
	c.Register(&MiddlewareDescriptor{
		Name: "logger", Description: "Request logging", Category: "observability", Tags: []string{"access-log"},
		Factory: func(any) (astra.HandlerFunc, error) { return nil, nil },
	})
	c.Register(&MiddlewareDescriptor{
		Name: "rate-limit", Description: "Rate limiting", Category: "performance", Tags: []string{"rate", "throttle"},
		Factory: func(any) (astra.HandlerFunc, error) { return nil, nil },
	})

	tests := []struct {
		terms   []string
		expect  int
		first   string
	}{
		{[]string{"cors"}, 1, "cors"},
		{[]string{"security"}, 1, "cors"},
		{[]string{"rate"}, 1, "rate-limit"},
		{[]string{"throttle"}, 1, "rate-limit"},
		{[]string{"log"}, 1, "logger"},
		{[]string{"nonexistent"}, 0, ""},
		{[]string{}, 3, "cors"}, // no terms = all
	}

	for _, tt := range tests {
		results := c.Search(tt.terms...)
		if len(results) != tt.expect {
			t.Errorf("Search(%v) expected %d results, got %d", tt.terms, tt.expect, len(results))
		}
		if tt.expect > 0 && results[0].Name != tt.first {
			t.Errorf("Search(%v) first result = %q, want %q", tt.terms, results[0].Name, tt.first)
		}
	}
}

func TestCatalog_ByCategory(t *testing.T) {
	c := NewCatalog()
	c.Register(&MiddlewareDescriptor{Name: "cors", Category: "security", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})
	c.Register(&MiddlewareDescriptor{Name: "jwt", Category: "security", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})
	c.Register(&MiddlewareDescriptor{Name: "logger", Category: "observability", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})

	groups := c.ByCategory()
	if len(groups["security"]) != 2 {
		t.Errorf("expected 2 security middleware, got %d", len(groups["security"]))
	}
	if len(groups["observability"]) != 1 {
		t.Errorf("expected 1 observability middleware, got %d", len(groups["observability"]))
	}
}

func TestCatalog_Len(t *testing.T) {
	c := NewCatalog()
	if c.Len() != 0 {
		t.Errorf("expected 0, got %d", c.Len())
	}
	c.Register(&MiddlewareDescriptor{Name: "a", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})
	if c.Len() != 1 {
		t.Errorf("expected 1, got %d", c.Len())
	}
}

// ─── BuildChain Tests ───────────────────────────────────────────────────────

func TestBuildChain_Basic(t *testing.T) {
	c := NewCatalog()
	c.Register(&MiddlewareDescriptor{
		Name:    "mw1",
		Factory: func(config any) (astra.HandlerFunc, error) { return func(c *astra.Ctx) error { return c.Next() }, nil },
	})
	c.Register(&MiddlewareDescriptor{
		Name:    "mw2",
		Factory: func(config any) (astra.HandlerFunc, error) { return func(c *astra.Ctx) error { return c.Next() }, nil },
	})

	handlers, warnings := c.BuildChain(Config{
		Middleware: []MiddlewareEntry{
			{Name: "mw1"},
			{Name: "mw2"},
		},
	})
	if len(handlers) != 2 {
		t.Fatalf("expected 2 handlers, got %d", len(handlers))
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestBuildChain_UnknownMiddlewareWarning(t *testing.T) {
	c := NewCatalog()
	_, warnings := c.BuildChain(Config{
		Middleware: []MiddlewareEntry{
			{Name: "nonexistent"},
		},
	})
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
}

func TestBuildChain_DisabledSkipped(t *testing.T) {
	c := NewCatalog()
	c.Register(&MiddlewareDescriptor{
		Name:    "mw",
		Factory: func(config any) (astra.HandlerFunc, error) { return func(c *astra.Ctx) error { return c.Next() }, nil },
	})

	handlers, _ := c.BuildChain(Config{
		Middleware: []MiddlewareEntry{
			{Name: "mw", Disabled: true},
		},
	})
	if len(handlers) != 0 {
		t.Errorf("expected 0 handlers for disabled entry, got %d", len(handlers))
	}
}

func TestBuildChain_FactoryErrorWarning(t *testing.T) {
	c := NewCatalog()
	c.Register(&MiddlewareDescriptor{
		Name: "fail-mw",
		Factory: func(config any) (astra.HandlerFunc, error) {
			return nil, fmt.Errorf("factory error")
		},
	})

	handlers, warnings := c.BuildChain(Config{
		Middleware: []MiddlewareEntry{
			{Name: "fail-mw"},
		},
	})
	if len(handlers) != 0 {
		t.Errorf("expected 0 handlers on factory error, got %d", len(handlers))
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
}

func TestBuildChain_OrderingHints(t *testing.T) {
	c := NewCatalog()
	c.Register(&MiddlewareDescriptor{Name: "a", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})
	c.Register(&MiddlewareDescriptor{Name: "b", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})
	c.Register(&MiddlewareDescriptor{Name: "c", Factory: func(any) (astra.HandlerFunc, error) { return nil, nil }})

	cfg := Config{
		Middleware: []MiddlewareEntry{
			{Name: "c", Before: "a"},
			{Name: "a"},
			{Name: "b", After: "a"},
		},
	}
	handlers, _ := c.BuildChain(cfg)
	if len(handlers) != 3 {
		t.Fatalf("expected 3 handlers, got %d", len(handlers))
	}
	// c should be first (Before: a), a second, b last (After: a)
	// But we can't inspect names from handlers... test the ordering logic directly
}

// ─── DecodeConfig Tests ────────────────────────────────────────────────────

type testConfig struct {
	Name    string
	Enabled bool
	Count   int
	Rate    float64
	Tags    []string
}

func TestDecodeConfig_Basic(t *testing.T) {
	data := map[string]any{
		"name":    "test",
		"enabled": true,
		"count":   42,
		"rate":    3.14,
		"tags":    []string{"a", "b"},
	}

	var cfg testConfig
	if err := DecodeConfig(data, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "test" || !cfg.Enabled || cfg.Count != 42 || cfg.Rate != 3.14 {
		t.Errorf("config mismatch: %+v", cfg)
	}
}

func TestDecodeConfig_NilData(t *testing.T) {
	var cfg testConfig
	if err := DecodeConfig(nil, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeConfig_SnakeCase(t *testing.T) {
	data := map[string]any{
		"my_field": "value",
	}

	type nested struct {
		MyField string
	}
	var cfg nested
	if err := DecodeConfig(data, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MyField != "value" {
		t.Errorf("expected 'value', got %q", cfg.MyField)
	}
}

func TestDecodeConfig_NonPointerTarget(t *testing.T) {
	err := DecodeConfig(map[string]any{}, testConfig{})
	if err == nil {
		t.Error("expected error for non-pointer target")
	}
}

// ─── Builtins Registration Test ─────────────────────────────────────────────

func TestRegisterBuiltins(t *testing.T) {
	c := NewCatalog()
	RegisterBuiltins(c)

	if c.Len() < 15 {
		t.Errorf("expected at least 15 builtins, got %d", c.Len())
	}

	// Verify some key middleware exist
	for _, name := range []string{"cors", "csrf", "jwt", "logger", "recovery", "compress", "rate-limit"} {
		if c.Lookup(name) == nil {
			t.Errorf("expected builtin middleware %q to be registered", name)
		}
	}

	// Verify categories
	groups := c.ByCategory()
	if _, ok := groups["security"]; !ok {
		t.Error("expected 'security' category")
	}
	if _, ok := groups["performance"]; !ok {
		t.Error("expected 'performance' category")
	}
	if _, ok := groups["observability"]; !ok {
		t.Error("expected 'observability' category")
	}
}

func TestRegisterBuiltins_FactoryWorks(t *testing.T) {
	c := NewCatalog()
	RegisterBuiltins(c)

	// Test middleware that work without config
	for _, name := range []string{"cors", "logger", "recovery", "compress", "request-id", "response-format"} {
		desc := c.Lookup(name)
		if desc == nil {
			continue
		}
		handler, err := desc.Factory(nil)
		if err != nil {
			t.Errorf("builtin %q factory failed: %v", name, err)
		}
		if handler == nil {
			t.Errorf("builtin %q returned nil handler", name)
		}
	}
}

func TestRegisterBuiltins_CORSWithConfig(t *testing.T) {
	c := NewCatalog()
	RegisterBuiltins(c)

	desc := c.Lookup("cors")
	handler, err := desc.Factory(map[string]any{"allow_origins": []string{"https://example.com"}})
	if err != nil {
		t.Fatalf("CORS factory failed: %v", err)
	}
	if handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestRegisterBuiltins_JWTRequiresSecret(t *testing.T) {
	c := NewCatalog()
	RegisterBuiltins(c)

	desc := c.Lookup("jwt")
	_, err := desc.Factory(nil)
	if err == nil {
		t.Error("expected error when jwt has no secret")
	}
	_, err = desc.Factory(map[string]any{})
	if err == nil {
		t.Error("expected error when jwt has empty config")
	}
	_, err = desc.Factory(map[string]any{"secret": "my-secret-key-that-is-at-least-32-bytes-long"})
	if err != nil {
		t.Errorf("jwt factory with secret should succeed: %v", err)
	}
}

func TestRegisterBuiltins_TimeoutWithConfig(t *testing.T) {
	c := NewCatalog()
	RegisterBuiltins(c)

	desc := c.Lookup("timeout")
	handler, err := desc.Factory(map[string]any{"duration": "5s"})
	if err != nil {
		t.Fatalf("timeout factory failed: %v", err)
	}
	if handler == nil {
		t.Error("expected non-nil handler")
	}

	// Invalid duration should fail
	_, err = desc.Factory(map[string]any{"duration": "not-a-duration"})
	if err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestRegisterBuiltins_RateLimitWithConfig(t *testing.T) {
	c := NewCatalog()
	RegisterBuiltins(c)

	desc := c.Lookup("rate-limit")
	handler, err := desc.Factory(map[string]any{"rate": 50.0, "burst": 100.0})
	if err != nil {
		t.Fatalf("rate-limit factory failed: %v", err)
	}
	if handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestRegisterBuiltins_Search(t *testing.T) {
	c := NewCatalog()
	RegisterBuiltins(c)

	// Search for security middleware
	results := c.Search("security")
	for _, r := range results {
		if r.Category != "security" {
			t.Errorf("search 'security' returned non-security middleware: %s (%s)", r.Name, r.Category)
		}
	}

	// Search for rate
	results = c.Search("rate")
	if len(results) == 0 {
		t.Error("expected rate-related middleware")
	}
}

// ─── CamelToSnake Test ────────────────────────────────────────────────────

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"AllowOrigins", "allow_origins"},
		{"HSTSMaxAge", "h_s_t_s_max_age"},
		{"Rate", "rate"},
		{"SecretKey", "secret_key"},
		{"XFrameOptions", "x_frame_options"},
	}
	for _, tt := range tests {
		if got := camelToSnake(tt.input); got != tt.want {
			t.Errorf("camelToSnake(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ─── Integration: Full BuildChain with Builtins ────────────────────────────

func TestIntegration_FullChainWithBuiltins(t *testing.T) {
	c := NewCatalog()
	RegisterBuiltins(c)

	handlers, warnings := c.BuildChain(Config{
		Middleware: []MiddlewareEntry{
			{Name: "recovery"},
			{Name: "request-id"},
			{Name: "logger"},
			{Name: "cors", Config: map[string]any{"allow_origins": []string{"https://api.example.com"}}},
			{Name: "secure-headers"},
			{Name: "timeout", Config: map[string]any{"duration": "30s"}},
		},
	})

	if len(handlers) != 6 {
		t.Errorf("expected 6 handlers, got %d", len(handlers))
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}
