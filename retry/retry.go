// Package retry provides configurable retry logic with exponential backoff and jitter.
//
// Usage:
//
//	policy := retry.Policy{
//	    MaxAttempts: 3,
//	    Delay:       100 * time.Millisecond,
//	    MaxDelay:    5 * time.Second,
//	    Multiplier:  2.0,
//	    Jitter:      true,
//	}
//	err := retry.Do(ctx, policy, func(ctx context.Context) error {
//	    return callRemoteService(ctx)
//	})
package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"time"
)

// ─── Policy ───────────────────────────────────────────────────────────────────

// Policy defines how retries are performed.
type Policy struct {
	// MaxAttempts is the total number of calls (first attempt + retries).
	// 1 = no retry, 3 = one retry after first failure. Default: 3.
	MaxAttempts int
	// Delay is the base delay before the second attempt. Default: 100ms.
	Delay time.Duration
	// MaxDelay caps the computed backoff. Default: 10s.
	MaxDelay time.Duration
	// Multiplier is the backoff multiplier per attempt. Default: 2.0.
	// Set to 1.0 for constant delay.
	Multiplier float64
	// Jitter adds ±25% random jitter to each delay to prevent thundering herds.
	// Default: true.
	Jitter bool
	// Retryable decides whether an error should trigger a retry.
	// If nil, DefaultRetryable is used (retry on non-4xx errors).
	Retryable func(error) bool
}

// DefaultPolicy is sensible defaults for idempotent RPC calls.
var DefaultPolicy = Policy{
	MaxAttempts: 3,
	Delay:       100 * time.Millisecond,
	MaxDelay:    10 * time.Second,
	Multiplier:  2.0,
	Jitter:      true,
}

func (p *Policy) withDefaults() Policy {
	out := *p
	if out.MaxAttempts <= 0 {
		out.MaxAttempts = DefaultPolicy.MaxAttempts
	}
	if out.Delay <= 0 {
		out.Delay = DefaultPolicy.Delay
	}
	if out.MaxDelay <= 0 {
		out.MaxDelay = DefaultPolicy.MaxDelay
	}
	if out.Multiplier <= 1 {
		out.Multiplier = DefaultPolicy.Multiplier
	}
	if out.Retryable == nil {
		out.Retryable = DefaultRetryable
	}
	return out
}

// ─── Retryable predicate ──────────────────────────────────────────────────────

// HTTPError is satisfied by errors that expose an HTTP status code.
// It is intentionally unexported to avoid coupling to a specific HTTP framework.
type httpCoder interface {
	HTTPCode() int
}

// DefaultRetryable returns true for any error that is not an HTTP 4xx response.
// 4xx errors represent client mistakes (bad request, auth failure, etc.) and
// retrying them will not help.
func DefaultRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Check for context cancellation / deadline — never retry these.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Check for an HTTP status code embedded in the error.
	var coder httpCoder
	if errors.As(err, &coder) {
		code := coder.HTTPCode()
		return code < 400 || code >= 500 // retry only on 5xx, not 4xx
	}
	// Unknown error type — retry by default (network errors, etc.).
	return true
}

// HTTPStatusRetryable builds a Retryable function that retries only on the
// given HTTP status codes (e.g. 429, 503).
func HTTPStatusRetryable(codes ...int) func(error) bool {
	set := make(map[int]struct{}, len(codes))
	for _, c := range codes {
		set[c] = struct{}{}
	}
	return func(err error) bool {
		if err == nil {
			return false
		}
		var coder httpCoder
		if errors.As(err, &coder) {
			_, ok := set[coder.HTTPCode()]
			return ok
		}
		return true
	}
}

// StatusError is a simple error that carries an HTTP status code.
// Useful in tests or when wrapping raw http.Response errors.
type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Code, e.Message) }
func (e *StatusError) HTTPCode() int { return e.Code }

// NewStatusError creates a StatusError from an http.Response.
// It returns nil when resp.StatusCode < 400.
func NewStatusError(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	return &StatusError{Code: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
}

// ─── Do ───────────────────────────────────────────────────────────────────────

// Do executes fn with retry logic defined by policy.
//
// It returns nil on the first successful call, or an error wrapping the last
// failure once MaxAttempts is exhausted or the context is cancelled.
func Do(ctx context.Context, policy Policy, fn func(ctx context.Context) error) error {
	p := policy.withDefaults()

	var lastErr error
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		// Check context before each attempt.
		if ctx.Err() != nil {
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		// Check retryability.
		if !p.Retryable(lastErr) {
			return lastErr
		}

		// Don't sleep after the final attempt.
		if attempt == p.MaxAttempts {
			break
		}

		d := nextDelay(p, attempt)
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(d):
		}
	}

	return fmt.Errorf("retry: all %d attempts failed: %w", p.MaxAttempts, lastErr)
}

// nextDelay computes the sleep duration for attempt n (1-based).
// Uses exponential backoff: delay * multiplier^(n-1), capped at MaxDelay.
// Optionally applies ±25% jitter to prevent thundering herds.
func nextDelay(p Policy, attempt int) time.Duration {
	d := float64(p.Delay) * math.Pow(p.Multiplier, float64(attempt-1))
	if d > float64(p.MaxDelay) {
		d = float64(p.MaxDelay)
	}
	if p.Jitter {
		// Add jitter in [-25%, +25%] of the computed delay.
		jitter := d * 0.25 * (2*rand.Float64() - 1)
		d += jitter
		if d < 0 {
			d = 0
		}
	}
	return time.Duration(d)
}
