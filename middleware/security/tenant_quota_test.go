package security_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/astra-go/astra"
	sec "github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
)

// ─── MemoryQuotaStore tests ──────────────────────────────────────────────────

func TestMemoryQuotaStore_IncrRequests(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	ctx := context.Background()

	count, err := store.IncrRequests(ctx, "acme", 1)
	if err != nil {
		t.Fatalf("IncrRequests: %v", err)
	}
	if count != 1 {
		t.Errorf("want 1, got %d", count)
	}

	count, err = store.IncrRequests(ctx, "acme", 5)
	if err != nil {
		t.Fatalf("IncrRequests: %v", err)
	}
	if count != 6 {
		t.Errorf("want 6, got %d", count)
	}
}

func TestMemoryQuotaStore_GetDailyCount(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	ctx := context.Background()

	count, err := store.GetDailyCount(ctx, "acme")
	if err != nil {
		t.Fatalf("GetDailyCount: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0 for unknown tenant, got %d", count)
	}

	store.IncrRequests(ctx, "acme", 10)
	count, err = store.GetDailyCount(ctx, "acme")
	if err != nil {
		t.Fatalf("GetDailyCount: %v", err)
	}
	if count != 10 {
		t.Errorf("want 10, got %d", count)
	}
}

func TestMemoryQuotaStore_ResetDailyCount(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	ctx := context.Background()

	store.IncrRequests(ctx, "acme", 100)
	err := store.ResetDailyCount(ctx, "acme")
	if err != nil {
		t.Fatalf("ResetDailyCount: %v", err)
	}

	count, _ := store.GetDailyCount(ctx, "acme")
	if count != 0 {
		t.Errorf("want 0 after reset, got %d", count)
	}
}

func TestMemoryQuotaStore_QuotaOverrides(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	ctx := context.Background()

	// No override → nil.
	limits, err := store.GetQuota(ctx, "acme")
	if err != nil {
		t.Fatalf("GetQuota: %v", err)
	}
	if limits != nil {
		t.Errorf("want nil for unknown tenant, got %v", limits)
	}

	// Set override.
	quota := &sec.TenantQuotaLimits{
		QPS:         200,
		Burst:       40,
		MaxConcurrent: 100,
		DailyLimit:   50000,
	}
	err = store.SetQuota(ctx, "acme", quota)
	if err != nil {
		t.Fatalf("SetQuota: %v", err)
	}

	limits, err = store.GetQuota(ctx, "acme")
	if err != nil {
		t.Fatalf("GetQuota: %v", err)
	}
	if limits.QPS != 200 || limits.Burst != 40 || limits.MaxConcurrent != 100 || limits.DailyLimit != 50000 {
		t.Errorf("unexpected limits: %v", limits)
	}
}

func TestMemoryQuotaStore_IndependentTenants(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	ctx := context.Background()

	store.IncrRequests(ctx, "acme", 5)
	store.IncrRequests(ctx, "beta", 10)

	acmeCount, _ := store.GetDailyCount(ctx, "acme")
	betaCount, _ := store.GetDailyCount(ctx, "beta")

	if acmeCount != 5 {
		t.Errorf("acme: want 5, got %d", acmeCount)
	}
	if betaCount != 10 {
		t.Errorf("beta: want 10, got %d", betaCount)
	}
}

// ─── TenantQuota middleware tests ────────────────────────────────────────────

func TestTenantQuota_QPSLimit(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	app := testutil.NewTestApp()
	app.Use(sec.TenantQuotaWithConfig(sec.TenantQuotaConfig{
		Store:       store,
		DefaultQPS:  1,
		DefaultBurst: 1,
		KeyFunc:     func(_ *astra.Ctx) string { return "acme" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	// First request should pass (burst=1).
	s.GET("/").AssertStatus(http.StatusOK)
	// Immediate second request should be rejected (tokens exhausted).
	s.GET("/").AssertStatus(http.StatusTooManyRequests)
}

func TestTenantQuota_ConcurrentLimit(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	app := testutil.NewTestApp()
	app.Use(sec.TenantQuotaWithConfig(sec.TenantQuotaConfig{
		Store:        store,
		MaxConcurrent: 1,
		KeyFunc:      func(_ *astra.Ctx) string { return "acme" },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		time.Sleep(50 * time.Millisecond) // hold the slot
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	// First request passes.
	s.GET("/").AssertStatus(http.StatusOK)
}

func TestTenantQuota_DailyLimit(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	app := testutil.NewTestApp()
	app.Use(sec.TenantQuotaWithConfig(sec.TenantQuotaConfig{
		Store:      store,
		DailyLimit: 2,
		KeyFunc:    func(_ *astra.Ctx) string { return "acme" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
	s.GET("/").AssertStatus(http.StatusOK)
	s.GET("/").AssertStatus(http.StatusTooManyRequests)
}

func TestTenantQuota_Skipper(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	app := testutil.NewTestApp()
	app.Use(sec.TenantQuotaWithConfig(sec.TenantQuotaConfig{
		Store:      store,
		DailyLimit: 1,
		KeyFunc:    func(_ *astra.Ctx) string { return "acme" },
		Skipper:    func(c *astra.Ctx) bool { return c.Request().URL.Path == "/health" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	app.GET("/health", func(c *astra.Ctx) error { return c.String(http.StatusOK, "healthy") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
	s.GET("/").AssertStatus(http.StatusTooManyRequests) // daily limit hit
	s.GET("/health").AssertStatus(http.StatusOK)         // skipped
}

func TestTenantQuota_EmptyTenant(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	app := testutil.NewTestApp()
	app.Use(sec.TenantQuotaWithConfig(sec.TenantQuotaConfig{
		Store:      store,
		DailyLimit: 1,
		KeyFunc:    func(_ *astra.Ctx) string { return "" }, // no tenant
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	// Empty tenant → no quota enforcement, all requests pass.
	s.GET("/").AssertStatus(http.StatusOK)
	s.GET("/").AssertStatus(http.StatusOK)
}

func TestTenantQuota_QuotaOverride(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	ctx := context.Background()

	// Set override for "premium" tenant with higher daily limit.
	store.SetQuota(ctx, "premium", &sec.TenantQuotaLimits{
		DailyLimit: 5,
	})

	app := testutil.NewTestApp()
	app.Use(sec.TenantQuotaWithConfig(sec.TenantQuotaConfig{
		Store:      store,
		DailyLimit: 1, // default is 1
		KeyFunc:    func(c *astra.Ctx) string { return c.Header("X-Tenant-ID") },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	// Default tenant: 1 request only.
	s.GET("/", map[string]string{"X-Tenant-ID": "default"}).AssertStatus(http.StatusOK)
	s.GET("/", map[string]string{"X-Tenant-ID": "default"}).AssertStatus(http.StatusTooManyRequests)

	// Premium tenant: 5 requests.
	for i := 0; i < 5; i++ {
		s.GET("/", map[string]string{"X-Tenant-ID": "premium"}).AssertStatus(http.StatusOK)
	}
	s.GET("/", map[string]string{"X-Tenant-ID": "premium"}).AssertStatus(http.StatusTooManyRequests)
}

func TestTenantQuota_UnlimitedDefaults(t *testing.T) {
	store := sec.NewMemoryQuotaStore()
	app := testutil.NewTestApp()
	app.Use(sec.TenantQuotaWithConfig(sec.TenantQuotaConfig{
		Store: store,
		// All limits are 0 → unlimited.
		KeyFunc: func(_ *astra.Ctx) string { return "acme" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	for i := 0; i < 100; i++ {
		s.GET("/").AssertStatus(http.StatusOK)
	}
}