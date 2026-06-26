package token

import (
	"context"
	"testing"
	"time"
)

func TestMemoryManager_Issue_Find(t *testing.T) {
	mgr := NewMemoryManager()
	ctx := context.Background()

	access, refresh, entry, err := mgr.Issue(ctx, 42, 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected non-empty tokens")
	}
	if entry.UIN != 42 {
		t.Fatalf("expected UIN 42, got %d", entry.UIN)
	}
	if entry.ExpiresAt <= time.Now().Unix() {
		t.Fatal("expiresAt should be in the future")
	}
	if entry.TokenHash == "" || entry.RefreshHash == "" {
		t.Fatal("expected non-empty hashes")
	}

	// Find by access
	found, err := mgr.FindByAccess(ctx, access)
	if err != nil {
		t.Fatalf("FindByAccess failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find token by access")
	}
	if found.UIN != 42 {
		t.Fatalf("expected UIN 42, got %d", found.UIN)
	}

	// Find by refresh
	found, err = mgr.FindByRefresh(ctx, refresh)
	if err != nil {
		t.Fatalf("FindByRefresh failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find token by refresh")
	}
}

func TestMemoryManager_SSO_Enforcement(t *testing.T) {
	mgr := NewMemoryManager()
	ctx := context.Background()

	_, _, entry1, err := mgr.Issue(ctx, 42, 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("first Issue failed: %v", err)
	}

	// Issue again for same user — should evict the first pair.
	_, _, entry2, err := mgr.Issue(ctx, 42, 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("second Issue failed: %v", err)
	}

	if entry1.TokenHash == entry2.TokenHash {
		t.Fatal("expected different token hashes after re-issue")
	}
}

func TestMemoryManager_Blacklist(t *testing.T) {
	mgr := NewMemoryManager()
	ctx := context.Background()

	access, _, _, err := mgr.Issue(ctx, 42, 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	// Before blacklist — should be findable
	found, _ := mgr.FindByAccess(ctx, access)
	if found == nil {
		t.Fatal("expected to find token before blacklist")
	}

	// Blacklist
	if err := mgr.Blacklist(ctx, access, 15*time.Minute); err != nil {
		t.Fatalf("Blacklist failed: %v", err)
	}

	// After blacklist — should NOT be findable
	blacklisted, err := mgr.IsBlacklisted(ctx, access)
	if err != nil {
		t.Fatalf("IsBlacklisted failed: %v", err)
	}
	if !blacklisted {
		t.Fatal("expected token to be blacklisted")
	}

	found, _ = mgr.FindByAccess(ctx, access)
	if found != nil {
		t.Fatal("expected FindByAccess to return nil for blacklisted token")
	}
}

func TestMemoryManager_JTI(t *testing.T) {
	mgr := NewMemoryManager()
	ctx := context.Background()

	jti := "test-jti-123"

	used, err := mgr.IsJTIUsed(ctx, jti)
	if err != nil {
		t.Fatalf("IsJTIUsed failed: %v", err)
	}
	if used {
		t.Fatal("expected fresh JTI to not be used")
	}

	if err := mgr.SetJTI(ctx, jti, 5*time.Minute); err != nil {
		t.Fatalf("SetJTI failed: %v", err)
	}

	used, err = mgr.IsJTIUsed(ctx, jti)
	if err != nil {
		t.Fatalf("IsJTIUsed failed: %v", err)
	}
	if !used {
		t.Fatal("expected JTI to be marked as used")
	}
}

func TestMemoryManager_DeleteByAccount(t *testing.T) {
	mgr := NewMemoryManager()
	ctx := context.Background()

	access, refresh, _, err := mgr.Issue(ctx, 42, 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	if err := mgr.DeleteByAccount(ctx, 42); err != nil {
		t.Fatalf("DeleteByAccount failed: %v", err)
	}

	// Both tokens should be gone
	if found, _ := mgr.FindByAccess(ctx, access); found != nil {
		t.Fatal("expected FindByAccess to return nil after DeleteByAccount")
	}
	if found, _ := mgr.FindByRefresh(ctx, refresh); found != nil {
		t.Fatal("expected FindByRefresh to return nil after DeleteByAccount")
	}
}

func TestMemoryManager_UnknownToken(t *testing.T) {
	mgr := NewMemoryManager()
	ctx := context.Background()

	found, err := mgr.FindByAccess(ctx, "nonexistent-token")
	if err != nil {
		t.Fatalf("FindByAccess failed: %v", err)
	}
	if found != nil {
		t.Fatal("expected nil for unknown token")
	}
}

func TestMemoryManager_ConcurrentAccess(t *testing.T) {
	mgr := NewMemoryManager()
	ctx := context.Background()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int64) {
			_, _, _, err := mgr.Issue(ctx, id, 15*time.Minute, 24*time.Hour)
			if err != nil {
				t.Errorf("concurrent Issue failed: %v", err)
			}
			done <- true
		}(int64(i + 1))
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestMemoryManager_BlacklistExpiry(t *testing.T) {
	mgr := NewMemoryManager()
	ctx := context.Background()

	access, _, _, err := mgr.Issue(ctx, 42, 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	// Blacklist with zero TTL (immediate expiry)
	if err := mgr.Blacklist(ctx, access, 0); err != nil {
		t.Fatalf("Blacklist failed: %v", err)
	}

	// Should not be blacklisted (already expired)
	blacklisted, err := mgr.IsBlacklisted(ctx, access)
	if err != nil {
		t.Fatalf("IsBlacklisted failed: %v", err)
	}
	if blacklisted {
		t.Fatal("expected blacklist to have expired")
	}
}

func TestMemoryManager_JTIExpiry(t *testing.T) {
	mgr := NewMemoryManager()
	ctx := context.Background()

	jti := "expired-jti"

	// Set with a short negative duration to simulate immediate expiry.
	// Using 0 would set the expiry to now, so the check should pass immediately.
	// Instead let's set a very short positive TTL then wait.
	if err := mgr.SetJTI(ctx, jti+"-never", time.Hour); err != nil {
		t.Fatalf("SetJTI failed: %v", err)
	}

	used, _ := mgr.IsJTIUsed(ctx, jti+"-never")
	if !used {
		t.Fatal("expected JTI with 1h TTL to be used")
	}
}
