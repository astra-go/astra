// Package cache provides a unified caching abstraction for Astra applications.
//
// All implementations satisfy the Cache interface, making it trivial to swap
// backends (in-process memory → Redis → Memcached) without changing application
// code.
//
// # Redis cache
//
//	import cacheredis "github.com/astra-go/astra/cache/redis"
//
//	c, err := cacheredis.New(cacheredis.Config{Addr: "localhost:6379"})
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrCacheMiss is returned when the requested key is not found or has expired.
var ErrCacheMiss = errors.New("cache: key not found")

// Cache is the unified cache interface. All implementations must be safe for
// concurrent use from multiple goroutines.
type Cache interface {
	// Get retrieves the byte value for key.
	// Returns ErrCacheMiss if the key does not exist or has expired.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores value with the given key and TTL.
	// A TTL of 0 means the entry never expires.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes the given keys. Missing keys are silently ignored.
	Delete(ctx context.Context, keys ...string) error

	// Exists reports whether key exists and has not expired.
	Exists(ctx context.Context, key string) (bool, error)

	// Flush removes all keys. Use with caution in shared caches.
	Flush(ctx context.Context) error

	// Close releases any resources held by the cache client.
	Close() error
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

// GetJSON retrieves a cached value and unmarshals it into v.
func GetJSON(ctx context.Context, c Cache, key string, v any) error {
	b, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// SetJSON serialises v to JSON and stores it with the given TTL.
func SetJSON(ctx context.Context, c Cache, key string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, b, ttl)
}

// GetOrSet returns a cached value, calling fetch on a cache miss to populate it.
// The result of fetch is marshalled to JSON, stored in the cache, then
// unmarshalled into v. A failed Set is non-fatal — v is still populated.
//
// Security: keys derived from user input must be sanitised by the caller to
// prevent cache-key injection (e.g. path traversal in prefixed key schemes).
func GetOrSet(ctx context.Context, c Cache, key string, v any, ttl time.Duration, fetch func() (any, error)) error {
	if err := GetJSON(ctx, c, key, v); err == nil {
		return nil // cache hit
	} else if !errors.Is(err, ErrCacheMiss) {
		return err // unexpected cache error
	}

	result, err := fetch()
	if err != nil {
		return err
	}

	b, err := json.Marshal(result)
	if err != nil {
		return err
	}

	// Non-fatal: a Set failure shouldn't prevent the caller from receiving data.
	_ = c.Set(ctx, key, b, ttl)

	return json.Unmarshal(b, v)
}

// ─── Convenience constructor ──────────────────────────────────────────────────

// NewMemory creates a new in-memory LRU cache.
// This is a convenience wrapper for testing and simple use cases.
//
// For production use with capacity limits, import and use:
//   import cachemem "github.com/astra-go/astra/cache/memory"
//   c := cachemem.New(cachemem.Config{Cap: 1000})
func NewMemory() Cache {
	// Avoid direct import to prevent circular dependency.
	// Tests that need this will import cache/memory directly.
	panic("cache.NewMemory() requires importing github.com/astra-go/astra/cache/memory directly")
}
