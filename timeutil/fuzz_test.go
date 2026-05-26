package timeutil

import (
	"encoding/json"
	"testing"
)

// FuzzTimeUnmarshalJSON tests that Time.UnmarshalJSON never panics
// on arbitrary JSON input. This exercises the timestamp parsing cascade
// (configured layout → RFC3339 → fallbacks) and the Unix timestamp path.
func FuzzTimeUnmarshalJSON(f *testing.F) {
	seeds := []string{
		"null",
		`""`,
		`"2024-01-15 10:30:00"`,
		`"2024-01-15T10:30:00Z"`,
		"1705312200",
		"-100000",
		`"not a date"`,
		`"2024-13-45 99:99:99"`,
		`""`,
		"3.14",
		"true",
		"[]",
		`"2024-01-15"`,
		`"15:04:05"`,
		`"日本語"`,
		`"2024-01-15T10:30:00.123456789Z"`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		var tm Time
		// Must not panic; errors are acceptable for malformed input
		_ = json.Unmarshal([]byte(input), &tm)
	})
}

// FuzzTimeUnmarshalText tests the TextUnmarshaler path with raw byte input.
func FuzzTimeUnmarshalText(f *testing.F) {
	seeds := [][]byte{
		[]byte(""),
		[]byte("2024-01-15 10:30:00"),
		[]byte("2024-01-15T10:30:00Z"),
		[]byte("garbage"),
		[]byte("2024-13-45"),
		[]byte("0"),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		var tm Time
		_ = tm.UnmarshalText(input)
	})
}
