//go:build ignore

// tenant_contract_check.go verifies TenantRepository[T] satisfies
// contract.Repository[T] at compile time.
// If a method signature changes, this file will fail to compile with a clear error.
// This file is never compiled in normal builds (//go:build ignore above).

package orm

import (
	"context"

	"github.com/astra-go/astra/contract"
)

func _tenantRepoContractCheck[T any]() {
	var r *TenantRepository[T]
	_ = interface {
		Create(context.Context, *T) error
		FindByID(context.Context, any) (*T, error)
		FindAll(context.Context, *Page) ([]T, int64, error)
		FindWhere(context.Context, any, ...any) ([]T, error)
		First(context.Context, any, ...any) (*T, error)
		Count(context.Context, any, ...any) (int64, error)
		Update(context.Context, *T) error
		Updates(context.Context, any, any) error
		Delete(context.Context, any) error
	}(r)
	var _ contract.Repository[T] = r
}
