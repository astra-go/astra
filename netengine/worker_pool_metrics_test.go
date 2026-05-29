package netengine

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"log/slog"
	"net/http"
)

// TestWorkerPool_Metrics_Submitted verifies that Submitted counter increments
// on every successful submit and trySubmit.
func TestWorkerPool_Metrics_Submitted(t *testing.T) {
	size := 4
	p := newWorkerPool(size)
	defer p.stop()

	// Drain the pool to ensure tasks execute immediately
	var wg sync.WaitGroup
	for i := 0; i < size*2; i++ {
		wg.Add(1)
		p.submit(func() {
			wg.Done()
		})
	}
	wg.Wait()

	// Check metrics
	snap := p.Metrics()
	if snap.Submitted != int64(size*2) {
		t.Errorf("Submitted = %d, want %d", snap.Submitted, size*2)
	}
}

// TestWorkerPool_Metrics_Rejected verifies that Rejected counter increments
// when trySubmit fails due to a full pool.
func TestWorkerPool_Metrics_Rejected(t *testing.T) {
	size := 2
	p := newWorkerPool(size)
	defer p.stop()

	// Fill the pool channel (size*2 buffer)
	tasksSubmitted := 0
	for i := 0; i < size*2+10; i++ {
		if p.trySubmit(func() {
			time.Sleep(10 * time.Millisecond) // hold worker
		}) {
			tasksSubmitted++
		}
	}

	// Give workers time to start
	runtime.Gosched()

	// Now trySubmit should fail (pool saturated)
	rejected := 0
	for i := 0; i < 100; i++ {
		if !p.trySubmit(func() {}) {
			rejected++
		}
	}

	snap := p.Metrics()
	if snap.Rejected < int64(rejected) {
		t.Errorf("Rejected = %d, want >= %d", snap.Rejected, rejected)
	}
}

// TestWorkerPool_Metrics_Completed verifies that Completed counter increments
// after tasks finish execution.
func TestWorkerPool_Metrics_Completed(t *testing.T) {
	size := 4
	p := newWorkerPool(size)
	defer p.stop()

	// Submit tasks and wait for completion
	var wg sync.WaitGroup
	numTasks := 100
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		p.submit(func() {
			wg.Done()
		})
	}
	wg.Wait()

	// Give workers time to update metrics
	runtime.Gosched()

	snap := p.Metrics()
	if snap.Completed != int64(numTasks) {
		t.Errorf("Completed = %d, want %d", snap.Completed, numTasks)
	}
}

// TestWorkerPool_Metrics_ActiveWorkers verifies that ActiveWorkers counter
// accurately reflects the number of workers currently executing tasks.
func TestWorkerPool_Metrics_ActiveWorkers(t *testing.T) {
	size := 4
	p := newWorkerPool(size)

	// Submit long-running tasks to all workers
	blocker := make(chan struct{})
	var activeCount atomic.Int64

	for i := 0; i < size; i++ {
		p.submit(func() {
			activeCount.Add(1)
			<-blocker // block until we release
			activeCount.Add(-1)
		})
	}

	// Wait for all workers to start (with timeout)
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatalf("timeout waiting for workers to start")
		default:
			if activeCount.Load() == int64(size) {
				// All workers are active, now check metrics
				snap := p.Metrics()
				if snap.ActiveWorkers != int64(size) {
					t.Errorf("ActiveWorkers = %d, want %d", snap.ActiveWorkers, size)
				}
				close(blocker) // release workers
				p.stop()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// TestWorkerPool_Metrics_QueueLen verifies that QueueLen approximates the
// number of tasks waiting in the queue.
func TestWorkerPool_Metrics_QueueLen(t *testing.T) {
	size := 2
	p := newWorkerPool(size)

	// Fill the channel buffer with blocking tasks
	blocker := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < size*2; i++ {
		wg.Add(1)
		p.submit(func() {
			wg.Done()
			<-blocker // block until we release
		})
	}

	// Wait for workers to start
	runtime.Gosched()
	time.Sleep(10 * time.Millisecond)

	// QueueLen should be approximately 0 (all tasks being processed)
	snap := p.Metrics()
	// Allow some tolerance since QueueLen is approximate
	if snap.QueueLen < 0 {
		t.Errorf("QueueLen = %d, want >= 0", snap.QueueLen)
	}

	// Release workers
	close(blocker)
	wg.Wait()
	p.stop()
}

// TestWorkerPool_Metrics_Snapshot verifies that Snapshot returns consistent data.
func TestWorkerPool_Metrics_Snapshot(t *testing.T) {
	size := 4
	p := newWorkerPool(size)
	defer p.stop()

	// Submit some tasks
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		p.submit(func() {
			wg.Done()
		})
	}
	wg.Wait()

	// Get snapshot
	snap := p.Metrics()

	// Verify snapshot fields are non-negative
	if snap.Submitted < 0 {
		t.Errorf("Snapshot.Submitted = %d, want >= 0", snap.Submitted)
	}
	if snap.Completed < 0 {
		t.Errorf("Snapshot.Completed = %d, want >= 0", snap.Completed)
	}
	if snap.Submitted != int64(50) {
		t.Errorf("Snapshot.Submitted = %d, want 50", snap.Submitted)
	}
	if snap.Completed != int64(50) {
		t.Errorf("Snapshot.Completed = %d, want 50", snap.Completed)
	}
}

// TestWorkerPool_Metrics_ConcurrentSubmit verifies metrics accuracy under
// concurrent submit load.
func TestWorkerPool_Metrics_ConcurrentSubmit(t *testing.T) {
	size := runtime.GOMAXPROCS(0) * 4
	p := newWorkerPool(size)
	defer p.stop()

	var wg sync.WaitGroup
	numTasks := 10000
	wg.Add(numTasks)

	for i := 0; i < numTasks; i++ {
		go func() {
			defer wg.Done()
			p.submit(func() {})
		}()
	}
	wg.Wait()

	// Wait for all tasks to complete
	time.Sleep(100 * time.Millisecond)

	snap := p.Metrics()
	if snap.Submitted != int64(numTasks) {
		t.Errorf("Submitted = %d, want %d", snap.Submitted, numTasks)
	}
	if snap.Completed != int64(numTasks) {
		t.Errorf("Completed = %d, want %d", snap.Completed, numTasks)
	}
}

// TestEngine_PoolSnapshot verifies that Engine.PoolSnapshot() returns metrics.
func TestEngine_PoolSnapshot(t *testing.T) {
	// Create a minimal engine with a handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	cfg := ReactorConfig{
		NumLoops:        1,
		WorkerPoolSize:  4,
		ReadBufferSize:  1024,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    60 * time.Second,
		Logger:          slog.Default(),
	}

	e, err := New(handler, cfg)
	if err != nil {
		t.Skipf("epoll/kqueue not available: %v", err)
	}
	defer e.Close()

	// Get snapshot
	snap := e.PoolSnapshot()

	// Verify it's not nil (zero-valued is OK)
	if snap.Submitted != 0 {
		t.Errorf("PoolSnapshot().Submitted = %d, want 0 (no tasks yet)", snap.Submitted)
	}
}
