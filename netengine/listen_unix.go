//go:build linux || darwin || freebsd || netbsd || openbsd

package netengine

import (
	"context"
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// ListenOptions configures socket-level options applied when creating a TCP
// listener via Listen.  The zero value is safe: no special options are set.
type ListenOptions struct {
	// ReusePort enables SO_REUSEPORT so that multiple independent listeners
	// (in separate goroutines or processes) can bind the same address.
	// The OS kernel load-balances incoming connections across all active
	// listeners.  This is the building block for Prefork deployments.
	ReusePort bool

	// FastOpen enables TCP Fast Open (TFO) on the listening socket.
	// TFO allows data to be sent in the initial SYN exchange, eliminating
	// one full round-trip for repeat clients and reducing tail latency.
	//
	// Requires Linux ≥ 3.6 or macOS ≥ 10.11.  On unsupported platforms
	// Listen returns an error; callers should treat this as non-fatal and
	// fall back to a plain listener.
	FastOpen bool

	// FastOpenQueueLen is the TFO pending-connection queue depth.
	//   - Linux: the kernel TFO backlog for this socket (default 256).
	//   - macOS:  the kernel ignores this value; any non-zero value enables TFO.
	// Has no effect when FastOpen is false.
	FastOpenQueueLen int
}

// Listen creates a TCP listener on addr with the requested socket options.
// network must be "tcp", "tcp4", or "tcp6".
//
// For the common SO_REUSEPORT-only case, ListenReusePort is a simpler wrapper.
func Listen(network, addr string, opts ListenOptions) (net.Listener, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("netengine: Listen: unsupported network %q (want tcp/tcp4/tcp6)", network)
	}

	queueLen := opts.FastOpenQueueLen
	if opts.FastOpen && queueLen <= 0 {
		queueLen = 256
	}

	lc := net.ListenConfig{
		Control: func(_, _ string, c syscall.RawConn) error {
			var firstErr error
			ctrlErr := c.Control(func(fd uintptr) {
				if opts.ReusePort {
					if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil && firstErr == nil {
						firstErr = fmt.Errorf("netengine: SO_REUSEPORT: %w", err)
					}
				}
				if opts.FastOpen {
					if err := setSockOptTFO(fd, queueLen); err != nil && firstErr == nil {
						firstErr = fmt.Errorf("netengine: TCP_FASTOPEN: %w", err)
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

// ListenReusePort creates a TCP listener on addr with SO_REUSEPORT enabled.
//
// Multiple independent processes (or goroutines) may call ListenReusePort with
// the same addr; the OS kernel load-balances incoming connections across all
// active listeners.  This is the building block for Prefork-style deployments
// where each worker process runs its own Engine:
//
//	// In each child process:
//	ln, err := netengine.ListenReusePort("tcp", ":8080")
//	if err != nil { log.Fatal(err) }
//	engine.Serve(ln)
//
// Prefork reduces per-process GC heap size because each process handles only
// a fraction of total traffic, keeping the working set smaller and therefore
// STW pause times shorter.  On modern Go (1.14+) with sub-millisecond GC
// pauses, Prefork is rarely necessary; it is most useful when P99 latency
// must be kept under 1 ms at very high concurrency.
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
