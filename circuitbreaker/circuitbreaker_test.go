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
		circuitbreaker.WithInterval(50*time.Millisecond))

	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	err := b.Call(context.Background(), func() error { return nil })
	testutil.AssertErrorIs(t, err, circuitbreaker.ErrOpen)
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

func TestOpen_BecomesHalfOpenAfterInterval(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("recover"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(40*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2))

	_ = b.Call(context.Background(), func() error { return errBoom })
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())

	time.Sleep(60 * time.Millisecond)

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
		circuitbreaker.WithInterval(30*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2))

	_ = b.Call(context.Background(), func() error { return errBoom })
	time.Sleep(45 * time.Millisecond)

	// Probe fails → back to Open.
	testutil.AssertError(t, b.Call(context.Background(), func() error { return errBoom }))
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

// ─── State: HalfOpen → Open on Timeout ────────────────────────────────────────

func TestHalfOpen_TimeoutForcesOpen(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("hoff-timeout"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(30*time.Millisecond),
		circuitbreaker.WithTimeout(30*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(100)) // never reached in this test

	_ = b.Call(context.Background(), func() error { return errBoom })
	time.Sleep(45 * time.Millisecond)

	// First probe succeeds (1 success, far below the huge threshold).
	testutil.AssertNoError(t, b.Call(context.Background(), func() error { return nil }))
	testutil.AssertEqual(t, circuitbreaker.StateHalfOpen, b.State())

	// Wait past the HalfOpen timeout; the next call force-reopens.
	time.Sleep(45 * time.Millisecond)
	err := b.Call(context.Background(), func() error { return nil })
	testutil.AssertErrorIs(t, err, circuitbreaker.ErrOpen)
	testutil.AssertEqual(t, circuitbreaker.StateOpen, b.State())
}

// ─── MaxRequests concurrency limit ────────────────────────────────────────────

func TestHalfOpen_MaxRequestsLimit(t *testing.T) {
	b := circuitbreaker.New(circuitbreaker.WithName("maxreq"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(20*time.Millisecond),
		circuitbreaker.WithMaxRequests(1),
		circuitbreaker.WithSuccessThreshold(100))

	_ = b.Call(context.Background(), func() error { return errBoom })
	time.Sleep(30 * time.Millisecond)

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
		circuitbreaker.WithInterval(20*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2),
		circuitbreaker.WithOnStateChange(func(_ string, from, to circuitbreaker.State) {
			ch <- [2]circuitbreaker.State{from, to}
		}))

	_ = b.Call(context.Background(), func() error { return errBoom }) // Closed→Open
	time.Sleep(30 * time.Millisecond)
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
		circuitbreaker.WithInterval(5*time.Millisecond),
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
	wg.Wait()
	testutil.AssertEqual(t, int64(10000), atomic.LoadInt64(&ops))
	// The breaker must still be in a valid state.
	switch b.State() {
	case circuitbreaker.StateClosed, circuitbreaker.StateOpen, circuitbreaker.StateHalfOpen:
	default:
		t.Fatalf("invalid state: %v", b.State())
	}
}

// ─── State.String ─────────────────────────────────────────────────────────────

func TestState_String(t *testing.T) {
	testutil.AssertEqual(t, "closed", circuitbreaker.StateClosed.String())
	testutil.AssertEqual(t, "open", circuitbreaker.StateOpen.String())
	testutil.AssertEqual(t, "half-open", circuitbreaker.StateHalfOpen.String())
}
