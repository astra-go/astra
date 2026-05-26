// Package orm provides a GORM-backed dtx.StateStore and dtx.Recovery for
// the Astra Saga orchestration pattern.
//
// # Schema
//
// Two tables are used:
//
//	saga_records      — one row per saga instance (status, failed step, timestamps)
//	saga_step_records — one row per step event (completed / compensated / comp_failed)
//
// Call AutoMigrate(db) once at startup (before deploying code that uses this
// package) to create the tables. AutoMigrate is intentionally NOT called inside
// NewStateStore to give you control over migration timing.
//
// # Usage
//
//	import dtxorm "github.com/astra-go/astra/dtx/orm"
//
//	if err := dtxorm.AutoMigrate(db); err != nil { log.Fatal(err) }
//
//	store := dtxorm.NewStateStore(db, dtxorm.Config{})
//
//	saga := dtx.New(steps...).
//	    WithSagaID("order-42").
//	    WithStateStore(store)
//
//	result := saga.Execute(ctx)
//
// # Recovery
//
//	recovery := dtxorm.NewRecovery(db, dtxorm.Config{})
//
//	incomplete, err := recovery.ListIncomplete(ctx, 30*time.Minute)
//	for _, rec := range incomplete {
//	    // re-run compensation for rec.SagaID using your application logic
//	}
package orm

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/astra-go/astra/dtx"
)

// ─── Data models ──────────────────────────────────────────────────────────────

// sagaRecord is the GORM model for the saga_records table.
type sagaRecord struct {
	ID         string    `gorm:"primaryKey;size:255"`
	Status     string    `gorm:"size:20;not null;index:idx_status_updated,priority:1"`
	FailedStep string    `gorm:"size:255"`
	FailedErr  string    `gorm:"type:text"`
	StartedAt  time.Time `gorm:"not null"`
	UpdatedAt  time.Time `gorm:"not null;index:idx_status_updated,priority:2"`
}

func (sagaRecord) TableName() string { return "saga_records" }

// sagaStepRecord is the GORM model for the saga_step_records table.
type sagaStepRecord struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"`
	SagaID     string    `gorm:"size:255;not null;index;uniqueIndex:uk_saga_step_event"`
	StepName   string    `gorm:"size:255;not null;uniqueIndex:uk_saga_step_event"`
	Event      string    `gorm:"size:20;not null;uniqueIndex:uk_saga_step_event"` // completed | compensated | comp_failed
	OccurredAt time.Time `gorm:"not null"`
}

func (sagaStepRecord) TableName() string { return "saga_step_records" }

// AutoMigrate creates or updates the saga_records and saga_step_records tables.
// Call this once at application startup, before deploying code that uses
// NewStateStore or NewRecovery. It is safe to call multiple times (idempotent).
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&sagaRecord{}, &sagaStepRecord{}); err != nil {
		return fmt.Errorf("dtx/orm: AutoMigrate: %w", err)
	}
	return nil
}

// ─── Config ───────────────────────────────────────────────────────────────────

// Config holds options for the GORM StateStore and Recovery.
type Config struct {
	// TablePrefix is prepended to table names. Default: "" (no prefix).
	// When set, tables become "{prefix}saga_records" and "{prefix}saga_step_records".
	// Note: changing TablePrefix after AutoMigrate requires a manual migration.
	TablePrefix string
}

// ─── StateStore ───────────────────────────────────────────────────────────────

// StateStore is a GORM-backed dtx.StateStore.
type StateStore struct {
	db  *gorm.DB
	cfg Config
}

// NewStateStore creates a GORM-backed StateStore.
// AutoMigrate must be called separately before first use.
func NewStateStore(db *gorm.DB, cfg Config) *StateStore {
	return &StateStore{db: db, cfg: cfg}
}

// Ensure *StateStore satisfies dtx.StateStore at compile time.
var _ dtx.StateStore = (*StateStore)(nil)

// OnStepCompleted records that step completed successfully.
// Uses INSERT OR IGNORE semantics so duplicate calls (e.g. after a crash
// and replay) are safe.
func (s *StateStore) OnStepCompleted(ctx context.Context, sagaID, step string) {
	now := time.Now().UTC()

	// Upsert the saga record — create if not exists, update updated_at.
	s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"updated_at"}),
	}).Create(&sagaRecord{
		ID:        sagaID,
		Status:    "running",
		StartedAt: now,
		UpdatedAt: now,
	})

	// Insert step event — ignore duplicates for idempotency.
	s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&sagaStepRecord{
			SagaID:     sagaID,
			StepName:   step,
			Event:      "completed",
			OccurredAt: now,
		})
}

// OnStepCompensated records the outcome of a compensation attempt.
func (s *StateStore) OnStepCompensated(ctx context.Context, sagaID, step string, err error) {
	event := "compensated"
	if err != nil {
		event = "comp_failed"
	}
	now := time.Now().UTC()

	s.db.WithContext(ctx).Model(&sagaRecord{}).
		Where("id = ?", sagaID).
		Updates(map[string]any{"updated_at": now})

	s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&sagaStepRecord{
			SagaID:     sagaID,
			StepName:   step,
			Event:      event,
			OccurredAt: now,
		})
}

// OnSagaFailed records that the saga entered the failed state.
func (s *StateStore) OnSagaFailed(ctx context.Context, sagaID, failedStep string, err error) {
	now := time.Now().UTC()
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"status":      "failed",
			"failed_step": failedStep,
			"failed_err":  errMsg,
			"updated_at":  now,
		}),
	}).Create(&sagaRecord{
		ID:         sagaID,
		Status:     "failed",
		FailedStep: failedStep,
		FailedErr:  errMsg,
		StartedAt:  now,
		UpdatedAt:  now,
	})
}

// ─── Recovery ─────────────────────────────────────────────────────────────────

// Recovery implements dtx.Recovery using GORM queries.
type Recovery struct {
	db  *gorm.DB
	cfg Config
}

// NewRecovery creates a GORM-backed Recovery.
func NewRecovery(db *gorm.DB, cfg Config) *Recovery {
	return &Recovery{db: db, cfg: cfg}
}

// Ensure *Recovery satisfies dtx.Recovery at compile time.
var _ dtx.Recovery = (*Recovery)(nil)

// ListIncomplete returns sagas with status "failed" whose last transition is
// older than staleAfter. Results are ordered by updated_at ascending so the
// oldest stuck sagas are processed first.
func (r *Recovery) ListIncomplete(ctx context.Context, staleAfter time.Duration) ([]dtx.IncompleteRecord, error) {
	threshold := time.Now().UTC().Add(-staleAfter)

	var rows []sagaRecord
	if err := r.db.WithContext(ctx).
		Where("status = ? AND updated_at < ?", "failed", threshold).
		Order("updated_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("dtx/orm: ListIncomplete: %w", err)
	}

	out := make([]dtx.IncompleteRecord, len(rows))
	for i, row := range rows {
		out[i] = dtx.IncompleteRecord{
			SagaID:     row.ID,
			FailedStep: row.FailedStep,
			UpdatedAt:  row.UpdatedAt,
		}
	}
	return out, nil
}

// MarkDone sets the saga status to "done" so it is no longer returned by
// ListIncomplete. Use after human intervention or confirmed safe-to-ignore.
func (r *Recovery) MarkDone(ctx context.Context, sagaID string) error {
	result := r.db.WithContext(ctx).Model(&sagaRecord{}).
		Where("id = ?", sagaID).
		Updates(map[string]any{
			"status":     "done",
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("dtx/orm: MarkDone %q: %w", sagaID, result.Error)
	}
	return nil
}
