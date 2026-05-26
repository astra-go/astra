// Package sanitize provides query-parameter redaction utilities shared across
// the middleware and observability sub-modules.
//
// This is an internal package — it is not part of the public API.
package sanitize

import (
	"net/url"
	"strings"
)

// DefaultSensitiveParams is the canonical list of query-parameter names
// redacted by default in access logs and trace spans.
var DefaultSensitiveParams = []string{
	"token", "access_token", "refresh_token", "id_token",
	"password", "passwd", "secret", "api_key", "apikey",
	"key", "auth", "authorization", "sig", "signature",
	"client_secret",
}

// BuildSet converts a slice of parameter names into a
// case-insensitive lookup set for O(1) membership tests.
func BuildSet(params []string) map[string]bool {
	set := make(map[string]bool, len(params))
	for _, p := range params {
		set[strings.ToLower(p)] = true
	}
	return set
}

// RedactQuery replaces the values of sensitive keys in q with "REDACTED".
// Operates in-place; q is returned for chaining.
func RedactQuery(q url.Values, sensitiveSet map[string]bool) url.Values {
	for key := range q {
		if sensitiveSet[strings.ToLower(key)] {
			q[key] = []string{"REDACTED"}
		}
	}
	return q
}

// RawQuery parses rawQuery, redacts sensitive values, and re-encodes it.
// Returns rawQuery unchanged when sensitiveSet is empty.
// If parsing fails the entire string is replaced with "[REDACTED]".
func RawQuery(rawQuery string, sensitiveSet map[string]bool) string {
	if len(sensitiveSet) == 0 {
		return rawQuery
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "[REDACTED]"
	}
	return RedactQuery(q, sensitiveSet).Encode()
}
