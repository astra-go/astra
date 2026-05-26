package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/astra-go/astra/retry"
	"github.com/astra-go/astra/testutil"
)

// ─── Do ───────────────────────────────────────────────────────────────────────

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 3}, func(_ context.Context) error {
		calls++
		return nil
	})
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, 1, calls)
}

func TestDo_RetriesOnTransientError(t *testing.T) {
	calls := 0
	target := 3
	err := retry.Do(context.Background(), retry.Policy{
		MaxAttempts: 5,
		Delay:       time.Millisecond,
		Multiplier:  1,
	}, func(_ context.Context) error {
		calls++
		if calls < target {
			return errors.New("transient")
		}
		return nil
	})
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, target, calls)
}

func TestDo_ExhaustsMaxAttempts(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), retry.Policy{
		MaxAttempts: 3,
		Delay:       time.Millisecond,
		Multiplier:  1,
	}, func(_ context.Context) error {
		calls++
		return errors.New("always fails")
	})
	testutil.AssertError(t, err)
	testutil.AssertEqual(t, 3, calls)
}

func TestDo_DoesNotRetry4xx(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), retry.Policy{
		MaxAttempts: 5,
		Delay:       time.Millisecond,
	}, func(_ context.Context) error {
		calls++
		return &retry.StatusError{Code: 400, Message: "bad request"}
	})
	testutil.AssertError(t, err)
	// Should not retry 4xx — only 1 call.
	testutil.AssertEqual(t, 1, calls)
}

func TestDo_Retries5xx(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), retry.Policy{
		MaxAttempts: 3,
		Delay:       time.Millisecond,
		Multiplier:  1,
	}, func(_ context.Context) error {
		calls++
		return &retry.StatusError{Code: 503, Message: "service unavailable"}
	})
	testutil.AssertError(t, err)
	testutil.AssertEqual(t, 3, calls) // should have retried all 3 times
}

func TestDo_StopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := retry.Do(ctx, retry.Policy{
		MaxAttempts: 10,
		Delay:       50 * time.Millisecond,
	}, func(_ context.Context) error {
		calls++
		if calls == 2 {
			cancel() // cancel after second call
		}
		return errors.New("retry")
	})

	testutil.AssertError(t, err)
	if calls > 3 {
		t.Errorf("expected retry to stop after context cancel, got %d calls", calls)
	}
}

func TestDo_ContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	calls := 0
	err := retry.Do(ctx, retry.Policy{MaxAttempts: 5}, func(_ context.Context) error {
		calls++
		return errors.New("error")
	})
	testutil.AssertError(t, err)
	// context cancelled before loop — 0 or 1 calls depending on timing
	if calls > 2 {
		t.Errorf("expected at most 1 call, got %d", calls)
	}
}

func TestDo_CustomRetryable_AllowsOnlySpecificCodes(t *testing.T) {
	calls := 0
	retryable429 := retry.HTTPStatusRetryable(429)

	// 429 → should retry
	err := retry.Do(context.Background(), retry.Policy{
		MaxAttempts: 3,
		Delay:       time.Millisecond,
		Multiplier:  1,
		Retryable:   retryable429,
	}, func(_ context.Context) error {
		calls++
		return &retry.StatusError{Code: 429, Message: "too many requests"}
	})
	testutil.AssertError(t, err)
	testutil.AssertEqual(t, 3, calls)

	// 500 → should NOT retry with this custom policy
	calls = 0
	err = retry.Do(context.Background(), retry.Policy{
		MaxAttempts: 3,
		Delay:       time.Millisecond,
		Retryable:   retryable429,
	}, func(_ context.Context) error {
		calls++
		return &retry.StatusError{Code: 500, Message: "internal error"}
	})
	testutil.AssertError(t, err)
	testutil.AssertEqual(t, 1, calls)
}

// ─── DefaultRetryable ─────────────────────────────────────────────────────────

func TestDefaultRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic error", errors.New("network timeout"), true},
		{"4xx status", &retry.StatusError{Code: 400}, false},
		{"5xx status", &retry.StatusError{Code: 503}, true},
		{"context cancelled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := retry.DefaultRetryable(tc.err)
			testutil.AssertEqual(t, tc.want, got)
		})
	}
}

// ─── StatusError ─────────────────────────────────────────────────────────────

func TestStatusError_Error(t *testing.T) {
	e := &retry.StatusError{Code: 404, Message: "Not Found"}
	if e.Error() == "" {
		t.Error("StatusError.Error() should not be empty")
	}
	testutil.AssertEqual(t, 404, e.HTTPCode())
}
