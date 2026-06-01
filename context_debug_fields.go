//go:build astra_debug

package astra

// debugFields contains fields that are only present in debug builds.
// These fields are embedded in Ctx when the astra_debug build tag is set.
type debugFields struct {
	// goroutineID tracks the goroutine that owns this context.
	// Set on first access, verified on subsequent accesses.
	// Uses atomic operations to avoid false positives from compiler reordering.
	goroutineID int64
}
