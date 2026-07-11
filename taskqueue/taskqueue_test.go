package taskqueue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/astra-go/astra/mq"
)

func TestRetryPolicy_RetryDelay(t *testing.T) {
	tests := []struct {
		name    string
		policy  RetryPolicy
		attempt int
		wantMin int // minimum seconds
		wantMax int // maximum seconds (with jitter)
	}{
		{
			name: "attempt 1, default policy",
			policy: RetryPolicy{
				MaxRetries:        3,
				BackoffBase:       5,
				BackoffMultiplier: 3,
				BackoffMax:        300,
				BackoffJitter:     false,
			},
			attempt: 1,
			wantMin: 5,
			wantMax: 5,
		},
		{
			name: "attempt 2, default policy",
			policy: RetryPolicy{
				MaxRetries:        3,
				BackoffBase:       5,
				BackoffMultiplier: 3,
				BackoffMax:        300,
				BackoffJitter:     false,
			},
			attempt: 2,
			wantMin: 15,
			wantMax: 15,
		},
		{
			name: "attempt 3, default policy",
			policy: RetryPolicy{
				MaxRetries:        3,
				BackoffBase:       5,
				BackoffMultiplier: 3,
				BackoffMax:        300,
				BackoffJitter:     false,
			},
			attempt: 3,
			wantMin: 45,
			wantMax: 45,
		},
		{
			name: "attempt 4, max exceeded",
			policy: RetryPolicy{
				MaxRetries:        3,
				BackoffBase:       5,
				BackoffMultiplier: 3,
				BackoffMax:        300,
				BackoffJitter:     false,
			},
			attempt: 4,
			wantMin: 0,
			wantMax: 0,
		},
		{
			name: "backoff capped at max",
			policy: RetryPolicy{
				MaxRetries:        10,
				BackoffBase:       60,
				BackoffMultiplier: 10,
				BackoffMax:        300,
				BackoffJitter:     false,
			},
			attempt: 4, // 60 * 10^3 = 600000s, capped at 300
			wantMin: 300,
			wantMax: 300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.policy.RetryDelay(tt.attempt)
			gotSec := int(got.Seconds())
			if gotSec < tt.wantMin || gotSec > tt.wantMax {
				t.Errorf("RetryDelay(%d) = %v, want between %ds and %ds", tt.attempt, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	p := DefaultRetryPolicy
	for i := 1; i <= 3; i++ {
		if !p.ShouldRetry(i) {
			t.Errorf("ShouldRetry(%d) = false, want true", i)
		}
	}
	if p.ShouldRetry(4) {
		t.Errorf("ShouldRetry(4) = true, want false (MaxRetries=3)")
	}
}

func TestRetryPolicy_MaxDeliver(t *testing.T) {
	if DefaultRetryPolicy.MaxDeliver() != 4 {
		t.Errorf("MaxDeliver() = %d, want 4 (MaxRetries+1)", DefaultRetryPolicy.MaxDeliver())
	}

	noRetry := RetryPolicy{MaxRetries: 0}
	if noRetry.MaxDeliver() != 1 {
		t.Errorf("MaxDeliver() = %d, want 1 (no retry)", noRetry.MaxDeliver())
	}
}

func TestRouter_Dispatch(t *testing.T) {
	r := NewRouter(nil)
	var called bool

	r.Register("send_email", func(ctx context.Context, data json.RawMessage) error {
		called = true
		return nil
	})

	msg := &mq.Message{
		Payload: json.RawMessage(`{"task_type":"send_email","data":{"to":"a@b.com"}}`),
		Meta:    map[string]any{"task_type": "send_email"},
	}

	if err := r.Dispatch(context.Background(), msg); err != nil {
		t.Fatalf("Dispatch() = %v, want nil", err)
	}

	if !called {
		t.Error("handler was not called")
	}
}

func TestRouter_Dispatch_MissingHandler(t *testing.T) {
	r := NewRouter(nil)
	r.Register("send_email", func(ctx context.Context, data json.RawMessage) error { return nil })

	msg := &mq.Message{
		Payload: json.RawMessage(`{"task_type":"unknown_type"}`),
	}

	err := r.Dispatch(context.Background(), msg)
	if err == nil {
		t.Fatal("Dispatch() = nil, want error for unknown task type")
	}
}

func TestHandlerSkip(t *testing.T) {
	err := HandlerSkip("intentionally skipped")
	if !IsSkip(err) {
		t.Error("IsSkip(HandlerSkip(...)) = false, want true")
	}
	if !errors.Is(err, ErrSkip) {
		t.Error("errors.Is(err, ErrSkip) = false")
	}
}

func TestMessage(t *testing.T) {
	msg, err := Message("send_email", map[string]string{"to": "a@b.com"})
	if err != nil {
		t.Fatalf("Message() = %v", err)
	}

	var p TaskPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if p.TaskType != "send_email" {
		t.Errorf("TaskType = %q, want %q", p.TaskType, "send_email")
	}

	var data map[string]string
	if err := json.Unmarshal(p.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data["to"] != "a@b.com" {
		t.Errorf("data[to] = %q, want %q", data["to"], "a@b.com")
	}
}
