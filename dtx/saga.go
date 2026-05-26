// Package dtx implements the Saga orchestration pattern for distributed transactions.
//
// A Saga is a sequence of local transactions linked by compensating transactions.
// If any step fails, all previously completed steps are compensated (undone) in
// reverse order — giving the system a consistent "all or nothing" outcome without
// requiring a 2PC coordinator.
//
// # Usage
//
//	saga := dtx.New(
//	    dtx.Step{
//	        Name:       "debit-account",
//	        Forward:    func(ctx context.Context) error { return debitAccount(ctx, amount) },
//	        Compensate: func(ctx context.Context) error { return creditAccount(ctx, amount) },
//	    },
//	    dtx.Step{
//	        Name:       "reserve-inventory",
//	        Forward:    func(ctx context.Context) error { return reserveItem(ctx, itemID) },
//	        Compensate: func(ctx context.Context) error { return releaseItem(ctx, itemID) },
//	    },
//	    dtx.Step{
//	        Name:    "send-confirmation",
//	        Forward: func(ctx context.Context) error { return sendEmail(ctx, userEmail) },
//	        // No Compensate — email already sent, cannot unsend.
//	    },
//	)
//
//	result := saga.Execute(ctx)
//	if result.Err != nil {
//	    log.Printf("saga failed at %q: %v", result.FailedStep, result.Err)
//	    // result.CompensationErrors holds any errors from the rollback
//	}
//
// # Compensation strategy
//
// Compensations run in reverse order of the completed forwards. If a step has
// no Compensate function (e.g. an irreversible side-effect), it is silently
// skipped during rollback. Compensation errors are collected in
// SagaResult.CompensationErrors but do not prevent other compensations from
// running.
//
// # Persistence and crash recovery
//
// By default, saga execution state is held entirely in memory. If the process
// crashes after some Forward steps have completed but before compensation
// finishes, those steps remain permanently un-compensated ("stuck saga").
//
// To guard against this, supply a StateStore via WithStateStore. The store
// receives a callback for every state transition (step completed, step
// compensated, saga failed) and can persist them to a database, Redis, or any
// durable backend. On restart, your application can query the store, detect
// incomplete sagas, and re-run compensation manually.
//
// A minimal Redis example:
//
//	type RedisStateStore struct{ client *redis.Client }
//
//	func (r *RedisStateStore) OnStepCompleted(ctx context.Context, sagaID, step string) {
//	    r.client.SAdd(ctx, "saga:"+sagaID+":completed", step)
//	}
//	func (r *RedisStateStore) OnStepCompensated(ctx context.Context, sagaID, step string, err error) {
//	    r.client.SAdd(ctx, "saga:"+sagaID+":compensated", step)
//	}
//	func (r *RedisStateStore) OnSagaFailed(ctx context.Context, sagaID, failedStep string, err error) {
//	    r.client.Set(ctx, "saga:"+sagaID+":failed", failedStep, 0)
//	}
//
//	saga := dtx.New(steps...).
//	    WithSagaID("order-42").
//	    WithStateStore(&RedisStateStore{client: rdb})
package dtx

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// StepFn is a function that performs or undoes a single saga step.
// A non-nil return value signals failure.
type StepFn func(ctx context.Context) error

// Step describes one unit of work in a Saga.
type Step struct {
	// Name is a human-readable identifier used in logs and SagaResult.
	Name string

	// Forward is the "do" function. Required.
	Forward StepFn

	// Compensate is the "undo" function called when a later step fails.
	// May be nil for non-compensable steps (e.g. sending an email).
	Compensate StepFn
}

// SagaResult contains the outcome of a Saga.Execute call.
type SagaResult struct {
	// Completed is the ordered list of step names whose Forward ran successfully.
	Completed []string

	// FailedStep is the name of the step whose Forward returned an error.
	// Empty when the saga succeeded.
	FailedStep string

	// CompensationErrors accumulates errors from individual Compensate calls.
	// Non-nil only when at least one compensation failed.
	// A compensation error does not prevent remaining compensations from running.
	CompensationErrors []error

	// Err is the original error returned by the failed Forward step.
	// Nil when the saga completed successfully.
	Err error
}

// Succeeded reports whether the saga completed without any failure.
func (r *SagaResult) Succeeded() bool {
	return r.Err == nil && r.FailedStep == ""
}

// StateStore is an optional persistence hook for Saga execution state.
// Implement it to record step transitions to a durable backend (database,
// Redis, etc.) so that incomplete sagas can be detected and recovered after
// a process crash.
//
// All methods are called synchronously inside Execute; keep them fast or
// delegate to a background queue. Errors returned by the store are logged
// but do not alter the saga's own error handling.
//
// Use NoopStateStore (the zero-value default) when persistence is not needed.
type StateStore interface {
	// OnStepCompleted is called after a Forward function returns nil.
	OnStepCompleted(ctx context.Context, sagaID, step string)

	// OnStepCompensated is called after a Compensate function returns.
	// err is nil when compensation succeeded, non-nil when it failed.
	OnStepCompensated(ctx context.Context, sagaID, step string, err error)

	// OnSagaFailed is called once, immediately after the failing Forward
	// returns an error and before compensation begins.
	OnSagaFailed(ctx context.Context, sagaID, failedStep string, err error)
}

// IncompleteRecord describes a saga that was interrupted before completing
// compensation. Returned by Recovery.ListIncomplete.
type IncompleteRecord struct {
	SagaID     string
	FailedStep string
	UpdatedAt  time.Time
}

// Recovery scans a durable backend for sagas that started compensation but
// never finished (e.g. due to a process crash). Implement this interface on
// top of a StateStore backend (Redis, GORM, etc.) to enable crash recovery.
type Recovery interface {
	// ListIncomplete returns all sagas whose last update is older than
	// staleAfter and that have not been marked done.
	ListIncomplete(ctx context.Context, staleAfter time.Duration) ([]IncompleteRecord, error)

	// MarkDone removes a saga from the incomplete set once compensation has
	// been re-run successfully.
	MarkDone(ctx context.Context, sagaID string) error
}

// NoopStateStore is a StateStore that does nothing. It is the default when
// no store is provided via WithStateStore.
type NoopStateStore struct{}

func (NoopStateStore) OnStepCompleted(_ context.Context, _, _ string)                    {}
func (NoopStateStore) OnStepCompensated(_ context.Context, _, _ string, _ error)         {}
func (NoopStateStore) OnSagaFailed(_ context.Context, _, _ string, _ error)              {}

// Saga executes a sequence of steps with automatic compensation on failure.
// Zero value is not usable; construct via New.
//
// State is held in memory by default. For crash recovery, supply a durable
// StateStore via WithStateStore and a stable identifier via WithSagaID.
type Saga struct {
	steps  []Step
	log    *slog.Logger
	store  StateStore
	sagaID string
}

// New creates a Saga from the given steps.
// Steps are executed in the order supplied.
func New(steps ...Step) *Saga {
	return &Saga{
		steps:  steps,
		log:    slog.Default(),
		store:  NoopStateStore{},
		sagaID: "",
	}
}

// WithLogger replaces the default slog.Default() logger used for execution tracing.
// Passing nil resets the logger to slog.Default().
func (s *Saga) WithLogger(l *slog.Logger) *Saga {
	if l == nil {
		l = slog.Default()
	}
	s.log = l
	return s
}

// WithStateStore attaches a persistence store that receives callbacks on every
// state transition. Use this to implement crash recovery: persist completed
// steps to a durable backend so stuck sagas can be detected and re-compensated
// after a process restart.
//
// Passing nil resets to NoopStateStore (no-op, in-memory only).
func (s *Saga) WithStateStore(store StateStore) *Saga {
	if store == nil {
		store = NoopStateStore{}
	}
	s.store = store
	return s
}

// WithSagaID sets a stable identifier for this execution, forwarded to all
// StateStore callbacks. Required for meaningful persistence — choose an ID
// that uniquely identifies the business transaction (e.g. order ID).
// If unset, an empty string is passed to the store.
func (s *Saga) WithSagaID(id string) *Saga {
	s.sagaID = id
	return s
}

// Execute runs each step's Forward function in order.
//
// On success, Execute returns a SagaResult with Err == nil and FailedStep == "".
//
// On failure, Execute:
//  1. Records the failed step in SagaResult.FailedStep.
//  2. Runs Compensate for every already-completed step, in reverse order.
//  3. Returns a SagaResult with Err set to the forward failure.
//
// Execute is not safe for concurrent use on the same Saga value.
// Create a new Saga per execution if needed.
func (s *Saga) Execute(ctx context.Context) *SagaResult {
	result := &SagaResult{}

	for i, step := range s.steps {
		if step.Forward == nil {
			result.FailedStep = step.Name
			result.Err = fmt.Errorf("dtx: step %q has nil Forward", step.Name)
			s.log.Error("dtx: nil Forward", "step", step.Name)
			s.store.OnSagaFailed(ctx, s.sagaID, step.Name, result.Err)
			s.compensate(ctx, result, i-1)
			return result
		}

		s.log.Info("dtx: executing step", "step", step.Name)
		if err := step.Forward(ctx); err != nil {
			result.FailedStep = step.Name
			result.Err = fmt.Errorf("dtx: step %q failed: %w", step.Name, err)
			s.log.Error("dtx: step failed", "step", step.Name, "err", err)
			s.store.OnSagaFailed(ctx, s.sagaID, step.Name, result.Err)
			s.compensate(ctx, result, i-1)
			return result
		}

		result.Completed = append(result.Completed, step.Name)
		s.store.OnStepCompleted(ctx, s.sagaID, step.Name)
		s.log.Info("dtx: step completed", "step", step.Name)
	}

	return result
}

// compensate runs Compensate for steps[0..lastCompleted] in reverse order.
func (s *Saga) compensate(ctx context.Context, result *SagaResult, lastCompleted int) {
	for i := lastCompleted; i >= 0; i-- {
		step := s.steps[i]
		if step.Compensate == nil {
			s.log.Info("dtx: no compensate defined, skipping", "step", step.Name)
			continue
		}
		s.log.Info("dtx: compensating", "step", step.Name)
		err := step.Compensate(ctx)
		s.store.OnStepCompensated(ctx, s.sagaID, step.Name, err)
		if err != nil {
			s.log.Error("dtx: compensation failed", "step", step.Name, "err", err)
			result.CompensationErrors = append(result.CompensationErrors,
				fmt.Errorf("dtx: compensate %q: %w", step.Name, err))
		} else {
			s.log.Info("dtx: compensation succeeded", "step", step.Name)
		}
	}
}
