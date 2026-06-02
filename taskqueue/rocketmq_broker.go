//go:build rocketmq
// +build rocketmq

package taskqueue

// This file provides the RocketMQ broker, enabled with build tag "rocketmq".

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	rmq "github.com/apache/rocketmq-clients/golang/v5"
	"github.com/apache/rocketmq-clients/golang/v5/credentials"
)

// RocketmqConfig configures the RocketMQ broker.
type RocketmqConfig struct {
	// Endpoint is the RocketMQ Proxy gRPC endpoint. e.g. "localhost:8081".
	Endpoint string

	// NameSpace isolates topics in multi-tenant deployments. Optional.
	NameSpace string

	// KeyPrefix is the prefix for all topic names. Default: "tq".
	KeyPrefix string

	// ConsumerGroup is the RocketMQ consumer group name. Default: "tq-workers".
	ConsumerGroup string

	// Queues is the list of queue names the consumer subscribes to.
	Queues []string

	// AccessKey and SecretKey are the AK/SK credentials. Both empty = no auth.
	AccessKey string
	SecretKey string
}

func (c *RocketmqConfig) setRocketmqDefaults() {
	if c.KeyPrefix == "" {
		c.KeyPrefix = "tq"
	}
	if c.ConsumerGroup == "" {
		c.ConsumerGroup = "tq-workers"
	}
}

// RocketmqBroker is a RocketMQ-backed Broker.
type RocketmqBroker struct {
	producer rmq.Producer
	consumer rmq.SimpleConsumer
	prefix   string
	mu       sync.Mutex // serialises Receive calls

	inflight sync.Map // taskID → *rmq.MessageView
}

func (b *RocketmqBroker) rocketmqMainTopic(q string) string { return b.prefix + "-" + q }
func (b *RocketmqBroker) rocketmqDeadTopic(q string) string { return b.prefix + "-" + q + "-dead" }

// NewRocketmqBroker creates a Broker, starts the producer and consumer.
func NewRocketmqBroker(cfg RocketmqConfig) (*RocketmqBroker, error) {
	cfg.setRocketmqDefaults()

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

	return &RocketmqBroker{
		producer: producer,
		consumer: consumer,
		prefix:   cfg.KeyPrefix,
	}, nil
}

// ─── Broker interface ─────────────────────────────────────────────────────────

// Enqueue sends the task to RocketMQ.
func (b *RocketmqBroker) Enqueue(ctx context.Context, task *Task) error {
	now := time.Now()
	task.UpdatedAt = now

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue rocketmq: marshal task: %w", err)
	}

	msg := &rmq.Message{
		Topic: b.rocketmqMainTopic(task.Queue),
		Body:  data,
	}
	if task.UniqueKey != "" {
		msg.SetKeys(task.UniqueKey)
	}
	if task.ProcessAt.After(now) {
		task.State = StateScheduled
		msg.SetDelayTimestamp(task.ProcessAt)
	} else {
		task.State = StatePending
	}

	if _, err := b.producer.Send(ctx, msg); err != nil {
		return fmt.Errorf("taskqueue rocketmq: send task %q: %w", task.ID, err)
	}
	return nil
}

// Dequeue calls SimpleConsumer.Receive to fetch one task.
func (b *RocketmqBroker) Dequeue(ctx context.Context, _ []string, _ time.Time) (*Task, error) {
	b.mu.Lock()
	mvs, err := b.consumer.Receive(ctx, 1, DefaultTimeout)
	b.mu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrNoTask
	}
	if len(mvs) == 0 {
		return nil, ErrNoTask
	}

	mv := mvs[0]
	var task Task
	if err := json.Unmarshal(mv.GetBody(), &task); err != nil {
		_ = b.consumer.Ack(ctx, mv)
		return nil, fmt.Errorf("taskqueue rocketmq: unmarshal task: %w", err)
	}
	if err := task.Validate(); err != nil {
		_ = b.consumer.Ack(ctx, mv)
		return nil, fmt.Errorf("taskqueue rocketmq: invalid task: %w", err)
	}

	task.State = StateActive
	b.inflight.Store(task.ID, mv)
	return &task, nil
}

// Ack acknowledges the task as successfully processed.
func (b *RocketmqBroker) Ack(ctx context.Context, task *Task) error {
	mv, ok := b.rocketmqInflightLoad(task.ID)
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
func (b *RocketmqBroker) Nack(ctx context.Context, task *Task, lastErr string, retryAt time.Time) error {
	task.LastError = lastErr
	task.UpdatedAt = time.Now()

	mv, ok := b.rocketmqInflightLoad(task.ID)
	if !ok {
		return fmt.Errorf("taskqueue rocketmq: ack %q: not found in inflight map", task.ID)
	}
	defer b.inflight.Delete(task.ID)

	if retryAt.IsZero() {
		task.State = StateDead
		data, err := json.Marshal(task)
		if err != nil {
			return fmt.Errorf("taskqueue rocketmq: marshal dead task: %w", err)
		}
		deadMsg := &rmq.Message{
			Topic: b.rocketmqDeadTopic(task.Queue),
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

	task.State = StateRetry
	delay := time.Until(retryAt)
	if delay < 0 {
		delay = 0
	}
	if err := b.consumer.ChangeInvisibleDuration(mv, delay); err != nil {
		return fmt.Errorf("taskqueue rocketmq: change invisible duration %q: %w", task.ID, err)
	}
	return nil
}

// Schedule is a no-op for RocketMQ.
func (b *RocketmqBroker) Schedule(_ context.Context) error { return nil }

// ReapStale is a no-op for RocketMQ.
func (b *RocketmqBroker) ReapStale(_ context.Context) error { return nil }

// Close gracefully stops the producer and consumer.
func (b *RocketmqBroker) Close() error {
	consumerErr := b.consumer.GracefulStop()
	producerErr := b.producer.GracefulStop()
	if consumerErr != nil {
		return consumerErr
	}
	return producerErr
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (b *RocketmqBroker) rocketmqInflightLoad(taskID string) (*rmq.MessageView, bool) {
	v, ok := b.inflight.Load(taskID)
	if !ok {
		return nil, false
	}
	return v.(*rmq.MessageView), true
}

// Verify RocketmqBroker implements Broker at compile time.
var _ Broker = (*RocketmqBroker)(nil)
