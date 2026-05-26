// Package rocketmq provides a RocketMQ-backed implementation of the
// taskqueue.Broker interface using rocketmq-clients/golang/v5.
//
// # Topic layout
//
//	tq-{queue}       — main work topic (Normal or FIFO)
//	tq-{queue}-dead  — dead-lettered tasks (produced manually)
//
// # Delayed delivery
//
// Tasks with ProcessAt in the future are published with a delivery timestamp
// (Message.SetDelayTimestamp). RocketMQ delivers them at the specified time.
//
// # Retry
//
// On Nack with a retryAt time, the broker calls SimpleConsumer.ChangeInvisibleDuration
// to delay the next redelivery, avoiding a busy-retry loop. This relies on
// RocketMQ's native invisible-duration mechanism (similar to SQS visibility timeout).
//
// # Schedule / ReapStale
//
// Both are no-ops: RocketMQ handles delayed delivery and re-delivery of
// unacknowledged messages natively via the invisible duration.
//
// # Usage
//
//	broker, err := rocketmq.New(rocketmq.Config{
//	    Endpoint:      "localhost:8081",
//	    ConsumerGroup: "my-app-workers",
//	    Queues:        []string{"default", "critical"},
//	})
//	defer broker.Close()
package rocketmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	rmq "github.com/apache/rocketmq-clients/golang/v5"
	"github.com/apache/rocketmq-clients/golang/v5/credentials"

	"github.com/astra-go/astra/taskqueue"
)

// Config configures the RocketMQ broker.
type Config struct {
	// Endpoint is the RocketMQ Proxy gRPC endpoint. e.g. "localhost:8081".
	Endpoint string

	// NameSpace isolates topics in multi-tenant deployments. Optional.
	NameSpace string

	// KeyPrefix is the prefix for all topic names. Default: "tq".
	KeyPrefix string

	// ConsumerGroup is the RocketMQ consumer group name. Default: "tq-workers".
	ConsumerGroup string

	// Queues is the list of queue names the consumer subscribes to.
	// Each queue name maps to topic "{KeyPrefix}-{queue}".
	// Required for the consumer; the producer discovers topics dynamically.
	Queues []string

	// AccessKey and SecretKey are the AK/SK credentials. Both empty = no auth.
	AccessKey string
	SecretKey string
}

func (c *Config) setDefaults() {
	if c.KeyPrefix == "" {
		c.KeyPrefix = "tq"
	}
	if c.ConsumerGroup == "" {
		c.ConsumerGroup = "tq-workers"
	}
}

// Broker is a RocketMQ-backed taskqueue.Broker.
type Broker struct {
	producer rmq.Producer
	consumer rmq.SimpleConsumer
	prefix   string
	mu       sync.Mutex // serialises Receive calls (SimpleConsumer is not concurrent-safe per call)

	inflight sync.Map // taskID → *rmq.MessageView
}

// ─── Topic helpers ────────────────────────────────────────────────────────────

func (b *Broker) mainTopic(q string) string { return b.prefix + "-" + q }
func (b *Broker) deadTopic(q string) string { return b.prefix + "-" + q + "-dead" }

// ─── Constructor ──────────────────────────────────────────────────────────────

// New creates a Broker, starts the producer and consumer, and returns when
// both are ready to use.
func New(cfg Config) (*Broker, error) {
	cfg.setDefaults()

	rmqCfg := &rmq.Config{
		Endpoint:  cfg.Endpoint,
		NameSpace: cfg.NameSpace,
	}
	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		rmqCfg.Credentials = &credentials.SessionCredentials{
			AccessKey:    cfg.AccessKey,
			AccessSecret: cfg.SecretKey,
		}
	}

	// Build the list of topics the producer needs to know about.
	producerTopics := make([]string, 0, len(cfg.Queues)*2)
	for _, q := range cfg.Queues {
		producerTopics = append(producerTopics, cfg.KeyPrefix+"-"+q)
		producerTopics = append(producerTopics, cfg.KeyPrefix+"-"+q+"-dead")
	}

	producer, err := rmq.NewProducer(rmqCfg, rmq.WithTopics(producerTopics...))
	if err != nil {
		return nil, fmt.Errorf("taskqueue rocketmq: new producer: %w", err)
	}
	if err := producer.Start(); err != nil {
		return nil, fmt.Errorf("taskqueue rocketmq: start producer: %w", err)
	}

	// Build subscription expressions: subscribe to all configured queues.
	subExprs := make(map[string]*rmq.FilterExpression, len(cfg.Queues))
	for _, q := range cfg.Queues {
		subExprs[cfg.KeyPrefix+"-"+q] = rmq.SUB_ALL
	}

	consumer, err := rmq.NewSimpleConsumer(rmqCfg,
		rmq.WithSimpleAwaitDuration(200*time.Millisecond),
		rmq.WithSimpleSubscriptionExpressions(subExprs),
	)
	if err != nil {
		_ = producer.GracefulStop()
		return nil, fmt.Errorf("taskqueue rocketmq: new consumer: %w", err)
	}
	if err := consumer.Start(); err != nil {
		_ = producer.GracefulStop()
		return nil, fmt.Errorf("taskqueue rocketmq: start consumer: %w", err)
	}

	return &Broker{
		producer: producer,
		consumer: consumer,
		prefix:   cfg.KeyPrefix,
	}, nil
}

// ─── Broker interface ─────────────────────────────────────────────────────────

// Enqueue sends the task to RocketMQ.
// If ProcessAt is in the future, the message is published with a delivery
// timestamp so RocketMQ holds it until the specified time.
// UniqueKey is set as the message key for broker-level deduplication.
func (b *Broker) Enqueue(ctx context.Context, task *taskqueue.Task) error {
	now := time.Now()
	task.UpdatedAt = now

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue rocketmq: marshal task: %w", err)
	}

	msg := &rmq.Message{
		Topic: b.mainTopic(task.Queue),
		Body:  data,
	}
	if task.UniqueKey != "" {
		msg.SetKeys(task.UniqueKey)
	}
	if task.ProcessAt.After(now) {
		task.State = taskqueue.StateScheduled
		msg.SetDelayTimestamp(task.ProcessAt)
	} else {
		task.State = taskqueue.StatePending
	}

	if _, err := b.producer.Send(ctx, msg); err != nil {
		return fmt.Errorf("taskqueue rocketmq: send task %q: %w", task.ID, err)
	}
	return nil
}

// Dequeue calls SimpleConsumer.Receive to fetch one task.
// Returns ErrNoTask when no message is available within the await duration.
func (b *Broker) Dequeue(ctx context.Context, _ []string, _ time.Time) (*taskqueue.Task, error) {
	b.mu.Lock()
	mvs, err := b.consumer.Receive(ctx, 1, taskqueue.DefaultTimeout)
	b.mu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// RocketMQ returns an error when no message is available within
		// the await duration — treat as ErrNoTask.
		return nil, taskqueue.ErrNoTask
	}
	if len(mvs) == 0 {
		return nil, taskqueue.ErrNoTask
	}

	mv := mvs[0]
	var task taskqueue.Task
	if err := json.Unmarshal(mv.GetBody(), &task); err != nil {
		// Poison message — ack to remove it.
		_ = b.consumer.Ack(ctx, mv)
		return nil, fmt.Errorf("taskqueue rocketmq: unmarshal task: %w", err)
	}
	if err := task.Validate(); err != nil {
		// Invalid task fields — treat as poison, ack to remove it.
		_ = b.consumer.Ack(ctx, mv)
		return nil, fmt.Errorf("taskqueue rocketmq: invalid task: %w", err)
	}

	task.State = taskqueue.StateActive
	b.inflight.Store(task.ID, mv)
	return &task, nil
}

// Ack acknowledges the task as successfully processed.
func (b *Broker) Ack(ctx context.Context, task *taskqueue.Task) error {
	mv, ok := b.inflightLoad(task.ID)
	if !ok {
		return fmt.Errorf("taskqueue rocketmq: ack %q: not found in inflight map", task.ID)
	}
	if err := b.consumer.Ack(ctx, mv); err != nil {
		return fmt.Errorf("taskqueue rocketmq: ack %q: %w", task.ID, err)
	}
	b.inflight.Delete(task.ID)
	return nil
}

// Nack records task failure.
//   - If retryAt is zero: the task is dead-lettered by producing to the dead
//     topic and acking the original message (prevents RocketMQ re-delivery).
//   - Otherwise: ChangeInvisibleDuration delays re-delivery until retryAt,
//     leveraging RocketMQ's native invisible-duration mechanism.
func (b *Broker) Nack(ctx context.Context, task *taskqueue.Task, lastErr string, retryAt time.Time) error {
	task.LastError = lastErr
	task.UpdatedAt = time.Now()

	mv, ok := b.inflightLoad(task.ID)
	if !ok {
		return fmt.Errorf("taskqueue rocketmq: nack %q: not found in inflight map", task.ID)
	}
	defer b.inflight.Delete(task.ID)

	if retryAt.IsZero() {
		// Dead-letter: produce to dead topic, then ack the original.
		task.State = taskqueue.StateDead
		data, err := json.Marshal(task)
		if err != nil {
			return fmt.Errorf("taskqueue rocketmq: marshal dead task: %w", err)
		}
		deadMsg := &rmq.Message{
			Topic: b.deadTopic(task.Queue),
			Body:  data,
		}
		if _, err := b.producer.Send(ctx, deadMsg); err != nil {
			return fmt.Errorf("taskqueue rocketmq: send dead task %q: %w", task.ID, err)
		}
		if err := b.consumer.Ack(ctx, mv); err != nil {
			return fmt.Errorf("taskqueue rocketmq: ack dead task %q: %w", task.ID, err)
		}
		return nil
	}

	// Retry: change invisible duration so the message becomes visible at retryAt.
	task.State = taskqueue.StateRetry
	delay := time.Until(retryAt)
	if delay < 0 {
		delay = 0
	}
	if err := b.consumer.ChangeInvisibleDuration(mv, delay); err != nil {
		return fmt.Errorf("taskqueue rocketmq: change invisible duration %q: %w", task.ID, err)
	}
	return nil
}

// Schedule is a no-op for RocketMQ: scheduled delivery is handled natively
// via Message.SetDelayTimestamp. Retry re-delivery is managed via
// ChangeInvisibleDuration.
func (b *Broker) Schedule(_ context.Context) error { return nil }

// ReapStale is a no-op for RocketMQ: when the invisible duration expires
// without an Ack, RocketMQ automatically makes the message visible again.
func (b *Broker) ReapStale(_ context.Context) error { return nil }

// Close gracefully stops the producer and consumer.
func (b *Broker) Close() error {
	consumerErr := b.consumer.GracefulStop()
	producerErr := b.producer.GracefulStop()
	if consumerErr != nil {
		return consumerErr
	}
	return producerErr
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (b *Broker) inflightLoad(taskID string) (*rmq.MessageView, bool) {
	v, ok := b.inflight.Load(taskID)
	if !ok {
		return nil, false
	}
	return v.(*rmq.MessageView), true
}
