package netengine

import (
	"sync"
)

// workerPool is a bounded pool of goroutines that execute tasks (HTTP request
// handling).  When all workers are busy, submit blocks until one is free,
// providing natural back-pressure without goroutine proliferation.
type workerPool struct {
	tasks chan func()
	quit  chan struct{}
	wg    sync.WaitGroup
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
					task()
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
		return true
	default:
		return false
	}
}

// stop signals all workers to exit and waits for them to finish.
func (p *workerPool) stop() {
	close(p.quit)
	p.wg.Wait()
}
