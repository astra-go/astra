// Package health provides automatic /health, /ready, and /live endpoint
// registration for Astra applications.
//
// The three endpoints follow the Kubernetes probe convention:
//
//   - /live   — liveness:  is the process alive? Always returns 200 unless the
//               app is shutting down. Kubernetes restarts pods that fail liveness.
//   - /ready  — readiness: are all dependencies reachable? Returns 200 only when
//               every registered Probe passes. Kubernetes stops routing traffic
//               to pods that fail readiness.
//   - /health — combined:  aggregates live + ready status in a single JSON response.
//
// # Usage
//
//	import "github.com/astra-go/astra/health"
//
//	health.Register(app)   // /health, /ready, /live with no probes
//
//	// With dependency probes
//	health.Register(app,
//	    health.WithProbe("db",    orm.DBProbe(db)),
//	    health.WithProbe("redis", health.RedisProbe(rdb)),
//	    health.WithPrefix("/internal"),   // → /internal/health etc.
//	)
package health

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/astra-go/astra"
)

// ProbeFunc is a function that checks the health of a dependency.
// It returns nil when healthy and an error describing the failure otherwise.
type ProbeFunc func(ctx context.Context) error

// named pairs a probe with its display name.
type named struct {
	name  string
	probe ProbeFunc
}

// options holds the configuration for Register.
type options struct {
	prefix string
	probes []named
}

// Option configures health endpoint registration.
type Option func(*options)

// WithPrefix overrides the URL prefix (default: "").
// Example: WithPrefix("/internal") → /internal/health, /internal/ready, /internal/live
func WithPrefix(prefix string) Option {
	return func(o *options) { o.prefix = prefix }
}

// WithProbe adds a named dependency probe to the readiness check.
func WithProbe(name string, probe ProbeFunc) Option {
	return func(o *options) {
		o.probes = append(o.probes, named{name: name, probe: probe})
	}
}

// Register mounts /health, /ready, and /live on the given router.
// The router parameter is typically *astra.App, which satisfies astra.RouteRegistrar.
func Register(app astra.RouteRegistrar, opts ...Option) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	h := &handler{probes: o.probes}

	app.GET(o.prefix+"/live", h.live)
	app.GET(o.prefix+"/ready", h.ready)
	app.GET(o.prefix+"/health", h.health)
}

// handler holds the registered probes and serves the health endpoints.
type handler struct {
	probes []named
}

// live always returns 200 — it only signals that the Go process is alive.
func (h *handler) live(c *astra.Ctx) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ready runs all probes and returns 200 only if every probe passes.
func (h *handler) ready(c *astra.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	details, ok := h.runProbes(ctx)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"status":  "degraded",
			"details": details,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status":  "ok",
		"details": details,
	})
}

// health is a combined liveness + readiness response.
func (h *handler) health(c *astra.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	details, ready := h.runProbes(ctx)
	status := "ok"
	code := http.StatusOK
	if !ready {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	return c.JSON(code, map[string]any{
		"status":  status,
		"live":    true,
		"ready":   ready,
		"details": details,
	})
}

// runProbes executes all probes concurrently.
// Returns a map of probe name → "ok"|error message, and whether all passed.
func (h *handler) runProbes(ctx context.Context) (map[string]string, bool) {
	if len(h.probes) == 0 {
		return map[string]string{}, true
	}

	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(h.probes))

	var wg sync.WaitGroup
	for _, p := range h.probes {
		wg.Add(1)
		go func(n named) {
			defer wg.Done()
			results <- result{name: n.name, err: n.probe(ctx)}
		}(p)
	}
	wg.Wait()
	close(results)

	details := make(map[string]string, len(h.probes))
	allOK := true
	for r := range results {
		if r.err != nil {
			details[r.name] = r.err.Error()
			allOK = false
		} else {
			details[r.name] = "ok"
		}
	}
	return details, allOK
}
