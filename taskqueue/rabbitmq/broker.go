// Package rabbitmq provides a RabbitMQ-backed implementation of the
// taskqueue.Broker interface using amqp091-go.
//
// # Required RabbitMQ plugin
//
// Delayed and retry messages rely on the rabbitmq_delayed_message_exchange
// plugin. Disable with Config.UseDelayedExchange=false; in that mode scheduled
// messages are delivered immediately (delay is not honoured).
//
// # Topology (declared on New)
//
//	tq.work          — direct exchange, durable. Routes tasks to queues.
//	tq.delayed       — x-delayed-message exchange (plugin). Routes delayed tasks.
//	tq.dead          — direct exchange, durable. Routes dead-lettered tasks.
//	tq-{queue}       — durable queue. DLX=tq.dead, routing key={queue}.
//	tq-{queue}-dead  — durable queue. Holds dead-lettered tasks.
//
// # Usage
//
//	broker, err := rabbitmq.New(rabbitmq.Config{
//	    URL:                "amqp://guest:guest@localhost:5672/",
//	    UseDelayedExchange: true,
//	})
//	defer broker.Close()
//
//	client := taskqueue.NewClient(broker)
//	client.EnqueueTask(ctx, "email:welcome", payload)
package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/astra-go/astra/taskqueue"
)

// Config configures the RabbitMQ broker.
type Config struct {
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

func (c *Config) setDefaults() {
	if c.KeyPrefix == "" {
		c.KeyPrefix = "tq"
	}
}

// inflightEntry holds the acknowledgement handle for a message that has been
// dequeued but not yet Ack'd or Nack'd.
type inflightEntry struct {
	deliveryTag uint64
	ch          *amqp.Channel
}

// Broker is a RabbitMQ-backed taskqueue.Broker.
type Broker struct {
	conn   *amqp.Connection
	pubCh  *amqp.Channel // used for publishing; protected by pubMu
	getCh  *amqp.Channel // used for basic.get; protected by getMu
	pubMu  sync.Mutex
	getMu  sync.Mutex
	prefix string
	delay  bool

	inflight sync.Map // taskID → inflightEntry
	dedup    sync.Map // uniqueKey → time.Time (expiry)
}

// ─── Name helpers ─────────────────────────────────────────────────────────────

func (b *Broker) workExchange() string    { return b.prefix + ".work" }
func (b *Broker) delayedExchange() string { return b.prefix + ".delayed" }
func (b *Broker) deadExchange() string    { return b.prefix + ".dead" }
func (b *Broker) queueName(q string) string { return b.prefix + "-" + q }
func (b *Broker) deadQueue(q string) string { return b.prefix + "-" + q + "-dead" }

// ─── Constructor ──────────────────────────────────────────────────────────────

// New connects to RabbitMQ, declares exchanges/queues, and returns a Broker.
func New(cfg Config) (*Broker, error) {
	cfg.setDefaults()

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

	b := &Broker{
		conn:   conn,
		pubCh:  pubCh,
		getCh:  getCh,
		prefix: cfg.KeyPrefix,
		delay:  cfg.UseDelayedExchange,
	}

	if err := b.declareTopology(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return b, nil
}

// NewFromConn creates a Broker from an existing *amqp.Connection.
func NewFromConn(conn *amqp.Connection, cfg Config) (*Broker, error) {
	cfg.setDefaults()

	pubCh, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("taskqueue rabbitmq: open pub channel: %w", err)
	}
	getCh, err := conn.Channel()
	if err != nil {
		_ = pubCh.Close()
		return nil, fmt.Errorf("taskqueue rabbitmq: open get channel: %w", err)
	}

	b := &Broker{
		conn:   conn,
		pubCh:  pubCh,
		getCh:  getCh,
		prefix: cfg.KeyPrefix,
		delay:  cfg.UseDelayedExchange,
	}
	if err := b.declareTopology(); err != nil {
		_ = pubCh.Close()
		_ = getCh.Close()
		return nil, err
	}
	return b, nil
}

func (b *Broker) declareTopology() error {
	ch := b.pubCh

	// Work exchange (direct).
	if err := ch.ExchangeDeclare(b.workExchange(), "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: declare work exchange: %w", err)
	}

	// Dead exchange (direct).
	if err := ch.ExchangeDeclare(b.deadExchange(), "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: declare dead exchange: %w", err)
	}

	// Delayed exchange (x-delayed-message plugin) — best-effort.
	if b.delay {
		args := amqp.Table{"x-delayed-type": "direct"}
		if err := ch.ExchangeDeclare(b.delayedExchange(), "x-delayed-message", true, false, false, false, args); err != nil {
			// Plugin not available — fall back to immediate delivery.
			b.delay = false
		}
	}

	return nil
}

// ensureQueue declares the work queue and its dead-letter queue on demand.
// It is safe to call multiple times for the same queue name.
func (b *Broker) ensureQueue(ch *amqp.Channel, queue string) error {
	// Work queue — DLX points to dead exchange.
	workArgs := amqp.Table{
		"x-dead-letter-exchange":     b.deadExchange(),
		"x-dead-letter-routing-key":  queue,
	}
	if _, err := ch.QueueDeclare(b.queueName(queue), true, false, false, false, workArgs); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: declare queue %q: %w", queue, err)
	}
	if err := ch.QueueBind(b.queueName(queue), queue, b.workExchange(), false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: bind queue %q: %w", queue, err)
	}

	// Dead queue.
	if _, err := ch.QueueDeclare(b.deadQueue(queue), true, false, false, false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: declare dead queue %q: %w", queue, err)
	}
	if err := ch.QueueBind(b.deadQueue(queue), queue, b.deadExchange(), false, nil); err != nil {
		return fmt.Errorf("taskqueue rabbitmq: bind dead queue %q: %w", queue, err)
	}

	// Bind delayed exchange to work queue (so delayed messages land in the work queue).
	if b.delay {
		if err := ch.QueueBind(b.queueName(queue), queue, b.delayedExchange(), false, nil); err != nil {
			return fmt.Errorf("taskqueue rabbitmq: bind delayed→work queue %q: %w", queue, err)
		}
	}
	return nil
}

// ─── Broker interface ─────────────────────────────────────────────────────────

// Enqueue publishes the task to the appropriate exchange.
// Returns ErrDuplicateTask when a unique key collision is detected.
func (b *Broker) Enqueue(_ context.Context, task *taskqueue.Task) error {
	now := time.Now()
	task.UpdatedAt = now

	// Deduplication check.
	if task.UniqueKey != "" && task.UniqueFor > 0 {
		expiry, loaded := b.dedup.Load(task.UniqueKey)
		if loaded && expiry.(time.Time).After(now) {
			return taskqueue.ErrDuplicateTask
		}
		b.dedup.Store(task.UniqueKey, now.Add(task.UniqueFor))
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue rabbitmq: marshal task: %w", err)
	}

	b.pubMu.Lock()
	defer b.pubMu.Unlock()

	// Ensure queue topology exists.
	if err := b.ensureQueue(b.pubCh, task.Queue); err != nil {
		return err
	}

	msg := amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Body:         data,
	}

	// Delayed delivery: use the delayed exchange with x-delay header (ms).
	if task.ProcessAt.After(now) && b.delay {
		delayMS := task.ProcessAt.Sub(now).Milliseconds()
		msg.Headers = amqp.Table{"x-delay": delayMS}
		task.State = taskqueue.StateScheduled
		return b.pubCh.Publish(b.delayedExchange(), task.Queue, false, false, msg)
	}

	task.State = taskqueue.StatePending
	return b.pubCh.Publish(b.workExchange(), task.Queue, false, false, msg)
}

// Dequeue synchronously pulls the next available task from one of the queues.
// Returns ErrNoTask when no message is available.
func (b *Broker) Dequeue(_ context.Context, queues []string, _ time.Time) (*taskqueue.Task, error) {
	b.getMu.Lock()
	defer b.getMu.Unlock()

	for _, q := range queues {
		// Ensure the queue exists before consuming.
		if err := b.ensureQueue(b.getCh, q); err != nil {
			return nil, err
		}

		msg, ok, err := b.getCh.Get(b.queueName(q), false /* no auto-ack */)
		if err != nil {
			return nil, fmt.Errorf("taskqueue rabbitmq: get from %q: %w", q, err)
		}
		if !ok {
			continue
		}

		var task taskqueue.Task
		if err := json.Unmarshal(msg.Body, &task); err != nil {
			// Poison message — nack without requeue.
			_ = b.getCh.Nack(msg.DeliveryTag, false, false)
			return nil, fmt.Errorf("taskqueue rabbitmq: unmarshal task: %w", err)
		}
		if err := task.Validate(); err != nil {
			// Invalid task fields — treat as poison, nack without requeue.
			_ = b.getCh.Nack(msg.DeliveryTag, false, false)
			return nil, fmt.Errorf("taskqueue rabbitmq: invalid task: %w", err)
		}

		task.State = taskqueue.StateActive
		b.inflight.Store(task.ID, inflightEntry{deliveryTag: msg.DeliveryTag, ch: b.getCh})
		return &task, nil
	}
	return nil, taskqueue.ErrNoTask
}

// Ack acknowledges successful task processing.
func (b *Broker) Ack(_ context.Context, task *taskqueue.Task) error {
	entry, ok := b.inflightLoad(task.ID)
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

	// Remove dedup lock on success.
	if task.UniqueKey != "" {
		b.dedup.Delete(task.UniqueKey)
	}
	return nil
}

// Nack records task failure. If retryAt is zero the task is dead-lettered;
// otherwise it is re-published with a delay via the delayed exchange.
func (b *Broker) Nack(_ context.Context, task *taskqueue.Task, lastErr string, retryAt time.Time) error {
	task.LastError = lastErr
	task.UpdatedAt = time.Now()

	entry, ok := b.inflightLoad(task.ID)
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
		// Dead-letter: publish to dead exchange, then ack original.
		task.State = taskqueue.StateDead
		msg := amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         data,
		}
		if pubErr := b.pubCh.Publish(b.deadExchange(), task.Queue, false, false, msg); pubErr != nil {
			return fmt.Errorf("taskqueue rabbitmq: publish dead task: %w", pubErr)
		}
	} else {
		// Retry: publish with delay via delayed exchange (or immediately if plugin disabled).
		task.State = taskqueue.StateRetry
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
			if pubErr := b.pubCh.Publish(b.delayedExchange(), task.Queue, false, false, msg); pubErr != nil {
				return fmt.Errorf("taskqueue rabbitmq: publish retry task: %w", pubErr)
			}
		} else {
			// No delay plugin — publish immediately to work exchange.
			if pubErr := b.pubCh.Publish(b.workExchange(), task.Queue, false, false, msg); pubErr != nil {
				return fmt.Errorf("taskqueue rabbitmq: publish retry task (no delay): %w", pubErr)
			}
		}
	}

	// Ack the original message so it doesn't go through RabbitMQ's own DLX.
	b.getMu.Lock()
	ackErr := entry.ch.Ack(entry.deliveryTag, false)
	b.getMu.Unlock()
	return ackErr
}

// Schedule is a no-op for RabbitMQ: delayed messages are handled natively by
// the x-delayed-message exchange.
func (b *Broker) Schedule(_ context.Context) error { return nil }

// ReapStale is a no-op for RabbitMQ: when a consumer connection drops,
// RabbitMQ automatically re-queues unacknowledged messages.
func (b *Broker) ReapStale(_ context.Context) error { return nil }

// Close closes the AMQP connection and all channels.
func (b *Broker) Close() error {
	return b.conn.Close()
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (b *Broker) inflightLoad(taskID string) (inflightEntry, bool) {
	v, ok := b.inflight.Load(taskID)
	if !ok {
		return inflightEntry{}, false
	}
	return v.(inflightEntry), true
}
