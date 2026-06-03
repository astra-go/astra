//go:build lua

package lua_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"github.com/astra-go/astra/rule/lua"
)

// redisAddr returns the Redis address from REDIS_ADDR env, or skips the test.
func redisAddr(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set; skipping Redis integration tests")
	}
	return addr
}

// newClient creates a Redis client for testing.
func newClient(t *testing.T) *goredis.Client {
	t.Helper()
	rdb := goredis.NewClient(&goredis.Options{Addr: redisAddr(t)})
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestRedisRunner_Register_Run(t *testing.T) {
	rdb := newClient(t)
	runner := lua.NewRedisRunner(rdb)

	// Simple script: return the first key.
	runner.Register("echo_key", `return KEYS[1]`)

	ctx := context.Background()
	cmd := runner.Run(ctx, "echo_key", []string{"my-key"})
	if err := cmd.Err(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	val, err := cmd.Text()
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if val != "my-key" {
		t.Errorf("want 'my-key', got %q", val)
	}
}

func TestRedisRunner_RegisterFile_Run(t *testing.T) {
	rdb := newClient(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "sum.lua")
	if err := os.WriteFile(path, []byte(`return tonumber(ARGV[1]) + tonumber(ARGV[2])`), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	runner := lua.NewRedisRunner(rdb)
	if err := runner.RegisterFile("sum", path); err != nil {
		t.Fatalf("RegisterFile: %v", err)
	}

	ctx := context.Background()
	cmd := runner.Run(ctx, "sum", nil, "3", "4")
	if err := cmd.Err(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	n, err := cmd.Int()
	if err != nil {
		t.Fatalf("Int: %v", err)
	}
	if n != 7 {
		t.Errorf("want 7, got %d", n)
	}
}

func TestRedisRunner_UnknownScript_Error(t *testing.T) {
	rdb := newClient(t)
	runner := lua.NewRedisRunner(rdb)

	cmd := runner.Run(context.Background(), "nonexistent", nil)
	if cmd.Err() == nil {
		t.Error("expected error for unregistered script name")
	}
}

func TestRedisRunner_MultipleScripts(t *testing.T) {
	rdb := newClient(t)
	runner := lua.NewRedisRunner(rdb)

	runner.Register("key_script", `return KEYS[1]`)
	runner.Register("arg_script", `return ARGV[1]`)

	ctx := context.Background()

	keyCmd := runner.Run(ctx, "key_script", []string{"thekey"})
	if err := keyCmd.Err(); err != nil {
		t.Fatalf("key_script Run: %v", err)
	}
	keyVal, _ := keyCmd.Text()
	if keyVal != "thekey" {
		t.Errorf("key_script: want 'thekey', got %q", keyVal)
	}

	argCmd := runner.Run(ctx, "arg_script", nil, "thearg")
	if err := argCmd.Err(); err != nil {
		t.Fatalf("arg_script Run: %v", err)
	}
	argVal, _ := argCmd.Text()
	if argVal != "thearg" {
		t.Errorf("arg_script: want 'thearg', got %q", argVal)
	}
}
