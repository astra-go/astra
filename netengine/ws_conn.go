package netengine

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
)

// WSEventLoop manages long-lived WebSocket connections within the Reactor
// engine.  Instead of spawning two goroutines per connection (readPump +
// writePump), it parks idle connections in the event loop's poller and only
// dispatches a worker when data arrives — the same zero-goroutine-idle model
// used for HTTP keep-alive connections.
//
// Architecture:
//
//	WS upgrade → WSEventLoop.Register() → poller watches fd
//	                                          │ (readable event)
//	                                          ▼
//	                                    Worker pool goroutine
//	                                    calls WSConn.OnMessage
//	                                          │
//	                                    Still active? → rearm in poller
//	                                    Closed?      → cleanup
//
// This design supports 100K+ concurrent WebSocket connections with a fixed
// pool of goroutines, compared to ~2 goroutines per connection with the
// gorilla/websocket Hub pattern.
type WSEventLoop struct {
	engine *Engine
	conns  sync.Map // fd (int) → *WSConn

	// onMessage is called in a worker pool goroutine when a WebSocket
	// connection receives data.  The callback must not block; offload
	// any long-running work to another goroutine.
	onMessage func(conn *WSConn, msgType int, data []byte)

	// onError is called when a WebSocket connection encounters an error.
	// After this call the connection is automatically unregistered.
	onError func(conn *WSConn, err error)

	// onClose is called after a connection is fully cleaned up.
	onClose func(conn *WSConn)

	activeConns int64 // atomic counter
}

// WSConnState tracks the ownership state of a WebSocket connection within
// the Reactor engine.
type WSConnState uint32

const (
	// wsIdle indicates the connection is parked in the event loop, waiting
	// for the next readable event.  Only the event-loop goroutine may
	// transition out of this state.
	wsIdle WSConnState = iota

	// wsDispatched indicates the connection has been handed to a worker
	// goroutine.  The event loop must not touch the WSConn while in this
	// state.
	wsDispatched

	// wsClosed indicates the connection is closed or being closed.
	// No further transitions are valid.
	wsClosed
)

// WSConn represents a WebSocket connection managed by the Reactor engine.
// After calling WSEventLoop.Register(), the connection is parked in the
// event loop's poller.  When data arrives, OnMessage is called in a worker
// goroutine; the application never needs to manage read/write goroutines.
type WSConn struct {
	// Conn is the underlying gorilla WebSocket connection.
	// It is safe to call WriteMessage / WriteJSON from any goroutine
	// (WSConn serialises writes internally).
	// ReadMessage must NOT be called by the application — the Reactor
	// engine calls it automatically when data arrives.

	// Meta holds arbitrary key-value data set by the application
	// (e.g. user ID, room ID).
	Meta map[string]any

	// fd is the raw file descriptor, used as the key in sync.Map.
	fd   int
	loop *eventLoop
	ws   *WSEventLoop

	// state tracks ownership: wsIdle → wsDispatched → wsClosed
	state atomic.Uint32

	// writeMu serialises concurrent WriteMessage calls.
	writeMu sync.Mutex

	// netConn is the underlying net.Conn (needed for poller rearm).
	netConn net.Conn
}

// newWSEventLoop creates a WSEventLoop bound to the given Engine.
func newWSEventLoop(e *Engine, onMessage func(*WSConn, int, []byte), onError func(*WSConn, error), onClose func(*WSConn)) *WSEventLoop {
	return &WSEventLoop{
		engine:    e,
		onMessage: onMessage,
		onError:   onError,
		onClose:   onClose,
	}
}

// Register adds a WebSocket connection to the event loop for polling.
// The connection's underlying net.Conn fd is registered with the poller;
// when data arrives, onMessage is called in a worker goroutine.
//
// The caller is responsible for performing the WebSocket upgrade before
// calling Register.  After this call, the caller must NOT read from the
// connection — the Reactor engine owns the read path.
//
// Returns the registered *WSConn for application use.
func (w *WSEventLoop) Register(nc net.Conn) (*WSConn, error) {
	fd, err := connFd(nc)
	if err != nil {
		nc.Close()
		return nil, err
	}

	// #nosec G115 - loopIdx 用于取模运算，结果范围受限于 loops 数量，不会溢出
	idx := int(atomic.AddUint64(&w.engine.loopIdx, 1)-1) % len(w.engine.loops)
	loop := w.engine.loops[idx]

	conn := &WSConn{
		fd:      fd,
		loop:    loop,
		ws:      w,
		netConn: nc,
		Meta:    make(map[string]any),
	}
	conn.state.Store(uint32(wsIdle))

	w.conns.Store(fd, conn)
	atomic.AddInt64(&w.activeConns, 1)

	// Register fd with the event loop's poller for one-shot read notification.
	if err := loop.poller.add(fd); err != nil {
		w.conns.Delete(fd)
		atomic.AddInt64(&w.activeConns, -1)
		nc.Close()
		return nil, err
	}

	return conn, nil
}

// Unregister removes a WebSocket connection from the event loop and closes it.
// It is safe to call from any goroutine, including from within OnMessage.
func (w *WSEventLoop) Unregister(conn *WSConn) {
	if !conn.state.CompareAndSwap(uint32(wsIdle), uint32(wsClosed)) &&
		!conn.state.CompareAndSwap(uint32(wsDispatched), uint32(wsClosed)) {
		return // already closed
	}

	w.conns.Delete(conn.fd)
	conn.loop.poller.del(conn.fd) //nolint:errcheck
	conn.netConn.Close()
	atomic.AddInt64(&w.activeConns, -1)

	if w.onClose != nil {
		w.onClose(conn)
	}
}

// ActiveConns returns the number of currently managed WebSocket connections.
func (w *WSEventLoop) ActiveConns() int64 {
	return atomic.LoadInt64(&w.activeConns)
}

// dispatchWSRead is the worker function that reads from a WebSocket connection
// when a readable event fires.  It is called from the worker pool.
func (w *WSEventLoop) dispatchWSRead(conn *WSConn) {
	// We use a non-blocking read approach: read one message, then rearm.
	// This prevents a single slow connection from monopolising a worker.
	msgType, data, err := readWSMessage(conn.netConn)
	if err != nil {
		if w.onError != nil {
			w.onError(conn, err)
		}
		w.Unregister(conn)
		return
	}

	if w.onMessage != nil {
		w.onMessage(conn, msgType, data)
	}

	// If the connection was closed by the application during OnMessage, skip rearm.
	if conn.state.Load() == uint32(wsClosed) {
		return
	}

	// Rearm the connection in the poller for the next read event.
	if !conn.state.CompareAndSwap(uint32(wsDispatched), uint32(wsIdle)) {
		return // already closed or in unexpected state
	}
	if err := conn.loop.poller.mod(conn.fd); err != nil {
		// Poller is closed or fd is invalid — clean up.
		if conn.state.CompareAndSwap(uint32(wsIdle), uint32(wsClosed)) {
			w.conns.Delete(conn.fd)
			conn.netConn.Close()
			atomic.AddInt64(&w.activeConns, -1)
			if w.onClose != nil {
				w.onClose(conn)
			}
		}
	}
}

// readWSMessage reads a single WebSocket message from the connection.
// This is a platform-agnostic read that works with any WebSocket library.
// The actual implementation is provided by the websocket package via a
// callback registered at engine creation time.
var readWSMessage func(nc net.Conn) (msgType int, data []byte, err error)

// RegisterWSReader sets the global WebSocket message reader function.
// This must be called once during initialization (before any WS connections
// are registered), typically from the websocket package's init().
func RegisterWSReader(fn func(nc net.Conn) (msgType int, data []byte, err error)) {
	readWSMessage = fn
}

// handleWSEvent is called by the event loop when a readable event fires on a
// WebSocket connection's fd.  It dispatches the read to the worker pool.
func (w *WSEventLoop) handleWSEvent(fd int) {
	val, ok := w.conns.Load(fd)
	if !ok {
		return
	}
	conn := val.(*WSConn)

	if !conn.state.CompareAndSwap(uint32(wsIdle), uint32(wsDispatched)) {
		return // already dispatched or closed
	}

	if !w.engine.workers.trySubmit(func() {
		w.dispatchWSRead(conn)
	}) {
		// Worker pool saturated — rearm the connection so we can try again
		// on the next event loop iteration, rather than dropping it.
		conn.state.CompareAndSwap(uint32(wsDispatched), uint32(wsIdle)) //nolint:errcheck
		// Try to re-register with poller; if that fails too, close the conn.
		if err := conn.loop.poller.mod(conn.fd); err != nil {
			if conn.state.CompareAndSwap(uint32(wsIdle), uint32(wsClosed)) {
				w.conns.Delete(conn.fd)
				conn.netConn.Close()
				atomic.AddInt64(&w.activeConns, -1)
				if w.onClose != nil {
					w.onClose(conn)
				}
			}
		}
	}
}

// WriteMessage sends a WebSocket message on the connection.
// It is safe to call from any goroutine; writes are serialised internally.
// This is a convenience wrapper; for library-specific write methods, use
// the WSConn's underlying connection directly with appropriate locking.
func (c *WSConn) WriteMessage(msgType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.state.Load() == uint32(wsClosed) {
		return errors.New("websocket: connection closed")
	}

	// Delegate to the registered write function.
	if writeWSMessage != nil {
		return writeWSMessage(c.netConn, msgType, data)
	}
	return errors.New("websocket: no write function registered")
}

// Close unregisters the connection from the event loop and closes it.
func (c *WSConn) Close() {
	c.ws.Unregister(c)
}

// IsClosed reports whether the connection has been closed.
func (c *WSConn) IsClosed() bool {
	return c.state.Load() == uint32(wsClosed)
}

// SetMeta stores a key-value pair in the connection's metadata.
func (c *WSConn) SetMeta(key string, value any) {
	c.Meta[key] = value
}

// GetMeta retrieves a value from the connection's metadata.
func (c *WSConn) GetMeta(key string) (any, bool) {
	v, ok := c.Meta[key]
	return v, ok
}

// writeWSMessage is the global WebSocket write function, set by the websocket
// package during initialization.
var writeWSMessage func(nc net.Conn, msgType int, data []byte) error

// RegisterWSWriter sets the global WebSocket message writer function.
func RegisterWSWriter(fn func(nc net.Conn, msgType int, data []byte) error) {
	writeWSMessage = fn
}
