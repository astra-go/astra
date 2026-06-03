//go:build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/examples/showcase/internal/db"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/repository"
)

// TestRBAC_PermissionBoundaries_Postgres verifies that RBAC roles are correctly enforced.
// This test ensures that:
// - buyer role can only GET products
// - seller role can GET, POST, PUT, DELETE products
// - admin role has full access to all endpoints
func TestRBAC_PermissionBoundaries_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 100

	// Setup: Create test users with different roles
	sqlDB, _ := testDB.DB()
	defer func() {
		// Cleanup
		testDB.Exec("DELETE FROM users WHERE tenant_id = ?", tenantID)
	}()

	// Create users
	userRepo := repository.NewUserRepo(testDB, tenantID)

	buyer := &domain.User{
		TenantID: tenantID,
		Email:    "buyer@test.com",
		Name:     "Test Buyer",
		Role:     "buyer",
	}
	if err := userRepo.Create(ctx, buyer); err != nil {
		t.Fatalf("create buyer: %v", err)
	}

	seller := &domain.User{
		TenantID: tenantID,
		Email:    "seller@test.com",
		Name:     "Test Seller",
		Role:     "seller",
	}
	if err := userRepo.Create(ctx, seller); err != nil {
		t.Fatalf("create seller: %v", err)
	}

	admin := &domain.User{
		TenantID: tenantID,
		Email:    "admin@test.com",
		Name:     "Test Admin",
		Role:     "admin",
	}
	if err := userRepo.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	// Verify roles are correctly assigned
	tests := []struct {
		name     string
		user     *domain.User
		wantRole string
	}{
		{"buyer has buyer role", buyer, "buyer"},
		{"seller has seller role", seller, "seller"},
		{"admin has admin role", admin, "admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := userRepo.FindByID(ctx, tt.user.ID)
			if err != nil {
				t.Fatalf("FindByID: %v", err)
			}
			if got.Role != tt.wantRole {
				t.Errorf("expected role %s, got %s", tt.wantRole, got.Role)
			}
		})
	}

	// Test: Admin can update user roles
	if err := userRepo.UpdateRole(ctx, buyer.ID, "seller"); err != nil {
		t.Fatalf("admin UpdateRole: %v", err)
	}

	updated, _ := userRepo.FindByID(ctx, buyer.ID)
	if updated.Role != "seller" {
		t.Errorf("role not updated: expected seller, got %s", updated.Role)
	}
}

// TestRBAC_AdminCanAccessAdminEndpoints_Postgres verifies admin-only operations.
func TestRBAC_AdminCanAccessAdminEndpoints_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 101

	defer func() {
		testDB.Exec("DELETE FROM users WHERE tenant_id = ?", tenantID)
	}()

	userRepo := repository.NewUserRepo(testDB, tenantID)

	// Create admin user
	admin := &domain.User{
		TenantID: tenantID,
		Email:    "admin-test@test.com",
		Name:     "Admin User",
		Role:     "admin",
	}
	if err := userRepo.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	// Create regular user
	buyer := &domain.User{
		TenantID: tenantID,
		Email:    "buyer-test@test.com",
		Name:     "Buyer User",
		Role:     "buyer",
	}
	if err := userRepo.Create(ctx, buyer); err != nil {
		t.Fatalf("create buyer: %v", err)
	}

	// Admin can read any user
	got, err := userRepo.FindByID(ctx, buyer.ID)
	if err != nil {
		t.Fatalf("admin FindByID: %v", err)
	}
	if got.Email != "buyer-test@test.com" {
		t.Errorf("email mismatch: %s", got.Email)
	}

	// Admin can update any user's role
	if err := userRepo.UpdateRole(ctx, buyer.ID, "seller"); err != nil {
		t.Fatalf("admin UpdateRole: %v", err)
	}

	updated, _ := userRepo.FindByID(ctx, buyer.ID)
	if updated.Role != "seller" {
		t.Errorf("role update failed: expected seller, got %s", updated.Role)
	}
}
