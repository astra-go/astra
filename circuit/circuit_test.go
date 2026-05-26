package circuit_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra/circuit"
	"github.com/astra-go/astra/testutil"
)

var errService = errors.New("service error")

// ─── State transitions ────────────────────────────────────────────────────────

func TestBreaker_StartsClosed(t *testing.T) {
	b := circuit.New(circuit.Config{Name: "test", Threshold: 3, Timeout: time.Minute})
	testutil.AssertEqual(t, circuit.StateClosed, b.State())
}

func TestBreaker_OpensAfterThreshold(t *testing.T) {
	tests := []struct {
		name      string
		failures  int
		threshold int64
		wantState circuit.State
	}{
		{"no failures", 0, 3, circuit.StateClosed},
		{"below threshold", 2, 3, circuit.StateClosed},
		{"at threshold", 3, 3, circuit.StateOpen},
		{"above threshold", 5, 3, circuit.StateOpen},
		{"threshold one", 1, 1, circuit.StateOpen},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := circuit.New(circuit.Config{
				Name:      tc.name,
				Threshold: tc.threshold,
				Timeout:   time.Minute,
			})
			for i := 0; i < tc.failures; i++ {
				_ = b.Do(func() error { return errService })
			}
			testutil.AssertEqual(t, tc.wantState, b.State())
		})
	}
}

func TestBreaker_OpenRejectsRequests(t *testing.T) {
	b := circuit.New(circuit.Config{Name: "open", Threshold: 1, Timeout: time.Minute})
	_ = b.Do(func() error { return errService }) // trips the breaker

	err := b.Do(func() error { return nil })
	testutil.AssertErrorIs(t, err, circuit.ErrOpen)
}

func TestBreaker_Success_ResetFailureCount(t *testing.T) {
	b := circuit.New(circuit.Config{Name: "reset", Threshold: 3, Timeout: time.Minute})

	// Two failures — not yet open.
	_ = b.Do(func() error { return errService })
	_ = b.Do(func() error { return errService })

	// A success resets the counter.
	_ = b.Do(func() error { return nil })

	// Two more failures — should still be closed (counter was reset).
	_ = b.Do(func() error { return errService })
	_ = b.Do(func() error { return errService })
	testutil.AssertEqual(t, circuit.StateClosed, b.State())
}

func TestBreaker_TransitionsToHalfOpen(t *testing.T) {
	b := circuit.New(circuit.Config{
		Name:      "half-open",
		Threshold: 1,
		Timeout:   10 * time.Millisecond,
	})
	_ = b.Do(func() error { return errService }) // open
	testutil.AssertEqual(t, circuit.StateOpen, b.State())

	time.Sleep(20 * time.Millisecond) // wait for timeout
	testutil.AssertEqual(t, circuit.StateHalfOpen, b.State())
}

func TestBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	b := circuit.New(circuit.Config{
		Name:                "close-from-half-open",
		Threshold:           1,
		Timeout:             5 * time.Millisecond,
		HalfOpenSuccesses:   2,
		HalfOpenMaxRequests: 2, // allow both probes through
	})
	_ = b.Do(func() error { return errService }) // open
	time.Sleep(10 * time.Millisecond)            // → half-open

	testutil.AssertEqual(t, circuit.StateHalfOpen, b.State())

	// 2 consecutive successes should close.
	testutil.AssertNoError(t, b.Do(func() error { return nil }))
	testutil.AssertNoError(t, b.Do(func() error { return nil }))
	testutil.AssertEqual(t, circuit.StateClosed, b.State())
}

func TestBreaker_HalfOpen_FailureReopens(t *testing.T) {
	b := circuit.New(circuit.Config{
		Name:      "reopen-from-half-open",
		Threshold: 1,
		Timeout:   5 * time.Millisecond,
	})
	_ = b.Do(func() error { return errService }) // open
	time.Sleep(10 * time.Millisecond)            // → half-open

	// A failure in half-open reopens.
	_ = b.Do(func() error { return errService })
	testutil.AssertEqual(t, circuit.StateOpen, b.State())
}

// ─── OnStateChange callback ───────────────────────────────────────────────────

func TestBreaker_StateChangeCallback_Fired(t *testing.T) {
	var changes []string
	var mu sync.Mutex

	b := circuit.New(circuit.Config{
		Name:      "callback",
		Threshold: 1,
		Timeout:   time.Minute,
		OnStateChange: func(name string, from, to circuit.State) {
			mu.Lock()
			changes = append(changes, from.String()+"→"+to.String())
			mu.Unlock()
		},
	})

	_ = b.Do(func() error { return errService }) // closed → open
	time.Sleep(10 * time.Millisecond)            // callback fires in goroutine

	mu.Lock()
	defer mu.Unlock()
	if len(changes) == 0 {
		t.Error("expected OnStateChange to be called")
	}
	testutil.AssertEqual(t, "closed→open", changes[0])
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func TestBreaker_Stats(t *testing.T) {
	b := circuit.New(circuit.Config{Name: "stats", Threshold: 5, Timeout: time.Minute})

	_ = b.Do(func() error { return errService })
	_ = b.Do(func() error { return errService })
	_ = b.Do(func() error { return nil })

	s := b.Stats()
	testutil.AssertEqual(t, "stats", s.Name)
	testutil.AssertEqual(t, circuit.StateClosed, s.State)
	testutil.AssertEqual(t, int64(0), s.Failures) // reset after success
}

// ─── Concurrent safety ────────────────────────────────────────────────────────

func TestBreaker_ConcurrentAccess(t *testing.T) {
	b := circuit.New(circuit.Config{Name: "concurrent", Threshold: 1000, Timeout: time.Minute})

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Do(func() error { return nil })
			_ = b.State()
			_ = b.Stats()
		}()
	}
	wg.Wait()
	// If there are any race conditions, -race flag will catch them.
}

// ─── Do propagates the function's error ───────────────────────────────────────

func TestBreaker_Do_PropagatesError(t *testing.T) {
	b := circuit.New(circuit.Config{Name: "propagate", Threshold: 10, Timeout: time.Minute})
	want := errors.New("specific error")
	got := b.Do(func() error { return want })
	testutil.AssertErrorIs(t, got, want)
}

func TestBreaker_Do_PropagatesNil(t *testing.T) {
	b := circuit.New(circuit.Config{Name: "nil", Threshold: 10, Timeout: time.Minute})
	testutil.AssertNoError(t, b.Do(func() error { return nil }))
}
