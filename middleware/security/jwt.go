package security

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/astra-go/astra"
	"github.com/golang-jwt/jwt/v5"
)

// DefaultJWTLeeway is the clock-skew tolerance applied when JWTConfig.Leeway
// is not explicitly set. 5 seconds covers typical NTP drift between a token
// issuer and this server without meaningfully extending token lifetime.
// SecretString wraps a secret value that redacts itself from all formatting
// and serialization outputs. Use this for HMAC keys, API tokens, and any
// sensitive string that must never appear in logs, error messages, or JSON.
//
// Create with NewSecretString or SecretStringFrom.
//
//	secret := middleware.NewSecretString("my-hmac-key")
//	_ = fmt.Sprintf("%v", secret)  // "[REDACTED]"
//	_ = json.Marshal(secret)        // "[REDACTED]"
//	_ = secret.Plain()              // "my-hmac-key"  (use sparingly)
//	_ = secret.String()             // "[REDACTED]"
//	_ = secret.GoString()           // "middleware.SecretString(3 bytes)"
type SecretString struct {
	val string
}

// NewSecretString creates a SecretString from a plain string.
func NewSecretString(s string) SecretString {
	return SecretString{val: s}
}

// String implements fmt.Stringer. Always returns "[REDACTED]".
func (SecretString) String() string { return "[REDACTED]" }

// GoString implements fmt.GoStringer. Returns a length hint without the value.
func (s SecretString) GoString() string {
	return fmt.Sprintf("middleware.SecretString(%d bytes)", len(s.val))
}

// MarshalText implements encoding.TextMarshaler. Always returns "[REDACTED]".
func (SecretString) MarshalText() ([]byte, error) { return []byte("[REDACTED]"), nil }

// MarshalJSON implements json.Marshaler. Always returns "[REDACTED]".
func (SecretString) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

// MarshalBinary implements encoding.BinaryMarshaler. Always returns "[REDACTED]".
// This prevents the secret value from leaking through gob or other binary codecs.
func (SecretString) MarshalBinary() ([]byte, error) { return []byte("[REDACTED]"), nil }

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
// SecretString must never be reconstructed from binary data; always returns an error.
func (*SecretString) UnmarshalBinary([]byte) error {
	return errors.New("SecretString: binary deserialization is not permitted")
}

// Plain returns the underlying secret value. Use sparingly — only when you
// need to pass the secret to a cryptographic function or third-party library.
func (s SecretString) Plain() string { return s.val }

// IsZero reports whether the secret is empty.
func (s SecretString) IsZero() bool { return s.val == "" }

const DefaultJWTLeeway = 30 * time.Second

// JWTCacheBackend is the interface that both the in-memory jwtCache and
// RedisJWTCache satisfy. Pass an implementation to JWTConfig.CacheBackend
// to plug in a custom or shared cache.
//
// Get returns (claims, true) on a hit, (nil, false) on a miss or error.
// Set stores claims with a TTL derived from expireAt (Unix seconds).
// Implementations must be safe for concurrent use.
type JWTCacheBackend interface {
	Get(ctx context.Context, sig string) (*Claims, bool)
	Set(ctx context.Context, sig string, claims *Claims, expireAt int64)
}

// StrictJWTLeeway disables clock-skew tolerance entirely. Pass it as
// JWTConfig.Leeway when tokens are short-lived or single-use and any
// post-expiry acceptance is unacceptable.
//
//	middleware.JWTWithConfig(middleware.JWTConfig{
//	    Secret: secret,
//	    Leeway: middleware.StrictJWTLeeway,
//	})
const StrictJWTLeeway = -1 * time.Nanosecond

// MinJWTKeyLength is the minimum acceptable byte-length for HMAC-SHA JWT secrets.
// HS256 produces 32-byte (256-bit) signatures; an equally strong key is required.
// Shorter keys severely weaken the MAC and are trivially brute-forced.
// Reject any HMACKey secret with len(secret) < MinJWTKeyLength.
const MinJWTKeyLength = 32

// MinRSABits is the minimum RSA key size (in bits) accepted by RSAPublicKey.
// 1024-bit RSA is broken; 2048 bits is the current baseline (NIST SP 800-57).
// Let's Encrypt, AWS, and most CAs require at least 2048 bits.
const MinRSABits = 2048

// MinECBits is the minimum ECDSA curve order (in bits) accepted by ECPublicKey.
// P-256 (256 bits) is the baseline; P-224 (224 bits) is weak and actively
// discouraged by NIST and major platforms.
const MinECBits = 256

// ClaimsKey is the context key under which the JWT middleware stores the parsed
// *Claims. Handlers retrieve claims via GetClaims(c) or c.Get(ClaimsKey).
//
// Written by: JWTWithConfig (using cfg.ContextKey, which defaults to ClaimsKey)
// Read by:    GetClaims, any handler that needs token claims
const ClaimsKey = "claims"

// Claims holds parsed JWT claims. Standard registered claims (sub, exp, iat…)
// are embedded; any custom fields are accessible through the Extra map.
type Claims struct {
	jwt.RegisteredClaims
	// Extra contains all non-registered claims from the token payload.
	Extra map[string]any
}

// JWTConfig configures the JWT middleware.
type JWTConfig struct {
	// KeyFunc returns the key used to validate the token signature.
	// Use HMACKey, RSAPublicKey, or ECPublicKey helpers, or supply your own.
	//
	// Required unless Secret is set.
	KeyFunc func(token *jwt.Token) (any, error)

	// AllowedAlgorithms is a whitelist of permitted signing algorithms.
	// If nil or empty, defaults to: HS256, HS384, HS512, RS256, RS384, RS512, ES256, ES384, ES512.
	// Use this to prevent algorithm confusion attacks (e.g., reject "none" or unexpected algorithms).
	//
	// Example: []string{"RS256", "RS384"} to only accept RSA-signed tokens.
	AllowedAlgorithms []string

	// Secret is a shorthand for HMAC (HS256) shared-secret validation.
	// Ignored when KeyFunc is set.
	//
	// Security: Secret is wrapped in a SecretString type that redacts itself
	// from fmt.Stringer, json.Marshaler, and fmt.GoStringer outputs to prevent
	// accidental leakage via logging or serialization.
	Secret SecretString

	// SecretRaw is the raw string form of Secret. Use this when you need to
	// pass the secret to a third-party library that expects a plain string.
	// Deprecated: Use Secret.Plain() instead. This field will be removed in a
	// future release. Prefer Secret for new code.
	//
	// If SecretRaw is set and Secret is empty, it is copied into Secret at
	// middleware init time for backward compatibility.
	SecretRaw string

	// ContextKey is the key used to store *Claims in the context. Default: "claims".
	ContextKey string

	// TokenLookup defines where to look for the token.
	// Format: "<source>:<name>" e.g. "header:Authorization", "query:token", "cookie:jwt"
	// Default: "header:Authorization"
	TokenLookup string

	// AuthScheme is the auth scheme in the Authorization header. Default: "Bearer".
	AuthScheme string

	// Leeway is the clock-skew tolerance applied to exp and nbf validation.
	//
	// Distributed systems always have some clock drift between the token issuer
	// and this server. A non-zero leeway prevents spurious 401s when the drift
	// is smaller than the configured value.
	//
	//   0 (not set)        — defaults to DefaultJWTLeeway (5s); covers typical NTP drift.
	//   StrictJWTLeeway    — no tolerance; token must be valid at exact current time.
	//   1s – 30s           — tune to your deployment's measured clock skew.
	//   > 30s              — avoid: effectively extends token lifetime by the leeway value.
	Leeway time.Duration

	// CacheBackend allows plugging in a custom JWT cache backend (e.g. Redis, multilevel).
	// When set, it takes priority over CacheSize; the internal L1 cache is not used.
	//
	// Example — two-level L1 (memory) + L2 (Redis) cache:
	//
	//	cache := middleware.NewMultiLevelJWTCache(1024, redisCache)
	//	app.Use(middleware.JWTWithConfig(middleware.JWTConfig{
	//	    Secret:       secret,
	//	    CacheBackend: cache,
	//	}))
	CacheBackend JWTCacheBackend

	// CacheSize is the maximum number of validated tokens to cache across all 16 shards.
	// A cached token's claims are returned directly on subsequent requests without
	// re-verifying the signature, trading cryptographic safety for throughput.
	//
	// Security implications:
	//   - Key rotation: cached entries remain valid until their exp even after the
	//     signing key changes. Drain or restart to clear the cache after rotation.
	//   - Token revocation: a revoked token that has already been cached will continue
	//     to pass validation until it expires. Do not use caching with revocation.
	//
	// Recommended values: 512–2048. 0 (default) disables caching.
	// Ignored when CacheBackend is set.
	CacheSize int

	// RevokeStore, when set, is checked on every request after signature
	// verification. If the token's signature is present in the store the
	// request is rejected with 401 Unauthorized, even if the token is
	// otherwise cryptographically valid and not yet expired.
	//
	// Use NewMemoryRevokeStore for single-instance deployments. For
	// multi-instance deployments implement TokenRevokeStore backed by a
	// shared store (e.g. Redis) so that a revocation on one instance is
	// immediately visible to all others.
	//
	// Revoke a token by calling store.Revoke(tokenSignature(raw), expireAt).
	// The helper RevokeToken(store, rawToken) extracts the signature for you.
	RevokeStore TokenRevokeStore

	// Skipper skips JWT validation for matching requests.
	// Overrides the deprecated SkipFunc field if both are set.
	Skipper Skipper

	// SkipFunc allows skipping JWT validation for certain requests.
	// Deprecated: use Skipper instead.
	SkipFunc func(*astra.Ctx) bool

	// ErrorHandler is called when validation fails.
	// If nil, a 401 Unauthorized response is written.
	ErrorHandler func(*astra.Ctx, error) error
}

// JWT returns a middleware that validates HS256 JWT tokens.
//
// Claims are stored in the context under the key "claims".
func JWT(secret string) astra.HandlerFunc {
	return JWTWithConfig(JWTConfig{Secret: NewSecretString(secret)})
}

// JWTWithConfig returns a JWT middleware with custom configuration.
func JWTWithConfig(cfg JWTConfig) astra.HandlerFunc {
	if cfg.ContextKey == "" {
		cfg.ContextKey = ClaimsKey
	}
	if cfg.AuthScheme == "" {
		cfg.AuthScheme = "Bearer"
	}
	if cfg.TokenLookup == "" {
		cfg.TokenLookup = "header:Authorization"
	}
	// Apply default leeway only when the caller left it at the zero value.
	// An explicit cfg.Leeway = 0 (set via a named field) is indistinguishable
	// from the zero value in Go, so we use the sentinel -1 to mean "strict".
	// Callers who want strict mode should use WithJWTStrictLeeway().
	leeway := cfg.Leeway
	if leeway == 0 {
		leeway = 5 * time.Second
	} else if leeway < 0 {
		leeway = 0
	}

	// Backward compatibility: if SecretRaw is set but Secret is empty, migrate.
	if cfg.Secret.IsZero() && cfg.SecretRaw != "" {
		cfg.Secret = NewSecretString(cfg.SecretRaw)
	}

	// Build keyFunc: prefer explicit KeyFunc, fall back to HMAC.
	// Wrap keyFunc to reject algorithm confusion attacks:
	//   - "alg: none" bypass
	//   - RSA public key as HMAC secret (Key Confusion)
	originalKeyFunc := cfg.KeyFunc
	if originalKeyFunc == nil {
		if cfg.Secret.IsZero() {
			panic("jwt middleware: either Secret or KeyFunc must be set")
		}
		originalKeyFunc = HMACKey(cfg.Secret.Plain())
	}

	// Set default allowed algorithms if not specified
	allowedAlgs := cfg.AllowedAlgorithms
	if len(allowedAlgs) == 0 {
		allowedAlgs = []string{"HS256", "HS384", "HS512", "RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}
	}
	allowedAlgSet := make(map[string]struct{}, len(allowedAlgs))
	for _, alg := range allowedAlgs {
		allowedAlgSet[alg] = struct{}{}
	}

	// Wrapped keyFunc that validates the "alg" header against whitelist.
	keyFunc := func(t *jwt.Token) (any, error) {
		// Check algorithm whitelist first
		alg := t.Method.Alg()
		if _, ok := allowedAlgSet[alg]; !ok {
			return nil, fmt.Errorf("astra/jwt: unexpected signing method: %v", t.Header["alg"])
		}
		// Reject "alg: none" explicitly (CVE-2015-2951)
		if t.Method == jwt.SigningMethodNone {
			return nil, fmt.Errorf("astra/jwt: unexpected signing method: %v", t.Header["alg"])
		}
		return originalKeyFunc(t)
	}

	parts := strings.SplitN(cfg.TokenLookup, ":", 2)
	source, name := parts[0], parts[1]

	// Pre-built parser reused for every request — saves one *jwt.Parser alloc per call.
	parser := jwt.NewParser(jwt.WithExpirationRequired(), jwt.WithLeeway(leeway))

	// Resolve cache backend: explicit CacheBackend > in-memory L1 (CacheSize) > nil
	var cacheBackend JWTCacheBackend
	if cfg.CacheBackend != nil {
		cacheBackend = cfg.CacheBackend
	} else if cfg.CacheSize > 0 {
		cacheBackend = newJWTCache(cfg.CacheSize)
	}

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) || (cfg.SkipFunc != nil && cfg.SkipFunc(c)) {
			c.Next()
			return nil
		}

		raw := extractToken(c, source, name, cfg.AuthScheme)
		if raw == "" {
			err := astra.NewHTTPError(http.StatusUnauthorized, "missing token")
			if cfg.ErrorHandler != nil {
				return cfg.ErrorHandler(c, err)
			}
			return err
		}

		var claims *Claims
		var pooled bool // true when we allocated from claimsPool and must return it

		if cacheBackend != nil {
			sig := tokenSignature(raw)
			ctx := c.Request().Context()
			if cached, ok := cacheBackend.Get(ctx, sig); ok {
				// Revocation check must happen even for cached tokens.
				if cfg.RevokeStore != nil && cfg.RevokeStore.IsRevoked(sig) {
					// Best-effort delete from cache backend
					type deleter interface {
						Delete(context.Context, string) error
					}
					if d, ok := cacheBackend.(deleter); ok {
						_ = d.Delete(ctx, sig)
					}
					err := astra.NewHTTPError(http.StatusUnauthorized, "token has been revoked")
					if cfg.ErrorHandler != nil {
						return cfg.ErrorHandler(c, err)
					}
					return err
				}
				claims = cached
			} else {
				var err error
				claims, err = parseTokenPooled(raw, parser, keyFunc)
				if err != nil {
					he := astra.NewHTTPError(http.StatusUnauthorized, err.Error()).WithInternal(err)
					if cfg.ErrorHandler != nil {
						return cfg.ErrorHandler(c, he)
					}
					return he
				}
				if cfg.RevokeStore != nil && cfg.RevokeStore.IsRevoked(sig) {
					releaseClaims(claims)
					he := astra.NewHTTPError(http.StatusUnauthorized, "token has been revoked")
					if cfg.ErrorHandler != nil {
						return cfg.ErrorHandler(c, he)
					}
					return he
				}
				if claims.RegisteredClaims.ExpiresAt != nil {
					cacheBackend.Set(ctx, sig, claims, claims.RegisteredClaims.ExpiresAt.Unix())
				} else {
					pooled = true
				}
			}
		} else {
			var err error
			claims, err = parseTokenPooled(raw, parser, keyFunc)
			if err != nil {
				he := astra.NewHTTPError(http.StatusUnauthorized, err.Error()).WithInternal(err)
				if cfg.ErrorHandler != nil {
					return cfg.ErrorHandler(c, he)
				}
				return he
			}
			if cfg.RevokeStore != nil {
				sig := tokenSignature(raw)
				if cfg.RevokeStore.IsRevoked(sig) {
					releaseClaims(claims)
					he := astra.NewHTTPError(http.StatusUnauthorized, "token has been revoked")
					if cfg.ErrorHandler != nil {
						return cfg.ErrorHandler(c, he)
					}
					return he
				}
			}
			pooled = true
		}

		c.Set(cfg.ContextKey, claims)
		c.Next()
		if pooled {
			releaseClaims(claims)
		}
		return nil
	}
}

// GetClaims retrieves parsed JWT Claims from the context.
// Returns nil when the JWT middleware was not applied or the request is
// unauthenticated.
func GetClaims(c *astra.Ctx) *Claims {
	v, _ := c.Get(ClaimsKey)
	claims, _ := v.(*Claims)
	return claims
}

// ─── Key helpers ──────────────────────────────────────────────────────────────

// HMACKey returns a KeyFunc for HMAC-SHA256 signed tokens (HS256).
func HMACKey(secret string) func(*jwt.Token) (any, error) {
	if len(secret) < MinJWTKeyLength {
		panic(fmt.Sprintf("jwt middleware: HMAC key must be at least %d bytes (got %d); use a longer secret", MinJWTKeyLength, len(secret)))
	}
	key := []byte(secret)
	return func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	}
}

// JWTFromEnv reads the JWT HMAC secret from the named environment variable.
// It panics if the variable is unset or its value is shorter than MinJWTKeyLength.
//
//	Typical usage: middleware.JWTWithConfig(middleware.JWTConfig{
//	    Secret: middleware.JWTFromEnv("JWT_SECRET"),
//	})
func JWTFromEnv(envVar string) SecretString {
	v := os.Getenv(envVar)
	if v == "" {
		panic(fmt.Sprintf("jwt middleware: environment variable %s is not set", envVar))
	}
	if len(v) < MinJWTKeyLength {
		panic(fmt.Sprintf("jwt middleware: %s value must be at least %d bytes (got %d)", envVar, MinJWTKeyLength, len(v)))
	}
	return NewSecretString(v)
}

// RSAPublicKey returns a KeyFunc for RSA-signed tokens (RS256 / RS384 / RS512).
// pemBytes must be a PEM-encoded PKIX public key or certificate.
func RSAPublicKey(pemBytes []byte) func(*jwt.Token) (any, error) {
	key, err := parseRSAPublicKey(pemBytes)
	if err != nil {
		return func(*jwt.Token) (any, error) { return nil, err }
	}
	// Enforce minimum key size to reject weak RSA keys (NIST SP 800-57)
	if key.N.BitLen() < MinRSABits {
		return func(*jwt.Token) (any, error) {
			return nil, fmt.Errorf("jwt: RSA key size must be at least %d bits (got %d)", MinRSABits, key.N.BitLen())
		}
	}
	pub := key
	return func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pub, nil
	}
}

// ECPublicKey returns a KeyFunc for ECDSA-signed tokens (ES256 / ES384 / ES512).
// pemBytes must be a PEM-encoded PKIX public key.
func ECPublicKey(pemBytes []byte) func(*jwt.Token) (any, error) {
	key, err := parseECPublicKey(pemBytes)
	if err != nil {
		return func(*jwt.Token) (any, error) { return nil, err }
	}
	// Enforce minimum curve order to reject weak EC curves (P-224, P-192, ...)
	curveBits := key.Curve.Params().BitSize
	if curveBits < MinECBits {
		return func(*jwt.Token) (any, error) {
			return nil, fmt.Errorf("jwt: EC key curve must be at least %d bits (got %d)", MinECBits, curveBits)
		}
	}
	ecKey := key
	return func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return ecKey, nil
	}
}

// ─── Pool helpers ─────────────────────────────────────────────────────────────

// claimsPool pools *Claims to eliminate one heap allocation per validated request.
var claimsPool = sync.Pool{New: func() any { return new(Claims) }}

// ndPool pools *jwt.NumericDate to eliminate up to three heap allocations per
// request (exp, nbf, iat). Only non-cached Claims use this pool; the cache owns
// its Claims pointers indefinitely.
var ndPool = sync.Pool{New: func() any { return new(jwt.NumericDate) }}

// releaseClaims returns a non-cached *Claims (and its *NumericDate children)
// to their respective pools. Must only be called after c.Next() returns, once
// all downstream handlers have finished reading the claims.
func releaseClaims(c *Claims) {
	if c == nil {
		return
	}
	if c.ExpiresAt != nil {
		*c.ExpiresAt = jwt.NumericDate{}
		ndPool.Put(c.ExpiresAt)
		c.ExpiresAt = nil
	}
	if c.NotBefore != nil {
		*c.NotBefore = jwt.NumericDate{}
		ndPool.Put(c.NotBefore)
		c.NotBefore = nil
	}
	if c.IssuedAt != nil {
		*c.IssuedAt = jwt.NumericDate{}
		ndPool.Put(c.IssuedAt)
		c.IssuedAt = nil
	}
	c.RegisteredClaims = jwt.RegisteredClaims{}
	c.Extra = nil
	claimsPool.Put(c)
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// extractToken extracts the raw JWT string from the request.
func extractToken(c *astra.Ctx, source, name, scheme string) string {
	switch source {
	case "header":
		auth := c.Header(name)
		if auth == "" {
			return ""
		}
		// Case-insensitive scheme match: "Bearer", "bearer", "BEARER" all work.
		if len(auth) > len(scheme) &&
			strings.EqualFold(auth[:len(scheme)], scheme) &&
			auth[len(scheme)] == ' ' {
			return auth[len(scheme)+1:]
		}
		return auth
	case "query":
		return c.Query(name)
	case "cookie":
		cookie, err := c.Request().Cookie(name)
		if err != nil {
			return ""
		}
		return cookie.Value
	}
	return ""
}

// registeredClaimKeys is the set of standard JWT registered claim names (RFC 7519 §4.1).
// Declared at package level to avoid one map allocation per validated request.
var registeredClaimKeys = map[string]struct{}{
	"iss": {}, "sub": {}, "aud": {},
	"exp": {}, "nbf": {}, "iat": {}, "jti": {},
}

// jwtMCPool pools *jwt.MapClaims to reduce per-request map allocations.
// The map is cleared before each use so pooled instances never leak claim data.
var jwtMCPool = sync.Pool{
	New: func() any {
		mc := make(jwt.MapClaims, 8)
		return &mc
	},
}

// parseTokenPooled parses and validates raw using pooled *jwt.MapClaims and a
// pooled *Claims. The returned *Claims must be released via releaseClaims once
// the handler chain has finished — unless ownership is transferred to the cache.
func parseTokenPooled(raw string, p *jwt.Parser, keyFunc jwt.Keyfunc) (*Claims, error) {
	mcp := jwtMCPool.Get().(*jwt.MapClaims)
	defer func() {
		for k := range *mcp {
			delete(*mcp, k)
		}
		jwtMCPool.Put(mcp)
	}()

	token, err := p.ParseWithClaims(raw, mcp, keyFunc)
	if err != nil {
		return nil, humanizeJWTError(err)
	}
	if !token.Valid {
		return nil, errJWTInvalid
	}

	mc := *mcp
	claims := claimsPool.Get().(*Claims)
	claims.RegisteredClaims = mapClaimsToRegisteredPooled(mc)

	for k, v := range mc {
		if _, ok := registeredClaimKeys[k]; !ok {
			if claims.Extra == nil {
				claims.Extra = make(map[string]any, 4)
			}
			claims.Extra[k] = v
		}
	}

	return claims, nil
}

// mapClaimsToRegisteredPooled converts MapClaims to a typed jwt.RegisteredClaims,
// fetching *jwt.NumericDate instances from ndPool instead of allocating them.
func mapClaimsToRegisteredPooled(mc jwt.MapClaims) jwt.RegisteredClaims {
	var reg jwt.RegisteredClaims
	if v, ok := mc["iss"].(string); ok {
		reg.Issuer = v
	}
	if v, ok := mc["sub"].(string); ok {
		reg.Subject = v
	}
	if v, ok := mc["jti"].(string); ok {
		reg.ID = v
	}
	switch v := mc["aud"].(type) {
	case string:
		reg.Audience = jwt.ClaimStrings{v}
	case []any:
		aud := make(jwt.ClaimStrings, 0, len(v))
		for _, a := range v {
			if s, ok := a.(string); ok {
				aud = append(aud, s)
			}
		}
		reg.Audience = aud
	}
	if v, ok := mc["exp"].(float64); ok {
		nd := ndPool.Get().(*jwt.NumericDate)
		nd.Time = time.Unix(int64(v), 0)
		reg.ExpiresAt = nd
	}
	if v, ok := mc["nbf"].(float64); ok {
		nd := ndPool.Get().(*jwt.NumericDate)
		nd.Time = time.Unix(int64(v), 0)
		reg.NotBefore = nd
	}
	if v, ok := mc["iat"].(float64); ok {
		nd := ndPool.Get().(*jwt.NumericDate)
		nd.Time = time.Unix(int64(v), 0)
		reg.IssuedAt = nd
	}
	return reg
}

// Pre-declared sentinel errors returned by humanizeJWTError.
// Package-level allocation eliminates the errors.New call (and its string alloc)
// on every failed validation, replacing it with a pointer comparison + return.
var (
	errJWTExpired     = errors.New("token expired")
	errJWTNotYetValid = errors.New("token not yet valid")
	errJWTSignature   = errors.New("invalid token signature")
	errJWTMalformed   = errors.New("malformed token")
	errJWTInvalid     = errors.New("invalid token")
)

func humanizeJWTError(err error) error {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return errJWTExpired
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		return errJWTNotYetValid
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		return errJWTSignature
	case errors.Is(err, jwt.ErrTokenMalformed):
		return errJWTMalformed
	case strings.Contains(err.Error(), "unexpected signing method"):
		// Preserve algorithm confusion attack error message
		return err
	default:
		return errJWTInvalid
	}
}

func parseRSAPublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("jwt: failed to decode PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Try as certificate
		cert, cerr := x509.ParseCertificate(block.Bytes)
		if cerr != nil {
			return nil, fmt.Errorf("jwt: parse public key: %w", err)
		}
		pub = cert.PublicKey
	}
	rsaKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("jwt: PEM key is not an RSA public key")
	}
	return rsaKey, nil
}

func parseECPublicKey(pemBytes []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("jwt: failed to decode PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("jwt: parse EC public key: %w", err)
	}
	ecKey, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("jwt: PEM key is not an EC public key")
	}
	return ecKey, nil
}
