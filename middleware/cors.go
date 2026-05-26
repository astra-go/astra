package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/astra-go/astra"
)

// CORSConfig configures the CORS middleware.
type CORSConfig struct {
	// AllowOrigins is the list of allowed origins. Use "*" to allow all.
	AllowOrigins []string
	// AllowMethods is the list of allowed HTTP methods.
	AllowMethods []string
	// AllowHeaders is the list of allowed request headers.
	AllowHeaders []string
	// ExposeHeaders is the list of response headers accessible to JavaScript.
	ExposeHeaders []string
	// AllowCredentials indicates whether credentials are allowed.
	AllowCredentials bool
	// MaxAge is the max age for preflight cache in seconds.
	MaxAge int
}

// DefaultCORSConfig allows all origins — suitable for development only.
//
// WARNING: Do NOT use this configuration in production. AllowOrigins: ["*"]
// permits requests from any domain. If the application also sets cookies or
// uses session-based auth, a cross-origin attacker can trigger authenticated
// requests from a malicious page. Use CORSStrict(origins...) or
// CORSWithConfig with an explicit allowlist in production.
var DefaultCORSConfig = CORSConfig{
	AllowOrigins: []string{"*"},
	AllowMethods: []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions,
	},
	AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
	MaxAge:       86400,
}

// CORS returns a middleware that allows all cross-origin requests.
//
// WARNING: This uses DefaultCORSConfig (AllowOrigins: ["*"]) which is
// intended for local development only. For production, use:
//
//	middleware.CORSStrict("https://app.example.com", "https://admin.example.com")
//	// or
//	middleware.CORSWithConfig(middleware.CORSConfig{AllowOrigins: [...]})
func CORS() astra.HandlerFunc {
	return CORSWithConfig(DefaultCORSConfig)
}

// CORSWithConfig returns a CORS middleware with custom configuration.
//
// Panics at startup if AllowCredentials is true and AllowOrigins contains "*".
// That combination is rejected by all browsers (per CORS spec §3.2 step 7):
// a wildcard origin with credentials enabled silently breaks all credentialed
// cross-origin requests rather than causing a visible startup error.
func CORSWithConfig(cfg CORSConfig) astra.HandlerFunc {
	for _, o := range cfg.AllowOrigins {
		if o == "*" && cfg.AllowCredentials {
			panic(`middleware.CORSWithConfig: AllowCredentials: true is incompatible with AllowOrigins: ["*"] — ` +
				`browsers reject credentialed responses with a wildcard origin (CORS spec §3.2 step 7). ` +
				`Use CORSStrict(origins...) or CORSWithConfig with an explicit origin list instead.`)
		}
	}

	allowMethods := strings.Join(cfg.AllowMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	// Pre-allocate static header value slices once at middleware init time.
	// http.Header.Set allocates []string{value} on every call; direct map
	// assignment with these pre-built slices eliminates those per-request allocs.
	hAllowMethods := []string{allowMethods}
	hAllowHeaders := []string{allowHeaders}
	hCredentials := []string{"true"}
	var hExposeHeaders []string
	if exposeHeaders != "" {
		hExposeHeaders = []string{exposeHeaders}
	}
	var hMaxAge []string
	if cfg.MaxAge > 0 {
		hMaxAge = []string{maxAge}
	}

	return func(c *astra.Ctx) error {
		origin := c.Header("Origin")
		if origin == "" {
			return nil
		}

		// Check if origin is allowed
		allowed := false
		for _, o := range cfg.AllowOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}
		if !allowed {
			// Per RFC 6454 §7.2 and the WHATWG CORS spec, if the origin is not on
			// the allow-list, simply omit the CORS response headers — do NOT
			// return 403. Browsers will block cross-origin access automatically
			// because the headers are absent. Returning 403 exposes the fact that
			// a CORS policy exists and breaks non-browser clients unnecessarily.
			return nil
		}

		// Handle preflight
		if c.Request().Method == http.MethodOptions {
			h := c.Writer().Header()
			// Direct map assignment skips the textproto.CanonicalMIMEHeaderKey
			// call inside Header.Set and reuses the pre-allocated []string slices.
			h["Access-Control-Allow-Origin"] = []string{origin}
			h["Access-Control-Allow-Methods"] = hAllowMethods
			h["Access-Control-Allow-Headers"] = hAllowHeaders
			if cfg.AllowCredentials {
				h["Access-Control-Allow-Credentials"] = hCredentials
			}
			if hMaxAge != nil {
				h["Access-Control-Max-Age"] = hMaxAge
			}
			c.AbortWithStatus(http.StatusNoContent)
			return nil
		}

		// Set CORS headers for actual request
		h := c.Writer().Header()
		h["Access-Control-Allow-Origin"] = []string{origin}
		if hExposeHeaders != nil {
			h["Access-Control-Expose-Headers"] = hExposeHeaders
		}
		if cfg.AllowCredentials {
			h["Access-Control-Allow-Credentials"] = hCredentials
		}

		return nil
	}
}

// CORSStrict returns a production-ready CORS middleware restricted to the
// explicitly listed origins. Unlike CORS(), it panics at startup if:
//   - no origins are provided (would silently block all cross-origin requests)
//   - the wildcard "*" is included (use CORS() or CORSWithConfig for that)
//
// Example:
//
//	app.Use(middleware.CORSStrict(
//	    "https://app.example.com",
//	    "https://admin.example.com",
//	))
func CORSStrict(allowedOrigins ...string) astra.HandlerFunc {
	if len(allowedOrigins) == 0 {
		panic("middleware.CORSStrict: at least one allowed origin is required")
	}
	for _, o := range allowedOrigins {
		if o == "*" {
			panic(`middleware.CORSStrict: wildcard "*" is not permitted; use CORS() or CORSWithConfig for open-origin dev configs`)
		}
	}
	return CORSWithConfig(CORSConfig{
		AllowOrigins: allowedOrigins,
		AllowMethods: DefaultCORSConfig.AllowMethods,
		AllowHeaders: DefaultCORSConfig.AllowHeaders,
		MaxAge:       DefaultCORSConfig.MaxAge,
	})
}
