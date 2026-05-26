//go:build integration

package integration_test

// Testcontainers-based integration tests for the Showcase application.
//
// These tests spin up real Postgres and Redis containers and exercise the full
// repository + service stack against them.  No mocks, no SQLite.
//
// Prerequisites: Docker (or a compatible runtime) must be available.
//
// Run:
//
//	go test -tags integration -v ./internal/integration/...
//
// The containers are started once in TestMain and torn down after all tests.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/astra-go/astra/examples/showcase/internal/db"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/repository"
	"github.com/astra-go/astra/examples/showcase/internal/service"
	astraorm "github.com/astra-go/astra/orm"
	"github.com/docker/go-connections/nat"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

var testDB *gorm.DB

// TestMain starts a Postgres container once for the entire test binary.
func TestMain(m *testing.M) {
	ctx := context.Background()

	pgC, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: tc.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "showcase",
				"POSTGRES_PASSWORD": "showcase",
				"POSTGRES_DB":       "showcase_test",
			},
			WaitingFor: wait.ForSQL("5432/tcp", "pgx", func(host string, port nat.Port) string {
				return fmt.Sprintf("postgres://showcase:showcase@%s:%s/showcase_test?sslmode=disable", host, port.Port())
			}).WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcontainers: start postgres: %v\n", err)
		os.Exit(1)
	}
	defer pgC.Terminate(ctx) //nolint:errcheck

	host, _ := pgC.Host(ctx)
	port, _ := pgC.MappedPort(ctx, "5432/tcp")
	dsn := fmt.Sprintf("postgres://showcase:showcase@%s:%s/showcase_test?sslmode=disable", host, port.Port())

	testDB, err = db.Open(db.Config{DSN: dsn, MaxOpen: 5, MaxIdle: 2, MaxLifetime: time.Minute})
	if err != nil {
		fmt.Fprintf(os.Stderr, "db open: %v\n", err)
		os.Exit(1)
	}
	if err := db.Migrate(testDB); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// ─── Repository integration tests ────────────────────────────────────────────

func TestProductRepo_CRUD_Postgres(t *testing.T) {
	repo := repository.NewProductRepo(testDB, 1)
	ctx := context.Background()

	// Create
	p := &domain.Product{TenantID: 1, Name: "Postgres Widget", Price: 19.99, Stock: 50}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}

	// FindByID
	got, err := repo.FindByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Name != "Postgres Widget" {
		t.Fatalf("name mismatch: %s", got.Name)
	}

	// Updates
	if err := repo.Updates(ctx, p.ID, map[string]any{"stock": 45}); err != nil {
		t.Fatalf("Updates: %v", err)
	}

	// DecrStock
	if err := repo.DecrStock(ctx, p.ID, 5); err != nil {
		t.Fatalf("DecrStock: %v", err)
	}
	got, _ = repo.FindByID(ctx, p.ID)
	if got.Stock != 40 {
		t.Fatalf("expected stock 40, got %d", got.Stock)
	}

	// Delete
	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, p.ID); err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestProductRepo_TenantIsolation_Postgres(t *testing.T) {
	ctx := context.Background()
	repo1 := repository.NewProductRepo(testDB, 10)
	repo2 := repository.NewProductRepo(testDB, 20)

	p := &domain.Product{TenantID: 10, Name: "Tenant10 Product", Price: 5.0, Stock: 1}
	if err := repo1.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Tenant 20 must not see tenant 10's product.
	_, err := repo2.FindByID(ctx, p.ID)
	if err == nil {
		t.Fatal("expected error: tenant 20 should not see tenant 10's product")
	}
}

func TestProductRepo_FindLowStock_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 30
	repo := repository.NewProductRepo(testDB, tenantID)

	for _, s := range []int{2, 5, 100} {
		p := &domain.Product{TenantID: tenantID, Name: fmt.Sprintf("stock-%d", s), Price: 1.0, Stock: s}
		_ = repo.Create(ctx, p)
	}

	low, err := repo.FindLowStock(ctx, 5)
	if err != nil {
		t.Fatalf("FindLowStock: %v", err)
	}
	if len(low) != 2 {
		t.Fatalf("expected 2 low-stock items, got %d", len(low))
	}
}

// ─── Service integration tests ────────────────────────────────────────────────

func TestOrderSvc_Create_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 40

	productRepo := repository.NewProductRepo(testDB, tenantID)
	orderRepo := repository.NewOrderRepo(testDB, tenantID)
	itemRepo := astraorm.NewRepository[domain.OrderItem](testDB)

	// Seed a product.
	p := &domain.Product{TenantID: tenantID, Name: "Integration Widget", Price: 10.0, Stock: 20}
	if err := productRepo.Create(ctx, p); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	svc := service.NewOrderSvc(orderRepo, itemRepo, productRepo, nil)
	order, err := svc.Create(ctx, tenantID, 1, domain.CreateOrderReq{
		Items: []domain.OrderItemReq{{ProductID: p.ID, Qty: 3}},
	})
	if err != nil {
		t.Fatalf("Create order: %v", err)
	}
	if order.Total != 30.0 {
		t.Fatalf("expected total 30.0, got %f", order.Total)
	}

	// Stock must have been decremented.
	updated, _ := productRepo.FindByID(ctx, p.ID)
	if updated.Stock != 17 {
		t.Fatalf("expected stock 17, got %d", updated.Stock)
	}
}

func TestOrderSvc_Create_CanaryDiscount_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 41

	productRepo := repository.NewProductRepo(testDB, tenantID)
	orderRepo := repository.NewOrderRepo(testDB, tenantID)
	itemRepo := astraorm.NewRepository[domain.OrderItem](testDB)

	p := &domain.Product{TenantID: tenantID, Name: "Canary Widget", Price: 100.0, Stock: 10}
	_ = productRepo.Create(ctx, p)

	svc := service.NewOrderSvc(orderRepo, itemRepo, productRepo, nil)
	order, err := svc.Create(ctx, tenantID, 1, domain.CreateOrderReq{
		Items:         []domain.OrderItemReq{{ProductID: p.ID, Qty: 1}},
		CanaryVersion: "v2",
	})
	if err != nil {
		t.Fatalf("Create canary order: %v", err)
	}
	// 5 % discount: 100 * 0.95 = 95
	if order.Total != 95.0 {
		t.Fatalf("expected total 95.0 (5%% discount), got %f", order.Total)
	}
}

func TestOrderSvc_Create_FindAll_Pagination_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 42

	productRepo := repository.NewProductRepo(testDB, tenantID)
	orderRepo := repository.NewOrderRepo(testDB, tenantID)
	itemRepo := astraorm.NewRepository[domain.OrderItem](testDB)

	p := &domain.Product{TenantID: tenantID, Name: "Paged Widget", Price: 5.0, Stock: 100}
	_ = productRepo.Create(ctx, p)

	svc := service.NewOrderSvc(orderRepo, itemRepo, productRepo, nil)
	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, tenantID, 1, domain.CreateOrderReq{
			Items: []domain.OrderItemReq{{ProductID: p.ID, Qty: 1}},
		})
		if err != nil {
			t.Fatalf("create order %d: %v", i, err)
		}
	}

	orders, total, err := orderRepo.FindByUser(ctx, 1, &astraorm.Page{PageNum: 1, PageSize: 3})
	if err != nil {
		t.Fatalf("FindByUser: %v", err)
	}
	if total < 5 {
		t.Fatalf("expected at least 5 orders, got %d", total)
	}
	if len(orders) != 3 {
		t.Fatalf("expected page size 3, got %d", len(orders))
	}
}
