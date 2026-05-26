//go:build windows

package netengine

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

// Windows socket-level constants used to set socket options.
// These are not currently exported by Go's syscall package but are stable
// Winsock2 values defined in <winsock2.h>.
const (
	// _SOL_SOCKET is the socket-options level for Windows (0xFFFF).
	// On Unix this is syscall.SOL_SOCKET; Windows uses a different value.
	_SOL_SOCKET = 0xffff

	// _SO_REUSEADDR allows multiple sockets to bind the same local address and
	// port.  On Windows this enables the same multi-process load-balancing
	// semantics as SO_REUSEPORT on Linux/BSD: the OS accepts connections on any
	// of the bound sockets.  Note: Windows does not have a separate
	// SO_REUSEPORT; SO_REUSEADDR covers both the address-reuse and the
	// multi-listener use-cases.
	_SO_REUSEADDR = 0x4

	// _IPPROTO_TCP is the protocol level for TCP socket options (= 6).
	_IPPROTO_TCP = 0x6

	// _TCP_FASTOPEN enables TCP Fast Open on a listening socket.
	// Available on Windows 10 version 1607 / Windows Server 2016 and later.
	// Setting this option requires no additional privileges.
	// Value: 15 (decimal), the same optname used on Linux.
	_TCP_FASTOPEN = 15
)

// ListenOptions configures socket-level options applied when creating a TCP
// listener via Listen.  The zero value is safe: no special options are set.
type ListenOptions struct {
	// ReusePort enables SO_REUSEADDR (Windows equivalent of SO_REUSEPORT) so
	// that multiple independent listeners can bind the same address and port.
	// The OS load-balances incoming connections across all active listeners,
	// which is the building block for Prefork deployments.
	//
	// Note: on Windows, SO_REUSEADDR provides the same multi-listener semantics
	// as SO_REUSEPORT on Linux/BSD.  The field is named ReusePort to keep the
	// cross-platform API consistent.
	ReusePort bool

	// FastOpen enables TCP Fast Open (TFO) on the listening socket.
	// Requires Windows 10 version 1607 / Windows Server 2016 or later.
	// On earlier Windows versions Listen returns an error; callers should treat
	// this as non-fatal and fall back to a plain listener.
	FastOpen bool

	// FastOpenQueueLen is accepted for API compatibility with the Unix version
	// but is silently ignored on Windows: the OS manages the TFO queue
	// internally and does not expose a per-socket backlog setting.
	FastOpenQueueLen int
}

// Listen creates a TCP listener on addr with the requested socket options.
// network must be "tcp", "tcp4", or "tcp6".
//
// For the common ReusePort-only case, ListenReusePort is a simpler wrapper.
func Listen(network, addr string, opts ListenOptions) (net.Listener, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("netengine: Listen: unsupported network %q (want tcp/tcp4/tcp6)", network)
	}

	lc := net.ListenConfig{
		Control: func(_, _ string, c syscall.RawConn) error {
			var firstErr error
			ctrlErr := c.Control(func(fd uintptr) {
				if opts.ReusePort {
					// syscall.SetsockoptInt is available on Windows; the fd
					// value is the underlying SOCKET handle cast to uintptr.
					if err := syscall.SetsockoptInt(syscall.Handle(fd), _SOL_SOCKET, _SO_REUSEADDR, 1); err != nil && firstErr == nil {
						firstErr = fmt.Errorf("netengine: SO_REUSEADDR: %w", err)
					}
				}
				if opts.FastOpen {
					// Enable TCP Fast Open.  The option value (1) switches TFO
					// on; the kernel manages the queue depth automatically.
					if err := syscall.SetsockoptInt(syscall.Handle(fd), _IPPROTO_TCP, _TCP_FASTOPEN, 1); err != nil && firstErr == nil {
						firstErr = fmt.Errorf("netengine: TCP_FASTOPEN: requires Windows 10 1607+: %w", err)
					}
				}
			})
			if ctrlErr != nil {
				return ctrlErr
			}
			return firstErr
		},
	}
	return lc.Listen(context.Background(), network, addr)
}

// ListenReusePort creates a TCP listener on addr with SO_REUSEADDR enabled,
// which on Windows provides the same multi-listener load-balancing semantics
// as SO_REUSEPORT on Linux/BSD.
//
// Multiple independent processes (or goroutines) may call ListenReusePort with
// the same addr; the OS kernel load-balances incoming connections across all
// active listeners.  This is the building block for Prefork-style deployments.
//
// network must be "tcp", "tcp4", or "tcp6".
func ListenReusePort(network, addr string) (net.Listener, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("netengine: ListenReusePort: unsupported network %q (want tcp/tcp4/tcp6)", network)
	}
	return Listen(network, addr, ListenOptions{ReusePort: true})
}
