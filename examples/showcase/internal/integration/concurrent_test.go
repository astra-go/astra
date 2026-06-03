//go:build integration

package integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/repository"
	"github.com/astra-go/astra/examples/showcase/internal/service"
	astraorm "github.com/astra-go/astra/orm"
)

// TestConcurrentStockDecrement_Postgres verifies that concurrent order creation
// correctly handles stock depletion without overselling.
// This test simulates a high-concurrency scenario where multiple users try to
// purchase the last few items in stock.
func TestConcurrentStockDecrement_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 300
	const initialStock = 100
	const numOrders = 50
	const qtyPerOrder = 2

	productRepo := repository.NewProductRepo(testDB, tenantID)
	orderRepo := repository.NewOrderRepo(testDB, tenantID)
	itemRepo := astraorm.NewRepository[domain.OrderItem](testDB)

	defer func() {
		testDB.Exec("DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE tenant_id = ?)", tenantID)
		testDB.Exec("DELETE FROM orders WHERE tenant_id = ?", tenantID)
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create a product with limited stock
	product := &domain.Product{
		TenantID: tenantID,
		Name:     "High Demand Widget",
		Price:    10.0,
		Stock:    initialStock,
	}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	svc := service.NewOrderSvc(orderRepo, itemRepo, productRepo, nil)

	// Test: Create 50 orders concurrently, each ordering 2 items
	// Expected: All 50 orders should succeed (100 items total)
	var wg sync.WaitGroup
	successChan := make(chan bool, numOrders)
	errorChan := make(chan error, numOrders)

	for i := 0; i < numOrders; i++ {
		wg.Add(1)
		go func(orderNum int) {
			defer wg.Done()

			// Use transaction to ensure atomicity
			tx := testDB.Begin()
			txCtx := context.WithValue(ctx, "tx", tx)

			orderReq := domain.CreateOrderReq{
				Items: []domain.OrderItemReq{{
					ProductID: product.ID,
					Qty:       qtyPerOrder,
				}},
			}

			_, err := svc.Create(txCtx, tenantID, uint(orderNum+1), orderReq)
			if err != nil {
				tx.Rollback()
				errorChan <- err
			} else {
				tx.Commit()
				successChan <- true
			}
		}(i)
	}

	wg.Wait()
	close(successChan)
	close(errorChan)

	// Collect results
	successCount := 0
	for range successChan {
		successCount++
	}

	errors := []error{}
	for err := range errorChan {
		errors = append(errors, err)
	}

	t.Logf("Success: %d orders, Failures: %d orders", successCount, len(errors))

	// Verify: All 50 orders should succeed
	if successCount != numOrders {
		t.Errorf("expected %d successful orders, got %d", numOrders, successCount)
	}

	// Verify: Final stock should be 0 (100 - 50*2 = 0)
	finalProduct, err := productRepo.FindByID(ctx, product.ID)
	if err != nil {
		t.Fatalf("fetch final product: %v", err)
	}

	expectedStock := initialStock - (numOrders * qtyPerOrder)
	if finalProduct.Stock != expectedStock {
		t.Errorf("expected final stock %d, got %d", expectedStock, finalStock.Stock)
	}

	// Verify: No overselling occurred
	if finalProduct.Stock < 0 {
		t.Errorf("CRITICAL: stock went negative (%d) — overselling detected!", finalStock.Stock)
	}
}

// TestConcurrentStockDecrement_InsufficientStock_Postgres verifies that orders
// fail gracefully when stock is depleted during concurrent access.
func TestConcurrentStockDecrement_InsufficientStock_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 301
	const initialStock = 10
	const numOrders = 20
	const qtyPerOrder = 1

	productRepo := repository.NewProductRepo(testDB, tenantID)
	orderRepo := repository.NewOrderRepo(testDB, tenantID)
	itemRepo := astraorm.NewRepository[domain.OrderItem](testDB)

	defer func() {
		testDB.Exec("DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE tenant_id = ?)", tenantID)
		testDB.Exec("DELETE FROM orders WHERE tenant_id = ?", tenantID)
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create a product with LIMITED stock
	product := &domain.Product{
		TenantID: tenantID,
		Name:     "Limited Stock Widget",
		Price:    5.0,
		Stock:    initialStock,
	}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	svc := service.NewOrderSvc(orderRepo, itemRepo, productRepo, nil)

	// Test: Try to create 20 orders when only 10 items available
	var wg sync.WaitGroup
	successChan := make(chan bool, numOrders)
	errorChan := make(chan error, numOrders)

	for i := 0; i < numOrders; i++ {
		wg.Add(1)
		go func(orderNum int) {
			defer wg.Done()

			tx := testDB.Begin()
			txCtx := context.WithValue(ctx, "tx", tx)

			orderReq := domain.CreateOrderReq{
				Items: []domain.OrderItemReq{{
					ProductID: product.ID,
					Qty:       qtyPerOrder,
				}},
			}

			_, err := svc.Create(txCtx, tenantID, uint(orderNum+1), orderReq)
			if err != nil {
				tx.Rollback()
				errorChan <- err
			} else {
				tx.Commit()
				successChan <- true
			}
		}(i)
	}

	wg.Wait()
	close(successChan)
	close(errorChan)

	// Collect results
	successCount := 0
	for range successChan {
		successCount++
	}

	failureCount := 0
	for range errorChan {
		failureCount++
	}

	t.Logf("Success: %d orders, Failures: %d orders", successCount, failureCount)

	// Verify: Only 10 orders should succeed (matching stock)
	if successCount != initialStock {
		t.Errorf("expected %d successful orders, got %d", initialStock, successCount)
	}

	// Verify: 10 orders should fail (insufficient stock)
	expectedFailures := numOrders - initialStock
	if failureCount != expectedFailures {
		t.Errorf("expected %d failed orders, got %d", expectedFailures, failureCount)
	}

	// Verify: Final stock should be 0
	finalProduct, err := productRepo.FindByID(ctx, product.ID)
	if err != nil {
		t.Fatalf("fetch final product: %v", err)
	}

	if finalProduct.Stock != 0 {
		t.Errorf("expected final stock 0, got %d", finalProduct.Stock)
	}

	// Verify: No overselling
	if finalProduct.Stock < 0 {
		t.Errorf("CRITICAL: stock went negative (%d)", finalProduct.Stock)
	}
}

// TestConcurrentStockDecrement_RaceCondition_Postgres stress tests the atomic
// stock decrement operation under high concurrency.
func TestConcurrentStockDecrement_RaceCondition_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 302
	const initialStock = 1000
	const numGoroutines = 100
	const decrementsPerGoroutine = 10

	productRepo := repository.NewProductRepo(testDB, tenantID)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create a product with high stock
	product := &domain.Product{
		TenantID: tenantID,
		Name:     "Race Condition Test Widget",
		Price:    1.0,
		Stock:    initialStock,
	}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	// Test: Decrement stock concurrently from multiple goroutines
	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines*decrementsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < decrementsPerGoroutine; j++ {
				if err := productRepo.DecrStock(ctx, product.ID, 1); err != nil {
					errorChan <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errorChan)

	// Check for errors
	errorCount := 0
	for err := range errorChan {
		t.Logf("DecrStock error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("encountered %d errors during concurrent decrements", errorCount)
	}

	// Verify: Final stock should be exactly 0
	finalProduct, err := productRepo.FindByID(ctx, product.ID)
	if err != nil {
		t.Fatalf("fetch final product: %v", err)
	}

	expectedStock := initialStock - (numGoroutines * decrementsPerGoroutine)
	if finalProduct.Stock != expectedStock {
		t.Errorf("expected final stock %d, got %d (race condition detected!)", expectedStock, finalProduct.Stock)
	}
}
