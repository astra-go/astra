package binding

import (
	"net/url"
	"testing"
)

type queryTarget struct {
	Name   string  `query:"name"`
	Age    int     `query:"age"`
	Active bool    `query:"active"`
	Score  float64 `query:"score"`
	Tags   []string `query:"tags"`
}

// FuzzBindQuery tests that BindQuery never panics on arbitrary
// query parameter values. This exercises setFieldValue for all
// scalar types and slices.
func FuzzBindQuery(f *testing.F) {
	seeds := []struct{ k, v string }{
		{"name", "Alice"},
		{"age", "25"},
		{"active", "true"},
		{"score", "3.14"},
		{"tags", "go"},
		{"name", ""},
		{"age", "not_a_number"},
		{"score", "NaN"},
		{"active", "maybe"},
		{"tags", ""},
		{"name", "日本語テスト"},
		{"age", "999999999999999999999999"},
		{"score", "1e308"},
		{"name", "<script>alert(1)</script>"},
		{"tags", "a"},
	}
	for _, s := range seeds {
		f.Add(s.k, s.v)
	}

	f.Fuzz(func(t *testing.T, key, val string) {
		v := url.Values{key: {val}}
		var target queryTarget
		// Must not panic; errors are acceptable for malformed input
		_ = BindQuery(v, &target)
	})
}

// FuzzBindQueryMultiple tests slice overflow protection with many
// repeated query parameter keys.
func FuzzBindQueryMultiple(f *testing.F) {
	f.Add(5, "tag")
	f.Add(1001, "x")
	f.Add(0, "")
	f.Add(50, "日本語")

	f.Fuzz(func(t *testing.T, count int, val string) {
		if count < 0 {
			count = 0
		}
		if count > 2000 {
			count = 2000
		}
		vals := make(url.Values)
		tags := make([]string, count)
		for i := range tags {
			tags[i] = val
		}
		vals["tags"] = tags
		var target queryTarget
		_ = BindQuery(vals, &target)
		// Must not panic; should return error when count > MaxSliceParams
	})
}

type pathTarget struct {
	ID   string `uri:"id"`
	Slug string `uri:"slug"`
}

// FuzzBindPath tests BindPath with arbitrary path parameter values.
func FuzzBindPath(f *testing.F) {
	seeds := []struct{ k, v string }{
		{"id", "42"},
		{"slug", "hello-world"},
		{"id", ""},
		{"id", "../../../etc/passwd"},
		{"slug", "<img onerror=alert(1)>"},
		{"id", "日本語"},
		{"unknown_key", "anything"},
	}
	for _, s := range seeds {
		f.Add(s.k, s.v)
	}

	f.Fuzz(func(t *testing.T, key, val string) {
		params := []Param{{Key: key, Value: val}}
		var target pathTarget
		_ = BindPath(params, &target)
	})
}
