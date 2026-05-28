package sanitize_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/astra-go/astra/internal/sanitize"
)

// ─── BuildSet ─────────────────────────────────────────────────────────────────

func TestBuildSet_CaseInsensitive(t *testing.T) {
	set := sanitize.BuildSet([]string{"Token", "API_KEY", "password"})
	for _, key := range []string{"token", "api_key", "password", "TOKEN", "Password"} {
		if !set[strings.ToLower(key)] {
			t.Errorf("BuildSet: expected %q to be in set", key)
		}
	}
}

func TestBuildSet_Empty(t *testing.T) {
	set := sanitize.BuildSet(nil)
	if len(set) != 0 {
		t.Errorf("expected empty set, got %v", set)
	}
}

func TestBuildSet_DefaultSensitiveParams(t *testing.T) {
	set := sanitize.BuildSet(sanitize.DefaultSensitiveParams)
	for _, key := range sanitize.DefaultSensitiveParams {
		if !set[key] {
			t.Errorf("DefaultSensitiveParams: %q missing from set", key)
		}
	}
}

// ─── RedactQuery ──────────────────────────────────────────────────────────────

func TestRedactQuery_RedactsSensitiveKeys(t *testing.T) {
	set := sanitize.BuildSet([]string{"token", "password"})
	q := url.Values{
		"token":    {"secret123"},
		"password": {"hunter2"},
		"page":     {"1"},
	}
	result := sanitize.RedactQuery(q, set)
	if result.Get("token") != "REDACTED" {
		t.Errorf("token: want REDACTED, got %q", result.Get("token"))
	}
	if result.Get("password") != "REDACTED" {
		t.Errorf("password: want REDACTED, got %q", result.Get("password"))
	}
	if result.Get("page") != "1" {
		t.Errorf("page: want 1, got %q", result.Get("page"))
	}
}

func TestRedactQuery_EmptySet_NoRedaction(t *testing.T) {
	set := sanitize.BuildSet(nil)
	q := url.Values{"token": {"secret"}}
	result := sanitize.RedactQuery(q, set)
	if result.Get("token") != "secret" {
		t.Errorf("empty set: token should not be redacted, got %q", result.Get("token"))
	}
}

func TestRedactQuery_CaseInsensitiveKey(t *testing.T) {
	set := sanitize.BuildSet([]string{"api_key"})
	q := url.Values{"API_KEY": {"mykey"}}
	result := sanitize.RedactQuery(q, set)
	if result.Get("API_KEY") != "REDACTED" {
		t.Errorf("case-insensitive: want REDACTED, got %q", result.Get("API_KEY"))
	}
}

func TestRedactQuery_ReturnsQForChaining(t *testing.T) {
	set := sanitize.BuildSet([]string{"token"})
	q := url.Values{"token": {"x"}}
	returned := sanitize.RedactQuery(q, set)
	if returned == nil {
		t.Error("RedactQuery should return the modified url.Values")
	}
}

// ─── RawQuery ─────────────────────────────────────────────────────────────────

func TestRawQuery_EmptySet_ReturnsUnchanged(t *testing.T) {
	raw := "token=secret&page=1"
	result := sanitize.RawQuery(raw, map[string]bool{})
	if result != raw {
		t.Errorf("empty set: want %q, got %q", raw, result)
	}
}

func TestRawQuery_RedactsSensitiveParam(t *testing.T) {
	set := sanitize.BuildSet([]string{"token"})
	raw := "token=secret&page=2"
	result := sanitize.RawQuery(raw, set)
	if strings.Contains(result, "secret") {
		t.Errorf("token value should be redacted, got %q", result)
	}
	if !strings.Contains(result, "REDACTED") {
		t.Errorf("expected REDACTED in result, got %q", result)
	}
	if !strings.Contains(result, "page=2") {
		t.Errorf("non-sensitive param should be preserved, got %q", result)
	}
}

func TestRawQuery_InvalidQuery_ReturnsRedacted(t *testing.T) {
	set := sanitize.BuildSet([]string{"token"})
	// A raw query with a percent-encoding error is invalid.
	result := sanitize.RawQuery("token=%ZZ", set)
	if result != "[REDACTED]" {
		t.Errorf("invalid query: want [REDACTED], got %q", result)
	}
}

func TestRawQuery_EmptyString_ReturnsEmpty(t *testing.T) {
	set := sanitize.BuildSet([]string{"token"})
	result := sanitize.RawQuery("", set)
	if result != "" {
		t.Errorf("empty raw query: want empty, got %q", result)
	}
}

func TestRawQuery_MultipleValues_AllRedacted(t *testing.T) {
	set := sanitize.BuildSet([]string{"token"})
	raw := "token=a&token=b&safe=ok"
	result := sanitize.RawQuery(raw, set)
	if strings.Contains(result, "=a") || strings.Contains(result, "=b") {
		t.Errorf("all token values should be redacted, got %q", result)
	}
}
