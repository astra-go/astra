//go:build windows

package netengine

import (
	"errors"
	"net"
	"os"
	"syscall"
)

// Winsock errno values not exported by Go's syscall package.
// Source: <winsock2.h> / MSDN "Windows Sockets Error Codes".
const (
	// _WSAEINTR (10004): a blocking Winsock call was cancelled via
	// WSACancelBlockingCall; analogous to EINTR on Unix.
	_WSAEINTR syscall.Errno = 10004

	// _WSAEMFILE (10024): too many open sockets; analogous to EMFILE on Unix.
	// Windows merges per-process and system-wide limits into a single code.
	_WSAEMFILE syscall.Errno = 10024

	// _WSAEWOULDBLOCK (10035): non-blocking operation has no data ready;
	// analogous to EAGAIN / EWOULDBLOCK on Unix.
	_WSAEWOULDBLOCK syscall.Errno = 10035

	// _WSAENOBUFS (10055): no buffer space available; analogous to ENOBUFS.
	_WSAENOBUFS syscall.Errno = 10055
)

// isTemporary reports whether err is a transient network error that the accept
// loop should silently retry without sleeping.
//
// On Windows the error chain is identical to Unix:
//
//	net.OpError → os.SyscallError → syscall.Errno
//
// but the errno values are Winsock-specific (WSAE* codes):
//
//   - WSAEWOULDBLOCK (10035): no connections ready on a non-blocking socket;
//     analogous to EAGAIN/EWOULDBLOCK on Unix.
//   - WSAEINTR (10004): blocking call interrupted by WSACancelBlockingCall;
//     analogous to EINTR on Unix — retry immediately.
//   - WSAECONNABORTED (10053): software-caused connection abort; the TCP stack
//     discarded the connection before accept returned — retry.
//     (syscall.WSAECONNABORTED is exported by Go's syscall package.)
func isTemporary(err error) bool {
	errno := extractErrno(err)
	if errno == 0 {
		return false
	}
	switch errno {
	case _WSAEWOULDBLOCK, _WSAEINTR, syscall.WSAECONNABORTED:
		return true
	}
	return false
}

// isResourceExhaustion reports whether err indicates the process or system has
// run out of OS resources for new connections.
//
// Windows Winsock equivalents:
//   - WSAEMFILE (10024): per-process socket/handle limit reached; analogous to
//     EMFILE on Unix.  Windows has no separate system-wide ENFILE equivalent —
//     a single constant covers both process and system exhaustion.
//   - WSAENOBUFS (10055): insufficient buffer space available; analogous to
//     ENOBUFS on Unix.
func isResourceExhaustion(err error) bool {
	errno := extractErrno(err)
	if errno == 0 {
		return false
	}
	switch errno {
	case _WSAEMFILE, _WSAENOBUFS:
		return true
	}
	return false
}

// extractErrno walks the net.OpError → os.SyscallError → syscall.Errno chain
// and returns the underlying Errno, or 0 if the chain cannot be resolved.
// The Windows and Unix implementations are identical; only the constants differ.
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
