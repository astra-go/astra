//go:build astra_debug

package astra

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

// debugGoroutineID returns the current goroutine ID by parsing the
// first line of a single-goroutine stack trace. This is portable
// across all Go implementations but allocates a small stack buffer.
//
// For lower overhead, callers can swap this implementation to use
// the github.com/petermattis/goid library (cgo-based, ~5ns vs ~1µs).
func debugGoroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Format: "goroutine 123 [running]:\n..."
	idField := strings.Fields(string(buf[:n]))[1]
	id, _ := strconv.ParseInt(idField, 10, 64)
	return id
}

// debugCheckConcurrency verifies that the context is accessed from the same
// goroutine that owns it. If concurrent access is detected, it panics with
// a descriptive error message including correct usage patterns.
//
// This function is only compiled when the astra_debug build tag is set.
// In production builds (without the tag), this function is a no-op and the
// goroutineID field is not present in Ctx, resulting in zero overhead.
func (c *Ctx) debugCheckConcurrency() {
	currentGID := debugGoroutineID()

	// First access: record the owner goroutine ID.
	if atomic.LoadInt64(&c.goroutineID) == 0 {
		atomic.StoreInt64(&c.goroutineID, currentGID)
		return
	}

	// Subsequent access: verify it's the same goroutine.
	ownerGID := atomic.LoadInt64(&c.goroutineID)
	if ownerGID != currentGID {
		panic(fmt.Sprintf(
			"\n"+
				"────────────────────────────────────────────────────────────────────\n"+
				"  astra: concurrent Ctx access detected (astra_debug mode)\n"+
				"────────────────────────────────────────────────────────────────────\n"+
				"  Owner goroutine:  %d\n"+
				"  Current goroutine: %d\n"+
				"\n"+
				"  Ctx is NOT goroutine-safe.\n"+
				"  The same *Ctx must never be used from multiple goroutines.\n"+
				"\n"+
				"  ✖ WRONG — sharing Ctx across goroutines:\n"+
				"      go func() {\n"+
				"          c.JSON(200, data)   // ❌ panic: concurrent access\n"+
				"      }()\n"+
				"\n"+
				"  ✔ RIGHT — use Clone() to create a goroutine-local copy:\n"+
				"      cc := c.Clone()\n"+
				"      go func() {\n"+
				"          cc.JSON(200, data)   // ✅ safe\n"+
				"      }()\n"+
				"\n"+
				"  ✔ RIGHT — copy values, not Ctx:\n"+
				"      val := c.Get(\"userID\")\n"+
				"      go func() {\n"+
				"          process(val)   // ✅ only pass values\n"+
				"      }()\n"+
				"\n"+
				"  ✔ RIGHT — CloneWithContext for cancellation:\n"+
				"      ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)\n"+
				"      defer cancel()\n"+
				"      cc := c.CloneWithContext(ctx)\n"+
				"      go cc.JSON(200, data)\n"+
				"\n"+
				"  Docs: https://github.com/your-org/astra/blob/main/docs/concurrency.md\n"+
				"────────────────────────────────────────────────────────────────────\n",
			ownerGID, currentGID,
		))
	}
}

// debugReset clears the goroutine ID when the context is recycled
// back to the pool. This allows the Ctx to be owned by a new
// goroutine after reset().
func (c *Ctx) debugReset() {
	atomic.StoreInt64(&c.goroutineID, 0)
}

// debugCheckGoroutineID is a no-op helper for use in tests.
// It returns the recorded owner goroutine ID (0 if not yet set).
func (c *Ctx) debugGetOwnerGoroutineID() int64 {
	return atomic.LoadInt64(&c.goroutineID)
}

// DebugBuild reports whether the astra_debug build tag was set.
// This is useful for tests that only run under debug builds.
func DebugBuild() bool {
	return true
}
