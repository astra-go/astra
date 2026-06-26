package middleware

import (
	"crypto/sha256"
	"fmt"
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

// ============================================================================
// Data masking — for logging, API responses, and PII protection.
// ============================================================================

// MaskSensitive masks all but the last showLast characters of input.
// When input is empty, returns "***".  When showLast ≥ length of input,
// returns a fixed-length mask (max 8 asterisks).
//
//	MaskSensitive("13812345678", 4)  → "********5678"
//	MaskSensitive("a", 2)             → "********"
func MaskSensitive(input string, showLast int) string {
	if input == "" {
		return "***"
	}
	runes := []rune(input)
	l := len(runes)

	if showLast < 0 || showLast >= l {
		n := l
		if n > 8 {
			n = 8
		}
		return strings.Repeat("*", n)
	}

	return strings.Repeat("*", l-showLast) + string(runes[l-showLast:])
}

// MaskPhone masks a phone number, revealing only the last 4 digits.
//
//	MaskPhone("13812345678") → "********5678"
func MaskPhone(phone string) string {
	return MaskSensitive(phone, 4)
}

// MaskEmail masks an email address, revealing only the first character and
// the domain part.
//
//	MaskEmail("user@example.com") → "u***@example.com"
func MaskEmail(email string) string {
	if email == "" {
		return "***"
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return MaskSensitive(email, 2)
	}
	username := parts[0]
	domain := parts[1]

	if len(username) == 0 {
		return "***@" + domain
	}

	masked := string([]rune(username)[0]) + strings.Repeat("*", max(len(username)-1, 1))
	return masked + "@" + domain
}

// MaskToken masks a token value, revealing only the last 4 characters.
//
//	MaskToken("abc123xyz789") → "********9789"
func MaskToken(token string) string {
	return MaskSensitive(token, 4)
}

// HashSensitive irreversibly hashes a sensitive value using SHA-256 and
// returns the first 16 hex characters for use in log correlation without
// exposing the plaintext value.
//
//	HashSensitive("user@example.com") → "a1b2c3d4e5f6g7h8"
func HashSensitive(input string) string {
	if input == "" {
		return "empty"
	}
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", h)[:16]
}

// SanitizeLogField automatically selects a masking strategy based on the
// field name.  Recognised prefixes: phone/mobile → MaskPhone, email → MaskEmail,
// token/password/code → MaskToken.  Other fields are returned unchanged.
//
//	SanitizeLogField("phone", "13812345678") → "********5678"
//	SanitizeLogField("email", "a@b.com")     → "a***@b.com"
func SanitizeLogField(fieldName, value string) string {
	lower := strings.ToLower(fieldName)
	switch {
	case strings.Contains(lower, "phone"), strings.Contains(lower, "mobile"):
		return MaskPhone(value)
	case strings.Contains(lower, "email"):
		return MaskEmail(value)
	case strings.Contains(lower, "token"), strings.Contains(lower, "password"), strings.Contains(lower, "code"):
		return MaskToken(value)
	default:
		return value
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
