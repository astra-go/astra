package netengine

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
)

// resourceExhaustionBackoff is the sleep duration when Accept fails due to
// EMFILE / ENFILE / ENOBUFS / ENOMEM.  A brief pause lets existing connections
// close and free descriptors; without it the accept loop would spin at 100 % CPU.
const resourceExhaustionBackoff = 5 * time.Millisecond

// ReactorConfig controls the Reactor engine.
type ReactorConfig struct {
	// NumLoops is the number of event loops (sub-reactors).
	// Each loop owns one epoll/kqueue instance.
	// Defaults to runtime.GOMAXPROCS(0).
	NumLoops int

	// WorkerPoolSize is the maximum number of goroutines that concurrently
	// handle HTTP requests.  Defaults to 4 × GOMAXPROCS.
	WorkerPoolSize int

	// ReadBufferSize is the per-connection read buffer in bytes (default: 16 KiB).
	ReadBufferSize int

	// ReadTimeout is the max duration to read a complete request (default: 30 s).
	// Zero disables the timeout.
	ReadTimeout time.Duration

	// WriteTimeout is the max duration to write a response (default: 60 s).
	// Zero disables the timeout.
	WriteTimeout time.Duration

	// ConnChannelBuffer is no longer used.
	// New connections are dispatched directly to the worker pool via
	// dispatchNewDirect, bypassing the per-loop addCh channel entirely.
	// The field is retained for backward compatibility with existing configs.
	//
	// Deprecated: has no effect.
	ConnChannelBuffer int

	// TLSHandshakeTimeout is the max duration allowed for a TLS handshake.
	// Zero uses the default of 5 seconds.
	TLSHandshakeTimeout time.Duration

	// Logger is used for internal engine messages.  Defaults to slog.Default().
	Logger *slog.Logger
}

func (c *ReactorConfig) setDefaults() {
	if c.NumLoops <= 0 {
		c.NumLoops = runtime.GOMAXPROCS(0)
	}
	if c.WorkerPoolSize <= 0 {
		c.WorkerPoolSize = 4 * runtime.GOMAXPROCS(0)
	}
	if c.ReadBufferSize <= 0 {
		c.ReadBufferSize = 16 * 1024
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 60 * time.Second
	}
	if c.ConnChannelBuffer <= 0 {
		c.ConnChannelBuffer = 1024 // kept for compatibility; not used internally
	}
	if c.TLSHandshakeTimeout <= 0 {
		c.TLSHandshakeTimeout = 5 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Engine is a Reactor-pattern HTTP server.
//
// Architecture:
//
//	Accept loop ──round-robin──► N event loops (epoll/kqueue)
//	                                     │  (on readable)
//	                                     ▼
//	                           Worker pool (P goroutines)
//	                                     │  (TLS: handshake + protocol routing)
//	                                     ├── h2 → go http2.Server.ServeConn
//	                                     └── h1 → handler.ServeHTTP(rw, req)
//
// Idle connections park in event loops and consume zero goroutines.
// Active connections borrow a worker goroutine only while handling a request.
// TLS handshakes occur in worker goroutines so the accept loop is never blocked.
type Engine struct {
	cfg         ReactorConfig
	handler     http.Handler
	h2srv       *http2.Server // non-nil when TLS h2 is enabled
	loops       []*eventLoop
	workers     *workerPool
	wsLoop      *WSEventLoop // non-nil when WebSocket event loop is enabled
	activeConns int64        // atomic counter
	loopIdx     uint64       // atomic, round-robin index
	ctx         context.Context
	cancel      context.CancelFunc
}

// New creates a Reactor engine backed by the given HTTP handler.
// Returns an error on platforms where epoll/kqueue is unavailable.
func New(handler http.Handler, cfg ReactorConfig) (*Engine, error) {
	cfg.setDefaults()
	e := &Engine{
		cfg:     cfg,
		handler: handler,
		loops:   make([]*eventLoop, cfg.NumLoops),
	}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.workers = newWorkerPool(cfg.WorkerPoolSize)

	for i := range e.loops {
		loop, err := newEventLoop(e, i)
		if err != nil {
			// Clean up already-created loops.
			for j := range i {
				e.loops[j].close()
			}
			e.workers.stop()
			return nil, err
		}
		e.loops[i] = loop
	}
	return e, nil
}

// EnableH2 configures the engine to route HTTP/2 connections (identified by
// ALPN negotiation) to an http2.Server instead of the Reactor path.
// Must be called before Serve.
func (e *Engine) EnableH2(h2srv *http2.Server) {
	e.h2srv = h2srv
}

// EnableWS enables WebSocket event loop integration.
// When enabled, WebSocket connections registered via WSEventLoop.Register()
// are polled by the Reactor engine's event loops instead of requiring
// dedicated goroutines per connection.
//
// Parameters:
//   - onMessage: called when a WebSocket message arrives; runs in worker goroutine
//   - onError: called when a read error occurs (optional, may be nil)
//   - onClose: called after a connection is fully cleaned up (optional, may be nil)
//
// Must be called before Serve.
func (e *Engine) EnableWS(onMessage func(*WSConn, int, []byte), onError func(*WSConn, error), onClose func(*WSConn)) {
	e.wsLoop = newWSEventLoop(e, onMessage, onError, onClose)
}

// WS returns the WSEventLoop for managing WebSocket connections.
// Returns nil if EnableWS has not been called.
func (e *Engine) WS() *WSEventLoop {
	return e.wsLoop
}

// Serve starts all event loops and then blocks accepting connections from ln.
// It returns only when ln is closed (or an unrecoverable accept error occurs).
func (e *Engine) Serve(ln net.Listener) error {
	// Start event loop goroutines.
	var wg sync.WaitGroup
	for _, loop := range e.loops {
		wg.Go(loop.run)
	}

	// Accept loop: round-robin connections across event loops.
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Distinguish closed listener (normal shutdown) from real errors.
			select {
			case <-e.quitCh():
				// Engine was closed; drain loops and return.
				for _, loop := range e.loops {
					loop.close()
				}
				wg.Wait()
				e.workers.stop()
				return nil
			default:
			}
			// EMFILE / ENFILE / ENOBUFS / ENOMEM: OS ran out of resources.
			// Sleep briefly to let existing connections close and free
			// descriptors, then retry.  Without the sleep the loop would spin
			// at 100 % CPU until resources recover.
			if isResourceExhaustion(err) {
				e.cfg.Logger.Warn("netengine: accept: resource exhaustion, backing off",
					"err", err, "backoff", resourceExhaustionBackoff)
				time.Sleep(resourceExhaustionBackoff)
				continue
			}
			// EAGAIN / EINTR / ECONNABORTED: transient; retry immediately.
			if isTemporary(err) {
				continue
			}
			for _, loop := range e.loops {
				loop.close()
			}
			wg.Wait()
			e.workers.stop()
			return err
		}
		atomic.AddInt64(&e.activeConns, 1)
		// loopIdx 用于取模运算，结果范围受限于 loops 数量，不会溢出
		idx := int(atomic.AddUint64(&e.loopIdx, 1)-1) % len(e.loops)
		e.loops[idx].dispatchNewDirect(conn)
	}
}

// Close signals the engine to shut down.  Existing connections are closed.
// You normally close the net.Listener instead; Close is for emergency shutdown.
func (e *Engine) Close() {
	e.cancel() // Cancel the context to release resources
	for _, loop := range e.loops {
		loop.close()
	}
	e.workers.stop()
}

// ActiveConns returns the current number of open connections.
func (e *Engine) ActiveConns() int64 {
	return atomic.LoadInt64(&e.activeConns)
}

// NumLoops returns the number of event loops.
func (e *Engine) NumLoops() int { return len(e.loops) }

// WorkerPoolSize returns the configured worker pool size.
func (e *Engine) WorkerPoolSize() int { return e.cfg.WorkerPoolSize }

// PoolSnapshot returns a point-in-time snapshot of the worker pool metrics.
// If the engine was created without a worker pool (fallback mode), it returns
// a zero-valued snapshot.
func (e *Engine) PoolSnapshot() PoolMetricsSnapshot {
	if e.workers != nil {
		return e.workers.Metrics()
	}
	return PoolMetricsSnapshot{}
}

// quitCh returns a channel that is never closed (placeholder for future
// graceful-shutdown integration via context).
func (e *Engine) quitCh() <-chan struct{} {
	return make(chan struct{}) // never fires; Serve exits via listener close
}

