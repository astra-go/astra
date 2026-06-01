package router

import (
	"regexp"
	"sync"
)

// regexpCache maps anchored pattern strings ("^(?:<pattern>)$") to their
// compiled *regexp.Regexp.  Using sync.Map because the access pattern is
// write-once-at-startup / read-many-at-runtime: sync.Map avoids the global
// mutex contention that a plain map+RWMutex would impose under high load.
var regexpCache sync.Map // key: string → value: *regexp.Regexp

// getOrCompileRegexp returns a shared *regexp.Regexp for the anchored pattern
// "^(?:<pattern>)$", compiling and caching it on first use.  All routes with
// the same pattern share one Regexp instance and therefore one internal machine
// pool, which reduces pool fragmentation and GC pressure under concurrency.
func getOrCompileRegexp(pattern string) (*regexp.Regexp, error) {
	anchored := "^(?:" + pattern + ")$"
	if v, ok := regexpCache.Load(anchored); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(anchored)
	if err != nil {
		return nil, err
	}
	// LoadOrStore is safe under concurrent insertNode calls at startup.
	// If another goroutine stored first, we use its instance and let ours be GC'd.
	actual, _ := regexpCache.LoadOrStore(anchored, re)
	return actual.(*regexp.Regexp), nil
}
