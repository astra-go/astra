package router

import "sync"

// Fast matcher exports for fuzz tests in the main astra package.
var (
	FastDigits        = fastDigits
	FastLower         = fastLower
	FastUpper         = fastUpper
	FastAlpha         = fastAlpha
	FastAlphanum      = fastAlphanum
	FastAlphanumLower = fastAlphanumLower
	FastAlphanumUpper = fastAlphanumUpper
	FastSlugLower     = fastSlugLower
	FastSlug          = fastSlug
	FastIdentifier    = fastIdentifier
)

// GetOrCompileRegexp exposes getOrCompileRegexp for fuzz tests.
var GetOrCompileRegexp = getOrCompileRegexp

// AllowedCache exposes the allowedCache field for white-box tests.
func AllowedCache(r *Router) *sync.Map {
	return &r.allowedCache
}
