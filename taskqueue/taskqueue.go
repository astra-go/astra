// Package taskqueue provides a distributed task queue for Astra applications.
//
// Five persistent backends are supported — choose based on your infrastructure:
//
//	taskqueue/redis    — Redis-backed (recommended; low-latency, Lua-atomic ops)
//	taskqueue/mongo    — MongoDB-backed (for teams already operating MongoDB)
//	taskqueue/rabbitmq — RabbitMQ-backed (AMQP; requires x-delayed-message plugin
//	                     for accurate delayed/retry delivery)
//	taskqueue/kafka    — Kafka-backed (franz-go; retry via separate topic,
//	                     promoted by the periodic Schedule goroutine)
//	taskqueue/rocketmq — RocketMQ 5.x-backed (gRPC; native delay + invisible
//	                     duration for retry, no extra infrastructure needed)
//
// # Quick start (Redis)
//
//	import (
//	    "github.com/astra-go/astra/taskqueue"
//	    tqredis "github.com/astra-go/astra/taskqueue/redis"
//	)
//
//	broker, _ := tqredis.New(tqredis.Config{Addr: "localhost:6379"})
//	client  := taskqueue.NewClient(broker)
//	defer client.Close()
//
//	// Enqueue a task immediately
//	client.EnqueueTask(ctx, "email:welcome", payload,
//	    taskqueue.WithQueue("critical"),
//	    taskqueue.WithMaxRetries(5),
//	)
//
//	// Enqueue a delayed task
//	client.EnqueueTask(ctx, "report:generate", payload,
//	    taskqueue.WithProcessIn(10 * time.Minute),
//	)
//
//	// Worker server
//	mux := taskqueue.NewServeMux()
//	mux.HandleFunc("email:welcome", func(ctx context.Context, t *taskqueue.Task) error {
//	    return sendWelcomeEmail(t.Payload)
//	})
//
//	srv := taskqueue.NewServer(taskqueue.ServerConfig{
//	    Broker:      broker,
//	    Queues:      map[string]int{"critical": 6, "default": 3, "low": 1},
//	    Concurrency: 20,
//	})
//	srv.Run(ctx, mux)
//
// # Task lifecycle
//
//	pending ──────────────────────────────► active ──► done
//	    ▲   (process_at reached)             │
//	    │                                    │ error, retried < max
//	    │                                    ▼
//	scheduled ◄── (ProcessIn/ProcessAt)   retry ──► pending (after backoff)
//	                                         │
//	                                         │ error, retried >= max
//	                                         ▼
//	                                        dead
//
// # Priority queues
//
// Workers poll queues in weighted priority order. Configure weights in ServerConfig.Queues:
//
//	Queues: map[string]int{"critical": 6, "default": 3, "low": 1}
//	// Workers will poll "critical" 6 times more often than "low".
//
// # Task deduplication
//
//	client.EnqueueTask(ctx, "invoice:send", payload,
//	    taskqueue.WithUnique("invoice-42", 10*time.Minute),
//	)
//	// A second Enqueue with the same unique key within 10 minutes
//	// returns ErrDuplicateTask without storing a duplicate task.
//
// # Cron tasks
//
//	srv.RegisterCron("0 9 * * *", "report:daily", nil,
//	    taskqueue.WithUnique("report:daily", 23*time.Hour),
//	)
//	// Registers a task that fires every day at 09:00.
//	// WithUnique prevents duplicates when multiple server instances run.
package taskqueue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

// ErrDuplicateTask is returned by Broker.Enqueue when a task with the same
// unique key already exists within its deduplication window.
var ErrDuplicateTask = errors.New("taskqueue: duplicate task in uniqueness window")

// ErrNoTask is returned by Broker.Dequeue when no pending task is available
// in any of the requested queues.
var ErrNoTask = errors.New("taskqueue: no task available")

// ErrTaskNotFound is returned when a task ID cannot be found in the broker.
var ErrTaskNotFound = errors.New("taskqueue: task not found")

// ─── Task state ───────────────────────────────────────────────────────────────

// State represents the lifecycle stage of a Task.
type State string

const (
	// StatePending means the task is ready for immediate processing.
	StatePending State = "pending"
	// StateActive means the task has been dequeued and a worker is running it.
	StateActive State = "active"
	// StateScheduled means the task is waiting for its ProcessAt time.
	StateScheduled State = "scheduled"
	// StateRetry means the task failed and is waiting for its next retry window.
	StateRetry State = "retry"
	// StateDead means the task exhausted all retries and will not be re-run.
	StateDead State = "dead"
	// StateDone means the task was processed successfully.
	StateDone State = "done"
)

// ─── Task ─────────────────────────────────────────────────────────────────────

// DefaultQueue is the queue name used when WithQueue is not specified.
const DefaultQueue = "default"

// DefaultMaxRetries is the retry limit used when WithMaxRetries is not specified.
const DefaultMaxRetries = 3

// DefaultTimeout is the per-task execution timeout when WithTimeout is not set.
const DefaultTimeout = 30 * time.Minute

// Task is the unit of work. Producers create Tasks; workers receive Tasks.
type Task struct {
	// ID is a UUID v4 assigned automatically by NewTask.
	ID string

	// Type is the name of the handler to route this task to,
	// e.g. "email:welcome", "invoice:generate".
	Type string

	// Payload is the task body — typically JSON-encoded domain data.
	Payload []byte

	// Queue is the destination queue name. Default: "default".
	Queue string

	// State is the current lifecycle stage. Set by the broker.
	State State

	// MaxRetries is the maximum number of retry attempts. Default: 3.
	MaxRetries int

	// Retried is the number of times the task has been retried so far.
	Retried int

	// Timeout is the per-execution time limit. Default: 30 minutes.
	// The worker cancels the handler's context when Timeout elapses.
	Timeout time.Duration

	// ProcessAt is the earliest time at which the task should be processed.
	// Zero value means immediately.
	ProcessAt time.Time

	// UniqueKey is the deduplication key. Empty means no deduplication.
	UniqueKey string

	// UniqueFor is the duration of the deduplication window.
	UniqueFor time.Duration

	// LastError stores the most recent handler error message.
	LastError string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate returns an error if the Task is missing required fields set during
// enqueue. Brokers call this after JSON deserialization to reject malformed or
// tampered messages before they reach application handlers.
func (t *Task) Validate() error {
	if t.ID == "" {
		return errors.New("taskqueue: task missing ID")
	}
	if t.Type == "" {
		return errors.New("taskqueue: task missing Type")
	}
	if t.Queue == "" {
		return errors.New("taskqueue: task missing Queue")
	}
	return nil
}

// ─── Broker interface ─────────────────────────────────────────────────────────

// Broker is the storage abstraction. Implement this interface to add new backends.
// All methods must be safe for concurrent use.
type Broker interface {
	// Enqueue atomically stores the task and places it in pending or scheduled
	// state. Returns ErrDuplicateTask if a unique key collision is detected.
	Enqueue(ctx context.Context, task *Task) error

	// Dequeue atomically moves the highest-priority pending task from one of the
	// given queues to active state and returns it.
	// deadline is the wall-clock time by which the worker must Ack or Nack;
	// it is used by ReapStale to recover crashed workers.
	// queues is ordered by priority (first = highest).
	// Returns ErrNoTask when no pending task is available.
	Dequeue(ctx context.Context, queues []string, deadline time.Time) (*Task, error)

	// Ack marks the task as successfully done and removes it from the active set.
	Ack(ctx context.Context, task *Task) error

	// Nack records a task failure.
	// If retryAt.IsZero(), the task is moved to the dead queue.
	// Otherwise, the task is moved to the retry set for redelivery at retryAt.
	// lastErr is stored in Task.LastError for observability.
	Nack(ctx context.Context, task *Task, lastErr string, retryAt time.Time) error

	// Schedule promotes tasks in the scheduled and retry states whose ProcessAt
	// time has elapsed back to pending. Called periodically by the Server.
	Schedule(ctx context.Context) error

	// ReapStale recovers tasks stuck in the active state past their deadline
	// (caused by worker crashes). Moves them back to pending. Called periodically.
	ReapStale(ctx context.Context) error

	// Close releases all resources held by the broker.
	Close() error
}

// ─── Handler and ServeMux ─────────────────────────────────────────────────────

// Handler processes a task. Return nil to acknowledge (success), return an error
// to nack (failure; the Server will retry or dead-letter based on MaxRetries).
type Handler func(ctx context.Context, task *Task) error

// ServeMux routes incoming tasks to their registered handlers by task type.
// It is analogous to http.ServeMux.
type ServeMux struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewServeMux creates an empty ServeMux.
func NewServeMux() *ServeMux {
	return &ServeMux{handlers: make(map[string]Handler)}
}

// Handle registers handler h for the given task type.
// Panics if h is nil or taskType is empty.
func (m *ServeMux) Handle(taskType string, h Handler) {
	if taskType == "" {
		panic("taskqueue: empty task type")
	}
	if h == nil {
		panic("taskqueue: nil handler")
	}
	m.mu.Lock()
	m.handlers[taskType] = h
	m.mu.Unlock()
}

// HandleFunc registers fn as the handler for the given task type.
func (m *ServeMux) HandleFunc(taskType string, fn func(ctx context.Context, task *Task) error) {
	m.Handle(taskType, Handler(fn))
}

// ProcessTask dispatches the task to its registered handler.
// Returns an error if no handler is registered for task.Type.
func (m *ServeMux) ProcessTask(ctx context.Context, task *Task) error {
	m.mu.RLock()
	h, ok := m.handlers[task.Type]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("taskqueue: no handler registered for type %q", task.Type)
	}
	return h(ctx, task)
}
