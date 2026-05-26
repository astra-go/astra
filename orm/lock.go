package orm

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ─── Lock scope functions ─────────────────────────────────────────────────────
//
// All lock scopes must be used inside an active transaction. Pass them to
// Repository.Scopes() or directly to *gorm.DB.Scopes().
//
// MySQL and PostgreSQL differences are handled transparently by GORM:
//   - FOR UPDATE  → MySQL: FOR UPDATE      / PostgreSQL: FOR UPDATE
//   - FOR SHARE   → MySQL: LOCK IN SHARE MODE / PostgreSQL: FOR SHARE

// ForUpdate returns a GORM scope that appends FOR UPDATE to the query,
// acquiring an exclusive row lock. Concurrent readers using FOR UPDATE or FOR
// SHARE will block until the lock is released.
//
//	err := orm.RunTx(ctx, db, func(tx *gorm.DB) error {
//	    orders, err := repo.WithCtx(ctx).Scopes(orm.ForUpdate()).FindWhere("status = ?", "pending")
//	    // ... process orders, then update
//	})
func ForUpdate() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
}

// ForShare returns a GORM scope for FOR SHARE (PostgreSQL) / LOCK IN SHARE MODE
// (MySQL). Multiple transactions may hold share locks simultaneously; an
// exclusive (UPDATE) lock will block until all share locks are released.
//
//	balances, err := repo.WithCtx(ctx).Scopes(orm.ForShare()).FindWhere("account_id = ?", id)
func ForShare() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "SHARE"})
	}
}

// ForUpdateSkipLocked returns a GORM scope for FOR UPDATE SKIP LOCKED.
// Rows locked by another transaction are silently skipped rather than blocking.
//
// Ideal for concurrent job-queue consumers: each worker fetches a distinct
// batch of unprocessed rows without contention or deadlocks.
//
//	// Worker goroutine — each fetches its own exclusive set:
//	err := orm.RunTx(ctx, db, func(tx *gorm.DB) error {
//	    jobs, err := repo.WithCtx(ctx).
//	        Scopes(orm.ForUpdateSkipLocked()).
//	        FindWhere("status = ?", "queued")
//	    // ...
//	})
func ForUpdateSkipLocked() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
	}
}

// ForUpdateNoWait returns a GORM scope for FOR UPDATE NOWAIT.
// If any selected row is already locked, the query immediately fails with a
// database error rather than waiting — useful when retrying is preferable to
// blocking.
//
//	order, err := repo.WithCtx(ctx).Scopes(orm.ForUpdateNoWait()).First("id = ?", id)
//	if err != nil {
//	    // lock contention: retry or return 409 Conflict
//	}
func ForUpdateNoWait() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "UPDATE", Options: "NOWAIT"})
	}
}

// ForShareSkipLocked returns a GORM scope for FOR SHARE SKIP LOCKED.
// Useful when you need consistent reads of unlocked rows without blocking.
func ForShareSkipLocked() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "SHARE", Options: "SKIP LOCKED"})
	}
}

// WithLock returns a custom locking scope for non-standard syntax.
// strength is "UPDATE" or "SHARE"; options is an optional modifier such as
// "NOWAIT", "SKIP LOCKED", or "OF table_name".
//
//	repo.Scopes(orm.WithLock("UPDATE", "OF orders NOWAIT"))
func WithLock(strength, options string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: strength, Options: options})
	}
}

// ─── Repository lock shortcuts ────────────────────────────────────────────────

// FindByIDForUpdate retrieves the record by primary key and acquires an
// exclusive row lock (FOR UPDATE). Must be called within an active transaction.
//
//	err := orm.RunTx(ctx, db, func(tx *gorm.DB) error {
//	    user, err := repo.WithCtx(ctx).FindByIDForUpdate(ctx, userID)
//	    if err != nil { return err }
//	    user.Balance -= amount
//	    return repo.WithCtx(ctx).Update(ctx, user)
//	})
func (r *Repository[T]) FindByIDForUpdate(ctx context.Context, id any) (*T, error) {
	var entity T
	err := FromCtx(ctx, r.db).Clauses(clause.Locking{Strength: "UPDATE"}).First(&entity, id).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// FindWhereForUpdate retrieves records matching query and acquires exclusive
// row locks (FOR UPDATE). Must be called within an active transaction.
func (r *Repository[T]) FindWhereForUpdate(ctx context.Context, query any, args ...any) ([]T, error) {
	var entities []T
	err := FromCtx(ctx, r.db).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(query, args...).Find(&entities).Error
	if err != nil {
		return nil, err
	}
	return entities, nil
}

// FindByIDForShare retrieves the record by primary key with a shared row lock
// (FOR SHARE / LOCK IN SHARE MODE). Must be called within an active transaction.
func (r *Repository[T]) FindByIDForShare(ctx context.Context, id any) (*T, error) {
	var entity T
	err := FromCtx(ctx, r.db).Clauses(clause.Locking{Strength: "SHARE"}).First(&entity, id).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// ─── Optimistic locking ───────────────────────────────────────────────────────

// ErrOptimisticConflict is returned by UpdateOptimistic when the record was
// modified by another transaction between the read and the write.
// Callers should retry the operation or surface a 409 Conflict to the client.
var ErrOptimisticConflict = errors.New("orm: optimistic lock conflict: record was modified concurrently")

// UpdateOptimistic performs a version-based compare-and-swap update.
//
// It updates the record identified by id only when its "version" column matches
// currentVersion, atomically setting version = currentVersion+1 alongside the
// provided field values. When RowsAffected == 0, ErrOptimisticConflict is
// returned and the caller should re-read the record and retry.
//
// model must be a pointer to the model struct (e.g. &User{}).
// updates must NOT include a "version" key — it is managed automatically.
//
//	// Read phase (outside any transaction):
//	user, err := userRepo.FindByID(userID)
//
//	// Modify in memory:
//	user.Name = "Alice Updated"
//
//	// Write phase — safe against concurrent writes:
//	err = orm.UpdateOptimistic(ctx, db, &User{}, user.ID, user.Version, map[string]any{
//	    "name":  user.Name,
//	    "email": user.Email,
//	})
//	if errors.Is(err, orm.ErrOptimisticConflict) {
//	    // Re-read user and retry, or return 409 to client.
//	}
func UpdateOptimistic(ctx context.Context, db *gorm.DB, model any, id any, currentVersion int, updates map[string]any) error {
	patched := make(map[string]any, len(updates)+1)
	for k, v := range updates {
		patched[k] = v
	}
	patched["version"] = currentVersion + 1

	result := db.WithContext(ctx).
		Model(model).
		Where("id = ? AND version = ?", id, currentVersion).
		Updates(patched)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticConflict
	}
	return nil
}
