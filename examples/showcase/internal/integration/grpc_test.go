//go:build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/grpc"
	"github.com/astra-go/astra/examples/showcase/internal/repository"
	pb "github.com/astra-go/astra/examples/showcase/internal/pb"
)

// TestGRPC_GetStock_Postgres verifies the gRPC GetStock RPC method.
func TestGRPC_GetStock_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 500

	productRepo := repository.NewProductRepo(testDB, tenantID)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create a product
	product := &domain.Product{
		TenantID: tenantID,
		Name:     "gRPC Test Widget",
		Price:    25.0,
		Stock:    75,
	}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("create product: %v", err)
	}

	// Create gRPC service
	inventorySvc := grpc.NewInventoryService(productRepo)

	// Test: GetStock RPC
	req := &pb.GetStockRequest{
		TenantId:  uint32(tenantID),
		ProductId: uint32(product.ID),
	}

	resp, err := inventorySvc.GetStock(ctx, req)
	if err != nil {
		t.Fatalf("GetStock: %v", err)
	}

	if resp.ProductId != uint32(product.ID) {
		t.Errorf("product_id mismatch: expected %d, got %d", product.ID, resp.ProductId)
	}

	if resp.Stock != 75 {
		t.Errorf("stock mismatch: expected 75, got %d", resp.Stock)
	}

	if resp.Available != true {
		t.Error("expected available=true")
	}
}

// TestGRPC_GetStock_NotFound_Postgres verifies error handling for non-existent products.
func TestGRPC_GetStock_NotFound_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 501

	productRepo := repository.NewProductRepo(testDB, tenantID)
	inventorySvc := grpc.NewInventoryService(productRepo)

	// Test: GetStock for non-existent product
	req := &pb.GetStockRequest{
		TenantId:  uint32(tenantID),
		ProductId: 99999, // Non-existent
	}

	_, err := inventorySvc.GetStock(ctx, req)
	if err == nil {
		t.Fatal("expected error for non-existent product")
	}
}

// TestGRPC_BatchGetStock_Postgres verifies the BatchGetStock RPC method.
func TestGRPC_BatchGetStock_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 502

	productRepo := repository.NewProductRepo(testDB, tenantID)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create multiple products
	productIDs := []uint32{}
	for i := 1; i <= 5; i++ {
		p := &domain.Product{
			TenantID: tenantID,
			Name:     "Batch Product " + string(rune('A'+i-1)),
			Price:    float64(i * 10),
			Stock:    i * 10,
		}
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("create product %d: %v", i, err)
		}
		productIDs = append(productIDs, uint32(p.ID))
	}

	// Create gRPC service
	inventorySvc := grpc.NewInventoryService(productRepo)

	// Test: BatchGetStock RPC
	req := &pb.BatchGetStockRequest{
		TenantId:   uint32(tenantID),
		ProductIds: productIDs,
	}

	resp, err := inventorySvc.BatchGetStock(ctx, req)
	if err != nil {
		t.Fatalf("BatchGetStock: %v", err)
	}

	if len(resp.Items) != 5 {
		t.Errorf("expected 5 items, got %d", len(resp.Items))
	}

	// Verify each item
	for i, item := range resp.Items {
		if item.ProductId != productIDs[i] {
			t.Errorf("item %d: product_id mismatch", i)
		}
		expectedStock := int32((i + 1) * 10)
		if item.Stock != expectedStock {
			t.Errorf("item %d: expected stock %d, got %d", i, expectedStock, item.Stock)
		}
	}
}

// TestGRPC_DecrStock_Postgres verifies the DecrStock RPC method.
func TestGRPC_DecrStock_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 503

	productRepo := repository.NewProductRepo(testDB, tenantID)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create a product
	product := &domain.Product{
		TenantID: tenantID,
		Name:     "Decr Test Widget",
		Price:    15.0,
		Stock:    100,
	}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("create product: %v", err)
	}

	// Create gRPC service
	inventorySvc := grpc.NewInventoryService(productRepo)

	// Test: DecrStock RPC
	req := &pb.DecrStockRequest{
		TenantId:  uint32(tenantID),
		ProductId: uint32(product.ID),
		Quantity:  25,
	}

	resp, err := inventorySvc.DecrStock(ctx, req)
	if err != nil {
		t.Fatalf("DecrStock: %v", err)
	}

	if resp.Stock != 75 {
		t.Errorf("expected stock 75 after decrement, got %d", resp.Stock)
	}

	// Verify in database
	updated, _ := productRepo.FindByID(ctx, product.ID)
	if updated.Stock != 75 {
		t.Errorf("DB stock mismatch: expected 75, got %d", updated.Stock)
	}
}

// TestGRPC_DecrStock_InsufficientStock_Postgres verifies error handling
// when trying to decrement more than available stock.
func TestGRPC_DecrStock_InsufficientStock_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 504

	productRepo := repository.NewProductRepo(testDB, tenantID)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create a product with limited stock
	product := &domain.Product{
		TenantID: tenantID,
		Name:     "Limited Stock Widget",
		Price:    10.0,
		Stock:    5,
	}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("create product: %v", err)
	}

	// Create gRPC service
	inventorySvc := grpc.NewInventoryService(productRepo)

	// Test: Try to decrement more than available
	req := &pb.DecrStockRequest{
		TenantId:  uint32(tenantID),
		ProductId: uint32(product.ID),
		Quantity:  10, // More than available (5)
	}

	_, err := inventorySvc.DecrStock(ctx, req)
	if err == nil {
		t.Fatal("expected error for insufficient stock")
	}

	// Verify stock unchanged
	unchanged, _ := productRepo.FindByID(ctx, product.ID)
	if unchanged.Stock != 5 {
		t.Errorf("stock should remain 5, got %d", unchanged.Stock)
	}
}

// TestGRPC_ListLowStock_Postgres verifies the server-streaming ListLowStock RPC.
func TestGRPC_ListLowStock_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 505

	productRepo := repository.NewProductRepo(testDB, tenantID)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create products with varying stock levels
	stockLevels := []int{2, 5, 8, 15, 25, 50}
	for i, stock := range stockLevels {
		p := &domain.Product{
			TenantID: tenantID,
			Name:     "Stock Level " + string(rune('A'+i)),
			Price:    1.0,
			Stock:    stock,
		}
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("create product %d: %v", i, err)
		}
	}

	// Create gRPC service
	inventorySvc := grpc.NewInventoryService(productRepo)

	// Test: ListLowStock with threshold 10
	req := &pb.ListLowStockRequest{
		TenantId:  uint32(tenantID),
		Threshold: 10,
	}

	// Mock stream server for testing
	mockStream := &mockListLowStockServer{ctx: ctx, items: []*pb.StockItem{}}

	err := inventorySvc.ListLowStock(req, mockStream)
	if err != nil {
		t.Fatalf("ListLowStock: %v", err)
	}

	// Verify: Should return 3 items (stock 2, 5, 8)
	if len(mockStream.items) != 3 {
		t.Errorf("expected 3 low-stock items, got %d", len(mockStream.items))
	}

	// Verify stock values
	expectedStocks := []int32{2, 5, 8}
	for i, item := range mockStream.items {
		if item.Stock != expectedStocks[i] {
			t.Errorf("item %d: expected stock %d, got %d", i, expectedStocks[i], item.Stock)
		}
	}
}

// mockListLowStockServer is a mock implementation of the gRPC stream server
// for testing server-streaming RPCs.
type mockListLowStockServer struct {
	pb.InventoryService_ListLowStockServer
	ctx   context.Context
	items []*pb.StockItem
}

func (m *mockListLowStockServer) Send(item *pb.StockItem) error {
	m.items = append(m.items, item)
	return nil
}

func (m *mockListLowStockServer) Context() context.Context {
	return m.ctx
}

// TestGRPC_TenantIsolation_Postgres verifies that gRPC methods respect tenant isolation.
func TestGRPC_TenantIsolation_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantA uint = 506
	const tenantB uint = 507

	productRepoA := repository.NewProductRepo(testDB, tenantA)
	productRepoB := repository.NewProductRepo(testDB, tenantB)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id IN (?, ?)", tenantA, tenantB)
	}()

	// Tenant A creates a product
	productA := &domain.Product{
		TenantID: tenantA,
		Name:     "Tenant A Product",
		Price:    50.0,
		Stock:    100,
	}
	if err := productRepoA.Create(ctx, productA); err != nil {
		t.Fatalf("create tenant A product: %v", err)
	}

	// Create gRPC service for tenant B
	inventorySvcB := grpc.NewInventoryService(productRepoB)

	// Test: Tenant B tries to access tenant A's product
	req := &pb.GetStockRequest{
		TenantId:  uint32(tenantB), // Tenant B context
		ProductId: uint32(productA.ID),
	}

	_, err := inventorySvcB.GetStock(ctx, req)
	if err == nil {
		t.Fatal("expected error: tenant B should NOT access tenant A's product")
	}
}
