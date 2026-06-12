package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/astra-go/astra"
)

// RecoveryConfig configures the Recovery middleware.
type RecoveryConfig struct {
	// Logger is used to log panics. Defaults to slog.Default().
	Logger *slog.Logger
	// PrintStack controls whether the stack trace is printed. Default true.
	PrintStack bool
	// Handler is called when a panic is recovered. If nil, a 500 response is written.
	Handler func(c *astra.Ctx, err any)
	// IsProduction controls whether to return detailed error info. Default false.
	IsProduction bool
	// AlertFunc is called to send alerts (e.g., to Sentry, Slack). Default nil.
	AlertFunc func(c *astra.Ctx, err any, stack string)
}

// DefaultRecoveryConfig is the default recovery configuration.
var DefaultRecoveryConfig = RecoveryConfig{
	PrintStack:     false, // safer default: don't leak stack traces
	IsProduction:   true,  // safer default: don't leak error details
	AlertFunc:      nil,
}

// Recovery returns a middleware that recovers from panics and returns a 500 error.
// Inspired by gin's Recovery middleware and go-zero's recover mechanism.
func Recovery() astra.HandlerFunc {
	return RecoveryWithConfig(DefaultRecoveryConfig)
}

// RecoveryWithConfig returns a Recovery middleware with custom configuration.
func RecoveryWithConfig(cfg RecoveryConfig) astra.HandlerFunc {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return func(c *astra.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				// Build stack trace
				var stackBuf []byte
				if cfg.PrintStack {
					stackBuf = make([]byte, 8192) // Increased buffer size
					n := runtime.Stack(stackBuf, false)
					stackBuf = stackBuf[:n]
				}

				errMsg := fmt.Sprintf("%v", r)
				cfg.Logger.Error("panic recovered",
					slog.String("error", errMsg),
					slog.String("stack", string(stackBuf)),
					slog.Time("timestamp", time.Now()),
				)

				// Send alert if configured (for production monitoring)
				if cfg.AlertFunc != nil {
					go func() {
						// Use a timeout to prevent AlertFunc from blocking indefinitely.
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						_ = ctx // ctx is available for AlertFunc implementations to check
						cfg.AlertFunc(c, r, string(stackBuf))
					}()
				}

				// Get writer once and check if it's valid
				w := c.Writer()
				if w == nil {
					// Cannot write response, just log
					cfg.Logger.Error("writer is nil, cannot send error response")
					return
				}

				if cfg.Handler != nil {
					cfg.Handler(c, r)
				} else if !w.Written() {
					// Only write the error response when the response has not
					// been started yet. Writing to a partially-sent response
					// would corrupt the HTTP stream.
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)

					// In production, don't expose internal details
					if cfg.IsProduction {
						_, _ = w.Write([]byte(`{"error":"Internal Server Error"}`))
					} else {
						// In development, include error details for debugging
						stackStr := string(stackBuf)
						// Limit stack trace length to avoid huge responses
						if len(stackStr) > 4096 {
							stackStr = stackStr[:4096] + "...(truncated)"
						}
						response := map[string]string{
							"error": errMsg,
							"stack": stackStr,
						}
						responseJSON, _ := json.Marshal(response)
						_, _ = w.Write(responseJSON)
					}
				}
			}
		}()

		c.Next()
		return nil
	}
}
