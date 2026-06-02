//go:build !astra_debug

package astra

// debugCheckConcurrency is a no-op in production builds.
// When the astra_debug build tag is not set, this function compiles
// to nothing and the goroutineID field is not present in Ctx.
func (c *Ctx) debugCheckConcurrency() {}

// debugReset is a no-op in production builds.
func (c *Ctx) debugReset() {}

// debugGetOwnerGoroutineID always returns 0 in production builds.
func (c *Ctx) debugGetOwnerGoroutineID() int64 {
	return 0
}

// DebugBuild reports whether the astra_debug build tag was set.
// Always false in production builds.
func DebugBuild() bool {
	return false
}
