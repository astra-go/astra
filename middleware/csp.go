// Package middleware — Content-Security-Policy (CSP) middleware.
//
// Sets the Content-Security-Policy (and optionally Content-Security-Policy-Report-Only)
// response header. Supports per-request nonce injection for 'nonce-...' source expressions.
//
// # Static CSP
//
//	app.Use(middleware.CSP(middleware.CSPConfig{
//	    Policy: "default-src 'self'; img-src 'self' data:; script-src 'self'",
//	}))
//
// # Nonce injection
//
// When NonceFunc is set, a random nonce is generated per request, stored in the
// Context under the key "csp_nonce", and the literal string "{nonce}" in the
// policy is replaced with the generated value.
//
//	app.Use(middleware.CSP(middleware.CSPConfig{
//	    Policy:    "default-src 'self'; script-src 'nonce-{nonce}'",
//	    NonceFunc: middleware.RandomNonce,
//	}))
//
//	// In a template handler:
//	func page(c *contract.Context) error {
//	    nonce := middleware.CSPNonce(c) // "" if nonce not configured
//	    return c.HTML(200, `<script nonce="`+nonce+`">…</script>`)
//	}
//
// # Report-only mode
//
//	app.Use(middleware.CSP(middleware.CSPConfig{
//	    Policy:     "default-src 'self'",
//	    ReportOnly: true,
//	    ReportURI:  "https://csp.example.com/report",
//	}))
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/astra-go/astra"
)

const cspNonceKey = "csp_nonce"

// CSPConfig configures the CSP middleware.
type CSPConfig struct {
	// Policy is the raw CSP directive string.
	// Use "{nonce}" as a placeholder that will be replaced with the generated nonce.
	// Example: "default-src 'self'; script-src 'nonce-{nonce}'"
	Policy string

	// ReportOnly sends the policy as Content-Security-Policy-Report-Only instead of
	// enforcing it. Violations are reported but not blocked.
	ReportOnly bool

	// ReportURI is appended as "; report-uri <uri>" when non-empty.
	ReportURI string

	// NonceFunc generates a random nonce per request.
	// When nil, no nonce is generated and "{nonce}" placeholders are left as-is.
	// Use middleware.RandomNonce for the built-in 128-bit base64url nonce generator.
	NonceFunc func() (string, error)

	// Skipper skips the middleware for matching requests.
	Skipper func(c *astra.Ctx) bool
}

// RandomNonce generates a cryptographically random 128-bit (16-byte) base64url nonce.
func RandomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CSP returns a Content-Security-Policy middleware.
func CSP(cfg CSPConfig) astra.HandlerFunc {
	headerName := "Content-Security-Policy"
	if cfg.ReportOnly {
		headerName = "Content-Security-Policy-Report-Only"
	}

	return func(c *astra.Ctx) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			c.Next()
			return nil
		}

		policy := cfg.Policy

		// Inject nonce when NonceFunc is provided.
		if cfg.NonceFunc != nil {
			nonce, err := cfg.NonceFunc()
			if err == nil && nonce != "" {
				c.Set(cspNonceKey, nonce)
				policy = strings.ReplaceAll(policy, "{nonce}", nonce)
			}
		}

		// Append report-uri directive.
		if cfg.ReportURI != "" {
			if policy != "" {
				policy += "; "
			}
			policy += "report-uri " + cfg.ReportURI
		}

		if policy != "" {
			c.Writer().Header().Set(headerName, policy)
		}

		c.Next()
		return nil
	}
}

// CSPNonce returns the nonce generated for this request by the CSP middleware.
// Returns "" if the CSP middleware was not configured with a NonceFunc.
func CSPNonce(c *astra.Ctx) string {
	v, _ := c.Get(cspNonceKey)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
