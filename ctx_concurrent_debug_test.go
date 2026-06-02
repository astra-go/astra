//go:build astra_debug

package astra

import (
	"net/http/httptest"
	"sync"
	"testing"
)

func TestDebugConcurrentAccessPanic(t *testing.T) {
	app := New()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)

	// 通过 ServeHTTP 获取一个真实的 Ctx（从 pool 里取）
	var c *Ctx
	app.ServeHTTP(w, r)
	// 上面那行会调用 pool 里的 Ctx.reset()，但我们需要直接拿到 Ctx
	// 改用直接构造的方式
	c = &Ctx{app: app}
	c.reset(w, r)
	c.debugReset() // 模拟刚从 pool 取出，清空 goroutineID

	var wg sync.WaitGroup
	wg.Add(1)

	// 主 goroutine 先访问，记录 ownerGID
	c.Set("key", "value")

	// 另一个 goroutine 访问同一 Ctx，应该 panic
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				t.Logf("✓ panic detected (expected):\n%s", r)
			} else {
				t.Errorf("✗ expected panic for concurrent Ctx access, but none occurred")
			}
		}()
		c.Get("key")
	}()

	wg.Wait()
}

func TestDebugSameGoroutineOK(t *testing.T) {
	app := New()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	c := &Ctx{app: app}
	c.reset(w, r)
	c.debugReset()

	// 同一个 goroutine 多次访问，不应该 panic
	c.Set("key", "v1")
	v, ok := c.Get("key")
	if !ok || v != "v1" {
		t.Fatalf("unexpected value: %v, %v", v, ok)
	}
	c.Set("key", "v2")
	v, ok = c.Get("key")
	if !ok || v != "v2" {
		t.Fatalf("unexpected value after update: %v, %v", v, ok)
	}
	t.Log("✓ same-goroutine access OK (no panic)")
}

func TestDebugCloneResetsGoroutine(t *testing.T) {
	app := New()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	c := &Ctx{app: app}
	c.reset(w, r)
	c.debugReset()
	c.Set("key", "value")

	// Clone 出来的 Ctx 应该可以在新 goroutine 使用
	cc := c.Clone()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 不应该 panic，因为 cc 是新的 Ctx，goroutineID 已被 reset
		v, ok := cc.Get("key")
		if !ok || v != "value" {
			t.Errorf("unexpected cloned value: %v, %v", v, ok)
		}
	}()
	wg.Wait()
	t.Log("✓ Clone() works in new goroutine (no panic)")
}
