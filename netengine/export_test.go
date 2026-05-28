//go:build linux || darwin || freebsd || netbsd || openbsd

package netengine

import (
	"net"
	"os"
	"syscall"
)

// Exported wrappers for white-box testing of unexported functions.

var StatusLine = statusLine

func IsTemporary(err error) bool          { return isTemporary(err) }
func IsResourceExhaustion(err error) bool { return isResourceExhaustion(err) }
func ExtractErrno(err error) syscall.Errno { return extractErrno(err) }
func ConnFd(c net.Conn) (int, error)      { return connFd(c) }

// MakeOpError builds a net.OpError wrapping a syscall.Errno for testing.
func MakeOpError(errno syscall.Errno) error {
	return &net.OpError{
		Op:  "accept",
		Err: &os.SyscallError{Syscall: "accept", Err: errno},
	}
}

// WorkerPool exposes the internal workerPool for testing.
type WorkerPool = workerPool

func NewWorkerPool(size int) *WorkerPool { return newWorkerPool(size) }

func (p *WorkerPool) Submit(task func())        { p.submit(task) }
func (p *WorkerPool) TrySubmit(task func()) bool { return p.trySubmit(task) }
func (p *WorkerPool) Stop()                     { p.stop() }
