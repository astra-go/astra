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
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/astra-go/astra"
)

// startTime records when the process booted; used in /health uptime.
var startTime = time.Now()

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
	prefix    string
	probes     []named
	startup    []named
	timeout    time.Duration
	version    string
	buildInfo  map[string]string
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

// WithStartupProbe adds a startup probe. K8s uses startup probes for
// slow-initializing applications.
func WithStartupProbe(name string, probe ProbeFunc) Option {
	return func(o *options) {
		o.startup = append(o.startup, named{name: name, probe: probe})
	}
}

// WithTimeout sets the per-probe timeout (default: 5s).
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithVersion sets the application version reported in /health.
func WithVersion(v string) Option {
	return func(o *options) { o.version = v }
}

// WithBuildInfo adds arbitrary key-value pairs to the /health response.
func WithBuildInfo(k, v string) Option {
	return func(o *options) {
		if o.buildInfo == nil {
			o.buildInfo = make(map[string]string)
		}
		o.buildInfo[k] = v
	}
}

// Register mounts /health, /ready, and /live on the given router.
// The router parameter is typically *astra.App, which satisfies astra.RouteRegistrar.
func Register(app astra.RouteRegistrar, opts ...Option) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	h := &handler{probes: o.probes, timeout: o.timeout, version: o.version, buildInfo: o.buildInfo}

	if len(o.startup) > 0 {
		sh := &startupHandler{probes: o.startup, maxFailures: 30}
		app.GET(o.prefix+"/startup", sh.startup)
	}

	app.GET(o.prefix+"/live", h.live)
	app.GET(o.prefix+"/ready", h.ready)
	app.GET(o.prefix+"/health", h.health)
}

// handler holds the registered probes and serves the health endpoints.
type handler struct {
	probes    []named
	timeout   time.Duration
	version   string
	buildInfo map[string]string
}

type startupHandler struct {
	mu         sync.Mutex
	probes     []named
	maxFailures int
	failures   map[string]int64
	started    map[string]bool
}

// live always returns 200 — it only signals that the Go process is alive.
func (h *handler) live(c *astra.Ctx) error {
	resp := map[string]any{"status": "ok"}
	if h.version != "" {
		resp["version"] = h.version
	}
	return c.JSON(http.StatusOK, resp)
}

// ready runs all probes and returns 200 only if every probe passes.
func (h *handler) ready(c *astra.Ctx) error {
	t := h.timeout
	if t == 0 {
		t = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), t)
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
	t := h.timeout
	if t == 0 {
		t = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), t)
	defer cancel()

	details, ready := h.runProbes(ctx)
	status := "ok"
	code := http.StatusOK
	if !ready {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	resp := map[string]any{
		"status":  status,
		"live":    true,
		"ready":   ready,
		"details": details,
		"uptime":  time.Since(startTime).Round(time.Second).String(),
	}
	if h.version != "" {
		resp["version"] = h.version
	}
	if len(h.buildInfo) > 0 {
		resp["build"] = h.buildInfo
	}
	return c.JSON(code, resp)
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

// startup runs the startup probe. Returns 200 when started, 503 when initializing.
func (sh *startupHandler) startup(c *astra.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	sh.mu.Lock()
	if sh.failures == nil {
		sh.failures = make(map[string]int64)
		sh.started = make(map[string]bool)
	}
	sh.mu.Unlock()

	details := make(map[string]string, len(sh.probes))
	allStarted := true

	for _, p := range sh.probes {
		sh.mu.Lock()
		if sh.started[p.name] {
			sh.mu.Unlock()
			details[p.name] = "started"
			continue
		}
		sh.mu.Unlock()

		err := p.probe(ctx)
		sh.mu.Lock()
		if err == nil {
			sh.started[p.name] = true
			details[p.name] = "started"
		} else {
			sh.failures[p.name]++
			fails := sh.failures[p.name]
			allStarted = false
			if fails >= int64(sh.maxFailures) {
				details[p.name] = fmt.Sprintf("failed: %s (%d/%d)", err.Error(), fails, sh.maxFailures)
			} else {
				details[p.name] = fmt.Sprintf("initializing (%d/%d): %s", fails, sh.maxFailures, err.Error())
			}
		}
		sh.mu.Unlock()
	}

	if allStarted {
		return c.JSON(http.StatusOK, map[string]any{"status": "ok", "details": details})
	}
	return c.JSON(http.StatusServiceUnavailable, map[string]any{"status": "initializing", "details": details})
}

// GolangStats returns a ProbeFunc that checks Go runtime stats.
// Fails if goroutine count exceeds maxGoroutines or memory exceeds maxMemoryMB.
func GolangStats(maxGoroutines int, maxMemoryMB uint64) ProbeFunc {
	return func(ctx context.Context) error {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		g := runtime.NumGoroutine()
		if maxGoroutines > 0 && g > maxGoroutines {
			return fmt.Errorf("goroutines: %d > %d", g, maxGoroutines)
		}
		memMB := m.HeapAlloc / 1024 / 1024
		if maxMemoryMB > 0 && memMB > maxMemoryMB {
			return fmt.Errorf("heap: %dMB > %dMB", memMB, maxMemoryMB)
		}
		return nil
	}
}
