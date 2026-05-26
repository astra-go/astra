// Package loadbalance provides load-balancing strategies for service instances.
//
// All balancers implement the Balancer interface and are safe for concurrent use.
//
//	b := loadbalance.NewRoundRobin()
//	inst, err := b.Pick(instances, "")
//
// The key parameter is only used by ConsistentHash; other balancers ignore it.
//
// # Health filtering
//
// ServiceInstance has no Healthy field — the discovery layer already returns only
// live endpoints. If you need to exclude degraded instances (e.g. marked via
// Metadata), call Filter before Pick:
//
//	healthy := loadbalance.Filter(instances, func(i *discovery.ServiceInstance) bool {
//	    return i.Metadata["status"] != "draining"
//	})
//	inst, err := lb.Pick(healthy, key)
package loadbalance

import (
	"errors"
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/astra-go/astra/discovery"
)

// ErrNoInstances is returned when the instance list is empty.
var ErrNoInstances = errors.New("loadbalance: no available instances")

// ─── Balancer interface ───────────────────────────────────────────────────────

// Balancer picks one instance from a candidate list.
//
// key is an optional routing hint (e.g. user-ID or session token).
// It is used by ConsistentHash; round-robin, random, weighted, and least-conn
// balancers ignore it.
type Balancer interface {
	Pick(instances []*discovery.ServiceInstance, key string) (*discovery.ServiceInstance, error)
}

// ─── Reporter interface ───────────────────────────────────────────────────────

// Reporter is optionally implemented by Balancers that track per-instance
// health and latency. When a Client's balancer implements Reporter, the Client
// automatically calls RecordSuccess / RecordError after each request, enabling
// adaptive load balancing without manual instrumentation.
//
// P2C and OutlierDetector implement this interface.
type Reporter interface {
	// RecordSuccess is called after a successful request to inst.
	// elapsed is the end-to-end request latency for EWMA updates.
	RecordSuccess(inst *discovery.ServiceInstance, elapsed time.Duration)
	// RecordError is called after a failed request (network error or 5xx).
	RecordError(inst *discovery.ServiceInstance, elapsed time.Duration)
}

// Doner is optionally implemented by Balancers that track in-flight request
// counts via an explicit decrement. Callers MUST call Done after each Pick
// unless RecordSuccess / RecordError already handles it (as P2C does).
//
// LeastConn implements this interface. Clients check for Doner and call Done
// automatically when the balancer does not also implement Reporter.
type Doner interface {
	Done(inst *discovery.ServiceInstance)
}

// ─── Filter utilities ─────────────────────────────────────────────────────────

// Filter returns all instances for which fn returns true.
func Filter(instances []*discovery.ServiceInstance, fn func(*discovery.ServiceInstance) bool) []*discovery.ServiceInstance {
	out := make([]*discovery.ServiceInstance, 0, len(instances))
	for _, inst := range instances {
		if fn(inst) {
			out = append(out, inst)
		}
	}
	return out
}

// FilterByMetadata returns instances whose Metadata[key] equals value.
func FilterByMetadata(instances []*discovery.ServiceInstance, key, value string) []*discovery.ServiceInstance {
	return Filter(instances, func(inst *discovery.ServiceInstance) bool {
		return inst.Metadata[key] == value
	})
}

// LocalityFirst returns the subset of instances matching metadata key=value,
// giving locality-aware (zone/region) preference.
// Falls back to the full list if no local instances are found — no blackout risk.
//
//	zone := loadbalance.LocalityFirst(insts, "zone", "us-east-1")
//	inst, _ := lb.Pick(zone, "")
func LocalityFirst(instances []*discovery.ServiceInstance, key, value string) []*discovery.ServiceInstance {
	local := FilterByMetadata(instances, key, value)
	if len(local) > 0 {
		return local
	}
	return instances
}

// ─── Round-Robin ──────────────────────────────────────────────────────────────

// RoundRobin picks instances in a cyclic order regardless of weight.
// Uses a lock-free atomic counter.
type RoundRobin struct {
	counter uint64
}

// NewRoundRobin creates a RoundRobin balancer.
func NewRoundRobin() *RoundRobin { return &RoundRobin{} }

// Pick selects the next instance in round-robin order. key is ignored.
func (r *RoundRobin) Pick(instances []*discovery.ServiceInstance, _ string) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	n := atomic.AddUint64(&r.counter, 1)
	return instances[(n-1)%uint64(len(instances))], nil
}

// ─── Random ───────────────────────────────────────────────────────────────────

// Random picks an instance uniformly at random. key is ignored.
type Random struct{}

// NewRandom creates a Random balancer.
func NewRandom() *Random { return &Random{} }

// Pick selects an instance at random. key is ignored.
func (*Random) Pick(instances []*discovery.ServiceInstance, _ string) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	return instances[rand.IntN(len(instances))], nil
}

// ─── Weighted Random ──────────────────────────────────────────────────────────

// Weighted picks instances proportionally to their Weight field using weighted
// random selection (stateless). For smooth, evenly-spread distribution use
// SmoothWeighted instead.
type Weighted struct{}

// NewWeighted creates a Weighted balancer.
func NewWeighted() *Weighted { return &Weighted{} }

// Pick selects an instance with probability proportional to its weight. key is ignored.
func (*Weighted) Pick(instances []*discovery.ServiceInstance, _ string) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	if len(instances) == 1 {
		return instances[0], nil
	}

	total := 0
	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}

	r := rand.IntN(total)
	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}
		r -= w
		if r < 0 {
			return inst, nil
		}
	}
	return instances[len(instances)-1], nil
}

// ─── Smooth Weighted Round Robin (SWRR) ──────────────────────────────────────

// SmoothWeighted implements the Smooth Weighted Round-Robin algorithm
// (nginx's variant). Unlike random weighted selection, SWRR distributes
// high-weight instances evenly across requests rather than in bursts.
//
// Algorithm (per Pick):
//  1. Each instance's currentWeight += its Weight.
//  2. Pick the instance with the highest currentWeight.
//  3. Reduce that instance's currentWeight by the total weight sum.
//
// Example for weights [5, 3, 2] over 10 picks:
//
//	A A B A B C A B A C  (compare to random which may burst A five times in a row)
type SmoothWeighted struct {
	mu      sync.Mutex
	current map[string]int // instance ID → running current weight
}

// NewSmoothWeighted creates a SmoothWeighted balancer.
func NewSmoothWeighted() *SmoothWeighted {
	return &SmoothWeighted{current: make(map[string]int)}
}

// Pick selects the next instance using smooth weighted round-robin. key is ignored.
func (s *SmoothWeighted) Pick(instances []*discovery.ServiceInstance, _ string) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	if len(instances) == 1 {
		return instances[0], nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	total := 0
	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}
		total += w
		s.current[inst.ID] += w
	}

	var best *discovery.ServiceInstance
	var bestCW int
	for _, inst := range instances {
		if cw := s.current[inst.ID]; best == nil || cw > bestCW {
			best = inst
			bestCW = cw
		}
	}

	s.current[best.ID] -= total
	return best, nil
}

// ─── Least Connections ────────────────────────────────────────────────────────

// LeastConn picks the instance with the fewest active (in-flight) requests.
// Callers MUST call Done when a request completes so the counter is decremented.
//
//	inst, err := lb.Pick(instances, "")
//	defer lb.Done(inst)
//
// LeastConn is safe for concurrent use.
type LeastConn struct {
	mu     sync.Mutex
	counts map[string]int64 // instance ID → active count
}

// NewLeastConn creates a LeastConn balancer.
func NewLeastConn() *LeastConn {
	return &LeastConn{counts: make(map[string]int64)}
}

// Pick selects the instance with the lowest number of active requests.
// On a tie, the first tied instance in the input slice wins.
// key is ignored.
func (l *LeastConn) Pick(instances []*discovery.ServiceInstance, _ string) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	var best *discovery.ServiceInstance
	var bestCount int64
	for _, inst := range instances {
		c := l.counts[inst.ID]
		if best == nil || c < bestCount {
			best = inst
			bestCount = c
		}
	}
	l.counts[best.ID]++
	return best, nil
}

// Done decrements the active-request counter for inst.
// It must be called exactly once for each successful Pick.
func (l *LeastConn) Done(inst *discovery.ServiceInstance) {
	if inst == nil {
		return
	}
	l.mu.Lock()
	if l.counts[inst.ID] > 0 {
		l.counts[inst.ID]--
	}
	l.mu.Unlock()
}

// ─── P2C — Power of Two Choices with EWMA ────────────────────────────────────

// P2C implements the Power of Two Choices algorithm with EWMA latency scoring.
// It randomly samples 2 candidates and picks the one with the lower composite
// score: score = (inflight + 1) × ewmaLatency.
//
// This achieves near-optimal load distribution with O(1) complexity and adapts
// automatically to heterogeneous instance performance — the algorithm used by
// Envoy and go-zero for service-to-service calls.
//
// Callers MUST call RecordSuccess or RecordError (or Done for the no-latency
// variant) after each Pick so the inflight counter stays accurate.
type P2C struct {
	mu    sync.Mutex
	stats map[string]*p2cStat
}

type p2cStat struct {
	inflight int64
	ewma     float64 // nanoseconds, EWMA of request latency
	count    int64
}

const p2cEWMADecay = 0.9 // keep 90 % of old value, blend in 10 % of new sample

func (st *p2cStat) score() float64 {
	ewma := st.ewma
	if ewma <= 0 {
		ewma = 1e6 // 1 ms default for new/unseen instances
	}
	return float64(st.inflight+1) * ewma
}

// NewP2C creates a P2C balancer.
func NewP2C() *P2C { return &P2C{stats: make(map[string]*p2cStat)} }

// Pick samples 2 random candidates and returns the one with the lower score.
// key is ignored.
func (p *P2C) Pick(instances []*discovery.ServiceInstance, _ string) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	if len(instances) == 1 {
		p.mu.Lock()
		p.statFor(instances[0].ID).inflight++
		p.mu.Unlock()
		return instances[0], nil
	}

	// Sample 2 distinct indices.
	i := rand.IntN(len(instances))
	j := rand.IntN(len(instances) - 1)
	if j >= i {
		j++
	}
	a, b := instances[i], instances[j]

	p.mu.Lock()
	sa, sb := p.statFor(a.ID), p.statFor(b.ID)
	chosen := a
	if sb.score() < sa.score() {
		chosen = b
	}
	p.statFor(chosen.ID).inflight++
	p.mu.Unlock()
	return chosen, nil
}

// Done decrements the inflight counter without updating the EWMA.
// Use RecordSuccess / RecordError instead when latency data is available.
func (p *P2C) Done(inst *discovery.ServiceInstance) {
	if inst == nil {
		return
	}
	p.mu.Lock()
	if st := p.statFor(inst.ID); st.inflight > 0 {
		st.inflight--
	}
	p.mu.Unlock()
}

// RecordSuccess decrements inflight and updates the EWMA with the real latency.
func (p *P2C) RecordSuccess(inst *discovery.ServiceInstance, elapsed time.Duration) {
	if inst == nil {
		return
	}
	p.mu.Lock()
	st := p.statFor(inst.ID)
	if st.inflight > 0 {
		st.inflight--
	}
	p.applyEWMA(st, elapsed)
	p.mu.Unlock()
}

// RecordError decrements inflight and updates the EWMA (slow errors are
// deprioritised just like slow successes).
func (p *P2C) RecordError(inst *discovery.ServiceInstance, elapsed time.Duration) {
	if inst == nil {
		return
	}
	p.mu.Lock()
	st := p.statFor(inst.ID)
	if st.inflight > 0 {
		st.inflight--
	}
	p.applyEWMA(st, elapsed)
	p.mu.Unlock()
}

func (p *P2C) statFor(id string) *p2cStat {
	st, ok := p.stats[id]
	if !ok {
		st = &p2cStat{}
		p.stats[id] = st
	}
	return st
}

func (p *P2C) applyEWMA(st *p2cStat, elapsed time.Duration) {
	ns := float64(elapsed.Nanoseconds())
	if st.count == 0 {
		st.ewma = ns
	} else {
		st.ewma = p2cEWMADecay*st.ewma + (1-p2cEWMADecay)*ns
	}
	st.count++
}

// ─── Consistent Hash ─────────────────────────────────────────────────────────

// vnodeRing is an immutable ring snapshot for a given set of instances.
// Stored and loaded atomically so Pick reads never block.
type vnodeRing struct {
	fingerprint uint64
	vnodes      []vnode
}

// ConsistentHash assigns requests to instances deterministically based on a key.
// It uses a virtual node ring with FNV-1a hashing to minimize re-mapping when
// the instance list changes.
//
// The ring snapshot is held in an atomic.Pointer so Pick reads are lock-free.
// The ring is rebuilt (under a mutex) only when the instance fingerprint
// changes; concurrent readers always observe a consistent immutable snapshot.
//
// If key is empty, ConsistentHash falls back to random selection.
type ConsistentHash struct {
	replicas int
	ring     atomic.Pointer[vnodeRing]
	mu       sync.Mutex // guards ring rebuild only; reads are lock-free
}

// NewConsistentHash creates a ConsistentHash balancer.
// replicas is the number of virtual nodes per instance (default 150).
func NewConsistentHash(replicas int) *ConsistentHash {
	if replicas <= 0 {
		replicas = 150
	}
	return &ConsistentHash{replicas: replicas}
}

type vnode struct {
	hash uint32
	inst *discovery.ServiceInstance
}

// Pick selects an instance deterministically by key.
// If key is empty, returns a random instance.
func (c *ConsistentHash) Pick(instances []*discovery.ServiceInstance, key string) (*discovery.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	if key == "" {
		return instances[rand.IntN(len(instances))], nil
	}

	fp := instanceFingerprintHash(instances)

	// Fast path: lock-free load — zero contention on steady-state traffic.
	current := c.ring.Load()
	if current == nil || current.fingerprint != fp {
		current = c.buildAndStore(instances, fp)
	}

	h := fnv1a32(key)
	ring := current.vnodes
	idx := sort.Search(len(ring), func(i int) bool { return ring[i].hash >= h })
	if idx == len(ring) {
		idx = 0
	}
	return ring[idx].inst, nil
}

// buildAndStore computes the ring for instances and atomically stores it.
// Double-checked locking prevents duplicate rebuilds under contention.
func (c *ConsistentHash) buildAndStore(instances []*discovery.ServiceInstance, fp uint64) *vnodeRing {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Another goroutine may have built it while we waited for the lock.
	if current := c.ring.Load(); current != nil && current.fingerprint == fp {
		return current
	}

	ring := make([]vnode, 0, len(instances)*c.replicas)
	for _, inst := range instances {
		// Allocate the scratch buffer once per instance and reuse it across all
		// replica iterations — saves (replicas-1) allocs per instance vs the
		// previous fmt.Sprintf("%s#%d", id, i) approach.
		buf := make([]byte, 0, len(inst.ID)+12)
		buf = append(buf, inst.ID...)
		buf = append(buf, '#')
		base := len(buf)
		for i := range c.replicas {
			b := strconv.AppendInt(buf[:base], int64(i), 10)
			ring = append(ring, vnode{hash: fnv1a32b(b), inst: inst})
		}
	}
	slices.SortFunc(ring, func(a, b vnode) int {
		if a.hash < b.hash {
			return -1
		}
		if a.hash > b.hash {
			return 1
		}
		return 0
	})

	r := &vnodeRing{fingerprint: fp, vnodes: ring}
	c.ring.Store(r)
	return r
}

// instanceFingerprintHash returns an order-independent hash of the instance ID
// set. Uses sum+XOR combination so the result changes when any ID is added,
// removed, or replaced — with negligible collision probability (~1/2^64).
// Zero allocations; O(N).
func instanceFingerprintHash(instances []*discovery.ServiceInstance) uint64 {
	var sum, xorSum uint64
	for _, inst := range instances {
		h := fnv1a64(inst.ID)
		sum += h
		xorSum ^= h
	}
	return (sum * 2654435761) ^ (xorSum * 2246822519) ^ (uint64(len(instances)) * 1000000007)
}

// fnv1a32 returns the FNV-1a 32-bit hash of a string.
// Inlined to avoid the heap allocation that fnv.New32a() + []byte(s) incurs.
func fnv1a32(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// fnv1a32b returns the FNV-1a 32-bit hash of a byte slice.
func fnv1a32b(b []byte) uint32 {
	var h uint32 = 2166136261
	for _, c := range b {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

// fnv1a64 returns the FNV-1a 64-bit hash of a string. Used for fingerprinting.
func fnv1a64(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
