//go:build linux || darwin || freebsd || netbsd || openbsd

package netengine

import (
	"errors"
	"net"
	"os"
	"syscall"
)

// isTemporary reports whether err is a transient network error that the accept
// loop should silently retry without sleeping.
//
// Go 1.21 deprecated net.Error.Temporary() (it always returns false), so we
// inspect the error chain directly via errors.As to reach the raw syscall.Errno:
//
//   net.OpError → os.SyscallError → syscall.Errno
//
// Errors handled:
//   - EAGAIN / EWOULDBLOCK: no connections ready (should not fire on blocking
//     sockets, but present on some BSDs under high load)
//   - EINTR: the accept syscall was interrupted by a signal; retry immediately
//   - ECONNABORTED: the client sent a RST between connect(2) and accept(2);
//     the OS discarded the SYN but we must retry to serve the next connection
func isTemporary(err error) bool {
	errno := extractErrno(err)
	if errno == 0 {
		return false
	}
	// syscall.EAGAIN == syscall.EWOULDBLOCK on Linux and macOS (same value),
	// so list only EAGAIN to avoid duplicate-case compile errors on platforms
	// where the two constants resolve to the same integer.
	switch errno {
	case syscall.EAGAIN, syscall.EINTR, syscall.ECONNABORTED:
		return true
	}
	return false
}

// isResourceExhaustion reports whether err indicates the process or system has
// run out of OS resources for new connections.  The accept loop should sleep
// briefly (to let existing connections close and free descriptors) and then
// retry — giving up entirely would leave the server dead.
//
// Errors handled:
//   - EMFILE: this process's per-process open-file-descriptor limit is full
//   - ENFILE: the system-wide open-file-descriptor limit is full
//   - ENOBUFS: the socket buffer pool is exhausted (Linux / BSDs)
//   - ENOMEM: the kernel cannot allocate memory for a new socket struct
func isResourceExhaustion(err error) bool {
	errno := extractErrno(err)
	if errno == 0 {
		return false
	}
	switch errno {
	case syscall.EMFILE, syscall.ENFILE, syscall.ENOBUFS, syscall.ENOMEM:
		return true
	}
	return false
}

// extractErrno walks the net.OpError → os.SyscallError → syscall.Errno chain
// and returns the underlying Errno, or 0 if the chain cannot be resolved.
func extractErrno(err error) syscall.Errno {
	var ne *net.OpError
	if !errors.As(err, &ne) {
		return 0
	}
	var se *os.SyscallError
	if !errors.As(ne.Err, &se) {
		return 0
	}
	if errno, ok := se.Err.(syscall.Errno); ok {
		return errno
	}
	return 0
}
