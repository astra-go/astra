// Package redis provides a Redis-backed session.Store implementation.
//
// Session data is serialized as JSON and stored in Redis with a configurable
// key prefix and TTL. Compatible with Redis standalone, Sentinel, and Cluster
// (uses redis.UniversalClient).
//
// # Usage
//
//	import (
//	    "github.com/astra-go/astra/session"
//	    sessredis "github.com/astra-go/astra/session/redis"
//	)
//
//	store := sessredis.New(redisClient, sessredis.Config{
//	    KeyPrefix: "myapp:sess:",
//	})
//	app.Use(session.Middleware(store, session.Config{
//	    SecretKey:    os.Getenv("SESSION_SECRET"),
//	    Secure:       true,
//	    CookieMaxAge: 86400,
//	}))
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/astra-go/astra/session"
)

// Config configures the Redis session store.
type Config struct {
	// KeyPrefix is prepended to every session key in Redis.
	// Default: "session:".
	KeyPrefix string
}

// Store is a Redis-backed session.Store.
type Store struct {
	client    redis.UniversalClient
	keyPrefix string
}

// New creates a Redis-backed Store.
func New(client redis.UniversalClient, cfgs ...Config) *Store {
	cfg := Config{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "session:"
	}
	return &Store{client: client, keyPrefix: cfg.KeyPrefix}
}

// Load retrieves session data by ID from Redis.
// Returns an empty non-nil map when the key does not exist.
func (s *Store) Load(ctx context.Context, id string) (map[string]any, error) {
	raw, err := s.client.Get(ctx, s.key(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return make(map[string]any), nil
		}
		return nil, fmt.Errorf("session/redis: Load %q: %w", id, err)
	}

	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("session/redis: Load %q decode: %w", id, err)
	}
	if values == nil {
		values = make(map[string]any)
	}
	return values, nil
}

// Save persists session data to Redis with the given TTL.
func (s *Store) Save(ctx context.Context, id string, values map[string]any, ttl time.Duration) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("session/redis: Save %q encode: %w", id, err)
	}
	if err := s.client.Set(ctx, s.key(id), raw, ttl).Err(); err != nil {
		return fmt.Errorf("session/redis: Save %q: %w", id, err)
	}
	return nil
}

// Delete removes the session from Redis.
func (s *Store) Delete(ctx context.Context, id string) error {
	if err := s.client.Del(ctx, s.key(id)).Err(); err != nil {
		return fmt.Errorf("session/redis: Delete %q: %w", id, err)
	}
	return nil
}

func (s *Store) key(id string) string {
	return s.keyPrefix + id
}

// Verify Store implements session.Store at compile time.
var _ session.Store = (*Store)(nil)
