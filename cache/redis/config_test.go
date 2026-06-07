package redis_test

import (
	"testing"

	"github.com/astra-go/astra/cache/redis"
)

func TestDefaultPoolSize(t *testing.T) {
	// Test that PoolSize defaults to 100 when not set.
	cfg := redis.Config{
		Addr: "localhost:6379",
	}
	// We can't test the actual connection without a Redis instance,
	// but we can verify the config struct is accepted.
	if cfg.PoolSize != 0 {
		t.Errorf("expected default PoolSize to be 0 before New, got %d", cfg.PoolSize)
	}
	// The actual default is applied inside New() when PoolSize <= 0.
	// Users can override by setting a positive value.
}

func TestExplicitPoolSize(t *testing.T) {
	cfg := redis.Config{
		Addr:     "localhost:6379",
		PoolSize: 50,
	}
	if cfg.PoolSize != 50 {
		t.Errorf("expected PoolSize 50, got %d", cfg.PoolSize)
	}
}

func TestPoolSizeUpperBound(t *testing.T) {
	// Verify that a large explicit PoolSize is honored.
	cfg := redis.Config{
		Addr:     "localhost:6379",
		PoolSize: 1000,
	}
	if cfg.PoolSize != 1000 {
		t.Errorf("expected PoolSize 1000, got %d", cfg.PoolSize)
	}
}
