//go:build !astra_debug

package astra

// debugCheckConcurrency is a no-op in production builds.
// When the astra_debug build tag is not set, this function compiles to nothing
// and the goroutineID field is not present in Ctx, resulting in zero overhead.
func (c *Ctx) debugCheckConcurrency() {}

// debugReset is a no-op in production builds.
func (c *Ctx) debugReset() {}
