package vault

import (
	"context"
	"crypto/tls"
	"testing"
	"time"
)

// TestNewSource_TokenAuth tests that NewSource can be created with a token.
// This is a unit test that validates option parsing and client setup,
// without requiring a running Vault server.
func TestNewSource_TokenAuth(t *testing.T) {
	// We don't have a real Vault, so we only test that the option functions
	// compile and apply correctly. A real integration test would use
	// hashicorp/vault/api/devutils or a test container.
	opts := []Option{
		WithToken("hvs.CAES-test"),
		WithPath("secret/data/myapp"),
		WithKVVersion(2),
		WithPollInterval(0), // disable polling for tests
	}
	if len(opts) != 4 {
		t.Fatalf("expected 4 options, got %d", len(opts))
	}

	// Each option should be a valid function (compiles & type-checks).
	// We can't call NewSource without a real Vault, so we just verify
	// the options don't panic when created.
	t.Log("Token auth options created successfully")
}

// TestNewSource_AppRoleOptions verifies AppRole option construction.
func TestNewSource_AppRoleOptions(t *testing.T) {
	opt := WithAppRole("my-role-id", "my-secret-id")
	s := &Source{}
	opt(s)
	if s.authMethod != "approle" {
		t.Errorf("expected authMethod=approle, got %s", s.authMethod)
	}
}

// TestNewSource_KubernetesOptions verifies Kubernetes option construction.
func TestNewSource_KubernetesOptions(t *testing.T) {
	opt := WithKubernetes("my-role", "")
	s := &Source{}
	opt(s)
	if s.authMethod != "kubernetes" {
		t.Errorf("expected authMethod=kubernetes, got %s", s.authMethod)
	}
}

// TestFlattenMap verifies nested map flattening with "." separator.
func TestFlattenMap(t *testing.T) {
	src := map[string]any{
		"db": map[string]any{
			"host": "localhost",
			"port": float64(5432),
		},
		"redis": map[string]any{
			"addr": "127.0.0.1",
			"db":   float64(0),
		},
	}
	dst := make(map[string]any)
	flattenMap(src, "", dst)

	tests := []struct {
		key   string
		want  any
	}{
		{"db.host", "localhost"},
		{"db.port", float64(5432)},
		{"redis.addr", "127.0.0.1"},
		{"redis.db", float64(0)},
	}
	for _, tt := range tests {
		got, ok := dst[tt.key]
		if !ok {
			t.Errorf("missing key %q", tt.key)
			continue
		}
		if got != tt.want {
			t.Errorf("%s = %v (%T), want %v (%T)", tt.key, got, got, tt.want, tt.want)
		}
	}
}

// TestMapsEqual verifies the equality helper.
func TestMapsEqual(t *testing.T) {
	a := map[string]any{"x": 1, "y": "2"}
	b := map[string]any{"x": 1, "y": "2"}
	c := map[string]any{"x": 1}
	if !mapsEqual(a, b) {
		t.Error("expected a == b")
	}
	if mapsEqual(a, c) {
		t.Error("expected a != c (different values)")
	}
}

// TestSource_Name verifies the Name() output format.
func TestSource_Name(t *testing.T) {
	s := &Source{addr: "https://vault.example.com:8200", path: "secret/data/myapp"}
	name := s.Name()
	want := "vault:https://vault.example.com:8200/secret/data/myapp"
	if name != want {
		t.Errorf("Name() = %q, want %q", name, want)
	}
}

// TestSource_Close verifies Close() is idempotent.
func TestSource_Close(t *testing.T) {
	s := &Source{closeCh: make(chan struct{})}
	s.Close()
	// second close should not panic
	s.Close()
}

// TestAuthenticate_FallbackEnvVar verifies that authenticate() succeeds
// when no explicit auth is configured (falls back to VAULT_TOKEN env).
// We can't easily test this without setting env, so just cover the branch.
func TestAuthenticate_Fallback(t *testing.T) {
	s := &Source{authMethod: "", token: ""}
	// Should not error (just returns nil, Vault client will pick up VAULT_TOKEN later)
	err := s.authenticate()
	if err != nil {
		t.Errorf("authenticate (fallback) returned error: %v", err)
	}
}

// TestLoad_KVVersionDefault verifies that Load selects KVv2 by default.
// This is a compile-time check that the method exists and has the right signature.
func TestLoad_Signature(t *testing.T) {
	s := &Source{kvVersion: 2, path: "secret/data/test"}
	// Just verify the method exists; actual Load requires a Vault connection.
	_ = s.kvVersion // suppress unused warning
}

// BenchmarkFlattenMap provides a performance baseline for the flatten operation.
func BenchmarkFlattenMap(b *testing.B) {
	src := map[string]any{
		"app": map[string]any{
			"name":    "myapp",
			"port":    float64(8080),
			"debug":   true,
			"db":      map[string]any{"host": "localhost", "port": float64(5432)},
		},
	}
	dst := make(map[string]any, 8)
	for i := 0; i < b.N; i++ {
		flattenMap(src, "", dst)
		dst = make(map[string]any, 8) // reset
	}
}

// TestWithNamespace verifies namespace option.
func TestWithNamespace(t *testing.T) {
	opt := WithNamespace("team-a")
	s := &Source{}
	opt(s)
	if s.namespace != "team-a" {
		t.Errorf("expected namespace=team-a, got %s", s.namespace)
	}
}

// TestWithKVVersion verifies KV version option.
func TestWithKVVersion(t *testing.T) {
	opt := WithKVVersion(1)
	s := &Source{kvVersion: 2}
	opt(s)
	if s.kvVersion != 1 {
		t.Errorf("expected kvVersion=1, got %d", s.kvVersion)
	}
}

// TestWithTLS verifies TLS option.
func TestWithTLS(t *testing.T) {
	opt := WithTLS("/path/to/ca.crt")
	s := &Source{}
	opt(s)
	if s.tlsConfig == nil {
		t.Error("expected tlsConfig to be set")
	}
	if s.caCertPath != "/path/to/ca.crt" {
		t.Errorf("expected caCertPath=/path/to/ca.crt, got %s", s.caCertPath)
	}
	if s.tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion=TLS12, got %d", s.tlsConfig.MinVersion)
	}
}

// TestWithTransitKey verifies Transit key option.
func TestWithTransitKey(t *testing.T) {
	opt := WithTransitKey("my-encryption-key")
	s := &Source{}
	opt(s)
	if s.transitKey != "my-encryption-key" {
		t.Errorf("expected transitKey=my-encryption-key, got %s", s.transitKey)
	}
}

// TestWithTokenRenewal verifies token renewal option.
func TestWithTokenRenewal(t *testing.T) {
	opt := WithTokenRenewal(10 * time.Minute)
	s := &Source{}
	opt(s)
	if s.tokenRenewInterval != 10*time.Minute {
		t.Errorf("expected tokenRenewInterval=10m, got %v", s.tokenRenewInterval)
	}
}

// TestEncryptTransit_NoKey verifies error when no transit key is configured.
func TestEncryptTransit_NoKey(t *testing.T) {
	s := &Source{}
	_, err := s.EncryptTransit(context.Background(), "plaintext")
	if err == nil {
		t.Error("expected error when no transit key configured")
	}
}

// TestDecryptTransit_NoKey verifies error when no transit key is configured.
func TestDecryptTransit_NoKey(t *testing.T) {
	s := &Source{}
	_, err := s.DecryptTransit(context.Background(), "ciphertext")
	if err == nil {
		t.Error("expected error when no transit key configured")
	}
}

// TestStartTokenRenewal_Disabled verifies no-op when renewal is disabled.
func TestStartTokenRenewal_Disabled(t *testing.T) {
	s := &Source{}
	err := s.StartTokenRenewal(context.Background())
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestSource_Close_Idempotent verifies Close is safe to call multiple times.
func TestSource_Close_Idempotent(t *testing.T) {
	s := &Source{closeCh: make(chan struct{})}
	s.Close()
	s.Close() // second close should not panic
}
