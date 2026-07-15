package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra/cache"
)

func TestNew_Defaults(t *testing.T) {
	c := New()
	defer c.Close()

	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0", c.Len())
	}
	if c.Cap() != 0 {
		t.Errorf("Cap = %d, want 0 (unlimited)", c.Cap())
	}
}

func TestNew_WithConfig(t *testing.T) {
	c := New(Config{Cap: 100, CleanupInterval: time.Second})
	defer c.Close()

	if c.Cap() != 100 {
		t.Errorf("Cap = %d, want 100", c.Cap())
	}
}

func TestSetAndGet(t *testing.T) {
	c := New()
	defer c.Close()

	ctx := context.Background()
	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	val, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "v" {
		t.Errorf("value = %q, want %q", string(val), "v")
	}
}

func TestGet_Miss(t *testing.T) {
	c := New()
	defer c.Close()

	_, err := c.Get(context.Background(), "no-such-key")
	if err != cache.ErrCacheMiss {
		t.Errorf("Get() = %v, want ErrCacheMiss", err)
	}
}

func TestSet_Update(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "k", []byte("a"), time.Minute)
	_ = c.Set(ctx, "k", []byte("b"), time.Minute)

	val, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "b" {
		t.Errorf("value = %q, want %q", string(val), "b")
	}
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1 (update, not duplicate)", c.Len())
	}
}

func TestGet_Expired(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "k", []byte("v"), 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	_, err := c.Get(ctx, "k")
	if err != cache.ErrCacheMiss {
		t.Errorf("Get() = %v, want ErrCacheMiss (expired)", err)
	}
	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0 (lazy eviction)", c.Len())
	}
}

func TestDelete(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "a", []byte("1"), time.Minute)
	_ = c.Set(ctx, "b", []byte("2"), time.Minute)
	_ = c.Set(ctx, "c", []byte("3"), time.Minute)

	if err := c.Delete(ctx, "a", "c"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}

	_, err := c.Get(ctx, "b")
	if err != nil {
		t.Errorf("key 'b' should still exist: %v", err)
	}
}

func TestDelete_Missing(t *testing.T) {
	c := New()
	defer c.Close()

	// Deleting a missing key should not error.
	if err := c.Delete(context.Background(), "no-such"); err != nil {
		t.Errorf("Delete missing = %v, want nil", err)
	}
}

func TestExists(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "k", []byte("v"), time.Minute)

	ok, err := c.Exists(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("Exists(k) = false, want true")
	}

	ok, err = c.Exists(ctx, "absent")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Exists(absent) = true, want false")
	}
}

func TestExists_DoesNotPromote(t *testing.T) {
	c := New(Config{Cap: 2})
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "a", []byte("1"), time.Minute)
	_ = c.Set(ctx, "b", []byte("2"), time.Minute)

	// Exists should NOT promote 'a' to front.
	_, _ = c.Exists(ctx, "a")

	// Add a third key — 'b' was set more recently, so 'a' should be evicted
	// if Exists didn't promote it. If it did promote 'a', 'b' gets evicted.
	_ = c.Set(ctx, "c", []byte("3"), time.Minute)

	// 'b' should still be there (was set after 'a').
	ok, _ := c.Exists(ctx, "b")
	if !ok {
		t.Error("Exists did promote 'a' — 'b' was evicted instead")
	}
}

func TestFlush(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = c.Set(ctx, string(rune('a'+i)), []byte{byte(i)}, time.Minute)
	}

	if err := c.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0 after Flush", c.Len())
	}
}

func TestClose(t *testing.T) {
	c := New()
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second Close should be safe (sync.Once).
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	// Close after use should still be safe.
}

func TestLen(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = c.Set(ctx, string(rune('a'+i)), []byte{byte(i)}, time.Minute)
	}
	if c.Len() != 5 {
		t.Errorf("Len = %d, want 5", c.Len())
	}
}

func TestSet_TTLZero_NeverExpires(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "forever", []byte("data"), 0)

	// Immediate Get should work.
	val, err := c.Get(ctx, "forever")
	if err != nil {
		t.Fatalf("Get immediately: %v", err)
	}
	if string(val) != "data" {
		t.Errorf("value = %q", string(val))
	}

	// After a short wait, should still be accessible.
	time.Sleep(20 * time.Millisecond)
	ok, err := c.Exists(ctx, "forever")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("TTL=0 entry should not expire")
	}
}

func TestLRU_Eviction(t *testing.T) {
	c := New(Config{Cap: 3})
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "a", []byte("1"), time.Minute)
	_ = c.Set(ctx, "b", []byte("2"), time.Minute)
	_ = c.Set(ctx, "c", []byte("3"), time.Minute)

	// Access 'a' so it becomes MRU.
	_, _ = c.Get(ctx, "a")

	// Add a 4th key → should evict LRU, which is 'b'.
	_ = c.Set(ctx, "d", []byte("4"), time.Minute)

	if c.Len() != 3 {
		t.Errorf("Len = %d, want 3", c.Len())
	}

	_, err := c.Get(ctx, "b")
	if err != cache.ErrCacheMiss {
		t.Errorf("Get(b) = %v, want ErrCacheMiss (should be evicted)", err)
	}

	// 'a' should survive (was accessed, promoted to MRU).
	val, err := c.Get(ctx, "a")
	if err != nil {
		t.Errorf("Get(a) after eviction: %v", err)
	}
	if string(val) != "1" {
		t.Errorf("Get(a) = %q, want 1", string(val))
	}
}

func TestLRU_Eviction_ExpiredFirst(t *testing.T) {
	c := New(Config{Cap: 2})
	defer c.Close()
	ctx := context.Background()

	// Insert two entries: one expired, one live.
	_ = c.Set(ctx, "live", []byte("L"), time.Hour)
	_ = c.Set(ctx, "dead", []byte("D"), 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	// Adding a new entry should evict the expired 'dead' before the live one.
	_ = c.Set(ctx, "new", []byte("N"), time.Hour)

	// 'live' should survive.
	ok, err := c.Exists(ctx, "live")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("live entry should survive — expired entry should be evicted first")
	}

	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2 (new + live)", c.Len())
	}
}

func TestConcurrent(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := string(rune('a' + (id % 10)))
			_ = c.Set(ctx, key, []byte{byte(id)}, time.Minute)
			_, _ = c.Get(ctx, key)
			_, _ = c.Exists(ctx, key)
		}(i)
	}
	wg.Wait()

	// After all goroutines, no internal corruption.
	if c.Len() > 10 {
		t.Errorf("Len = %d, want <= 10 (10 unique keys)", c.Len())
	}
}

func TestGet_DefensiveCopy(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	original := []byte("original")
	_ = c.Set(ctx, "k", original, time.Minute)

	val, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the returned value.
	val[0] = 'X'

	// Get again — should still be original.
	val2, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if string(val2) != "original" {
		t.Errorf("value = %q, want original (defensive copy was not made)", string(val2))
	}
}

func TestSet_DefensiveCopy(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	mutable := []byte("mutable")
	_ = c.Set(ctx, "k", mutable, time.Minute)

	// Mutate the original slice.
	mutable[0] = 'X'

	val, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "mutable" {
		t.Errorf("value = %q, want mutable (defensive copy was not made on Set)", string(val))
	}
}

func TestBackgroundCleanup(t *testing.T) {
	c := New(Config{CleanupInterval: 50 * time.Millisecond})
	defer c.Close()
	ctx := context.Background()

	// Insert entries that expire very quickly.
	for i := 0; i < 5; i++ {
		_ = c.Set(ctx, string(rune('a'+i)), []byte{byte(i)}, 10*time.Millisecond)
	}

	// Wait for expiry + cleanup.
	time.Sleep(150 * time.Millisecond)

	// Background cleanup should have removed expired entries.
	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0 (background cleanup)", c.Len())
	}
}

func TestLRU_EvictAllLive(t *testing.T) {
	c := New(Config{Cap: 2})
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "a", []byte("1"), time.Minute)
	_ = c.Set(ctx, "b", []byte("2"), time.Minute)
	// 'a' is LRU, but never accessed.

	_ = c.Set(ctx, "c", []byte("3"), time.Minute)

	// 'a' should be evicted (LRU).
	_, err := c.Get(ctx, "a")
	if err != cache.ErrCacheMiss {
		t.Errorf("Get(a) = %v, want ErrCacheMiss (evicted)", err)
	}
	_, err = c.Get(ctx, "b")
	if err != nil {
		t.Errorf("Get(b) = %v, want nil", err)
	}
}

func TestSet_RefreshTTL(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "k", []byte("v"), 10*time.Millisecond)
	// Refresh with a new TTL.
	_ = c.Set(ctx, "k", []byte("v"), time.Hour)

	time.Sleep(20 * time.Millisecond)

	_, err := c.Get(ctx, "k")
	if err != nil {
		t.Errorf("Get after refresh: %v, want nil (TTL was refreshed)", err)
	}
}

func TestExists_Expired(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "k", []byte("v"), 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	ok, err := c.Exists(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Exists = true, want false for expired key")
	}
	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0 (lazy eviction)", c.Len())
	}
}

func TestCap_Unlimited(t *testing.T) {
	c := New()
	defer c.Close()
	ctx := context.Background()

	// Insert many entries — unlimited should not evict anything.
	for i := 0; i < 100; i++ {
		_ = c.Set(ctx, string(rune(i)), []byte{byte(i)}, time.Minute)
	}
	if c.Len() != 100 {
		t.Errorf("Len = %d, want 100 (unlimited capacity)", c.Len())
	}
}
