package redis_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/astra-go/astra/taskqueue"
	tqredis "github.com/astra-go/astra/taskqueue/redis"
)

// Set REDIS_ADDR=localhost:6379 to run these tests.
func requireRedis(t *testing.T) *goredis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("set REDIS_ADDR to run Redis integration tests")
	}
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		PoolSize: 10,
	})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func newTestBroker(t *testing.T) *tqredis.Broker {
	t.Helper()
	rdb := requireRedis(t)
	b := tqredis.NewFromClient(rdb, "test-tq")
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// writePoisonTask injects a malformed task directly to Redis for testing.
func writePoisonTask(t *testing.T, rdb *goredis.Client, taskID string, queue string, data []byte) {
	t.Helper()
	ctx := context.Background()

	// Write the task data directly to the task key.
	taskKey := "test-tq:task:" + taskID
	if err := rdb.Set(ctx, taskKey, data, 0).Err(); err != nil {
		t.Fatalf("write poison task: %v", err)
	}

	// Add the task ID to the pending queue.
	pendingKey := "test-tq:" + queue + ":pending"
	if err := rdb.LPush(ctx, pendingKey, taskID).Err(); err != nil {
		t.Fatalf("push poison task to pending queue: %v", err)
	}

	// Register the queue.
	queuesKey := "test-tq:queues"
	if err := rdb.SAdd(ctx, queuesKey, queue).Err(); err != nil {
		t.Fatalf("register queue: %v", err)
	}
}

func TestRedis_EnqueueDequeueAck(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]string{"hello": "redis"})
	task := taskqueue.NewTask("test:type", payload, taskqueue.WithQueue("default"))

	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("task ID mismatch: got %q, want %q", got.ID, task.ID)
	}
	if got.State != taskqueue.StateActive {
		t.Errorf("state = %q, want active", got.State)
	}

	if err := b.Ack(ctx, got); err != nil {
		t.Fatalf("Ack: %v", err)
	}
}

func TestRedis_NackDead(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	task := taskqueue.NewTask("test:dead", nil, taskqueue.WithQueue("default"))
	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}

	// Nack with zero retryAt → dead letter.
	if err := b.Nack(ctx, got, "simulated failure", time.Time{}); err != nil {
		t.Fatalf("Nack (dead): %v", err)
	}
}

func TestRedis_Schedule(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	// Enqueue a scheduled task.
	task := taskqueue.NewTask("test:scheduled", nil,
		taskqueue.WithQueue("default"),
		taskqueue.WithProcessIn(time.Second),
	)
	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Task should not be in pending queue yet.
	if got, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute)); err == nil {
		t.Fatalf("unexpectedly dequeued scheduled task: %+v", got)
	}

	// Wait for scheduled time.
	time.Sleep(2 * time.Second)

	// Schedule should promote the task to pending.
	if err := b.Schedule(ctx); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	// Now the task should be available.
	got, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Dequeue after schedule: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("task ID mismatch: got %q, want %q", got.ID, task.ID)
	}
}

func TestRedis_ReapStale(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	task := taskqueue.NewTask("test:reap", nil, taskqueue.WithQueue("default"))
	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}

	// Simulate a crash by not acking the task.
	// Wait for the deadline to expire.
	time.Sleep(2 * time.Second)

	// ReapStale should recover the task.
	if err := b.ReapStale(ctx); err != nil {
		t.Fatalf("ReapStale: %v", err)
	}

	// The task should be available again.
	recovered, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Dequeue after reap: %v", err)
	}
	if recovered.ID != task.ID {
		t.Errorf("task ID mismatch: got %q, want %q", recovered.ID, task.ID)
	}
}

// ─── Poison message / Validate() tests ─────────────────────────────────────────

func TestRedis_DequeueInvalidJSON(t *testing.T) {
	rdb := requireRedis(t)
	b := newTestBroker(t)
	ctx := context.Background()

	// Write malformed JSON directly to Redis.
	writePoisonTask(t, rdb, "poison-1", "default", []byte(`{invalid json`))

	// Dequeue should return an error (unmarshal failure).
	_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}

	// The error should mention unmarshal.
	if !errors.Is(err, nil) && !containsString(err.Error(), "unmarshal") && !containsString(err.Error(), "invalid character") {
		t.Errorf("error should mention unmarshal: %v", err)
	}

	// Verify the poison message was removed from the pending queue.
	pendingKey := "test-tq:default:pending"
	length := rdb.LLen(ctx, pendingKey).Val()
	if length != 0 {
		t.Errorf("expected pending queue to be empty, got %d", length)
	}
}

func TestRedis_DequeueMissingID(t *testing.T) {
	rdb := requireRedis(t)
	b := newTestBroker(t)
	ctx := context.Background()

	// Create a task with all fields except ID.
	task := map[string]any{
		"Type":    "test:type",
		"Queue":   "default",
		"Payload": []byte(`{"data": "value"}`),
		"State":   "pending",
	}
	body, _ := json.Marshal(task)
	writePoisonTask(t, rdb, "poison-2", "default", body)

	// Dequeue should return a validation error.
	_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
	if !containsString(err.Error(), "invalid task") && !containsString(err.Error(), "missing ID") {
		t.Errorf("error should mention invalid task or missing ID: %v", err)
	}

	// Verify the poison message was removed from the pending queue.
	pendingKey := "test-tq:default:pending"
	length := rdb.LLen(ctx, pendingKey).Val()
	if length != 0 {
		t.Errorf("expected pending queue to be empty, got %d", length)
	}
}

func TestRedis_DequeueMissingType(t *testing.T) {
	rdb := requireRedis(t)
	b := newTestBroker(t)
	ctx := context.Background()

	// Create a task with all fields except Type.
	task := map[string]any{
		"ID":      "task-123",
		"Queue":   "default",
		"Payload": []byte(`{"data": "value"}`),
		"State":   "pending",
	}
	body, _ := json.Marshal(task)
	writePoisonTask(t, rdb, "poison-3", "default", body)

	// Dequeue should return a validation error.
	_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected error for missing Type, got nil")
	}
	if !containsString(err.Error(), "invalid task") && !containsString(err.Error(), "missing Type") {
		t.Errorf("error should mention invalid task or missing Type: %v", err)
	}

	// Verify the poison message was removed from the pending queue.
	pendingKey := "test-tq:default:pending"
	length := rdb.LLen(ctx, pendingKey).Val()
	if length != 0 {
		t.Errorf("expected pending queue to be empty, got %d", length)
	}
}

func TestRedis_DequeueMissingQueue(t *testing.T) {
	rdb := requireRedis(t)
	b := newTestBroker(t)
	ctx := context.Background()

	// Create a task with all fields except Queue.
	task := map[string]any{
		"ID":      "task-123",
		"Type":    "test:type",
		"Payload": []byte(`{"data": "value"}`),
		"State":   "pending",
	}
	body, _ := json.Marshal(task)
	writePoisonTask(t, rdb, "poison-4", "default", body)

	// Dequeue should return a validation error.
	_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected error for missing Queue, got nil")
	}
	if !containsString(err.Error(), "invalid task") && !containsString(err.Error(), "missing Queue") {
		t.Errorf("error should mention invalid task or missing Queue: %v", err)
	}

	// Verify the poison message was removed from the pending queue.
	pendingKey := "test-tq:default:pending"
	length := rdb.LLen(ctx, pendingKey).Val()
	if length != 0 {
		t.Errorf("expected pending queue to be empty, got %d", length)
	}
}

func TestRedis_PoisonMessageRemoved(t *testing.T) {
	rdb := requireRedis(t)
	b := newTestBroker(t)
	ctx := context.Background()

	// Write multiple poison tasks.
	for range 3 {
		taskID := "poison-batch-" + string(rune('a'+0))
		writePoisonTask(t, rdb, taskID, "default", []byte(`{poison message `))
	}

	// Dequeue should return errors for all poison tasks and remove them.
	attempts := 0
	maxAttempts := 10
	for attempts < maxAttempts {
		_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if errors.Is(err, taskqueue.ErrNoTask) {
			// Queue is empty - poison tasks were properly removed.
			break
		}
		if err != nil {
			// Got an error (expected for poison tasks), continue.
			attempts++
			continue
		}
		t.Fatalf("unexpected success dequeueing poison task")
	}

	if attempts >= maxAttempts {
		t.Fatalf("did not empty queue after %d dequeue attempts", maxAttempts)
	}

	if attempts != 3 {
		t.Errorf("expected to process 3 poison tasks, got %d", attempts)
	}

	// Verify the pending queue is empty.
	pendingKey := "test-tq:default:pending"
	length := rdb.LLen(ctx, pendingKey).Val()
	if length != 0 {
		t.Errorf("expected pending queue to be empty, got %d", length)
	}
}

// Helper to check if a string contains a substring (case-insensitive).
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
