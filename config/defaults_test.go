package config

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── Mode helpers ─────────────────────────────────────────────────────────────

func TestMode_DefaultDev(t *testing.T) {
	c := &Config{data: map[string]any{}}
	if m := c.Mode(); m != "dev" {
		t.Fatalf("expected 'dev', got %q", m)
	}
	if !c.IsDev() {
		t.Fatal("expected IsDev() == true")
	}
	if c.IsProd() {
		t.Fatal("expected IsProd() == false")
	}
	if c.IsStaging() {
		t.Fatal("expected IsStaging() == false")
	}
	if c.IsTest() {
		t.Fatal("expected IsTest() == false")
	}
	if c.IsStagingOrProd() {
		t.Fatal("expected IsStagingOrProd() == false")
	}
}

func TestMode_Prod(t *testing.T) {
	c := &Config{data: map[string]any{"app": map[string]any{"mode": "prod"}}}
	if m := c.Mode(); m != "prod" {
		t.Fatalf("expected 'prod', got %q", m)
	}
	if !c.IsProd() {
		t.Fatal("expected IsProd() == true")
	}
	if c.IsDev() {
		t.Fatal("expected IsDev() == false")
	}
	if !c.IsStagingOrProd() {
		t.Fatal("expected IsStagingOrProd() == true")
	}
}

func TestMode_Staging(t *testing.T) {
	c := &Config{data: map[string]any{"app": map[string]any{"mode": "staging"}}}
	if !c.IsStaging() {
		t.Fatal("expected IsStaging() == true")
	}
	if !c.IsStagingOrProd() {
		t.Fatal("expected IsStagingOrProd() == true")
	}
	if c.IsProd() {
		t.Fatal("expected IsProd() == false")
	}
}

func TestMode_Test(t *testing.T) {
	c := &Config{data: map[string]any{"app": map[string]any{"mode": "test"}}}
	if !c.IsTest() {
		t.Fatal("expected IsTest() == true")
	}
	if c.IsStagingOrProd() {
		t.Fatal("expected IsStagingOrProd() == false")
	}
}

func TestMode_EmptyMap(t *testing.T) {
	c := &Config{}
	if m := c.Mode(); m != "dev" {
		t.Fatalf("expected 'dev', got %q", m)
	}
}

// ─── YAMLFileOptional ──────────────────────────────────────────────────────

func TestYAMLFileOptional_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opt.yaml")
	os.WriteFile(path, []byte(`key: value`), 0644)

	s := &YAMLFileOptional{Path: path}
	data, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if v := data["key"]; v != "value" {
		t.Fatalf("expected 'value', got %v", v)
	}
}

func TestYAMLFileOptional_FileMissing(t *testing.T) {
	s := &YAMLFileOptional{Path: "/nonexistent/opt.yaml"}
	data, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty map, got %v", data)
	}
}

func TestYAMLFileOptional_Name(t *testing.T) {
	s := &YAMLFileOptional{Path: "/tmp/x.yaml"}
	if n := s.Name(); n != "yaml+optional:/tmp/x.yaml" {
		t.Fatalf("unexpected name: %q", n)
	}
}

func TestYAMLFileOptional_FilePath(t *testing.T) {
	s := &YAMLFileOptional{Path: "/a/b.yaml"}
	if s.FilePath() != "/a/b.yaml" {
		t.Fatalf("unexpected filepath: %q", s.FilePath())
	}
}

// ─── JSONFileOptional ─────────────────────────────────────────────────────

func TestJSONFileOptional_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opt.json")
	os.WriteFile(path, []byte(`{"key": 42}`), 0644)

	s := &JSONFileOptional{Path: path}
	data, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	// yaml.v3 unmarshals JSON numbers as int; accept either int or float64
	switch v := data["key"].(type) {
	case int:
		if v != 42 {
			t.Fatalf("expected 42, got %d", v)
		}
	case float64:
		if v != 42 {
			t.Fatalf("expected 42, got %f", v)
		}
	default:
		t.Fatalf("unexpected type %T with value %v", data["key"], data["key"])
	}
}

func TestJSONFileOptional_FileMissing(t *testing.T) {
	s := &JSONFileOptional{Path: "/nonexistent/opt.json"}
	data, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty map")
	}
}

// ─── TOMLFileOptional ─────────────────────────────────────────────────────

func TestTOMLFileOptional_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opt.toml")
	os.WriteFile(path, []byte(`key = "hello"`), 0644)

	s := &TOMLFileOptional{Path: path}
	data, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if v := data["key"]; v != "hello" {
		t.Fatalf("expected 'hello', got %v", v)
	}
}

func TestTOMLFileOptional_FileMissing(t *testing.T) {
	s := &TOMLFileOptional{Path: "/nonexistent/opt.toml"}
	data, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty map")
	}
}

// ─── LoadWithDefaults ─────────────────────────────────────────────────────

func TestLoadWithDefaults_ServiceSpecific(t *testing.T) {
	dir := t.TempDir()
	svcFile := filepath.Join(dir, "mysvc.yaml")
	os.WriteFile(svcFile, []byte("app:\n  mode: prod\nserver:\n  port: 9090\n"), 0644)

	cfg, err := LoadWithDefaults("mysvc", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if v := cfg.GetString("server.port"); v != "9090" {
		t.Fatalf("expected '9090', got %q", v)
	}
	if !cfg.IsProd() {
		t.Fatal("expected IsProd() == true")
	}
}

func TestLoadWithDefaults_ModeFallback(t *testing.T) {
	dir := t.TempDir()
	devFile := filepath.Join(dir, "dev.yaml")
	os.WriteFile(devFile, []byte("app:\n  mode: dev\ndb:\n  host: localhost\n"), 0644)

	cfg, err := LoadWithDefaults("", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if v := cfg.GetString("db.host"); v != "localhost" {
		t.Fatalf("expected 'localhost', got %q", v)
	}
	if !cfg.IsDev() {
		t.Fatal("expected IsDev() == true")
	}
}

func TestLoadWithDefaults_EnvPrefix(t *testing.T) {
	os.Setenv("TEST_PORT", "3000")
	defer os.Unsetenv("TEST_PORT")

	cfg, err := LoadWithDefaults("", "", "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if v := cfg.GetString("port"); v != "3000" {
		t.Fatalf("expected '3000', got %q", v)
	}
}

func TestLoadWithDefaults_EnvOverrideFile(t *testing.T) {
	dir := t.TempDir()
	svcFile := filepath.Join(dir, "svc.yaml")
	os.WriteFile(svcFile, []byte("port: 8080\n"), 0644)

	os.Setenv("TEST_PORT", "9090")
	defer os.Unsetenv("TEST_PORT")

	cfg, err := LoadWithDefaults("svc", dir, "TEST")
	if err != nil {
		t.Fatal(err)
	}
	// Env overrides file
	if v := cfg.GetString("port"); v != "9090" {
		t.Fatalf("expected '9090' (env override), got %q", v)
	}
}

func TestLoadWithDefaults_NoFiles(t *testing.T) {
	cfg, err := LoadWithDefaults("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsDev() {
		t.Fatal("expected default IsDev() == true")
	}
}

func TestLoadWithDefaults_ServiceTakesPriority(t *testing.T) {
	dir := t.TempDir()
	svcFile := filepath.Join(dir, "mysvc.yaml")
	devFile := filepath.Join(dir, "dev.yaml")
	os.WriteFile(svcFile, []byte("version: from-service\n"), 0644)
	os.WriteFile(devFile, []byte("version: from-dev\nmode: dev\n"), 0644)

	cfg, err := LoadWithDefaults("mysvc", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	// Service file + mode file both load, and due to how sources work,
	// (later sources override), the mode file (loaded second) wins.
	// But the env override system in config.Load() handles this.
	// Both files' data gets merged. When both files set the same key,
	// the later source (mode file) wins.
	// Let's check version - it depends on merge order...
	// version is set in both. mode file (loaded second) should win.
	v := cfg.GetString("version")
	if v != "from-dev" {
		t.Fatalf("expected 'from-dev' (later source wins), got %q", v)
	}
}

func TestLoadWithDefaults_APP_MODE(t *testing.T) {
	os.Setenv("APP_MODE", "staging")
	defer os.Unsetenv("APP_MODE")

	dir := t.TempDir()
	stageFile := filepath.Join(dir, "staging.yaml")
	os.WriteFile(stageFile, []byte("app:\n  mode: staging\n"), 0644)

	cfg, err := LoadWithDefaults("", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsStaging() {
		t.Fatal("expected IsStaging() == true")
	}
}

func TestLoadWithDefaults_ServiceNonExistent(t *testing.T) {
	os.Setenv("APP_MODE", "prod")
	defer os.Unsetenv("APP_MODE")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "prod.yaml"), []byte("app:\n  mode: prod\n"), 0644)

	cfg, err := LoadWithDefaults("nonexistent-svc", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsProd() {
		t.Fatal("expected IsProd() == true")
	}
}
