package astra

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"
)

// builtinRecovery is a lightweight panic-recovery middleware used internally
// by New() when EnableRecovery is true (the default).  It is intentionally
// minimal — for advanced features (alert hooks, custom handlers, stack
// filtering), use middleware.Recovery() from the middleware sub-package
// instead (with WithRecovery(false) to disable the built-in one).
func builtinRecovery() HandlerFunc {
	return func(c *Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				stackBuf := make([]byte, 8192)
				n := runtime.Stack(stackBuf, false)
				stackBuf = stackBuf[:n]

				errMsg := fmt.Sprintf("%v", r)
				slog.Error("panic recovered",
					slog.String("error", errMsg),
					slog.String("stack", string(stackBuf)),
					slog.Time("timestamp", time.Now()),
				)

				c.Abort()

				w := c.Writer()
				if w != nil && !w.Written() {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					resp := map[string]string{"error": "Internal Server Error"}
					respJSON, _ := json.Marshal(resp)
					_, _ = w.Write(respJSON)
				}
			}
		}()
		// Call c.Next() to execute the rest of the handler chain.
		// Next() now returns the first error from any handler in the chain.
		// By returning that error, we propagate it to the outer Next() loop
		// (started by router.Handle), which will stop iterating.
		return c.Next()
	}
}
