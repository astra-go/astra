package health

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ─── Advanced Probes ───────────────────────────────────────────────────────

// TCPProbe returns a ProbeFunc that establishes a TCP connection to host:port.
// Useful for checking database, cache, or message broker connectivity.
func TCPProbe(host string, port int) ProbeFunc {
	addr := fmt.Sprintf("%s:%d", host, port)
	return func(ctx context.Context) error {
		d := net.Dialer{Timeout: deadline(ctx, 3*time.Second)}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("tcp: %s: %w", addr, err)
		}
		conn.Close()
		return nil
	}
}

// DNSProbe returns a ProbeFunc that resolves the given hostname.
// Checks that the DNS system is working and the hostname is reachable.
func DNSProbe(hostname string) ProbeFunc {
	return func(ctx context.Context) error {
		r := &net.Resolver{}
		_, err := r.LookupHost(ctx, hostname)
		if err != nil {
			return fmt.Errorf("dns: %s: %w", hostname, err)
		}
		return nil
	}
}

// DiskProbe returns a ProbeFunc that checks available disk space at the given path.
// It fails if available bytes are below the minFree threshold.
func DiskProbe(path string, minFree uint64) ProbeFunc {
	return func(ctx context.Context) error {
		var stat unixFSStat
		if err := stat.statFS(path); err != nil {
			return fmt.Errorf("disk: %s: %w", path, err)
		}
		avail := stat.bavail() * stat.bsize()
		if avail < minFree {
			return fmt.Errorf("disk: %s: only %d bytes free (%d required)", path, avail, minFree)
		}
		return nil
	}
}

// MemoryProbe returns a ProbeFunc that checks system memory usage.
// Fails if available memory drops below minAvailable bytes.
func MemoryProbe(minAvailable uint64) ProbeFunc {
	return func(ctx context.Context) error {
		var m memoryStat
		avail := m.available()
		if avail < minAvailable {
			return fmt.Errorf("memory: %d bytes available (%d required)", avail, minAvailable)
		}
		return nil
	}
}

// CompositeProbe combines multiple probes into one. All probes run concurrently.
// Fails if any probe fails.
func CompositeProbe(name string, probes ...ProbeFunc) ProbeFunc {
	return func(ctx context.Context) error {
		errs := make([]error, len(probes))
		var wg sync.WaitGroup
		for i, p := range probes {
			wg.Add(1)
			go func(idx int, pf ProbeFunc) {
				defer wg.Done()
				errs[idx] = pf(ctx)
			}(i, p)
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// ─── Probe Throttler ──────────────────────────────────────────────────────

// ThrottledProbe wraps a ProbeFunc with caching. The probe is only executed
// once per cooldown period; subsequent calls return the cached result.
// This prevents hammering external dependencies when K8s probes fire every few seconds.
type ThrottledProbe struct {
	inner     ProbeFunc
	cooldown  time.Duration
	mu        sync.RWMutex
	lastRun   time.Time
	lastErr   error
	lastState atomic.Bool // true = healthy
}

// NewThrottledProbe creates a cached wrapper around the given probe.
// The probe is re-executed only after cooldown has elapsed since the last run.
func NewThrottledProbe(inner ProbeFunc, cooldown time.Duration) *ThrottledProbe {
	tp := &ThrottledProbe{
		inner:    inner,
		cooldown: cooldown,
	}
	tp.lastState.Store(true) // assume healthy initially
	return tp
}

// Run executes the probe, using the cached result if within cooldown.
func (tp *ThrottledProbe) Run(ctx context.Context) error {
	// Fast path: check cache
	tp.mu.RLock()
	if time.Since(tp.lastRun) < tp.cooldown {
		err := tp.lastErr
		tp.mu.RUnlock()
		return err
	}
	tp.mu.RUnlock()

	// Slow path: execute probe
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Double-check after acquiring write lock
	if time.Since(tp.lastRun) < tp.cooldown {
		return tp.lastErr
	}

	tp.lastErr = tp.inner(ctx)
	tp.lastRun = time.Now()
	tp.lastState.Store(tp.lastErr == nil)
	return tp.lastErr
}

// IsHealthy returns the cached health state without executing the probe.
func (tp *ThrottledProbe) IsHealthy() bool {
	return tp.lastState.Load()
}

// Probe returns a ProbeFunc for this throttled probe.
func (tp *ThrottledProbe) Probe() ProbeFunc {
	return tp.Run
}

// ─── Startup Probe ──────────────────────────────────────────────────────────

// StartupProbe manages a probe that tracks initial application readiness.
// It uses a failure counter that resets on success; if maxFailures consecutive
// failures occur before the first success, the probe reports unhealthy.
// After the first success, it always passes (normal readiness takes over).
type StartupProbe struct {
	inner       ProbeFunc
	maxFailures int
	failures    atomic.Int64
	started     atomic.Bool
}

// NewStartupProbe creates a startup probe that tolerates maxFailures consecutive
// failures before the first success. After the first success, it always passes.
func NewStartupProbe(inner ProbeFunc, maxFailures int) *StartupProbe {
	return &StartupProbe{
		inner:       inner,
		maxFailures: maxFailures,
	}
}

// Run executes the startup probe.
func (sp *StartupProbe) Run(ctx context.Context) error {
	if sp.started.Load() {
		return nil // already started successfully
	}

	err := sp.inner(ctx)
	if err == nil {
		sp.started.Store(true)
		return nil
	}

	fails := sp.failures.Add(1)
	if int(fails) >= sp.maxFailures {
		return fmt.Errorf("startup: probe failed %d/%d consecutive times: %w", fails, sp.maxFailures, err)
	}
	return fmt.Errorf("startup: probe not yet ready (%d/%d failures): %w", fails, sp.maxFailures, err)
}

// IsStarted returns true if the startup probe has passed at least once.
func (sp *StartupProbe) IsStarted() bool {
	return sp.started.Load()
}

// Probe returns a ProbeFunc for this startup probe.
func (sp *StartupProbe) Probe() ProbeFunc {
	return sp.Run
}

// ─── Platform-specific helpers ──────────────────────────────────────────────

// deadline returns the remaining duration from ctx or the fallback.
func deadline(ctx context.Context, fallback time.Duration) time.Duration {
	if d, ok := ctx.Deadline(); ok {
		if remaining := time.Until(d); remaining > 0 {
			return remaining
		}
	}
	return fallback
}
