// Package middleware — CSRF protection middleware.
//
// Implements the Double-Submit Cookie pattern:
//  1. On the first request (or when the cookie is absent) a random CSRF token
//     is generated and set as a cookie.
//  2. On mutating requests (POST / PUT / PATCH / DELETE) the middleware
//     compares the cookie value with the token submitted in the request
//     (header, form field, or query param).  Mismatch → 403 Forbidden.
//
// # Usage
//
//	app.Use(middleware.CSRF("32-byte-secret-key-goes-here!!!"))
//
//	// Read the token in a handler (inject it into your HTML form / SPA):
//	app.GET("/form", func(c *contract.Context) error {
//	    token := middleware.GetCSRFToken(c)
//	    return c.HTML(200, `<form><input name="_csrf" value="`+token+`"></form>`)
//	})
//
// # Safe methods
//
// GET, HEAD, OPTIONS, and TRACE are skipped; only mutating verbs are
// validated.
//
// # Token lookup order (configurable via TokenLookup)
//
// Default: "header:X-CSRF-Token", then "form:_csrf".
package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/astra-go/astra"
)

// CSRFConfig configures the CSRF middleware.
type CSRFConfig struct {
	// Secret is used to HMAC-sign the token so it cannot be forged.
	// Must be at least 16 bytes.
	Secret []byte

	// CookieName is the name of the CSRF cookie. Default: "_csrf".
	CookieName string

	// CookiePath is the cookie Path attribute. Default: "/".
	CookiePath string

	// CookieDomain is the cookie Domain attribute. Leave empty to match the request host.
	CookieDomain string

	// CookieMaxAge is the Max-Age of the CSRF cookie. Default: 24h.
	CookieMaxAge time.Duration

	// CookieSecure sets the Secure flag on the cookie. Default: true.
	// Must be false for local HTTP development testing.
	CookieSecure bool

	// CookieHTTPOnly sets the HttpOnly flag. Default: true.
	// Set to false for SPA apps that need JavaScript access to the token.
	CookieHTTPOnly bool

	// CookieSameSite sets the SameSite attribute. Default: http.SameSiteLaxMode.
	CookieSameSite http.SameSite

	// ContextKey is the context key for storing the token. Default: "csrf_token".
	ContextKey string

	// TokenLookup defines where to find the submitted token.
	// Format: "source:name[,source:name]..."
	//   source: "header", "form", "query"
	// Default: "header:X-CSRF-Token,form:_csrf"
	TokenLookup string

	// SkipFunc allows skipping CSRF validation for certain requests.
	SkipFunc func(*astra.Ctx) bool
}

// DefaultCSRFConfig is the default CSRF configuration.
var DefaultCSRFConfig = CSRFConfig{
	CookieName:     "_csrf",
	CookiePath:     "/",
	CookieMaxAge:   24 * time.Hour,
	CookieSecure:   true,
	CookieHTTPOnly: true, // Prevents JavaScript access, mitigating XSS-based token theft
	CookieSameSite: http.SameSiteLaxMode,
	ContextKey:     "csrf_token",
	TokenLookup:    "header:X-CSRF-Token,form:_csrf",
}

// csrfTokenLen is the number of random bytes in a token.
const csrfTokenLen = 32

// CSRF returns a CSRF protection middleware using the Double-Submit Cookie
// pattern with HMAC-SHA256 signing.
func CSRF(secret string) astra.HandlerFunc {
	cfg := DefaultCSRFConfig
	cfg.Secret = []byte(secret)
	return CSRFWithConfig(cfg)
}

// CSRFWithConfig returns a CSRF middleware with custom configuration.
func CSRFWithConfig(cfg CSRFConfig) astra.HandlerFunc {
	// Apply defaults for zero-value fields
	if cfg.CookieName == "" {
		cfg.CookieName = DefaultCSRFConfig.CookieName
	}
	if cfg.CookiePath == "" {
		cfg.CookiePath = "/"
	}
	if cfg.CookieMaxAge == 0 {
		cfg.CookieMaxAge = DefaultCSRFConfig.CookieMaxAge
	}
	if cfg.CookieSameSite == 0 {
		cfg.CookieSameSite = http.SameSiteLaxMode
	}
	if cfg.ContextKey == "" {
		cfg.ContextKey = DefaultCSRFConfig.ContextKey
	}
	if cfg.TokenLookup == "" {
		cfg.TokenLookup = DefaultCSRFConfig.TokenLookup
	}

	// Parse token lookups once at startup.
	type lookup struct{ source, name string }
	var lookups []lookup
	for _, part := range strings.Split(cfg.TokenLookup, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) == 2 {
			lookups = append(lookups, lookup{kv[0], kv[1]})
		}
	}

	safeMethods := map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodOptions: true,
		http.MethodTrace:   true,
	}

	return func(c *astra.Ctx) error {
		if cfg.SkipFunc != nil && cfg.SkipFunc(c) {
			return nil
		}

		// Read or generate the token stored in the cookie.
		cookieToken := ""
		if cookie, err := c.Request().Cookie(cfg.CookieName); err == nil {
			cookieToken = cookie.Value
		}
		if !isValidCSRFToken(cookieToken, cfg.Secret) {
			cookieToken = generateCSRFToken(cfg.Secret)
			setCSRFCookie(c, cfg, cookieToken)
		}

		// Store the token in context so handlers can embed it in responses.
		c.Set(cfg.ContextKey, cookieToken)

		// Safe methods skip the token comparison.
		if safeMethods[c.Request().Method] {
			c.Next()
			return nil
		}

		// Mutating request — compare submitted token with cookie.
		submitted := ""
		for _, lk := range lookups {
			submitted = extractCSRFToken(c, lk.source, lk.name)
			if submitted != "" {
				break
			}
		}

		if !csrfTokensEqual(cookieToken, submitted, cfg.Secret) {
			return astra.NewHTTPError(http.StatusForbidden, "CSRF token mismatch")
		}

		c.Next()
		return nil
	}
}

// GetCSRFToken returns the CSRF token stored in the context for the current
// request. Use this to embed the token in HTML forms or JSON API responses.
func GetCSRFToken(c *astra.Ctx) string {
	token, _ := c.Get("csrf_token")
	s, _ := token.(string)
	return s
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// generateCSRFToken creates a new random HMAC-signed CSRF token.
// Format: base64(random_bytes) + "." + base64(HMAC(random_bytes, secret))
func generateCSRFToken(secret []byte) string {
	raw := make([]byte, csrfTokenLen)
	if _, err := rand.Read(raw); err != nil {
		panic("csrf: crypto/rand failed: " + err.Error())
	}
	b64 := base64.RawURLEncoding.EncodeToString(raw)
	return b64 + "." + sign(b64, secret)
}

func sign(value string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// isValidCSRFToken reports whether token is structurally valid and has a valid HMAC.
// It must have two non-empty parts separated by ".", and the second part
// must be a valid base64-encoded HMAC-SHA256 of the first part.
func isValidCSRFToken(token string, secret []byte) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	// Verify the HMAC signature.
	if _, err := base64.RawURLEncoding.DecodeString(parts[0]); err != nil {
		return false
	}
	expected := sign(parts[0], secret)
	return hmac.Equal([]byte(expected), []byte(parts[1]))
}

// csrfTokensEqual performs a constant-time comparison of the cookie token with
// the submitted token, also verifying the HMAC signature on both sides.
func csrfTokensEqual(cookie, submitted string, secret []byte) bool {
	if !isValidCSRFToken(cookie, secret) || !isValidCSRFToken(submitted, secret) {
		return false
	}
	// Both HMACs verified by isValidCSRFToken above.
	// Compare the random value (first part) of both tokens.
	cookieParts := strings.SplitN(cookie, ".", 2)
	parts := strings.SplitN(submitted, ".", 2)
	return hmac.Equal([]byte(cookieParts[0]), []byte(parts[0]))
}

func setCSRFCookie(c *astra.Ctx, cfg CSRFConfig, token string) {
	http.SetCookie(c.Writer(), &http.Cookie{
		Name:     cfg.CookieName,
		Value:    token,
		Path:     cfg.CookiePath,
		Domain:   cfg.CookieDomain,
		MaxAge:   int(cfg.CookieMaxAge.Seconds()),
		Secure:   cfg.CookieSecure,
		HttpOnly: cfg.CookieHTTPOnly,
		SameSite: cfg.CookieSameSite,
	})
}

func extractCSRFToken(c *astra.Ctx, source, name string) string {
	switch source {
	case "header":
		return c.Header(name)
	case "form":
		return c.PostForm(name)
	case "query":
		return c.Query(name)
	}
	return ""
}
