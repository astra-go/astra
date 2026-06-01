package orm

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRoundRobinBalancer(t *testing.T) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db3, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	healthy := []*gorm.DB{db1, db2, db3}
	balancer := &RoundRobinBalancer{}

	ctx := context.Background()
	results := make(map[*gorm.DB]int)

	for i := 0; i < 300; i++ {
		selected := balancer.Select(ctx, healthy)
		results[selected]++
	}

	// Each replica should get exactly 100 requests (300 / 3)
	for db, count := range results {
		if count != 100 {
			t.Errorf("replica %p got %d requests, expected 100", db, count)
		}
	}
}

func TestRoundRobinBalancer_EmptyHealthy(t *testing.T) {
	balancer := &RoundRobinBalancer{}
	selected := balancer.Select(context.Background(), nil)
	if selected != nil {
		t.Errorf("expected nil for empty healthy list, got %v", selected)
	}
}

func TestWeightedRoundRobinBalancer(t *testing.T) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db3, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	weights := map[*gorm.DB]int{
		db1: 3, // 60% (3/5)
		db2: 1, // 20% (1/5)
		db3: 1, // 20% (1/5)
	}

	balancer := NewWeightedRoundRobinBalancer(weights)
	healthy := []*gorm.DB{db1, db2, db3}

	ctx := context.Background()
	results := make(map[*gorm.DB]int)

	for i := 0; i < 500; i++ {
		selected := balancer.Select(ctx, healthy)
		results[selected]++
	}

	// db1 should get ~300 requests (60%)
	// db2 should get ~100 requests (20%)
	// db3 should get ~100 requests (20%)
	if results[db1] != 300 {
		t.Errorf("db1 got %d requests, expected 300", results[db1])
	}
	if results[db2] != 100 {
		t.Errorf("db2 got %d requests, expected 100", results[db2])
	}
	if results[db3] != 100 {
		t.Errorf("db3 got %d requests, expected 100", results[db3])
	}
}

func TestWeightedRoundRobinBalancer_DefaultWeight(t *testing.T) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	// db1 has explicit weight, db2 uses default weight=1
	weights := map[*gorm.DB]int{
		db1: 2,
	}

	balancer := NewWeightedRoundRobinBalancer(weights)
	healthy := []*gorm.DB{db1, db2}

	ctx := context.Background()
	results := make(map[*gorm.DB]int)

	for i := 0; i < 300; i++ {
		selected := balancer.Select(ctx, healthy)
		results[selected]++
	}

	// db1 should get 200 requests (2/3)
	// db2 should get 100 requests (1/3)
	if results[db1] != 200 {
		t.Errorf("db1 got %d requests, expected 200", results[db1])
	}
	if results[db2] != 100 {
		t.Errorf("db2 got %d requests, expected 100", results[db2])
	}
}

func TestWeightedRoundRobinBalancer_SetWeight(t *testing.T) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	balancer := NewWeightedRoundRobinBalancer(nil)
	balancer.SetWeight(db1, 5)

	healthy := []*gorm.DB{db1}
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		selected := balancer.Select(ctx, healthy)
		if selected != db1 {
			t.Errorf("expected db1, got %v", selected)
		}
	}
}

func TestWeightedRoundRobinBalancer_ZeroWeight(t *testing.T) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	balancer := NewWeightedRoundRobinBalancer(nil)
	balancer.SetWeight(db1, 0) // Should be normalized to 1

	healthy := []*gorm.DB{db1}
	selected := balancer.Select(context.Background(), healthy)
	if selected != db1 {
		t.Errorf("expected db1 even with zero weight, got %v", selected)
	}
}

func TestLeastConnectionsBalancer(t *testing.T) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db3, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	balancer := NewLeastConnectionsBalancer()
	healthy := []*gorm.DB{db1, db2, db3}
	ctx := context.Background()

	// First 3 requests should go to different replicas (all have 0 connections)
	selected1 := balancer.Select(ctx, healthy)
	balancer.Select(ctx, healthy)
	balancer.Select(ctx, healthy)

	// All replicas should have 1 active connection now
	if balancer.GetActiveConnections(db1) != 1 {
		t.Errorf("db1 connections = %d, expected 1", balancer.GetActiveConnections(db1))
	}
	if balancer.GetActiveConnections(db2) != 1 {
		t.Errorf("db2 connections = %d, expected 1", balancer.GetActiveConnections(db2))
	}
	if balancer.GetActiveConnections(db3) != 1 {
		t.Errorf("db3 connections = %d, expected 1", balancer.GetActiveConnections(db3))
	}

	// Complete one query
	balancer.OnSuccess(selected1, time.Millisecond)

	// Next request should go to the replica that just finished (now has 0 connections)
	selected4 := balancer.Select(ctx, healthy)
	if selected4 != selected1 {
		t.Errorf("expected request to go to replica with 0 connections")
	}

	// Verify connection counts
	if balancer.GetActiveConnections(selected1) != 1 {
		t.Errorf("selected1 connections = %d, expected 1", balancer.GetActiveConnections(selected1))
	}
}

func TestLeastConnectionsBalancer_Concurrent(t *testing.T) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	balancer := NewLeastConnectionsBalancer()
	healthy := []*gorm.DB{db1, db2}
	ctx := context.Background()

	var wg sync.WaitGroup
	selections := make(chan *gorm.DB, 100)

	// Simulate 100 concurrent requests
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			selected := balancer.Select(ctx, healthy)
			selections <- selected
			time.Sleep(time.Millisecond)
			balancer.OnSuccess(selected, time.Millisecond)
		}()
	}

	wg.Wait()
	close(selections)

	// Count selections
	results := make(map[*gorm.DB]int)
	for db := range selections {
		results[db]++
	}

	// Both replicas should get roughly equal load (40-60 range is acceptable)
	for db, count := range results {
		if count < 40 || count > 60 {
			t.Errorf("replica %p got %d requests, expected 40-60", db, count)
		}
	}

	// All connections should be released
	if balancer.GetActiveConnections(db1) != 0 {
		t.Errorf("db1 has %d active connections, expected 0", balancer.GetActiveConnections(db1))
	}
	if balancer.GetActiveConnections(db2) != 0 {
		t.Errorf("db2 has %d active connections, expected 0", balancer.GetActiveConnections(db2))
	}
}

func TestLeastConnectionsBalancer_OnError(t *testing.T) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	balancer := NewLeastConnectionsBalancer()
	healthy := []*gorm.DB{db1}

	selected := balancer.Select(context.Background(), healthy)
	if balancer.GetActiveConnections(selected) != 1 {
		t.Errorf("expected 1 active connection after select")
	}

	balancer.OnError(selected, gorm.ErrRecordNotFound)
	if balancer.GetActiveConnections(selected) != 0 {
		t.Errorf("expected 0 active connections after error")
	}
}

func TestReadWriteRouter_WithWeightedBalancer(t *testing.T) {
	primary, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	replica1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	replica2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	weights := map[*gorm.DB]int{
		replica1: 3,
		replica2: 1,
	}

	router := NewReadWriteRouter(primary, replica1, replica2)
	defer router.Close()

	router.SetLoadBalancer(NewWeightedRoundRobinBalancer(weights))

	rep1SQL, _ := replica1.DB()
	rep2SQL, _ := replica2.DB()

	ctx := context.Background()
	results := make(map[*sql.DB]int)

	for i := 0; i < 400; i++ {
		db := router.Read(ctx)
		s, _ := db.DB()
		results[s]++
	}

	// replica1 should get ~300 requests (75%)
	// replica2 should get ~100 requests (25%)
	if results[rep1SQL] != 300 {
		t.Errorf("replica1 got %d requests, expected 300", results[rep1SQL])
	}
	if results[rep2SQL] != 100 {
		t.Errorf("replica2 got %d requests, expected 100", results[rep2SQL])
	}
}

func TestReadWriteRouter_WithLeastConnectionsBalancer(t *testing.T) {
	primary, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	replica1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	replica2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	router := NewReadWriteRouter(primary, replica1, replica2)
	defer router.Close()

	router.SetLoadBalancer(NewLeastConnectionsBalancer())

	rep1SQL, _ := replica1.DB()
	rep2SQL, _ := replica2.DB()

	ctx := context.Background()

	// First two reads should go to different replicas
	db1 := router.Read(ctx)
	db2 := router.Read(ctx)

	s1, _ := db1.DB()
	s2, _ := db2.DB()

	if s1 == s2 {
		t.Errorf("expected first two reads to go to different replicas")
	}
	if s1 != rep1SQL && s1 != rep2SQL {
		t.Errorf("unexpected replica selected")
	}
}

func BenchmarkRoundRobinBalancer(b *testing.B) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db3, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	healthy := []*gorm.DB{db1, db2, db3}
	balancer := &RoundRobinBalancer{}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.Select(ctx, healthy)
	}
}

func BenchmarkWeightedRoundRobinBalancer(b *testing.B) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db3, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	weights := map[*gorm.DB]int{db1: 3, db2: 1, db3: 1}
	balancer := NewWeightedRoundRobinBalancer(weights)
	healthy := []*gorm.DB{db1, db2, db3}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.Select(ctx, healthy)
	}
}

func BenchmarkLeastConnectionsBalancer(b *testing.B) {
	db1, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db2, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db3, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	balancer := NewLeastConnectionsBalancer()
	healthy := []*gorm.DB{db1, db2, db3}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selected := balancer.Select(ctx, healthy)
		balancer.OnSuccess(selected, time.Microsecond)
	}
}
