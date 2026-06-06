package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	gorillawebsocket "github.com/gorilla/websocket"
)

func TestPool_Get_Put(t *testing.T) {
	cfg := PoolConfig{
		MaxIdle: 10,
		Dialer: func(ctx context.Context) (*ClientConn, error) {
			return &ClientConn{
				ID:      "conn-1",
				Healthy: atomic.Bool{},
			}, nil
		},
	}

	pool := NewPool(cfg)
	defer pool.Close()

	// Get a connection (creates new, active=1).
	conn, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if conn == nil {
		t.Fatal("Get returned nil")
	}

	// Put it back (active=0, idle=1).
	pool.Put(conn)

	// Get it again (idle=0, active=1).
	conn2, _ := pool.Get(context.Background())
	if conn2 == nil {
		t.Error("expected to get connection from pool")
	}

	// Stats should show 0 idle after get.
	stats := pool.Stats()
	if stats.Active != 1 {
		t.Errorf("expected 1 active, got %d", stats.Active)
	}

	pool.Put(conn2)
}

func TestPool_MaxActive(t *testing.T) {
	cfg := PoolConfig{
		MaxIdle: 10,
		MaxActive: 2,
		Dialer: func(ctx context.Context) (*ClientConn, error) {
			return &ClientConn{Healthy: atomic.Bool{}}, nil
		},
	}

	pool := NewPool(cfg)
	defer pool.Close()

	// Get two connections.
	conn1, _ := pool.Get(context.Background())
	conn2, _ := pool.Get(context.Background())

	// Return them to the pool.
	pool.Put(conn1)
	pool.Put(conn2)

	// Get them again (should come from idle pool).
	conn3, _ := pool.Get(context.Background())
	conn4, _ := pool.Get(context.Background())

	// Both should succeed since we have idle connections.
	if conn3 == nil || conn4 == nil {
		t.Error("expected to get connections from pool")
	}

	pool.Put(conn3)
	pool.Put(conn4)
}

func TestPool_MaxIdle(t *testing.T) {
	cfg := PoolConfig{
		MaxIdle: 2,
		Dialer: func(ctx context.Context) (*ClientConn, error) {
			return &ClientConn{Healthy: atomic.Bool{}}, nil
		},
	}

	pool := NewPool(cfg)
	defer pool.Close()

	// Warm up the pool with 5 connections.
	conns := make([]*ClientConn, 5)
	for i := 0; i < 5; i++ {
		conn, _ := pool.Get(context.Background())
		conns[i] = conn
	}
	pool.PutMany(conns)

	// Only MaxIdle should remain in pool (idle=2).
	stats := pool.Stats()
	if stats.Idle != 2 {
		t.Errorf("expected 2 idle, got %d", stats.Idle)
	}
}

func TestPool_UnhealthyConnection(t *testing.T) {
	cfg := PoolConfig{
		MaxIdle: 10,
		Dialer: func(ctx context.Context) (*ClientConn, error) {
			return &ClientConn{Healthy: atomic.Bool{}}, nil
		},
	}

	pool := NewPool(cfg)
	defer pool.Close()

	conn, _ := pool.Get(context.Background())
	conn.Healthy.Store(false)
	pool.Put(conn)

	// Unhealthy connection should not be returned to pool.
	stats := pool.Stats()
	if stats.Idle != 0 {
		t.Errorf("expected 0 idle for unhealthy connection, got %d", stats.Idle)
	}
}

func TestPool_Close(t *testing.T) {
	cfg := PoolConfig{
		MaxIdle: 10,
		Dialer: func(ctx context.Context) (*ClientConn, error) {
			return &ClientConn{Healthy: atomic.Bool{}}, nil
		},
	}

	pool := NewPool(cfg)

	conn, _ := pool.Get(context.Background())
	pool.Put(conn)
	pool.Close()

	// Get after close should fail.
	_, err := pool.Get(context.Background())
	if err == nil {
		t.Error("expected error after close")
	}
}

func TestPoolStats(t *testing.T) {
	cfg := PoolConfig{
		MaxIdle:   100,
		MaxActive: 1000,
		Dialer: func(ctx context.Context) (*ClientConn, error) {
			return &ClientConn{Healthy: atomic.Bool{}}, nil
		},
	}

	pool := NewPool(cfg)
	defer pool.Close()

	stats := pool.Stats()
	if stats.MaxIdle != 100 {
		t.Errorf("expected MaxIdle=100, got %d", stats.MaxIdle)
	}
	if stats.MaxActive != 1000 {
		t.Errorf("expected MaxActive=1000, got %d", stats.MaxActive)
	}
}

func TestReconnectingPool_Connect(t *testing.T) {
	// Start a WebSocket server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := gorillawebsocket.Upgrader{}
		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()
	}))
	defer server.Close()

	url := "ws" + server.URL[4:] // http:// → ws://

	// Create a real dialer.
	dialer := gorillawebsocket.Dialer{}

	pool := NewReconnectingPool(url, PoolConfig{
		MaxReconnect: 1, // Only 1 attempt for fast test.
		Dialer: func(ctx context.Context) (*ClientConn, error) {
			conn, _, err := dialer.DialContext(ctx, url, nil)
			if err != nil {
				return nil, err
			}
			return &ClientConn{
				Conn:    conn,
				Healthy: atomic.Bool{},
			}, nil
		},
	})
	defer pool.Close()

	ctx := context.Background()
	if err := pool.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if pool.Conn() == nil {
		t.Error("expected connection")
	}
}

// BenchmarkPoolGetPut benchmarks pool get/put operations.
func BenchmarkPoolGetPut(b *testing.B) {
	cfg := PoolConfig{
		MaxIdle: 100,
		Dialer: func(ctx context.Context) (*ClientConn, error) {
			return &ClientConn{Healthy: atomic.Bool{}}, nil
		},
	}

	pool := NewPool(cfg)
	defer pool.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, _ := pool.Get(context.Background())
		pool.Put(conn)
	}
}

// BenchmarkPoolConcurrentGetPut benchmarks concurrent pool access.
func BenchmarkPoolConcurrentGetPut(b *testing.B) {
	cfg := PoolConfig{
		MaxIdle:   100,
		MaxActive: 1000,
		Dialer: func(ctx context.Context) (*ClientConn, error) {
			return &ClientConn{Healthy: atomic.Bool{}}, nil
		},
	}

	pool := NewPool(cfg)
	defer pool.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, _ := pool.Get(context.Background())
			pool.Put(conn)
		}
	})
}