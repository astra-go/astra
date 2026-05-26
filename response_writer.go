package astra

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/astra-go/astra/contract"
)

// ResponseWriter is re-exported from contract for backward compatibility.
// It is an enhanced http.ResponseWriter that tracks status and body size.
type ResponseWriter = contract.ResponseWriter

type responseWriter struct {
	http.ResponseWriter
	status  int
	size    int
	written bool
}

func newResponseWriter(w http.ResponseWriter) ResponseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.written {
		return
	}
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.written = true
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(rw.status)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// WriteString implements io.StringWriter.  It avoids the []byte allocation
// that would occur if callers used Write([]byte(s)) for string payloads.
// The underlying http.ResponseWriter (Go's net/http or httptest.ResponseRecorder)
// also implements io.StringWriter, so the allocation is eliminated end-to-end.
func (rw *responseWriter) WriteString(s string) (int, error) {
	if !rw.written {
		rw.WriteHeader(rw.status)
	}
	n, err := io.WriteString(rw.ResponseWriter, s)
	rw.size += n
	return n, err
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) Size() int {
	return rw.size
}

func (rw *responseWriter) Written() bool {
	return rw.written
}

// Hijack implements http.Hijacker for WebSocket support.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("astra: underlying ResponseWriter does not support hijacking")
	}
	return hijacker.Hijack()
}

// Flush implements http.Flusher for SSE support.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Push implements http.Pusher for HTTP/2 server push.
func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}
