package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/astra-go/astra"
)

// CORSConfig configures the CORS middleware.
type CORSConfig struct {
	// AllowOrigins is the list of allowed origins.
	// WARNING: Do NOT use "*" in production. Use CORSStrict() or CORS() instead.
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
	// LogWarnings controls whether to log warnings about insecure CORS config.
	// Default: true (logs warnings if "*" is used or if CORS is misconfigured).
	LogWarnings bool
}

// DefaultCORSConfig is a RESTRICTIVE default config that DENIES all cross-origin requests.
//
// ⚠️ SECURITY CHANGE (v4.1): Previously this default allowed all origins ("*").
// That was INSECURE and could lead to CSRF attacks in production.
//
// To allow cross-origin requests, use:
//   - middleware.CORS("https://app.example.com", "https://admin.example.com")
//   - middleware.CORSStrict("https://app.example.com")
//   - middleware.CORSWithConfig(middleware.CORSConfig{AllowOrigins: [...]})
//
// For local development ONLY, use middleware.CORSPermissive().
var DefaultCORSConfig = CORSConfig{
	AllowOrigins: []string{}, // ⚠️ SECURITY: Empty by default (deny all)
	AllowMethods: []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions,
	},
	AllowHeaders:  []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
	MaxAge:        86400,
	LogWarnings:    true,
}

// CORS returns a production-ready CORS middleware restricted to the explicitly
// listed origins. It panics at startup if no origins are provided or if the
// wildcard "*" is included — use CORSPermissive() for open-origin dev configs.
//
// Example:
//
//	app.Use(middleware.CORS(
//	    "https://app.example.com",
//	    "https://admin.example.com",
//	))
func CORS(origins ...string) astra.HandlerFunc {
	return CORSStrict(origins...)
}

// CORSPermissive returns a middleware that allows all cross-origin requests.
//
// ⚠️ WARNING: This uses AllowOrigins: ["*"] which is INSECURE for production.
// Only use this for local development.
//
// For production, use:
//
//	middleware.CORS("https://app.example.com", "https://admin.example.com")
//	middleware.CORSStrict("https://app.example.com")
//	middleware.CORSWithConfig(middleware.CORSConfig{AllowOrigins: [...]})
func CORSPermissive() astra.HandlerFunc {
	if DefaultCORSConfig.LogWarnings {
		slog.Warn("CORS: Using permissive config (AllowOrigins: [*]). This is INSECURE and should NOT be used in production.")
	}
	return CORSWithConfig(CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: DefaultCORSConfig.AllowMethods,
		AllowHeaders: DefaultCORSConfig.AllowHeaders,
		MaxAge:       DefaultCORSConfig.MaxAge,
	})
}

// CORSWithConfig returns a CORS middleware with custom configuration.
//
// Panics at startup if AllowCredentials is true and AllowOrigins contains "*".
// That combination is rejected by all browsers (per CORS spec §3.2 step 7):
// a wildcard origin with credentials enabled silently breaks all credentialed
// cross-origin requests rather than causing a visible startup error.
func CORSWithConfig(cfg CORSConfig) astra.HandlerFunc {
	// Validate config at startup
	for _, o := range cfg.AllowOrigins {
		if o == "*" && cfg.AllowCredentials {
			panic(`middleware.CORSWithConfig: AllowCredentials: true is incompatible with AllowOrigins: ["*"] — ` +
				`browsers reject credentialed responses with a wildcard origin (CORS spec §3.2 step 7). ` +
				`Use CORSStrict(origins...) or CORSWithConfig with an explicit origin list instead.`)
		}
	}

	// Log warning if wildcard is used
	if cfg.LogWarnings {
		for _, o := range cfg.AllowOrigins {
			if o == "*" {
				slog.Warn("CORS: Wildcard origin (*) detected. This allows ALL domains to access your API. " +
					"Use CORSStrict() or CORSWithConfig with explicit origins for production.")
			}
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
			// No Origin header means this is either a same-origin request
			// (browser doesn't send Origin for same-origin) or a non-browser
			// client. CORS headers are not needed for same-origin requests.
			// For non-browser clients, CORS is irrelevant since they don't
			// enforce the same-origin policy.
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
			if cfg.LogWarnings {
				slog.Debug("CORS: Origin not allowed", "origin", origin)
			}
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
// explicitly listed origins. Unlike CORSPermissive(), it panics at startup if:
//   - no origins are provided (would silently block all cross-origin requests)
//   - the wildcard "*" is included (use CORSPermissive() or CORSWithConfig for that)
//
// Note: CORS(origins...) is an alias for this function.
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
			panic(`middleware.CORSStrict: wildcard "*" is not permitted; use CORSPermissive() or CORSWithConfig for open-origin dev configs`)
		}
	}
	return CORSWithConfig(CORSConfig{
		AllowOrigins: allowedOrigins,
		AllowMethods:  DefaultCORSConfig.AllowMethods,
		AllowHeaders:  DefaultCORSConfig.AllowHeaders,
		MaxAge:        DefaultCORSConfig.MaxAge,
		LogWarnings:   DefaultCORSConfig.LogWarnings,
	})
}

// CORSProduction is a helper function that returns a CORS middleware configured
// for production use. It reads allowed origins from environment variable
// CORS_ALLOWED_ORIGINS (comma-separated list).
//
// Example:
//
//	# .env or environment
//	CORS_ALLOWED_ORIGINS=https://app.example.com,https://admin.example.com
//
//	// main.go
//	app.Use(middleware.CORSProduction())
func CORSProduction() astra.HandlerFunc {
	originsEnv := os.Getenv("CORS_ALLOWED_ORIGINS")
	if originsEnv == "" {
		panic("middleware.CORSProduction: CORS_ALLOWED_ORIGINS environment variable is required in production")
	}

	origins := strings.Split(originsEnv, ",")
	for i, o := range origins {
		origins[i] = strings.TrimSpace(o)
	}

	// Validate origins
	if len(origins) == 0 {
		panic("middleware.CORSProduction: at least one allowed origin is required")
	}
	for _, o := range origins {
		if o == "" {
			panic("middleware.CORSProduction: empty origin in CORS_ALLOWED_ORIGINS")
		}
		if o == "*" {
			panic(`middleware.CORSProduction: wildcard "*" is not allowed in production. Use explicit origins.`)
		}
	}

	slog.Info("CORS: Production config loaded", "origins", origins)

	return CORSStrict(origins...)
}
