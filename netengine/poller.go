// Package netengine implements a Reactor-pattern HTTP server using IO multiplexing
// (epoll on Linux, kqueue on macOS/BSDs) to handle thousands of concurrent connections
// with a fixed, small pool of goroutines.
//
// Design
//
// Unlike the standard net/http which spawns one goroutine per connection, netengine
// uses three concurrency layers:
//
//  1. Accept loop   – one goroutine accepts connections and distributes them
//                     round-robin to event loops.
//  2. Event loops   – N goroutines (N = GOMAXPROCS), each owning an epoll/kqueue
//                     instance.  Idle connections sit here consuming NO goroutine.
//                     When a connection becomes readable the event loop dispatches
//                     it to the worker pool.
//  3. Worker pool   – P goroutines (P = 4×GOMAXPROCS by default) execute the full
//                     HTTP read→route→write cycle.
//
// A connection only occupies a worker goroutine while an actual request is in
// flight.  Between requests the connection is parked in the event loop at zero
// goroutine cost.
package netengine

// pollEvent is the normalised event returned by the platform poller.
type pollEvent struct {
	fd      int
	readable bool
	hangup  bool
	errored bool
}

// pollerBackend is the platform-specific IO multiplexer interface.
// Implementations (epoll, kqueue) are in the build-tag files.
type pollerBackend interface {
	// add registers fd for one-shot read notification.
	add(fd int) error
	// mod re-arms a previously fired (one-shot) fd.
	mod(fd int) error
	// del unregisters fd.
	del(fd int) error
	// wait blocks until at least one event fires and populates events[:n].
	// It drains the wakeup pipe internally before returning.
	wait(events []pollEvent) (int, error)
	// wakeup interrupts a blocking wait call from another goroutine.
	wakeup()
	// close releases the poller's OS resources.
	close() error
}
