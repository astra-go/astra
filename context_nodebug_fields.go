//go:build !astra_debug

package astra

// debugFields is empty in production builds to avoid any overhead.
type debugFields struct{}
