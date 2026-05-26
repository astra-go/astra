//go:build !linux && !darwin

package netengine

import "fmt"

// setSockOptTFO is a stub for platforms where TCP Fast Open is not supported
// by this package.  Callers should check the returned error and either warn
// the user or fall back to a plain listener.
func setSockOptTFO(_ uintptr, _ int) error {
	return fmt.Errorf("netengine: TCP_FASTOPEN is not supported on this platform")
}
