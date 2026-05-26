package redis_test

import (
	"context"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/astra-go/astra/dtx"
	dtxredis "github.com/astra-go/astra/dtx/redis"
)

// redisClient returns a test Redis client.
// Set REDIS_ADDR to override the default "localhost:6379".
// Tests are skipped when Redis is not reachable.
func redisClient(t *testing.T) goredis.UniversalClient {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("dtx/redis: skipping integration tests — Redis not available at %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func uniquePrefix(t *testing.T) string {
	return "dtxtest:" + t.Name() + ":"
}

func flushPrefix(t *testing.T, client goredis.UniversalClient, prefix string) {
	t.Helper()
	ctx := context.Background()
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			break
		}
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
}

// ─── StateStore ───────────────────────────────────────────────────────────────

func TestStateStore_SuccessPath(t *testing.T) {
	client := redisClient(t)
	prefix := uniquePrefix(t)
	t.Cleanup(func() { flushPrefix(t, client, prefix) })

	store := dtxredis.NewStateStore(client, dtxredis.Config{KeyPrefix: prefix + "dtx"})

	ctx := context.Background()
	store.OnStepCompleted(ctx, "saga-1", "step-a")
	store.OnStepCompleted(ctx, "saga-1", "step-b")

	members, err := client.SMembers(ctx, prefix+"dtx:saga-1:completed").Result()
	if err != nil {
		t.Fatalf("SMembers: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 completed members, got %v", members)
	}

	status, err := client.Get(ctx, prefix+"dtx:saga-1:status").Result()
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	if status != "running" {
		t.Errorf("expected status=running, got %q", status)
	}
}

func TestStateStore_FailurePath(t *testing.T) {
	client := redisClient(t)
	prefix := uniquePrefix(t)
	t.Cleanup(func() { flushPrefix(t, client, prefix) })

	store := dtxredis.NewStateStore(client, dtxredis.Config{KeyPrefix: prefix + "dtx"})

	ctx := context.Background()
	store.OnStepCompleted(ctx, "saga-2", "step-a")
	store.OnSagaFailed(ctx, "saga-2", "step-b", nil)
	store.OnStepCompensated(ctx, "saga-2", "step-a", nil)

	status, _ := client.Get(ctx, prefix+"dtx:saga-2:status").Result()
	if status != "failed" {
		t.Errorf("expected status=failed, got %q", status)
	}

	failedStep, _ := client.Get(ctx, prefix+"dtx:saga-2:failed_step").Result()
	if failedStep != "step-b" {
		t.Errorf("expected failed_step=step-b, got %q", failedStep)
	}

	compVal, _ := client.HGet(ctx, prefix+"dtx:saga-2:compensated", "step-a").Result()
	if compVal != "ok" {
		t.Errorf("expected compensated[step-a]=ok, got %q", compVal)
	}
}

func TestStateStore_CompensationError_RecordedAsErr(t *testing.T) {
	client := redisClient(t)
	prefix := uniquePrefix(t)
	t.Cleanup(func() { flushPrefix(t, client, prefix) })

	store := dtxredis.NewStateStore(client, dtxredis.Config{KeyPrefix: prefix + "dtx"})

	ctx := context.Background()
	store.OnStepCompleted(ctx, "saga-3", "step-a")
	store.OnSagaFailed(ctx, "saga-3", "step-b", nil)
	store.OnStepCompensated(ctx, "saga-3", "step-a", errFake)

	val, _ := client.HGet(ctx, prefix+"dtx:saga-3:compensated", "step-a").Result()
	if val != "err" {
		t.Errorf("expected compensated[step-a]=err, got %q", val)
	}
}

var errFake = &fakeErr{}

type fakeErr struct{}

func (e *fakeErr) Error() string { return "fake error" }

func TestStateStore_MarkSucceeded_DeletesKeys(t *testing.T) {
	client := redisClient(t)
	prefix := uniquePrefix(t)
	t.Cleanup(func() { flushPrefix(t, client, prefix) })

	store := dtxredis.NewStateStore(client, dtxredis.Config{KeyPrefix: prefix + "dtx"})

	ctx := context.Background()
	store.OnStepCompleted(ctx, "saga-4", "step-a")

	if err := store.MarkSucceeded(ctx, "saga-4"); err != nil {
		t.Fatalf("MarkSucceeded: %v", err)
	}

	n, _ := client.Exists(ctx, prefix+"dtx:saga-4:status").Result()
	if n != 0 {
		t.Error("expected status key to be deleted after MarkSucceeded")
	}
}

// ─── Integration with dtx.Saga ────────────────────────────────────────────────

func TestStateStore_IntegrationWithSaga(t *testing.T) {
	client := redisClient(t)
	prefix := uniquePrefix(t)
	t.Cleanup(func() { flushPrefix(t, client, prefix) })

	store := dtxredis.NewStateStore(client, dtxredis.Config{KeyPrefix: prefix + "dtx"})

	saga := dtx.New(
		dtx.Step{
			Name:       "a",
			Forward:    func(_ context.Context) error { return nil },
			Compensate: func(_ context.Context) error { return nil },
		},
		dtx.Step{
			Name:    "b",
			Forward: func(_ context.Context) error { return errFake },
		},
	).WithSagaID("integ-1").WithStateStore(store)

	result := saga.Execute(context.Background())
	if result.Succeeded() {
		t.Fatal("expected saga to fail")
	}

	ctx := context.Background()
	status, _ := client.Get(ctx, prefix+"dtx:integ-1:status").Result()
	if status != "failed" {
		t.Errorf("expected status=failed after saga failure, got %q", status)
	}
}

// ─── Recovery ─────────────────────────────────────────────────────────────────

func TestRecovery_ListIncomplete_ReturnsStaleFailed(t *testing.T) {
	client := redisClient(t)
	prefix := uniquePrefix(t)
	t.Cleanup(func() { flushPrefix(t, client, prefix) })

	cfg := dtxredis.Config{KeyPrefix: prefix + "dtx"}
	store := dtxredis.NewStateStore(client, cfg)
	recovery := dtxredis.NewRecovery(client, cfg)

	ctx := context.Background()

	// Simulate a saga that failed 1 hour ago.
	store.OnSagaFailed(ctx, "stale-saga", "step-x", nil)
	// Backdate updated_at to simulate staleness.
	staleTime := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	client.Set(ctx, prefix+"dtx:stale-saga:updated_at", staleTime, cfg.TTL)

	// A fresh saga that failed just now — should NOT appear.
	store.OnSagaFailed(ctx, "fresh-saga", "step-y", nil)

	incomplete, err := recovery.ListIncomplete(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("ListIncomplete: %v", err)
	}

	found := false
	for _, rec := range incomplete {
		if rec.SagaID == "stale-saga" {
			found = true
			if rec.FailedStep != "step-x" {
				t.Errorf("expected FailedStep=step-x, got %q", rec.FailedStep)
			}
		}
		if rec.SagaID == "fresh-saga" {
			t.Error("fresh-saga should not appear in ListIncomplete")
		}
	}
	if !found {
		t.Error("stale-saga not found in ListIncomplete")
	}
}

func TestRecovery_MarkDone_RemovesFromList(t *testing.T) {
	client := redisClient(t)
	prefix := uniquePrefix(t)
	t.Cleanup(func() { flushPrefix(t, client, prefix) })

	cfg := dtxredis.Config{KeyPrefix: prefix + "dtx"}
	store := dtxredis.NewStateStore(client, cfg)
	recovery := dtxredis.NewRecovery(client, cfg)

	ctx := context.Background()
	store.OnSagaFailed(ctx, "done-saga", "step-z", nil)
	staleTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	client.Set(ctx, prefix+"dtx:done-saga:updated_at", staleTime, cfg.TTL)

	if err := recovery.MarkDone(ctx, "done-saga"); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}

	incomplete, err := recovery.ListIncomplete(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("ListIncomplete: %v", err)
	}
	for _, rec := range incomplete {
		if rec.SagaID == "done-saga" {
			t.Error("done-saga should not appear after MarkDone")
		}
	}
}
