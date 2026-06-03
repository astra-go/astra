package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/astra-go/astra/cache"
	"github.com/astra-go/astra/cache/memory"
	"github.com/astra-go/astra/testutil"
)

var ctx = context.Background()

// ─── MemoryCache ──────────────────────────────────────────────────────────────

func TestMemory_SetGet(t *testing.T) {
	c := memory.New()
	defer c.Close()

	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	val, err := c.Get(ctx, "k")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "v", string(val))
}

func TestMemory_Miss(t *testing.T) {
	c := memory.New()
	defer c.Close()

	_, err := c.Get(ctx, "missing")
	testutil.AssertError(t, err)
	testutil.AssertErrorIs(t, err, cache.ErrCacheMiss)
}

func TestMemory_Expiry(t *testing.T) {
	c := memory.New()
	defer c.Close()

	if err := c.Set(ctx, "ttl", []byte("x"), 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	_, err := c.Get(ctx, "ttl")
	testutil.AssertError(t, err)
	testutil.AssertErrorIs(t, err, cache.ErrCacheMiss)
}

func TestMemory_Exists(t *testing.T) {
	c := memory.New()
	defer c.Close()

	ok, err := c.Exists(ctx, "nokey")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, false, ok)

	_ = c.Set(ctx, "nokey", []byte("1"), time.Minute)
	ok, err = c.Exists(ctx, "nokey")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, ok)
}

func TestMemory_Delete(t *testing.T) {
	c := memory.New()
	defer c.Close()

	_ = c.Set(ctx, "a", []byte("1"), time.Minute)
	_ = c.Set(ctx, "b", []byte("2"), time.Minute)

	if err := c.Delete(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	_, err := c.Get(ctx, "a")
	testutil.AssertErrorIs(t, err, cache.ErrCacheMiss)
	_, err = c.Get(ctx, "b")
	testutil.AssertErrorIs(t, err, cache.ErrCacheMiss)
}

func TestMemory_Flush(t *testing.T) {
	c := memory.New()
	defer c.Close()

	_ = c.Set(ctx, "x", []byte("y"), time.Minute)
	if err := c.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := c.Get(ctx, "x")
	testutil.AssertErrorIs(t, err, cache.ErrCacheMiss)
}

func TestMemory_NoTTL(t *testing.T) {
	// TTL == 0 means the entry never expires
	c := memory.New()
	defer c.Close()

	_ = c.Set(ctx, "forever", []byte("yes"), 0)
	val, err := c.Get(ctx, "forever")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "yes", string(val))
}

func TestMemory_IsolatesValues(t *testing.T) {
	// Mutations to the returned slice must not corrupt the stored value
	c := memory.New()
	defer c.Close()

	original := []byte("hello")
	_ = c.Set(ctx, "safe", original, time.Minute)

	// Mutate the original slice after storing
	original[0] = 'X'

	val, _ := c.Get(ctx, "safe")
	testutil.AssertEqual(t, "hello", string(val))

	// Mutate the returned slice
	val[0] = 'Z'
	val2, _ := c.Get(ctx, "safe")
	testutil.AssertEqual(t, "hello", string(val2))
}

func TestMemory_Overwrite(t *testing.T) {
	c := memory.New()
	defer c.Close()

	_ = c.Set(ctx, "k", []byte("v1"), time.Minute)
	_ = c.Set(ctx, "k", []byte("v2"), time.Minute)
	val, _ := c.Get(ctx, "k")
	testutil.AssertEqual(t, "v2", string(val))
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

func TestGetSetJSON(t *testing.T) {
	c := memory.New()
	defer c.Close()

	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	p := payload{Name: "alice", Age: 30}
	if err := cache.SetJSON(ctx, c, "user", p, time.Minute); err != nil {
		t.Fatal(err)
	}

	var got payload
	if err := cache.GetJSON(ctx, c, "user", &got); err != nil {
		t.Fatal(err)
	}
	testutil.AssertEqual(t, p.Name, got.Name)
	testutil.AssertEqual(t, p.Age, got.Age)
}

func TestGetOrSet(t *testing.T) {
	c := memory.New()
	defer c.Close()

	calls := 0
	fn := func() (any, error) {
		calls++
		return "computed", nil
	}

	var result string
	err := cache.GetOrSet(ctx, c, "lazy", &result, time.Minute, fn)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "computed", result)
	testutil.AssertEqual(t, 1, calls)

	// Second call must hit the cache
	var result2 string
	err = cache.GetOrSet(ctx, c, "lazy", &result2, time.Minute, fn)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "computed", result2)
	testutil.AssertEqual(t, 1, calls) // not incremented
}

func TestGetOrSet_FnError(t *testing.T) {
	c := memory.New()
	defer c.Close()

	sentinel := errors.New("load failed")
	var result string
	err := cache.GetOrSet(ctx, c, "fail", &result, time.Minute, func() (any, error) {
		return nil, sentinel
	})
	testutil.AssertError(t, err)
	testutil.AssertErrorIs(t, err, sentinel)
}

// ─── MockCache (testutil) ─────────────────────────────────────────────────────

func TestMockCache_RecordsCalls(t *testing.T) {
	mc := testutil.NewMockCache()

	_ = mc.Set(ctx, "key1", []byte("v1"), time.Minute)
	_, _ = mc.Get(ctx, "key1")
	_, _ = mc.Get(ctx, "missing")
	_ = mc.Delete(ctx, "key1")

	calls := mc.Calls()
	if len(calls) != 4 {
		t.Fatalf("want 4 calls, got %d", len(calls))
	}
	testutil.AssertEqual(t, "Set", calls[0].Method)
	testutil.AssertEqual(t, "key1", calls[0].Key)
	testutil.AssertEqual(t, time.Minute, calls[0].TTL)

	testutil.AssertEqual(t, "Get", calls[1].Method)
	testutil.AssertEqual(t, nil, calls[1].Err)

	testutil.AssertEqual(t, "Get", calls[2].Method)
	testutil.AssertErrorIs(t, calls[2].Err, cache.ErrCacheMiss)

	testutil.AssertEqual(t, "Delete", calls[3].Method)
}

func TestMockCache_Reset(t *testing.T) {
	mc := testutil.NewMockCache()
	_ = mc.Set(ctx, "x", []byte("1"), time.Minute)
	mc.Reset(ctx)

	testutil.AssertEqual(t, 0, len(mc.Calls()))
	_, err := mc.Get(ctx, "x")
	testutil.AssertErrorIs(t, err, cache.ErrCacheMiss)
}
