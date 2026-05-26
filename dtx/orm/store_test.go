package orm_test

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/astra-go/astra/dtx"
	dtxorm "github.com/astra-go/astra/dtx/orm"
)

// openDB opens an in-memory SQLite database and runs AutoMigrate.
func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := dtxorm.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

// ─── StateStore ───────────────────────────────────────────────────────────────

func TestStateStore_SuccessPath_StepRecorded(t *testing.T) {
	db := openDB(t)
	store := dtxorm.NewStateStore(db, dtxorm.Config{})

	ctx := context.Background()
	store.OnStepCompleted(ctx, "saga-1", "step-a")
	store.OnStepCompleted(ctx, "saga-1", "step-b")

	var count int64
	db.Table("saga_step_records").
		Where("saga_id = ? AND event = ?", "saga-1", "completed").
		Count(&count)
	if count != 2 {
		t.Errorf("expected 2 completed step records, got %d", count)
	}

	var rec struct{ Status string }
	db.Table("saga_records").Select("status").Where("id = ?", "saga-1").Scan(&rec)
	if rec.Status != "running" {
		t.Errorf("expected status=running, got %q", rec.Status)
	}
}

func TestStateStore_FailurePath_StatusAndStepRecorded(t *testing.T) {
	db := openDB(t)
	store := dtxorm.NewStateStore(db, dtxorm.Config{})

	ctx := context.Background()
	store.OnStepCompleted(ctx, "saga-2", "step-a")
	store.OnSagaFailed(ctx, "saga-2", "step-b", nil)
	store.OnStepCompensated(ctx, "saga-2", "step-a", nil)

	var rec struct {
		Status     string
		FailedStep string
	}
	db.Table("saga_records").Select("status, failed_step").Where("id = ?", "saga-2").Scan(&rec)
	if rec.Status != "failed" {
		t.Errorf("expected status=failed, got %q", rec.Status)
	}
	if rec.FailedStep != "step-b" {
		t.Errorf("expected failed_step=step-b, got %q", rec.FailedStep)
	}

	var compCount int64
	db.Table("saga_step_records").
		Where("saga_id = ? AND event = ?", "saga-2", "compensated").
		Count(&compCount)
	if compCount != 1 {
		t.Errorf("expected 1 compensated record, got %d", compCount)
	}
}

func TestStateStore_CompensationError_RecordedAsCompFailed(t *testing.T) {
	db := openDB(t)
	store := dtxorm.NewStateStore(db, dtxorm.Config{})

	ctx := context.Background()
	store.OnStepCompleted(ctx, "saga-3", "step-a")
	store.OnSagaFailed(ctx, "saga-3", "step-b", nil)
	store.OnStepCompensated(ctx, "saga-3", "step-a", errFake)

	var count int64
	db.Table("saga_step_records").
		Where("saga_id = ? AND event = ?", "saga-3", "comp_failed").
		Count(&count)
	if count != 1 {
		t.Errorf("expected 1 comp_failed record, got %d", count)
	}
}

var errFake = &fakeErr{}

type fakeErr struct{}

func (e *fakeErr) Error() string { return "fake error" }

// TestStateStore_Idempotent verifies that duplicate OnStepCompleted calls
// (e.g. after a crash and replay) do not create duplicate step records.
func TestStateStore_Idempotent_DuplicateStepIgnored(t *testing.T) {
	db := openDB(t)
	store := dtxorm.NewStateStore(db, dtxorm.Config{})

	ctx := context.Background()
	store.OnStepCompleted(ctx, "saga-4", "step-a")
	store.OnStepCompleted(ctx, "saga-4", "step-a") // duplicate

	var count int64
	db.Table("saga_step_records").
		Where("saga_id = ? AND step_name = ? AND event = ?", "saga-4", "step-a", "completed").
		Count(&count)
	if count != 1 {
		t.Errorf("expected 1 step record after duplicate call, got %d", count)
	}
}

// ─── Integration with dtx.Saga ────────────────────────────────────────────────

func TestStateStore_IntegrationWithSaga(t *testing.T) {
	db := openDB(t)
	store := dtxorm.NewStateStore(db, dtxorm.Config{})

	saga := dtx.New(
		dtx.Step{
			Name:       "a",
			Forward:    func(_ context.Context) error { return nil },
			Compensate: func(_ context.Context) error { return nil },
		},
		dtx.Step{
			Name:    "b",
			Forward: func(_ context.Context) error { return errFake },
		},
	).WithSagaID("integ-1").WithStateStore(store)

	result := saga.Execute(context.Background())
	if result.Succeeded() {
		t.Fatal("expected saga to fail")
	}

	var rec struct{ Status string }
	db.Table("saga_records").Select("status").Where("id = ?", "integ-1").Scan(&rec)
	if rec.Status != "failed" {
		t.Errorf("expected status=failed after saga failure, got %q", rec.Status)
	}
}

// ─── Recovery ─────────────────────────────────────────────────────────────────

func TestRecovery_ListIncomplete_ReturnsStaleFailed(t *testing.T) {
	db := openDB(t)
	store := dtxorm.NewStateStore(db, dtxorm.Config{})
	recovery := dtxorm.NewRecovery(db, dtxorm.Config{})

	ctx := context.Background()

	// Simulate a saga that failed 1 hour ago by inserting directly.
	store.OnSagaFailed(ctx, "stale-saga", "step-x", nil)
	// Backdate updated_at to simulate staleness.
	db.Table("saga_records").
		Where("id = ?", "stale-saga").
		Update("updated_at", time.Now().UTC().Add(-1*time.Hour))

	// A fresh saga that failed just now — should NOT appear.
	store.OnSagaFailed(ctx, "fresh-saga", "step-y", nil)

	incomplete, err := recovery.ListIncomplete(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("ListIncomplete: %v", err)
	}

	found := false
	for _, rec := range incomplete {
		if rec.SagaID == "stale-saga" {
			found = true
			if rec.FailedStep != "step-x" {
				t.Errorf("expected FailedStep=step-x, got %q", rec.FailedStep)
			}
		}
		if rec.SagaID == "fresh-saga" {
			t.Error("fresh-saga should not appear in ListIncomplete")
		}
	}
	if !found {
		t.Error("stale-saga not found in ListIncomplete")
	}
}

func TestRecovery_MarkDone_RemovesFromList(t *testing.T) {
	db := openDB(t)
	store := dtxorm.NewStateStore(db, dtxorm.Config{})
	recovery := dtxorm.NewRecovery(db, dtxorm.Config{})

	ctx := context.Background()
	store.OnSagaFailed(ctx, "done-saga", "step-z", nil)
	db.Table("saga_records").
		Where("id = ?", "done-saga").
		Update("updated_at", time.Now().UTC().Add(-2*time.Hour))

	if err := recovery.MarkDone(ctx, "done-saga"); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}

	incomplete, err := recovery.ListIncomplete(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("ListIncomplete: %v", err)
	}
	for _, rec := range incomplete {
		if rec.SagaID == "done-saga" {
			t.Error("done-saga should not appear after MarkDone")
		}
	}
}

func TestRecovery_ListIncomplete_Empty(t *testing.T) {
	db := openDB(t)
	recovery := dtxorm.NewRecovery(db, dtxorm.Config{})

	incomplete, err := recovery.ListIncomplete(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("ListIncomplete on empty DB: %v", err)
	}
	if len(incomplete) != 0 {
		t.Errorf("expected empty result, got %v", incomplete)
	}
}
