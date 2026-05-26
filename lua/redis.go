package lua

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/redis/go-redis/v9"
)

// RedisRunner executes named Lua scripts against a Redis server using
// EVALSHA with automatic fallback to EVAL on NOSCRIPT errors.
//
// It accepts a redis.Scripter, which is satisfied by *redis.Client,
// *redis.ClusterClient, *redis.Ring, and similar go-redis types.
type RedisRunner struct {
	rdb     redis.Scripter
	scripts map[string]*redis.Script
	mu      sync.RWMutex
}

// NewRedisRunner creates a new RedisRunner backed by rdb.
func NewRedisRunner(rdb redis.Scripter) *RedisRunner {
	return &RedisRunner{
		rdb:     rdb,
		scripts: make(map[string]*redis.Script),
	}
}

// Register stores an inline Lua script source under name.
// It does not validate the script; errors surface at Run time.
func (r *RedisRunner) Register(name, src string) {
	r.mu.Lock()
	r.scripts[name] = redis.NewScript(src)
	r.mu.Unlock()
}

// RegisterFile loads a Lua script from file and registers it under name.
func (r *RedisRunner) RegisterFile(name, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("lua/redis: read file %q: %w", file, err)
	}
	r.Register(name, string(data))
	return nil
}

// Run executes the named script with the given keys and args.
// EVALSHA is tried first; on a NOSCRIPT error go-redis automatically
// retries with EVAL — no extra handling is needed.
//
// If name is not registered, the returned *redis.Cmd carries a non-nil Err().
func (r *RedisRunner) Run(ctx context.Context, name string, keys []string, args ...any) *redis.Cmd {
	r.mu.RLock()
	sc, ok := r.scripts[name]
	r.mu.RUnlock()

	if !ok {
		cmd := redis.NewCmd(ctx, "evalsha")
		cmd.SetErr(fmt.Errorf("lua/redis: script %q not registered", name))
		return cmd
	}
	return sc.Run(ctx, r.rdb, keys, args...)
}
