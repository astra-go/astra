//go:build rabbitmq
// +build rabbitmq

package taskqueue

// This file provides the RabbitMQ broker, enabled with build tag "rabbitmq".

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitmqConfig configures the RabbitMQ broker.
type RabbitmqConfig struct {
	// URL is the AMQP connection URL. e.g. "amqp://guest:guest@localhost:5672/"
	URL string

	// KeyPrefix is the namespace prepended to all exchange/queue names.
	// Default: "tq".
	KeyPrefix string

	// UseDelayedExchange enables the x-delayed-message exchange for delayed
	// and retry messages. Requires the rabbitmq_delayed_message_exchange plugin.
	// Default: true. Set false to disable (delays are not honoured).
	UseDelayedExchange bool
}

func (c *RabbitmqConfig) setRabbitmqDefaults() {
	if c.KeyPrefix == "" {
		c.KeyPrefix = "tq"
	}
}

// rabbitmqInflightEntry holds the acknowledgement handle for a dequeued message.
type rabbitmqInflightEntry struct {
	deliveryTag uint64
	ch          *amqp.Channel
}

// RabbitmqBroker is a RabbitMQ-backed Broker.
type RabbitmqBroker struct {
	conn   *amqp.Connection
	pubCh  *amqp.Channel // used for publishing; protected by pubMu
	getCh  *amqp.Channel // used for basic.get; protected by getMu
	pubMu  sync.Mutex
	getMu  sync.Mutex
	prefix string
	delay  bool

	inflight sync.Map // taskID → rabbitmqInflightEntry
	dedup    sync.Map // uniqueKey → time.Time (expiry)
}

// ─── Name helpers ─────────────────────────────────────────────────────────────

func (b *RabbitmqBroker) rmqWorkExchange() string    { return b.prefix + ".work" }
func (b *RabbitmqBroker) rmqDelayedExchange() string { return b.prefix + ".delayed" }
func (b *RabbitmqBroker) rmqDeadExchange() string    { return b.prefix + ".dead" }
func (b *RabbitmqBroker) rmqQueueName(q string) string { return b.prefix + "-" + q }
func (b *RabbitmqBroker) rmqDeadQueue(q string) string { return b.prefix + "-" + q + "-dead" }

// ─── Constructor ──────────────────────────────────────────────────────────────

// NewRabbitmqBroker connects to RabbitMQ, declares exchanges/queues, and returns a Broker.
func NewRabbitmqBroker(cfg RabbitmqConfig) (*RabbitmqBroker, error) {
	cfg.setRabbitmqDefaults()

	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("taskqueue rabbitmq: dial %q: %w", cfg.URL, err)
	}

	pubCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("taskqueue rabbitmq: open pub channel: %w", err)
	}

	getCh, err := conn.Channel()
	if err != nil {
		_ = pubCh.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("taskqueue rabbitmq: open get channel: %w", err)
	}

	b := &RabbitmqBroker{
		conn:   conn,
		pubCh:  pubCh,
		getCh:  getCh,
		prefix: cfg.KeyPrefix,
		delay:  cfg.UseDelayedExchange,
	}

	if err := b.rabbitmqDeclareTopology(); err != nil {
		if pubCh != nil {
			_ = pubCh.Close()
		}
		if getCh != nil {
			_ = getCh.Close()
		}
		_ = conn.Close()
		return nil, err
	}
	return b, nil
}

// NewRabbitmqBrokerFromConn creates a Broker from an existing *amqp.Connection.
func NewRabbitmqBrokerFromConn(conn *amqp.Connection, cfg RabbitmqConfig) (*RabbitmqBroker, error) {
	cfg.setRabbitmqDefaults()

	pubCh, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("taskqueue rabbitmq: open pub channel: %w", err)
	}
	getCh, err := conn.Channel()
	if err != nil {
		_ = pubCh.Close()
		return nil, fmt.Errorf("taskqueue rabbitmq: open get channel: %w", err)
	}

	b := &RabbitmqBroker{
		conn:   conn,
		pubCh:  pubCh,
		getCh:  getCh,
		prefix: cfg.KeyPrefix,
		delay:  cfg.UseDelayedExchange,
	}
	if err := b.rabbitmqDeclareTopology(); err != nil {
		if pubCh != nil {
			_ = pubCh.Close()
		}
		if getCh != nil {
			_ = getCh.Close()
		}
		return nil, err
	}
	return b, nil
}

func (b *RabbitmqBroker) rabbitmqDeclareTopology() error {
	ch := b.pubCh

	// Work exchange (direct).
	if err := ch.ExchangeDeclare(b.rmqWorkExchange(), "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: declare work exchange: %w", err)
	}

	// Dead exchange (direct).
	if err := ch.ExchangeDeclare(b.rmqDeadExchange(), "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: declare dead exchange: %w", err)
	}

	// Delayed exchange (x-delayed-message plugin) — best-effort.
	if b.delay {
		args := amqp.Table{"x-delayed-type": "direct"}
		if err := ch.ExchangeDeclare(b.rmqDelayedExchange(), "x-delayed-message", true, false, false, false, args); err != nil {
			b.delay = false
		}
	}

	return nil
}

// rmqEnsureQueue declares the work queue and its dead-letter queue on demand.
func (b *RabbitmqBroker) rmqEnsureQueue(ch *amqp.Channel, queue string) error {
	workArgs := amqp.Table{
		"x-dead-letter-exchange":     b.rmqDeadExchange(),
		"x-dead-letter-routing-key":  queue,
	}
	if _, err := ch.QueueDeclare(b.rmqQueueName(queue), true, false, false, false, workArgs); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: declare queue %q: %w", queue, err)
	}
	if err := ch.QueueBind(b.rmqQueueName(queue), queue, b.rmqWorkExchange(), false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: bind queue %q: %w", queue, err)
	}

	if _, err := ch.QueueDeclare(b.rmqDeadQueue(queue), true, false, false, false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: declare dead queue %q: %w", queue, err)
	}
	if err := ch.QueueBind(b.rmqDeadQueue(queue), queue, b.rmqDeadExchange(), false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: bind dead queue %q: %w", queue, err)
	}

	if b.delay {
		if err := ch.QueueBind(b.rmqQueueName(queue), queue, b.rmqDelayedExchange(), false, nil); err != nil {
			return fmt.Errorf("taskqueue rabbitmq: bind delayed→work queue %q: %w", queue, err)
		}
	}
	return nil
}

// ─── Broker interface ─────────────────────────────────────────────────────────

// Enqueue publishes the task to the appropriate exchange.
// Returns ErrDuplicateTask when a unique key collision is detected.
func (b *RabbitmqBroker) Enqueue(_ context.Context, task *Task) error {
	now := time.Now()
	task.UpdatedAt = now

	if task.UniqueKey != "" && task.UniqueFor > 0 {
		expiry, loaded := b.dedup.Load(task.UniqueKey)
		if loaded && expiry.(time.Time).After(now) {
			return ErrDuplicateTask
		}
		b.dedup.Store(task.UniqueKey, now.Add(task.UniqueFor))
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue rabbitmq: marshal task: %w", err)
	}

	b.pubMu.Lock()
	defer b.pubMu.Unlock()

	if err := b.rmqEnsureQueue(b.pubCh, task.Queue); err != nil {
		return err
	}

	msg := amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Body:         data,
	}

	if task.ProcessAt.After(now) && b.delay {
		delayMS := task.ProcessAt.Sub(now).Milliseconds()
		msg.Headers = amqp.Table{"x-delay": delayMS}
		task.State = StateScheduled
		return b.pubCh.Publish(b.rmqDelayedExchange(), task.Queue, false, false, msg)
	}

	task.State = StatePending
	return b.pubCh.Publish(b.rmqWorkExchange(), task.Queue, false, false, msg)
}

// Dequeue synchronously pulls the next available task from one of the queues.
func (b *RabbitmqBroker) Dequeue(_ context.Context, queues []string, _ time.Time) (*Task, error) {
	b.getMu.Lock()
	defer b.getMu.Unlock()

	for _, q := range queues {
		if err := b.rmqEnsureQueue(b.getCh, q); err != nil {
			return nil, err
		}

		msg, ok, err := b.getCh.Get(b.rmqQueueName(q), false)
		if err != nil {
			return nil, fmt.Errorf("taskqueue rabbitmq: get from %q: %w", q, err)
		}
		if !ok {
			continue
		}

		var task Task
		if err := json.Unmarshal(msg.Body, &task); err != nil {
			_ = b.getCh.Nack(msg.DeliveryTag, false, false)
			return nil, fmt.Errorf("taskqueue rabbitmq: unmarshal task: %w", err)
		}
		if err := task.Validate(); err != nil {
			_ = b.getCh.Nack(msg.DeliveryTag, false, false)
			return nil, fmt.Errorf("taskqueue rabbitmq: invalid task: %w", err)
		}

		task.State = StateActive
		b.inflight.Store(task.ID, rabbitmqInflightEntry{deliveryTag: msg.DeliveryTag, ch: b.getCh})
		return &task, nil
	}
	return nil, ErrNoTask
}

// Ack acknowledges successful task processing.
func (b *RabbitmqBroker) Ack(_ context.Context, task *Task) error {
	entry, ok := b.rabbitmqInflightLoad(task.ID)
	if !ok {
		return fmt.Errorf("taskqueue rabbitmq: ack %q: not found in inflight map", task.ID)
	}
	b.getMu.Lock()
	err := entry.ch.Ack(entry.deliveryTag, false)
	b.getMu.Unlock()
	if err != nil {
		return fmt.Errorf("taskqueue rabbitmq: ack %q: %w", task.ID, err)
	}
	b.inflight.Delete(task.ID)

	if task.UniqueKey != "" {
		b.dedup.Delete(task.UniqueKey)
	}
	return nil
}

// Nack records task failure.
func (b *RabbitmqBroker) Nack(_ context.Context, task *Task, lastErr string, retryAt time.Time) error {
	task.LastError = lastErr
	task.UpdatedAt = time.Now()

	entry, ok := b.rabbitmqInflightLoad(task.ID)
	if !ok {
		return fmt.Errorf("taskqueue rabbitmq: nack %q: not found in inflight map", task.ID)
	}
	defer b.inflight.Delete(task.ID)

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue rabbitmq: marshal task for nack: %w", err)
	}

	b.pubMu.Lock()
	defer b.pubMu.Unlock()

	if retryAt.IsZero() {
		task.State = StateDead
		msg := amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         data,
		}
		if pubErr := b.pubCh.Publish(b.rmqDeadExchange(), task.Queue, false, false, msg); pubErr != nil {
			return fmt.Errorf("taskqueue rabbitmq: publish dead task: %w", pubErr)
		}
	} else {
		task.State = StateRetry
		msg := amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         data,
		}
		if b.delay {
			delayMS := time.Until(retryAt).Milliseconds()
			if delayMS < 0 {
				delayMS = 0
			}
			msg.Headers = amqp.Table{"x-delay": delayMS}
			if pubErr := b.pubCh.Publish(b.rmqDelayedExchange(), task.Queue, false, false, msg); pubErr != nil {
				return fmt.Errorf("taskqueue rabbitmq: publish retry task: %w", pubErr)
			}
		} else {
			if pubErr := b.pubCh.Publish(b.rmqWorkExchange(), task.Queue, false, false, msg); pubErr != nil {
				return fmt.Errorf("taskqueue rabbitmq: publish retry task (no delay): %w", pubErr)
			}
		}
	}

	b.getMu.Lock()
	ackErr := entry.ch.Ack(entry.deliveryTag, false)
	b.getMu.Unlock()
	return ackErr
}

// Schedule is a no-op for RabbitMQ.
func (b *RabbitmqBroker) Schedule(_ context.Context) error { return nil }

// ReapStale is a no-op for RabbitMQ.
func (b *RabbitmqBroker) ReapStale(_ context.Context) error { return nil }

// Close closes the AMQP connection and all channels.
func (b *RabbitmqBroker) Close() error {
	return b.conn.Close()
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (b *RabbitmqBroker) rabbitmqInflightLoad(taskID string) (rabbitmqInflightEntry, bool) {
	v, ok := b.inflight.Load(taskID)
	if !ok {
		return rabbitmqInflightEntry{}, false
	}
	return v.(rabbitmqInflightEntry), true
}

// Verify RabbitmqBroker implements Broker at compile time.
var _ Broker = (*RabbitmqBroker)(nil)
