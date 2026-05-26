package taskqueue

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TaskOption configures a Task before it is enqueued.
type TaskOption func(*Task)

// NewTask creates a new Task with the given type name and payload, applying
// all options. The task ID is auto-generated as a UUID v4.
//
// Defaults:
//   - Queue:      "default"
//   - MaxRetries: 3
//   - Timeout:    30 minutes
//   - ProcessAt:  time.Now() (immediate)
func NewTask(taskType string, payload []byte, opts ...TaskOption) *Task {
	now := time.Now()
	t := &Task{
		ID:         uuid.New().String(),
		Type:       taskType,
		Payload:    payload,
		Queue:      DefaultQueue,
		State:      StatePending,
		MaxRetries: DefaultMaxRetries,
		Timeout:    DefaultTimeout,
		ProcessAt:  now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// WithQueue sets the destination queue for the task.
// Tasks in different queues can be given different priorities in the Server.
func WithQueue(name string) TaskOption {
	return func(t *Task) { t.Queue = name }
}

// WithMaxRetries sets the maximum number of retry attempts.
// When Retried reaches MaxRetries the task is moved to the dead queue.
func WithMaxRetries(n int) TaskOption {
	return func(t *Task) { t.MaxRetries = n }
}

// WithTimeout sets the per-execution time limit for this task.
// The worker cancels the handler context when the timeout elapses.
// If the handler does not honour context cancellation the worker
// will still Nack after the timeout.
func WithTimeout(d time.Duration) TaskOption {
	return func(t *Task) { t.Timeout = d }
}

// WithProcessIn schedules the task for execution after the given delay.
// Mutually exclusive with WithProcessAt — last applied wins.
func WithProcessIn(d time.Duration) TaskOption {
	return func(t *Task) { t.ProcessAt = time.Now().Add(d) }
}

// WithProcessAt schedules the task for execution at the given wall-clock time.
// Mutually exclusive with WithProcessIn — last applied wins.
func WithProcessAt(tm time.Time) TaskOption {
	return func(t *Task) { t.ProcessAt = tm }
}

// WithUnique sets a deduplication key and window for the task.
//
// If a task with the same unique key already exists in the broker within the
// given window, Enqueue returns ErrDuplicateTask without storing a duplicate.
//
// When key is empty, a SHA-256 hash of the task type and payload is used as
// the dedup key, making the window content-addressed.
//
// Typical usage:
//
//	// Content-addressed dedup: same payload won't run twice in 10 min
//	taskqueue.WithUnique("", 10*time.Minute)
//
//	// Explicit key: useful when payload varies but logical identity doesn't
//	taskqueue.WithUnique(fmt.Sprintf("invoice:%d", invoiceID), 1*time.Hour)
func WithUnique(key string, window time.Duration) TaskOption {
	return func(t *Task) {
		if key == "" {
			h := sha256.Sum256(append([]byte(t.Type+":"), t.Payload...))
			key = fmt.Sprintf("%x", h[:12])
		}
		t.UniqueKey = fmt.Sprintf("%s:%s:%s", t.Queue, t.Type, key)
		t.UniqueFor = window
	}
}
