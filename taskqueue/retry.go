package taskqueue

import (
	"math/rand/v2"
	"time"
)

// RetryPolicy controls how many times and how quickly a failed task is retried.
//
// This differs from mq.MaxDeliver in that it supports exponential backoff
// and a hard ceiling on total retry time, giving more predictable behavior
// for long-running tasks.
type RetryPolicy struct {
	// MaxRetries is the maximum number of retry attempts after the initial
	// attempt. 0 means no retries. Default: 3.
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// BackoffBase is the base delay in seconds for exponential backoff.
	// The delay for attempt n is: BackoffBase * BackoffMultiplier^(n-1).
	// Default: 5 seconds.
	BackoffBase int `yaml:"backoff_base" json:"backoff_base"`

	// BackoffMultiplier multiplies the base delay on each retry.
	// Default: 3.
	BackoffMultiplier int `yaml:"backoff_multiplier" json:"backoff_multiplier"`

	// BackoffMax is the maximum delay cap in seconds.
	// Default: 300 seconds (5 minutes).
	BackoffMax int `yaml:"backoff_max" json:"backoff_max"`

	// BackoffJitter enables ±25% random jitter to prevent thundering herds.
	// Default: true.
	BackoffJitter bool `yaml:"backoff_jitter" json:"backoff_jitter"`
}

// DefaultRetryPolicy returns sensible defaults for background tasks.
var DefaultRetryPolicy = RetryPolicy{
	MaxRetries:        3,
	BackoffBase:       5,
	BackoffMultiplier: 3,
	BackoffMax:        300,
	BackoffJitter:     true,
}

// RetryDelay computes the delay before attempt number n (1-indexed).
// Returns 0 if n > MaxRetries.
func (p RetryPolicy) RetryDelay(n int) time.Duration {
	if n < 1 || n > p.MaxRetries {
		return 0
	}

	delay := p.BackoffBase
	for i := 1; i < n; i++ {
		delay *= p.BackoffMultiplier
	}

	if delay > p.BackoffMax {
		delay = p.BackoffMax
	}

	d := time.Duration(delay) * time.Second

	if p.BackoffJitter {
		// ±25% jitter using math/rand/v2 (not for security)
		jitter := float64(d) * 0.25
		d += time.Duration(float64(-jitter) + float64(jitter*2)*rand.Float64())
	}

	return d
}

// ShouldRetry reports whether a retry attempt n should proceed.
func (p RetryPolicy) ShouldRetry(attempt int) bool {
	return attempt <= p.MaxRetries
}

// MaxDeliver returns the equivalent JetStream MaxDeliver count.
func (p RetryPolicy) MaxDeliver() int {
	if p.MaxRetries <= 0 {
		return 1
	}
	return p.MaxRetries + 1 // +1 for the initial attempt
}
