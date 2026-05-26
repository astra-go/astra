// Package contract — orm.go defines the generic data-access abstractions that
// decouple business and service layers from any concrete ORM library.
//
// Business code should depend only on Repository[T] and TxRunner; it must
// not import gorm.io/gorm or any other ORM package directly.  This keeps
// the domain layer unit-testable with a lightweight in-memory mock and
// makes the underlying storage implementation swappable without touching
// the service layer.
//
// # Typical usage
//
//	type UserService struct {
//	    repo contract.Repository[User]
//	    tx   contract.TxRunner
//	}
//
//	func (s *UserService) Transfer(ctx context.Context, from, to int64, amount float64) error {
//	    return s.tx.RunTx(ctx, func(txCtx context.Context) error {
//	        sender, _ := s.repo.FindByID(txCtx, from)
//	        receiver, _ := s.repo.FindByID(txCtx, to)
//	        sender.Balance -= amount
//	        receiver.Balance += amount
//	        _ = s.repo.Update(txCtx, sender)
//	        return s.repo.Update(txCtx, receiver)
//	    })
//	}
//
// # Unit testing with a mock
//
//	func TestTransfer(t *testing.T) {
//	    svc := &UserService{
//	        repo: &MockUserRepo{users: map[int64]*User{...}},
//	        tx:   &MockTxRunner{},
//	    }
//	    err := svc.Transfer(context.Background(), 1, 2, 100)
//	    if err != nil { t.Fatal(err) }
//	}
package contract

import "context"

// Page holds pagination parameters extracted from an HTTP request.
// It is a value type; copy freely.
//
// orm.ParsePage parses Page from query parameters;
// orm.Paginate converts it to a GORM scope.
type Page struct {
	// PageNum is the 1-based page index.
	PageNum int
	// PageSize is the number of items per page.
	PageSize int
	// Offset is the pre-computed SQL offset (= (PageNum-1) * PageSize).
	Offset int
}

// Repository[T] is the generic data-access contract.
//
// All methods accept a context.Context as the first argument so that:
//   - Active transactions propagated by TxRunner.RunTx are picked up
//     automatically via orm.FromCtx — no explicit tx plumbing required.
//   - OTel trace spans from the request context flow into query spans.
//
// Implementations (e.g. *orm.Repository[T]) satisfy this interface.
// Tests can substitute a plain struct mock without starting a database.
type Repository[T any] interface {
	// Create inserts entity and returns the first error encountered.
	Create(ctx context.Context, entity *T) error

	// FindByID retrieves a record by primary key.
	// Returns an error wrapping gorm.ErrRecordNotFound when the record
	// does not exist.
	FindByID(ctx context.Context, id any) (*T, error)

	// FindAll retrieves records with optional pagination.
	// Pass p to apply LIMIT/OFFSET; pass nil to fetch all rows (use with
	// care on large tables). Returns (records, totalCount, error).
	FindAll(ctx context.Context, p *Page) ([]T, int64, error)

	// FindWhere retrieves records matching query.
	// query may be a string ("status = ?"), a map, or a struct.
	FindWhere(ctx context.Context, query any, args ...any) ([]T, error)

	// First returns the first record matching query ordered by primary key.
	// Returns an error wrapping gorm.ErrRecordNotFound when no row matches.
	First(ctx context.Context, query any, args ...any) (*T, error)

	// Count returns the number of records matching query.
	Count(ctx context.Context, query any, args ...any) (int64, error)

	// Update saves all non-zero fields of entity (full-model save semantics).
	Update(ctx context.Context, entity *T) error

	// Updates applies a partial update to the record identified by id.
	// values can be a map[string]any or a struct (only non-zero fields updated).
	Updates(ctx context.Context, id any, values any) error

	// Delete removes the record with the given primary key.
	Delete(ctx context.Context, id any) error
}

// TxRunner executes fn inside a database transaction.
//
// fn receives txCtx — a derived context that carries the active transaction.
// Nested service calls using orm.FromCtx(txCtx, db) participate in the same
// transaction automatically without any explicit tx plumbing.
//
// The transaction is committed when fn returns nil and rolled back on any
// error or panic (the panic is re-propagated after rollback).
//
//	err := txRunner.RunTx(ctx, func(txCtx context.Context) error {
//	    if err := userRepo.Create(txCtx, &user); err != nil {
//	        return err // triggers rollback
//	    }
//	    return orderRepo.Create(txCtx, &order)
//	})
type TxRunner interface {
	RunTx(ctx context.Context, fn func(txCtx context.Context) error) error
}
