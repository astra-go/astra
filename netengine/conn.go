package netengine

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/http2"
)

// Connection ownership states for connState.state.
//
// Ownership protocol:
//
//	stateIdle       — connection is parked in the event loop's conns map,
//	                  waiting for the next readable event.  Only the event-loop
//	                  goroutine may transition out of this state.
//
//	stateDispatched — connection has been handed to a worker goroutine by a
//	                  successful CAS(idle→dispatched).  The event loop must not
//	                  touch the connState while in this state.
//
//	stateClosed     — terminal state; connection is closed or being closed.
//	                  No further transitions are valid.
const (
	stateIdle       uint32 = 0
	stateDispatched uint32 = 1
	stateClosed     uint32 = 2
)

// connState holds per-connection state across keep-alive requests.
type connState struct {
	nc   net.Conn
	fd   int
	loop *eventLoop
	br   *bufio.Reader // buffered reader reused across requests on this connection

	// registered is true once poller.add has been called for this fd.
	// New connections dispatched directly (dispatchNewDirect) start as false;
	// rearmConn calls poller.add on the first keep-alive rearm and poller.mod
	// on all subsequent rearms.
	registered atomic.Uint32

	// dispatchFn is a pre-bound method value for dispatch(), allocated once
	// per connState pointer lifetime.  Reusing the same func() across
	// keep-alive requests (and across sync.Pool reuse) eliminates the
	// per-request closure allocation that cs.dispatch would otherwise create.
	dispatchFn func()

	// state is the ownership token for this connection.
	// Transitions:
	//   accept:     (new)  → dispatched   (dispatchNewDirect, direct submit)
	//   worker:     dispatched → idle    (rearmConn, on keep-alive)
	//   worker:     dispatched → closed  (workerCloseConn, on error / no keep-alive)
	//   event-loop: idle  → dispatched   (handleEvent, via CAS)
	//   event-loop: idle  → closed       (closeConn, on hangup / error)
	//   shutdown:   idle  → closed       (eventLoop.close)
	state atomic.Uint32
}

// connStatePool recycles connState structs to eliminate the per-connection
// heap allocation.  The embedded bufio.Reader (and its 16 KiB backing array)
// are also reused: Reset(nc) replaces the underlying reader without reallocating.
var connStatePool = sync.Pool{New: func() any { return new(connState) }}

func acquireConnState(nc net.Conn, fd int, loop *eventLoop) *connState {
	cs := connStatePool.Get().(*connState)
	cs.nc = nc
	cs.fd = fd
	cs.loop = loop
	cs.registered.Store(0)
	cs.state.Store(stateIdle)
	if cs.br == nil {
		cs.br = bufio.NewReaderSize(nc, loop.engine.cfg.ReadBufferSize)
	} else {
		cs.br.Reset(nc)
	}
	// dispatchFn is a bound method value for cs.dispatch.  We create it once
	// per connState pointer (not per request) so the same func() is reused
	// for every keep-alive round-trip and across sync.Pool re-acquisition.
	if cs.dispatchFn == nil {
		cs.dispatchFn = cs.dispatch
	}
	return cs
}

func releaseConnState(cs *connState) {
	// Nil out pointers so the GC can reclaim the net.Conn and eventLoop
	// while the connState sits idle in the pool.  br and dispatchFn are kept:
	// br so its backing buffer survives for the next acquireConnState call,
	// dispatchFn so the pre-bound method value is not reallocated on reuse.
	cs.nc = nil
	cs.loop = nil
	connStatePool.Put(cs)
}

// dispatch reads one HTTP request, calls the handler, writes the response, and
// either re-arms the connection in the event loop (keep-alive) or closes it.
// It is always called from a worker pool goroutine (state = stateDispatched).
//
// For TLS connections the handshake is performed here (not in the accept loop)
// so the accept goroutine is never blocked by crypto or network latency.
// After the handshake, h2 connections are handed off to http2.Server.ServeConn
// in a new goroutine and the worker returns to the pool immediately.
func (cs *connState) dispatch() {
	if tc, ok := cs.nc.(*tls.Conn); ok {
		cfg := &cs.loop.engine.cfg
		tc.SetDeadline(time.Now().Add(cfg.TLSHandshakeTimeout))
		if err := tc.Handshake(); err != nil {
			tc.SetDeadline(time.Time{})
			cs.loop.workerCloseConn(cs)
			return
		}
		tc.SetDeadline(time.Time{})

		if h2srv := cs.loop.engine.h2srv; h2srv != nil {
			if tc.ConnectionState().NegotiatedProtocol == "h2" {
				// Release connState back to pool before handing off to h2.
				// The h2 goroutine owns the raw *tls.Conn from here on.
				cs.loop.workerCloseConn(cs)
				go func() {
					h2srv.ServeConn(tc, &http2.ServeConnOpts{
						Handler: cs.loop.engine.handler,
						Context: cs.loop.engine.ctx,
					})
				}()
				return
			}
		}
	}

	keepAlive, err := cs.handleRequest()
	if err != nil || !keepAlive {
		cs.loop.workerCloseConn(cs)
		return
	}
	// Re-arm via the event loop.  If the client already pipelined the next
	// request, the poller fires immediately because epoll/kqueue is
	// level-triggered for registered FDs with pending data.
	cs.loop.rearmConn(cs)
}

// handleRequest performs the read→handle→write cycle for one HTTP request.
func (cs *connState) handleRequest() (keepAlive bool, err error) {
	cfg := &cs.loop.engine.cfg
	if cfg.ReadTimeout > 0 {
		cs.nc.SetReadDeadline(time.Now().Add(cfg.ReadTimeout))
	}

	req, err := http.ReadRequest(cs.br)
	if err != nil {
		return false, err
	}
	cs.nc.SetReadDeadline(time.Time{})

	rw := acquireBufRW()

	// Invoke the application handler (Astra's router is an http.Handler).
	cs.loop.engine.handler.ServeHTTP(rw, req)

	// Drain any unread request body before the response so the conn is clean.
	io.Copy(io.Discard, req.Body) //nolint:errcheck
	req.Body.Close()

	keepAlive = isKeepAlive(req, rw)
	if !keepAlive {
		rw.header.Set("Connection", "close")
	}

	if cfg.WriteTimeout > 0 {
		cs.nc.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
	}
	err = rw.flushTo(cs.nc, req)
	cs.nc.SetWriteDeadline(time.Time{})

	releaseBufRW(rw)
	return keepAlive, err
}

// isKeepAlive decides whether the connection should be kept alive.
func isKeepAlive(req *http.Request, rw *bufResponseWriter) bool {
	if strings.EqualFold(rw.header.Get("Connection"), "close") {
		return false
	}
	if req.ProtoMajor == 1 && req.ProtoMinor == 0 {
		return strings.EqualFold(req.Header.Get("Connection"), "keep-alive")
	}
	return !strings.EqualFold(req.Header.Get("Connection"), "close")
}

// ─── bufResponseWriter ────────────────────────────────────────────────────────

// bufResponseWriter buffers the full response in memory so we can set
// Content-Length before writing to the wire.
type bufResponseWriter struct {
	header http.Header
	body   []byte // append-only; reset between uses
	status int
	wrote  bool
}

var bufRWPool = sync.Pool{
	New: func() any {
		return &bufResponseWriter{header: make(http.Header)}
	},
}

// statusLineCache pre-computes "200 OK", "404 Not Found", etc. once at init time.
// Indexed directly by status code (100–599): O(1) lookup with zero allocation on
// the hot path, eliminating the fmt.Sprintf + string concat that http.Response.Write
// would otherwise perform on every response.
var statusLineCache [600]string

func init() {
	for code := 100; code < 600; code++ {
		if text := http.StatusText(code); text != "" {
			statusLineCache[code] = strconv.Itoa(code) + " " + text
		}
	}
}

func statusLine(code int) string {
	if code >= 100 && code < 600 {
		if s := statusLineCache[code]; s != "" {
			return s
		}
	}
	return strconv.Itoa(code) + " " + http.StatusText(code)
}

// respBufWriterPool reuses *bufio.Writer instances across responses.
// This avoids the per-response heap allocation that http.Response.Write
// performs internally when it creates its own bufio.Writer.
// 8 KiB covers typical API response headers + most JSON bodies without spilling.
var respBufWriterPool = sync.Pool{
	New: func() any { return bufio.NewWriterSize(nil, 8*1024) },
}

func acquireBufRW() *bufResponseWriter {
	rw := bufRWPool.Get().(*bufResponseWriter)
	rw.status = http.StatusOK
	rw.wrote = false
	rw.body = rw.body[:0]
	for k := range rw.header {
		delete(rw.header, k)
	}
	return rw
}

func releaseBufRW(rw *bufResponseWriter) { bufRWPool.Put(rw) }

func (w *bufResponseWriter) Header() http.Header { return w.header }

func (w *bufResponseWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
}

func (w *bufResponseWriter) Write(b []byte) (int, error) {
	w.wrote = true
	w.body = append(w.body, b...)
	return len(b), nil
}

// flushTo serialises the buffered response directly onto the wire using a
// pooled bufio.Writer, avoiding the allocations that http.Response.Write
// would otherwise incur:
//   - no *http.Response struct allocation
//   - no fmt.Sprintf for the status line (pre-cached in statusLineCache)
//   - no string(w.body) copy (writes []byte directly)
//   - no io.NopCloser / strings.NewReader wrapper allocation
//   - no internal bufio.Writer allocation inside http.Response.Write
//
// Content-Length is always known (bufResponseWriter buffers the full body),
// so Transfer-Encoding: chunked is never needed.
func (w *bufResponseWriter) flushTo(conn net.Conn, req *http.Request) error {
	bw := respBufWriterPool.Get().(*bufio.Writer)
	bw.Reset(conn)

	// ── Status line ────────────────────────────────────────────────────────
	bw.WriteString("HTTP/1.1 ")
	bw.WriteString(statusLine(w.status))
	bw.WriteString("\r\n")

	// ── Date ───────────────────────────────────────────────────────────────
	if w.header.Get("Date") == "" {
		bw.WriteString("Date: ")
		bw.WriteString(time.Now().UTC().Format(http.TimeFormat))
		bw.WriteString("\r\n")
	}

	// ── Content-Length ─────────────────────────────────────────────────────
	// Always computable: the full body is buffered before we write headers.
	// Using strconv.AppendInt into a stack-allocated [20]byte avoids any heap
	// allocation for the decimal representation.
	var clBuf [20]byte
	bw.WriteString("Content-Length: ")
	bw.Write(strconv.AppendInt(clBuf[:0], int64(len(w.body)), 10))
	bw.WriteString("\r\n")

	// ── Application headers ────────────────────────────────────────────────
	for k, vs := range w.header {
		for _, v := range vs {
			bw.WriteString(k)
			bw.WriteString(": ")
			bw.WriteString(v)
			bw.WriteString("\r\n")
		}
	}

	// ── Header/body separator ──────────────────────────────────────────────
	bw.WriteString("\r\n")

	// ── Body ───────────────────────────────────────────────────────────────
	// RFC 7231 §4.3.2: a HEAD response MUST NOT include a message body,
	// but headers (including Content-Length) reflect what a GET would return.
	if req == nil || req.Method != "HEAD" {
		bw.Write(w.body)
	}

	err := bw.Flush()
	bw.Reset(nil) // release reference to conn before returning to pool
	respBufWriterPool.Put(bw)
	return err
}

// connFd returns the raw file descriptor of a net.Conn.
// It works for *net.TCPConn, *tls.Conn wrapping TCPConn, and any other Conn
// that exposes syscall.RawConn via SyscallConn().
func connFd(c net.Conn) (int, error) {
	type sysConner interface {
		SyscallConn() (syscall.RawConn, error)
	}
	sc, ok := c.(sysConner)
	if !ok {
		return 0, fmt.Errorf("netengine: conn does not implement SyscallConn")
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return 0, err
	}
	var fd int
	if err = raw.Control(func(f uintptr) { fd = int(f) }); err != nil {
		return 0, err
	}
	return fd, nil
}
