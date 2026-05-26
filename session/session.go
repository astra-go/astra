// Package session provides HTTP session management for Astra.
//
// Sessions are backed by a pluggable Store (currently Redis is provided).
// The session ID is stored in a signed HttpOnly cookie; all data lives in
// the store. This means:
//   - No user-visible data leakage (cookie only carries the opaque ID)
//   - Sessions survive server restarts
//   - Easy revocation by deleting from the store
//
// # Quick start
//
//	import (
//	    "github.com/astra-go/astra/session"
//	    sessredis "github.com/astra-go/astra/session/redis"
//	)
//
//	store := sessredis.New(redisClient, sessredis.Config{KeyPrefix: "sess:"})
//	app.Use(session.Middleware(store))
//
//	app.POST("/login", func(c **astra.Ctx) error {
//	    sess := session.Get(c)
//	    sess.Set("user_id", 42)
//	    return c.JSON(200, astra.Map{"ok": true})
//	})
//
//	app.GET("/profile", func(c **astra.Ctx) error {
//	    sess := session.Get(c)
//	    uid, ok := sess.GetInt("user_id")
//	    if !ok { return astra.NewHTTPError(401, "not authenticated") }
//	    return c.JSON(200, astra.Map{"user_id": uid})
//	})
//
// # Session lifecycle
//
// The middleware automatically:
//  1. Reads the session cookie on every request.
//  2. Loads session data from the store (or starts a new empty session).
//  3. After the handler returns, saves any dirty session back to the store and
//     refreshes the cookie.
//
// Calling Destroy() inside a handler marks the session for deletion: the store
// entry and cookie are removed when the response is written.
//
// # Cookie signing
//
// The session ID is HMAC-SHA256 signed with SecretKey to prevent cookie
// forgery. Set a strong random key (≥32 bytes) in Config.SecretKey.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/astra-go/astra"
)

// Store is the session data persistence backend.
type Store interface {
	// Load retrieves session data by ID. Returns an empty non-nil map when
	// the session does not exist (callers treat missing as a new session).
	Load(ctx context.Context, id string) (map[string]any, error)

	// Save persists session data for the given ID with the provided TTL.
	Save(ctx context.Context, id string, values map[string]any, ttl time.Duration) error

	// Delete removes the session from the store.
	Delete(ctx context.Context, id string) error
}

// Config configures the session middleware.
type Config struct {
	// SecretKey is used to HMAC-sign the cookie value.
	// Must be set; recommended length ≥ 32 bytes.
	SecretKey string

	// CookieName is the HTTP cookie name. Default: "sid".
	CookieName string

	// CookiePath is the cookie Path attribute. Default: "/".
	CookiePath string

	// CookieDomain sets the cookie Domain attribute.
	CookieDomain string

	// CookieMaxAge is the cookie Max-Age in seconds.
	// 0 means a session cookie (expires when the browser closes).
	// Default: 0.
	CookieMaxAge int

	// Secure sets the cookie Secure attribute.
	// Must be true in production (HTTPS). Default: false.
	Secure bool

	// HTTPOnly sets the cookie HttpOnly attribute. Default: true.
	HTTPOnly bool

	// SameSite controls the SameSite cookie attribute. Default: SameSiteLaxMode.
	SameSite http.SameSite

	// IdleTimeout is the TTL applied when saving sessions to the store.
	// Default: 24 hours.
	IdleTimeout time.Duration
}

func (c *Config) setDefaults() {
	if c.CookieName == "" {
		c.CookieName = "sid"
	}
	if c.CookiePath == "" {
		c.CookiePath = "/"
	}
	if c.SameSite == 0 {
		c.SameSite = http.SameSiteLaxMode
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = 24 * time.Hour
	}
	if !c.HTTPOnly {
		c.HTTPOnly = true // override zero-value to default true
	}
}

// contextKey is the unexported key used to store the session in *astra.Ctx.
// We use a string key matching the type name to avoid any conversion issues.
const sessionContextKey = "github.com/astra-go/astra/session.Session"

// Session holds the per-request session state.
// All methods are safe for concurrent use.
type Session struct {
	id        string
	values    map[string]any
	mu        sync.RWMutex
	dirty     bool
	isNew     bool
	destroyed bool

	store Store
	cfg   Config
}

// ID returns the session ID.
func (s *Session) ID() string { return s.id }

// IsNew reports whether the session was created in this request.
func (s *Session) IsNew() bool { return s.isNew }

// Set stores value under key and marks the session as dirty.
func (s *Session) Set(key string, value any) {
	s.mu.Lock()
	s.values[key] = value
	s.dirty = true
	s.mu.Unlock()
}

// Get returns the value stored under key and whether it was found.
func (s *Session) Get(key string) (any, bool) {
	s.mu.RLock()
	v, ok := s.values[key]
	s.mu.RUnlock()
	return v, ok
}

// GetString is a typed helper that returns the string value at key.
func (s *Session) GetString(key string) (string, bool) {
	v, ok := s.Get(key)
	if !ok {
		return "", false
	}
	str, ok := v.(string)
	return str, ok
}

// GetInt is a typed helper for integer values.
// JSON round-trips store numbers as float64; this handles both.
func (s *Session) GetInt(key string) (int, bool) {
	v, ok := s.Get(key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// GetInt64 is a typed helper for int64 values.
func (s *Session) GetInt64(key string) (int64, bool) {
	v, ok := s.Get(key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}

// Delete removes key from the session.
func (s *Session) Delete(key string) {
	s.mu.Lock()
	delete(s.values, key)
	s.dirty = true
	s.mu.Unlock()
}

// Clear removes all keys from the session.
func (s *Session) Clear() {
	s.mu.Lock()
	s.values = make(map[string]any)
	s.dirty = true
	s.mu.Unlock()
}

// Destroy marks the session for deletion.
// On response flush the store entry is removed and the cookie is expired.
func (s *Session) Destroy() {
	s.mu.Lock()
	s.destroyed = true
	s.dirty = true
	s.mu.Unlock()
}

// Middleware returns an Astra middleware that loads, injects, and auto-saves
// the session on every request.
func Middleware(store Store, cfgs ...Config) astra.MiddlewareFunc {
	cfg := Config{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	cfg.setDefaults()
	if cfg.SecretKey == "" {
		panic("session: Config.SecretKey must not be empty")
	}

	return func(c *astra.Ctx) error {
		sess := load(c.Request(), store, cfg)
		c.Set(sessionContextKey, sess)

		c.Next()

		// Flush: save or destroy session, update cookie.
		flush(c.Writer(), c.Request(), sess, cfg)
		return nil
	}
}

// Get retrieves the session from the Astra context.
// Panics if Middleware was not applied — prevents silent nil dereferences.
func Get(c *astra.Ctx) *Session {
	v, ok := c.Get(sessionContextKey)
	if !ok {
		panic("session: Middleware not applied on this route")
	}
	return v.(*Session)
}

// ─── internal ────────────────────────────────────────────────────────────────

// load reads the session cookie, verifies the signature, and loads data from
// the store. Always returns a non-nil *Session.
func load(r *http.Request, store Store, cfg Config) *Session {
	sess := &Session{
		store: store,
		cfg:   cfg,
	}

	id := readCookie(r, cfg)
	if id == "" {
		// No valid cookie — start a fresh session.
		sess.id = uuid.NewString()
		sess.values = make(map[string]any)
		sess.isNew = true
		return sess
	}

	values, err := store.Load(r.Context(), id)
	if err != nil {
		slog.Warn("session: load error", "err", err)
		sess.id = uuid.NewString()
		sess.values = make(map[string]any)
		sess.isNew = true
		return sess
	}

	sess.id = id
	sess.values = values
	sess.isNew = len(values) == 0
	return sess
}

// flush saves or destroys the session and updates the response cookie.
func flush(w http.ResponseWriter, r *http.Request, sess *Session, cfg Config) {
	sess.mu.RLock()
	destroyed := sess.destroyed
	dirty := sess.dirty
	sess.mu.RUnlock()

	if destroyed {
		_ = sess.store.Delete(r.Context(), sess.id)
		http.SetCookie(w, expireCookie(cfg))
		return
	}

	if !dirty && !sess.isNew {
		return // nothing to persist
	}

	sess.mu.RLock()
	values := copyValues(sess.values)
	sess.mu.RUnlock()

	if err := sess.store.Save(r.Context(), sess.id, values, cfg.IdleTimeout); err != nil {
		slog.Error("session: save error", "err", err)
		return
	}
	http.SetCookie(w, buildCookie(sess.id, cfg))
}

// readCookie parses the session ID from the request cookie.
// Returns empty string if the cookie is absent or the signature is invalid.
func readCookie(r *http.Request, cfg Config) string {
	cookie, err := r.Cookie(cfg.CookieName)
	if err != nil {
		return ""
	}
	return verifySigned(cookie.Value, cfg.SecretKey)
}

// buildCookie constructs the session cookie with the signed ID.
func buildCookie(id string, cfg Config) *http.Cookie {
	return &http.Cookie{
		Name:     cfg.CookieName,
		Value:    sign(id, cfg.SecretKey),
		Path:     cfg.CookiePath,
		Domain:   cfg.CookieDomain,
		MaxAge:   cfg.CookieMaxAge,
		Secure:   cfg.Secure,
		HttpOnly: cfg.HTTPOnly,
		SameSite: cfg.SameSite,
	}
}

// expireCookie returns a cookie with MaxAge=-1 to instruct the browser to
// delete the session cookie.
func expireCookie(cfg Config) *http.Cookie {
	return &http.Cookie{
		Name:     cfg.CookieName,
		Value:    "",
		Path:     cfg.CookiePath,
		Domain:   cfg.CookieDomain,
		MaxAge:   -1,
		Secure:   cfg.Secure,
		HttpOnly: cfg.HTTPOnly,
		SameSite: cfg.SameSite,
	}
}

// ─── cookie signing (HMAC-SHA256) ────────────────────────────────────────────

// sign returns "<id>.<base64(hmac(id, key))>".
func sign(id, key string) string {
	mac := computeMAC(id, key)
	return id + "." + base64.RawURLEncoding.EncodeToString(mac)
}

// verifySigned parses and verifies a signed cookie value.
// Returns the raw ID on success, or "" on failure.
func verifySigned(value, key string) string {
	dot := strings.LastIndexByte(value, '.')
	if dot < 0 {
		return ""
	}
	id := value[:dot]
	sig, err := base64.RawURLEncoding.DecodeString(value[dot+1:])
	if err != nil {
		return ""
	}
	expected := computeMAC(id, key)
	if !hmac.Equal(sig, expected) {
		return ""
	}
	return id
}

func computeMAC(data, key string) []byte {
	h := hmac.New(sha256.New, []byte(key))
	_, _ = fmt.Fprint(h, data)
	return h.Sum(nil)
}

func copyValues(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
