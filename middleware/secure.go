// Package middleware provides HTTP middleware for the Astra web framework.
//
// Secure sets a suite of security-related HTTP response headers on every
// response to harden the application against common web vulnerabilities:
//
//   - Strict-Transport-Security (HSTS)   — force HTTPS
//   - X-Frame-Options                    — clickjacking protection
//   - X-Content-Type-Options             — MIME sniffing protection
//   - Referrer-Policy                    — referrer information control
//   - Content-Security-Policy (CSP)      — XSS / injection mitigation
//   - Permissions-Policy                 — browser feature access control
//
// # Usage
//
//	app.Use(middleware.SecureHeaders())
//
// # Custom config
//
//	app.Use(middleware.SecureHeaders(middleware.SecureConfig{
//	    HSTSMaxAge:         31536000,
//	    FrameOption:        middleware.FrameDeny,
//	    ContentTypeNosniff: true,
//	    ReferrerPolicy:     "strict-origin-when-cross-origin",
//	    CSP:                "default-src 'self'",
//	}))

package middleware

import (
	"fmt"
	"strings"

	"github.com/astra-go/astra"
)

// FrameOption is the value of the X-Frame-Options header.
type FrameOption string

const (
	// FrameDeny prohibits the page from being displayed in a frame.
	FrameDeny FrameOption = "DENY"
	// FrameSameOrigin allows framing only from the same origin.
	FrameSameOrigin FrameOption = "SAMEORIGIN"
)

// SecureConfig configures the SecureHeaders middleware.
// All fields are optional; zero values disable the corresponding header.
type SecureConfig struct {
	// HSTSMaxAge sets the max-age directive (in seconds) of the
	// Strict-Transport-Security header. 0 disables the header.
	// Recommended: 31536000 (1 year).
	HSTSMaxAge int

	// HSTSIncludeSubdomains appends "includeSubDomains" to the HSTS header.
	HSTSIncludeSubdomains bool

	// HSTSPreload appends "preload" to the HSTS header.
	HSTSPreload bool

	// FrameOption sets the X-Frame-Options header.
	// Use FrameDeny or FrameSameOrigin. Empty string disables the header.
	FrameOption FrameOption

	// ContentTypeNosniff sets "X-Content-Type-Options: nosniff" when true.
	ContentTypeNosniff bool

	// ReferrerPolicy sets the Referrer-Policy header.
	// Example: "strict-origin-when-cross-origin"
	ReferrerPolicy string

	// CSP sets the Content-Security-Policy header value.
	// Example: "default-src 'self'; img-src *"
	CSP string

	// PermissionsPolicy sets the Permissions-Policy header value.
	// Example: "geolocation=(), microphone=()"
	PermissionsPolicy string

	// CrossOriginOpenerPolicy sets the Cross-Origin-Opener-Policy header.
	// Example: "same-origin". Empty string disables the header.
	CrossOriginOpenerPolicy string
}

// defaultSecureConfig is an opinionated secure-by-default configuration.
var defaultSecureConfig = SecureConfig{
	HSTSMaxAge:              31536000, // 1 year
	HSTSIncludeSubdomains:   true,
	FrameOption:             FrameDeny,
	ContentTypeNosniff:      true,
	ReferrerPolicy:          "strict-origin-when-cross-origin",
	PermissionsPolicy:       "camera=(), microphone=(), geolocation=()",
	CrossOriginOpenerPolicy: "same-origin",
}

// SecureHeaders returns a middleware that sets security response headers.
// Call with no arguments to use the opinionated default configuration.
func SecureHeaders(cfgs ...SecureConfig) astra.HandlerFunc {
	cfg := defaultSecureConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	// Pre-build header values so they are not recomputed per request.
	hsts := buildHSTS(cfg)

	return func(c *astra.Ctx) error {
		h := c.Writer().Header()

		if hsts != "" {
			h.Set("Strict-Transport-Security", hsts)
		}
		if cfg.FrameOption != "" {
			h.Set("X-Frame-Options", string(cfg.FrameOption))
		}
		if cfg.ContentTypeNosniff {
			h.Set("X-Content-Type-Options", "nosniff")
		}
		if cfg.ReferrerPolicy != "" {
			h.Set("Referrer-Policy", cfg.ReferrerPolicy)
		}
		if cfg.CSP != "" {
			h.Set("Content-Security-Policy", cfg.CSP)
		}
		if cfg.PermissionsPolicy != "" {
			h.Set("Permissions-Policy", cfg.PermissionsPolicy)
		}
		if cfg.CrossOriginOpenerPolicy != "" {
			h.Set("Cross-Origin-Opener-Policy", cfg.CrossOriginOpenerPolicy)
		}

		c.Next()
		return nil
	}
}

func buildHSTS(cfg SecureConfig) string {
	if cfg.HSTSMaxAge <= 0 {
		return ""
	}
	parts := []string{fmt.Sprintf("max-age=%d", cfg.HSTSMaxAge)}
	if cfg.HSTSIncludeSubdomains {
		parts = append(parts, "includeSubDomains")
	}
	if cfg.HSTSPreload {
		parts = append(parts, "preload")
	}
	return strings.Join(parts, "; ")
}
