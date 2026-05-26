package astra_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/astra-go/astra"
)

// captureCtx runs a handler and returns the *Ctx seen inside it.
func captureCtx(t *testing.T, setup func(*astra.App)) *astra.Ctx {
	t.Helper()
	var captured *astra.Ctx
	app := astra.New()
	setup(app)
	// We need to capture the Ctx; use a channel to pass it out.
	// The actual capture is done inside the setup closure.
	_ = captured
	return nil
}

// ─── Clone: KV store isolation ───────────────────────────────────────────────

func TestCtx_Clone_KVStoreIsolated(t *testing.T) {
	app := astra.New()
	var cloneVal any
	app.GET("/clone-kv", func(c *astra.Ctx) error {
		c.Set("key", "original")
		clone := c.Clone()

		// Write to clone must not affect original.
		clone.Set("key", "cloned")

		origVal, _ := c.Get("key")
		cloneVal, _ = clone.Get("key")

		if origVal != "original" {
			t.Errorf("original ctx: want 'original', got %v", origVal)
		}
		return c.String(http.StatusOK, "%v", cloneVal)
	})

	req := httptest.NewRequest(http.MethodGet, "/clone-kv", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if w.Body.String() != "cloned" {
		t.Errorf("clone KV: want 'cloned', got %s", w.Body.String())
	}
}

// ─── Clone: original KV write does not affect clone ──────────────────────────

func TestCtx_Clone_OriginalWriteDoesNotAffectClone(t *testing.T) {
	app := astra.New()
	app.GET("/clone-orig", func(c *astra.Ctx) error {
		c.Set("key", "before")
		clone := c.Clone()

		// Mutate original after cloning.
		c.Set("key", "after")

		v, _ := clone.Get("key")
		if v != "before" {
			t.Errorf("clone should see 'before', got %v", v)
		}
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/clone-orig", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// ─── Clone: routeKey and params are inherited ─────────────────────────────────

func TestCtx_Clone_InheritsRouteKeyAndParams(t *testing.T) {
	app := astra.New()
	app.GET("/users/:id", func(c *astra.Ctx) error {
		clone := c.Clone()

		if clone.Param("id") != c.Param("id") {
			t.Errorf("clone Param: want %q, got %q", c.Param("id"), clone.Param("id"))
		}
		origRoute, _ := c.Get(astra.RouteKey)
		cloneRoute, _ := clone.Get(astra.RouteKey)
		if origRoute != cloneRoute {
			t.Errorf("clone RouteKey: want %v, got %v", origRoute, cloneRoute)
		}
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// ─── Clone: nop writer — response methods must not panic ─────────────────────

func TestCtx_Clone_NopWriterDoesNotPanic(t *testing.T) {
	app := astra.New()
	app.GET("/clone-nop", func(c *astra.Ctx) error {
		clone := c.Clone()
		// These must not panic or write to the real response.
		_ = clone.JSON(http.StatusOK, map[string]string{"x": "y"})
		_ = clone.String(http.StatusOK, "hello")
		_ = clone.NoContent(http.StatusNoContent)
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/clone-nop", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("real response should be 'ok', got %s", w.Body.String())
	}
}

// ─── Clone: goroutine concurrent read — race detector must be silent ──────────

func TestCtx_Clone_GoroutineConcurrentRead(t *testing.T) {
	app := astra.New()
	app.GET("/clone-race", func(c *astra.Ctx) error {
		c.Set("user", "alice")
		c.Set("role", "admin")
		clone := c.Clone()

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Concurrent reads on the clone — must not race.
				_ = clone.GetString("user")
				_ = clone.GetString("role")
				_ = clone.Param("id")
			}()
		}
		wg.Wait()
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/clone-race", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// ─── CloneWithContext: Done() reflects the provided context ───────────────────

func TestCtx_CloneWithContext_DoneReflectsCtx(t *testing.T) {
	app := astra.New()
	app.GET("/clone-ctx", func(c *astra.Ctx) error {
		ctx, cancel := context.WithCancel(context.Background())
		clone := c.CloneWithContext(ctx)

		cancel()

		select {
		case <-clone.Request().Context().Done():
			// expected
		default:
			t.Error("clone request context should be cancelled")
		}
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/clone-ctx", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// ─── Clone: isClone flag is set ──────────────────────────────────────────────

func TestCtx_Clone_IsCloneFlagSet(t *testing.T) {
	app := astra.New()
	app.GET("/clone-flag", func(c *astra.Ctx) error {
		clone := c.Clone()
		if !clone.IsClone() {
			t.Error("clone.IsClone() should be true")
		}
		if c.IsClone() {
			t.Error("original.IsClone() should be false")
		}
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/clone-flag", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// ─── Clone: CloneWithContext also sets isClone flag ───────────────────────────

func TestCtx_CloneWithContext_IsCloneFlagSet(t *testing.T) {
	app := astra.New()
	app.GET("/clone-ctx-flag", func(c *astra.Ctx) error {
		clone := c.CloneWithContext(context.Background())
		if !clone.IsClone() {
			t.Error("CloneWithContext result: IsClone() should be true")
		}
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/clone-ctx-flag", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}


func TestCtx_Clone_OriginalUnaffectedAfterGoroutine(t *testing.T) {
	app := astra.New()
	app.GET("/clone-lifecycle", func(c *astra.Ctx) error {
		c.Set("token", "secret")
		clone := c.Clone()

		done := make(chan struct{})
		go func() {
			defer close(done)
			// Goroutine writes to its own clone — must not affect original.
			clone.Set("token", "goroutine-value")
		}()
		<-done

		// Original must still hold its value.
		if v := c.GetString("token"); v != "secret" {
			t.Errorf("original token: want 'secret', got %q", v)
		}
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/clone-lifecycle", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}
