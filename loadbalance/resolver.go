package loadbalance

import (
	"context"
	"sync"
	"time"

	"github.com/astra-go/astra/discovery"
)

// ─── Resolver ────────────────────────────────────────────────────────────────

// Resolver maintains a live, in-memory snapshot of service instances by
// subscribing once to discovery.Registry.Watch. Callers read Instances() at
// O(1) cost — no network round-trip per request.
//
// Create with NewResolver; call Close when the resolver is no longer needed.
//
//	r, err := loadbalance.NewResolver(ctx, registry, "user-svc")
//	if err != nil { ... }
//	defer r.Close()
//
//	inst, err := lb.Pick(r.Instances(), "")
type Resolver struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu       sync.RWMutex
	snapshot []*discovery.ServiceInstance
}

// NewResolver subscribes to Watch on reg for serviceName and blocks until the
// first snapshot is received (or ctx is cancelled).
func NewResolver(ctx context.Context, reg discovery.Registry, serviceName string) (*Resolver, error) {
	watchCtx, cancel := context.WithCancel(ctx)

	ch, err := reg.Watch(watchCtx, serviceName)
	if err != nil {
		cancel()
		return nil, err
	}

	r := &Resolver{
		cancel: cancel,
		done:   make(chan struct{}),
	}

	// Wait for the first snapshot so callers always have a non-nil list.
	first, ok := <-ch
	if !ok {
		cancel()
		return nil, ErrNoInstances
	}
	r.mu.Lock()
	r.snapshot = first
	r.mu.Unlock()

	go r.loop(ch)
	return r, nil
}

func (r *Resolver) loop(ch <-chan []*discovery.ServiceInstance) {
	defer close(r.done)
	for insts := range ch {
		cp := make([]*discovery.ServiceInstance, len(insts))
		copy(cp, insts)
		r.mu.Lock()
		r.snapshot = cp
		r.mu.Unlock()
	}
}

// Instances returns the current live instance list. Never nil after NewResolver.
func (r *Resolver) Instances() []*discovery.ServiceInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot
}

// Close cancels the underlying Watch subscription. Safe to call multiple times.
func (r *Resolver) Close() {
	r.cancel()
	<-r.done
}

// ─── OutlierDetector ─────────────────────────────────────────────────────────

// OutlierConfig controls the passive health checking behaviour of OutlierDetector.
type OutlierConfig struct {
	// ConsecutiveErrors is the number of consecutive errors before an instance
	// is ejected. Default: 5.
	ConsecutiveErrors int

	// EjectionInterval is how long an ejected instance stays out.
	// Default: 30s.
	EjectionInterval time.Duration

	// MaxEjectionPct caps the percentage of the instance list that can be
	// ejected simultaneously (0–100). Default: 50.
	// When all remaining instances would be ejected, the full list is used as
	// a fallback to avoid a total blackout.
	MaxEjectionPct int
}

func (c *OutlierConfig) applyDefaults() {
	if c.ConsecutiveErrors <= 0 {
		c.ConsecutiveErrors = 5
	}
	if c.EjectionInterval <= 0 {
		c.EjectionInterval = 30 * time.Second
	}
	if c.MaxEjectionPct <= 0 {
		c.MaxEjectionPct = 50
	}
}

type outlierStat struct {
	consecutive int       // consecutive error count
	ejectedAt   time.Time // zero = not ejected
}

// OutlierDetector wraps a Balancer and implements Reporter.
// It tracks per-instance consecutive errors and ejects unhealthy backends for
// EjectionInterval, mirroring Envoy's outlier-detection behaviour.
//
//	od := loadbalance.NewOutlierDetector(loadbalance.NewP2C(), loadbalance.OutlierConfig{})
//	inst, err := od.Pick(resolver.Instances(), key)
//	// … do request …
//	od.RecordSuccess(inst, elapsed) // or RecordError
type OutlierDetector struct {
	inner Balancer
	cfg   OutlierConfig

	mu    sync.Mutex
	stats map[string]*outlierStat
}

// NewOutlierDetector creates an OutlierDetector wrapping inner.
func NewOutlierDetector(inner Balancer, cfg OutlierConfig) *OutlierDetector {
	cfg.applyDefaults()
	return &OutlierDetector{
		inner: inner,
		cfg:   cfg,
		stats: make(map[string]*outlierStat),
	}
}

// Pick filters out currently ejected instances, then delegates to the inner
// balancer. If filtering would leave zero candidates, falls back to the full
// list (no total-blackout).
func (o *OutlierDetector) Pick(instances []*discovery.ServiceInstance, key string) (*discovery.ServiceInstance, error) {
	healthy := o.healthy(instances)
	if len(healthy) == 0 {
		healthy = instances // fallback: full list
	}
	return o.inner.Pick(healthy, key)
}

// RecordSuccess resets the consecutive-error counter for inst and readmits it
// if it was ejected.
func (o *OutlierDetector) RecordSuccess(inst *discovery.ServiceInstance, elapsed time.Duration) {
	if inst == nil {
		return
	}
	o.mu.Lock()
	st := o.statFor(inst.ID)
	st.consecutive = 0
	st.ejectedAt = time.Time{}
	o.mu.Unlock()

	if r, ok := o.inner.(Reporter); ok {
		r.RecordSuccess(inst, elapsed)
	}
}

// RecordError increments the consecutive-error counter for inst and ejects it
// if the threshold is reached.
func (o *OutlierDetector) RecordError(inst *discovery.ServiceInstance, elapsed time.Duration) {
	if inst == nil {
		return
	}
	now := time.Now()
	o.mu.Lock()
	st := o.statFor(inst.ID)
	st.consecutive++
	if st.consecutive >= o.cfg.ConsecutiveErrors && st.ejectedAt.IsZero() {
		st.ejectedAt = now
	}
	o.mu.Unlock()

	if r, ok := o.inner.(Reporter); ok {
		r.RecordError(inst, elapsed)
	}
}

// EjectedInstances returns the IDs of currently ejected instances.
func (o *OutlierDetector) EjectedInstances() []string {
	now := time.Now()
	o.mu.Lock()
	defer o.mu.Unlock()

	var out []string
	for id, st := range o.stats {
		if !st.ejectedAt.IsZero() && now.Before(st.ejectedAt.Add(o.cfg.EjectionInterval)) {
			out = append(out, id)
		}
	}
	return out
}

// healthy returns the subset of instances that are not currently ejected,
// respecting MaxEjectionPct.
func (o *OutlierDetector) healthy(instances []*discovery.ServiceInstance) []*discovery.ServiceInstance {
	now := time.Now()
	o.mu.Lock()
	defer o.mu.Unlock()

	maxEject := len(instances) * o.cfg.MaxEjectionPct / 100
	if maxEject == 0 {
		maxEject = 1 // always allow at least one ejection when threshold > 0
	}

	out := make([]*discovery.ServiceInstance, 0, len(instances))
	ejectedCount := 0
	for _, inst := range instances {
		st := o.stats[inst.ID]
		isEjected := st != nil &&
			!st.ejectedAt.IsZero() &&
			now.Before(st.ejectedAt.Add(o.cfg.EjectionInterval))
		if isEjected && ejectedCount < maxEject {
			ejectedCount++
			continue
		}
		// Auto-readmit: clear ejection flag if interval has passed
		if st != nil && !st.ejectedAt.IsZero() && !now.Before(st.ejectedAt.Add(o.cfg.EjectionInterval)) {
			st.ejectedAt = time.Time{}
			st.consecutive = 0
		}
		out = append(out, inst)
	}
	return out
}

func (o *OutlierDetector) statFor(id string) *outlierStat {
	st, ok := o.stats[id]
	if !ok {
		st = &outlierStat{}
		o.stats[id] = st
	}
	return st
}

// Ensure OutlierDetector implements Reporter.
var _ Reporter = (*OutlierDetector)(nil)
