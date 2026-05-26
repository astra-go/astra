package dtx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/astra-go/astra/dtx"
	"github.com/astra-go/astra/testutil"
)

var errStep = errors.New("step error")
var errComp = errors.New("compensation error")

// ─── Success path ─────────────────────────────────────────────────────────────

func TestSaga_AllStepsSucceed(t *testing.T) {
	executed := []string{}
	saga := dtx.New(
		dtx.Step{
			Name:    "step-1",
			Forward: func(_ context.Context) error { executed = append(executed, "step-1"); return nil },
		},
		dtx.Step{
			Name:    "step-2",
			Forward: func(_ context.Context) error { executed = append(executed, "step-2"); return nil },
		},
		dtx.Step{
			Name:    "step-3",
			Forward: func(_ context.Context) error { executed = append(executed, "step-3"); return nil },
		},
	)

	result := saga.Execute(context.Background())

	testutil.AssertNoError(t, result.Err)
	if !result.Succeeded() {
		t.Error("expected Succeeded() == true")
	}
	if result.FailedStep != "" {
		t.Errorf("expected no FailedStep, got %q", result.FailedStep)
	}
	if len(result.Completed) != 3 {
		t.Fatalf("expected 3 completed, got %v", result.Completed)
	}
	if executed[0] != "step-1" || executed[1] != "step-2" || executed[2] != "step-3" {
		t.Errorf("execution order wrong: %v", executed)
	}
}

// ─── First step fails — no compensation needed ────────────────────────────────

func TestSaga_FirstStepFails_NoCompensation(t *testing.T) {
	compensated := false
	saga := dtx.New(
		dtx.Step{
			Name:       "step-1",
			Forward:    func(_ context.Context) error { return errStep },
			Compensate: func(_ context.Context) error { compensated = true; return nil },
		},
		dtx.Step{
			Name:    "step-2",
			Forward: func(_ context.Context) error { panic("should not execute") },
		},
	)

	result := saga.Execute(context.Background())

	if result.Succeeded() {
		t.Error("expected failure")
	}
	testutil.AssertErrorIs(t, result.Err, errStep)
	testutil.AssertEqual(t, "step-1", result.FailedStep)
	if len(result.Completed) != 0 {
		t.Errorf("expected no completed steps, got %v", result.Completed)
	}
	// step-1 failed, nothing was completed before it — no compensation.
	if compensated {
		t.Error("step-1 failed at forward; its own compensate must NOT run")
	}
}

// ─── Second step fails — first step compensated ───────────────────────────────

func TestSaga_SecondStepFails_FirstCompensated(t *testing.T) {
	order := []string{}

	saga := dtx.New(
		dtx.Step{
			Name:       "step-1",
			Forward:    func(_ context.Context) error { order = append(order, "fwd-1"); return nil },
			Compensate: func(_ context.Context) error { order = append(order, "cmp-1"); return nil },
		},
		dtx.Step{
			Name:    "step-2",
			Forward: func(_ context.Context) error { order = append(order, "fwd-2"); return errStep },
		},
	)

	result := saga.Execute(context.Background())

	testutil.AssertErrorIs(t, result.Err, errStep)
	testutil.AssertEqual(t, "step-2", result.FailedStep)
	if len(result.Completed) != 1 || result.Completed[0] != "step-1" {
		t.Errorf("expected [step-1] completed, got %v", result.Completed)
	}

	// forward order: fwd-1, fwd-2; then reverse compensation: cmp-1
	if len(order) != 3 || order[0] != "fwd-1" || order[1] != "fwd-2" || order[2] != "cmp-1" {
		t.Errorf("unexpected execution order: %v", order)
	}
}

// ─── Mid-chain failure, multi-step compensation ──────────────────────────────

func TestSaga_ThirdStepFails_CompensatesInReverseOrder(t *testing.T) {
	order := []string{}

	saga := dtx.New(
		dtx.Step{
			Name:       "a",
			Forward:    func(_ context.Context) error { order = append(order, "fwd-a"); return nil },
			Compensate: func(_ context.Context) error { order = append(order, "cmp-a"); return nil },
		},
		dtx.Step{
			Name:       "b",
			Forward:    func(_ context.Context) error { order = append(order, "fwd-b"); return nil },
			Compensate: func(_ context.Context) error { order = append(order, "cmp-b"); return nil },
		},
		dtx.Step{
			Name:    "c",
			Forward: func(_ context.Context) error { order = append(order, "fwd-c"); return errStep },
		},
	)

	result := saga.Execute(context.Background())

	testutil.AssertErrorIs(t, result.Err, errStep)
	testutil.AssertEqual(t, "c", result.FailedStep)

	// Compensation must run in reverse order: b → a
	want := []string{"fwd-a", "fwd-b", "fwd-c", "cmp-b", "cmp-a"}
	if len(order) != len(want) {
		t.Fatalf("order len mismatch: want %v, got %v", want, order)
	}
	for i, v := range want {
		if order[i] != v {
			t.Errorf("order[%d]: want %s, got %s", i, v, order[i])
		}
	}
}

// ─── Nil Compensate is silently skipped ──────────────────────────────────────

func TestSaga_NilCompensate_Skipped(t *testing.T) {
	order := []string{}

	saga := dtx.New(
		dtx.Step{
			Name:       "a",
			Forward:    func(_ context.Context) error { order = append(order, "fwd-a"); return nil },
			Compensate: func(_ context.Context) error { order = append(order, "cmp-a"); return nil },
		},
		dtx.Step{
			Name:       "b",
			Forward:    func(_ context.Context) error { order = append(order, "fwd-b"); return nil },
			Compensate: nil, // intentionally no compensate
		},
		dtx.Step{
			Name:    "c",
			Forward: func(_ context.Context) error { return errStep },
		},
	)

	result := saga.Execute(context.Background())

	testutil.AssertErrorIs(t, result.Err, errStep)
	// b has no compensate, only a is compensated
	if len(order) != 3 || order[2] != "cmp-a" {
		t.Errorf("expected cmp-a after skipping b; got %v", order)
	}
	// No compensation error from skipping nil
	if len(result.CompensationErrors) != 0 {
		t.Errorf("nil compensate should not produce error, got %v", result.CompensationErrors)
	}
}

// ─── Compensation error is collected, others still run ───────────────────────

func TestSaga_CompensationError_CollectedAndOthersContinue(t *testing.T) {
	compensated := []string{}

	saga := dtx.New(
		dtx.Step{
			Name:       "a",
			Forward:    func(_ context.Context) error { return nil },
			Compensate: func(_ context.Context) error { compensated = append(compensated, "cmp-a"); return nil },
		},
		dtx.Step{
			Name:       "b",
			Forward:    func(_ context.Context) error { return nil },
			Compensate: func(_ context.Context) error { return errComp }, // fails
		},
		dtx.Step{
			Name:    "c",
			Forward: func(_ context.Context) error { return errStep },
		},
	)

	result := saga.Execute(context.Background())

	testutil.AssertErrorIs(t, result.Err, errStep)
	// Compensation error must be collected
	if len(result.CompensationErrors) != 1 {
		t.Fatalf("expected 1 compensation error, got %d: %v", len(result.CompensationErrors), result.CompensationErrors)
	}
	testutil.AssertErrorIs(t, result.CompensationErrors[0], errComp)
	// "a" must still have been compensated despite "b"'s compensation failing
	if len(compensated) != 1 || compensated[0] != "cmp-a" {
		t.Errorf("expected cmp-a to run despite b's error; got %v", compensated)
	}
}

// ─── Nil Forward is an error ──────────────────────────────────────────────────

func TestSaga_NilForward_ReturnsError(t *testing.T) {
	saga := dtx.New(
		dtx.Step{Name: "broken", Forward: nil},
	)
	result := saga.Execute(context.Background())
	if result.Succeeded() {
		t.Error("nil Forward should fail")
	}
	if result.Err == nil {
		t.Error("expected non-nil Err for nil Forward")
	}
}

// ─── Single-step success ──────────────────────────────────────────────────────

func TestSaga_SingleStep_Success(t *testing.T) {
	ran := false
	saga := dtx.New(dtx.Step{
		Name:    "only",
		Forward: func(_ context.Context) error { ran = true; return nil },
	})
	result := saga.Execute(context.Background())
	testutil.AssertNoError(t, result.Err)
	if !ran {
		t.Error("single step did not run")
	}
}

// ─── Context cancellation propagates ─────────────────────────────────────────

func TestSaga_ContextCancelled_StepReceivesIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	var receivedCtx context.Context
	saga := dtx.New(dtx.Step{
		Name: "ctx-check",
		Forward: func(c context.Context) error {
			receivedCtx = c
			return c.Err() // returns context.Canceled
		},
	})

	result := saga.Execute(ctx)
	if result.Succeeded() {
		t.Error("expected failure when context is cancelled")
	}
	if !errors.Is(receivedCtx.Err(), context.Canceled) {
		t.Error("context cancellation was not propagated to Forward")
	}
}

// ─── Succeeded helper ────────────────────────────────────────────────────────

func TestSagaResult_Succeeded(t *testing.T) {
	ok := &dtx.SagaResult{}
	if !ok.Succeeded() {
		t.Error("empty SagaResult should be Succeeded()")
	}

	fail := &dtx.SagaResult{Err: errStep, FailedStep: "x"}
	if fail.Succeeded() {
		t.Error("failed SagaResult should not be Succeeded()")
	}
}

// ─── WithLogger does not panic ────────────────────────────────────────────────

func TestSaga_WithLogger_DoesNotPanic(t *testing.T) {
	saga := dtx.New(
		dtx.Step{Name: "x", Forward: func(_ context.Context) error { return nil }},
	).WithLogger(nil) // nil logger falls back to slog.Default implicitly
	// Just ensure it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("WithLogger(nil) panicked: %v", r)
		}
	}()
	saga.Execute(context.Background()) //nolint
}

// ─── Empty saga succeeds ──────────────────────────────────────────────────────

func TestSaga_EmptySaga_Succeeds(t *testing.T) {
	result := dtx.New().Execute(context.Background())
	if !result.Succeeded() {
		t.Errorf("empty saga should succeed; got err=%v failedStep=%q", result.Err, result.FailedStep)
	}
	if len(result.Completed) != 0 {
		t.Errorf("expected no completed steps, got %v", result.Completed)
	}
	if len(result.CompensationErrors) != 0 {
		t.Errorf("expected no compensation errors, got %v", result.CompensationErrors)
	}
}

// ─── Multiple compensation failures — all collected ───────────────────────────

// TestSaga_MultipleCompensationErrors_AllCollected verifies that when N>1
// compensation functions all return errors, every error is accumulated in
// CompensationErrors in reverse-execution order (last completed compensated
// first) and that the slice length equals the number of failing compensations.
func TestSaga_MultipleCompensationErrors_AllCollected(t *testing.T) {
	var errA = errors.New("comp-a-fail")
	var errB = errors.New("comp-b-fail")

	saga := dtx.New(
		dtx.Step{
			Name:       "a",
			Forward:    func(_ context.Context) error { return nil },
			Compensate: func(_ context.Context) error { return errA },
		},
		dtx.Step{
			Name:       "b",
			Forward:    func(_ context.Context) error { return nil },
			Compensate: func(_ context.Context) error { return errB },
		},
		dtx.Step{
			Name:    "c",
			Forward: func(_ context.Context) error { return errStep },
		},
	)

	result := saga.Execute(context.Background())

	testutil.AssertErrorIs(t, result.Err, errStep)
	testutil.AssertEqual(t, "c", result.FailedStep)

	if len(result.CompensationErrors) != 2 {
		t.Fatalf("expected 2 compensation errors, got %d: %v",
			len(result.CompensationErrors), result.CompensationErrors)
	}
	// Compensation runs b → a (reverse order), so errors are appended in that order.
	testutil.AssertErrorIs(t, result.CompensationErrors[0], errB)
	testutil.AssertErrorIs(t, result.CompensationErrors[1], errA)
}

// ─── Context propagation to compensation ──────────────────────────────────────

// TestSaga_ContextPassedToCompensation verifies that the Execute context is
// forwarded verbatim to compensation functions — including cancellation state.
// The first step cancels the shared context and returns nil (success); the
// second step fails because ctx.Err() is non-nil.  The compensation of the
// first step must then receive the same cancelled context.
func TestSaga_ContextPassedToCompensation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var compensateCtx context.Context

	saga := dtx.New(
		dtx.Step{
			Name: "a",
			Forward: func(c context.Context) error {
				cancel() // cancel the shared context; step still succeeds
				return nil
			},
			Compensate: func(c context.Context) error {
				compensateCtx = c
				return nil
			},
		},
		dtx.Step{
			Name:    "b",
			Forward: func(c context.Context) error { return c.Err() },
		},
	)

	result := saga.Execute(ctx)

	if result.Succeeded() {
		t.Error("expected failure (context was cancelled before step-b ran)")
	}
	if compensateCtx == nil {
		t.Fatal("compensation of step-a did not run")
	}
	if !errors.Is(compensateCtx.Err(), context.Canceled) {
		t.Errorf("compensation received non-cancelled context: %v", compensateCtx.Err())
	}
}

// ─── StateStore ───────────────────────────────────────────────────────────────

type recordingStore struct {
	completed   []string // sagaID+":"+step
	compensated []string // sagaID+":"+step (":ok" or ":err")
	failed      []string // sagaID+":"+failedStep
}

func (r *recordingStore) OnStepCompleted(_ context.Context, sagaID, step string) {
	r.completed = append(r.completed, sagaID+":"+step)
}
func (r *recordingStore) OnStepCompensated(_ context.Context, sagaID, step string, err error) {
	suffix := ":ok"
	if err != nil {
		suffix = ":err"
	}
	r.compensated = append(r.compensated, sagaID+":"+step+suffix)
}
func (r *recordingStore) OnSagaFailed(_ context.Context, sagaID, failedStep string, _ error) {
	r.failed = append(r.failed, sagaID+":"+failedStep)
}

func TestStateStore_SuccessPath_OnlyCompletedCalled(t *testing.T) {
	store := &recordingStore{}
	saga := dtx.New(
		dtx.Step{Name: "s1", Forward: func(_ context.Context) error { return nil }},
		dtx.Step{Name: "s2", Forward: func(_ context.Context) error { return nil }},
	).WithStateStore(store).WithSagaID("tx-1")

	result := saga.Execute(context.Background())

	testutil.AssertNoError(t, result.Err)
	if len(store.completed) != 2 {
		t.Fatalf("expected 2 OnStepCompleted calls, got %v", store.completed)
	}
	testutil.AssertEqual(t, "tx-1:s1", store.completed[0])
	testutil.AssertEqual(t, "tx-1:s2", store.completed[1])
	if len(store.failed) != 0 {
		t.Errorf("OnSagaFailed must not be called on success: %v", store.failed)
	}
	if len(store.compensated) != 0 {
		t.Errorf("OnStepCompensated must not be called on success: %v", store.compensated)
	}
}

func TestStateStore_FailurePath_AllCallbacksFired(t *testing.T) {
	store := &recordingStore{}
	saga := dtx.New(
		dtx.Step{
			Name:       "a",
			Forward:    func(_ context.Context) error { return nil },
			Compensate: func(_ context.Context) error { return nil },
		},
		dtx.Step{
			Name:       "b",
			Forward:    func(_ context.Context) error { return nil },
			Compensate: func(_ context.Context) error { return errComp },
		},
		dtx.Step{
			Name:    "c",
			Forward: func(_ context.Context) error { return errStep },
		},
	).WithStateStore(store).WithSagaID("tx-2")

	saga.Execute(context.Background())

	// Completed: a, b
	if len(store.completed) != 2 {
		t.Fatalf("expected 2 completed, got %v", store.completed)
	}
	testutil.AssertEqual(t, "tx-2:a", store.completed[0])
	testutil.AssertEqual(t, "tx-2:b", store.completed[1])

	// Failed: c
	if len(store.failed) != 1 {
		t.Fatalf("expected 1 failed, got %v", store.failed)
	}
	testutil.AssertEqual(t, "tx-2:c", store.failed[0])

	// Compensated: b (err), a (ok) — reverse order
	if len(store.compensated) != 2 {
		t.Fatalf("expected 2 compensated, got %v", store.compensated)
	}
	testutil.AssertEqual(t, "tx-2:b:err", store.compensated[0])
	testutil.AssertEqual(t, "tx-2:a:ok", store.compensated[1])
}

func TestStateStore_NilStore_UsesNoop(t *testing.T) {
	saga := dtx.New(
		dtx.Step{Name: "x", Forward: func(_ context.Context) error { return nil }},
	).WithStateStore(nil)
	// Must not panic
	result := saga.Execute(context.Background())
	testutil.AssertNoError(t, result.Err)
}

func TestNoopStateStore_ImplementsInterface(t *testing.T) {
	// Compile-time check: NoopStateStore satisfies StateStore.
	var _ dtx.StateStore = dtx.NoopStateStore{}
}
