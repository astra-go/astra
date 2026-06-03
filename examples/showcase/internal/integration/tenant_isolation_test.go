//go:build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/repository"
)

// TestTenantIsolation_ProductCrossAccess_Postgres verifies that tenants cannot access
// each other's products, even with direct ID queries.
func TestTenantIsolation_ProductCrossAccess_Postgres(t *testing.T) {
	ctx := context.Background()

	// Create two separate tenants
	const tenantA uint = 200
	const tenantB uint = 201

	repoA := repository.NewProductRepo(testDB, tenantA)
	repoB := repository.NewProductRepo(testDB, tenantB)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id IN (?, ?)", tenantA, tenantB)
	}()

	// Tenant A creates a product
	productA := &domain.Product{
		TenantID: tenantA,
		Name:     "Tenant A Secret Product",
		Price:    99.99,
		Stock:    100,
	}
	if err := repoA.Create(ctx, productA); err != nil {
		t.Fatalf("tenant A create: %v", err)
	}

	// Tenant B creates a product
	productB := &domain.Product{
		TenantID: tenantB,
		Name:     "Tenant B Secret Product",
		Price:    49.99,
		Stock:    50,
	}
	if err := repoB.Create(ctx, productB); err != nil {
		t.Fatalf("tenant B create: %v", err)
	}

	// Test: Tenant A can read its own product
	gotA, err := repoA.FindByID(ctx, productA.ID)
	if err != nil {
		t.Fatalf("tenant A FindByID: %v", err)
	}
	if gotA.Name != "Tenant A Secret Product" {
		t.Errorf("name mismatch for tenant A")
	}

	// Test: Tenant B CANNOT read tenant A's product (should return error)
	_, err = repoB.FindByID(ctx, productA.ID)
	if err == nil {
		t.Fatal("expected error: tenant B should NOT access tenant A's product")
	}

	// Test: Tenant A CANNOT read tenant B's product
	_, err = repoA.FindByID(ctx, productB.ID)
	if err == nil {
		t.Fatal("expected error: tenant A should NOT access tenant B's product")
	}

	// Test: Tenant B can read its own product
	gotB, err := repoB.FindByID(ctx, productB.ID)
	if err != nil {
		t.Fatalf("tenant B FindByID: %v", err)
	}
	if gotB.Name != "Tenant B Secret Product" {
		t.Errorf("name mismatch for tenant B")
	}
}

// TestTenantIsolation_OrderAccess_Postgres verifies order isolation between tenants.
func TestTenantIsolation_OrderAccess_Postgres(t *testing.T) {
	ctx := context.Background()

	const tenantA uint = 202
	const tenantB uint = 203

	productRepoA := repository.NewProductRepo(testDB, tenantA)
	orderRepoA := repository.NewOrderRepo(testDB, tenantA)
	orderRepoB := repository.NewOrderRepo(testDB, tenantB)

	defer func() {
		testDB.Exec("DELETE FROM orders WHERE tenant_id IN (?, ?)", tenantA, tenantB)
		testDB.Exec("DELETE FROM products WHERE tenant_id IN (?, ?)", tenantA, tenantB)
	}()

	// Tenant A creates product and order
	productA := &domain.Product{
		TenantID: tenantA,
		Name:     "Product for Tenant A",
		Price:    10.0,
		Stock:    10,
	}
	if err := productRepoA.Create(ctx, productA); err != nil {
		t.Fatalf("create product A: %v", err)
	}

	orderA := &domain.Order{
		TenantID: tenantA,
		UserID:   1,
		Total:    10.0,
		Status:   "pending",
	}
	if err := orderRepoA.Create(ctx, orderA); err != nil {
		t.Fatalf("create order A: %v", err)
	}

	// Test: Tenant A can read its own order
	gotA, err := orderRepoA.FindByID(ctx, orderA.ID)
	if err != nil {
		t.Fatalf("tenant A FindByID: %v", err)
	}
	if gotA.Total != 10.0 {
		t.Errorf("total mismatch for tenant A")
	}

	// Test: Tenant B CANNOT read tenant A's order
	_, err = orderRepoB.FindByID(ctx, orderA.ID)
	if err == nil {
		t.Fatal("expected error: tenant B should NOT access tenant A's order")
	}

	// Test: FindByUser with tenant isolation
	ordersA, totalA, err := orderRepoA.FindByUser(ctx, 1, nil)
	if err != nil {
		t.Fatalf("tenant A FindByUser: %v", err)
	}
	if totalA == 0 {
		t.Error("tenant A should see its own orders")
	}

	ordersB, totalB, err := orderRepoB.FindByUser(ctx, 1, nil)
	if err != nil {
		t.Fatalf("tenant B FindByUser: %v", err)
	}
	// Tenant B should see no orders (different tenant)
	if totalB != 0 {
		t.Errorf("tenant B should see 0 orders, got %d", totalB)
	}
	if len(ordersB) != 0 {
		t.Errorf("tenant B should see empty orders, got %d", len(ordersB))
	}
}

// TestTenantIsolation_UserAccess_Postgres verifies user isolation between tenants.
func TestTenantIsolation_UserAccess_Postgres(t *testing.T) {
	ctx := context.Background()

	const tenantA uint = 204
	const tenantB uint = 205

	userRepoA := repository.NewUserRepo(testDB, tenantA)
	userRepoB := repository.NewUserRepo(testDB, tenantB)

	defer func() {
		testDB.Exec("DELETE FROM users WHERE tenant_id IN (?, ?)", tenantA, tenantB)
	}()

	// Tenant A creates a user
	userA := &domain.User{
		TenantID: tenantA,
		Email:    "user-a@tenant-a.com",
		Name:     "User A",
		Role:     "buyer",
	}
	if err := userRepoA.Create(ctx, userA); err != nil {
		t.Fatalf("create user A: %v", err)
	}

	// Test: Tenant A can read its own user
	gotA, err := userRepoA.FindByID(ctx, userA.ID)
	if err != nil {
		t.Fatalf("tenant A FindByID: %v", err)
	}
	if gotA.Email != "user-a@tenant-a.com" {
		t.Errorf("email mismatch for tenant A")
	}

	// Test: Tenant B CANNOT read tenant A's user
	_, err = userRepoB.FindByID(ctx, userA.ID)
	if err == nil {
		t.Fatal("expected error: tenant B should NOT access tenant A's user")
	}

	// Test: FindByEmail with tenant isolation
	gotByEmail, err := userRepoA.FindByEmail(ctx, "user-a@tenant-a.com")
	if err != nil {
		t.Fatalf("tenant A FindByEmail: %v", err)
	}
	if gotByEmail.ID != userA.ID {
		t.Error("tenant A should find user by email")
	}

	// Tenant B should NOT find tenant A's user by email
	_, err = userRepoB.FindByEmail(ctx, "user-a@tenant-a.com")
	if err == nil {
		t.Fatal("expected error: tenant B should NOT find tenant A's user")
	}
}
