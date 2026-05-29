// Package netengine — white-box micro-benchmarks for the bounded worker pool.
//
// These benchmarks live in package netengine (not netengine_test) to access
// the unexported workerPool type directly and measure raw pool throughput
// without the HTTP overhead captured by engine_bench_test.go.
//
// Run:
//
//	go test -bench=BenchmarkWorkerPool -benchmem -count=3 ./netengine/
package netengine

import (
	"runtime"
	"sync/atomic"
	"testing"
)

// poolBenchSink prevents dead-code elimination.
var poolBenchSink int64

// BenchmarkWorkerPool_TrySubmit measures the hot path: a single goroutine
// non-blocking-submitting tasks to an empty pool.
//
// trySubmit is on the critical path of the event-loop → worker handoff.
// This benchmark isolates the channel-send overhead.
func BenchmarkWorkerPool_TrySubmit(b *testing.B) {
	size := runtime.GOMAXPROCS(0)
	p := newWorkerPool(size)
	defer p.stop()

	var n int64
	task := func() { atomic.AddInt64(&n, 1) }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Spin until the pool has capacity; mirrors the event-loop retry.
		for !p.trySubmit(task) {
			runtime.Gosched()
		}
	}
	b.StopTimer()
	poolBenchSink = atomic.LoadInt64(&n)
}

// BenchmarkWorkerPool_Submit_Parallel measures concurrent blocking-submit
// throughput: GOMAXPROCS goroutines all submitting tasks simultaneously.
//
// This corresponds to N event loops delivering work to the shared pool
// at the same instant and models steady-state throughput.
func BenchmarkWorkerPool_Submit_Parallel(b *testing.B) {
	size := runtime.GOMAXPROCS(0) * 4
	p := newWorkerPool(size)
	defer p.stop()

	var n int64
	task := func() { atomic.AddInt64(&n, 1) }

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p.submit(task)
		}
	})
	b.StopTimer()
	poolBenchSink = atomic.LoadInt64(&n)
}

// BenchmarkWorkerPool_TrySubmit_Parallel measures the non-blocking trySubmit
// from multiple goroutines — the saturated event-loop path that triggers 503
// load shedding.
func BenchmarkWorkerPool_TrySubmit_Parallel(b *testing.B) {
	size := runtime.GOMAXPROCS(0) * 4
	p := newWorkerPool(size)
	defer p.stop()

	var n int64
	task := func() { atomic.AddInt64(&n, 1) }

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for !p.trySubmit(task) {
				runtime.Gosched()
			}
		}
	})
	b.StopTimer()
	poolBenchSink = atomic.LoadInt64(&n)
}

// BenchmarkWorkerPool_Metrics_Snapshot measures the cost of taking a metrics
// snapshot. This is on the read path for health-check endpoints.
func BenchmarkWorkerPool_Metrics_Snapshot(b *testing.B) {
	size := runtime.GOMAXPROCS(0) * 4
	p := newWorkerPool(size)
	defer p.stop()

	// Pre-submit some tasks to make the snapshot non-trivial
	var n int64
	for i := 0; i < size*2; i++ {
		p.submit(func() {
			atomic.AddInt64(&n, 1)
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := p.Metrics()
		poolBenchSink = snap.Submitted + snap.Completed
	}
}

// BenchmarkWorkerPool_Submit_WithMetrics measures submit throughput with
// metrics enabled, to verify the atomic operations don't significantly
// impact performance.
func BenchmarkWorkerPool_Submit_WithMetrics(b *testing.B) {
	size := runtime.GOMAXPROCS(0) * 4
	p := newWorkerPool(size)
	defer p.stop()

	var n int64
	task := func() { atomic.AddInt64(&n, 1) }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.submit(task)
	}
	b.StopTimer()
	poolBenchSink = atomic.LoadInt64(&n)
}

// BenchmarkWorkerPool_TrySubmit_WithMetrics measures trySubmit throughput with
// metrics enabled.
func BenchmarkWorkerPool_TrySubmit_WithMetrics(b *testing.B) {
	size := runtime.GOMAXPROCS(0) * 4
	p := newWorkerPool(size)
	defer p.stop()

	var n int64
	task := func() { atomic.AddInt64(&n, 1) }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for !p.trySubmit(task) {
			runtime.Gosched()
		}
	}
	b.StopTimer()
	poolBenchSink = atomic.LoadInt64(&n)
}
