package metrics

import (
	"math"
	"strings"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// RequestStats contains aggregated request statistics.
type RequestStats struct {
	mu sync.RWMutex
	total map[string]int64 // key: "METHOD /path:status"
	latencies map[string]*Histogram
	lastReset time.Time
	count     int64
	count200  int64
	countErr  int64
}

// Histogram stores latency buckets for percentile calculation.
type Histogram struct {
	mu      sync.Mutex
	count   int64
	sum     int64
	buckets map[float64]int64 // percentile -> count
	values  []float64         // raw values for percentile calculation
}

// PoolStats contains pool statistics.
type PoolStats struct {
	mu    sync.RWMutex
	Pools map[string]*PoolStat // name -> stats
}

// PoolStat contains statistics for a single pool.
type PoolStat struct {
	Name      string
	Active    int
	Idle      int
	WaitCount int64
	Hits      int64
	Misses    int64
	Timeouts  int64
}

// SystemStats contains system-level statistics.
type SystemStats struct {
	MemAlloc    uint64
	MemTotal    uint64
	MemLimit    uint64
	Goroutines  int
	CPUPercent  float64
	Uptime      time.Duration
	LastGC      time.Time
	NumGC      int
}

// EndpointStat contains statistics for a single endpoint.
type EndpointStat struct {
	Path        string
	Method      string
	Count       int64
	AvgLatency  time.Duration
	P50Latency  time.Duration
	P95Latency  time.Duration
	P99Latency  time.Duration
	ErrorRate   float64
	RequestsPerSec float64
}

// globalRegistry is the default metrics registry.
var globalRegistry atomic.Value // *Registry

func init() {
	globalRegistry.Store(NewRegistry())
}

// Registry is the central metrics registry.
type Registry struct {
	requests  *RequestStats
	pools     *PoolStats
	startTime time.Time
}

// NewRegistry creates a new metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		requests: &RequestStats{
			total:     make(map[string]int64),
			latencies: make(map[string]*Histogram),
			lastReset: time.Now(),
		},
		pools: &PoolStats{
			Pools: make(map[string]*PoolStat),
		},
		startTime: time.Now(),
	}
}

// RecordRequest records a request with the given parameters.
func (r *Registry) RecordRequest(path, method string, status int, duration time.Duration) {
	key := method + " " + path
	histKey := key + ":" + itoa(status)

	r.requests.mu.Lock()
	r.requests.total[key]++
	r.requests.total[histKey]++
	r.requests.count++
	if status >= 200 && status < 300 {
		r.requests.count200++
	} else {
		r.requests.countErr++
	}
	r.requests.mu.Unlock()

	// Update histogram
	r.requests.mu.Lock()
	hist := r.requests.latencies[histKey]
	if hist == nil {
		hist = NewHistogram()
		r.requests.latencies[histKey] = hist
	}
	r.requests.mu.Unlock()

	hist.Record(duration)
}

// PoolStats returns the current pool statistics.
func (r *Registry) PoolStats() *PoolStats {
	stats := &PoolStats{
		Pools: make(map[string]*PoolStat),
	}

	r.pools.mu.RLock()
	defer r.pools.mu.RUnlock()

	totalActive := 0
	totalIdle := 0
	for name, ps := range r.pools.Pools {
		stats.Pools[name] = &PoolStat{
			Name:      name,
			Active:    ps.Active,
			Idle:      ps.Idle,
			WaitCount: ps.WaitCount,
			Hits:      ps.Hits,
			Misses:    ps.Misses,
			Timeouts:  ps.Timeouts,
		}
		totalActive += ps.Active
		totalIdle += ps.Idle
	}

	return stats
}

// RequestStats returns the current request statistics.
func (r *Registry) RequestStats() *RequestStats {
	stats := &RequestStats{
		total:     make(map[string]int64),
		latencies: make(map[string]*Histogram),
		lastReset: r.requests.lastReset,
		count:     atomic.LoadInt64(&r.requests.count),
		count200:  atomic.LoadInt64(&r.requests.count200),
		countErr:  atomic.LoadInt64(&r.requests.countErr),
	}

	r.requests.mu.RLock()
	defer r.requests.mu.RUnlock()

	for k, v := range r.requests.total {
		stats.total[k] = v
	}
	for k, h := range r.requests.latencies {
		stats.latencies[k] = h
	}

	return stats
}

// RecordPoolOperation records a pool operation.
func (r *Registry) RecordPoolOperation(poolName string, op string) {
	r.pools.mu.Lock()
	defer r.pools.mu.Unlock()

	ps := r.pools.Pools[poolName]
	if ps == nil {
		ps = &PoolStat{Name: poolName}
		r.pools.Pools[poolName] = ps
	}

	switch op {
	case "hit":
		ps.Hits++
	case "miss":
		ps.Misses++
	case "wait":
		ps.WaitCount++
	case "timeout":
		ps.Timeouts++
	}
}

// UpdatePoolStats updates the pool statistics.
func (r *Registry) UpdatePoolStats(poolName string, active, idle int) {
	r.pools.mu.Lock()
	defer r.pools.mu.Unlock()

	ps := r.pools.Pools[poolName]
	if ps == nil {
		ps = &PoolStat{Name: poolName}
		r.pools.Pools[poolName] = ps
	}

	ps.Active = active
	ps.Idle = idle
}

// TotalRequests returns the total number of requests.
func (s *RequestStats) TotalRequests() int64 {
	return atomic.LoadInt64(&s.count)
}

// RequestsPerSec returns the requests per second rate.
func (s *RequestStats) RequestsPerSec() float64 {
	elapsed := time.Since(s.lastReset).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&s.count)) / elapsed
}

// AvgLatency returns the average latency.
func (s *RequestStats) AvgLatency() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := int64(0)
	count := int64(0)
	for _, hist := range s.latencies {
		c := atomic.LoadInt64(&hist.count)
		if c > 0 {
			s := atomic.LoadInt64(&hist.sum)
			total += s
			count += c
		}
	}

	if count == 0 {
		return 0
	}
	return time.Duration(total / count)
}

// P99Latency returns the P99 latency.
func (s *RequestStats) P99Latency() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var p99s []float64
	for _, hist := range s.latencies {
		if p := hist.Percentile(99); p > 0 {
			p99s = append(p99s, p)
		}
	}

	if len(p99s) == 0 {
		return 0
	}

	// Return max P99 across all endpoints
	max := p99s[0]
	for _, p := range p99s[1:] {
		if p > max {
			max = p
		}
	}
	return time.Duration(max)
}

// ErrorRate returns the error rate.
func (s *RequestStats) ErrorRate() float64 {
	total := atomic.LoadInt64(&s.count)
	if total == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&s.countErr)) / float64(total)
}

// TopEndpoints returns the top endpoints by request count.
func (s *RequestStats) TopEndpoints() []EndpointStat {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats []EndpointStat
	for key, count := range s.total {
		// Skip status-specific keys (contain ':')
		if strings.Contains(key, ":") {
			continue
		}
		
		// Extract path from key (format: "METHOD /path")
		path := key
		if idx := strings.LastIndex(key, " "); idx >= 0 {
			path = key[idx+1:]
		}
		
		stats = append(stats, EndpointStat{
			Path:  path,
			Count: count,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	return stats
}

// NewHistogram creates a new histogram.
func NewHistogram() *Histogram {
	return &Histogram{
		buckets: make(map[float64]int64),
	}
}

// Record records a duration.
func (h *Histogram) Record(duration time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ns := duration.Nanoseconds()
	atomic.AddInt64(&h.count, 1)
	atomic.AddInt64(&h.sum, ns)
	h.values = append(h.values, float64(ns))
}

// Percentile returns the value at the given percentile.
func (h *Histogram) Percentile(p float64) float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.values) == 0 {
		return 0
	}

	// Simple percentile calculation
	sorted := make([]float64, len(h.values))
	copy(sorted, h.values)
	sort.Float64s(sorted)

	idx := int(math.Ceil(float64(len(sorted))*p/100)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}

	return sorted[idx]
}

// Percentiles returns predefined percentiles.
func (h *Histogram) Percentiles() []PercentileBucket {
	return []PercentileBucket{
		{Percentile: "0.5", Count: int64(h.Percentile(50))},
		{Percentile: "0.9", Count: int64(h.Percentile(90))},
		{Percentile: "0.95", Count: int64(h.Percentile(95))},
		{Percentile: "0.99", Count: int64(h.Percentile(99))},
	}
}

// PercentileBucket is a histogram bucket.
type PercentileBucket struct {
	Percentile string
	Count     int64
}

// CollectSystemStats collects system-level statistics.
func CollectSystemStats() *SystemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &SystemStats{
		MemAlloc:   m.Alloc,
		MemTotal:   m.TotalAlloc,
		MemLimit:   m.Sys,
		Goroutines: runtime.NumGoroutine(),
		CPUPercent: 0, // Would need external package for CPU%
		Uptime:     time.Since(globalRegistry.Load().(*Registry).startTime),
		LastGC:     time.Unix(0, int64(m.LastGC)),
		NumGC:      int(m.NumGC),
	}
}

// itoa converts an int to a string.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte(n % 10)
		n /= 10
	}
	return string(digits[i:])
}

// GlobalRegistry returns the global registry.
func GlobalRegistry() *Registry {
	return globalRegistry.Load().(*Registry)
}

// RecordRequest is a convenience function for recording a request to the global registry.
func RecordRequest(path, method string, status int, duration time.Duration) {
	GlobalRegistry().RecordRequest(path, method, status, duration)
}