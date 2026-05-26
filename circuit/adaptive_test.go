package circuit_test

import (
	"errors"
	"testing"
	"time"

	"github.com/astra-go/astra/circuit"
	"github.com/astra-go/astra/testutil"
)

// ─── Adaptive circuit breaker ─────────────────────────────────────────────────

func TestAdaptive_StartsClosed(t *testing.T) {
	ab := circuit.NewAdaptive(circuit.AdaptiveConfig{Name: "starts-closed"})
	testutil.AssertEqual(t, circuit.StateClosed, ab.State())
}

func TestAdaptive_TripsOnHighErrorRate(t *testing.T) {
	ab := circuit.NewAdaptive(circuit.AdaptiveConfig{
		Name:               "error-rate",
		Window:             time.Second,
		BucketCount:        10,
		MinRequests:        10,
		ErrorRateThreshold: 0.5,
	})

	fail := func() error { return errors.New("fail") }
	pass := func() error { return nil }

	// 6 failures, 4 successes = 60% error rate over 10 requests → should trip.
	for i := 0; i < 6; i++ {
		_ = ab.Do(fail)
	}
	for i := 0; i < 4; i++ {
		_ = ab.Do(pass)
	}

	testutil.AssertEqual(t, circuit.StateOpen, ab.State())
}

func TestAdaptive_DoesNotTripBelowMinRequests(t *testing.T) {
	ab := circuit.NewAdaptive(circuit.AdaptiveConfig{
		Name:               "min-req",
		MinRequests:        20,
		ErrorRateThreshold: 0.1,
	})

	// Only 5 requests (all failures) — below MinRequests threshold.
	for i := 0; i < 5; i++ {
		_ = ab.Do(func() error { return errors.New("err") })
	}

	testutil.AssertEqual(t, circuit.StateClosed, ab.State())
}

func TestAdaptive_DoesNotTripBelowErrorRateThreshold(t *testing.T) {
	ab := circuit.NewAdaptive(circuit.AdaptiveConfig{
		Name:               "low-rate",
		MinRequests:        10,
		ErrorRateThreshold: 0.8, // only trip at 80%+ error rate
	})

	fail := func() error { return errors.New("fail") }
	pass := func() error { return nil }

	// 3 failures, 7 successes = 30% error rate → should NOT trip.
	for i := 0; i < 3; i++ {
		_ = ab.Do(fail)
	}
	for i := 0; i < 7; i++ {
		_ = ab.Do(pass)
	}

	testutil.AssertEqual(t, circuit.StateClosed, ab.State())
}

func TestAdaptive_TripsOnHighLatency(t *testing.T) {
	ab := circuit.NewAdaptive(circuit.AdaptiveConfig{
		Name:               "latency",
		Window:             time.Second,
		MinRequests:        5,
		ErrorRateThreshold: 1.0,                  // disable error rate tripping
		LatencyThreshold:   2 * time.Millisecond, // very low threshold
	})

	// All requests take 5ms — P99 will exceed the 2ms threshold.
	for i := 0; i < 5; i++ {
		_ = ab.Do(func() error {
			time.Sleep(5 * time.Millisecond)
			return nil
		})
	}

	testutil.AssertEqual(t, circuit.StateOpen, ab.State())
}

func TestAdaptive_RejectsWhenOpen(t *testing.T) {
	ab := circuit.NewAdaptive(circuit.AdaptiveConfig{
		Name:               "open-reject",
		MinRequests:        5,
		ErrorRateThreshold: 0.5,
	})

	// Trip the breaker.
	for i := 0; i < 5; i++ {
		_ = ab.Do(func() error { return errors.New("err") })
	}

	// Should reject with ErrOpen.
	err := ab.Do(func() error { return nil })
	testutil.AssertErrorIs(t, err, circuit.ErrOpen)
}

func TestAdaptive_RecoveryAfterTimeout(t *testing.T) {
	ab := circuit.NewAdaptive(circuit.AdaptiveConfig{
		Name:                "recovery",
		MinRequests:         5,
		ErrorRateThreshold:  0.5,
		Timeout:             20 * time.Millisecond,
		HalfOpenSuccesses:   2,
		HalfOpenMaxRequests: 2, // allow both probes through
	})

	// Trip.
	for i := 0; i < 5; i++ {
		_ = ab.Do(func() error { return errors.New("err") })
	}
	testutil.AssertEqual(t, circuit.StateOpen, ab.State())

	// Wait for timeout → half-open.
	time.Sleep(30 * time.Millisecond)
	testutil.AssertEqual(t, circuit.StateHalfOpen, ab.State())

	// 2 successes → closed.
	testutil.AssertNoError(t, ab.Do(func() error { return nil }))
	testutil.AssertNoError(t, ab.Do(func() error { return nil }))
	testutil.AssertEqual(t, circuit.StateClosed, ab.State())
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func TestAdaptive_Stats_ErrorRate(t *testing.T) {
	ab := circuit.NewAdaptive(circuit.AdaptiveConfig{
		Name:        "stats",
		MinRequests: 4,
		Window:      time.Second,
	})

	_ = ab.Do(func() error { return errors.New("err") })
	_ = ab.Do(func() error { return errors.New("err") })
	_ = ab.Do(func() error { return nil })
	_ = ab.Do(func() error { return nil })

	s := ab.Stats()
	testutil.AssertEqual(t, "stats", s.Name)
	testutil.AssertEqual(t, int64(4), s.TotalReqs)
	testutil.AssertEqual(t, int64(2), s.ErrorReqs)

	const wantRate = 0.5
	if s.ErrorRate < wantRate-0.01 || s.ErrorRate > wantRate+0.01 {
		t.Errorf("ErrorRate: want ~%.2f, got %.2f", wantRate, s.ErrorRate)
	}
}

func TestAdaptive_Stats_EmptyWindow(t *testing.T) {
	ab := circuit.NewAdaptiveSimple("empty")
	s := ab.Stats()
	testutil.AssertEqual(t, int64(0), s.TotalReqs)
	testutil.AssertEqual(t, float64(0), s.ErrorRate)
}

func TestAdaptive_HalfOpen_FailureReopens(t *testing.T) {
	ab := circuit.NewAdaptive(circuit.AdaptiveConfig{
		Name:               "half-open-fail",
		MinRequests:        5,
		ErrorRateThreshold: 0.5,
		Timeout:            10 * time.Millisecond,
	})

	for i := 0; i < 5; i++ {
		_ = ab.Do(func() error { return errors.New("err") })
	}
	time.Sleep(15 * time.Millisecond) // → half-open

	testutil.AssertEqual(t, circuit.StateHalfOpen, ab.State())

	// Failure in half-open → back to open.
	_ = ab.Do(func() error { return errors.New("probe failed") })
	testutil.AssertEqual(t, circuit.StateOpen, ab.State())
}
