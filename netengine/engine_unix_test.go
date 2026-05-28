//go:build linux || darwin || freebsd || netbsd || openbsd

package netengine_test

import (
	"errors"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/astra-go/astra/netengine"
)

// ─── statusLine ───────────────────────────────────────────────────────────────

func TestStatusLine_KnownCodes(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{200, "200 OK"},
		{404, "404 Not Found"},
		{500, "500 Internal Server Error"},
		{201, "201 Created"},
	}
	for _, tc := range cases {
		got := netengine.StatusLine(tc.code)
		if got != tc.want {
			t.Errorf("StatusLine(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestStatusLine_UnknownCode(t *testing.T) {
	// Code outside 100-599 range — falls through to strconv path.
	got := netengine.StatusLine(999)
	if got != "999 " {
		t.Errorf("StatusLine(999) = %q, want %q", got, "999 ")
	}
}

// ─── extractErrno / isTemporary / isResourceExhaustion ───────────────────────

func TestExtractErrno_NonOpError_ReturnsZero(t *testing.T) {
	if got := netengine.ExtractErrno(errors.New("plain error")); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestExtractErrno_OpErrorWithErrno(t *testing.T) {
	err := netengine.MakeOpError(syscall.EAGAIN)
	if got := netengine.ExtractErrno(err); got != syscall.EAGAIN {
		t.Errorf("expected EAGAIN, got %d", got)
	}
}

func TestIsTemporary_EAGAIN(t *testing.T) {
	err := netengine.MakeOpError(syscall.EAGAIN)
	if !netengine.IsTemporary(err) {
		t.Error("EAGAIN should be temporary")
	}
}

func TestIsTemporary_EINTR(t *testing.T) {
	err := netengine.MakeOpError(syscall.EINTR)
	if !netengine.IsTemporary(err) {
		t.Error("EINTR should be temporary")
	}
}

func TestIsTemporary_ECONNABORTED(t *testing.T) {
	err := netengine.MakeOpError(syscall.ECONNABORTED)
	if !netengine.IsTemporary(err) {
		t.Error("ECONNABORTED should be temporary")
	}
}

func TestIsTemporary_NonTransient(t *testing.T) {
	err := netengine.MakeOpError(syscall.EBADF)
	if netengine.IsTemporary(err) {
		t.Error("EBADF should not be temporary")
	}
}

func TestIsTemporary_NonOpError(t *testing.T) {
	if netengine.IsTemporary(errors.New("plain")) {
		t.Error("plain error should not be temporary")
	}
}

func TestIsResourceExhaustion_EMFILE(t *testing.T) {
	err := netengine.MakeOpError(syscall.EMFILE)
	if !netengine.IsResourceExhaustion(err) {
		t.Error("EMFILE should be resource exhaustion")
	}
}

func TestIsResourceExhaustion_ENFILE(t *testing.T) {
	err := netengine.MakeOpError(syscall.ENFILE)
	if !netengine.IsResourceExhaustion(err) {
		t.Error("ENFILE should be resource exhaustion")
	}
}

func TestIsResourceExhaustion_ENOBUFS(t *testing.T) {
	err := netengine.MakeOpError(syscall.ENOBUFS)
	if !netengine.IsResourceExhaustion(err) {
		t.Error("ENOBUFS should be resource exhaustion")
	}
}

func TestIsResourceExhaustion_ENOMEM(t *testing.T) {
	err := netengine.MakeOpError(syscall.ENOMEM)
	if !netengine.IsResourceExhaustion(err) {
		t.Error("ENOMEM should be resource exhaustion")
	}
}

func TestIsResourceExhaustion_NonExhaustion(t *testing.T) {
	err := netengine.MakeOpError(syscall.EAGAIN)
	if netengine.IsResourceExhaustion(err) {
		t.Error("EAGAIN should not be resource exhaustion")
	}
}

func TestIsResourceExhaustion_NonOpError(t *testing.T) {
	if netengine.IsResourceExhaustion(errors.New("plain")) {
		t.Error("plain error should not be resource exhaustion")
	}
}

// ─── connFd ───────────────────────────────────────────────────────────────────

func TestConnFd_TCPConn_ReturnsValidFd(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c, _ := ln.Accept()
		if c != nil {
			c.Close()
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	fd, err := netengine.ConnFd(conn)
	if err != nil {
		t.Errorf("ConnFd: %v", err)
	}
	if fd <= 0 {
		t.Errorf("expected positive fd, got %d", fd)
	}
	<-done
}

func TestConnFd_NonSyscallConn_ReturnsError(t *testing.T) {
	// A mock conn that does not implement SyscallConn.
	_, err := netengine.ConnFd(&mockConn{})
	if err == nil {
		t.Error("expected error for conn without SyscallConn")
	}
}

// mockConn implements net.Conn but not SyscallConn.
type mockConn struct{}

func (m *mockConn) Read(_ []byte) (int, error)         { return 0, nil }
func (m *mockConn) Write(_ []byte) (int, error)        { return 0, nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *mockConn) SetDeadline(_ time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(_ time.Time) error { return nil }

// ─── Engine.EnableH2 ─────────────────────────────────────────────────────────

func TestEngine_EnableH2_DoesNotPanic(t *testing.T) {
	eng, err := netengine.New(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), netengine.ReactorConfig{NumLoops: 1})
	if err != nil {
		t.Skipf("netengine.New: %v", err)
	}
	// EnableH2 with nil is a no-op but must not panic.
	eng.EnableH2(nil)
}

// ─── workerPool ───────────────────────────────────────────────────────────────

func TestWorkerPool_Submit_ExecutesTask(t *testing.T) {
	p := netengine.NewWorkerPool(2)
	defer p.Stop()

	done := make(chan struct{}, 1)
	p.Submit(func() { done <- struct{}{} })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("task was not executed within 1s")
	}
}

func TestWorkerPool_TrySubmit_ReturnsTrueWhenCapacity(t *testing.T) {
	p := netengine.NewWorkerPool(4)
	defer p.Stop()

	ok := p.TrySubmit(func() {})
	if !ok {
		t.Error("TrySubmit should return true when pool has capacity")
	}
}

func TestWorkerPool_TrySubmit_ReturnsFalseWhenFull(t *testing.T) {
	// Pool size 1, buffer = size*2 = 2. Block the worker so the buffer fills.
	p := netengine.NewWorkerPool(1)
	defer p.Stop()

	block := make(chan struct{})
	// Fill the worker and the buffer.
	for i := 0; i < 3; i++ {
		p.TrySubmit(func() { <-block })
	}
	// Now the channel should be full — TrySubmit must return false.
	ok := p.TrySubmit(func() {})
	close(block)
	if ok {
		t.Error("TrySubmit should return false when pool is saturated")
	}
}
