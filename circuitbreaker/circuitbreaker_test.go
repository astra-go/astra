package circuitbreaker_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astra-go/astra/circuitbreaker"
	"github.com/astra-go/astra/testutil"
)

var errBoom = errors.New("boom")

// ─── Construction & defaults ─────────────────────────────────────────────────

func TestNew_Defaults(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("d"))
	s := b.Settings()
	testutil.AssertEqual(t, uint32(5), s.MaxFailures)
	testutil.AssertEqual(t, 30*time.Second, s.Interval)
	testutil.AssertEqual(t, 60*time.Second, s.Timeout)
	testutil.AssertEqual(t, uint32(2), s.SuccessThreshold)
	testutil.AssertEqual(t, uint32(1), s.MaxRequests)
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())

	// sanity: a fresh breaker satisfies the CircuitBreaker interface.
	var _ circuitbreaker.CircuitBreaker = b
}

// ─── State: Closed → Open ─────────────────────────────────────────────────────

func TestClosed_StartsClosed(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("closed"), circuitbreaker.WithMaxFailures(3))
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())
}

func TestClosed_OpensAfterMaxFailures(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("trip"), circuitbreaker.WithMaxFailures(3))

	testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())

	testutil.AssertError(t, b.Call(context.Background(), func() error { return errBoom }))
	testutil.AssertError(t, b.Call(context.Background(), func() error { return errBoom }))
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())

	// third consecutive failure trips the breaker.
	testutil.AssertError(t, b.Call(context.Background(), func() error { return errBoom }))
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

func TestClosed_ASuccessResetsConsecutiveFailures(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("reset"), circuitbreaker.WithMaxFailures(2))
	_ = b.Call(context.Background(), func() error { return errBoom })
	_ = b.Call(context.Background(), func() error { return nil }) // resets
	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State()) // not yet 2 consecutive
	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

// ─── State: Open → HalfOpen after Interval ───────────────────────────────────

func TestOpen_RejectsBeforeInterval(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("reject"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(100*time.Millisecond))

	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	err := b.Call(context.Background(), func() error { return nil })
	testutil.AssertErrorIs(t, err, circuitbreaker.ErrOpen)
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

func TestOpen_BecomesHalfOpenAfterInterval(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("recover"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2))

	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	// Wait well past the interval to avoid CI timing flakes.
	time.Sleep(150 * time.Millisecond)

	// First probe after the interval is admitted and runs fn.
	ran := false
	testutil.AssertNoError(t, b.Call(context.Background(), func() error {
		ran = true
		return nil
	}))
	testutil.AssertEqual(t, true, ran)
	// Only one success so far (< SuccessThreshold): still HalfOpen.
	testutil.AssertEqual(t, circuitbreaker.StateHalfOpen, b.State())

	// Second success closes the breaker.
	testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())
}

// ─── State: HalfOpen → Open on failure ────────────────────────────────────────

func TestHalfOpen_FailureReopens(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("hoff-fail"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2))

	_ = b.Call(context.Background(), func() error { return errBoom })
	time.Sleep(120 * time.Millisecond)

	// Probe fails → back to Open.
	testutil.AssertError(t, b.Call(context.Background(), func() error { return errBoom }))
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

// ─── State: HalfOpen → Open on Timeout ────────────────────────────────────────

func TestHalfOpen_TimeoutForcesOpen(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("hoff-timeout"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithTimeout(50*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(100)) // never reached in this test

	_ = b.Call(context.Background(), func() error { return errBoom })
	time.Sleep(120 * time.Millisecond)

	// First probe succeeds (1 success, far below the huge threshold).
	testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
	testutil.AssertEqual(t, circuitbreaker.StateHalfOpen, b.State())

	// Wait past the HalfOpen timeout; the next call force-reopens.
	time.Sleep(120 * time.Millisecond)
	err := b.Call(context.Background(), func() error { return nil })
	testutil.AssertErrorIs(t, err, circuitbreaker.ErrOpen)
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

// ─── MaxRequests concurrency limit ────────────────────────────────────────────

func TestHalfOpen_MaxRequestsLimit(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("maxreq"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithMaxRequests(1),
		circuitbreaker.WithSuccessThreshold(100))

	_ = b.Call(context.Background(), func() error { return errBoom })
	time.Sleep(120 * time.Millisecond)

	started := make(chan struct{})
	release := make(chan struct{})
	blocking := func() error {
		close(started)
		<-release
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = b.Call(context.Background(), blocking) // admitted as the only probe
	}()
	<-started // first probe is now blocked in fn; inflight == 1

	var rejected int32
	var mu sync.Mutex
	var rejErrs []error
	var wg2 sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			err := b.Call(context.Background(), func() error { return nil })
			if errors.Is(err, circuitbreaker.ErrOpen) {
				atomic.AddInt32(&rejected, 1)
				mu.Lock()
				rejErrs = append(rejErrs, err)
				mu.Unlock()
			}
		}()
	}
	wg2.Wait()
	close(release)
	wg.Wait()

	testutil.AssertEqual(t, int32(5), atomic.LoadInt32(&rejected))
	testutil.AssertEqual(t, 5, len(rejErrs))
}

// ─── Custom ReadyToTrip (count / ratio) ───────────────────────────────────────

func TestReadyToTrip_TotalFailures(t *testing.T) {
	// Trip after 3 total failures (not necessarily consecutive).
	b := circuitbreaker.New(circuitbreaker.WithName("ratio"),
		circuitbreaker.WithReadyToTrip(func(c circuitbreaker.Counts) bool {
			return c.TotalFailures >= 3
		}))

	_ = b.Call(context.Background(), func() error { return errBoom }) // 1
	_ = b.Call(context.Background(), func() error { return nil })    // success
	_ = b.Call(context.Background(), func() error { return errBoom }) // 2
	_ = b.Call(context.Background(), func() error { return nil })    // success
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())
	_ = b.Call(context.Background(), func() error { return errBoom }) // 3 total
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

// ─── Custom IsSuccessful ──────────────────────────────────────────────────────

func TestIsSuccessful_Custom(t *testing.T) {
	ignored := errors.New("ignored")
	b := circuitbreaker.New(circuitbreaker.WithName("is-success"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithIsSuccessful(func(err error) bool {
			return err == nil || errors.Is(err, ignored)
		}))

	// errBoom counts as a failure → trips immediately.
	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	b.Reset()
	// ignored error counts as success → never trips.
	for i := 0; i < 5; i++ {
		_ = b.Call(context.Background(), func() error { return ignored })
	}
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())
}

// ─── OnStateChange callback ───────────────────────────────────────────────────

func TestOnStateChange_Fired(t *testing.T) {
	ch := make(chan [2]circuitbreaker.State, 16)
	b := circuitbreaker.New(circuitbreaker.WithName("cb"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2),
		circuitbreaker.WithOnStateChange(func(_ string, from, to circuitbreaker.State) {
			ch <- [2]circuitbreaker.State{from, to}
		}))

	_ = b.Call(context.Background(), func() error { return errBoom }) // Closed→Open
	time.Sleep(120 * time.Millisecond)
	_ = b.Call(context.Background(), func() error { return nil })    // Open→HalfOpen
	_ = b.Call(context.Background(), func() error { return nil })    // HalfOpen→Closed

	want := [][2]circuitbreaker.State{
		{circuitbreaker.StateClosed, circuitbreaker.StateOpen},
		{circuitbreaker.StateOpen, circuitbreaker.StateHalfOpen},
		{circuitbreaker.StateHalfOpen, circuitbreaker.StateClosed},
	}
	for i := 0; i < len(want); i++ {
		select {
		case got := <-ch:
			testutil.AssertEqual(t, want[i], got)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for transition #%d (%v)", i, want[i])
		}
	}
}

// ─── Context handling ─────────────────────────────────────────────────────────

func TestCall_ContextCancelled(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("ctx"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ran := false
	err := b.Call(ctx, func() error { ran = true; return nil })
	testutil.AssertErrorIs(t, err, context.Canceled)
	testutil.AssertEqual(t, false, ran)
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())
}

// ─── Counts & Reset ───────────────────────────────────────────────────────────

func TestCounts_And_Reset(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("counts"),
		circuitbreaker.WithMaxFailures(2))

	_ = b.Call(context.Background(), func() error { return nil })
	_ = b.Call(context.Background(), func() error { return errBoom })
	_ = b.Call(context.Background(), func() error { return errBoom }) // opens

	c := b.Counts()
	testutil.AssertEqual(t, int64(3), c.Requests)
	testutil.AssertEqual(t, int64(1), c.TotalSuccesses)
	testutil.AssertEqual(t, int64(2), c.TotalFailures)
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	b.Reset()
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())
	testutil.AssertEqual(t, int64(0), b.Counts().Requests)
}

// ─── Concurrency / race safety ────────────────────────────────────────────────

func TestConcurrent_NoRace(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("race"),
		circuitbreaker.WithMaxFailures(3),
		circuitbreaker.WithInterval(10*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2))

	var ops int64
	var wg sync.WaitGroup
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				atomic.AddInt64(&ops, 1)
				_ = b.Call(context.Background(), func() error {
					if (i%3 == 0) && (i%7 != 0) {
						return errBoom
					}
					return nil
				})
			}
		}()
	}

	// Wait with a generous timeout for slow CI machines.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent test timed out")
	}

	testutil.AssertEqual(t, int64(10000), atomic.LoadInt64(&ops))
	// The breaker must still be in a valid state.
	switch b.State() {
	case circuitbreaker.StateClosed, circuitbreaker.StateOpen, circuitbreaker.StateHalfOpen:
	default:
		t.Fatalf("invalid state: %v", b.State())
	}

	// Counts should be internally consistent (admitted calls only, rejected don't count).
	c := b.Counts()
	testutil.AssertEqual(t, c.Requests, c.TotalSuccesses+c.TotalFailures)
	t.Logf("admitted=%d rejected=%d", c.Requests, int64(10000)-c.Requests)
}

// ─── State.String ─────────────────────────────────────────────────────────────

func TestState_String(t *testing.T) {
	testutil.AssertEqual(t, "closed", circuitbreaker.StateClosed.String())
	testutil.AssertEqual(t, "open", circuitbreaker.StateOpen.String())
	testutil.AssertEqual(t, "half-open", circuitbreaker.StateHalfOpen.String())
}

// ─── Edge case: Interval <= 0 disables auto-recovery ─────────────────────────

func TestInterval_Zero_NeverRecovers(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("no-interval"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(0)) // never auto-recover

	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	// Even after a long wait, the breaker stays Open.
	time.Sleep(100 * time.Millisecond)
	err := b.Call(context.Background(), func() error { return nil })
	testutil.AssertErrorIs(t, err, circuitbreaker.ErrOpen)
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

func TestInterval_Negative_ImmediateHalfOpen(t *testing.T) {
	// Interval <= 0 means Open→HalfOpen transition happens on the very next call
	// (no waiting). This is useful for manual recovery via Reset().
	b := circuitbreaker.New(circuitbreaker.WithName("neg-interval"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(-1),
		circuitbreaker.WithSuccessThreshold(1)) // one success is enough to close

	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	// Immediate HalfOpen transition without waiting, single success closes.
	testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
	testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())
}

// ─── Edge case: Timeout == 0 disables HalfOpen timeout ────────────────────────

func TestTimeout_Zero_NoForceReopen(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("no-timeout"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithTimeout(0),
		circuitbreaker.WithSuccessThreshold(100)) // never close naturally

	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	// Transition to HalfOpen
	time.Sleep(120 * time.Millisecond)
	testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
	testutil.AssertEqual(t, circuitbreaker.StateHalfOpen, b.State())

	// Long wait — without Timeout, the breaker stays HalfOpen.
	time.Sleep(200 * time.Millisecond)
	testutil.AssertEqual(t, circuitbreaker.StateHalfOpen, b.State())

	// Further calls are admitted (probe limit permitting).
	testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
	testutil.AssertEqual(t, circuitbreaker.StateHalfOpen, b.State())
}

// ─── Inflight accounting under HalfOpen stress ───────────────────────────────

func TestHalfOpen_InflightAccounting(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("inflight"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithMaxRequests(3),
		circuitbreaker.WithSuccessThreshold(2))

	// Trip the breaker.
	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	// Wait for HalfOpen transition.
	time.Sleep(120 * time.Millisecond)

	// Hold multiple probes in-flight simultaneously.
	var startedOnce sync.Once
	started := make(chan struct{})
	release := make(chan struct{})

	var inFlight int32
	var maxInFlight int32

	blocking := func() error {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, cur) {
				break
			}
		}
		startedOnce.Do(func() { close(started) })
		<-release
		atomic.AddInt32(&inFlight, -1)
		return nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Call(context.Background(), blocking)
		}()
	}

	// Wait until all 3 probes are blocked.
	<-started
	time.Sleep(100 * time.Millisecond)

	// With MaxRequests=3 and 3 probes in-flight, the 4th must be rejected.
	err := b.Call(context.Background(), func() error { return nil })
	testutil.AssertErrorIs(t, err, circuitbreaker.ErrOpen)

	// All 3 in-flight probes should have been admitted.
	testutil.AssertEqual(t, int32(3), atomic.LoadInt32(&maxInFlight))

	// Release probes.
	close(release)
	wg.Wait()

	// After probes complete, inflight returns to 0 and new calls are admitted.
	time.Sleep(50 * time.Millisecond)
	testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
}

// ─── Panic recovery: inflight counter must not leak ───────────────────────────

func TestCall_FnPanic_InflightCleanup(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("panic"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithMaxRequests(1),
		circuitbreaker.WithSuccessThreshold(100))

	// Trip the breaker.
	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	time.Sleep(120 * time.Millisecond)

	// A HalfOpen probe that panics.
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_ = b.Call(context.Background(), func() error {
			panic("boom")
		})
	}()
	testutil.AssertEqual(t, true, panicked)

	// Panic counts as failure → breaker is Open again.
	// But inflight was cleaned up: after the interval, a new probe is admitted
	// (proving the inflight counter was properly decremented during recovery).
	time.Sleep(120 * time.Millisecond)
	testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
}

// ─── Successful call after panic does not leave stale failure counts ──────────

func TestCall_PanicCountsAsFailure(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("panic-fail"),
		circuitbreaker.WithMaxFailures(2))

	func() {
		defer func() { recover() }()
		_ = b.Call(context.Background(), func() error { panic("boom") })
	}()

	c := b.Counts()
	testutil.AssertEqual(t, int64(1), c.Requests)
	testutil.AssertEqual(t, int64(0), c.TotalSuccesses)
	testutil.AssertEqual(t, int64(1), c.TotalFailures)

	// Another failure should trip the breaker (MaxFailures=2).
	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

// ─── Multiple Open ↔ HalfOpen cycles ──────────────────────────────────────────

func TestHalfOpen_MultipleRecoveryCycles(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("cycles"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2))

	for cycle := 0; cycle < 3; cycle++ {
		// Trip the breaker.
		_ = b.Call(context.Background(), func() error { return errBoom })
		testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

		// Wait for HalfOpen.
		time.Sleep(120 * time.Millisecond)

		// Fail the probe → back to Open.
		testutil.AssertError(t, b.Call(context.Background(), func() error { return errBoom }))
		testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
	}
}

func TestHalfOpen_MultipleRecoverySuccess(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("cycles-ok"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2))

	for cycle := 0; cycle < 3; cycle++ {
		_ = b.Call(context.Background(), func() error { return errBoom })
		testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

		time.Sleep(120 * time.Millisecond)

		// Two successful probes → Closed.
		testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
		testutil.AssertEqual(t, circuitbreaker.StateHalfOpen, b.State())
		testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
		testutil.AssertEqual(t, circuitbreaker.StateClosed, b.State())
	}
}

// ─── MaxRequests=0 defaults to 1 ─────────────────────────────────────────────

func TestHalfOpen_MaxRequestsZeroDefaultsToOne(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("maxreq0"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(50*time.Millisecond),
		circuitbreaker.WithMaxRequests(0),
		circuitbreaker.WithSuccessThreshold(100))

	_ = b.Call(context.Background(), func() error { return errBoom })
	time.Sleep(120 * time.Millisecond)

	// First probe is admitted (MaxRequests defaults to 1).
	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = b.Call(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	// Second probe must be rejected (MaxRequests==1).
	err := b.Call(context.Background(), func() error { return nil })
	testutil.AssertErrorIs(t, err, circuitbreaker.ErrOpen)

	close(release)
}

// ─── Names are settable and retrievable ───────────────────────────────────────

func TestName_Retrievable(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("my-cb"))
	testutil.AssertEqual(t, "my-cb", b.Name())
}

// ─── Default name ─────────────────────────────────────────────────────────────

func TestName_Default(t *testing.T) {
	b := circuitbreaker.New()
	testutil.AssertEqual(t, "circuit-breaker", b.Name())
}
