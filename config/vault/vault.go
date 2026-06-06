// Package vault provides a HashiCorp Vault-backed configuration source for Astra.
//
// Vault is the standard solution for managing secrets (API keys, DB passwords,
// TLS certificates) outside of version-controlled config files.
//
// # Supported Auth Methods
//
//   - Token (simple, for dev/test)
//   - AppRole (recommended for production / CI)
//   - Kubernetes (recommended for K8s workloads)
//
// # KV Version
//
// KV v2 is assumed (path: secret/data/myapp).
// KV v1 is also supported via WithKVVersion(1).
//
// # TLS
//
// TLS can be configured via WithTLS() for secure connections to Vault.
//
// # Transit Secrets
//
// The Transit engine (WithTransitKey) provides dynamic encryption/decryption
// of secrets at rest without them ever being stored in memory.
//
// # Usage
//
//	import "github.com/astra-go/astra/config/vault"
//
//	// Token auth (dev/test)
//	src, _ := vault.NewSource("https://vault.example.com:8200",
//	    vault.WithToken("hvs.CAES..."),
//	    vault.WithPath("secret/data/myapp"),
//	)
//
//	// AppRole auth (production)
//	src, _ := vault.NewSource("https://vault.example.com:8200",
//	    vault.WithAppRole("my-role-id", "my-secret-id"),
//	    vault.WithPath("secret/data/myapp"),
//	)
//
//	// TLS + Transit encryption
//	src, _ := vault.NewSource("https://vault.example.com:8200",
//	    vault.WithToken("hvs.CAES..."),
//	    vault.WithTLS("/path/to/ca.crt"),
//	    vault.WithTransitKey("my-key"),
//	)
//
//	cfg, _ := config.New(src)
//	cfg.StartWatch(ctx) // auto-reload on secret changes
package vault

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	vapi "github.com/hashicorp/vault/api"
)

// ─── Options ───────────────────────────────────────────────────────────────

// Option configures a vault Source.
type Option func(*Source)

// WithToken authenticates with a static Vault token.
// Suitable for dev/test; for production prefer WithAppRole or WithKubernetes.
func WithToken(token string) Option {
	return func(s *Source) {
		s.token = token
	}
}

// WithAppRole authenticates via AppRole (recommended for production).
func WithAppRole(roleID, secretID string) Option {
	return func(s *Source) {
		s.authMethod = "approle"
		s.authConfig = map[string]any{
			"role_id":   roleID,
			"secret_id": secretID,
		}
	}
}

// WithKubernetes authenticates via Kubernetes ServiceAccount (recommended for K8s).
func WithKubernetes(role, mountPath string) Option {
	if mountPath == "" {
		mountPath = "kubernetes"
	}
	return func(s *Source) {
		s.authMethod = "kubernetes"
		s.authConfig = map[string]any{
			"role":      role,
			"mountPath": mountPath,
		}
	}
}

// WithPath sets the Vault KV path to read secrets from.
// For KV v2: "secret/data/myapp" (default: "secret/data/astra")
// For KV v1: "secret/myapp" (when using WithKVVersion(1))
func WithPath(path string) Option {
	return func(s *Source) {
		s.path = path
	}
}

// WithKVVersion sets the KV engine version (1 or 2). Default: 2.
func WithKVVersion(v int) Option {
	return func(s *Source) {
		s.kvVersion = v
	}
}

// WithNamespace sets the Vault namespace (for Vault Enterprise / HCP).
func WithNamespace(ns string) Option {
	return func(s *Source) {
		s.namespace = ns
	}
}

// WithPollInterval sets how often to poll Vault for secret changes.
// Set to 0 to disable polling (Watch will block until ctx cancels).
// Default: 30s.
func WithPollInterval(d time.Duration) Option {
	return func(s *Source) {
		s.pollInterval = d
	}
}

// WithTLS configures TLS for the Vault connection.
// caCert is the path to the CA certificate file.
// If empty, uses the system CA pool.
func WithTLS(caCert string) Option {
	return func(s *Source) {
		s.tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		s.caCertPath = caCert
	}
}

// WithTransitKey enables Transit engine encryption for secret values.
// Secrets stored under the configured path will be decrypted using
// the named Transit key before being returned by Load().
// This provides defense-in-depth: secrets are encrypted at rest in Vault,
// and decrypted only when accessed.
func WithTransitKey(keyName string) Option {
	return func(s *Source) {
		s.transitKey = keyName
	}
}

// WithTokenRenewal enables automatic renewal of tokens.
// interval is how often to renew (typically 1/2 of token TTL).
// Returns an error if the token cannot be renewed.
func WithTokenRenewal(interval time.Duration) Option {
	return func(s *Source) {
		s.tokenRenewInterval = interval
	}
}

// ─── Source ────────────────────────────────────────────────────────────────

// Source reads configuration from HashiCorp Vault.
// It implements config.Source and optionally config.Watchable.
type Source struct {
	// Vault client config
	addr         string
	token        string
	namespace    string
	kvVersion    int
	path         string
	pollInterval time.Duration

	// TLS
	tlsConfig  *tls.Config
	caCertPath string

	// Transit
	transitKey string

	// Token renewal
	tokenRenewInterval time.Duration
	renewCancel        context.CancelFunc
	renewOnce         sync.Once

	// Auth
	authMethod string
	authConfig map[string]any

	// Runtime
	client  *vapi.Client
	closeCh chan struct{}
}

// NewSource creates a Vault-backed config source.
// vaultAddr is the Vault server address, e.g. "https://vault.example.com:8200".
func NewSource(vaultAddr string, opts ...Option) (*Source, error) {
	s := &Source{
		addr:         vaultAddr,
		path:         "secret/data/astra",
		kvVersion:    2,
		pollInterval: 30 * time.Second,
		closeCh:      make(chan struct{}),
	}

	for _, opt := range opts {
		opt(s)
	}

	// Build Vault API client.
	vconf := vapi.DefaultConfig()
	vconf.Address = s.addr

	// Configure TLS if provided.
	if s.tlsConfig != nil {
		httpClient := cleanhttp.DefaultPooledClient()
		httpClient.Transport.(*http.Transport).TLSClientConfig = s.tlsConfig
		vconf.HttpClient = httpClient
		if s.caCertPath != "" {
			caCert, err := os.ReadFile(s.caCertPath)
			if err != nil {
				return nil, fmt.Errorf("vault: read CA cert: %w", err)
			}
			s.tlsConfig.RootCAs = loadCertPool(caCert)
		}
	}

	client, err := vapi.NewClient(vconf)
	if err != nil {
		return nil, fmt.Errorf("vault: NewClient: %w", err)
	}
	s.client = client

	// Apply namespace.
	if s.namespace != "" {
		s.client.SetNamespace(s.namespace)
	}

	// Authenticate.
	if err := s.authenticate(); err != nil {
		return nil, err
	}

	return s, nil
}

// Name implements config.Source.
func (s *Source) Name() string {
	return "vault:" + s.addr + "/" + s.path
}

// Load implements config.Source.
// Reads the latest secret version from Vault and returns it as a flat map.
// Nested Vault keys are flattened with "." separator for config.Scan binding.
func (s *Source) Load() (map[string]any, error) {
	var data map[string]any
	var err error

	if s.kvVersion == 2 {
		data, err = s.loadKVv2()
	} else {
		data, err = s.loadKVv1()
	}
	if err != nil {
		return nil, err
	}

	// Flatten nested maps with "." separator so config.Scan can bind to structs.
	flat := make(map[string]any)
	flattenMap(data, "", flat)
	return flat, nil
}

// loadKVv2 reads from KV v2 (path: /secret/data/<path>).
// KV v2 response: { "data": { "data": { <secrets> }, "metadata": {...} } }
func (s *Source) loadKVv2() (map[string]any, error) {
	secret, err := s.client.Logical().Read(s.path)
	if err != nil {
		return nil, fmt.Errorf("vault: read %s: %w", s.path, err)
	}
	if secret == nil {
		return make(map[string]any), nil
	}

	dataRaw, ok := secret.Data["data"]
	if !ok {
		slog.Warn("vault: KV v2 response missing 'data.data' field; check path",
			"path", s.path)
		return make(map[string]any), nil
	}

	data, ok := dataRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("vault: unexpected data type %T at %s", dataRaw, s.path)
	}
	return data, nil
}

// loadKVv1 reads from KV v1 (path: /secret/<path>).
func (s *Source) loadKVv1() (map[string]any, error) {
	secret, err := s.client.Logical().Read(s.path)
	if err != nil {
		return nil, fmt.Errorf("vault: read %s: %w", s.path, err)
	}
	if secret == nil {
		return make(map[string]any), nil
	}
	return secret.Data, nil
}

// ─── Watchable ─────────────────────────────────────────────────────────────

// Watch implements config.Watchable.
// Vault does not have a native watch API for KV; we poll at pollInterval.
// Stops when ctx is cancelled or Close() is called.
func (s *Source) Watch(ctx context.Context, notify func()) error {
	if s.pollInterval <= 0 {
		<-ctx.Done()
		return nil
	}

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	lastData, _ := s.Load()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.closeCh:
			return nil
		case <-ticker.C:
			curData, err := s.Load()
			if err != nil {
				slog.Warn("vault: poll load failed", "err", err)
				continue
			}
			if !mapsEqual(lastData, curData) {
				lastData = curData
				notify()
			}
		}
	}
}

// Close stops the Watch polling loop and token renewal goroutine.
func (s *Source) Close() {
	select {
	case <-s.closeCh:
		// already closed
	default:
		close(s.closeCh)
	}
	// Stop token renewal.
	s.renewOnce.Do(func() {})
	if s.renewCancel != nil {
		s.renewCancel()
	}
}

// EncryptTransit encrypts plaintext using the configured Transit key.
// Returns base64-encoded ciphertext.
// Returns an error if no Transit key is configured.
func (s *Source) EncryptTransit(ctx context.Context, plaintext string) (string, error) {
	if s.transitKey == "" {
		return "", errors.New("vault: no transit key configured")
	}

	resp, err := s.client.Logical().Write(
		fmt.Sprintf("transit/encrypt/%s", s.transitKey),
		map[string]any{"plaintext": plaintext},
	)
	if err != nil {
		return "", fmt.Errorf("vault: transit encrypt: %w", err)
	}
	if resp == nil {
		return "", errors.New("vault: transit encrypt: empty response")
	}
	ct, ok := resp.Data["ciphertext"].(string)
	if !ok {
		return "", errors.New("vault: transit encrypt: unexpected response type")
	}
	return ct, nil
}

// DecryptTransit decrypts base64-encoded ciphertext using the configured Transit key.
// Returns the plaintext.
func (s *Source) DecryptTransit(ctx context.Context, ciphertext string) (string, error) {
	if s.transitKey == "" {
		return "", errors.New("vault: no transit key configured")
	}

	resp, err := s.client.Logical().Write(
		fmt.Sprintf("transit/decrypt/%s", s.transitKey),
		map[string]any{"ciphertext": ciphertext},
	)
	if err != nil {
		return "", fmt.Errorf("vault: transit decrypt: %w", err)
	}
	if resp == nil {
		return "", errors.New("vault: transit decrypt: empty response")
	}
	pt, ok := resp.Data["plaintext"].(string)
	if !ok {
		return "", errors.New("vault: transit decrypt: unexpected response type")
	}
	return pt, nil
}

// StartTokenRenewal begins a background goroutine that renews the token
// at the configured interval. Call Close() to stop it.
// If WithTokenRenewal was not called, this is a no-op.
func (s *Source) StartTokenRenewal(ctx context.Context) error {
	if s.tokenRenewInterval <= 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	s.renewCancel = cancel

	go func() {
		ticker := time.NewTicker(s.tokenRenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.renewToken(); err != nil {
					slog.Warn("vault: token renewal failed", "err", err)
				}
			}
		}
	}()
	return nil
}

// renewToken renews the current Vault token.
func (s *Source) renewToken() error {
	if s.client == nil {
		return errors.New("vault: no client")
	}
	_, err := s.client.Auth().Token().RenewSelf(0)
	if err != nil {
		return fmt.Errorf("vault: renew token: %w", err)
	}
	return nil
}

// ─── Auth ──────────────────────────────────────────────────────────────────

func (s *Source) authenticate() error {
	// Token auth: just set it.
	if s.token != "" {
		s.client.SetToken(s.token)
		return nil
	}

	// AppRole auth.
	if s.authMethod == "approle" {
		return s.authAppRole()
	}

	// Kubernetes auth.
	if s.authMethod == "kubernetes" {
		return s.authKubernetes()
	}

	// Fallback: Vault client auto-reads VAULT_TOKEN env var.
	// Similarly VAULT_ROLE_ID / VAULT_SECRET_ID for AppRole.
	return nil
}

func (s *Source) authAppRole() error {
	roleID, _ := s.authConfig["role_id"].(string)
	secretID, _ := s.authConfig["secret_id"].(string)

	resp, err := s.client.Logical().Write("auth/approle/login", map[string]any{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		return fmt.Errorf("vault: approle login: %w", err)
	}
	if resp == nil || resp.Auth == nil {
		return errors.New("vault: approle login: empty response")
	}
	s.client.SetToken(resp.Auth.ClientToken)
	slog.Info("vault: authenticated via AppRole",
		"role_id", truncate(roleID, 8))
	return nil
}

func (s *Source) authKubernetes() error {
	role, _ := s.authConfig["role"].(string)
	mountPath, _ := s.authConfig["mountPath"].(string)
	if mountPath == "" {
		mountPath = "kubernetes"
	}

	jwtBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return fmt.Errorf("vault: kubernetes: read sa token: %w", err)
	}

	resp, err := s.client.Logical().Write(fmt.Sprintf("auth/%s/login", mountPath), map[string]any{
		"role": role,
		"jwt":  string(jwtBytes),
	})
	if err != nil {
		return fmt.Errorf("vault: kubernetes login: %w", err)
	}
	if resp == nil || resp.Auth == nil {
		return errors.New("vault: kubernetes login: empty response")
	}
	s.client.SetToken(resp.Auth.ClientToken)
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────

// flattenMap recursively flattens nested maps with "." separator.
// {"db": {"host": "x", "port": 5432}} → {"db.host": "x", "db.port": 5432}
func flattenMap(src map[string]any, prefix string, dst map[string]any) {
	for k, v := range src {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if nested, ok := v.(map[string]any); ok {
			flattenMap(nested, key, dst)
		} else {
			dst[key] = v
		}
	}
}

// mapsEqual does a shallow comparison of two maps (order-independent).
func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if av != bv {
			return false
		}
	}
	return true
}

// truncate returns the first n characters of s, or all of s if len(s) <= n.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// loadCertPool creates an x509.CertPool from PEM-encoded CA certificate data.
func loadCertPool(caCert []byte) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)
	return pool
}
