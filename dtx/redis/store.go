// Package redis provides a Redis-backed dtx.StateStore and dtx.Recovery for
// the Astra Saga orchestration pattern.
//
// # Key layout
//
//	{prefix}:{sagaID}:status       STRING  — "running" | "failed" | "done"
//	{prefix}:{sagaID}:failed_step  STRING  — name of the step that failed
//	{prefix}:{sagaID}:updated_at   STRING  — RFC3339 timestamp of last transition
//	{prefix}:{sagaID}:completed    SET     — step names whose Forward succeeded
//	{prefix}:{sagaID}:compensated  HSET    — step → "ok" | "err"
//
// All keys for a saga share the same TTL (default 7 days) so they expire
// together. Successful sagas are deleted immediately after Execute returns to
// avoid unbounded growth.
//
// # Usage
//
//	import dtxredis "github.com/astra-go/astra/dtx/redis"
//
//	store := dtxredis.NewStateStore(redisClient, dtxredis.Config{})
//
//	saga := dtx.New(steps...).
//	    WithSagaID("order-42").
//	    WithStateStore(store)
//
//	result := saga.Execute(ctx)
//
// # Recovery
//
//	recovery := dtxredis.NewRecovery(redisClient, dtxredis.Config{})
//
//	incomplete, err := recovery.ListIncomplete(ctx, 30*time.Minute)
//	for _, rec := range incomplete {
//	    // re-run compensation for rec.SagaID using your application logic
//	}
//
// # Security
//
// SagaIDs are used directly as Redis key segments. Do not derive them from
// untrusted user input. If you must, hash or URL-encode the value first.
package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/astra-go/astra/dtx"
)

const (
	defaultKeyPrefix = "dtx"
	defaultTTL       = 7 * 24 * time.Hour
)

// Config holds options for the Redis StateStore and Recovery.
type Config struct {
	// KeyPrefix is prepended to every Redis key. Default: "dtx".
	// Use this to namespace keys when sharing a Redis instance between services.
	// Security: do not derive KeyPrefix from external input.
	KeyPrefix string

	// TTL is how long saga state is retained in Redis after the last transition.
	// Default: 7 days. Set to 0 to use the default.
	TTL time.Duration
}

func (c *Config) setDefaults() {
	if c.KeyPrefix == "" {
		c.KeyPrefix = defaultKeyPrefix
	}
	if c.TTL == 0 {
		c.TTL = defaultTTL
	}
}

// StateStore is a Redis-backed dtx.StateStore.
type StateStore struct {
	client goredis.UniversalClient
	cfg    Config
}

// NewStateStore creates a Redis-backed StateStore.
// The caller owns the client lifecycle; call client.Close() when done.
func NewStateStore(client goredis.UniversalClient, cfg Config) *StateStore {
	cfg.setDefaults()
	return &StateStore{client: client, cfg: cfg}
}

// Ensure *StateStore satisfies dtx.StateStore at compile time.
var _ dtx.StateStore = (*StateStore)(nil)

func (s *StateStore) key(sagaID, suffix string) string {
	return s.cfg.KeyPrefix + ":" + sagaID + ":" + suffix
}

// OnStepCompleted records that step completed successfully.
// Errors are silently swallowed — persistence failures do not alter saga flow.
func (s *StateStore) OnStepCompleted(ctx context.Context, sagaID, step string) {
	pipe := s.client.Pipeline()
	pipe.SAdd(ctx, s.key(sagaID, "completed"), step)
	pipe.Set(ctx, s.key(sagaID, "status"), "running", s.cfg.TTL)
	pipe.Set(ctx, s.key(sagaID, "updated_at"), time.Now().UTC().Format(time.RFC3339), s.cfg.TTL)
	pipe.Expire(ctx, s.key(sagaID, "completed"), s.cfg.TTL)
	_, _ = pipe.Exec(ctx)
}

// OnStepCompensated records the outcome of a compensation attempt.
func (s *StateStore) OnStepCompensated(ctx context.Context, sagaID, step string, err error) {
	val := "ok"
	if err != nil {
		val = "err"
	}
	pipe := s.client.Pipeline()
	pipe.HSet(ctx, s.key(sagaID, "compensated"), step, val)
	pipe.Set(ctx, s.key(sagaID, "updated_at"), time.Now().UTC().Format(time.RFC3339), s.cfg.TTL)
	pipe.Expire(ctx, s.key(sagaID, "compensated"), s.cfg.TTL)
	_, _ = pipe.Exec(ctx)
}

// OnSagaFailed records that the saga entered the failed state and compensation
// is about to begin.
func (s *StateStore) OnSagaFailed(ctx context.Context, sagaID, failedStep string, _ error) {
	now := time.Now().UTC().Format(time.RFC3339)
	pipe := s.client.Pipeline()
	pipe.Set(ctx, s.key(sagaID, "status"), "failed", s.cfg.TTL)
	pipe.Set(ctx, s.key(sagaID, "failed_step"), failedStep, s.cfg.TTL)
	pipe.Set(ctx, s.key(sagaID, "updated_at"), now, s.cfg.TTL)
	_, _ = pipe.Exec(ctx)
}

// MarkSucceeded deletes all state for a successfully completed saga.
// Call this from your application after Execute returns a successful result
// to prevent unbounded key growth.
func (s *StateStore) MarkSucceeded(ctx context.Context, sagaID string) error {
	keys := []string{
		s.key(sagaID, "status"),
		s.key(sagaID, "failed_step"),
		s.key(sagaID, "updated_at"),
		s.key(sagaID, "completed"),
		s.key(sagaID, "compensated"),
	}
	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("dtx/redis: MarkSucceeded %q: %w", sagaID, err)
	}
	return nil
}

// ─── Recovery ─────────────────────────────────────────────────────────────────

// Recovery implements dtx.Recovery using a Redis SCAN over status keys.
type Recovery struct {
	client goredis.UniversalClient
	cfg    Config
}

// NewRecovery creates a Redis-backed Recovery.
func NewRecovery(client goredis.UniversalClient, cfg Config) *Recovery {
	cfg.setDefaults()
	return &Recovery{client: client, cfg: cfg}
}

// Ensure *Recovery satisfies dtx.Recovery at compile time.
var _ dtx.Recovery = (*Recovery)(nil)

// ListIncomplete scans Redis for sagas with status "failed" whose last
// transition is older than staleAfter. It uses SCAN to avoid blocking the
// server on large keyspaces.
func (r *Recovery) ListIncomplete(ctx context.Context, staleAfter time.Duration) ([]dtx.IncompleteRecord, error) {
	pattern := r.cfg.KeyPrefix + ":*:status"
	threshold := time.Now().UTC().Add(-staleAfter)

	var results []dtx.IncompleteRecord
	var cursor uint64

	for {
		keys, next, err := r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("dtx/redis: ListIncomplete scan: %w", err)
		}

		for _, statusKey := range keys {
			sagaID := r.extractSagaID(statusKey)
			if sagaID == "" {
				continue
			}

			status, err := r.client.Get(ctx, statusKey).Result()
			if err != nil || status != "failed" {
				continue
			}

			updatedAtStr, err := r.client.Get(ctx, r.cfg.KeyPrefix+":"+sagaID+":updated_at").Result()
			if err != nil {
				continue
			}
			updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
			if err != nil || updatedAt.After(threshold) {
				continue
			}

			failedStep, _ := r.client.Get(ctx, r.cfg.KeyPrefix+":"+sagaID+":failed_step").Result()
			results = append(results, dtx.IncompleteRecord{
				SagaID:     sagaID,
				FailedStep: failedStep,
				UpdatedAt:  updatedAt,
			})
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}

	return results, nil
}

// MarkDone sets the saga status to "done" so it is no longer returned by
// ListIncomplete. Use after human intervention or confirmed safe-to-ignore.
func (r *Recovery) MarkDone(ctx context.Context, sagaID string) error {
	key := r.cfg.KeyPrefix + ":" + sagaID + ":status"
	if err := r.client.Set(ctx, key, "done", r.cfg.TTL).Err(); err != nil {
		return fmt.Errorf("dtx/redis: MarkDone %q: %w", sagaID, err)
	}
	return nil
}

// extractSagaID parses "{prefix}:{sagaID}:status" → sagaID.
func (r *Recovery) extractSagaID(statusKey string) string {
	prefix := r.cfg.KeyPrefix + ":"
	suffix := ":status"
	if !strings.HasPrefix(statusKey, prefix) || !strings.HasSuffix(statusKey, suffix) {
		return ""
	}
	inner := statusKey[len(prefix) : len(statusKey)-len(suffix)]
	// Reject keys that contain additional colons beyond the sagaID itself
	// only if the sagaID itself is empty.
	if inner == "" {
		return ""
	}
	return inner
}
