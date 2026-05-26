package repository

import (
	"context"

	"github.com/astra-go/astra/examples/showcase/internal/domain"
	astraorm "github.com/astra-go/astra/orm"
	"gorm.io/gorm"
)

// ─── ProductRepo ──────────────────────────────────────────────────────────────

type ProductRepo struct {
	*TenantRepository[domain.Product]
}

func NewProductRepo(db *gorm.DB, tenantID uint) *ProductRepo {
	return &ProductRepo{NewTenantRepository[domain.Product](db, tenantID)}
}

// DecrStock atomically decrements stock by qty for the given product.
// Returns gorm.ErrRecordNotFound when the product doesn't exist or has
// insufficient stock — the caller should treat this as a 409 Conflict.
func (r *ProductRepo) DecrStock(ctx context.Context, productID uint, qty int) error {
	db := astraorm.FromCtx(ctx, r.DB())
	result := db.Model(&domain.Product{}).
		Where("id = ? AND stock >= ? AND deleted_at IS NULL", productID, qty).
		Update("stock", gorm.Expr("stock - ?", qty))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound // insufficient stock or not found
	}
	return nil
}

// FindLowStock returns products whose stock is at or below threshold.
func (r *ProductRepo) FindLowStock(ctx context.Context, threshold int) ([]domain.Product, error) {
	return r.FindWhere(ctx, "stock <= ?", threshold)
}

// ─── OrderRepo ────────────────────────────────────────────────────────────────

type OrderRepo struct {
	*TenantRepository[domain.Order]
}

func NewOrderRepo(db *gorm.DB, tenantID uint) *OrderRepo {
	return &OrderRepo{NewTenantRepository[domain.Order](db, tenantID)}
}

// FindWithItems retrieves an order with its items, products, and user preloaded.
func (r *OrderRepo) FindWithItems(ctx context.Context, orderID uint) (*domain.Order, error) {
	var order domain.Order
	db := astraorm.FromCtx(ctx, r.DB())
	err := db.
		Preload("User").
		Preload("Items.Product").
		Where("id = ? AND tenant_id = ?", orderID, r.tenantID).
		First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// FindByUser retrieves paginated orders for a specific user within the tenant.
func (r *OrderRepo) FindByUser(ctx context.Context, userID uint, p *astraorm.Page) ([]domain.Order, int64, error) {
	var orders []domain.Order
	var total int64
	db := astraorm.FromCtx(ctx, r.DB())
	q := db.Model(&domain.Order{}).Where("user_id = ? AND tenant_id = ?", userID, r.tenantID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if p != nil {
		q = q.Scopes(astraorm.Paginate(*p))
	}
	if err := q.Preload("User").Preload("Items.Product").Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// ─── UserRepo ─────────────────────────────────────────────────────────────────

type UserRepo struct {
	*TenantRepository[domain.User]
}

func NewUserRepo(db *gorm.DB, tenantID uint) *UserRepo {
	return &UserRepo{NewTenantRepository[domain.User](db, tenantID)}
}

// FindByEmail returns the user with the given email within the tenant.
func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.scoped().First(ctx, "email = ?", email)
}

// FindByOAuth returns the user identified by the OAuth provider + subject pair.
func (r *UserRepo) FindByOAuth(ctx context.Context, provider, sub string) (*domain.User, error) {
	return r.scoped().First(ctx, "oauth_provider = ? AND oauth_sub = ?", provider, sub)
}

// ─── TenantRepo ───────────────────────────────────────────────────────────────

type TenantRepo struct {
	base *astraorm.Repository[domain.Tenant]
}

func NewTenantRepo(db *gorm.DB) *TenantRepo {
	return &TenantRepo{base: astraorm.NewRepository[domain.Tenant](db)}
}

func (r *TenantRepo) FindByID(ctx context.Context, id uint) (*domain.Tenant, error) {
	return r.base.FindByID(ctx, id)
}

func (r *TenantRepo) FindByName(ctx context.Context, name string) (*domain.Tenant, error) {
	return r.base.First(ctx, "name = ?", name)
}

func (r *TenantRepo) Create(ctx context.Context, t *domain.Tenant) error {
	return r.base.Create(ctx, t)
}
