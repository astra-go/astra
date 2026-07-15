// Package taskqueue provides a typed task dispatch layer on top of mq.
//
// It adds:
//   - Type-based routing: dispatch messages to typed handlers (TaskType → Handler).
//   - Exponential backoff: configurable backoff between retry attempts.
//   - DLQ envelope: standardized dead-letter payload with full context.
//   - Task store: optional persistence (Postgres, Redis) for observability.
//
// Example (NATS JetStream):
//
//	r := taskqueue.NewRouter()
//
//	r.Register("send_email", func(ctx context.Context, data json.RawMessage) error {
//	    var email EmailPayload
//	    if err := json.Unmarshal(data, &email); err != nil {
//	        return err
//	    }
//	    return sendEmail(ctx, email)
//	})
//
//	// Dispatcher wraps the NATS consumer
//	d := taskqueue.NewDispatcher(mqConsumer, r)
//
//	// d.Start(ctx) blocks; messages are dispatched to handlers by TaskType
package taskqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/astra-go/astra/mq"
)

// TaskType identifies the kind of task being dispatched.
// Convention: lowercase_with_underscores (e.g. "send_email", "payment_callback").
type TaskType string

// TaskPayload is the standard envelope for task messages.
// The Router reads TaskType and Data from this struct by default.
type TaskPayload struct {
	TaskType TaskType        `json:"task_type"`
	Data     json.RawMessage `json:"data"`
	UIN      string          `json:"uin,omitempty"`
}

// TaskHandler is the signature for a typed task handler.
//
// Return nil to ACK the message (task completed successfully).
// Return [ErrSkip] to ACK without retry (task is intentionally discarded).
// Return any other error to trigger retry (up to RetryPolicy limits).
type TaskHandler func(ctx context.Context, data json.RawMessage) error

// ErrSkip is a sentinel error returned by a TaskHandler to signal the message
// should be acknowledged without retry. Use [HandlerSkip] to create this error.
var ErrSkip = skipError{msg: "task: skipped"}

type skipError struct{ msg string }

func (e skipError) Error() string { return e.msg }

// Is implements error wrapping so errors.Is(err, taskqueue.ErrSkip) returns true.
func (e skipError) Is(target error) bool { return target == ErrSkip }

// HandlerSkip creates an error that causes the dispatcher to ACK the message
// without retrying. This is useful for tasks that are intentionally
// idempotent or should be silently dropped.
func HandlerSkip(msg string) error {
	return &skipError{msg: msg}
}

// IsSkip reports whether err is a skip signal.
func IsSkip(err error) bool {
	_, ok := err.(*skipError)
	return ok
}

// Router dispatches incoming messages to typed handlers.
type Router struct {
	handlers map[TaskType]TaskHandler
	mu       sync.RWMutex
	log      *slog.Logger
}

// NewRouter creates a new Router. An optional *slog.Logger can be provided
// for structured logging of dispatch events.
func NewRouter(log *slog.Logger) *Router {
	if log == nil {
		log = slog.Default()
	}
	return &Router{
		handlers: make(map[TaskType]TaskHandler),
		log:      log,
	}
}

// Register associates a TaskHandler with a TaskType.
// Handlers are called in order of TaskType matching.
func (r *Router) Register(tt TaskType, h TaskHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[tt] = h
}

// Handler returns the handler registered for tt, if any.
func (r *Router) Handler(tt TaskType) (TaskHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[tt]
	return h, ok
}

// Dispatch delivers msg to the appropriate TaskHandler.
// It reads TaskType from msg.Meta["task_type"] or from TaskPayload in msg.Payload.
func (r *Router) Dispatch(ctx context.Context, msg *mq.Message) error {
	tt := r.resolveTaskType(msg)
	if tt == "" {
		return fmt.Errorf("taskqueue: cannot determine task type from message")
	}

	h, ok := r.Handler(tt)
	if !ok {
		r.log.Warn("taskqueue: no handler registered",
			"task_type", tt,
			"topic", msg.Topic,
			"retry_count", msg.RetryCount)
		return fmt.Errorf("taskqueue: no handler for task type: %s", tt)
	}

	// Extract task data: prefer explicit Data field, fall back to raw payload.
	data := r.extractData(msg)

	r.log.Debug("taskqueue: dispatching",
		"task_type", tt,
		"topic", msg.Topic,
		"retry_count", msg.RetryCount)

	return h(ctx, data)
}

// resolveTaskType reads the task type from msg.Meta or the JSON payload.
func (r *Router) resolveTaskType(msg *mq.Message) TaskType {
	// 1. Try msg.Meta["task_type"]
	if msg.Meta != nil {
		if t, ok := msg.Meta["task_type"].(string); ok && t != "" {
			return TaskType(t)
		}
	}

	// 2. Try parsing TaskPayload
	var p TaskPayload
	if err := json.Unmarshal(msg.Payload, &p); err == nil && p.TaskType != "" {
		return p.TaskType
	}

	return ""
}

// extractData returns the task data payload, preferring the Data field
// over the raw payload.
func (r *Router) extractData(msg *mq.Message) json.RawMessage {
	var p TaskPayload
	if err := json.Unmarshal(msg.Payload, &p); err == nil && p.Data != nil {
		return p.Data
	}
	return msg.Payload
}

// Message builds a mq.Message from a TaskPayload.
func Message(tt TaskType, data any, opts ...MessageOption) (*mq.Message, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("taskqueue: marshal data: %w", err)
	}

	payload, err := json.Marshal(TaskPayload{
		TaskType: tt,
		Data:     dataBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("taskqueue: marshal payload: %w", err)
	}

	msg := &mq.Message{Payload: payload}
	for _, opt := range opts {
		opt(msg)
	}
	return msg, nil
}

// MessageOption configures a task message before publishing.
type MessageOption func(*mq.Message)

// WithTaskTypeMeta sets the TaskType in msg.Meta so Dispatch can read it
// without unmarshaling the payload.
func WithTaskTypeMeta(tt TaskType) MessageOption {
	return func(msg *mq.Message) {
		if msg.Meta == nil {
			msg.Meta = make(map[string]any)
		}
		msg.Meta["task_type"] = string(tt)
	}
}

// WithTaskUIN embeds the UIN in the payload.
func WithTaskUIN(uin string) MessageOption {
	return func(msg *mq.Message) {
		var p TaskPayload
		if err := json.Unmarshal(msg.Payload, &p); err == nil {
			p.UIN = uin
			if b, err2 := json.Marshal(p); err2 == nil {
				msg.Payload = b
			}
		}
	}
}

// WithTaskDelay sets a delivery delay.
func WithTaskDelay(d time.Duration) MessageOption {
	return func(msg *mq.Message) { msg.Delay = d }
}

// WithTaskIdempKey sets the idempotency key.
func WithTaskIdempKey(k string) MessageOption {
	return func(msg *mq.Message) { msg.IdempKey = k }
}

// WithTaskRetryMax overrides the default retry limit.
func WithTaskRetryMax(n int) MessageOption {
	return func(msg *mq.Message) { msg.RetryMax = n }
}

