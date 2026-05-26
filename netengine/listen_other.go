//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !windows

package netengine

import (
	"fmt"
	"net"
)

// ListenOptions is defined here for platforms where no special socket options
// are supported.  All fields are accepted but silently ignored.
type ListenOptions struct {
	ReusePort        bool
	FastOpen         bool
	FastOpenQueueLen int
}

// Listen creates a plain TCP listener on unsupported platforms.
// The opts argument is accepted for API compatibility but ignored; if
// opts.ReusePort or opts.FastOpen is true, an error is returned because the
// requested capabilities are not available on this platform.
func Listen(network, addr string, opts ListenOptions) (net.Listener, error) {
	if opts.ReusePort {
		return nil, fmt.Errorf("netengine: Listen: SO_REUSEPORT is not supported on this platform; use net.Listen(%q) instead", addr)
	}
	if opts.FastOpen {
		return nil, fmt.Errorf("netengine: Listen: TCP_FASTOPEN is not supported on this platform")
	}
	return net.Listen(network, addr)
}

// ListenReusePort is not supported on this platform.
// SO_REUSEPORT requires Linux, macOS, or a BSD kernel.
// Returns an error; callers should fall back to net.Listen.
func ListenReusePort(_, addr string) (net.Listener, error) {
	return nil, fmt.Errorf("netengine: ListenReusePort: SO_REUSEPORT not supported on this platform; use net.Listen(%q) instead", addr)
}
