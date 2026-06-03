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
