package marketplace

import (
	"fmt"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	"github.com/astra-go/astra/middleware/security"
)

// RegisterBuiltins registers all framework-bundled middleware into the catalog.
// Call this once during application setup.
func RegisterBuiltins(c *Catalog) {
	// ── Security ─────────────────────────────────────────────────────────
	c.Register(&MiddlewareDescriptor{
		Name:        "cors",
		Description: "Cross-Origin Resource Sharing (CORS) headers control",
		Category:    "security",
		Tags:        []string{"origin", "headers", "cross-origin"},
		ConfigType:  "CORSConfig",
		DefaultFn:   func() any { return middleware.CORSConfig{} },
		Factory:     factoryCORS,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "csrf",
		Description: "Cross-Site Request Forgery protection with double-submit cookie",
		Category:    "security",
		Tags:        []string{"csrf", "token", "cookie"},
		ConfigType:  "CSRFConfig",
		DefaultFn:   func() any { return middleware.CSRFConfig{} },
		Factory:     factoryCSRF,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "csp",
		Description: "Content-Security-Policy headers with nonce support",
		Category:    "security",
		Tags:        []string{"content-security", "xss", "headers"},
		ConfigType:  "CSPConfig",
		DefaultFn:   func() any { return middleware.CSPConfig{} },
		Factory:     factoryCSP,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "secure-headers",
		Description: "Security hardening headers (HSTS, X-Frame-Options, etc.)",
		Category:    "security",
		Tags:        []string{"hsts", "x-frame", "headers", "hardening"},
		ConfigType:  "SecureConfig",
		DefaultFn:   func() any { return middleware.SecureConfig{} },
		Factory:     factorySecureHeaders,
	})

	// ── Authentication & Authorization ───────────────────────────────────
	c.Register(&MiddlewareDescriptor{
		Name:        "jwt",
		Description: "JSON Web Token authentication and validation",
		Category:    "security",
		Tags:        []string{"jwt", "auth", "token", "bearer"},
		ConfigType:  "JWTConfig",
		DefaultFn:   func() any { return security.JWTConfig{} },
		Factory:     factoryJWT,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "api-key",
		Description: "API key authentication via header or query parameter",
		Category:    "security",
		Tags:        []string{"api-key", "auth", "key"},
		ConfigType:  "APIKeyConfig",
		DefaultFn:   func() any { return security.APIKeyConfig{} },
		Factory:     factoryAPIKey,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "ip-filter",
		Description: "IP address allowlist/denylist filtering",
		Category:    "security",
		Tags:        []string{"ip", "filter", "firewall", "acl"},
		ConfigType:  "IPFilterConfig",
		DefaultFn:   func() any { return security.IPFilterConfig{} },
		Factory:     factoryIPFilter,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "signature",
		Description: "Request signature verification (HMAC)",
		Category:    "security",
		Tags:        []string{"signature", "hmac", "verification"},
		ConfigType:  "SignatureConfig",
		DefaultFn:   func() any { return security.SignatureConfig{} },
		Factory:     factorySignature,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "tenant",
		Description: "Multi-tenant identification and isolation",
		Category:    "security",
		Tags:        []string{"tenant", "multi-tenant", "isolation"},
		ConfigType:  "TenantConfig",
		DefaultFn:   func() any { return security.TenantConfig{} },
		Factory:     factoryTenant,
	})

	// ── Rate Limiting ────────────────────────────────────────────────────
	c.Register(&MiddlewareDescriptor{
		Name:        "rate-limit",
		Description: "Token bucket rate limiter (in-memory)",
		Category:    "performance",
		Tags:        []string{"rate-limit", "throttle", "token-bucket"},
		ConfigType:  "RateLimitConfig",
		DefaultFn:   func() any { return security.RateLimitConfig{} },
		Factory:     factoryRateLimit,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "sliding-window",
		Description: "Sliding window rate limiter (in-memory)",
		Category:    "performance",
		Tags:        []string{"rate-limit", "sliding-window", "throttle"},
		ConfigType:  "SlidingWindowConfig",
		DefaultFn:   func() any { return security.SlidingWindowConfig{} },
		Factory:     factorySlidingWindow,
	})

	// ── Logging & Observability ─────────────────────────────────────────
	c.Register(&MiddlewareDescriptor{
		Name:        "logger",
		Description: "Structured HTTP request/response logging with tracing integration",
		Category:    "observability",
		Tags:        []string{"log", "access-log", "structured", "tracing"},
		ConfigType:  "LoggerConfig",
		DefaultFn:   func() any { return middleware.LoggerConfig{} },
		Factory:     factoryLogger,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "request-id",
		Description: "Generates and injects unique request identifiers (X-Request-ID)",
		Category:    "observability",
		Tags:        []string{"request-id", "trace", "correlation"},
		ConfigType:  "RequestIDConfig",
		DefaultFn:   func() any { return RequestIDConfig{} },
		Factory:     factoryRequestID,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "response-format",
		Description: "Standardizes API response envelope format",
		Category:    "observability",
		Tags:        []string{"response", "format", "envelope", "api"},
		ConfigType:  "ResponseFormatConfig",
		DefaultFn:   func() any { return middleware.ResponseFormatConfig{} },
		Factory:     factoryResponseFormat,
	})

	// ── Error Handling ───────────────────────────────────────────────────
	c.Register(&MiddlewareDescriptor{
		Name:        "recovery",
		Description: "Recovers from panics with structured error logging",
		Category:    "utility",
		Tags:        []string{"panic", "recovery", "error-handling"},
		ConfigType:  "RecoveryConfig",
		DefaultFn:   func() any { return middleware.RecoveryConfig{} },
		Factory:     factoryRecovery,
	})

	// ── Performance ────────────────────────────────────────────────────
	c.Register(&MiddlewareDescriptor{
		Name:        "compress",
		Description: "HTTP response compression (gzip/brotli)",
		Category:    "performance",
		Tags:        []string{"gzip", "brotli", "compression", "performance"},
		ConfigType:  "CompressConfig",
		DefaultFn:   func() any { return middleware.CompressConfig{} },
		Factory:     factoryCompress,
	})
	c.Register(&MiddlewareDescriptor{
		Name:        "timeout",
		Description: "Per-request timeout enforcement",
		Category:    "performance",
		Tags:        []string{"timeout", "deadline", "performance"},
		ConfigType:  "TimeoutConfig",
		DefaultFn:   func() any { return TimeoutConfig{} },
		Factory:     factoryTimeout,
	})

	// ── Utility ────────────────────────────────────────────────────────
	c.Register(&MiddlewareDescriptor{
		Name:        "canary",
		Description: "Canary deployment routing by header, cookie, or hash",
		Category:    "utility",
		Tags:        []string{"canary", "deployment", "routing", "release"},
		ConfigType:  "CanaryConfig",
		DefaultFn:   func() any { return CanaryConfig{} },
		Factory:     factoryCanary,
	})
}

// ─── Simple Config Structs ──────────────────────────────────────────────────
// These exist for middleware that don't export a config struct or use non-map configs.

// RequestIDConfig is not exported by the middleware package, so we define a minimal one.
type RequestIDConfig struct {
	Header string `json:"header"` // Default: "X-Request-ID"
}

// TimeoutConfig holds timeout configuration.
type TimeoutConfig struct {
	Duration string `json:"duration"` // Go duration string, e.g. "30s"
}

// CanaryConfig holds canary middleware configuration.
type CanaryConfig struct {
	// Empty for now; canary uses its own rule structure.
}

// ─── Factory Implementations ──────────────────────────────────────────────────
// Each factory extracts config from map[string]any and calls the real middleware.

func factoryCORS(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.CORSPermissive(), nil
	}
	var cfg middleware.CORSConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.AllowOrigins) == 0 {
		cfg.AllowOrigins = []string{"*"}
	}
	return middleware.CORSWithConfig(cfg), nil
}

func factoryCSRF(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.CSRF("default-secret-change-me"), nil
	}
	var cfg middleware.CSRFConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Secret) == 0 {
		cfg.Secret = []byte("default-secret-change-me")
	}
	return middleware.CSRFWithConfig(cfg), nil
}

func factoryCSP(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.CSP(middleware.CSPConfig{}), nil
	}
	var cfg middleware.CSPConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	return middleware.CSP(cfg), nil
}

func factorySecureHeaders(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.SecureHeaders(), nil
	}
	var cfg middleware.SecureConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	return middleware.SecureHeaders(cfg), nil
}

func factoryJWT(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, fmt.Errorf("jwt middleware requires a 'secret' config key")
	}
	secret, _ := m["secret"].(string)
	if secret == "" {
		return nil, fmt.Errorf("jwt middleware: 'secret' is required")
	}
	return security.JWT(secret), nil
}

func factoryAPIKey(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return security.APIKey(security.APIKeyConfig{}), nil
	}
	var cfg security.APIKeyConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	return security.APIKey(cfg), nil
}

func factoryIPFilter(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return security.IPFilter(security.IPFilterConfig{}), nil
	}
	var cfg security.IPFilterConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	return security.IPFilter(cfg), nil
}

func factorySignature(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, fmt.Errorf("signature middleware requires a 'secret_key' config key")
	}
	var cfg security.SignatureConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.SecretKey) == 0 {
		if sk, ok := m["secret_key"].(string); ok {
			cfg.SecretKey = []byte(sk)
		}
	}
	return security.SignatureWithConfig(cfg), nil
}

func factoryTenant(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return security.Tenant(), nil
	}
	var cfg security.TenantConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	return security.Tenant(cfg), nil
}

func factoryRateLimit(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return security.RateLimit(100, 200), nil // sensible defaults
	}
	rate := 100.0
	burst := 200
	if v, ok := m["rate"].(float64); ok {
		rate = v
	}
	if v, ok := m["burst"].(float64); ok {
		burst = int(v)
	}
	return security.RateLimitWithConfig(security.RateLimitConfig{
		Rate:  rate,
		Burst: burst,
	}), nil
}

func factorySlidingWindow(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return security.SlidingWindow(1000, time.Minute), nil
	}
	limit := int64(1000)
	window := time.Minute
	if v, ok := m["limit"].(float64); ok {
		limit = int64(v)
	}
	if v, ok := m["window"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			window = d
		}
	}
	mw, _ := security.SlidingWindowWithConfig(security.SlidingWindowConfig{
		Limit:  limit,
		Window: window,
	}); return mw, nil
}

func factoryLogger(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.Logger(), nil
	}
	var cfg middleware.LoggerConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	return middleware.LoggerWithConfig(cfg), nil
}

func factoryRequestID(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.RequestID(), nil
	}
	// RequestID doesn't export a config struct; we just use WithGenerator if needed
	_ = m
	return middleware.RequestID(), nil
}

func factoryResponseFormat(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.ResponseFormat(), nil
	}
	var cfg middleware.ResponseFormatConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	return middleware.ResponseFormatWithConfig(cfg), nil
}

func factoryRecovery(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.Recovery(), nil
	}
	var cfg middleware.RecoveryConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	return middleware.RecoveryWithConfig(cfg), nil
}

func factoryCompress(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.Compress(), nil
	}
	var cfg middleware.CompressConfig
	if err := DecodeConfig(m, &cfg); err != nil {
		return nil, err
	}
	return middleware.CompressWithConfig(cfg), nil
}

func factoryTimeout(config any) (astra.HandlerFunc, error) {
	m, ok := config.(map[string]any)
	if !ok || len(m) == 0 {
		return middleware.Timeout(30 * time.Second), nil
	}
	duration := "30s"
	if v, ok := m["duration"].(string); ok {
		duration = v
	}
	d, err := time.ParseDuration(duration)
	if err != nil {
		return nil, fmt.Errorf("timeout: invalid duration %q: %w", duration, err)
	}
	return middleware.Timeout(d), nil
}

func factoryCanary(config any) (astra.HandlerFunc, error) {
	// Canary requires []CanaryRule which can't be easily expressed as map[string]any.
	// Return a pass-through handler when no rules are provided.
	return func(c *astra.Ctx) error { return c.Next() }, nil
}
