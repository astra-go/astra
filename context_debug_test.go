//go:build astra_debug

package astra

import (
	"sync"
	"testing"
)

// TestDebugConcurrentAccess verifies that concurrent access to Ctx is detected
// in debug builds. This test only runs when the astra_debug build tag is set.
func TestDebugConcurrentAccess(t *testing.T) {
	app := New()
	app.GET("/test", func(c *Ctx) error {
		return c.String(200, "ok")
	})

	c := app.allocateContext()
	c.reset(nil, nil)

	// First access from main goroutine should succeed.
	c.Set("key1", "value1")

	// Concurrent access from another goroutine should panic.
	var wg sync.WaitGroup
	wg.Add(1)

	panicChan := make(chan bool, 1)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panicChan <- true
			} else {
				panicChan <- false
			}
		}()
		// This should panic because it's a different goroutine.
		c.Set("key2", "value2")
	}()

	wg.Wait()
	close(panicChan)

	if !<-panicChan {
		t.Fatal("expected panic on concurrent Ctx access, got none")
	}
}

// TestDebugConcurrentGet verifies that concurrent Get() is also detected.
func TestDebugConcurrentGet(t *testing.T) {
	app := New()
	c := app.allocateContext()
	c.reset(nil, nil)

	// Set from main goroutine.
	c.Set("key", "value")

	// Concurrent Get from another goroutine should panic.
	var wg sync.WaitGroup
	wg.Add(1)

	panicChan := make(chan bool, 1)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panicChan <- true
			} else {
				panicChan <- false
			}
		}()
		// This should panic.
		c.Get("key")
	}()

	wg.Wait()
	close(panicChan)

	if !<-panicChan {
		t.Fatal("expected panic on concurrent Ctx.Get(), got none")
	}
}

// TestDebugCloneIsSafe verifies that Clone() creates a goroutine-safe copy.
func TestDebugCloneIsSafe(t *testing.T) {
	app := New()
	c := app.allocateContext()
	c.reset(nil, nil)

	// Set from main goroutine.
	c.Set("key", "value")

	// Clone should be safe to use in another goroutine.
	clone := c.Clone()

	var wg sync.WaitGroup
	wg.Add(1)

	errChan := make(chan error, 1)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				// Unexpected panic
				errChan <- r.(error)
			}
		}()
		// This should NOT panic because clone is independent.
		clone.Set("key2", "value2")
		v, ok := clone.Get("key")
		if !ok || v != "value" {
			t.Errorf("expected cloned value, got %v, %v", v, ok)
		}
		errChan <- nil
	}()

	wg.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		t.Fatalf("unexpected panic in cloned context: %v", err)
	}
}

// TestDebugResetAllowsReuse verifies that reset() clears the goroutine ID,
// allowing the context to be reused by a different goroutine.
func TestDebugResetAllowsReuse(t *testing.T) {
	app := New()
	c := app.allocateContext()

	// First use in goroutine 1.
	c.reset(nil, nil)
	c.Set("key1", "value1")

	// Reset should clear the goroutine ID.
	c.reset(nil, nil)

	// Now use in a different goroutine should succeed.
	var wg sync.WaitGroup
	wg.Add(1)

	errChan := make(chan error, 1)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				// Unexpected panic
				errChan <- r.(error)
			}
		}()
		// This should NOT panic because reset() cleared the owner.
		c.Set("key2", "value2")
		errChan <- nil
	}()

	wg.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		t.Fatalf("unexpected panic after reset: %v", err)
	}
}

// TestDebugResponseMethodsConcurrency verifies that response methods
// also detect concurrent access.
func TestDebugResponseMethodsConcurrency(t *testing.T) {
	app := New()
	c := app.allocateContext()
	c.reset(nil, nil)

	// First access from main goroutine.
	c.Set("init", "done")

	// Concurrent response method should panic.
	var wg sync.WaitGroup
	wg.Add(1)

	panicChan := make(chan bool, 1)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panicChan <- true
			} else {
				panicChan <- false
			}
		}()
		// This should panic.
		c.SetHeader("X-Test", "value")
	}()

	wg.Wait()
	close(panicChan)

	if !<-panicChan {
		t.Fatal("expected panic on concurrent response method, got none")
	}
}
