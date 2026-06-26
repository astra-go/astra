package mask

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// Sensitive masks an arbitrary input string, showing only the last `n`
// characters. If n < 0 or n >= len(input), the input is fully masked with
// at most 8 asterisks. An empty input returns "[redacted]".
func Sensitive(input string, showLast int) string {
	if input == "" {
		return "[redacted]"
	}
	runes := []rune(input)
	length := len(runes)

	if showLast < 0 || showLast >= length {
		return strings.Repeat("*", min(length, 8))
	}

	masked := strings.Repeat("*", length-showLast)
	return masked + string(runes[length-showLast:])
}

// Phone masks a phone number, keeping the last 4 digits visible.
func Phone(phone string) string {
	return Sensitive(phone, 4)
}

// Token masks an access or refresh token, keeping the last 4 characters.
func Token(token string) string {
	return Sensitive(token, 4)
}

// Email masks an email address, keeping the first character and the domain.
//
//	user@example.com → u***@example.com
//	@example.com     → [redacted]@example.com
//	notanemail       → full mask
//	empty            → [redacted]
func Email(email string) string {
	if email == "" {
		return "[redacted]"
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return Sensitive(email, 2)
	}
	username := parts[0]
	domain := parts[1]

	if len(username) == 0 {
		return "[redacted]@" + domain
	}

	first := string([]rune(username)[0])
	maskedUser := first + strings.Repeat("*", max(len(username)-1, 1))
	return maskedUser + "@" + domain
}

// Hash computes a truncated SHA-256 hex digest of the input (one-way
// obfuscation). An empty input returns "[empty]".
func Hash(input string) string {
	if input == "" {
		return "[empty]"
	}
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", h[:8])
}

// ByFieldName chooses a masking strategy based on the field name (case-insensitive).
//
//	phone/mobile → Phone()
//	email        → Email()
//	token/pwd/pw → Token()  (tokens, passwords, codes, secrets, keys)
//	default      → returned as-is
func ByFieldName(fieldName, value string) string {
	fieldLower := strings.ToLower(fieldName)
	switch {
	case containsAny(fieldLower, "phone", "mobile"):
		return Phone(value)
	case strings.Contains(fieldLower, "email"):
		return Email(value)
	case containsAny(fieldLower, "token", "password", "passwd", "pwd", "code", "secret", "key"):
		return Token(value)
	default:
		return value
	}
}

// -- helpers --

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
