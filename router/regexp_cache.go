package router

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/golang-lru/v2"
)

// regexpCache is an LRU cache mapping anchored pattern strings ("^(?:<pattern>)$")
// to their compiled *regexp.Regexp.
//
// Why LRU instead of sync.Map?
//   - sync.Map in the old code had no eviction policy — an attacker could exhaust
//     memory by sending requests with unique regex path parameters.
//   - LRU bounds memory at compile time (max 1024 compiled regexps ≈ 1–2 MB).
//   - hashicorp/golang-lru/v2 is goroutine-safe and used by Docker, Cilium, etc.
//
// Size rationale:
//   - Typical app: < 20 regex routes.
//   - Large app: < 200 regex routes.
//   - 1024 is 5× the practical upper bound, leaving room for dynamic patterns
//     (e.g. user-generated routes) without memory exhaustion.
var regexpCache *lru.Cache[string, *regexp.Regexp]

// init creates the shared LRU cache (1024 entries, ~1–2 MB).
func init() {
	var err error
	// 1024 is the maximum number of distinct regex patterns that can be cached
	// simultaneously.  Exceeding this evicts the least-recently-used entry.
	regexpCache, err = lru.New[string, *regexp.Regexp](1024)
	if err != nil {
		panic(fmt.Sprintf("astra/router: failed to create regexp LRU cache: %v", err))
	}
}

// getOrCompileRegexp returns a shared *regexp.Regexp for the anchored pattern
// "^(?:<pattern>)$", compiling and caching it on first use.
//
// All routes with the same pattern share one Regexp instance and therefore one
// internal machine pool, which reduces pool fragmentation and GC pressure.
//
// Concurrency: lru.Cache is goroutine-safe; concurrent callers of
// Get/Add will all receive the same *Regexp instance.
func getOrCompileRegexp(pattern string) (*regexp.Regexp, error) {
	anchored := "^(?:" + pattern + ")$"

	// 1. Fast path: cache hit.
	if v, ok := regexpCache.Get(anchored); ok {
		return v, nil
	}

	// 2. Slow path: compile and store.
	re, err := regexp.Compile(anchored)
	if err != nil {
		return nil, err
	}

	// Add returns true if the key was new (not already present due to a race).
	// If two goroutines race here, both compile — but only one wins the Add;
	// the loser's regexp is discarded and GC'd.
	regexpCache.Add(anchored, re)

	return re, nil
}
