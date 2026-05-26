// adaptive.go — AdaptiveBreaker
//
// Unlike the basic Breaker (which trips on N consecutive failures),
// AdaptiveBreaker trips when either:
//   - The error rate over a rolling time window exceeds a configurable threshold, OR
//   - The P99 request latency over the same window exceeds a configurable threshold.
//
// This makes it far more useful in real workloads where failures are not always
// consecutive (e.g. 40% error rate with interleaved successes).
//
// # State machine
//
//	Closed  → Open      when error rate ≥ ErrorRateThreshold (and total ≥ MinRequests)
//	                    OR P99 latency ≥ LatencyThreshold (and total ≥ MinRequests)
//	Open    → HalfOpen  after Timeout elapses
//	HalfOpen→ Closed    after HalfOpenSuccesses consecutive successes
//	HalfOpen→ Open      on any failure
//
// # Rolling window
//
// Stats are accumulated in BucketCount fixed-size time buckets spanning
// Window in total (default: 10 buckets × 1 s each = 10 s window).
// Expired buckets are discarded lazily (no background goroutine required).
//
// # P99 latency
//
// Latencies are stored in a fixed-size ring buffer (LatencySampleSize entries,
// default 256). The 99th percentile is calculated by sorting a copy of the
// buffer.  This is accurate when traffic is steady; for very bursty loads
// consider a DDSketch or t-digest implementation.
package circuit

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/astra-go/astra"
)

// AdaptiveConfig holds the configuration for an AdaptiveBreaker.
type AdaptiveConfig struct {
	// Name is a human-readable identifier (used in error messages / logs).
	Name string

	// ── Rolling window ──────────────────────────────────────────────────────

	// Window is the total duration of the rolling stats window.
	// Default: 10s.
	Window time.Duration
	// BucketCount is the number of time buckets within Window.
	// More buckets = finer granularity but slightly higher memory usage.
	// Default: 10.
	BucketCount int

	// ── Error-rate tripping ──────────────────────────────────────────────────

	// ErrorRateThreshold is the error fraction [0.0, 1.0] that trips the
	// circuit.  E.g. 0.5 = trip when ≥50% of requests fail.  Default: 0.5.
	ErrorRateThreshold float64
	// MinRequests is the minimum number of requests in the window before the
	// error-rate threshold is evaluated.  Prevents false trips on low traffic.
	// Default: 10.
	MinRequests int64

	// ── Latency-based tripping ───────────────────────────────────────────────

	// LatencyThreshold trips the circuit when the P99 request latency exceeds
	// this value.  0 = disabled (default).
	LatencyThreshold time.Duration
	// LatencySampleSize is the capacity of the latency ring buffer used for
	// P99 computation.  Default: 256.
	LatencySampleSize int

	// ── Recovery ─────────────────────────────────────────────────────────────

	// Timeout is how long the circuit stays open before switching to HalfOpen.
	// Default: 30s.
	Timeout time.Duration
	// HalfOpenSuccesses is the number of consecutive successes needed in
	// HalfOpen to close the circuit.  Default: 2.
	HalfOpenSuccesses int64
	// HalfOpenMaxRequests limits concurrent probe requests in HalfOpen.
	// Default: 1.
	HalfOpenMaxRequests int64

	// OnStateChange is called (in a goroutine) when the state changes.
	OnStateChange func(name string, from, to State)
}

func (c *AdaptiveConfig) applyDefaults() {
	if c.Name == "" {
		c.Name = "adaptive"
	}
	if c.Window <= 0 {
		c.Window = 10 * time.Second
	}
	if c.BucketCount <= 0 {
		c.BucketCount = 10
	}
	if c.ErrorRateThreshold <= 0 {
		c.ErrorRateThreshold = 0.5
	}
	if c.MinRequests <= 0 {
		c.MinRequests = 10
	}
	if c.LatencySampleSize <= 0 {
		c.LatencySampleSize = 256
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.HalfOpenSuccesses <= 0 {
		c.HalfOpenSuccesses = 2
	}
	if c.HalfOpenMaxRequests <= 0 {
		c.HalfOpenMaxRequests = 1
	}
}

// AdaptiveBreaker is a thread-safe circuit breaker that trips based on
// error rate and/or P99 latency thresholds over a rolling time window.
type AdaptiveBreaker struct {
	mu  sync.Mutex
	cfg AdaptiveConfig

	state     State
	lastTrip  time.Time
	halfOKs   int64 // consecutive successes in HalfOpen
	halfTotal int64 // concurrent probes in HalfOpen

	// rolling time-window buckets
	buckets   []adaptiveBucket
	bucketDur time.Duration // duration of one bucket

	// latency ring buffer for P99
	latRing    []int64 // nanoseconds
	latHead    int
	latFilled  bool
	latScratch []int64       // reused sort buffer — avoids per-call allocation
	latDirty   bool          // true when new samples added since last p99()
	latCached  time.Duration // memoised P99; valid when !latDirty

	// test hook — override time.Now
	nowFn func() time.Time
}

// adaptiveBucket is one time-slot in the rolling window.
type adaptiveBucket struct {
	start  time.Time
	total  int64
	errors int64
}

// NewAdaptive creates a new AdaptiveBreaker with the given configuration.
func NewAdaptive(cfg AdaptiveConfig) *AdaptiveBreaker {
	cfg.applyDefaults()
	ab := &AdaptiveBreaker{
		cfg:       cfg,
		buckets:   make([]adaptiveBucket, cfg.BucketCount),
		bucketDur: cfg.Window / time.Duration(cfg.BucketCount),
		latRing:   make([]int64, cfg.LatencySampleSize),
		nowFn:     time.Now,
	}
	// Initialise all bucket start times to the zero value so they are
	// treated as expired (and reset) on the first request.
	return ab
}

// NewAdaptiveSimple creates an AdaptiveBreaker with default settings.
func NewAdaptiveSimple(name string) *AdaptiveBreaker {
	return NewAdaptive(AdaptiveConfig{Name: name})
}

// State returns the current circuit state.
func (ab *AdaptiveBreaker) State() State {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	return ab.currentState()
}

// AdaptiveStats returns a snapshot of the current breaker statistics.
type AdaptiveStats struct {
	Name        string
	State       State
	ErrorRate   float64
	P99Latency  time.Duration
	TotalReqs   int64
	ErrorReqs   int64
	LastTrip    time.Time
}

// Stats returns a snapshot of the rolling-window statistics.
func (ab *AdaptiveBreaker) Stats() AdaptiveStats {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	total, errs := ab.windowTotals()
	var errRate float64
	if total > 0 {
		errRate = float64(errs) / float64(total)
	}
	return AdaptiveStats{
		Name:       ab.cfg.Name,
		State:      ab.currentState(),
		ErrorRate:  errRate,
		P99Latency: ab.p99(),
		TotalReqs:  total,
		ErrorReqs:  errs,
		LastTrip:   ab.lastTrip,
	}
}

// Do executes fn within the adaptive circuit breaker.
// It measures execution time for latency tracking and counts errors.
func (ab *AdaptiveBreaker) Do(fn func() error) error {
	if err := ab.before(); err != nil {
		return err
	}

	start := ab.nowFn()
	err := fn()
	latency := ab.nowFn().Sub(start)

	ab.after(err, latency)
	return err
}

// ─── Internal state machine ───────────────────────────────────────────────────

// currentState advances Open→HalfOpen if the timeout has elapsed.
// Must be called with ab.mu held.
func (ab *AdaptiveBreaker) currentState() State {
	if ab.state == StateOpen && ab.nowFn().Sub(ab.lastTrip) >= ab.cfg.Timeout {
		ab.transition(StateHalfOpen)
	}
	return ab.state
}

func (ab *AdaptiveBreaker) before() error {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	switch ab.currentState() {
	case StateOpen:
		return ErrOpen
	case StateHalfOpen:
		if ab.halfTotal >= ab.cfg.HalfOpenMaxRequests {
			return ErrOpen
		}
		ab.halfTotal++
	}
	return nil
}

func (ab *AdaptiveBreaker) after(err error, latency time.Duration) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	// Always record stats regardless of state so the window is accurate.
	ab.record(err != nil, latency)

	switch ab.currentState() {
	case StateClosed:
		if ab.shouldTrip() {
			ab.trip()
		}
	case StateHalfOpen:
		if err != nil {
			ab.trip()
		} else {
			ab.halfOKs++
			if ab.halfOKs >= ab.cfg.HalfOpenSuccesses {
				ab.transition(StateClosed)
				ab.resetWindow()
			}
		}
	}
}

// shouldTrip evaluates whether the breaker should trip based on the current
// rolling window stats. Must be called with ab.mu held.
func (ab *AdaptiveBreaker) shouldTrip() bool {
	total, errs := ab.windowTotals()
	if total < ab.cfg.MinRequests {
		return false
	}

	errorRate := float64(errs) / float64(total)
	if errorRate >= ab.cfg.ErrorRateThreshold {
		return true
	}

	if ab.cfg.LatencyThreshold > 0 {
		if ab.p99() >= ab.cfg.LatencyThreshold {
			return true
		}
	}

	return false
}

func (ab *AdaptiveBreaker) trip() {
	ab.lastTrip = ab.nowFn()
	ab.transition(StateOpen)
}

func (ab *AdaptiveBreaker) transition(to State) {
	from := ab.state
	if from == to {
		return
	}
	ab.state = to
	ab.halfOKs = 0
	ab.halfTotal = 0
	if ab.cfg.OnStateChange != nil {
		go ab.cfg.OnStateChange(ab.cfg.Name, from, to)
	}
}

// ─── Rolling window ───────────────────────────────────────────────────────────

// record adds a data point to the current bucket, rotating if necessary.
// Must be called with ab.mu held.
func (ab *AdaptiveBreaker) record(isErr bool, latency time.Duration) {
	now := ab.nowFn()
	ab.rotateBuckets(now)

	// The current bucket is always at index 0 (after rotation).
	ab.buckets[0].total++
	if isErr {
		ab.buckets[0].errors++
	}

	// Record latency in the ring buffer.
	if latency > 0 {
		ab.latRing[ab.latHead] = latency.Nanoseconds()
		ab.latHead = (ab.latHead + 1) % len(ab.latRing)
		if ab.latHead == 0 {
			ab.latFilled = true
		}
		ab.latDirty = true
	}
}

// rotateBuckets advances expired buckets and ensures bucket[0] covers now.
// Must be called with ab.mu held.
func (ab *AdaptiveBreaker) rotateBuckets(now time.Time) {
	if ab.buckets[0].start.IsZero() {
		// First call — initialise the first bucket.
		ab.buckets[0].start = now
		return
	}

	// How many new buckets do we need?
	elapsed := now.Sub(ab.buckets[0].start)
	newBuckets := int(elapsed / ab.bucketDur)
	if newBuckets == 0 {
		return // still within the current bucket
	}

	if newBuckets >= len(ab.buckets) {
		// Entire window expired — reset everything.
		ab.resetWindow()
		ab.buckets[0].start = now
		return
	}

	// Rotate: shift existing buckets right, prepend new empty bucket(s).
	shift := min(newBuckets, len(ab.buckets))
	copy(ab.buckets[shift:], ab.buckets[:len(ab.buckets)-shift])
	for i := range shift {
		ab.buckets[i] = adaptiveBucket{
			start: ab.buckets[shift].start.Add(time.Duration(shift-i) * ab.bucketDur),
		}
	}
	ab.buckets[0].start = now
}

// windowTotals sums total and error counts across all live buckets.
// Must be called with ab.mu held.
func (ab *AdaptiveBreaker) windowTotals() (total, errors int64) {
	cutoff := ab.nowFn().Add(-ab.cfg.Window)
	for _, b := range ab.buckets {
		if b.start.IsZero() || b.start.Before(cutoff) {
			continue
		}
		total += b.total
		errors += b.errors
	}
	return
}

// resetWindow zeroes all bucket counters and the latency ring buffer.
// Must be called with ab.mu held.
func (ab *AdaptiveBreaker) resetWindow() {
	for i := range ab.buckets {
		ab.buckets[i] = adaptiveBucket{}
	}
	for i := range ab.latRing {
		ab.latRing[i] = 0
	}
	ab.latHead = 0
	ab.latFilled = false
	ab.latDirty = false
	ab.latCached = 0
}

// ─── P99 latency ─────────────────────────────────────────────────────────────

// p99 returns the 99th-percentile latency from the ring buffer.
// Must be called with ab.mu held.
func (ab *AdaptiveBreaker) p99() time.Duration {
	if !ab.latDirty {
		return ab.latCached
	}

	n := ab.latHead
	if ab.latFilled {
		n = len(ab.latRing)
	}
	if n == 0 {
		ab.latDirty = false
		ab.latCached = 0
		return 0
	}

	// Grow the scratch buffer only when needed; reuse otherwise to avoid allocation.
	if cap(ab.latScratch) < n {
		ab.latScratch = make([]int64, n)
	}
	scratch := ab.latScratch[:n]
	if ab.latFilled {
		copy(scratch, ab.latRing)
	} else {
		copy(scratch, ab.latRing[:n])
	}

	slices.Sort(scratch)

	idx := max(int(math.Ceil(float64(n)*0.99))-1, 0)
	if idx >= n {
		idx = n - 1
	}

	ab.latCached = time.Duration(scratch[idx])
	ab.latDirty = false
	return ab.latCached
}

// ─── Astra Middleware ─────────────────────────────────────────────────────────

// Middleware returns an Astra middleware that wraps each request with the
// adaptive circuit breaker.
//
//   - HTTP 5xx responses are counted as errors.
//   - Request latency (from handler start to response) is recorded for P99.
//   - When the circuit is open, 503 Service Unavailable is returned immediately.
func (ab *AdaptiveBreaker) Middleware() astra.MiddlewareFunc {
	return func(c *astra.Ctx) error {
		if err := ab.before(); err != nil {
			return astra.NewHTTPError(http.StatusServiceUnavailable,
				fmt.Sprintf("circuit breaker [%s] is open", ab.cfg.Name))
		}

		start := ab.nowFn()
		c.Next()
		latency := ab.nowFn().Sub(start)

		status := c.Writer().Status()
		var callErr error
		if status >= 500 {
			callErr = fmt.Errorf("http %d", status)
		}

		ab.after(callErr, latency)
		return nil
	}
}
