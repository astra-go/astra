package netengine

import (
	"sync"
	"sync/atomic"
)

// paddedInt64 is an atomic.Int64 padded to a full 64-byte cache line.
// Placing each counter on its own line eliminates false sharing when multiple
// goroutines increment different counters concurrently.
type paddedInt64 struct {
	v atomic.Int64
	_ [56]byte // pad to 64 bytes (cache line size on x86-64 and ARM64)
}

// PoolMetrics holds atomic counters for worker pool statistics.
// Each counter is padded to its own cache line to prevent false sharing.
type PoolMetrics struct {
	Submitted     paddedInt64 // successfully submitted tasks
	Rejected      paddedInt64 // tasks rejected by trySubmit (pool saturated)
	Completed     paddedInt64 // tasks that finished execution
	QueueLen      paddedInt64 // approximate number of tasks waiting in queue
	ActiveWorkers paddedInt64 // workers currently executing a task
}

// PoolMetricsSnapshot is a point-in-time copy of PoolMetrics.
type PoolMetricsSnapshot struct {
	Submitted     int64
	Rejected      int64
	Completed     int64
	QueueLen      int64
	ActiveWorkers int64
}

// workerPool is a bounded pool of goroutines that execute tasks (HTTP request
// handling).  When all workers are busy, submit blocks until one is free,
// providing natural back-pressure without goroutine proliferation.
type workerPool struct {
	tasks   chan func()
	quit    chan struct{}
	wg      sync.WaitGroup
	metrics PoolMetrics
}

func newWorkerPool(size int) *workerPool {
	p := &workerPool{
		tasks: make(chan func(), size*2), // buffered to absorb short bursts
		quit:  make(chan struct{}),
	}
	for i := 0; i < size; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case task, ok := <-p.tasks:
					if !ok {
						return
					}
					// Track active workers and queue length
					p.metrics.ActiveWorkers.v.Add(1)
					p.metrics.QueueLen.v.Add(-1)
					task()
					p.metrics.Completed.v.Add(1)
					p.metrics.ActiveWorkers.v.Add(-1)
				case <-p.quit:
					return
				}
			}
		}()
	}
	return p
}

// submit enqueues task for execution.  It blocks if the task channel is full,
// giving back-pressure to callers (event loops) rather than growing unboundedly.
func (p *workerPool) submit(task func()) {
	select {
	case p.tasks <- task:
		p.metrics.Submitted.v.Add(1)
		p.metrics.QueueLen.v.Add(1)
	case <-p.quit:
	}
}

// trySubmit attempts to enqueue task without blocking.
// Returns true if the task was enqueued, false if the pool is saturated.
// Callers may use this to shed load (e.g. respond with HTTP 503) rather than
// blocking the event loop goroutine.
func (p *workerPool) trySubmit(task func()) bool {
	select {
	case p.tasks <- task:
		p.metrics.Submitted.v.Add(1)
		p.metrics.QueueLen.v.Add(1)
		return true
	default:
		p.metrics.Rejected.v.Add(1)
		return false
	}
}

// stop signals all workers to exit and waits for them to finish.
func (p *workerPool) stop() {
	close(p.quit)
	p.wg.Wait()
}

// Metrics returns a snapshot of the current pool metrics.
func (p *workerPool) Metrics() PoolMetricsSnapshot {
	return PoolMetricsSnapshot{
		Submitted:     p.metrics.Submitted.v.Load(),
		Rejected:      p.metrics.Rejected.v.Load(),
		Completed:     p.metrics.Completed.v.Load(),
		QueueLen:      p.metrics.QueueLen.v.Load(),
		ActiveWorkers: p.metrics.ActiveWorkers.v.Load(),
	}
}
