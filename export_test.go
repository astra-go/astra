package astra

// SealPool exposes the private sealPool method for white-box benchmarks and
// tests that exercise the pool-sizing path without starting a real server.
func (a *App) SealPool() { a.sealPool() }

// JsonBufPool exposes jsonBufPool for white-box tests.
var JsonBufPool = &jsonBufPool

// JsonBufMaxCap exposes jsonBufMaxCap for white-box tests.
const JsonBufMaxCap = jsonBufMaxCap

// Fast matcher exports for fuzz tests.
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

// KvStoreMapThreshold exposes kvStoreMapThreshold for white-box tests.
const KvStoreMapThreshold = kvStoreMapThreshold

// CtxKvMap returns the internal kvMap field of a Ctx for white-box tests.
// Returns nil when the store has not yet been promoted to map mode.
func CtxKvMap(c *Ctx) map[string]any { return c.kvMap }

// AppRouter returns the concrete *Router from an App for white-box tests.
// Returns nil if the app uses a custom router.
func AppRouter(a *App) *Router {
	r, ok := a.router.(*Router)
	if !ok {
		return nil
	}
	return r
}

// RouterAllowedCacheLen returns the number of entries in the router's
// allowedMethods cache for white-box tests.
func RouterAllowedCacheLen(r *Router) int {
	n := 0
	r.allowedCache.Range(func(_, _ any) bool { n++; return true })
	return n
}
