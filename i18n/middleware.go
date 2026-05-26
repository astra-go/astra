package i18n

import (
	"github.com/astra-go/astra"
)

// MiddlewareConfig controls how the i18n middleware detects the request locale.
type MiddlewareConfig struct {
	// Bundle to use.  Defaults to the package-level Default bundle.
	Bundle *Bundle

	// QueryParam is the URL query parameter name used for explicit locale
	// selection (e.g. "?lang=zh").  Default: "lang".
	QueryParam string

	// Header is the request header name checked before Accept-Language.
	// Default: "X-Language".
	Header string
}

// Middleware returns an Astra middleware that detects the request locale and
// stores a *Translator in the context under the key "i18n.translator".
//
// Handlers retrieve the translation via i18n.T(c, key, args...) or by calling
// i18n.GetTranslator(c).T(key, args...) for repeated lookups in the same request.
//
// Detection priority:
//  1. URL query param (default key: "lang")
//  2. Request header (default: "X-Language")
//  3. Accept-Language header (quality-sorted, with base-language fallback)
//  4. Bundle's fallback locale
//
// Usage (global):
//
//	app.Use(i18n.Middleware())
//
// Usage (custom bundle):
//
//	bundle := i18n.NewDefault().
//	    Register("ja", jaMessages).
//	    SetFallback("ja")
//	app.Use(i18n.Middleware(i18n.MiddlewareConfig{Bundle: bundle}))
func Middleware(cfgs ...MiddlewareConfig) astra.MiddlewareFunc {
	cfg := MiddlewareConfig{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	if cfg.Bundle == nil {
		cfg.Bundle = GetDefault()
	}
	if cfg.QueryParam == "" {
		cfg.QueryParam = "lang"
	}
	if cfg.Header == "" {
		cfg.Header = "X-Language"
	}
	b := cfg.Bundle
	qp := cfg.QueryParam
	hdr := cfg.Header

	return func(c *astra.Ctx) error {
		locale := DetectLocale(
			b,
			c.Query(qp),
			c.Header(hdr),
			c.Header("Accept-Language"),
		)
		c.Set(contextKey, b.Translator(locale))
		c.Next()
		return nil
	}
}

// GetTranslator retrieves the *Translator stored in the context by Middleware.
// Returns the Default bundle's fallback Translator if Middleware was not mounted.
func GetTranslator(c *astra.Ctx) *Translator {
	if v, ok := c.Get(contextKey); ok {
		if tr, ok := v.(*Translator); ok {
			return tr
		}
	}
	b := GetDefault()
	return b.Translator(b.fallback)
}
