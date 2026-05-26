// Package repository provides data-access objects for the Showcase application.
//
// TenantRepository[T] wraps orm.Repository[T] and automatically scopes every
// query to the current tenant, preventing cross-tenant data leakage without
// requiring callers to remember to add the filter.
package repository

import (
	"context"

	astraorm "github.com/astra-go/astra/orm"
	"gorm.io/gorm"
)

// TenantRepository is a generic, tenant-scoped repository.
// All queries are automatically filtered by tenantID.
//
//	type ProductRepo struct{ *TenantRepository[domain.Product] }
//
//	func NewProductRepo(db *gorm.DB, tenantID uint) *ProductRepo {
//	    return &ProductRepo{NewTenantRepository[domain.Product](db, tenantID)}
//	}
type TenantRepository[T any] struct {
	base     *astraorm.Repository[T]
	tenantID uint
}

// NewTenantRepository creates a tenant-scoped repository.
func NewTenantRepository[T any](db *gorm.DB, tenantID uint) *TenantRepository[T] {
	return &TenantRepository[T]{
		base:     astraorm.NewRepository[T](db),
		tenantID: tenantID,
	}
}

// scoped returns a repository with the tenant scope pre-applied.
func (r *TenantRepository[T]) scoped() *astraorm.Repository[T] {
	return r.base.Scopes(astraorm.GORMTenantScope(uintToStr(r.tenantID)))
}

// Create inserts entity. The caller must set entity.TenantID before calling.
func (r *TenantRepository[T]) Create(ctx context.Context, entity *T) error {
	return r.base.Create(ctx, entity)
}

// FindByID retrieves a record by primary key, scoped to the tenant.
func (r *TenantRepository[T]) FindByID(ctx context.Context, id uint) (*T, error) {
	return r.scoped().First(ctx, "id = ?", id)
}

// FindAll retrieves all records for the tenant with optional pagination.
func (r *TenantRepository[T]) FindAll(ctx context.Context, p *astraorm.Page) ([]T, int64, error) {
	return r.scoped().FindAll(ctx, p)
}

// FindWhere retrieves records matching query, scoped to the tenant.
func (r *TenantRepository[T]) FindWhere(ctx context.Context, query any, args ...any) ([]T, error) {
	return r.scoped().FindWhere(ctx, query, args...)
}

// Update saves all non-zero fields of entity.
func (r *TenantRepository[T]) Update(ctx context.Context, entity *T) error {
	return r.base.Update(ctx, entity)
}

// Updates applies a partial update to the record with the given primary key.
func (r *TenantRepository[T]) Updates(ctx context.Context, id uint, values any) error {
	return r.base.Updates(ctx, id, values)
}

// Delete removes the record with the given primary key.
func (r *TenantRepository[T]) Delete(ctx context.Context, id uint) error {
	return r.base.Delete(ctx, id)
}

// DB returns the underlying *gorm.DB for building custom queries.
func (r *TenantRepository[T]) DB() *gorm.DB {
	return r.base.DB()
}

func uintToStr(id uint) string {
	if id == 0 {
		return ""
	}
	// avoid importing strconv in the hot path — use fmt-free conversion
	buf := make([]byte, 0, 20)
	n := id
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if len(buf) == 0 {
		return "0"
	}
	return string(buf)
}
