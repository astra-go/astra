//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !windows

package netengine

// isTemporary falls back to the deprecated net.Error.Temporary() interface on
// platforms where we cannot inspect syscall.Errno directly.
// NOTE: Go 1.21+ always returns false from Temporary(); this is a best-effort
// fallback for non-unix platforms only.
func isTemporary(err error) bool {
	type temporary interface{ Temporary() bool }
	t, ok := err.(temporary)
	return ok && t.Temporary()
}

// isResourceExhaustion always returns false on non-unix platforms; we cannot
// reliably distinguish resource exhaustion from fatal errors without the
// syscall.Errno chain.
func isResourceExhaustion(_ error) bool { return false }
