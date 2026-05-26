package repository_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/repository"
	astraorm "github.com/astra-go/astra/orm"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// openTestDB opens an in-memory SQLite database and migrates the schema.
// SQLite is used here to keep tests fast and dependency-free.
// Integration tests against real Postgres are in *_integration_test.go.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.Tenant{},
		&domain.User{},
		&domain.Product{},
		&domain.Order{},
		&domain.OrderItem{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedTenant inserts a tenant and returns its ID.
func seedTenant(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	tenant := &domain.Tenant{Name: "test-tenant", Plan: domain.PlanFree}
	if err := db.Create(tenant).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return tenant.ID
}

// ─── TenantRepository ─────────────────────────────────────────────────────────

func TestTenantRepository_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	tenantID := seedTenant(t, db)
	repo := repository.NewProductRepo(db, tenantID)
	ctx := context.Background()

	product := &domain.Product{
		TenantID: tenantID,
		Name:     "Widget",
		Price:    9.99,
		Stock:    10,
		Category: "widgets",
	}
	if err := repo.Create(ctx, product); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if product.ID == 0 {
		t.Fatal("expected non-zero ID after Create")
	}

	got, err := repo.FindByID(ctx, product.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Name != product.Name {
		t.Errorf("Name: got %q, want %q", got.Name, product.Name)
	}
}

func TestTenantRepository_TenantIsolation(t *testing.T) {
	db := openTestDB(t)
	tenantA := seedTenant(t, db)
	tenantB := &domain.Tenant{Name: "tenant-b", Plan: domain.PlanFree}
	db.Create(tenantB)

	repoA := repository.NewProductRepo(db, tenantA)
	repoB := repository.NewProductRepo(db, tenantB.ID)
	ctx := context.Background()

	// Insert a product under tenant A
	p := &domain.Product{TenantID: tenantA, Name: "A-only", Price: 1, Stock: 1}
	if err := repoA.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Tenant B should not see tenant A's product
	products, _, err := repoB.FindAll(ctx, nil)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(products) != 0 {
		t.Errorf("tenant isolation broken: tenant B sees %d products from tenant A", len(products))
	}
}

func TestTenantRepository_FindAll_Pagination(t *testing.T) {
	db := openTestDB(t)
	tenantID := seedTenant(t, db)
	repo := repository.NewProductRepo(db, tenantID)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		p := &domain.Product{TenantID: tenantID, Name: "p", Price: 1, Stock: 1}
		db.Create(p)
	}

	page := &astraorm.Page{PageNum: 1, PageSize: 2, Offset: 0}
	products, total, err := repo.FindAll(ctx, page)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if total != 5 {
		t.Errorf("total: got %d, want 5", total)
	}
	if len(products) != 2 {
		t.Errorf("page size: got %d, want 2", len(products))
	}
}

func TestTenantRepository_Update(t *testing.T) {
	db := openTestDB(t)
	tenantID := seedTenant(t, db)
	repo := repository.NewProductRepo(db, tenantID)
	ctx := context.Background()

	p := &domain.Product{TenantID: tenantID, Name: "old", Price: 1, Stock: 5}
	db.Create(p)

	if err := repo.Updates(ctx, p.ID, map[string]any{"name": "new", "price": 2.5}); err != nil {
		t.Fatalf("Updates: %v", err)
	}

	got, _ := repo.FindByID(ctx, p.ID)
	if got.Name != "new" {
		t.Errorf("Name: got %q, want %q", got.Name, "new")
	}
	if got.Price != 2.5 {
		t.Errorf("Price: got %v, want 2.5", got.Price)
	}
}

func TestProductRepo_DecrStock(t *testing.T) {
	db := openTestDB(t)
	tenantID := seedTenant(t, db)
	repo := repository.NewProductRepo(db, tenantID)
	ctx := context.Background()

	p := &domain.Product{TenantID: tenantID, Name: "item", Price: 1, Stock: 10}
	db.Create(p)

	// Normal decrement
	if err := repo.DecrStock(ctx, p.ID, 3); err != nil {
		t.Fatalf("DecrStock: %v", err)
	}
	got, _ := repo.FindByID(ctx, p.ID)
	if got.Stock != 7 {
		t.Errorf("Stock: got %d, want 7", got.Stock)
	}

	// Insufficient stock should fail
	if err := repo.DecrStock(ctx, p.ID, 100); err == nil {
		t.Error("expected error for insufficient stock, got nil")
	}
}

func TestOrderRepo_FindByUser(t *testing.T) {
	db := openTestDB(t)
	tenantID := seedTenant(t, db)
	ctx := context.Background()

	user := &domain.User{TenantID: tenantID, Email: "u@test.com", Name: "U", Role: domain.RoleBuyer}
	db.Create(user)

	for i := 0; i < 3; i++ {
		o := &domain.Order{TenantID: tenantID, UserID: user.ID, Total: 10, Status: domain.OrderStatusPending}
		db.Create(o)
	}

	repo := repository.NewOrderRepo(db, tenantID)
	orders, total, err := repo.FindByUser(ctx, user.ID, nil)
	if err != nil {
		t.Fatalf("FindByUser: %v", err)
	}
	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}
	if len(orders) != 3 {
		t.Errorf("len: got %d, want 3", len(orders))
	}
}
