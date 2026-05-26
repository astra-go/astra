package astra

import (
	"bytes"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// TestJsonBufPool_OversizedBufferNotReturned verifies that a bytes.Buffer whose
// cap has grown beyond jsonBufMaxCap is NOT returned to jsonBufPool after JSON()
// completes, preventing large backing arrays from accumulating in the pool.
func TestJsonBufPool_OversizedBufferNotReturned(t *testing.T) {
	// GC clears all sync.Pool contents, giving us a clean slate.
	runtime.GC()

	// Seed pool with a pre-expanded buffer whose cap exceeds the threshold.
	big := bytes.NewBuffer(make([]byte, 0, JsonBufMaxCap+1))
	JsonBufPool.Put(big)

	app := New()
	app.GET("/", func(c *Ctx) error {
		return c.JSON(200, Map{"ok": true})
	})

	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// GC again so that only items actually held by the pool survive.
	runtime.GC()

	// If the oversized buffer was mistakenly returned, Get() would hand it back.
	// If it was correctly discarded, Get() calls New() and returns a fresh
	// zero-cap buffer.
	got := JsonBufPool.Get()
	if b, ok := got.(*bytes.Buffer); ok && b.Cap() > JsonBufMaxCap {
		t.Errorf("oversized buffer (cap=%d) was returned to pool; want cap <= %d",
			b.Cap(), JsonBufMaxCap)
	}
}

// TestJsonBufPool_NormalBufferReturned verifies that a buffer within the cap
// limit IS returned to the pool and reused, preserving the pool's purpose.
func TestJsonBufPool_NormalBufferReturned(t *testing.T) {
	app := New()
	app.GET("/", func(c *Ctx) error {
		return c.JSON(200, Map{"ok": true})
	})

	// Two back-to-back requests; both should produce correct responses whether
	// the second reuses the pooled buffer or allocates a fresh one.
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

		if w.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
		if !strings.Contains(w.Body.String(), `"ok"`) {
			t.Errorf("request %d: unexpected body: %s", i+1, w.Body.String())
		}
	}
}
