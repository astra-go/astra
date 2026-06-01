//go:build astra_debug

package astra

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestCtx_ConcurrentAccess_Panics verifies that concurrent access to Ctx
// from multiple goroutines is detected and panics in debug mode.
func TestCtx_ConcurrentAccess_Panics(t *testing.T) {
	app := New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	ctx := app.allocateContext()
	ctx.reset(w, req)
	defer app.pool.Put(ctx)

	// First access from the main goroutine should succeed.
	ctx.Set("key1", "value1")

	// Concurrent access from another goroutine should panic.
	var wg sync.WaitGroup
	wg.Add(1)

	panicCaught := false
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panicCaught = true
				t.Logf("Expected panic caught: %v", r)
			}
		}()
		// This should panic because it's a different goroutine.
		ctx.Set("key2", "value2")
	}()

	wg.Wait()

	if !panicCaught {
		t.Fatal("Expected panic from concurrent Ctx access, but none occurred")
	}
}

// TestCtx_ConcurrentGet_Panics verifies that concurrent Get operations
// are also detected.
func TestCtx_ConcurrentGet_Panics(t *testing.T) {
	app := New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	ctx := app.allocateContext()
	ctx.reset(w, req)
	defer app.pool.Put(ctx)

	// Set a value from the main goroutine.
	ctx.Set("key", "value")

	// Concurrent Get from another goroutine should panic.
	var wg sync.WaitGroup
	wg.Add(1)

	panicCaught := false
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panicCaught = true
				t.Logf("Expected panic caught: %v", r)
			}
		}()
		// This should panic because it's a different goroutine.
		ctx.Get("key")
	}()

	wg.Wait()

	if !panicCaught {
		t.Fatal("Expected panic from concurrent Ctx.Get, but none occurred")
	}
}

// TestCtx_Clone_AllowsConcurrentAccess verifies that cloned contexts
// can be safely used in goroutines without triggering the concurrency check.
func TestCtx_Clone_AllowsConcurrentAccess(t *testing.T) {
	app := New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	ctx := app.allocateContext()
	ctx.reset(w, req)
	defer app.pool.Put(ctx)

	// Set a value in the original context.
	ctx.Set("original", "value")

	// Clone the context for use in a goroutine.
	clone := ctx.Clone()

	var wg sync.WaitGroup
	wg.Add(1)

	panicCaught := false
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panicCaught = true
				t.Errorf("Unexpected panic from cloned Ctx: %v", r)
			}
		}()
		// This should NOT panic because clone is a separate context.
		clone.Set("goroutine", "value")
		if v := clone.GetString("goroutine"); v != "value" {
			t.Errorf("Expected 'value', got %q", v)
		}
	}()

	wg.Wait()

	if panicCaught {
		t.Fatal("Cloned context should not panic on concurrent access")
	}

	// Original context should still be accessible from the main goroutine.
	if v := ctx.GetString("original"); v != "value" {
		t.Errorf("Expected 'value', got %q", v)
	}
}

// TestCtx_SameGoroutine_NoPanic verifies that repeated access from the
// same goroutine does not trigger the concurrency check.
func TestCtx_SameGoroutine_NoPanic(t *testing.T) {
	app := New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	ctx := app.allocateContext()
	ctx.reset(w, req)
	defer app.pool.Put(ctx)

	// Multiple accesses from the same goroutine should not panic.
	for i := 0; i < 100; i++ {
		ctx.Set("key", i)
		if v := ctx.GetInt("key"); v != i {
			t.Errorf("Expected %d, got %d", i, v)
		}
	}
}

// TestCtx_Reset_ClearsGoroutineID verifies that reset() clears the
// goroutine ID so the context can be reused by a different goroutine
// after being returned to the pool.
func TestCtx_Reset_ClearsGoroutineID(t *testing.T) {
	app := New()

	// First request in goroutine 1 (main).
	req1 := httptest.NewRequest(http.MethodGet, "/test1", nil)
	w1 := httptest.NewRecorder()
	ctx := app.allocateContext()
	ctx.reset(w1, req1)
	ctx.Set("key1", "value1")
	app.pool.Put(ctx)

	// Simulate a second request in a different goroutine.
	var wg sync.WaitGroup
	wg.Add(1)

	panicCaught := false
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panicCaught = true
				t.Errorf("Unexpected panic after reset: %v", r)
			}
		}()

		req2 := httptest.NewRequest(http.MethodGet, "/test2", nil)
		w2 := httptest.NewRecorder()
		// This should reuse the same context from the pool.
		ctx2 := app.pool.Get().(*Ctx)
		ctx2.reset(w2, req2)
		defer app.pool.Put(ctx2)

		// This should NOT panic because reset() cleared the goroutine ID.
		ctx2.Set("key2", "value2")
		if v := ctx2.GetString("key2"); v != "value2" {
			t.Errorf("Expected 'value2', got %q", v)
		}
	}()

	wg.Wait()

	if panicCaught {
		t.Fatal("Context should be reusable after reset")
	}
}
