// Package circuit provides circuit breaker patterns for fault tolerance.
//
// The package exposes two breaker types:
//
//   - [Breaker] — count-based: opens after N consecutive failures.
//   - [AdaptiveBreaker] — statistical: trips on error rate or P99 latency thresholds.
//
// Use [BreakerBuilder] for a fluent builder for [Config], and [AdaptiveBreakerBuilder]
// for a fluent builder for [AdaptiveConfig].
package circuit

import "time"

// BreakerBuilder provides a fluent builder for [Config].
//
// Example:
//
//	cb := NewBreakerBuilder("payment-svc").
//		WithFailureThreshold(10).
//		WithSuccessThreshold(3).
//		WithTimeout(30 * time.Second).
//		WithOnStateChange(func(name string, from, to State) {
//			log.Printf("circuit %s: %v → %v", name, from, to)
//		}).
//		Build()
type BreakerBuilder struct {
	cfg Config
}

// NewBreakerBuilder creates a builder with the given name.
func NewBreakerBuilder(name string) *BreakerBuilder {
	return &BreakerBuilder{cfg: Config{Name: name}}
}

// WithFailureThreshold sets the consecutive failure count before opening.
// Defaults to 5 if not set.
func (b *BreakerBuilder) WithFailureThreshold(n int) *BreakerBuilder {
	b.cfg.Threshold = int64(n)
	return b
}

// WithSuccessThreshold sets the consecutive success count to close from half-open.
// Defaults to 2 if not set.
func (b *BreakerBuilder) WithSuccessThreshold(n int) *BreakerBuilder {
	b.cfg.HalfOpenSuccesses = int64(n)
	return b
}

// WithTimeout sets how long the circuit stays open before transitioning to half-open.
// Defaults to 30s if not set.
func (b *BreakerBuilder) WithTimeout(d time.Duration) *BreakerBuilder {
	b.cfg.Timeout = d
	return b
}

// WithHalfOpenMaxRequests limits concurrent probes in half-open state.
// Defaults to 1 if not set.
func (b *BreakerBuilder) WithHalfOpenMaxRequests(n int) *BreakerBuilder {
	b.cfg.HalfOpenMaxRequests = int64(n)
	return b
}

// WithOnStateChange sets the callback invoked on every state transition.
func (b *BreakerBuilder) WithOnStateChange(fn func(name string, from, to State)) *BreakerBuilder {
	b.cfg.OnStateChange = fn
	return b
}

// Build creates a new [Breaker] with the configured settings.
func (b *BreakerBuilder) Build() *Breaker {
	return New(b.cfg)
}

// Config returns the underlying [Config] for inspection or reuse.
func (b *BreakerBuilder) Config() Config {
	return b.cfg
}

// AdaptiveBreakerBuilder provides a fluent builder for [AdaptiveConfig].
// AdaptiveBreaker trips on error rate or P99 latency, not just consecutive failures.
//
// Example:
//
//	ab := NewAdaptiveBreakerBuilder("order-svc").
//		WithErrorRateThreshold(0.5).
//		WithMinRequests(10).
//		WithLatencyThreshold(2 * time.Second).
//		WithWindow(10 * time.Second).
//		Build()
type AdaptiveBreakerBuilder struct {
	cfg AdaptiveConfig
}

// NewAdaptiveBreakerBuilder creates a builder with the given name.
func NewAdaptiveBreakerBuilder(name string) *AdaptiveBreakerBuilder {
	return &AdaptiveBreakerBuilder{cfg: AdaptiveConfig{Name: name}}
}

// WithErrorRateThreshold sets the error fraction [0.0, 1.0] that trips the circuit.
// Defaults to 0.5 if not set.
func (b *AdaptiveBreakerBuilder) WithErrorRateThreshold(f float64) *AdaptiveBreakerBuilder {
	b.cfg.ErrorRateThreshold = f
	return b
}

// WithMinRequests sets the minimum requests in the rolling window before evaluating.
// Defaults to 10 if not set.
func (b *AdaptiveBreakerBuilder) WithMinRequests(n int) *AdaptiveBreakerBuilder {
	b.cfg.MinRequests = int64(n)
	return b
}

// WithLatencyThreshold sets the P99 latency threshold that trips the circuit.
// 0 = disabled (default).
func (b *AdaptiveBreakerBuilder) WithLatencyThreshold(d time.Duration) *AdaptiveBreakerBuilder {
	b.cfg.LatencyThreshold = d
	return b
}

// WithWindow sets the rolling stats window duration.
// Defaults to 10s if not set.
func (b *AdaptiveBreakerBuilder) WithWindow(d time.Duration) *AdaptiveBreakerBuilder {
	b.cfg.Window = d
	return b
}

// WithBucketCount sets the number of time buckets within the window.
// More buckets = finer granularity, slightly higher memory.
// Defaults to 10 if not set.
func (b *AdaptiveBreakerBuilder) WithBucketCount(n int) *AdaptiveBreakerBuilder {
	b.cfg.BucketCount = n
	return b
}

// WithTimeout sets how long the circuit stays open before transitioning to half-open.
// Defaults to 30s if not set.
func (b *AdaptiveBreakerBuilder) WithTimeout(d time.Duration) *AdaptiveBreakerBuilder {
	b.cfg.Timeout = d
	return b
}

// WithSuccessThreshold sets consecutive successes needed to close from half-open.
// Defaults to 2 if not set.
func (b *AdaptiveBreakerBuilder) WithSuccessThreshold(n int) *AdaptiveBreakerBuilder {
	b.cfg.HalfOpenSuccesses = int64(n)
	return b
}

// WithHalfOpenMaxRequests limits concurrent probes in half-open state.
// Defaults to 1 if not set.
func (b *AdaptiveBreakerBuilder) WithHalfOpenMaxRequests(n int) *AdaptiveBreakerBuilder {
	b.cfg.HalfOpenMaxRequests = int64(n)
	return b
}

// WithLatencySampleSize sets the ring buffer size for P99 latency computation.
// Defaults to 256 if not set.
func (b *AdaptiveBreakerBuilder) WithLatencySampleSize(n int) *AdaptiveBreakerBuilder {
	b.cfg.LatencySampleSize = n
	return b
}

// WithOnStateChange sets the callback invoked on every state transition.
func (b *AdaptiveBreakerBuilder) WithOnStateChange(fn func(name string, from, to State)) *AdaptiveBreakerBuilder {
	b.cfg.OnStateChange = fn
	return b
}

// Build creates a new [AdaptiveBreaker] with the configured settings.
func (b *AdaptiveBreakerBuilder) Build() *AdaptiveBreaker {
	return NewAdaptive(b.cfg)
}

// Config returns the underlying [AdaptiveConfig] for inspection or reuse.
func (b *AdaptiveBreakerBuilder) Config() AdaptiveConfig {
	return b.cfg
}
