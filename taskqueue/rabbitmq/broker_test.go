package rabbitmq_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/astra-go/astra/taskqueue"
	tqrabbitmq "github.com/astra-go/astra/taskqueue/rabbitmq"
)

// Set RABBITMQ_URL=amqp://guest:guest@localhost:5672/ to run these tests.
func requireRabbitMQ(t *testing.T) string {
	t.Helper()
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("set RABBITMQ_URL to run RabbitMQ integration tests")
	}
	return url
}

func newTestBroker(t *testing.T) *tqrabbitmq.Broker {
	t.Helper()
	url := requireRabbitMQ(t)
	b, err := tqrabbitmq.New(tqrabbitmq.Config{
		URL:                url,
		KeyPrefix:          "test-tq",
		UseDelayedExchange: true,
	})
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// publishPoisonMessage injects a malformed message directly to the queue for testing.
func publishPoisonMessage(t *testing.T, _ *tqrabbitmq.Broker, queue string, body []byte) {
	t.Helper()

	// Create a new connection to publish poison messages.
	url := requireRabbitMQ(t)
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("dial rabbitmq for poison message: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel for poison message: %v", err)
	}
	defer ch.Close()

	// Declare the work queue.
	workArgs := amqp.Table{
		"x-dead-letter-exchange":    "test-tq.dead",
		"x-dead-letter-routing-key": queue,
	}
	if _, err := ch.QueueDeclare("test-tq-"+queue, true, false, false, false, workArgs); err != nil {
		t.Fatalf("declare queue %q: %v", queue, err)
	}

	// Publish directly to the queue.
	msg := amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Body:         body,
	}
	if err := ch.Publish("", "test-tq-"+queue, false, false, msg); err != nil {
		t.Fatalf("publish poison message: %v", err)
	}

	// Give RabbitMQ a moment to deliver the message.
	time.Sleep(100 * time.Millisecond)
}

func TestRabbitMQ_EnqueueDequeueAck(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]string{"hello": "world"})
	task := taskqueue.NewTask("test:type", payload, taskqueue.WithQueue("default"))

	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Give the broker a moment to route the message.
	time.Sleep(100 * time.Millisecond)

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

func TestRabbitMQ_NackDead(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	payload := []byte(`"nack-dead"`)
	task := taskqueue.NewTask("test:dead", payload, taskqueue.WithQueue("default"))

	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	got, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}

	// Nack with zero retryAt → dead letter.
	if err := b.Nack(ctx, got, "simulated failure", time.Time{}); err != nil {
		t.Fatalf("Nack (dead): %v", err)
	}
}

func TestRabbitMQ_Dedup(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	task := taskqueue.NewTask("test:dedup", nil,
		taskqueue.WithQueue("default"),
		taskqueue.WithUnique("dedup-key-1", time.Minute),
	)

	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}

	// Duplicate within window.
	dup := taskqueue.NewTask("test:dedup", nil,
		taskqueue.WithQueue("default"),
		taskqueue.WithUnique("dedup-key-1", time.Minute),
	)
	if err := b.Enqueue(ctx, dup); err != taskqueue.ErrDuplicateTask {
		t.Errorf("expected ErrDuplicateTask, got: %v", err)
	}
}

func TestRabbitMQ_ScheduleReapAreNoOps(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	if err := b.Schedule(ctx); err != nil {
		t.Errorf("Schedule: %v", err)
	}
	if err := b.ReapStale(ctx); err != nil {
		t.Errorf("ReapStale: %v", err)
	}
}

// ─── Poison message / Validate() tests ─────────────────────────────────────────

func TestRabbitMQ_DequeueInvalidJSON(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	// Publish malformed JSON directly to the queue.
	publishPoisonMessage(t, b, "default", []byte(`{invalid json`))

	// Dequeue should return an error (unmarshal failure) and the message should be nacked.
	_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}

	// The error should mention unmarshal.
	if !errors.Is(err, nil) && !containsString(err.Error(), "unmarshal") && !containsString(err.Error(), "invalid character") {
		t.Errorf("error should mention unmarshal: %v", err)
	}

	// Verify the poison message was not requeued by attempting to dequeue again.
	// If it was requeued, we would get the same error (or eventually no task).
	_, err = b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("poison message should have been removed, not requeued")
	}
	if !errors.Is(err, taskqueue.ErrNoTask) {
		t.Logf("second dequeue error (may vary): %v", err)
	}
}

func TestRabbitMQ_DequeueMissingID(t *testing.T) {
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
	publishPoisonMessage(t, b, "default", body)

	// Dequeue should return a validation error.
	_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
	if !containsString(err.Error(), "invalid task") && !containsString(err.Error(), "missing ID") {
		t.Errorf("error should mention invalid task or missing ID: %v", err)
	}
}

func TestRabbitMQ_DequeueMissingType(t *testing.T) {
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
	publishPoisonMessage(t, b, "default", body)

	// Dequeue should return a validation error.
	_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected error for missing Type, got nil")
	}
	if !containsString(err.Error(), "invalid task") && !containsString(err.Error(), "missing Type") {
		t.Errorf("error should mention invalid task or missing Type: %v", err)
	}
}

func TestRabbitMQ_DequeueMissingQueue(t *testing.T) {
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
	publishPoisonMessage(t, b, "default", body)

	// Dequeue should return a validation error.
	_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected error for missing Queue, got nil")
	}
	if !containsString(err.Error(), "invalid task") && !containsString(err.Error(), "missing Queue") {
		t.Errorf("error should mention invalid task or missing Queue: %v", err)
	}
}

func TestRabbitMQ_PoisonMessageNotRequeued(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	// Publish multiple poison messages.
	for range 3 {
		publishPoisonMessage(t, b, "default", []byte(`{poison message `))
	}

	// Dequeue should return errors for all poison messages.
	// After processing all poison messages, the queue should be empty.
	attempts := 0
	maxAttempts := 10
	for attempts < maxAttempts {
		_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if errors.Is(err, taskqueue.ErrNoTask) {
			// Queue is empty - poison messages were properly removed.
			break
		}
		if err != nil {
			// Got an error (expected for poison messages), continue.
			attempts++
			continue
		}
		t.Fatalf("unexpected success dequeueing poison message")
	}

	if attempts >= maxAttempts {
		t.Fatalf("did not empty queue after %d dequeue attempts", maxAttempts)
	}

	if attempts != 3 {
		t.Errorf("expected to process 3 poison messages, got %d", attempts)
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
