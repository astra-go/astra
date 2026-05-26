package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/astra-go/astra/contract"
	"gorm.io/gorm"
)

// ErrClickHouseTxNotSupported is returned when RunTx or RunTxWithOptions is
// called with a ClickHouse-backed *gorm.DB. ClickHouse does not support ACID
// transactions; use batch inserts instead.
var ErrClickHouseTxNotSupported = errors.New("orm: transactions are not supported on ClickHouse; use batch inserts instead")

// RunTx executes fn inside a new database transaction.
//
// The transaction is committed when fn returns nil; rolled back on any error.
// A panic inside fn triggers a rollback before the panic is re-propagated.
//
// The active *gorm.DB (tx) passed to fn is scoped to ctx so GORM plugins that
// read from context work correctly. Service-layer code that calls
// orm.FromCtx(ctx, db) will automatically participate in the same transaction
// when ctx is the one returned by the parent handler.
//
//	err := orm.RunTx(ctx, db, func(tx *gorm.DB) error {
//	    if err := tx.Create(&order).Error; err != nil {
//	        return err  // triggers rollback
//	    }
//	    return tx.Model(&inventory).UpdateColumn("stock", gorm.Expr("stock - ?", 1)).Error
//	})
func RunTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	return RunTxWithOptions(ctx, db, nil, fn)
}

// RunTxWithOptions is like RunTx but with explicit sql.TxOptions, allowing the
// caller to set the isolation level or mark the transaction as read-only.
//
//	// Serializable isolation — prevents phantom reads.
//	err := orm.RunTxWithOptions(ctx, db,
//	    &sql.TxOptions{Isolation: sql.LevelSerializable},
//	    func(tx *gorm.DB) error { ... },
//	)
//
//	// Read-only transaction for consistent multi-query reads (PostgreSQL).
//	err := orm.RunTxWithOptions(ctx, db,
//	    &sql.TxOptions{ReadOnly: true},
//	    func(tx *gorm.DB) error { ... },
//	)
func RunTxWithOptions(ctx context.Context, db *gorm.DB, opts *sql.TxOptions, fn func(tx *gorm.DB) error) error {
	if isClickHouse(db) {
		return ErrClickHouseTxNotSupported
	}
	tx := db.WithContext(ctx).Begin(opts)
	if tx.Error != nil {
		return fmt.Errorf("orm: begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback().Error
			panic(r) // re-panic so upstream recovery middleware can log the stack
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback().Error; rbErr != nil && rbErr != gorm.ErrInvalidTransaction {
			return fmt.Errorf("orm: rollback (original: %v): %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit().Error; err != nil {
		_ = tx.Rollback().Error
		return fmt.Errorf("orm: commit: %w", err)
	}
	return nil
}

// RunNestedTx runs fn inside a SAVEPOINT when an active transaction is already
// present in ctx, or starts a brand-new transaction when there is none.
//
// On error, only the savepoint is rolled back — the outer transaction is not
// affected and can continue or be retried. name must be unique within the outer
// transaction (used as the SAVEPOINT identifier).
//
//	// In an outer transaction:
//	err := orm.RunTx(ctx, db, func(outer *gorm.DB) error {
//	    // Attempt to create audit log; if it fails, skip but continue.
//	    _ = orm.RunNestedTx(WithTx(ctx, outer), db, "audit", func(tx *gorm.DB) error {
//	        return tx.Create(&auditEntry).Error
//	    })
//	    return outer.Create(&order).Error  // outer tx still intact
//	})
func RunNestedTx(ctx context.Context, db *gorm.DB, name string, fn func(tx *gorm.DB) error) error {
	if outer, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok && outer != nil {
		if err := outer.SavePoint(name).Error; err != nil {
			return fmt.Errorf("orm: savepoint %q: %w", name, err)
		}
		if err := fn(outer); err != nil {
			if rbErr := outer.RollbackTo(name).Error; rbErr != nil {
				return fmt.Errorf("orm: rollback to %q (original: %v): %w", name, err, rbErr)
			}
			return err
		}
		return nil
	}
	return RunTx(ctx, db, fn)
}

// RunTx executes fn in a transaction scoped to the repository's underlying DB.
// fn receives a transaction-bound *Repository[T] so all existing method calls
// (Create, Update, FindByID, …) continue to work without modification.
//
//	err := orderRepo.RunTx(ctx, func(r *orm.Repository[Order]) error {
//	    if err := r.Create(ctx, &order); err != nil { return err }
//	    return inventoryRepo.WithCtx(ctx).Updates(ctx, itemID, map[string]any{
//	        "stock": gorm.Expr("stock - 1"),
//	    })
//	})
func (r *Repository[T]) RunTx(ctx context.Context, fn func(repo *Repository[T]) error) error {
	return RunTx(ctx, r.db, func(tx *gorm.DB) error {
		return fn(&Repository[T]{db: tx})
	})
}

// ─── GORMTxRunner ─────────────────────────────────────────────────────────────

// GORMTxRunner implements contract.TxRunner using a *gorm.DB.
//
// Unlike Repository[T].RunTx (which passes a concrete *Repository[T] to fn),
// GORMTxRunner writes the transaction into the context via orm.WithTx so that
// any service-layer code using orm.FromCtx(txCtx, db) participates in the same
// transaction — including cross-repository operations:
//
//	txRunner := orm.NewTxRunner(db)
//
//	err := txRunner.RunTx(ctx, func(txCtx context.Context) error {
//	    if err := userRepo.Create(txCtx, &user); err != nil {
//	        return err
//	    }
//	    return orderRepo.Create(txCtx, &order)
//	})
type GORMTxRunner struct {
	db *gorm.DB
}

// Compile-time assertion: *GORMTxRunner must implement contract.TxRunner.
var _ contract.TxRunner = (*GORMTxRunner)(nil)

// NewTxRunner creates a GORMTxRunner backed by db.
func NewTxRunner(db *gorm.DB) *GORMTxRunner {
	return &GORMTxRunner{db: db}
}

// RunTx implements contract.TxRunner.
// fn receives txCtx — a derived context carrying the active *gorm.DB transaction
// via orm.WithTx.  All Repository[T] methods called with txCtx will automatically
// use this transaction through orm.FromCtx.
func (r *GORMTxRunner) RunTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return RunTx(ctx, r.db, func(tx *gorm.DB) error {
		return fn(WithTx(ctx, tx))
	})
}
