// Package circuit implements a thread-safe circuit breaker for Astra.
//
// The circuit breaker has three states:
//
//   - Closed  — requests pass through normally.
//   - Open    — requests are immediately rejected with ErrOpen.
//   - HalfOpen — a limited probe of requests pass through to test recovery.
//
// State transitions:
//
//	Closed  → Open      when consecutive failures ≥ Threshold
//	Open    → HalfOpen  after Timeout elapses
//	HalfOpen → Closed   when HalfOpenSuccesses succeed
//	HalfOpen → Open     on any failure
//
// Inspired by go-zero's breaker and the Netflix Hystrix design.
package circuit

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/astra-go/astra"
)

// ErrOpen is returned when the circuit is open and requests are rejected.
var ErrOpen = errors.New("circuit breaker open: service unavailable")

// State represents the circuit breaker state.
type State int

const (
	// StateClosed — normal operation, requests pass through.
	StateClosed State = iota
	// StateOpen — circuit is tripped, requests are rejected.
	StateOpen
	// StateHalfOpen — probe phase, limited requests pass through.
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	}
	return "unknown"
}

// Config holds the circuit breaker configuration.
type Config struct {
	// Name is a human-readable identifier for this breaker (used in logs/metrics).
	Name string
	// Threshold is the number of consecutive failures before opening the circuit.
	// Default: 5
	Threshold int64
	// Timeout is how long the circuit stays open before switching to half-open.
	// Default: 30s
	Timeout time.Duration
	// HalfOpenSuccesses is how many consecutive successes close the circuit from half-open.
	// Default: 2
	HalfOpenSuccesses int64
	// HalfOpenMaxRequests limits concurrent probes in half-open state.
	// Default: 1
	HalfOpenMaxRequests int64
	// OnStateChange is called (in a goroutine) whenever the state changes.
	OnStateChange func(name string, from, to State)
}

func (c *Config) applyDefaults() {
	if c.Threshold == 0 {
		c.Threshold = 5
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.HalfOpenSuccesses == 0 {
		c.HalfOpenSuccesses = 2
	}
	if c.HalfOpenMaxRequests == 0 {
		c.HalfOpenMaxRequests = 1
	}
	if c.Name == "" {
		c.Name = "default"
	}
}

// Breaker is a thread-safe circuit breaker.
type Breaker struct {
	mu          sync.Mutex
	cfg         Config
	state       State
	failures    int64     // consecutive failures in Closed state
	successes   int64     // consecutive successes in HalfOpen state
	halfOpens   int64     // concurrent probes in HalfOpen state
	lastFailure time.Time // time of last failure (used for Open→HalfOpen transition)
}

// New creates a new Breaker with the given configuration.
func New(cfg Config) *Breaker {
	cfg.applyDefaults()
	return &Breaker{cfg: cfg}
}

// NewSimple creates a Breaker with sensible defaults.
func NewSimple(name string) *Breaker {
	return New(Config{Name: name})
}

// State returns the current circuit state (safe for concurrent use).
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.currentState()
}

// currentState returns the state, advancing Open→HalfOpen if the timeout elapsed.
// Must be called with b.mu held.
func (b *Breaker) currentState() State {
	if b.state == StateOpen && time.Since(b.lastFailure) >= b.cfg.Timeout {
		b.transition(StateHalfOpen)
	}
	return b.state
}

// transition moves to a new state and resets counters.
// Must be called with b.mu held.
func (b *Breaker) transition(to State) {
	from := b.state
	if from == to {
		return
	}
	b.state = to
	b.failures = 0
	b.successes = 0
	b.halfOpens = 0
	if b.cfg.OnStateChange != nil {
		go b.cfg.OnStateChange(b.cfg.Name, from, to)
	}
}

// Do executes fn within the circuit breaker.
// Returns ErrOpen if the circuit is open.
// Any error returned by fn counts as a failure.
func (b *Breaker) Do(fn func() error) error {
	if err := b.beforeRequest(); err != nil {
		return err
	}

	err := fn()

	b.afterRequest(err)
	return err
}

func (b *Breaker) beforeRequest() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.currentState() {
	case StateOpen:
		return ErrOpen
	case StateHalfOpen:
		if b.halfOpens >= b.cfg.HalfOpenMaxRequests {
			return ErrOpen
		}
		b.halfOpens++
	}
	return nil
}

func (b *Breaker) afterRequest(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err != nil {
		b.onFailure()
	} else {
		b.onSuccess()
	}
}

func (b *Breaker) onSuccess() {
	switch b.state {
	case StateClosed:
		b.failures = 0
	case StateHalfOpen:
		b.successes++
		if b.successes >= b.cfg.HalfOpenSuccesses {
			b.transition(StateClosed)
		}
	}
}

func (b *Breaker) onFailure() {
	b.lastFailure = time.Now()
	switch b.state {
	case StateClosed:
		b.failures++
		if b.failures >= b.cfg.Threshold {
			b.transition(StateOpen)
		}
	case StateHalfOpen:
		b.transition(StateOpen)
	}
}

// Stats returns a snapshot of the current breaker statistics.
func (b *Breaker) Stats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Stats{
		Name:        b.cfg.Name,
		State:       b.currentState(),
		Failures:    b.failures,
		Successes:   b.successes,
		LastFailure: b.lastFailure,
	}
}

// Stats holds a point-in-time snapshot of breaker state.
type Stats struct {
	Name        string
	State       State
	Failures    int64
	Successes   int64
	LastFailure time.Time
}

// ─── Astra Middleware ─────────────────────────────────────────────────────────

// Middleware returns an Astra middleware that wraps each request with the circuit breaker.
// HTTP 5xx responses from downstream handlers count as failures.
func (b *Breaker) Middleware() astra.MiddlewareFunc {
	return func(c *astra.Ctx) error {
		if err := b.beforeRequest(); err != nil {
			return astra.NewHTTPError(http.StatusServiceUnavailable,
				fmt.Sprintf("circuit breaker [%s] is %s", b.cfg.Name, b.state))
		}

		c.Next()

		// Count 5xx responses as failures
		status := c.Writer().Status()
		if status >= 500 {
			b.afterRequest(fmt.Errorf("http %d", status))
		} else {
			b.afterRequest(nil)
		}
		return nil
	}
}

// WrapHandler wraps a standard http.Handler with circuit breaker protection.
func (b *Breaker) WrapHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := b.Do(func() error {
			next.ServeHTTP(w, r)
			return nil
		})
		if errors.Is(err, ErrOpen) {
			http.Error(w, "Service Unavailable (circuit open)", http.StatusServiceUnavailable)
		}
	})
}
