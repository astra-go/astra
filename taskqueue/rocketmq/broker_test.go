package rocketmq_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	rmq "github.com/apache/rocketmq-clients/golang/v5"

	"github.com/astra-go/astra/taskqueue"
	tqrocketmq "github.com/astra-go/astra/taskqueue/rocketmq"
)

// Set ROCKETMQ_ENDPOINT=localhost:8081 to run these tests.
func requireRocketMQ(t *testing.T) string {
	t.Helper()
	ep := os.Getenv("ROCKETMQ_ENDPOINT")
	if ep == "" {
		t.Skip("set ROCKETMQ_ENDPOINT to run RocketMQ integration tests")
	}
	return ep
}

func newTestBroker(t *testing.T) *tqrocketmq.Broker {
	t.Helper()
	ep := requireRocketMQ(t)
	b, err := tqrocketmq.New(tqrocketmq.Config{
		Endpoint:      ep,
		KeyPrefix:     "test-tq",
		ConsumerGroup: "test-tq-workers",
		Queues:        []string{"default"},
	})
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// publishPoisonMessage injects a malformed message directly to a RocketMQ topic for testing.
func publishPoisonMessage(t *testing.T, endpoint string, topic string, body []byte) {
	t.Helper()

	rmqCfg := &rmq.Config{
		Endpoint: endpoint,
	}

	// Create a producer client.
	producer, err := rmq.NewProducer(rmqCfg, rmq.WithTopics(topic))
	if err != nil {
		t.Fatalf("create rocketmq producer for poison message: %v", err)
	}
	defer producer.GracefulStop()

	if err := producer.Start(); err != nil {
		t.Fatalf("start rocketmq producer for poison message: %v", err)
	}

	// Publish the poison message.
	msg := &rmq.Message{
		Topic: topic,
		Body:  body,
	}
	if _, err := producer.Send(context.Background(), msg); err != nil {
		t.Fatalf("send poison message: %v", err)
	}

	// Give RocketMQ a moment to deliver the message.
	time.Sleep(200 * time.Millisecond)
}

func TestRocketMQ_EnqueueDequeueAck(t *testing.T) {
	b := newTestBroker(t)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]string{"hello": "rocketmq"})
	task := taskqueue.NewTask("test:type", payload, taskqueue.WithQueue("default"))

	if err := b.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Poll with retries — RocketMQ delivery is async.
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
	if got.State != taskqueue.StateActive {
		t.Errorf("state = %q, want active", got.State)
	}

	if err := b.Ack(ctx, got); err != nil {
		t.Fatalf("Ack: %v", err)
	}
}

func TestRocketMQ_NackDead(t *testing.T) {
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

func TestRocketMQ_NackRetry(t *testing.T) {
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

	// Nack retry — ChangeInvisibleDuration delays re-delivery.
	retryAt := time.Now().Add(3 * time.Second)
	if err := b.Nack(ctx, got, "retry test", retryAt); err != nil {
		t.Fatalf("Nack retry: %v", err)
	}
}

func TestRocketMQ_ScheduleReapAreNoOps(t *testing.T) {
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

func TestRocketMQ_DequeueInvalidJSON(t *testing.T) {
	ep := requireRocketMQ(t)
	b := newTestBroker(t)
	ctx := context.Background()

	// Publish malformed JSON directly to the topic.
	publishPoisonMessage(t, ep, "test-tq-default", []byte(`{invalid json`))

	// Dequeue should skip the poison message (ack it) and eventually return ErrNoTask.
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
			// Poison message was acked and removed.
			return
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}

	if got != nil {
		t.Fatalf("unexpectedly dequeued a valid task: %+v", got)
	}
	t.Fatalf("did not ack poison message, last error: %v", lastErr)
}

func TestRocketMQ_DequeueMissingID(t *testing.T) {
	ep := requireRocketMQ(t)
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
	publishPoisonMessage(t, ep, "test-tq-default", body)

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
	t.Fatalf("did not ack poison message, last error: %v", lastErr)
}

func TestRocketMQ_DequeueMissingType(t *testing.T) {
	ep := requireRocketMQ(t)
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
	publishPoisonMessage(t, ep, "test-tq-default", body)

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
	t.Fatalf("did not ack poison message, last error: %v", lastErr)
}

func TestRocketMQ_DequeueMissingQueue(t *testing.T) {
	ep := requireRocketMQ(t)
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
	publishPoisonMessage(t, ep, "test-tq-default", body)

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
	t.Fatalf("did not ack poison message, last error: %v", lastErr)
}

func TestRocketMQ_PoisonMessageAcked(t *testing.T) {
	ep := requireRocketMQ(t)
	b := newTestBroker(t)
	ctx := context.Background()

	// Publish multiple poison messages.
	for range 3 {
		publishPoisonMessage(t, ep, "test-tq-default", []byte(`{poison message `))
	}

	// Dequeue should skip all poison messages (ack them) and eventually return ErrNoTask.
	deadline := time.Now().Add(10 * time.Second)
	attempts := 0
	for time.Now().Before(deadline) {
		_, err := b.Dequeue(ctx, []string{"default"}, time.Now().Add(time.Minute))
		if errors.Is(err, taskqueue.ErrNoTask) {
			// All poison messages were acked and removed.
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

	t.Fatalf("did not ack all poison messages within deadline, processed %d", attempts)
}
