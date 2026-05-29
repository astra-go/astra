package netengine

import (
	"net"
	"sync"
	"sync/atomic"
)

// eventLoop owns a single poller (epoll/kqueue) instance and manages a set of
// connections.  It runs one long-lived goroutine that calls poller.wait() to
// collect readable events and dispatches each readable connection to the shared
// worker pool.
//
// New connections are no longer fed through an addCh channel: dispatchNewDirect
// extracts the fd, creates a connState, stores it in e.conns, and submits it
// directly to the worker pool — bypassing the poller round-trip for the first
// request on every connection.  The poller is only involved for keep-alive
// connections after the first request (via rearmConn → poller.add/mod).
//
// Idle connections consume NO goroutine while parked here.
type eventLoop struct {
	id     int
	engine *Engine
	poller pollerBackend
	conns  sync.Map // fd (int) → *connState
	quit   chan struct{}
}

func newEventLoop(e *Engine, id int) (*eventLoop, error) {
	loop := &eventLoop{
		id:     id,
		engine: e,
		quit:   make(chan struct{}),
	}
	p, err := newPoller(loop)
	if err != nil {
		return nil, err
	}
	loop.poller = p
	return loop, nil
}

// dispatchNewDirect accepts a freshly accepted connection and submits it
// immediately to the worker pool without registering it with the poller first.
//
// This eliminates the previous addCh → drainAddCh → poller.add → epoll/kqueue
// event → handleEvent round-trip for new connections, cutting the cold-path
// latency from ~72 µs to ~16 µs (loopback benchmark).
//
// The connState starts with registered=false.  If the first request's response
// indicates keep-alive, rearmConn calls poller.add to register the fd; on
// all subsequent rearms it calls poller.mod.  Short-lived (Connection: close)
// connections are closed by workerCloseConn and never enter the poller.
func (e *eventLoop) dispatchNewDirect(nc net.Conn) {
	fd, err := connFd(nc)
	if err != nil {
		nc.Close()
		atomic.AddInt64(&e.engine.activeConns, -1)
		e.engine.cfg.Logger.Warn("netengine: connFd failed", "err", err)
		return
	}
	cs := acquireConnState(nc, fd, e)
	cs.state.Store(stateDispatched)
	e.conns.Store(fd, cs)

	if !e.engine.workers.trySubmit(cs.dispatchFn) {
		// Worker pool is saturated.  Send a minimal HTTP 503 response and
		// close the connection immediately so the client gets fast feedback
		// instead of hanging, and the accept loop stays unblocked.
		const shedResp = "HTTP/1.1 503 Service Unavailable\r\n" +
			"Content-Length: 0\r\n" +
			"Connection: close\r\n\r\n"
		cs.state.Store(stateClosed)
		e.conns.Delete(cs.fd)
		cs.nc.Write([]byte(shedResp)) //nolint:errcheck
		cs.nc.Close()
		atomic.AddInt64(&e.engine.activeConns, -1)
		releaseConnState(cs)
		e.engine.cfg.Logger.Warn("netengine: worker pool saturated, shed connection",
			"loop", e.id)
	}
}

// run is the event loop body.  It executes in its own goroutine.
// Its sole responsibility is waiting for I/O events on keep-alive connections
// and dispatching them to the worker pool.  New connections are injected
// directly via dispatchNewDirect and do not require an event loop iteration.
func (e *eventLoop) run() {
	events := make([]pollEvent, 512)
	for {
		select {
		case <-e.quit:
			return
		default:
		}

		n, err := e.poller.wait(events)
		if err != nil {
			// Check whether the error is the expected result of poller.close()
			// being called during shutdown.  If so, exit cleanly without logging.
			select {
			case <-e.quit:
				return
			default:
			}
			e.engine.cfg.Logger.Error("netengine: poller.wait error", "loop", e.id, "err", err)
			return
		}

		for i := range n {
			e.handleEvent(events[i])
		}
	}
}

// handleEvent is called for each event returned by poller.wait.
// All connections seen here are registered (cs.registered == true) because
// only rearmConn calls poller.add/mod.
func (e *eventLoop) handleEvent(ev pollEvent) {
	val, ok := e.conns.Load(ev.fd)
	if !ok {
		return
	}
	cs := val.(*connState)

	// IMPORTANT: check ev.readable BEFORE ev.hangup || ev.errored.
	//
	// On Linux, EPOLLIN and EPOLLRDHUP (or EPOLLHUP) can arrive in the same
	// event: the peer sent a final request and then half-closed the connection
	// (TCP FIN).  If we close first we silently discard the buffered request.
	// On kqueue, EV_EOF and EVFILT_READ exhibit the same simultaneous behaviour.
	//
	// Dispatching the worker first is safe: handleRequest reads any pending
	// data, writes the response, then sees EOF → isKeepAlive returns false →
	// the worker calls workerCloseConn.
	if ev.readable {
		// Atomically transfer ownership to the worker: CAS idle → dispatched.
		if !cs.state.CompareAndSwap(stateIdle, stateDispatched) {
			return // already dispatched or closed — drop the spurious event
		}

		if !e.engine.workers.trySubmit(cs.dispatchFn) {
			const shedResp = "HTTP/1.1 503 Service Unavailable\r\n" +
				"Content-Length: 0\r\n" +
				"Connection: close\r\n\r\n"
			cs.state.Store(stateClosed)
			e.conns.Delete(cs.fd)
			e.poller.del(cs.fd) //nolint:errcheck
			cs.nc.Write([]byte(shedResp)) //nolint:errcheck
			cs.nc.Close()
			atomic.AddInt64(&e.engine.activeConns, -1)
			releaseConnState(cs)
			e.engine.cfg.Logger.Warn("netengine: worker pool saturated, shed connection",
				"loop", e.id)
		}
		return
	}
	if ev.hangup || ev.errored {
		e.closeConn(cs)
	}
}

// rearmConn re-registers a keep-alive connection to wait for the next request.
// Called by worker pool goroutines after a successful response (state = dispatched).
//
// Ownership hand-back protocol:
//  1. Keep state = stateDispatched during the entire poller operation.
//     For registered connections EPOLLONESHOT / EV_DISPATCH guarantees the fd is
//     still disabled — no new event can fire until mod() is called.
//     For unregistered (first keep-alive rearm) the fd is not yet in the poller,
//     so no event can fire before add() returns.
//  2. Call poller.add (first rearm) or poller.mod (subsequent rearms).
//     If add/mod fails (engine shutting down), transition directly from
//     dispatched to closed and clean up.
//  3. Only after the poller operation succeeds, mark stateIdle to hand ownership
//     back to the event loop. This ensures the event loop cannot interfere with
//     the connState while the worker is still operating on it.
func (e *eventLoop) rearmConn(cs *connState) {
	var err error
	if cs.registered.Load() == 0 {
		err = e.poller.add(cs.fd)
		if err == nil {
			cs.registered.Store(1)
		}
	} else {
		err = e.poller.mod(cs.fd)
	}

	if err != nil {
		// Poller is closed (engine shutdown) or the fd is invalid.
		// Transition directly from dispatched to closed.
		if cs.state.CompareAndSwap(stateDispatched, stateClosed) {
			e.conns.Delete(cs.fd)
			cs.nc.Close()
			atomic.AddInt64(&e.engine.activeConns, -1)
			releaseConnState(cs)
		}
		return
	}

	// Poller operation succeeded. Now it's safe to mark idle.
	cs.state.Store(stateIdle)
}

// closeConn closes a connection that is currently idle (event-loop owned).
// Called from the event loop goroutine on hangup / error events.
// Connections seen here are always registered (they arrived via poller events).
func (e *eventLoop) closeConn(cs *connState) {
	if !cs.state.CompareAndSwap(stateIdle, stateClosed) {
		return // dispatched (worker will close) or already closed
	}
	e.conns.Delete(cs.fd)
	if cs.registered.Load() == 1 {
		e.poller.del(cs.fd) //nolint:errcheck
	}
	cs.nc.Close()
	atomic.AddInt64(&e.engine.activeConns, -1)
	releaseConnState(cs)
}

// workerCloseConn closes a connection that is currently dispatched (worker owned).
// Called from worker pool goroutines when the request handler returns an error
// or the response indicates no keep-alive.
func (e *eventLoop) workerCloseConn(cs *connState) {
	if !cs.state.CompareAndSwap(stateDispatched, stateClosed) {
		return // already closed (race with engine shutdown)
	}
	e.conns.Delete(cs.fd)
	if cs.registered.Load() == 1 {
		e.poller.del(cs.fd) //nolint:errcheck
	}
	cs.nc.Close()
	atomic.AddInt64(&e.engine.activeConns, -1)
	releaseConnState(cs)
}

// close shuts down the event loop.
//
// Shutdown sequence:
//  1. Signal the event loop goroutine to stop (close e.quit).
//  2. Wake up a blocking poller.wait() so the goroutine sees e.quit immediately
//     and exits cleanly — without waiting for the next I/O event or for
//     poller.close() to forcibly unblock it via EBADF.
//  3. Close idle (event-loop-owned) connections.  Dispatched connections are
//     owned by worker goroutines; those workers will fail to re-arm the poller
//     (because it is being closed) and will call workerCloseConn themselves.
//     Connections with registered=false that are currently dispatched are
//     handled the same way — workerCloseConn skips poller.del for them.
//  4. Close the poller.
func (e *eventLoop) close() {
	// ① Signal the event loop goroutine.
	close(e.quit)

	// ② Unblock a blocking poller.wait() so the goroutine can exit cleanly
	//    via the e.quit check instead of via a EBADF error from poller.close().
	e.poller.wakeup()

	// ③ Close all connections.
	//
	//    Idle connections: CAS idle→closed, fully clean up (delete from map,
	//    dec activeConns, release connState).
	//
	//    Dispatched connections (workers own them): we cannot safely do full
	//    cleanup here because the worker still holds a pointer to connState.
	//    Instead, we force-close the underlying net.Conn so the worker's
	//    in-flight Read/Write returns an error immediately.  The worker then
	//    calls workerCloseConn (or rearmConn which detects the closed poller)
	//    which performs the remaining cleanup.  This is essential during
	//    shutdown when workers may be blocked in http.ReadRequest waiting for
	//    the next keep-alive request on a connection that the client will never
	//    use again — without a forced close those workers would block until
	//    ReadTimeout (30 s) expires.
	e.conns.Range(func(_, val any) bool {
		cs := val.(*connState)
		if cs.state.CompareAndSwap(stateIdle, stateClosed) {
			if cs.registered.Load() == 1 {
				e.poller.del(cs.fd) //nolint:errcheck
			}
			cs.nc.Close()
			atomic.AddInt64(&e.engine.activeConns, -1)
			releaseConnState(cs)
		} else if cs.state.Load() == stateDispatched {
			// Force-close to unblock the worker; full cleanup is done by
			// workerCloseConn after the worker sees the I/O error.
			cs.nc.Close()
		}
		return true
	})

	// ④ Close the poller — any remaining poller.wait() call will now return
	//    an error, ensuring the event loop goroutine eventually exits even if
	//    the wakeup() in step ② was lost.
	e.poller.close() //nolint:errcheck
}
