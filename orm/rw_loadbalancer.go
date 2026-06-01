// Package orm — rw_loadbalancer.go provides advanced load balancing strategies
// for read replica selection.
package orm

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// LoadBalancer defines the interface for replica selection strategies.
type LoadBalancer interface {
	// Select chooses a replica from the healthy list.
	// Returns nil if no replicas are available.
	Select(ctx context.Context, healthy []*gorm.DB) *gorm.DB

	// OnSuccess is called after a successful query to update metrics.
	OnSuccess(db *gorm.DB, duration time.Duration)

	// OnError is called after a failed query to update metrics.
	OnError(db *gorm.DB, err error)
}

// RoundRobinBalancer implements simple round-robin load balancing.
type RoundRobinBalancer struct {
	counter uint64
}

func (b *RoundRobinBalancer) Select(_ context.Context, healthy []*gorm.DB) *gorm.DB {
	if len(healthy) == 0 {
		return nil
	}
	idx := atomic.AddUint64(&b.counter, 1) % uint64(len(healthy))
	return healthy[idx]
}

func (b *RoundRobinBalancer) OnSuccess(_ *gorm.DB, _ time.Duration) {}
func (b *RoundRobinBalancer) OnError(_ *gorm.DB, _ error)         {}

// WeightedRoundRobinBalancer distributes load based on configured weights.
// Higher weight means more requests. Weight must be > 0.
//
// Example:
//
//	replica1 weight=3, replica2 weight=1
//	→ replica1 gets 75% of traffic, replica2 gets 25%
type WeightedRoundRobinBalancer struct {
	mu      sync.RWMutex
	weights map[*gorm.DB]int // replica → weight
	counter uint64
}

// NewWeightedRoundRobinBalancer creates a balancer with the given weights.
// Replicas not in the weights map are assigned weight=1.
func NewWeightedRoundRobinBalancer(weights map[*gorm.DB]int) *WeightedRoundRobinBalancer {
	return &WeightedRoundRobinBalancer{
		weights: weights,
	}
}

func (b *WeightedRoundRobinBalancer) Select(_ context.Context, healthy []*gorm.DB) *gorm.DB {
	if len(healthy) == 0 {
		return nil
	}

	b.mu.RLock()
	weights := b.weights
	b.mu.RUnlock()

	// Build weighted list: [db1, db1, db1, db2, ...] based on weights
	weighted := make([]*gorm.DB, 0, len(healthy)*2)
	for _, db := range healthy {
		w := weights[db]
		if w <= 0 {
			w = 1
		}
		for i := 0; i < w; i++ {
			weighted = append(weighted, db)
		}
	}

	idx := atomic.AddUint64(&b.counter, 1) % uint64(len(weighted))
	return weighted[idx]
}

func (b *WeightedRoundRobinBalancer) OnSuccess(_ *gorm.DB, _ time.Duration) {}
func (b *WeightedRoundRobinBalancer) OnError(_ *gorm.DB, _ error)         {}

// SetWeight updates the weight for a replica. Must be > 0.
func (b *WeightedRoundRobinBalancer) SetWeight(db *gorm.DB, weight int) {
	if weight <= 0 {
		weight = 1
	}
	b.mu.Lock()
	if b.weights == nil {
		b.weights = make(map[*gorm.DB]int)
	}
	b.weights[db] = weight
	b.mu.Unlock()
}

// LeastConnectionsBalancer selects the replica with the fewest active connections.
// Tracks in-flight queries per replica.
type LeastConnectionsBalancer struct {
	mu          sync.RWMutex
	connections map[*gorm.DB]*int64 // replica → active connection count
}

func NewLeastConnectionsBalancer() *LeastConnectionsBalancer {
	return &LeastConnectionsBalancer{
		connections: make(map[*gorm.DB]*int64),
	}
}

func (b *LeastConnectionsBalancer) Select(_ context.Context, healthy []*gorm.DB) *gorm.DB {
	if len(healthy) == 0 {
		return nil
	}

	b.mu.Lock()
	// Ensure all healthy replicas have a counter
	for _, db := range healthy {
		if _, exists := b.connections[db]; !exists {
			var zero int64
			b.connections[db] = &zero
		}
	}
	b.mu.Unlock()

	b.mu.RLock()
	defer b.mu.RUnlock()

	// Find replica with minimum active connections
	var minDB *gorm.DB
	var minCount int64 = 1<<63 - 1 // max int64

	for _, db := range healthy {
		if counter, ok := b.connections[db]; ok {
			count := atomic.LoadInt64(counter)
			if count < minCount {
				minCount = count
				minDB = db
			}
		}
	}

	if minDB != nil {
		atomic.AddInt64(b.connections[minDB], 1)
	}
	return minDB
}

func (b *LeastConnectionsBalancer) OnSuccess(db *gorm.DB, _ time.Duration) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if counter, ok := b.connections[db]; ok {
		atomic.AddInt64(counter, -1)
	}
}

func (b *LeastConnectionsBalancer) OnError(db *gorm.DB, _ error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if counter, ok := b.connections[db]; ok {
		atomic.AddInt64(counter, -1)
	}
}

// GetActiveConnections returns the current active connection count for a replica.
func (b *LeastConnectionsBalancer) GetActiveConnections(db *gorm.DB) int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if counter, ok := b.connections[db]; ok {
		return atomic.LoadInt64(counter)
	}
	return 0
}
