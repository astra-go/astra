package router

import (
	"regexp"
	"testing"
)

// ─── compileFastMatcher unit tests ────────────────────────────────────────────

func TestCompileFastMatcher_WellKnown(t *testing.T) {
	for pattern := range wellKnownMatchers {
		fm := compileFastMatcher(pattern)
		if fm == nil {
			t.Errorf("compileFastMatcher(%q) returned nil for well-known pattern", pattern)
		}
	}
}

func TestCompileFastMatcher_WellKnownExactResults(t *testing.T) {
	for pattern, expected := range wellKnownMatchers {
		fm := compileFastMatcher(pattern)
		if fm == nil {
			t.Errorf("compileFastMatcher(%q) returned nil", pattern)
			continue
		}
		testInputs := []string{"hello", "12345", "Hello123", "hello-world", "hello_world", "", "abc123DEF", "ABC"}
		for _, input := range testInputs {
			expectedResult := expected(input)
			gotResult := fm(input)
			if expectedResult != gotResult {
				t.Errorf("compileFastMatcher(%q)(%q) = %v, want %v", pattern, input, gotResult, expectedResult)
			}
		}
	}
}

func TestCompileFastMatcher_NewPatterns(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		// Hex patterns
		{`[0-9a-f]+`, "deadbeef", true},
		{`[0-9a-f]+`, "DEADBEEF", false},
		{`[0-9a-fA-F]+`, "DeadBeef", true},
		{`[0-9a-fA-F]+`, "deadBEEF42", true},
		{`[0-9a-fA-F]+`, "nothex", false},

		// \d shorthand
		{`\d+`, "12345", true},
		{`\d+`, "abc", false},

		// \w shorthand
		{`\w+`, "hello_world123", true},
		{`\w+`, "hello-world", false},

		// Bounded quantifiers
		{`[0-9]{1,3}`, "42", true},
		{`[0-9]{1,3}`, "1234", false},
		{`[0-9]{1,3}`, "", false},
		{`[0-9]{4}`, "2026", true},
		{`[0-9]{4}`, "99", false},
		{`[a-z]{2,5}`, "hi", true},
		{`[a-z]{2,5}`, "helloworld", false},

		// Negated class
		{`[^0-9]+`, "hello", true},
		{`[^0-9]+`, "h3llo", false},

		// Single-char class (no quantifier)
		{`[0-9]`, "5", true},
		{`[0-9]`, "55", false},

		// Unbounded with min > 1
		{`[a-z]{2,}`, "hi", true},
		{`[a-z]{2,}`, "a", false},
	}

	for _, tt := range tests {
		fm := compileFastMatcherNoWellKnown(tt.pattern)
		if fm == nil {
			t.Logf("compileFastMatcher(%q) returned nil (unsupported), skipping", tt.pattern)
			continue
		}
		got := fm(tt.input)
		if got != tt.want {
			t.Errorf("compileFastMatcher(%q)(%q) = %v, want %v", tt.pattern, tt.input, got, tt.want)
		}
	}
}

func TestCompileFastMatcher_Unsupported(t *testing.T) {
	unsupported := []string{
		`[0-9a-f]{8}-[0-9a-f]{4}`,
		`(a|b)+`,
		`a+b+`,
		`.*`,
		`\d+\.\d+`,
		`^hello$`,
	}
	for _, pattern := range unsupported {
		fm := compileFastMatcherNoWellKnown(pattern)
		if fm != nil {
			t.Errorf("compileFastMatcher(%q) should return nil for unsupported pattern, got non-nil", pattern)
		}
	}
}

func TestCompileFastMatcher_Correctness(t *testing.T) {
	patterns := []string{
		`[0-9]+`, `[a-z]+`, `[A-Z]+`, `[a-zA-Z]+`, `[a-zA-Z0-9]+`,
		`[a-z0-9]+`, `[A-Z0-9]+`, `[a-zA-Z0-9_]+`, `\d+`, `\w+`,
		`[0-9a-f]+`, `[0-9a-fA-F]+`,
		`[0-9]{1,3}`, `[a-z]{2,5}`, `[0-9]{4}`,
		`[a-z0-9\-]+`, `[a-zA-Z0-9_\-]+`,
		`[^0-9]+`,
	}

	inputs := []string{
		"", "a", "Z", "0", "9", "_", "-",
		"hello", "HELLO", "12345", "hello123",
		"Hello123", "hello-world", "hello_world",
		"deadbeef", "DeadBeef42",
		"abc!def", "with space", "ab", "abcde", "abcdef",
		"1", "12", "123", "1234",
		"2026", "99",
		"not-hex-zZ",
	}

	for _, pattern := range patterns {
		fm := compileFastMatcherNoWellKnown(pattern)
		if fm == nil {
			continue
		}
		re := regexp.MustCompile("^(?:" + pattern + ")$")
		for _, input := range inputs {
			expected := re.MatchString(input)
			got := fm(input)
			if got != expected {
				t.Errorf("mismatch: pattern=%q input=%q fastMatcher=%v regexp=%v",
					pattern, input, got, expected)
			}
		}
	}
}

// ─── Benchmarks: compileFastMatcher vs regexp.MatchString ─────────────────────

func BenchmarkFastMatcher_HexLower(b *testing.B) {
	fm := compileFastMatcher(`[0-9a-f]+`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fm("deadbeef")
	}
}

func BenchmarkRegexpMatchString_HexLower(b *testing.B) {
	re := regexp.MustCompile("^(?:[0-9a-f]+)$")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		re.MatchString("deadbeef")
	}
}

func BenchmarkFastMatcher_HexMixed(b *testing.B) {
	fm := compileFastMatcher(`[0-9a-fA-F]+`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fm("DeadBeef42")
	}
}

func BenchmarkRegexpMatchString_HexMixed(b *testing.B) {
	re := regexp.MustCompile("^(?:[0-9a-fA-F]+)$")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		re.MatchString("DeadBeef42")
	}
}

func BenchmarkFastMatcher_BoundedDigits(b *testing.B) {
	fm := compileFastMatcher(`[0-9]{1,10}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fm("12345")
	}
}

func BenchmarkRegexpMatchString_BoundedDigits(b *testing.B) {
	re := regexp.MustCompile("^(?:[0-9]{1,10})$")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		re.MatchString("12345")
	}
}

func BenchmarkFastMatcher_SlugDash(b *testing.B) {
	fm := compileFastMatcher(`[a-z0-9\-]+`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fm("my-slug-123")
	}
}

func BenchmarkRegexpMatchString_SlugDash(b *testing.B) {
	re := regexp.MustCompile("^(?:[a-z0-9\\-]+)$")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		re.MatchString("my-slug-123")
	}
}
