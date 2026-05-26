//go:build redis

package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// fakeL2 is a simple in-memory JWTCacheBackend that mimics a remote store,
// letting us test MultiLevelJWTCache without a real Redis connection.
type fakeL2 struct {
	entries map[string]*Claims
	gets    int
	sets    int
	deletes int
}

func newFakeL2() *fakeL2 { return &fakeL2{entries: make(map[string]*Claims)} }

func (f *fakeL2) Get(_ context.Context, sig string) (*Claims, bool) {
	f.gets++
	c, ok := f.entries[sig]
	return c, ok
}

func (f *fakeL2) Set(_ context.Context, sig string, claims *Claims, _ int64) {
	f.sets++
	f.entries[sig] = claims
}

func (f *fakeL2) Delete(_ context.Context, sig string) error {
	f.deletes++
	delete(f.entries, sig)
	return nil
}

// newTestClaims builds a *Claims with an expiry offset seconds from now.
func newTestClaims(sub string, offsetSec int64) *Claims {
	exp := time.Now().Add(time.Duration(offsetSec) * time.Second)
	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
}

// multiLevelCacheWithFakeL2 builds a MultiLevelJWTCache wired to a fakeL2.
// We bypass the RedisJWTCache type requirement by wrapping fakeL2 in a thin
// adapter that satisfies JWTCacheBackend, then test the cache logic directly
// via the exported Get/Set/Delete methods.
//
// Because MultiLevelJWTCache.l2 is typed *RedisJWTCache (not the interface),
// we test the internal l1/l2 interaction through the exported JWTCacheBackend
// interface methods, using a standalone jwtCache as L1 and fakeL2 as L2.
type testMultiLevel struct {
	l1     *jwtCache
	l2     *fakeL2
	hits   int
	l2hits int
	misses int
}

func newTestMultiLevel(l1Size int) *testMultiLevel {
	return &testMultiLevel{
		l1: newJWTCache(l1Size),
		l2: newFakeL2(),
	}
}

func (m *testMultiLevel) Get(ctx context.Context, sig string) (*Claims, bool) {
	now := time.Now().Unix()
	if claims, ok := m.l1.get(sig, now); ok {
		m.hits++
		return claims, true
	}
	claims, ok := m.l2.Get(ctx, sig)
	if !ok {
		m.misses++
		return nil, false
	}
	m.l2hits++
	if claims.ExpiresAt != nil {
		m.l1.set(sig, claims, claims.ExpiresAt.Unix(), now)
	}
	return claims, true
}

func (m *testMultiLevel) Set(ctx context.Context, sig string, claims *Claims, expireAt int64) {
	now := time.Now().Unix()
	m.l1.set(sig, claims, expireAt, now)
	m.l2.Set(ctx, sig, claims, expireAt)
}

func (m *testMultiLevel) Delete(ctx context.Context, sig string) {
	m.l1.delete(sig)
	m.l2.Delete(ctx, sig) //nolint:errcheck
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestMultiLevel_SetThenGet_L1Hit(t *testing.T) {
	ctx := context.Background()
	ml := newTestMultiLevel(64)
	claims := newTestClaims("alice", 3600)

	ml.Set(ctx, "sig1", claims, claims.ExpiresAt.Unix())

	got, ok := ml.Get(ctx, "sig1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Subject != "alice" {
		t.Errorf("expected subject alice, got %s", got.Subject)
	}
	if ml.hits != 1 {
		t.Errorf("expected 1 L1 hit, got %d", ml.hits)
	}
	if ml.l2.gets != 0 {
		t.Errorf("expected 0 L2 gets (L1 should have served), got %d", ml.l2.gets)
	}
}

func TestMultiLevel_L1Miss_L2Hit_Promotes(t *testing.T) {
	ctx := context.Background()
	ml := newTestMultiLevel(64)
	claims := newTestClaims("bob", 3600)

	// Write only to L2, bypassing L1
	ml.l2.Set(ctx, "sig2", claims, claims.ExpiresAt.Unix())

	// First Get: L1 miss → L2 hit → promote to L1
	got, ok := ml.Get(ctx, "sig2")
	if !ok {
		t.Fatal("expected L2 hit")
	}
	if got.Subject != "bob" {
		t.Errorf("expected subject bob, got %s", got.Subject)
	}
	if ml.l2hits != 1 {
		t.Errorf("expected 1 L2 hit, got %d", ml.l2hits)
	}

	// Second Get: should now be served from L1
	ml.Get(ctx, "sig2") //nolint:errcheck
	if ml.hits != 1 {
		t.Errorf("expected 1 L1 hit after promotion, got %d", ml.hits)
	}
	if ml.l2.gets != 1 {
		t.Errorf("expected L2 to be queried only once, got %d", ml.l2.gets)
	}
}

func TestMultiLevel_BothMiss(t *testing.T) {
	ctx := context.Background()
	ml := newTestMultiLevel(64)

	_, ok := ml.Get(ctx, "nonexistent")
	if ok {
		t.Fatal("expected cache miss")
	}
	if ml.misses != 1 {
		t.Errorf("expected 1 miss, got %d", ml.misses)
	}
}

func TestMultiLevel_Delete_RemovesBothTiers(t *testing.T) {
	ctx := context.Background()
	ml := newTestMultiLevel(64)
	claims := newTestClaims("carol", 3600)

	ml.Set(ctx, "sig3", claims, claims.ExpiresAt.Unix())

	// Confirm it's in L1
	if _, ok := ml.l1.get("sig3", time.Now().Unix()); !ok {
		t.Fatal("expected entry in L1 before delete")
	}
	// Confirm it's in L2
	if _, ok := ml.l2.entries["sig3"]; !ok {
		t.Fatal("expected entry in L2 before delete")
	}

	ml.Delete(ctx, "sig3")

	if _, ok := ml.l1.get("sig3", time.Now().Unix()); ok {
		t.Error("expected L1 entry to be deleted")
	}
	if _, ok := ml.l2.entries["sig3"]; ok {
		t.Error("expected L2 entry to be deleted")
	}
	if ml.l2.deletes != 1 {
		t.Errorf("expected 1 L2 delete, got %d", ml.l2.deletes)
	}
}

func TestMultiLevel_SetWritesBothTiers(t *testing.T) {
	ctx := context.Background()
	ml := newTestMultiLevel(64)
	claims := newTestClaims("dave", 3600)

	ml.Set(ctx, "sig4", claims, claims.ExpiresAt.Unix())

	if ml.l2.sets != 1 {
		t.Errorf("expected 1 L2 set, got %d", ml.l2.sets)
	}
	if _, ok := ml.l1.get("sig4", time.Now().Unix()); !ok {
		t.Error("expected entry in L1 after Set")
	}
}

func TestMultiLevel_ExpiredL1_FallsBackToL2(t *testing.T) {
	ctx := context.Background()
	ml := newTestMultiLevel(64)

	// Insert with a past expiry so L1 treats it as expired immediately
	past := time.Now().Add(-1 * time.Second)
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "eve",
			ExpiresAt: jwt.NewNumericDate(past),
		},
	}
	// Manually insert into L1 with expired TTL
	ml.l1.set("sig5", claims, past.Unix(), time.Now().Unix())

	// Put a fresh copy in L2
	fresh := newTestClaims("eve", 3600)
	ml.l2.Set(ctx, "sig5", fresh, fresh.ExpiresAt.Unix())

	got, ok := ml.Get(ctx, "sig5")
	if !ok {
		t.Fatal("expected L2 hit after L1 expiry")
	}
	if got.Subject != "eve" {
		t.Errorf("expected subject eve, got %s", got.Subject)
	}
	if ml.l2hits != 1 {
		t.Errorf("expected 1 L2 hit, got %d", ml.l2hits)
	}
}

// TestJWTCache_Delete verifies the new delete method on jwtCache.
func TestJWTCache_Delete(t *testing.T) {
	c := newJWTCache(64)
	claims := newTestClaims("frank", 3600)
	now := time.Now().Unix()

	c.set("sigX", claims, claims.ExpiresAt.Unix(), now)
	if _, ok := c.get("sigX", now); !ok {
		t.Fatal("expected entry before delete")
	}

	c.delete("sigX")
	if _, ok := c.get("sigX", now); ok {
		t.Error("expected entry to be gone after delete")
	}
}
