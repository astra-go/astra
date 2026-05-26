package astra

import (
	"testing"
)

// FuzzFastMatchers tests the internal fast-path matcher functions
// (fastDigits, fastLower, fastAlpha, etc.) never panic on arbitrary input.
// These are exposed via export_test.go variables.
func FuzzFastMatchers(f *testing.F) {
	seeds := []string{
		"",
		"abc",
		"123",
		"abc123",
		"ABC",
		"hello-world",
		"hello_world",
		"日本語",
		"🚀",
		"a b",
		"123abc",
		"!@#$%",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// None of these should panic
		_ = FastDigits(s)
		_ = FastLower(s)
		_ = FastUpper(s)
		_ = FastAlpha(s)
		_ = FastAlphanum(s)
		_ = FastAlphanumLower(s)
		_ = FastAlphanumUpper(s)
		_ = FastSlugLower(s)
		_ = FastSlug(s)
		_ = FastIdentifier(s)
	})
}

// FuzzGetOrCompileRegexp tests regex compilation with arbitrary patterns.
// Must not panic; compilation errors are acceptable.
func FuzzGetOrCompileRegexp(f *testing.F) {
	seeds := []string{
		"[0-9]+",
		`\d+`,
		"[a-z]+",
		"[invalid",
		"(?P<name>.*)",
		"^.*$",
		"",
		"(.+",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, pattern string) {
		re, err := GetOrCompileRegexp(pattern)
		if err != nil {
			return // compilation error is expected for invalid patterns
		}
		// If compiled successfully, matching must not panic
		_ = re.MatchString("test")
	})
}
