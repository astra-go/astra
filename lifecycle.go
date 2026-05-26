package astra

import (
	"context"
	"log/slog"
	"sync"
)

// Lifecycle manages the ordered start and stop hook chains for an App.
//
// Hooks registered via OnStart run sequentially before the server begins
// accepting connections; a hook error aborts startup. Hooks registered via
// OnStop run sequentially during graceful shutdown; stop-hook errors are
// tolerated (logged but not returned) so that all hooks always get a chance
// to clean up.
//
// Extracting this concern from App makes it independently testable: callers
// can create a Lifecycle, register hooks, and call RunStartHooks / RunStopHooks
// without starting an HTTP server.
type Lifecycle struct {
	startHooks []func(context.Context) error
	stopHooks  []func(context.Context) error
	mu         sync.RWMutex
}

// OnStart registers a hook that is called before the server starts accepting
// connections. Hooks run in registration order. If a hook returns an error the
// server does not start and the error is propagated to the caller of Run.
func (l *Lifecycle) OnStart(fn func(context.Context) error) {
	l.mu.Lock()
	l.startHooks = append(l.startHooks, fn)
	l.mu.Unlock()
}

// OnStop registers a hook that is called during graceful shutdown, after the
// server stops accepting new connections. Hooks run in reverse registration
// order (LIFO), mirroring di.Container.Stop semantics so that resources are
// released in the inverse order they were acquired. Errors are tolerated so
// that every registered hook runs.
func (l *Lifecycle) OnStop(fn func(context.Context) error) {
	l.mu.Lock()
	l.stopHooks = append(l.stopHooks, fn)
	l.mu.Unlock()
}

// RunStartHooks runs all registered start hooks in order.
// Returns the first error encountered; remaining hooks are not called.
func (l *Lifecycle) RunStartHooks(ctx context.Context) error {
	l.mu.RLock()
	hooks := l.startHooks
	l.mu.RUnlock()
	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			return err
		}
	}
	return nil
}

// RunStopHooks runs all registered stop hooks in reverse registration order
// (LIFO). Errors are logged but not returned so that every hook runs regardless
// of earlier failures.
func (l *Lifecycle) RunStopHooks(ctx context.Context) {
	l.mu.RLock()
	hooks := l.stopHooks
	l.mu.RUnlock()
	for i := len(hooks) - 1; i >= 0; i-- {
		if err := hooks[i](ctx); err != nil {
			slog.Error("stop hook failed", "err", err)
		}
	}
}
