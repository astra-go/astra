package config

import (
	"errors"
	"testing"
)

// ─── Validate unit tests ──────────────────────────────────────────────────────

func TestValidate_Required(t *testing.T) {
	type Cfg struct {
		Host string `yaml:"host" validate:"required"`
		Port int    `yaml:"port" validate:"required"`
	}

	if err := Validate(&Cfg{Host: "localhost", Port: 8080}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	err := Validate(&Cfg{})
	if err == nil {
		t.Fatal("expected validation error for zero values")
	}
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	if len(ve) != 2 {
		t.Errorf("expected 2 errors (host + port), got %d", len(ve))
	}
}

func TestValidate_Min(t *testing.T) {
	type Cfg struct {
		Port int `yaml:"port" validate:"min=1"`
	}

	if err := Validate(&Cfg{Port: 1}); err != nil {
		t.Fatalf("expected no error at boundary, got: %v", err)
	}
	if err := Validate(&Cfg{Port: 8080}); err != nil {
		t.Fatalf("expected no error above min, got: %v", err)
	}

	err := Validate(&Cfg{Port: 0})
	if err == nil {
		t.Fatal("expected error for port=0 with min=1")
	}
	checkRule(t, err, "port", "min")
}

func TestValidate_Max(t *testing.T) {
	type Cfg struct {
		Port int `yaml:"port" validate:"max=65535"`
	}

	if err := Validate(&Cfg{Port: 65535}); err != nil {
		t.Fatalf("expected no error at boundary, got: %v", err)
	}

	err := Validate(&Cfg{Port: 65536})
	if err == nil {
		t.Fatal("expected error for port=65536 with max=65535")
	}
	checkRule(t, err, "port", "max")
}

func TestValidate_MinLen(t *testing.T) {
	type Cfg struct {
		Key string `yaml:"key" validate:"minlen=32"`
	}

	if err := Validate(&Cfg{Key: "12345678901234567890123456789012"}); err != nil {
		t.Fatalf("expected no error for 32-char key, got: %v", err)
	}

	err := Validate(&Cfg{Key: "short"})
	if err == nil {
		t.Fatal("expected error for key shorter than 32 chars")
	}
	checkRule(t, err, "key", "minlen")
}

func TestValidate_MaxLen(t *testing.T) {
	type Cfg struct {
		Name string `yaml:"name" validate:"maxlen=10"`
	}

	if err := Validate(&Cfg{Name: "astra"}); err != nil {
		t.Fatalf("expected no error for short name, got: %v", err)
	}

	err := Validate(&Cfg{Name: "this-name-is-too-long"})
	if err == nil {
		t.Fatal("expected error for name longer than 10 chars")
	}
	checkRule(t, err, "name", "maxlen")
}

func TestValidate_OneOf(t *testing.T) {
	type Cfg struct {
		Mode string `yaml:"mode" validate:"oneof=debug|release|test"`
	}

	for _, valid := range []string{"debug", "release", "test"} {
		if err := Validate(&Cfg{Mode: valid}); err != nil {
			t.Errorf("expected no error for mode=%q, got: %v", valid, err)
		}
	}

	err := Validate(&Cfg{Mode: "production"})
	if err == nil {
		t.Fatal("expected error for mode=production")
	}
	checkRule(t, err, "mode", "oneof")
}

func TestValidate_Pattern(t *testing.T) {
	type Cfg struct {
		Slug string `yaml:"slug" validate:"pattern=^[a-z0-9-]+$"`
	}

	if err := Validate(&Cfg{Slug: "my-app-123"}); err != nil {
		t.Fatalf("expected no error for valid slug, got: %v", err)
	}

	err := Validate(&Cfg{Slug: "My App!"})
	if err == nil {
		t.Fatal("expected error for invalid slug")
	}
	checkRule(t, err, "slug", "pattern")
}

func TestValidate_NestedStruct(t *testing.T) {
	type DBCfg struct {
		Host string `yaml:"host" validate:"required"`
		Port int    `yaml:"port" validate:"min=1,max=65535"`
	}
	type AppCfg struct {
		DB DBCfg `yaml:"db"`
	}

	valid := AppCfg{DB: DBCfg{Host: "localhost", Port: 5432}}
	if err := Validate(&valid); err != nil {
		t.Fatalf("expected no error for valid nested config, got: %v", err)
	}

	invalid := AppCfg{DB: DBCfg{Port: 0}}
	err := Validate(&invalid)
	if err == nil {
		t.Fatal("expected errors for invalid nested config")
	}
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	// host required + port min=1 failure
	if len(ve) < 2 {
		t.Errorf("expected at least 2 errors, got %d: %v", len(ve), ve)
	}
}

func TestValidate_PointerToStruct(t *testing.T) {
	type DBCfg struct {
		Host string `yaml:"host" validate:"required"`
	}
	type AppCfg struct {
		DB *DBCfg `yaml:"db"`
	}

	// nil pointer — no validation on nested fields
	if err := Validate(&AppCfg{}); err != nil {
		t.Fatalf("nil pointer field should not trigger nested validation, got: %v", err)
	}

	// non-nil pointer with invalid nested field
	err := Validate(&AppCfg{DB: &DBCfg{}})
	if err == nil {
		t.Fatal("expected error for empty host in non-nil pointer")
	}
	checkRule(t, err, "db.host", "required")
}

func TestValidate_MultipleRulesOnOneField(t *testing.T) {
	type Cfg struct {
		Port int `yaml:"port" validate:"required,min=1,max=65535"`
	}

	if err := Validate(&Cfg{Port: 8080}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Port=0 violates both required and min=1.
	err := Validate(&Cfg{Port: 0})
	if err == nil {
		t.Fatal("expected errors for port=0")
	}
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	if len(ve) < 2 {
		t.Errorf("expected at least 2 errors (required + min), got %d", len(ve))
	}
}

func TestValidate_NonStructInput(t *testing.T) {
	if err := Validate(nil); err != nil {
		t.Fatalf("nil input should return nil, got: %v", err)
	}

	err := Validate("not a struct")
	if err == nil {
		t.Fatal("expected error for non-struct input")
	}
}

// ─── ScanAndValidate integration tests ───────────────────────────────────────

func TestConfig_ScanAndValidate_Valid(t *testing.T) {
	type ServerCfg struct {
		Host string `yaml:"host" validate:"required"`
		Port int    `yaml:"port" validate:"required,min=1,max=65535"`
		Mode string `yaml:"mode" validate:"oneof=debug|release|test"`
	}

	cfg, err := New(&Memory{Data: map[string]any{
		"host": "localhost",
		"port": 8080,
		"mode": "release",
	}})
	if err != nil {
		t.Fatal(err)
	}

	var srv ServerCfg
	if err := cfg.ScanAndValidate(&srv); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if srv.Host != "localhost" || srv.Port != 8080 || srv.Mode != "release" {
		t.Errorf("unexpected values: %+v", srv)
	}
}

func TestConfig_ScanAndValidate_Invalid(t *testing.T) {
	type ServerCfg struct {
		Host string `yaml:"host" validate:"required"`
		Port int    `yaml:"port" validate:"required,min=1,max=65535"`
	}

	cfg, err := New(&Memory{Data: map[string]any{
		"port": 99999, // exceeds max=65535
	}})
	if err != nil {
		t.Fatal(err)
	}

	var srv ServerCfg
	err = cfg.ScanAndValidate(&srv)
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
}

func TestConfig_ScanKeyAndValidate(t *testing.T) {
	type DBCfg struct {
		Host string `yaml:"host" validate:"required"`
		Port int    `yaml:"port" validate:"min=1,max=65535" default:"5432"`
	}

	cfg, err := New(&Memory{Data: map[string]any{
		"db": map[string]any{"host": "db.example.com"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	var db DBCfg
	if err := cfg.ScanKeyAndValidate("db", &db); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if db.Host != "db.example.com" {
		t.Errorf("expected host 'db.example.com', got %q", db.Host)
	}
	if db.Port != 5432 {
		t.Errorf("expected default port 5432, got %d", db.Port)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func checkRule(t *testing.T, err error, field, rule string) {
	t.Helper()
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
	for _, e := range ve {
		if e.Field == field && e.Rule == rule {
			return
		}
	}
	t.Errorf("expected ValidationError{Field:%q, Rule:%q}, got: %v", field, rule, ve)
}
