// Package rbac provides a Casbin-based RBAC authorization middleware for Astra.
//
// The middleware intercepts each request, extracts the subject (user/role),
// object (resource), and action (HTTP method or custom), then enforces the
// loaded Casbin policy. Requests that fail enforcement are rejected with
// HTTP 403 Forbidden.
//
// # Quick start — file-based policy
//
//	import (
//	    "github.com/casbin/casbin/v2"
//	    "github.com/astra-go/astra/auth/rbac"
//	)
//
//	e, _ := casbin.NewEnforcer("model.conf", "policy.csv")
//	app.Use(rbac.Middleware(rbac.Config{Enforcer: e}))
//
// # Inline policy (testing)
//
//	m, _ := model.NewModelFromString(`
//	    [request_definition]
//	    r = sub, obj, act
//	    [policy_definition]
//	    p = sub, obj, act
//	    [role_definition]
//	    g = _, _
//	    [policy_effect]
//	    e = some(where (p.eft == allow))
//	    [matchers]
//	    m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && regexMatch(r.act, p.act)
//	`)
//	adapter := fileadapter.NewAdapter("policy.csv")
//	e, _ := casbin.NewEnforcer(m, adapter)
//
// # Custom subject extractor (e.g. from JWT claims set by upstream middleware)
//
//	app.Use(rbac.Middleware(rbac.Config{
//	    Enforcer: e,
//	    GetSubject: func(c *astra.Ctx) string {
//	        uid, _ := c.Get("user_id")
//	        return fmt.Sprintf("%v", uid)
//	    },
//	}))
package rbac

import (
	"fmt"
	"net/http"

	"github.com/casbin/casbin/v2"

	"github.com/astra-go/astra"
)

// Config configures the RBAC middleware.
type Config struct {
	// Enforcer is the Casbin policy enforcer. Required.
	// Use casbin.NewEnforcer or casbin.NewSyncedEnforcer for thread safety.
	Enforcer *casbin.Enforcer

	// GetSubject extracts the policy subject (user, role, client ID, …) from
	// the request context. The middleware is called after authentication
	// middleware has run, so values set via c.Set() are available here.
	// Default: reads the context key "user_id" set by JWT middleware.
	GetSubject func(c *astra.Ctx) string

	// GetObject extracts the policy object (resource path).
	// Default: c.Request().URL.Path.
	GetObject func(c *astra.Ctx) string

	// GetAction extracts the policy action.
	// Default: c.Request().Method (e.g. "GET", "POST").
	GetAction func(c *astra.Ctx) string

	// Skipper returns true for requests that bypass RBAC enforcement.
	// Useful for public routes or health-check endpoints.
	Skipper func(c *astra.Ctx) bool

	// ErrorHandler overrides the default HTTP 403 JSON response.
	ErrorHandler astra.HandlerFunc
}

// Middleware returns a middleware that enforces RBAC policies on every request.
// It panics if Config.Enforcer is nil.
func Middleware(cfg Config) astra.MiddlewareFunc {
	if cfg.Enforcer == nil {
		panic("rbac: Config.Enforcer must not be nil")
	}
	if cfg.GetSubject == nil {
		cfg.GetSubject = defaultGetSubject
	}
	if cfg.GetObject == nil {
		cfg.GetObject = func(c *astra.Ctx) string { return c.Request().URL.Path }
	}
	if cfg.GetAction == nil {
		cfg.GetAction = func(c *astra.Ctx) string { return c.Request().Method }
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaultRBACError
	}

	return func(c *astra.Ctx) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			c.Next()
			return nil
		}

		sub := cfg.GetSubject(c)
		obj := cfg.GetObject(c)
		act := cfg.GetAction(c)

		allowed, err := cfg.Enforcer.Enforce(sub, obj, act)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, astra.Map{
				"code":    http.StatusInternalServerError,
				"message": "rbac: policy evaluation error",
			})
		}
		if !allowed {
			return cfg.ErrorHandler(c)
		}

		c.Next()
		return nil
	}
}

// HasPermission checks whether sub can perform act on obj using the enforcer.
// This is a helper for programmatic permission checks outside of middleware.
func HasPermission(e *casbin.Enforcer, sub, obj, act string) (bool, error) {
	return e.Enforce(sub, obj, act)
}

// AddRoleForUser assigns role to user in the enforcer's role manager.
func AddRoleForUser(e *casbin.Enforcer, user, role string) error {
	_, err := e.AddRoleForUser(user, role)
	return err
}

// RemoveRoleForUser removes role from user.
func RemoveRoleForUser(e *casbin.Enforcer, user, role string) error {
	_, err := e.DeleteRoleForUser(user, role)
	return err
}

// RolesForUser returns all roles assigned to user.
func RolesForUser(e *casbin.Enforcer, user string) ([]string, error) {
	return e.GetRolesForUser(user)
}

// ─── defaults ─────────────────────────────────────────────────────────────────

func defaultGetSubject(c *astra.Ctx) string {
	if v, ok := c.Get("user_id"); ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func defaultRBACError(c *astra.Ctx) error {
	return c.JSON(http.StatusForbidden, astra.Map{
		"code":    http.StatusForbidden,
		"message": "access denied",
	})
}
