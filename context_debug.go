//go:build astra_debug

package astra

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

// debugGoroutineID extracts the goroutine ID from the runtime stack trace.
// This is only used in debug builds to detect concurrent access to Ctx.
func debugGoroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Stack trace format: "goroutine 123 [running]:\n..."
	// Extract the numeric ID between "goroutine " and " [".
	idField := strings.Fields(string(buf[:n]))[1]
	id, _ := strconv.ParseInt(idField, 10, 64)
	return id
}

// debugCheckConcurrency verifies that the context is accessed from the same
// goroutine that owns it. If concurrent access is detected, it panics with
// a descriptive error message.
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
			"astra: concurrent Ctx access detected\n"+
				"  Owner goroutine: %d\n"+
				"  Current goroutine: %d\n"+
				"  Ctx is not goroutine-safe. Use c.Clone() to pass to goroutines.\n"+
				"  See: https://github.com/your-org/astra/blob/main/docs/concurrency.md",
			ownerGID, currentGID,
		))
	}
}

// debugReset clears the goroutine ID when the context is recycled.
// This allows the context to be reused by a different goroutine after
// being returned to the pool.
func (c *Ctx) debugReset() {
	atomic.StoreInt64(&c.goroutineID, 0)
}
