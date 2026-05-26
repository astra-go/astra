package middleware

import (
	"net/url"
	"strings"
)

// DefaultSensitiveParams is the canonical list of query-parameter names
// redacted by default in both access logs (Logger) and trace spans (Tracing).
// Extend or replace it via WithLoggerSensitiveParams / WithTracingRedactParams.
var DefaultSensitiveParams = []string{
	"token", "access_token", "refresh_token", "id_token",
	"password", "passwd", "secret", "api_key", "apikey",
	"key", "auth", "authorization", "sig", "signature",
	"client_secret",
}

// buildSensitiveSet converts a slice of parameter names into a
// case-insensitive lookup set for O(1) membership tests.
func buildSensitiveSet(params []string) map[string]bool {
	set := make(map[string]bool, len(params))
	for _, p := range params {
		set[strings.ToLower(p)] = true
	}
	return set
}

// redactQuery replaces the values of sensitive keys in q with "REDACTED".
// Operates in-place; q is returned for chaining.
func redactQuery(q url.Values, sensitiveSet map[string]bool) url.Values {
	for key := range q {
		if sensitiveSet[strings.ToLower(key)] {
			q[key] = []string{"REDACTED"}
		}
	}
	return q
}

// sanitizeRawQuery parses rawQuery, redacts sensitive values, and re-encodes it.
// Returns rawQuery unchanged when sensitiveSet is empty.
// If parsing fails the entire string is replaced with "[REDACTED]".
func sanitizeRawQuery(rawQuery string, sensitiveSet map[string]bool) string {
	if len(sensitiveSet) == 0 {
		return rawQuery
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		// Malformed query — surface nothing rather than potentially leaking.
		return "[REDACTED]"
	}
	return redactQuery(q, sensitiveSet).Encode()
}
