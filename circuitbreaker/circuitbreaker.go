// Package circuitbreaker implements a thread-safe circuit breaker for the
// Astra framework, inspired by the design of sony/gobreaker.
//
// A circuit breaker protects a downstream dependency from being overwhelmed
// while it is failing. It has three states:
//
//   - Closed    — requests pass through to the protected function normally.
//   - Open      — requests are rejected immediately with ErrOpen.
//   - HalfOpen  — a limited number of "probe" requests are let through to
//     test whether the dependency has recovered.
//
// # State machine
//
//	Closed    → Open      when failures reach the MaxFailures threshold
//	                        (or a custom ReadyToTrip predicate fires).
//	Open      → HalfOpen  after Interval elapses since the circuit opened.
//	HalfOpen  → Closed    after SuccessThreshold consecutive successful probes.
//	HalfOpen  → Open      on any probe failure, or if the HalfOpen Timeout
//	                        expires before the circuit can close.
//
// # Configuration (defaults in brackets)
//
//	MaxFailures     [5]  consecutive failures that trip Closed → Open.
//	Interval        [30s] time the circuit stays Open before probing HalfOpen.
//	                  (a.k.a. "half-open attempt interval": how long to wait
//	                  between recovery attempts.)
//	Timeout         [60s] maximum time the circuit may stay HalfOpen before it
//	                  is forced back to Open (re-armed). 0 disables the timeout.
//	SuccessThreshold [2] consecutive successful probes needed to close.
//	MaxRequests     [1]  max concurrent probes allowed while HalfOpen.
//
// # Concurrency model
//
// The breaker keeps its entire state in a single atomic snapshot
// (sync/atomic.Value) and performs all transitions via compare-and-swap loops,
// so it needs no mutex and no external dependencies. Counters and the
// in-flight probe count are carried inside that snapshot, which makes every
// transition an atomic, race-free operation.
//
// # Prometheus metrics (optional)
//
// The core package depends only on the standard library. Prometheus metrics
// are provided in metrics_prometheus.go, which is built only when the
// `prometheus` build tag is enabled:
//
//	go build -tags prometheus ./...
//	go test  -tags prometheus ./circuitbreaker/...
//
// Pass WithPrometheus(prometheus.DefaultRegisterer) to wire counters/gauges.
package circuitbreaker

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// ErrOpen is returned by Call when the circuit is open (or the HalfOpen probe
// limit has been reached) and the request is rejected without running fn.
var ErrOpen = errors.New("circuit breaker open: request rejected")

// errPanic is used internally to record a panicking fn in afterRequest.
var errPanic = errors.New("circuit breaker: fn panicked")

// Default* values used when a Settings field is left at its zero value.
const (
	DefaultMaxFailures     uint32 = 5
	DefaultInterval                = 30 * time.Second
	DefaultTimeout                 = 60 * time.Second
	DefaultSuccessThreshold uint32 = 2
	DefaultMaxRequests       uint32 = 1
)

// State is the current state of a circuit breaker.
type State int32

const (
	// StateClosed — normal operation; requests pass through.
	StateClosed State = iota
	// StateOpen — circuit is tripped; requests are rejected with ErrOpen.
	StateOpen
	// StateHalfOpen — recovery probe phase; a limited number of requests pass.
	StateHalfOpen
)

// String returns the lower-case name of the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Counts holds the running statistics of a breaker.
type Counts struct {
	Requests             int64 // total calls admitted (success + failure).
	TotalSuccesses       int64
	TotalFailures        int64
	ConsecutiveSuccesses int64
	ConsecutiveFailures  int64
}

// Settings configures a Breaker. Use the With* option functions or build a
// Settings value directly; zero-valued fields are filled with Default* values.
type Settings struct {
	// Name is a human-readable identifier, used in logs, callbacks and metrics.
	Name string

	// MaxFailures is the number of consecutive failures in Closed state that
	// trip the breaker (Closed → Open). Default 5.
	MaxFailures uint32

	// Interval is how long the circuit stays Open before it attempts a HalfOpen
	// probe — i.e. the interval between recovery attempts. Default 30s.
	// A value <= 0 means "never open" (the breaker will not auto-recover).
	Interval time.Duration

	// Timeout is the maximum time the breaker may remain HalfOpen before it is
	// forced back to Open and re-armed. Default 60s. 0 disables the timeout.
	Timeout time.Duration

	// SuccessThreshold is the number of consecutive successful probes required
	// to close the breaker from HalfOpen. Default 2.
	SuccessThreshold uint32

	// MaxRequests is the maximum number of concurrent probes permitted while
	// the breaker is HalfOpen. Default 1. 0 is treated as 1.
	MaxRequests uint32

	// ReadyToTrip, if set, overrides the default trip logic. It is consulted on
	// every Closed-state failure; return true to trip the breaker. The default
	// trips when ConsecutiveFailures >= MaxFailures.
	ReadyToTrip func(Counts) bool

	// IsSuccessful classifies the error returned by fn. Return true to count
	// the call as a success. Default: err == nil.
	IsSuccessful func(error) bool

	// OnStateChange, if set, is invoked (in its own goroutine) after every
	// state transition, with the breaker name and the from/to states.
	OnStateChange func(name string, from, to State)

	// metrics is an optional reporter for Prometheus (or any custom) metrics.
	// It is populated via WithMetrics / WithPrometheus and is otherwise nil.
	metrics MetricsReporter
}

// MetricsReporter receives lifecycle events from a Breaker. Implementations
// must be safe for concurrent use. The core package never imports an external
// metrics library; the Prometheus implementation lives in metrics_prometheus.go
// (built with -tags prometheus).
type MetricsReporter interface {
	// ReportState is called after a transition with the new state.
	ReportState(name string, s State)
	// ReportCall is called for every admitted/rejected call; result is one of
	// "success", "failure" or "rejected".
	ReportCall(name, result string)
	// ReportTransition is called once per state transition.
	ReportTransition(name string, from, to State)
}

// snapshot is the immutable, atomically-swapped unit of breaker state.
type snapshot struct {
	state           State
	counts          Counts
	inflight        int64 // probes currently running while HalfOpen.
	openExpiry      int64 // unix nano: earliest time Open may become HalfOpen.
	halfOpenEntered int64 // unix nano: time the breaker entered HalfOpen.
}

// Breaker is a circuit breaker. It implements the CircuitBreaker interface.
type Breaker struct {
	name     string
	settings Settings
	store    atomic.Value // stores *snapshot
}

// CircuitBreaker is the public contract for a circuit breaker.
type CircuitBreaker interface {
	// Call runs fn inside the breaker. It returns ErrOpen (without calling fn)
	// when the circuit is open or the HalfOpen probe limit is reached. The
	// context is checked before fn runs; if it is already done, ctx.Err() is
	// returned and fn is not executed.
	Call(ctx context.Context, fn func() error) error
	// State returns the current breaker state. The Open → HalfOpen transition
	// happens lazily on the next Call; State does not mutate the breaker.
	State() State
}

// Option configures a Breaker at construction time.
type Option func(*Settings)

// WithName sets the breaker name.
func WithName(name string) Option { return func(s *Settings) { s.Name = name } }

// WithMaxFailures sets the consecutive-failure trip threshold (Closed → Open).
func WithMaxFailures(n uint32) Option { return func(s *Settings) { s.MaxFailures = n } }

// WithInterval sets how long the breaker stays Open before probing HalfOpen.
func WithInterval(d time.Duration) Option { return func(s *Settings) { s.Interval = d } }

// WithTimeout sets the maximum HalfOpen duration before forcing back to Open.
func WithTimeout(d time.Duration) Option { return func(s *Settings) { s.Timeout = d } }

// WithSuccessThreshold sets the consecutive-success count needed to close.
func WithSuccessThreshold(n uint32) Option {
	return func(s *Settings) { s.SuccessThreshold = n }
}

// WithMaxRequests sets the max concurrent HalfOpen probes.
func WithMaxRequests(n uint32) Option { return func(s *Settings) { s.MaxRequests = n } }

// WithReadyToTrip overrides the default trip predicate.
func WithReadyToTrip(f func(Counts) bool) Option {
	return func(s *Settings) { s.ReadyToTrip = f }
}

// WithIsSuccessful overrides how fn's error is classified.
func WithIsSuccessful(f func(error) bool) Option {
	return func(s *Settings) { s.IsSuccessful = f }
}

// WithOnStateChange registers a transition callback.
func WithOnStateChange(f func(name string, from, to State)) Option {
	return func(s *Settings) { s.OnStateChange = f }
}

// WithMetrics attaches a custom MetricsReporter.
func WithMetrics(r MetricsReporter) Option {
	return func(s *Settings) { s.metrics = r }
}

// New constructs a Breaker with the supplied options and applies defaults.
func New(opts ...Option) *Breaker {
	s := Settings{Name: "circuit-breaker"} // default name set before opts.
	for _, o := range opts {
		o(&s)
	}
	applyDefaults(&s)

	b := &Breaker{name: s.Name, settings: s}
	b.store.Store(&snapshot{state: StateClosed})
	return b
}

func applyDefaults(s *Settings) {
	if s.MaxFailures == 0 {
		s.MaxFailures = DefaultMaxFailures
	}
	if s.Interval == 0 {
		s.Interval = DefaultInterval
	}
	if s.Timeout == 0 {
		s.Timeout = DefaultTimeout
	}
	if s.SuccessThreshold == 0 {
		s.SuccessThreshold = DefaultSuccessThreshold
	}
	if s.MaxRequests == 0 {
		s.MaxRequests = DefaultMaxRequests
	}
	if s.IsSuccessful == nil {
		s.IsSuccessful = func(err error) bool { return err == nil }
	}
	if s.ReadyToTrip == nil {
		threshold := int64(s.MaxFailures)
		s.ReadyToTrip = func(c Counts) bool { return c.ConsecutiveFailures >= threshold }
	}
}

// Name returns the breaker's configured name.
func (b *Breaker) Name() string { return b.name }

// Settings returns a copy of the effective settings.
func (b *Breaker) Settings() Settings { return b.settings }

// State returns the current breaker state (see the interface for semantics).
func (b *Breaker) State() State { return b.load().state }

// Counts returns a point-in-time copy of the breaker statistics.
func (b *Breaker) Counts() Counts { return b.load().counts }

// Reset forces the breaker back to the Closed state and clears all counters.
func (b *Breaker) Reset() {
	for {
		cur := b.load()
		ns := &snapshot{state: StateClosed}
		if b.store.CompareAndSwap(cur, ns) {
			return
		}
	}
}

// Call runs fn inside the breaker.
func (b *Breaker) Call(ctx context.Context, fn func() error) (err error) {
	if err := ctx.Err(); err != nil {
		b.reportCall("rejected")
		return err
	}
	if err := b.beforeRequest(); err != nil {
		return err
	}

	// Recover from panics in fn to prevent inflight counter leaks during
	// HalfOpen probes. The panic is re-thrown after cleanup so callers can
	// still observe the original panic with a recovery middleware.
	defer func() {
		if r := recover(); r != nil {
			b.afterRequest(errPanic)
			panic(r)
		}
	}()

	err = fn()
	b.afterRequest(err)
	return err
}

// beforeRequest performs admission control and time-based transitions. It
// returns ErrOpen (without running fn) when the request must be rejected.
func (b *Breaker) beforeRequest() error {
	for {
		cur := b.load()
		now := time.Now().UnixNano()
		ns := copySnapshot(cur)
		changed := false
		transitioned := false

		switch cur.state {
		case StateOpen:
			if b.settings.Interval <= 0 || now >= cur.openExpiry {
				// Open → HalfOpen: this call becomes the first probe.
				ns.state = StateHalfOpen
				ns.halfOpenEntered = now
				ns.counts.ConsecutiveFailures = 0
				ns.counts.ConsecutiveSuccesses = 0
				ns.inflight = 1
				changed = true
				transitioned = true
			} else {
				return b.reject()
			}
		case StateHalfOpen:
			if b.settings.Timeout > 0 && now-cur.halfOpenEntered >= b.settings.Timeout.Nanoseconds() {
				// HalfOpen timed out → force back to Open and re-arm.
				// Persist the transition, then reject this request.
				ns.state = StateOpen
				ns.openExpiry = now + b.settings.Interval.Nanoseconds()
				ns.halfOpenEntered = 0
				ns.counts.ConsecutiveFailures = 0
				ns.counts.ConsecutiveSuccesses = 0
				ns.inflight = 0
				changed = true
				transitioned = true

				// Do CAS and reject atomically
				if b.store.CompareAndSwap(cur, ns) {
					b.fireTransition(cur.state, ns.state)
				}
				return ErrOpen
			}
		}

		if ns.state == StateHalfOpen && !transitioned {
			// Normal HalfOpen probe admission with a concurrency limit.
			next := cur.inflight + 1
			if b.settings.MaxRequests > 0 && next > int64(b.settings.MaxRequests) {
				return b.reject()
			}
			ns.inflight = next
			changed = true
		}

		if !changed {
			return nil // Closed state, admitted, nothing to persist.
		}
		if b.store.CompareAndSwap(cur, ns) {
			if transitioned {
				b.fireTransition(cur.state, ns.state)
			}
			return nil
		}
		// Lost the race; reload and recompute.
	}
}

// afterRequest classifies the result, updates counters, and performs
// Closed→Open / HalfOpen→Closed / HalfOpen→Open transitions.
func (b *Breaker) afterRequest(err error) {
	success := b.settings.IsSuccessful(err)
	result := "success"
	if !success {
		result = "failure"
	}

	old, new := b.update(func(cur *snapshot) *snapshot {
		ns := copySnapshot(cur)
		ns.counts.Requests++
		if success {
			ns.counts.TotalSuccesses++
			ns.counts.ConsecutiveSuccesses++
			ns.counts.ConsecutiveFailures = 0
		} else {
			ns.counts.TotalFailures++
			ns.counts.ConsecutiveFailures++
			ns.counts.ConsecutiveSuccesses = 0
		}

		switch cur.state {
		case StateClosed:
			if b.settings.ReadyToTrip(ns.counts) {
				ns.state = StateOpen
				ns.openExpiry = time.Now().Add(b.settings.Interval).UnixNano()
				ns.counts.ConsecutiveFailures = 0
				ns.counts.ConsecutiveSuccesses = 0
				ns.inflight = 0
				ns.halfOpenEntered = 0
			}
		case StateHalfOpen:
			if success {
				if ns.counts.ConsecutiveSuccesses >= int64(b.settings.SuccessThreshold) {
					ns.state = StateClosed
					ns.counts.ConsecutiveFailures = 0
					ns.counts.ConsecutiveSuccesses = 0
					ns.inflight = 0
				}
			} else {
				ns.state = StateOpen
				ns.openExpiry = time.Now().Add(b.settings.Interval).UnixNano()
				ns.counts.ConsecutiveFailures = 0
				ns.counts.ConsecutiveSuccesses = 0
				ns.inflight = 0
				ns.halfOpenEntered = 0
			}
		case StateOpen:
			// Admissions cannot reach here; nothing to do.
		}

		// Release a HalfOpen probe slot (clamped at zero).
		if cur.state == StateHalfOpen {
			if ns.inflight > 0 {
				ns.inflight--
			} else {
				ns.inflight = 0
			}
		}
		return ns
	})

	b.reportCall(result)
	if old.state != new.state {
		b.fireTransition(old.state, new.state)
	}
}

// update applies fn to the current snapshot and commits it via CAS, retrying
// until it succeeds. fn must be a pure function of its input. It returns the
// old and new snapshots (equal when no commit was needed).
func (b *Breaker) update(fn func(cur *snapshot) *snapshot) (old, new *snapshot) {
	for {
		cur := b.load()
		ns := fn(cur)
		if b.store.CompareAndSwap(cur, ns) {
			return cur, ns
		}
	}
}

func (b *Breaker) load() *snapshot  { return b.store.Load().(*snapshot) }
func (b *Breaker) reject() error    { b.reportCall("rejected"); return ErrOpen }

func (b *Breaker) reportCall(result string) {
	if b.settings.metrics != nil {
		b.settings.metrics.ReportCall(b.name, result)
	}
}

func (b *Breaker) fireTransition(from, to State) {
	if b.settings.metrics != nil {
		b.settings.metrics.ReportTransition(b.name, from, to)
		b.settings.metrics.ReportState(b.name, to)
	}
	if b.settings.OnStateChange != nil {
		name := b.name
		fn := b.settings.OnStateChange
		go fn(name, from, to)
	}
}

func copySnapshot(s *snapshot) *snapshot {
	c := *s
	return &c
}
