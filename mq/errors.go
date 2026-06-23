// Package mq - semantic error handling for message consumption.
//
// Handlers can return *MQError to control retry behavior:
//   - ErrPermanent: permanent failure, send to DLQ, no retry
//   - ErrRetry:    retryable failure, retry after specified delay
//   - ErrSkip:     skip this message, no retry, no DLQ
//   - ErrPanic:    panic occurred, framework sends to DLQ
//
// Default behavior (returning a plain error): treated as ErrRetry
// with exponential backoff.
package mq

import (
	"errors"
	"fmt"
	"time"
)

// ErrorKind indicates the semantic meaning of a consumption error.
type ErrorKind int

const (
	// ErrPermanent means the error is permanent; the message should
	// be sent to the dead-letter queue (DLQ) and not retried.
	ErrPermanent ErrorKind = iota

	// ErrRetry means the error is retryable; the message should be
	// re-delivered after the specified RetryAfter duration.
	ErrRetry

	// ErrSkip means the message should be skipped (acked without
	// processing, not sent to DLQ).
	ErrSkip

	// ErrPanic means a panic occurred during processing; the framework
	// sends the message to DLQ.
	ErrPanic
)

// MQError is a semantic error returned by Handlers to control
// retry/dead-letter behavior.
type MQError struct {
	Kind       ErrorKind
	Cause      error
	RetryAfter time.Duration // only meaningful when Kind == ErrRetry
}

func (e *MQError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	switch e.Kind {
	case ErrPermanent:
		return "permanent failure"
	case ErrRetry:
		return fmt.Sprintf("retry after %v", e.RetryAfter)
	case ErrSkip:
		return "skip message"
	case ErrPanic:
		return "panic during processing"
	default:
		return "unknown MQError"
	}
}

func (e *MQError) Unwrap() error {
	return e.Cause
}

// ── Convenience constructors ──────────────────────────────────────────────

// Permanent wraps err as a permanent failure (no retry, send to DLQ).
func Permanent(err error) error {
	return &MQError{Kind: ErrPermanent, Cause: err}
}

// Retry wraps err as a retryable failure with the specified delay.
func Retry(err error, after time.Duration) error {
	return &MQError{Kind: ErrRetry, Cause: err, RetryAfter: after}
}

// Skip wraps err as a skip (no retry, no DLQ).
func Skip(err error) error {
	return &MQError{Kind: ErrSkip, Cause: err}
}

// ── Kind predicates ────────────────────────────────────────────────────────

// IsPermanent returns true if err is or wraps an ErrPermanent MQError.
func IsPermanent(err error) bool {
	var mqErr *MQError
	if errors.As(err, &mqErr) {
		return mqErr.Kind == ErrPermanent
	}
	return false
}

// IsRetry returns true if err is or wraps an ErrRetry MQError.
func IsRetry(err error) bool {
	var mqErr *MQError
	if errors.As(err, &mqErr) {
		return mqErr.Kind == ErrRetry
	}
	return false
}

// IsSkip returns true if err is or wraps an ErrSkip MQError.
func IsSkip(err error) bool {
	var mqErr *MQError
	if errors.As(err, &mqErr) {
		return mqErr.Kind == ErrSkip
	}
	return false
}

// ── RetryPolicy ───────────────────────────────────────────────────────────
//
// RetryPolicy defines how retries are performed.
// If Levels is non-empty, it is used for step-wise retry delays.
// Otherwise, exponential backoff (Base * 2^retryCount) is used.

// RetryPolicy defines the retry strategy for a message or consumer.
type RetryPolicy struct {
	// MaxRetries is the maximum number of retries.
	// If zero, the default (3) is used.
	MaxRetries int

	// Levels defines step-wise retry delays.
	// If non-empty, Levels[retryCount] is used for the delay.
	// If retryCount >= len(Levels), the message is sent to DLQ.
	Levels []time.Duration

	// Base is the base duration for exponential backoff.
	// Used when Levels is empty.
	// If zero, the default (1s) is used.
	Base time.Duration
}

// DefaultRetryPolicy returns the default RetryPolicy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 3,
		Base:       1 * time.Second,
	}
}

// NextDelay returns the delay for the next retry, and whether to continue
// retrying. If the retry count exceeds MaxRetries, (0, false) is returned
// (meaning: send to DLQ).
func (p *RetryPolicy) NextDelay(retryCount int) (time.Duration, bool) {
	if p == nil {
		dp := DefaultRetryPolicy()
		return dp.NextDelay(retryCount)
	}
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount >= p.MaxRetries {
		return 0, false
	}
	if len(p.Levels) > 0 && retryCount < len(p.Levels) {
		return p.Levels[retryCount], true
	}
	// Exponential backoff: Base * 2^retryCount
	base := p.Base
	if base == 0 {
		base = 1 * time.Second
	}
	return base * (1 << uint(retryCount)), true
}
