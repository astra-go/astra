//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/astra-go/astra/cache"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/repository"
	"github.com/astra-go/astra/examples/showcase/internal/service"
	"github.com/redis/go-redis/v9"
)

// setupRedisForTest creates a Redis client for integration tests.
func setupRedisForTest(t *testing.T) cache.Cache {
	t.Helper()

	// Connect to Redis (requires Docker container running)
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use a separate DB for testing
	})

	// Ping to verify connection
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available, skipping cache test: %v", err)
	}

	// Flush test DB before each test
	redisClient.FlushDB(ctx)

	return cache.NewRedisCache(redisClient)
}

// TestCache_ProductListCaching_Postgres verifies that product list queries
// are cached correctly and invalidated when products are updated.
func TestCache_ProductListCaching_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 400

	redisCache := setupRedisForTest(t)
	productRepo := repository.NewProductRepo(testDB, tenantID)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create test products
	for i := 1; i <= 5; i++ {
		p := &domain.Product{
			TenantID: tenantID,
			Name:     fmt.Sprintf("Product %d", i),
			Price:    float64(i * 10),
			Stock:    i * 10,
		}
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("create product %d: %v", i, err)
		}
	}

	// Create cached service
	cachedSvc := service.NewCachedProductSvc(productRepo, redisCache)

	// First call: Should hit database and populate cache
	t.Run("first call populates cache", func(t *testing.T) {
		products, total, err := cachedSvc.List(ctx, 1, 10)
		if err != nil {
			t.Fatalf("List (first call): %v", err)
		}
		if total < 5 {
			t.Errorf("expected at least 5 products, got %d", total)
		}
		if len(products) < 5 {
			t.Errorf("expected at least 5 products in list, got %d", len(products))
		}
	})

	// Second call: Should hit cache (verify by updating DB directly)
	t.Run("second call hits cache", func(t *testing.T) {
		// Update product price directly in DB (bypass cache)
		testDB.Exec("UPDATE products SET price = 9999.99 WHERE tenant_id = ? LIMIT 1", tenantID)

		// Call again - should still return cached (old) value
		products, _, err := cachedSvc.List(ctx, 1, 10)
		if err != nil {
			t.Fatalf("List (cached call): %v", err)
		}

		// Check that we got cached data (not the updated price)
		hasOldPrice := false
		for _, p := range products {
			if p.Price != 9999.99 {
				hasOldPrice = true
				break
			}
		}
		if !hasOldPrice {
			t.Error("expected cached data, but got fresh data from DB")
		}
	})

	// Cache invalidation test
	t.Run("cache invalidation on update", func(t *testing.T) {
		// Get first product
		products, _, _ := productRepo.FindAll(ctx, nil)
		if len(products) == 0 {
			t.Fatal("no products found")
		}
		firstProduct := products[0]

		// Update product via service (should invalidate cache)
		if err := cachedSvc.Update(ctx, firstProduct.ID, map[string]any{"price": 1234.56}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		// Wait a bit for cache invalidation to propagate
		time.Sleep(100 * time.Millisecond)

		// Next List call should fetch fresh data
		products, _, err := cachedSvc.List(ctx, 1, 10)
		if err != nil {
			t.Fatalf("List (after update): %v", err)
		}

		// Check that we got updated price
		hasUpdatedPrice := false
		for _, p := range products {
			if p.ID == firstProduct.ID && p.Price == 1234.56 {
				hasUpdatedPrice = true
				break
			}
		}
		if !hasUpdatedPrice {
			t.Error("cache not invalidated: expected updated price 1234.56")
		}
	})
}

// TestCache_ProductDetailCaching_Postgres verifies that individual product
// details are cached and invalidated correctly.
func TestCache_ProductDetailCaching_Postgres(t *testing.T) {
	ctx := context.Background()
	const tenantID uint = 401

	redisCache := setupRedisForTest(t)
	productRepo := repository.NewProductRepo(testDB, tenantID)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create a product
	product := &domain.Product{
		TenantID: tenantID,
		Name:     "Cacheable Widget",
		Price:    100.0,
		Stock:    50,
	}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("create product: %v", err)
	}

	cachedSvc := service.NewCachedProductSvc(productRepo, redisCache)

	// First call: Cache miss, should fetch from DB
	t.Run("cache miss", func(t *testing.T) {
		got, err := cachedSvc.GetByID(ctx, product.ID)
		if err != nil {
			t.Fatalf("GetByID (first call): %v", err)
		}
		if got.Name != "Cacheable Widget" {
			t.Errorf("name mismatch: %s", got.Name)
		}
	})

	// Second call: Cache hit
	t.Run("cache hit", func(t *testing.T) {
		// Update DB directly (bypass cache)
		testDB.Exec("UPDATE products SET name = ? WHERE id = ?", "Updated Name", product.ID)

		// Should still return cached value
		got, err := cachedSvc.GetByID(ctx, product.ID)
		if err != nil {
			t.Fatalf("GetByID (cached call): %v", err)
		}
		if got.Name != "Cacheable Widget" {
			t.Error("expected cached value 'Cacheable Widget'")
		}
	})

	// Cache invalidation
	t.Run("cache invalidation", func(t *testing.T) {
		// Update via service (should invalidate cache)
		if err := cachedSvc.Update(ctx, product.ID, map[string]any{"name": "Fresh Name"}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		// Should fetch fresh value
		got, err := cachedSvc.GetByID(ctx, product.ID)
		if err != nil {
			t.Fatalf("GetByID (after update): %v", err)
		}
		if got.Name != "Fresh Name" {
			t.Errorf("expected updated name 'Fresh Name', got '%s'", got.Name)
		}
	})

	// Delete invalidation
	t.Run("cache invalidation on delete", func(t *testing.T) {
		if err := cachedSvc.Delete(ctx, product.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// Should return error (not found)
		_, err := cachedSvc.GetByID(ctx, product.ID)
		if err == nil {
			t.Error("expected error after delete, but got cached value")
		}
	})
}

// TestCache_TTLExpiration_Postgres verifies that cache entries expire
// after their TTL.
func TestCache_TTLExpiration_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TTL test in short mode")
	}

	ctx := context.Background()
	const tenantID uint = 402

	redisCache := setupRedisForTest(t)
	productRepo := repository.NewProductRepo(testDB, tenantID)

	defer func() {
		testDB.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
	}()

	// Setup: Create a product
	product := &domain.Product{
		TenantID: tenantID,
		Name:     "TTL Test Widget",
		Price:    50.0,
		Stock:    25,
	}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("create product: %v", err)
	}

	// Use cache with short TTL (5 seconds for testing)
	shortTTLCache := cache.NewRedisCacheWithTTL(redisCache.(*cache.RedisCache).Client(), 5*time.Second)
	cachedSvc := service.NewCachedProductSvc(productRepo, shortTTLCache)

	// First call: Populate cache
	got1, err := cachedSvc.GetByID(ctx, product.ID)
	if err != nil {
		t.Fatalf("GetByID (first): %v", err)
	}
	if got1.Name != "TTL Test Widget" {
		t.Errorf("name mismatch: %s", got1.Name)
	}

	// Update DB directly (cache still valid)
	testDB.Exec("UPDATE products SET name = ? WHERE id = ?", "Updated During Cache", product.ID)

	// Immediate call: Should still return cached value
	got2, _ := cachedSvc.GetByID(ctx, product.ID)
	if got2.Name != "TTL Test Widget" {
		t.Error("expected cached value immediately after update")
	}

	// Wait for TTL to expire
	t.Log("Waiting for cache TTL to expire (6 seconds)...")
	time.Sleep(6 * time.Second)

	// After TTL: Should fetch fresh value from DB
	got3, err := cachedSvc.GetByID(ctx, product.ID)
	if err != nil {
		t.Fatalf("GetByID (after TTL): %v", err)
	}
	if got3.Name != "Updated During Cache" {
		t.Errorf("expected fresh value 'Updated During Cache', got '%s'", got3.Name)
	}
}
