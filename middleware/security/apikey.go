// Package middleware provides HTTP middleware for the Astra web framework.
//
// APIKey extracts an API key from the request (Header or query parameter),
// passes it to a caller-supplied Validator, and rejects requests that fail
// validation with HTTP 401.
//
// # Usage
//
//	app.Use(middleware.APIKey(middleware.APIKeyConfig{
//	    Validator: func(ctx context.Context, key string) error {
//	        if !db.KeyExists(key) {
//	            return errors.New("invalid api key")
//	        }
//	        return nil
//	    },
//	}))
//
// # Custom header and query param
//
//	middleware.APIKey(middleware.APIKeyConfig{
//	    Header:     "X-App-Token",
//	    QueryParam: "token",
//	    Validator:  myValidator,
//	})
//
// # Storing the key in context for downstream handlers
//
//	Validator: func(ctx context.Context, key string) error {
//	    info, err := db.LookupKey(key)
//	    if err != nil { return err }
//	    // store extra info via context — retrieve with ctx.Value(...)
//	    return nil
//	},

package security

import (
	"context"
	"net/http"
	"strings"

	"github.com/astra-go/astra"
)

// APIKeyConfig configures the APIKey middleware.
type APIKeyConfig struct {
	// Validator is called with the extracted key.
	// Return nil to allow the request; return any error to reject it.
	// Required — middleware panics if nil.
	Validator func(ctx context.Context, key string) error

	// Header is the request header that carries the API key.
	// Default: "X-API-Key".
	Header string

	// QueryParam is the URL query parameter that carries the API key.
	// Default: "api_key".
	// The header is checked first; the query parameter is the fallback.
	QueryParam string

	// Skipper skips the middleware for requests where it returns true.
	// Nil means never skip.
	Skipper Skipper

	// ErrorHandler overrides the default 401 JSON error response.
	ErrorHandler ErrorHandler
}

// APIKey returns a middleware that validates an API key on every request.
func APIKey(cfg APIKeyConfig) astra.HandlerFunc {
	if cfg.Validator == nil {
		panic("middleware: APIKeyConfig.Validator must not be nil")
	}
	if cfg.Header == "" {
		cfg.Header = "X-API-Key"
	}
	if cfg.QueryParam == "" {
		cfg.QueryParam = "api_key"
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaultAPIKeyError
	}

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			c.Next()
			return nil
		}

		key := extractKey(c, cfg.Header, cfg.QueryParam)
		if key == "" {
			return cfg.ErrorHandler(c)
		}

		if err := cfg.Validator(c.Request().Context(), key); err != nil {
			return cfg.ErrorHandler(c)
		}

		c.Next()
		return nil
	}
}

// extractKey looks up the API key in the header first, then the query param.
// Bearer-token style headers ("Bearer <key>") are stripped automatically.
func extractKey(c *astra.Ctx, header, queryParam string) string {
	if v := c.Request().Header.Get(header); v != "" {
		return stripBearer(v)
	}
	return c.Query(queryParam)
}

func stripBearer(s string) string {
	if strings.HasPrefix(s, "Bearer ") {
		return strings.TrimPrefix(s, "Bearer ")
	}
	return s
}

func defaultAPIKeyError(c *astra.Ctx) error {
	return c.JSON(http.StatusUnauthorized, map[string]any{
		"code":    http.StatusUnauthorized,
		"message": "missing or invalid API key",
	})
}
