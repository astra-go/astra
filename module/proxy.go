package module

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Proxy provides safe inter-module communication with built-in
// circuit-breaking, timeout, and retry support.
//
// Each Proxy is bound to a specific service type and name. Use the registry's
// GetProxy method to create one.
//
//	proxy := registry.GetProxy[*UserService]("service")
//	user, err := proxy.Call(ctx, 5*time.Second, func(svc *UserService) (*User, error) {
//	    return svc.Get(ctx, userID)
//	})
type Proxy[T any] struct {
	name    string
	resolve func() (T, error)

	mu         sync.Mutex
	circuit    *CircuitState
	retries    int
}

// ProxyOption configures a Proxy.
type ProxyOption func(*proxyConfig)

type proxyConfig struct {
	retries        int
	breakerEnabled bool
	breakerThreshold int
	breakerTimeout  time.Duration
}

// WithRetries sets the number of retries on failure (default: 0).
func WithRetries(n int) ProxyOption {
	return func(c *proxyConfig) { c.retries = n }
}

// WithCircuitBreaker enables circuit breaking with the given failure threshold
// and recovery timeout.
func WithCircuitBreaker(threshold int, timeout time.Duration) ProxyOption {
	return func(c *proxyConfig) {
		c.breakerEnabled = true
		c.breakerThreshold = threshold
		c.breakerTimeout = timeout
	}
}

// CircuitState tracks open/half-open/closed states.
type CircuitState struct {
	failures   int
	threshold int
	timeout    time.Duration
	state      string // "closed", "open", "half-open"
	lastFail   time.Time
}

func newCircuitState(threshold int, timeout time.Duration) *CircuitState {
	return &CircuitState{
		threshold: threshold,
		timeout:   timeout,
		state:     "closed",
	}
}

func (cs *CircuitState) allow() bool {
	switch cs.state {
	case "closed":
		return true
	case "open":
		if time.Since(cs.lastFail) > cs.timeout {
			cs.state = "half-open"
			return true
		}
		return false
	case "half-open":
		return true
	default:
		return true
	}
}

func (cs *CircuitState) recordSuccess() {
	cs.failures = 0
	cs.state = "closed"
}

func (cs *CircuitState) recordFailure() {
	cs.failures++
	cs.lastFail = time.Now()
	if cs.failures >= cs.threshold {
		cs.state = "open"
	}
}

// Call executes fn with the resolved service, applying timeout and retries.
func (p *Proxy[T]) Call(ctx context.Context, timeout time.Duration, fn func(svc T) error) error {
	return p.callWithRetry(ctx, timeout, fn, p.retries)
}

// Get resolves and returns the service directly (no timeout/retry).
func (p *Proxy[T]) Get(ctx context.Context) (T, error) {
	return p.resolve()
}

// State returns the current circuit breaker state, or "" if disabled.
// It automatically transitions from open to half-open if the timeout has elapsed.
func (p *Proxy[T]) State() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.circuit == nil {
		return ""
	}
	// Auto-transition open → half-open
	if p.circuit.state == "open" && time.Since(p.circuit.lastFail) > p.circuit.timeout {
		p.circuit.state = "half-open"
	}
	return p.circuit.state
}

func (p *Proxy[T]) callWithRetry(ctx context.Context, timeout time.Duration, fn func(svc T) error, retries int) error {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			// Simple linear backoff: 100ms * attempt
			backoff := time.Duration(100*attempt) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Circuit breaker check
		p.mu.Lock()
		if p.circuit != nil && !p.circuit.allow() {
			p.mu.Unlock()
			return fmt.Errorf("module %q: circuit breaker open", p.name)
		}
		p.mu.Unlock()

		// Timeout wrapper
		tctx, cancel := context.WithTimeout(ctx, timeout)
		err := p.doCall(tctx, fn, cancel)
		if err == nil {
			p.mu.Lock()
			if p.circuit != nil {
				p.circuit.recordSuccess()
			}
			p.mu.Unlock()
			return nil
		}

		lastErr = err
		p.mu.Lock()
		if p.circuit != nil {
			p.circuit.recordFailure()
		}
		p.mu.Unlock()

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("module %q: all %d retries failed, last: %w", p.name, retries+1, lastErr)
}

func (p *Proxy[T]) doCall(ctx context.Context, fn func(svc T) error, cancel context.CancelFunc) error {
	svc, err := p.resolve()
	if err != nil {
		cancel()
		return fmt.Errorf("module %q: resolve: %w", p.name, err)
	}
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- fn(svc)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}
