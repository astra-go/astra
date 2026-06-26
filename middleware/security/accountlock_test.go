package security_test

import (
	"context"
	"testing"
	"time"

	sec "github.com/astra-go/astra/middleware/security"
)

var ctx = context.Background()

func TestMemoryAccountLocker_IncrFail(t *testing.T) {
	l := sec.NewMemoryAccountLocker()
	count, err := l.IncrLoginFail(ctx, "test:user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
	count, _ = l.IncrLoginFail(ctx, "test:user-1")
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}
}

func TestMemoryAccountLocker_GetFailCount(t *testing.T) {
	l := sec.NewMemoryAccountLocker()
	count, _ := l.GetLoginFailCount(ctx, "test:user-2")
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
	_, _ = l.IncrLoginFail(ctx, "test:user-2")
	_, _ = l.IncrLoginFail(ctx, "test:user-2")
	count, _ = l.GetLoginFailCount(ctx, "test:user-2")
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}

func TestMemoryAccountLocker_ResetFail(t *testing.T) {
	l := sec.NewMemoryAccountLocker()
	_, _ = l.IncrLoginFail(ctx, "test:user-3")
	_ = l.ResetLoginFail(ctx, "test:user-3")
	count, _ := l.GetLoginFailCount(ctx, "test:user-3")
	if count != 0 {
		t.Fatalf("expected 0 after reset, got %d", count)
	}
}

func TestMemoryAccountLocker_IsLocked(t *testing.T) {
	l := sec.NewMemoryAccountLocker()
	locked, _ := l.IsLocked(ctx, "test:user-4")
	if locked {
		t.Fatal("should not be locked initially")
	}
	for i := 0; i < 5; i++ {
		_, _ = l.IncrLoginFail(ctx, "test:user-4")
	}
	locked, _ = l.IsLocked(ctx, "test:user-4")
	if !locked {
		t.Fatal("should be locked after 5 failures")
	}
	_ = l.ResetLoginFail(ctx, "test:user-4")
	locked, _ = l.IsLocked(ctx, "test:user-4")
	if !locked {
		t.Fatal("resetting fail count should not unlock")
	}
}

func TestMemoryAccountLocker_ManualLock(t *testing.T) {
	l := sec.NewMemoryAccountLocker()
	_ = l.LockAccount(ctx, "test:user-5")
	locked, _ := l.IsLocked(ctx, "test:user-5")
	if !locked {
		t.Fatal("should be locked after manual lock")
	}
}

func TestMemoryAccountLocker_ExpiredLock(t *testing.T) {
	l := sec.NewMemoryAccountLocker(
		sec.WithLockoutDuration(100 * time.Millisecond),
	)
	_ = l.LockAccount(ctx, "test:user-6")
	locked, _ := l.IsLocked(ctx, "test:user-6")
	if !locked {
		t.Fatal("should be locked immediately")
	}
	time.Sleep(150 * time.Millisecond)
	locked, _ = l.IsLocked(ctx, "test:user-6")
	if locked {
		t.Fatal("should be unlocked after expiry")
	}
}

func TestMemoryAccountLocker_IsolatedKeys(t *testing.T) {
	l := sec.NewMemoryAccountLocker()
	_, _ = l.IncrLoginFail(ctx, "user-a")
	_, _ = l.IncrLoginFail(ctx, "user-a")
	_, _ = l.IncrLoginFail(ctx, "user-b")
	countA, _ := l.GetLoginFailCount(ctx, "user-a")
	countB, _ := l.GetLoginFailCount(ctx, "user-b")
	if countA != 2 || countB != 1 {
		t.Fatalf("expected A=2,B=1, got A=%d,B=%d", countA, countB)
	}
}

func TestMemoryAccountLocker_CustomConfig(t *testing.T) {
	l := sec.NewMemoryAccountLocker(
		sec.WithMaxFails(2),
		sec.WithLockoutDuration(time.Minute),
	)
	for i := 0; i < 2; i++ {
		_, _ = l.IncrLoginFail(ctx, "test:user-7")
	}
	locked, _ := l.IsLocked(ctx, "test:user-7")
	if !locked {
		t.Fatal("should lock after 2 failures with custom max")
	}
}
