package i18n

import (
	"sort"
	"strconv"
	"strings"
)

// DetectLocale picks the best-matching locale from an HTTP request.
//
// Detection order (first match wins):
//  1. ?lang= query parameter (exact match)
//  2. X-Language request header (exact match)
//  3. Accept-Language header (quality-sorted; both full tag and language prefix)
//  4. Bundle's fallback locale
//
// "Match" means the locale is registered in b.
func DetectLocale(b *Bundle, query, xlang, acceptLang string) string {
	// 1. Explicit query param.
	if query != "" {
		if b.Has(query) {
			return query
		}
	}
	// 2. X-Language header.
	if xlang != "" {
		if b.Has(xlang) {
			return xlang
		}
	}
	// 3. Accept-Language header: try quality-sorted candidates.
	if acceptLang != "" {
		for _, candidate := range parseAcceptLanguage(acceptLang) {
			if b.Has(candidate) {
				return candidate
			}
		}
	}
	// 4. Fallback.
	return b.fallback
}

// parseAcceptLanguage returns locale candidates in descending quality order.
// For each tag, the bare language prefix is also added (e.g. "zh" from "zh-CN")
// so that a bundle with only "zh" satisfies "zh-CN,zh;q=0.9".
//
// Example: "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7"
// → ["zh-CN", "zh", "en-US", "en"]
func parseAcceptLanguage(header string) []string {
	type langQ struct {
		lang string
		q    float64
	}

	parts := strings.Split(header, ",")
	items := make([]langQ, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lang := part
		q := 1.0
		if i := strings.IndexByte(part, ';'); i != -1 {
			lang = strings.TrimSpace(part[:i])
			rest := strings.TrimSpace(part[i+1:])
			if strings.HasPrefix(rest, "q=") {
				if v, err := strconv.ParseFloat(rest[2:], 64); err == nil {
					q = v
				}
			}
		}
		if lang != "" {
			items = append(items, langQ{lang: lang, q: q})
		}
	}

	// Stable sort descending by q so that equal-q entries keep original order.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].q > items[j].q
	})

	// Deduplicate and expand base language.
	seen := make(map[string]bool, len(items)*2)
	result := make([]string, 0, len(items)*2)
	for _, item := range items {
		tag := item.lang
		if !seen[tag] {
			result = append(result, tag)
			seen[tag] = true
		}
		// Also try the bare language prefix (e.g. "zh" from "zh-CN").
		if i := strings.IndexByte(tag, '-'); i != -1 {
			base := tag[:i]
			if !seen[base] {
				result = append(result, base)
				seen[base] = true
			}
		}
	}
	return result
}
