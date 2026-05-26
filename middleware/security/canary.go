// Package middleware provides canary deployment / traffic-coloring middleware.
//
// Canary returns a middleware that evaluates rules in order and sets the
// "canary_version" context value on the first match. Downstream handlers can
// read the version with c.MustGet("canary_version") to implement feature flags,
// A/B tests, or blue/green routing.
//
// # Rule matching
//
// Each CanaryRule supports three independent match strategies:
//
//  1. Header — match by HTTP request header (name + optional value regex)
//  2. Cookie — match by HTTP cookie (name + optional value regex)
//  3. User ID hash — extract a user ID from the context, hash it, and check
//     hash(id) % Modulo == Remainder for percentage-based rollouts
//
// When multiple fields are set on one rule, ALL conditions must match (AND logic).
// Rules are evaluated in declaration order; the first matching rule wins.
//
// # Example
//
//	app.Use(middleware.Canary([]middleware.CanaryRule{
//	    // Explicit opt-in: X-Canary: true  → version "v2"
//	    {Header: "X-Canary", HeaderRE: "^true$", Version: "v2"},
//
//	    // Cookie-based: canary=1  → version "v2"
//	    {Cookie: "canary", CookieRE: "^1$", Version: "v2"},
//
//	    // 10 % rollout by user ID
//	    {UserIDKey: "user_id", Modulo: 10, Remainder: 0, Version: "v2"},
//	}))
//
//	// In a handler:
//	func myHandler(c *contract.Context) error {
//	    version, _ := c.Get("canary_version")
//	    if version == "v2" {
//	        return handleV2(c)
//	    }
//	    return handleStable(c)
//	}
package security

import (
	"fmt"
	"hash/fnv"
	"regexp"

	"github.com/astra-go/astra"
)

// canaryVersionKey is the context key used to expose the matched canary version.
const canaryVersionKey = "canary_version"

// CanaryRule defines the conditions and version label for one canary segment.
//
// Multiple fields within a single rule are ANDed: all conditions must pass.
// Rules in the slice are ORed: the first matching rule wins.
type CanaryRule struct {
	// Header is the name of the HTTP request header to check.
	// When non-empty, the header must be present.
	Header string

	// HeaderRE is a regular expression applied to the Header value.
	// Empty means "header must exist" (any value).
	HeaderRE string

	// Cookie is the name of the HTTP cookie to check.
	Cookie string

	// CookieRE is a regular expression applied to the Cookie value.
	// Empty means "cookie must exist" (any value).
	CookieRE string

	// UserIDKey is the context key used to retrieve the user's identifier
	// (e.g. "user_id" set by JWT middleware).
	// When set, Modulo must also be > 0.
	UserIDKey string

	// Modulo divides the FNV-1a hash of the user ID.
	// Modulo == 10, Remainder == 0 means ~10 % of users.
	Modulo int

	// Remainder is the expected result of hash(userID) % Modulo.
	// Default: 0.
	Remainder int

	// Version is the canary label written to the context when this rule matches.
	Version string
}

// compiledRule is the internal representation with pre-compiled regexps.
type compiledRule struct {
	CanaryRule
	headerRE *regexp.Regexp
	cookieRE *regexp.Regexp
}

// Canary returns a middleware that evaluates rules in order and writes the
// matched version to c.Set(canaryVersionKey, rule.Version).
//
// When no rule matches, canaryVersionKey is set to "" (stable traffic).
// Subsequent middleware or handlers can read the value with c.Get("canary_version").
func Canary(rules []CanaryRule) astra.HandlerFunc {
	compiled := make([]compiledRule, len(rules))
	for i, r := range rules {
		cr := compiledRule{CanaryRule: r}
		if r.HeaderRE != "" {
			cr.headerRE = regexp.MustCompile(r.HeaderRE)
		}
		if r.CookieRE != "" {
			cr.cookieRE = regexp.MustCompile(r.CookieRE)
		}
		compiled[i] = cr
	}

	return func(c *astra.Ctx) error {
		version := ""
		for _, cr := range compiled {
			if matchesRule(c, cr) {
				version = cr.Version
				break
			}
		}
		c.Set(canaryVersionKey, version)
		c.Next()
		return nil
	}
}

// matchesRule reports whether ALL conditions in cr are satisfied by the request.
func matchesRule(c *astra.Ctx, cr compiledRule) bool {
	// Header condition
	if cr.Header != "" {
		val := c.Request().Header.Get(cr.Header)
		if val == "" {
			return false
		}
		if cr.headerRE != nil && !cr.headerRE.MatchString(val) {
			return false
		}
	}

	// Cookie condition
	if cr.Cookie != "" {
		cookie, err := c.Request().Cookie(cr.Cookie)
		if err != nil {
			return false
		}
		if cr.cookieRE != nil && !cr.cookieRE.MatchString(cookie.Value) {
			return false
		}
	}

	// User ID hash condition
	if cr.UserIDKey != "" && cr.Modulo > 0 {
		raw, ok := c.Get(cr.UserIDKey)
		if !ok {
			return false
		}
		uid := fmt.Sprintf("%v", raw)
		if uid == "" {
			return false
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(uid))
		if int(h.Sum32())%cr.Modulo != cr.Remainder {
			return false
		}
	}

	// A rule with no conditions matches nothing (prevents accidental full-redirect).
	if cr.Header == "" && cr.Cookie == "" && cr.UserIDKey == "" {
		return false
	}

	return true
}
