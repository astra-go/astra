// Package middleware — HMAC request-signature verification.
//
// Protects API endpoints from replay attacks by requiring callers to include
// a timestamp, a one-time nonce, and an HMAC-SHA256 signature over the
// canonical request string.
//
// # Canonical string format
//
//	{method}\n{path}\n{timestamp}\n{nonce}\n{body-sha256-hex}
//
// The body hash is SHA-256 of the raw request body (empty string → SHA-256 of "").
//
// # Client-side signing (pseudocode)
//
//	timestamp = unix_seconds_now()
//	nonce     = random_hex(16)
//	body_hash = sha256_hex(request_body)
//	canonical = method + "\n" + path + "\n" + str(timestamp) + "\n" + nonce + "\n" + body_hash
//	signature = hmac_sha256_hex(secret_key, canonical)
//
//	headers:
//	  X-Timestamp: <timestamp>
//	  X-Nonce:     <nonce>
//	  X-Signature: <signature>
//
// # Usage
//
//	app.Use(middleware.Signature([]byte("my-shared-secret")))
//
//	// Fine-grained config
//	app.Use(middleware.SignatureWithConfig(middleware.SignatureConfig{
//	    SecretKey:    []byte(os.Getenv("API_SECRET")),
//	    TimestampTTL: 5 * time.Minute,
//	    NonceWindow:  5 * time.Minute,
//	    NonceStore:   myRedisStore,  // implement SignatureNonceStore interface
//	}))
package security

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/astra-go/astra"
)

// SignatureNonceStore is the interface for nonce deduplication storage.
// The default in-memory store is sufficient for single-node deployments;
// use a Redis-backed implementation for multi-instance services.
type SignatureNonceStore interface {
	// Seen returns true if the nonce was already consumed within the window.
	// If not seen, the implementation MUST record the nonce before returning false.
	Seen(nonce string, window time.Duration) (bool, error)
}

// SignatureConfig configures the HMAC signature middleware.
type SignatureConfig struct {
	// SecretKey is the HMAC shared secret. Required.
	SecretKey []byte

	// TimestampHeader is the request header that carries the Unix timestamp (seconds).
	// Default: "X-Timestamp".
	TimestampHeader string

	// NonceHeader is the request header that carries the one-time nonce.
	// Default: "X-Nonce".
	NonceHeader string

	// SignatureHeader is the request header that carries the hex-encoded HMAC.
	// Default: "X-Signature".
	SignatureHeader string

	// TimestampTTL is the acceptable clock skew between client and server.
	// Requests outside ±TimestampTTL are rejected. Default: 5 minutes.
	TimestampTTL time.Duration

	// NonceWindow is the window for which a nonce is remembered.
	// Should be >= TimestampTTL * 2. Default: 10 minutes.
	NonceWindow time.Duration

	// NonceStore is the nonce deduplication backend.
	// Default: in-memory (not suitable for multi-node).
	NonceStore SignatureNonceStore

	// Skipper skips verification for matching requests.
	Skipper Skipper
}

// Signature returns an HMAC request-signature middleware using default settings.
func Signature(secretKey []byte) astra.HandlerFunc {
	return SignatureWithConfig(SignatureConfig{SecretKey: secretKey})
}

// SignatureWithConfig returns an HMAC request-signature middleware.
func SignatureWithConfig(cfg SignatureConfig) astra.HandlerFunc {
	if len(cfg.SecretKey) == 0 {
		panic("middleware: Signature: SecretKey must not be empty")
	}
	if cfg.TimestampHeader == "" {
		cfg.TimestampHeader = "X-Timestamp"
	}
	if cfg.NonceHeader == "" {
		cfg.NonceHeader = "X-Nonce"
	}
	if cfg.SignatureHeader == "" {
		cfg.SignatureHeader = "X-Signature"
	}
	if cfg.TimestampTTL == 0 {
		cfg.TimestampTTL = 5 * time.Minute
	}
	if cfg.NonceWindow == 0 {
		cfg.NonceWindow = 10 * time.Minute
	}
	if cfg.NonceStore == nil {
		cfg.NonceStore = NewInMemoryNonceStore()
	}

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			c.Next()
			return nil
		}

		// ── 1. Read headers ─────────────────────────────────────────
		tsStr := c.Request().Header.Get(cfg.TimestampHeader)
		nonce := c.Request().Header.Get(cfg.NonceHeader)
		sig := c.Request().Header.Get(cfg.SignatureHeader)

		if tsStr == "" || nonce == "" || sig == "" {
			return c.JSON(http.StatusUnauthorized, map[string]any{
				"error": "missing signature headers",
			})
		}

		// ── 2. Validate timestamp ────────────────────────────────────
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]any{"error": "invalid timestamp"})
		}
		diff := time.Since(time.Unix(ts, 0))
		if math.Abs(diff.Seconds()) > cfg.TimestampTTL.Seconds() {
			return c.JSON(http.StatusUnauthorized, map[string]any{"error": "timestamp expired"})
		}

		// ── 3. Check nonce replay ────────────────────────────────────
		seen, err := cfg.NonceStore.Seen(nonce, cfg.NonceWindow)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "nonce store error"})
		}
		if seen {
			return c.JSON(http.StatusUnauthorized, map[string]any{"error": "nonce already used"})
		}

		// ── 4. Read and restore request body ────────────────────────
		var bodyBytes []byte
		if c.Request().Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request().Body)
			r := c.Request()
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			c.SetRequest(r)
		}
		bodyHash := sha256Hex(bodyBytes)

		// ── 5. Rebuild canonical string ──────────────────────────────
		canonical := fmt.Sprintf("%s\n%s\n%s\n%s\n%s",
			c.Request().Method,
			c.Request().URL.RequestURI(),
			tsStr,
			nonce,
			bodyHash,
		)

		// ── 6. Verify HMAC ───────────────────────────────────────────
		mac := hmac.New(sha256.New, cfg.SecretKey)
		mac.Write([]byte(canonical))
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(sig)) {
			return c.JSON(http.StatusUnauthorized, map[string]any{"error": "invalid signature"})
		}

		c.Next()
		return nil
	}
}

// sha256Hex returns the lowercase hex-encoded SHA-256 hash of b.
func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ─── In-memory nonce store ────────────────────────────────────────────────────

// InMemoryNonceStore is a goroutine-safe in-memory nonce deduplication store.
// It is suitable for single-node deployments. For multi-instance services,
// use a Redis-backed implementation.
// Call Close to stop the background reaper goroutine.
type InMemoryNonceStore struct {
	mu     sync.Mutex
	nonces map[string]time.Time
	stop   chan struct{}
	once   sync.Once
}

// NewInMemoryNonceStore creates a new InMemoryNonceStore with a background
// reaper that evicts expired nonces every minute.
func NewInMemoryNonceStore() *InMemoryNonceStore {
	s := &InMemoryNonceStore{
		nonces: make(map[string]time.Time),
		stop:   make(chan struct{}),
	}
	go s.reap()
	return s
}

// Seen returns true if the nonce was already recorded within its window.
// If not seen, the nonce is recorded before returning false.
func (s *InMemoryNonceStore) Seen(nonce string, window time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if exp, ok := s.nonces[nonce]; ok && time.Now().Before(exp) {
		return true, nil
	}
	s.nonces[nonce] = time.Now().Add(window)
	return false, nil
}

// Close stops the background reaper. Safe to call multiple times.
func (s *InMemoryNonceStore) Close() {
	s.once.Do(func() { close(s.stop) })
}

func (s *InMemoryNonceStore) reap() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for k, exp := range s.nonces {
				if now.After(exp) {
					delete(s.nonces, k)
				}
			}
			s.mu.Unlock()
		case <-s.stop:
			return
		}
	}
}
