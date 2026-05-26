//go:build windows

package netengine

import (
	"errors"
	"sync"
)

// errPollerClosed is returned by wait() when the poller has been closed.
var errPollerClosed = errors.New("netengine: windows poller closed")

// windowsPoller implements the pollerBackend interface for Windows using a
// goroutine-watcher approach.
//
// Background: Windows I/O Completion Ports (IOCP) are the native async I/O
// primitive on Windows.  However, Go's runtime already associates every net.Conn
// with its own internal IOCP port managed by the scheduler.  Associating the same
// SOCKET with a second IOCP raises ERROR_INVALID_PARAMETER.
//
// This implementation replicates one-shot readiness semantics at the Go layer:
//   - add(fd)  spawns one watcher goroutine per connection.
//   - The watcher goroutine blocks in bufio.Peek(1) — Go's scheduler parks it
//     on the runtime's IOCP until data arrives, consuming no OS thread.
//   - On data arrival the goroutine posts a pollEvent to a shared channel and
//     exits (one-shot: the goroutine is not reused for the next request).
//   - mod(fd)  spawns a fresh watcher goroutine for the keep-alive round-trip.
//   - wait()   reads from the ready channel, returning one batch of events.
//   - wakeup() unblocks a blocking wait() so the event loop can process addCh.
//   - close()  signals all watchers to exit via the loop's quit channel; watcher
//     goroutines observe the close when the connection is shut down or Peek
//     returns because nc.Close() was called during eventLoop.close().
//
// Memory model: at most one watcher goroutine per live connection; watchers are
// Go-scheduler-parked (not spinning), so their cost is one goroutine stack
// (~2 KB, grown on demand) rather than an OS thread.  The overall concurrency
// model (accept loop → N event-loop goroutines + M watcher goroutines → P
// workers) remains identical to the epoll/kqueue paths; only the idle-connection
// goroutine count differs: epoll/kqueue = O(1), Windows = O(active connections).
type windowsPoller struct {
	loop    *eventLoop          // back-reference: gives access to conns map and quit channel
	readyCh  chan pollEvent     // watcher goroutines post events here
	wakeupCh chan struct{}      // buffered(1): used by wakeup() to unblock wait()
	stopCh   chan struct{}      // closed by close(): signals watchers to exit

	mu    sync.Mutex
	armed map[int]struct{} // fds with an active (running) watcher goroutine
}

func newPoller(loop *eventLoop) (pollerBackend, error) {
	return &windowsPoller{
		loop:     loop,
		readyCh:  make(chan pollEvent, 4096),
		wakeupCh: make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
		armed:    make(map[int]struct{}),
	}, nil
}

// add registers fd and starts a one-shot watcher goroutine.
// Called from the event loop goroutine after the connState is stored in conns.
func (p *windowsPoller) add(fd int) error {
	p.mu.Lock()
	p.armed[fd] = struct{}{}
	p.mu.Unlock()
	go p.watch(fd)
	return nil
}

// mod re-arms fd for the next keep-alive request by spawning a new watcher
// goroutine.  This preserves one-shot semantics: exactly one goroutine monitors
// a given fd at any instant.
func (p *windowsPoller) mod(fd int) error {
	p.mu.Lock()
	_, already := p.armed[fd]
	if !already {
		p.armed[fd] = struct{}{}
	}
	p.mu.Unlock()
	if !already {
		go p.watch(fd)
	}
	return nil
}

// del removes fd from the armed set.  Any in-flight watcher goroutine will
// post its event but handleEvent will find the conn absent from e.conns
// (deleted by closeConn / workerCloseConn before del is called) and silently
// discard it.
func (p *windowsPoller) del(fd int) error {
	p.mu.Lock()
	delete(p.armed, fd)
	p.mu.Unlock()
	return nil
}

// watch is the per-connection watcher goroutine.
//
// It blocks in cs.br.Peek(1) until one of three conditions holds:
//  1. Data arrives   → post {fd, readable:true}
//  2. Peer closed    → post {fd, hangup:true}
//  3. Engine is shutting down (loop.quit closed, nc already closed by
//     eventLoop.close step ④) → exit without posting
//
// After posting the event the goroutine exits; mod(fd) starts a fresh one
// for the next request.
func (p *windowsPoller) watch(fd int) {
	// Retrieve the connState.  It must already be present because
	// event_loop.registerConn stores it before calling poller.add(fd).
	val, ok := p.loop.conns.Load(fd)
	if !ok {
		p.mu.Lock()
		delete(p.armed, fd)
		p.mu.Unlock()
		return
	}
	cs := val.(*connState)

	// Block here.  Go's runtime IOCP parks this goroutine on the scheduler's
	// internal IOCP until the socket becomes readable — no OS thread consumed.
	_, err := cs.br.Peek(1)

	// Remove from armed before any channel operation so that a concurrent
	// mod(fd) call spawns a fresh goroutine rather than treating this one as
	// still running.
	p.mu.Lock()
	delete(p.armed, fd)
	p.mu.Unlock()

	// If the engine is shutting down (loop.quit closed), exit without posting.
	// eventLoop.close() closes e.quit (step ①) before closing connections
	// (step ④), so this check reliably catches the shutdown path.
	select {
	case <-p.loop.quit:
		return
	default:
	}

	ev := pollEvent{fd: fd}
	if err != nil {
		// Peek returned an error: either the peer closed the connection (EOF /
		// WSAECONNRESET) or nc.Close() was called for an unrelated reason.
		ev.hangup = true
	} else {
		ev.readable = true
	}

	// Post the event.  If the poller is being closed concurrently, the stopCh
	// arm allows a clean exit rather than blocking on a full readyCh.
	select {
	case p.readyCh <- ev:
	case <-p.stopCh:
	}
}

// wait blocks until at least one event is available and returns a batch.
// It returns (0, nil) when unblocked by wakeup() (caller re-checks addCh).
// It returns errPollerClosed when close() has been called.
func (p *windowsPoller) wait(events []pollEvent) (int, error) {
	select {
	case <-p.stopCh:
		return 0, errPollerClosed

	case <-p.wakeupCh:
		// Unblocked by wakeup(): drain any immediately-ready events so the
		// caller processes them in the same iteration.
		return p.drain(events, 0), nil

	case ev := <-p.readyCh:
		events[0] = ev
		return p.drain(events, 1), nil
	}
}

// drain non-blockingly harvests up to len(events) events from readyCh,
// starting from index start (which may already contain one event).
func (p *windowsPoller) drain(events []pollEvent, start int) int {
	n := start
	for n < len(events) {
		select {
		case ev := <-p.readyCh:
			events[n] = ev
			n++
		default:
			return n
		}
	}
	return n
}

// wakeup unblocks a blocking wait() call from another goroutine (the accept
// loop uses this after enqueuing a new connection onto addCh).
func (p *windowsPoller) wakeup() {
	select {
	case p.wakeupCh <- struct{}{}:
	default: // already pending; no-op
	}
}

// close signals the poller to stop.  Watcher goroutines that are blocked in
// Peek(1) will be unblocked by eventLoop.close() calling nc.Close() (step ④),
// and will then observe loop.quit closed (step ①) and exit cleanly.
func (p *windowsPoller) close() error {
	close(p.stopCh)
	return nil
}
