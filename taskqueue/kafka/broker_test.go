package kafka_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/astra-go/astra/taskqueue"
	tqkafka "github.com/astra-go/astra/taskqueue/kafka"
)

// Set KAFKA_BROKERS=localhost:9092 to run these tests.
func requireKafka(t *testing.T) []string {
	t.Helper()
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("set KAFKA_BROKERS to run Kafka integration tests")
	}
	return strings.Split(brokers, ",")
}

func newTestBroker(t *testing.T) *tqkafka.Broker {
	t.Helper()
	brokers := requireKafka(t)
	b, err := tqkafka.New(tqkafka.Config{
		Brokers:       brokers,
		KeyPrefix:     "test-tq",
		ConsumerGroup: "test-tq-workers",
	})
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// publishPoisonMessage injects a malformed message directly to a Kafka topic for testing.
func publishPoisonMessage(t *testing.T, brokers []string, topic string, body []byte) {
	t.Helper()

	// Create a producer client.
	producer, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatalf("create kafka producer for poison message: %v", err)
	}
	defer producer.Close()

	// Publish the poison message.
	record := &kgo.Record{
		Topic: topic,
		Key:   []byte("poison-key"),
		Value: body,
	}
	results := producer.ProduceSync(context.Background(), record)
	if err := results.FirstErr(); err != nil {
		t.Fatalf("produce poison message: %v", err)
	}

	// Give Kafka a moment to deliver the message.
	time.Sleep(200 * time.Millisecond)
}

func TestKafka_EnqueueDequeueAck(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]string{"hello": "kafka"})
	task := taskqueue.NewTask("test:type", payload, taskqueue.WithQueue("default"))

	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Poll with a generous timeout — Kafka delivery is async.
	deadline := time.Now().Add(10 * time.Second)
	var got *taskqueue.Task
	for time.Now().Before(deadline) {
		var err error
		got, err = b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if err == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if got == nil {
		t.Fatal("Dequeue: timed out waiting for task")
	}
	if got.ID != task.ID {
		t.Errorf("task ID mismatch: got %q, want %q", got.ID, task.ID)
	}

	if err := b.Ack(ctx, got); err != nil {
		t.Fatalf("Ack: %v", err)
	}
}

func TestKafka_NackDead(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	task := taskqueue.NewTask("test:dead", nil, taskqueue.WithQueue("default"))
	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	var got *taskqueue.Task
	for time.Now().Before(deadline) {
		var err error
		got, err = b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if err == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if got == nil {
		t.Fatal("Dequeue: timed out")
	}

	if err := b.Nack(ctx, got, "test failure", time.Time{}); err != nil {
		t.Fatalf("Nack dead: %v", err)
	}
}

func TestKafka_NackRetry(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	task := taskqueue.NewTask("test:retry", nil, taskqueue.WithQueue("default"))
	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	var got *taskqueue.Task
	for time.Now().Before(deadline) {
		var err error
		got, err = b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if err == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if got == nil {
		t.Fatal("Dequeue: timed out")
	}

	// Nack with a short retry window.
	retryAt := time.Now().Add(2 * time.Second)
	if err := b.Nack(ctx, got, "retry test", retryAt); err != nil {
		t.Fatalf("Nack retry: %v", err)
	}

	// Schedule should promote the record after its process_at elapses.
	time.Sleep(3 * time.Second)
	if err := b.Schedule(ctx); err != nil {
		t.Errorf("Schedule: %v", err)
	}
}

func TestKafka_ReapStaleIsNoOp(t *testing.T) {
	b := newTestBroker(t)
	if err := b.ReapStale(context.Background()); err != nil {
		t.Errorf("ReapStale: %v", err)
	}
}

// ─── Poison message / Validate() tests ─────────────────────────────────────────

func TestKafka_DequeueInvalidJSON(t *testing.T) {
	brokers := requireKafka(t)
	b := newTestBroker(t)
	ctx := context.Background()

	// Publish malformed JSON directly to the topic.
	publishPoisonMessage(t, brokers, "test-tq-default", []byte(`{invalid json`))

	// Dequeue should skip the poison message (commit it) and eventually return ErrNoTask.
	deadline := time.Now().Add(5 * time.Second)
	var got *taskqueue.Task
	var lastErr error
	for time.Now().Before(deadline) {
		var err error
		got, err = b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if err == nil {
			// Successfully dequeued a valid task (unexpected in this test).
			break
		}
		if errors.Is(err, taskqueue.ErrNoTask) {
			// Poison message was committed and removed.
			return
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}

	if got != nil {
		t.Fatalf("unexpectedly dequeued a valid task: %+v", got)
	}
	t.Fatalf("did not commit poison message, last error: %v", lastErr)
}

func TestKafka_DequeueMissingID(t *testing.T) {
	brokers := requireKafka(t)
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
	publishPoisonMessage(t, brokers, "test-tq-default", body)

	// Dequeue should skip the poison message and eventually return ErrNoTask.
	deadline := time.Now().Add(5 * time.Second)
	var got *taskqueue.Task
	var lastErr error
	for time.Now().Before(deadline) {
		var err error
		got, err = b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if err == nil {
			break
		}
		if errors.Is(err, taskqueue.ErrNoTask) {
			// Poison message was committed and removed.
			return
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}

	if got != nil {
		t.Fatalf("unexpectedly dequeued a valid task: %+v", got)
	}
	t.Fatalf("did not commit poison message, last error: %v", lastErr)
}

func TestKafka_DequeueMissingType(t *testing.T) {
	brokers := requireKafka(t)
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
	publishPoisonMessage(t, brokers, "test-tq-default", body)

	// Dequeue should skip the poison message and eventually return ErrNoTask.
	deadline := time.Now().Add(5 * time.Second)
	var got *taskqueue.Task
	var lastErr error
	for time.Now().Before(deadline) {
		var err error
		got, err = b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if err == nil {
			break
		}
		if errors.Is(err, taskqueue.ErrNoTask) {
			return
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}

	if got != nil {
		t.Fatalf("unexpectedly dequeued a valid task: %+v", got)
	}
	t.Fatalf("did not commit poison message, last error: %v", lastErr)
}

func TestKafka_DequeueMissingQueue(t *testing.T) {
	brokers := requireKafka(t)
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
	publishPoisonMessage(t, brokers, "test-tq-default", body)

	// Dequeue should skip the poison message and eventually return ErrNoTask.
	deadline := time.Now().Add(5 * time.Second)
	var got *taskqueue.Task
	var lastErr error
	for time.Now().Before(deadline) {
		var err error
		got, err = b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if err == nil {
			break
		}
		if errors.Is(err, taskqueue.ErrNoTask) {
			return
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}

	if got != nil {
		t.Fatalf("unexpectedly dequeued a valid task: %+v", got)
	}
	t.Fatalf("did not commit poison message, last error: %v", lastErr)
}

func TestKafka_PoisonMessageCommitted(t *testing.T) {
	brokers := requireKafka(t)
	b := newTestBroker(t)
	ctx := context.Background()

	// Publish multiple poison messages.
	for range 3 {
		publishPoisonMessage(t, brokers, "test-tq-default", []byte(`{poison message `))
	}

	// Dequeue should skip all poison messages (commit them) and eventually return ErrNoTask.
	deadline := time.Now().Add(10 * time.Second)
	attempts := 0
	for time.Now().Before(deadline) {
		_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if errors.Is(err, taskqueue.ErrNoTask) {
			// All poison messages were committed and removed.
			if attempts != 3 {
				t.Errorf("expected to process 3 poison messages, got %d", attempts)
			}
			return
		}
		if err != nil {
			// Got an error (expected for poison messages), continue.
			attempts++
			time.Sleep(200 * time.Millisecond)
			continue
		}
		// Successfully dequeued a valid task (unexpected in this test).
		t.Fatalf("unexpectedly dequeued a valid task")
	}

	t.Fatalf("did not commit all poison messages within deadline, processed %d", attempts)
}
