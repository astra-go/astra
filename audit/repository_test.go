package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/astra-go/astra/audit"
)

var bg = context.Background()

func uin(v int64) *int64 { return &v }

// TestMemoryRepository_LogAndList verifies basic Log + List flow.
func TestMemoryRepository_LogAndList(t *testing.T) {
	repo := audit.NewMemoryRepository()

	tenantID := int64(1)
	entries, err := repo.List(bg, tenantID, audit.ActionLogin, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty list, got %d entries", len(entries))
	}

	if err := repo.Log(bg, audit.Entry{
		TenantID: tenantID,
		UIN:      uin(1001),
		Action:   audit.ActionLogin,
		Status:   audit.StatusSuccess,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err = repo.List(bg, tenantID, audit.ActionLogin, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != audit.ActionLogin || *entries[0].UIN != 1001 {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

// TestMemoryRepository_FilterByAction verifies action filtering.
func TestMemoryRepository_FilterByAction(t *testing.T) {
	repo := audit.NewMemoryRepository()
	tenantID := int64(1)

	for i := 0; i < 3; i++ {
		_ = repo.Log(bg, audit.Entry{TenantID: tenantID, Action: audit.ActionLogin, Status: audit.StatusSuccess})
		_ = repo.Log(bg, audit.Entry{TenantID: tenantID, Action: audit.ActionRegister, Status: audit.StatusSuccess})
	}

	entries, _ := repo.List(bg, tenantID, audit.ActionLogin, 100, 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 Login entries, got %d", len(entries))
	}

	entries, _ = repo.List(bg, tenantID, audit.ActionRegister, 100, 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 Register entries, got %d", len(entries))
	}
}

// TestMemoryRepository_FilterByTenant verifies tenant isolation.
func TestMemoryRepository_FilterByTenant(t *testing.T) {
	repo := audit.NewMemoryRepository()

	_ = repo.Log(bg, audit.Entry{TenantID: 1, Action: audit.ActionLogin, Status: audit.StatusSuccess})
	_ = repo.Log(bg, audit.Entry{TenantID: 2, Action: audit.ActionLogin, Status: audit.StatusSuccess})

	entries, _ := repo.List(bg, 1, "", 100, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for tenant 1, got %d", len(entries))
	}
}

// TestMemoryRepository_Pagination verifies limit/offset paging.
func TestMemoryRepository_Pagination(t *testing.T) {
	repo := audit.NewMemoryRepository()
	tenantID := int64(1)
	n := 10
	for i := 0; i < n; i++ {
		_ = repo.Log(bg, audit.Entry{TenantID: tenantID, Action: audit.ActionLogin, Status: audit.StatusSuccess})
	}

	entries, _ := repo.List(bg, tenantID, "", 3, 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries on first page, got %d", len(entries))
	}

	entries, _ = repo.List(bg, tenantID, "", 3, 3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries on second page, got %d", len(entries))
	}

	entries, _ = repo.List(bg, tenantID, "", 10, 9)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry on last page, got %d", len(entries))
	}
}

// TestMemoryRepository_ReverseOrder verifies reverse insertion order.
func TestMemoryRepository_ReverseOrder(t *testing.T) {
	repo := audit.NewMemoryRepository()
	tenantID := int64(1)

	_ = repo.Log(bg, audit.Entry{TenantID: tenantID, Action: audit.ActionLogin, Status: audit.StatusSuccess})
	_ = repo.Log(bg, audit.Entry{TenantID: tenantID, Action: audit.ActionRegister, Status: audit.StatusSuccess})

	entries, _ := repo.List(bg, tenantID, "", 10, 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Most recent first
	if entries[0].Action != audit.ActionRegister {
		t.Fatalf("expected most recent Register first, got %s", entries[0].Action)
	}
	if entries[1].Action != audit.ActionLogin {
		t.Fatalf("expected older Login second, got %s", entries[1].Action)
	}
}

// TestLog_RepoNil is a no-op when repo is nil.
func TestLog_RepoNil(t *testing.T) {
	audit.Log(bg, nil, 1, uin(100), audit.ActionLogin, audit.StatusSuccess, nil)
}

// TestLog_ContextMetadata verifies async logging with metadata propagation.
func TestLog_ContextMetadata(t *testing.T) {
	repo := audit.NewMemoryRepository()

	ctx := audit.WithClientMetadata(bg, "10.0.0.1", "test-agent", "dev-123", "req-abc")
	_ = repo.Log(bg, audit.Entry{TenantID: 1, Action: audit.ActionLogin, Status: audit.StatusSuccess})

	audit.Log(ctx, repo, 1, uin(100), audit.ActionLogin, audit.StatusSuccess, nil)
	audit.Log(ctx, repo, 1, uin(100), audit.ActionLogin, audit.StatusSuccess,
		audit.Details{"source": "web"})

	// Give async goroutines time to complete
	time.Sleep(100 * time.Millisecond)

	entries, _ := repo.List(bg, 1, audit.ActionLogin, 10, 0)
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d; async goroutines may not have completed", len(entries))
	}

	// Find the entries with metadata
	var foundIP, foundDetails bool
	for _, e := range entries {
		if e.IPAddress == "10.0.0.1" {
			foundIP = true
		}
		if e.Details != nil && e.Details["source"] == "web" {
			foundDetails = true
		}
	}
	if !foundIP {
		t.Fatal("expected an entry with IP 10.0.0.1")
	}
	if !foundDetails {
		t.Fatal("expected an entry with source=web details")
	}
}

// TestLogSync_ContextMetadata verifies synchronous logging.
func TestLogSync_ContextMetadata(t *testing.T) {
	repo := audit.NewMemoryRepository()

	ctx := audit.WithClientMetadata(bg, "10.0.0.2", "sync-agent", "dev-456", "req-xyz")

	if err := audit.LogSync(ctx, repo, 1, uin(200), audit.ActionRegister, audit.StatusSuccess,
		audit.Details{"method": "email"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := repo.List(bg, 1, audit.ActionRegister, 10, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].IPAddress != "10.0.0.2" {
		t.Fatalf("expected IP 10.0.0.2, got %s", entries[0].IPAddress)
	}
	if *entries[0].UIN != 200 {
		t.Fatalf("expected UIN 200, got %d", *entries[0].UIN)
	}
	if entries[0].Details["method"] != "email" {
		t.Fatalf("expected method=email, got %+v", entries[0].Details)
	}
}

// TestActionConstants verifies that all common action constants are non-empty.
func TestActionConstants(t *testing.T) {
	actions := []audit.Action{
		audit.ActionLogin, audit.ActionRegister, audit.ActionLogout,
		audit.ActionSendCode, audit.ActionVerifyCode,
		audit.ActionRefreshToken, audit.ActionVerifyToken,
		audit.ActionResetPwd, audit.ActionChangePwd,
		audit.ActionBindPhone, audit.ActionUnbindPhone,
		audit.ActionBindEmail, audit.ActionUnbindEmail,
		audit.ActionBindSocial, audit.ActionUnbindSocial,
		audit.ActionDeviceReport, audit.ActionRevokeTokens,
		audit.ActionAdminOp,
	}
	for _, a := range actions {
		if a == "" {
			t.Error("empty action constant found")
		}
	}
}

// TestStatusConstants verifies status constants.
func TestStatusConstants(t *testing.T) {
	if audit.StatusSuccess != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", audit.StatusSuccess)
	}
	if audit.StatusFailure != "FAILURE" {
		t.Errorf("expected FAILURE, got %s", audit.StatusFailure)
	}
}

// TestDetailsType verifies Details is usable as map[string]any.
func TestDetailsType(t *testing.T) {
	d := audit.Details{"key": "value", "count": 42}
	if d["key"] != "value" {
		t.Errorf("expected value, got %v", d["key"])
	}
	if d["count"] != 42 {
		t.Errorf("expected 42, got %v", d["count"])
	}
}

// BenchmarkMemoryRepository_Log benchmarks write performance.
func BenchmarkMemoryRepository_Log(b *testing.B) {
	repo := audit.NewMemoryRepository()
	entry := audit.Entry{TenantID: 1, UIN: uin(100), Action: audit.ActionLogin, Status: audit.StatusSuccess}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = repo.Log(bg, entry)
	}
}
