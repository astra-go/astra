package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime"

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
}

// DefaultRecoveryConfig is the default recovery configuration.
var DefaultRecoveryConfig = RecoveryConfig{
	PrintStack: true,
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
					stackBuf = make([]byte, 4096)
					n := runtime.Stack(stackBuf, false)
					stackBuf = stackBuf[:n]
				}

				errMsg := fmt.Sprintf("%v", r)
				cfg.Logger.Error("panic recovered",
					slog.String("error", errMsg),
					slog.String("stack", string(stackBuf)),
				)

				if cfg.Handler != nil {
					cfg.Handler(c, r)
				} else if !c.Writer().Written() {
					// Only write the error response when the response has not
					// been started yet. Writing to a partially-sent response
					// would corrupt the HTTP stream.
					c.Writer().Header().Set("Content-Type", "application/json; charset=utf-8")
					c.Writer().WriteHeader(http.StatusInternalServerError)
					_, _ = c.Writer().Write([]byte(`{"error":"Internal Server Error"}`))
				}
			}
		}()

		c.Next()
		return nil
	}
}
