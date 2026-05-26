// Package memcached provides a Memcached-backed cache implementation for Astra.
//
// # Usage
//
//	import "github.com/astra-go/astra/cache/memcached"
//
//	c, err := memcached.New(memcached.Config{
//	    Servers: []string{"localhost:11211"},
//	})
//	if err != nil { log.Fatal(err) }
//	defer c.Close()
//
//	c.Set(ctx, "key", []byte("hello"), time.Minute)
//	val, err := c.Get(ctx, "key")
//
// # Key prefix
//
//	c, _ := memcached.New(memcached.Config{
//	    Servers:   []string{"mc1:11211", "mc2:11211"},
//	    KeyPrefix: "myapp:",
//	})
//
// # Limitations
//
// Memcached keys must not contain whitespace or control characters. The
// implementation validates key characters before each operation.
// Values are limited to 1 MB by default in standard Memcached deployments.
package memcached

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/bradfitz/gomemcache/memcache"

	"github.com/astra-go/astra/cache"
)

// Config holds connection options for a Memcached pool.
type Config struct {
	// Servers is the list of Memcached server addresses (host:port).
	// The client uses consistent hashing to distribute keys across servers.
	Servers []string
	// MaxIdleConns sets the maximum number of idle connections per server.
	// Default (gomemcache): 2.
	MaxIdleConns int
	// Timeout is the socket read/write deadline. Default: 100ms.
	Timeout time.Duration
	// KeyPrefix is prepended to every key, e.g. "myapp:".
	// Use this to namespace keys when sharing a Memcached cluster.
	//
	// Security: KeyPrefix must not contain whitespace or control characters.
	KeyPrefix string
}

// Cache is a Memcached-backed implementation of cache.Cache.
//
// Security notes:
//   - Memcached has no built-in authentication or TLS. Run it on a private
//     network and firewall port 11211 from untrusted hosts.
//   - Key values are limited to 250 bytes and must not contain whitespace or
//     control characters. This implementation rejects invalid keys.
//   - Flush() erases ALL keys from the Memcached cluster — never call it in
//     response to untrusted input.
type Cache struct {
	client *memcache.Client
	prefix string
}

// New creates a Memcached-backed Cache.
// Returns an error if no servers are provided or if the initial health check
// (Get on a sentinel key) fails to reach at least one server.
func New(cfg Config) (*Cache, error) {
	if len(cfg.Servers) == 0 {
		return nil, errors.New("cache/memcached: at least one server address required")
	}
	if err := validateKeyPart(cfg.KeyPrefix); err != nil {
		return nil, fmt.Errorf("cache/memcached: invalid KeyPrefix: %w", err)
	}

	client := memcache.New(cfg.Servers...)
	if cfg.MaxIdleConns > 0 {
		client.MaxIdleConns = cfg.MaxIdleConns
	}
	if cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}

	// Connectivity check: a miss on a non-existent key is fine; a network
	// error means we cannot reach any server.
	if _, err := client.Get("__astra_ping__"); err != nil && !errors.Is(err, memcache.ErrCacheMiss) {
		return nil, fmt.Errorf("cache/memcached: connect: %w", err)
	}

	return &Cache{client: client, prefix: cfg.KeyPrefix}, nil
}

func (c *Cache) k(key string) (string, error) {
	full := c.prefix + key
	if err := validateKeyPart(full); err != nil {
		return "", fmt.Errorf("cache/memcached: invalid key %q: %w", full, err)
	}
	if len(full) > 250 {
		return "", fmt.Errorf("cache/memcached: key too long (%d bytes, max 250)", len(full))
	}
	return full, nil
}

// Get retrieves the value for key.
// Returns cache.ErrCacheMiss when the key does not exist.
func (c *Cache) Get(_ context.Context, key string) ([]byte, error) {
	k, err := c.k(key)
	if err != nil {
		return nil, err
	}
	item, err := c.client.Get(k)
	if errors.Is(err, memcache.ErrCacheMiss) {
		return nil, cache.ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	return item.Value, nil
}

// Set stores value with the given key and TTL.
// A TTL of 0 means no expiry. Memcached truncates TTL to seconds.
//
// Note: Memcached interprets TTLs > 30 days as Unix timestamps; durations
// longer than 30 days will not behave as expected.
func (c *Cache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	k, err := c.k(key)
	if err != nil {
		return err
	}
	exp := int32(0)
	if ttl > 0 {
		exp = int32(ttl.Seconds())
		if exp == 0 {
			exp = 1 // round sub-second TTLs up to 1 second
		}
	}
	return c.client.Set(&memcache.Item{
		Key:        k,
		Value:      value,
		Expiration: exp,
	})
}

// Delete removes the given keys. Missing keys are silently ignored.
func (c *Cache) Delete(_ context.Context, keys ...string) error {
	for _, key := range keys {
		k, err := c.k(key)
		if err != nil {
			return err
		}
		if err := c.client.Delete(k); err != nil && !errors.Is(err, memcache.ErrCacheMiss) {
			return err
		}
	}
	return nil
}

// Exists reports whether key exists in Memcached.
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.Get(ctx, key)
	if errors.Is(err, cache.ErrCacheMiss) {
		return false, nil
	}
	return err == nil, err
}

// Flush sends FlushAll to the Memcached cluster, removing all keys.
// This affects all namespaces sharing the cluster.
func (c *Cache) Flush(_ context.Context) error {
	return c.client.FlushAll()
}

// Close is a no-op: gomemcache manages its connection pool internally.
func (c *Cache) Close() error { return nil }

// ─── Key validation ───────────────────────────────────────────────────────────

// validateKeyPart ensures s contains no whitespace or control characters,
// as required by the Memcached protocol.
func validateKeyPart(s string) error {
	if strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) {
		return errors.New("must not contain whitespace or control characters")
	}
	return nil
}
