// Package redis provides a Redis-backed cache implementation for Astra,
// supporting both single-node and cluster deployments.
//
// # Single-node
//
//	c, err := redis.New(redis.Config{Addr: "localhost:6379"})
//	if err != nil { log.Fatal(err) }
//	defer c.Close()
//
// # Cluster
//
//	c, err := redis.NewCluster([]string{"node1:6379", "node2:6379"}, redis.Config{})
//
// # TLS (production)
//
//	c, err := redis.New(redis.Config{
//	    Addr:      "redis.example.com:6380",
//	    TLSConfig: &tls.Config{},
//	})
package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/astra-go/astra/cache"
)

// Config holds connection options for a single Redis node.
type Config struct {
	// Addr is the Redis server address (host:port). Default: "localhost:6379".
	Addr string
	// Password for the AUTH command. Leave empty when not required.
	Password string
	// DB is the Redis logical database number (0–15). Default: 0.
	DB int

	// PoolSize is the maximum number of socket connections.
	// Default (go-redis): 10 per CPU.
	PoolSize int
	// MinIdleConns is the minimum number of idle connections maintained.
	MinIdleConns int

	// DialTimeout is the timeout for establishing a new connection.
	DialTimeout time.Duration
	// ReadTimeout is the timeout for socket reads. Default: 3 seconds.
	ReadTimeout time.Duration
	// WriteTimeout is the timeout for socket writes. Default: ReadTimeout.
	WriteTimeout time.Duration

	// KeyPrefix is prepended to every key, e.g. "myapp:".
	// Use this to namespace keys when sharing a Redis instance between services.
	//
	// Security: when KeyPrefix is derived from external input, validate it to
	// prevent key-namespace escaping.
	KeyPrefix string

	// TLSConfig enables TLS. Use &tls.Config{} for default settings with server
	// certificate verification. Required for Redis over TLS / Redis Cloud.
	TLSConfig *tls.Config
}

// Cache is a Redis-backed implementation of cache.Cache.
//
// Security notes:
//   - Use TLSConfig in production to encrypt traffic between the application
//     and Redis. Credentials and session tokens must never travel in plaintext.
//   - Use KeyPrefix to isolate keys between applications sharing one Redis
//     instance; without it a crafted key could overwrite another app's data.
//   - Flush() sends FLUSHDB which erases all keys in the selected logical DB;
//     never expose it to untrusted callers.
type Cache struct {
	client *goredis.Client
	prefix string
}

// New creates a Redis-backed Cache and verifies connectivity with a PING.
func New(cfg Config) (*Cache, error) {
	if cfg.Addr == "" {
		cfg.Addr = "localhost:6379"
	}
	client := goredis.NewClient(&goredis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		TLSConfig:    cfg.TLSConfig,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("cache/redis: connect %s: %w", cfg.Addr, err)
	}
	return &Cache{client: client, prefix: cfg.KeyPrefix}, nil
}

// Client returns the underlying *goredis.Client for advanced operations.
func (c *Cache) Client() *goredis.Client { return c.client }

func (c *Cache) k(key string) string { return c.prefix + key }

// Get retrieves the byte value for key.
// Returns cache.ErrCacheMiss when the key does not exist.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.client.Get(ctx, c.k(key)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, cache.ErrCacheMiss
	}
	return val, err
}

// Set stores value with the given key and TTL.
// A TTL of 0 stores the key without expiry (Redis KeepTTL semantics are NOT
// used; passing 0 explicitly removes any existing TTL).
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, c.k(key), value, ttl).Err()
}

// Delete removes the given keys. Missing keys are silently ignored.
// Uses UNLINK (async, non-blocking) instead of DEL to avoid stalling the
// Redis event loop when deleting large values.
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = c.prefix + k
	}
	return c.client.Unlink(ctx, prefixed...).Err()
}

// Exists reports whether key exists in Redis.
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, c.k(key)).Result()
	return n > 0, err
}

// Flush sends FLUSHDB, removing all keys in the current logical database.
func (c *Cache) Flush(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}

// Close closes the Redis client connection pool.
func (c *Cache) Close() error {
	return c.client.Close()
}

// ─── Cluster support ─────────────────────────────────────────────────────────

// ClusterCache is a Redis Cluster-backed implementation of cache.Cache.
type ClusterCache struct {
	client *goredis.ClusterClient
	prefix string
}

// NewCluster creates a Redis Cluster-backed Cache and verifies connectivity.
//
//	c, err := redis.NewCluster([]string{"node1:6379", "node2:6379"}, redis.Config{})
func NewCluster(addrs []string, cfg Config) (*ClusterCache, error) {
	if len(addrs) == 0 {
		return nil, errors.New("cache/redis: cluster addrs must not be empty")
	}
	client := goredis.NewClusterClient(&goredis.ClusterOptions{
		Addrs:        addrs,
		Password:     cfg.Password,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		TLSConfig:    cfg.TLSConfig,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("cache/redis: cluster connect: %w", err)
	}
	return &ClusterCache{client: client, prefix: cfg.KeyPrefix}, nil
}

// Client returns the underlying *goredis.ClusterClient.
func (c *ClusterCache) Client() *goredis.ClusterClient { return c.client }

func (c *ClusterCache) k(key string) string { return c.prefix + key }

func (c *ClusterCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.client.Get(ctx, c.k(key)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, cache.ErrCacheMiss
	}
	return val, err
}

func (c *ClusterCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, c.k(key), value, ttl).Err()
}

func (c *ClusterCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = c.prefix + k
	}
	return c.client.Unlink(ctx, prefixed...).Err()
}

func (c *ClusterCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, c.k(key)).Result()
	return n > 0, err
}

func (c *ClusterCache) Flush(ctx context.Context) error {
	return c.client.ForEachShard(ctx, func(ctx context.Context, shard *goredis.Client) error {
		return shard.FlushDB(ctx).Err()
	})
}

func (c *ClusterCache) Close() error {
	return c.client.Close()
}
