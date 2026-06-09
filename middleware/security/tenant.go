// Package middleware provides HTTP middleware for the Astra web framework.
//
// Tenant extracts a tenant identifier from the incoming request and stores it
// in the Astra context so downstream handlers and GORM scopes can read it
// without touching the HTTP layer.
//
// # Extraction order
//
// By default the middleware checks sources in this order and uses the first non-empty value:
//
//  1. Header (default: "X-Tenant-ID")
//  2. Query parameter (default: "tenant_id")
//  3. Path parameter (e.g. ":tenant" in the route pattern)
//
// Use the Sources bitmask to restrict which sources are consulted.
//
// If none of the above yield a value the request is either passed through
// (Required=false) or rejected with HTTP 400 (Required=true).
//
// # Usage
//
//	app.Use(middleware.Tenant())
//
//	// Handler
//	func getOrders(c *astra.Ctx) error {
//	    tid := middleware.TenantID(c)   // "" when absent
//	    db.Scopes(orm.GORMTenantScope(tid)).Find(&orders)
//	    ...
//	}
package security

import (
	"context"
	"fmt"
	"net/http"

	"github.com/astra-go/astra"
)

const defaultTenantContextKey = "tenant_id"

// TenantSource is a bitmask that controls which request locations are checked
// when extracting the tenant ID.
type TenantSource uint8

const (
	TenantSrcHeader TenantSource = 1 << iota // read from request header
	TenantSrcQuery                            // read from URL query parameter
	TenantSrcPath                             // read from URL path parameter

	// tenantSrcAll is the default: all three sources are checked in order.
	tenantSrcAll TenantSource = TenantSrcHeader | TenantSrcQuery | TenantSrcPath
)

// TenantConfig configures the Tenant middleware.
type TenantConfig struct {
	// Header is the request header to read the tenant ID from.
	// Default: "X-Tenant-ID".
	Header string

	// QueryParam is the URL query parameter to read the tenant ID from.
	// Default: "tenant_id".
	QueryParam string

	// PathParam is the route path parameter to read the tenant ID from.
	// Default: "" (disabled).
	// Example: set to "tenant" for routes like "/api/:tenant/users".
	PathParam string

	// Sources is a bitmask of TenantSource values that controls which request
	// locations are consulted. Zero value means all sources (header, query, path).
	Sources TenantSource

	// ContextKey is the key under which the tenant ID is stored in the Astra
	// context (via c.Set). Default: "tenant_id".
	ContextKey string

	// Required rejects requests that carry no tenant ID with HTTP 400 when true.
	// Default: false (missing tenant is silently accepted).
	Required bool

	// Validator is an optional function that validates the extracted tenant ID.
	// Return a non-nil error to reject the request; the error message is
	// included in the 400 response.
	// When nil and Required is true, a default validator is applied that
	// rejects empty strings and values containing characters other than
	// alphanumeric, hyphens, and underscores. This prevents injection
	// attacks when the tenant ID is used in database queries.
	Validator func(ctx context.Context, tenantID string) error

	// Skipper skips tenant extraction for matching requests.
	Skipper Skipper

	// ErrorHandler overrides the default HTTP 400 JSON response.
	ErrorHandler ErrorHandler
}

// Tenant returns a middleware that extracts the tenant ID and stores it in ctx.
// The tenant ID is always required; use TenantOptional for optional extraction.
func Tenant(cfgs ...TenantConfig) astra.HandlerFunc {
	cfg := TenantConfig{Required: true}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	cfg.Required = true
	return buildTenantMiddleware(cfg)
}

// TenantOptional returns a Tenant middleware where a missing tenant ID is
// silently accepted (Required=false). Equivalent to Tenant(TenantConfig{Required: false}).
func TenantOptional(cfgs ...TenantConfig) astra.HandlerFunc {
	cfg := TenantConfig{Required: false}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
		cfg.Required = false
	}
	return buildTenantMiddleware(cfg)
}

// TenantFromContext returns a middleware that reads the tenant ID from an
// existing context key (set by a prior middleware such as JWT claims) rather
// than from the HTTP request. The tenant ID is re-stored under the default
// "tenant_id" key so TenantID() works as usual.
//
// If the key is absent or empty and Required is true (the default), the
// request is rejected with HTTP 400.
func TenantFromContext(key string) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		tid := c.GetString(key)
		if tid == "" {
			return defaultTenantError(c)
		}
		c.Set(defaultTenantContextKey, tid)
		c.Next()
		return nil
	}
}

func buildTenantMiddleware(cfg TenantConfig) astra.HandlerFunc {
	if cfg.Header == "" {
		cfg.Header = "X-Tenant-ID"
	}
	if cfg.QueryParam == "" {
		cfg.QueryParam = "tenant_id"
	}
	if cfg.ContextKey == "" {
		cfg.ContextKey = defaultTenantContextKey
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaultTenantError
	}
	if cfg.Sources == 0 {
		cfg.Sources = tenantSrcAll
	}

	// Apply a default validator when Required is true and none is provided.
	// This prevents injection attacks (e.g. SQL injection via tenant ID)
	// when the tenant ID is used in database queries like GORM scopes.
	if cfg.Required && cfg.Validator == nil {
		cfg.Validator = defaultTenantValidator
	}

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			c.Next()
			return nil
		}

		tid := ""

		// 1. Header
		if tid == "" && cfg.Sources&TenantSrcHeader != 0 {
			if v := c.Request().Header.Get(cfg.Header); v != "" {
				tid = v
			}
		}
		// 2. Query param
		if tid == "" && cfg.Sources&TenantSrcQuery != 0 {
			if v := c.Query(cfg.QueryParam); v != "" {
				tid = v
			}
		}
		// 3. Path param
		if tid == "" && cfg.Sources&TenantSrcPath != 0 && cfg.PathParam != "" {
			if v := c.Param(cfg.PathParam); v != "" {
				tid = v
			}
		}

		if tid == "" && cfg.Required {
			return cfg.ErrorHandler(c)
		}

		if tid != "" && cfg.Validator != nil {
			if err := cfg.Validator(c.Request().Context(), tid); err != nil {
				return c.JSON(http.StatusBadRequest, map[string]any{
					"code":    http.StatusBadRequest,
					"message": "invalid tenant: " + err.Error(),
				})
			}
		}

		c.Set(cfg.ContextKey, tid)
		c.Next()
		return nil
	}
}

// TenantID retrieves the tenant ID stored in the Astra context by Tenant middleware.
// Returns an empty string when the middleware was not applied or no tenant was found.
func TenantID(c *astra.Ctx) string {
	v, ok := c.Get(defaultTenantContextKey)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// TenantIDFromKey retrieves the tenant ID using a custom context key.
func TenantIDFromKey(c *astra.Ctx, key string) string {
	v, ok := c.Get(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func defaultTenantError(c *astra.Ctx) error {
	return c.JSON(http.StatusBadRequest, map[string]any{
		"code":    http.StatusBadRequest,
		"message": "missing required tenant ID",
	})
}

// defaultTenantValidator rejects tenant IDs that are empty or contain
// characters outside [a-zA-Z0-9_-]. This prevents injection when tenant IDs
// are interpolated into database queries (e.g. GORM scopes).
func defaultTenantValidator(_ context.Context, tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("tenant ID must not be empty")
	}
	for _, r := range tenantID {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("tenant ID contains invalid character %q; only alphanumeric, hyphen, and underscore are allowed", r)
		}
	}
	return nil
}
