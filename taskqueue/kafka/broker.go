// Package kafka provides a Kafka-backed implementation of the taskqueue.Broker
// interface using franz-go.
//
// # Topic layout
//
//	tq-{queue}        — main work topic; consumed by the worker consumer group
//	tq-{queue}-retry  — retry messages (header: x-process-at = unix seconds)
//	tq-{queue}-dead   — dead-lettered messages
//
// # Delayed delivery
//
// Delayed tasks (ProcessAt in the future) are produced to the retry topic with
// an "x-process-at" header. The broker's Schedule method periodically moves
// due messages from retry topics to the main topic.
//
// Kafka does not provide message-level acknowledgement. Ack commits the
// consumer group offset for the processed record. Nack (retry) commits the
// original record and produces a new message to the retry topic.
//
// # Usage
//
//	broker, err := kafka.New(kafka.Config{
//	    Brokers:       []string{"localhost:9092"},
//	    ConsumerGroup: "my-app-workers",
//	})
//	defer broker.Close()
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/astra-go/astra/taskqueue"
)

const (
	headerProcessAt = "x-process-at" // unix seconds, string-encoded
)

// Config configures the Kafka broker.
type Config struct {
	// Brokers is the list of Kafka bootstrap broker addresses.
	// e.g. []string{"localhost:9092"}
	Brokers []string

	// KeyPrefix is the prefix for all topic names. Default: "tq".
	KeyPrefix string

	// ConsumerGroup is the Kafka consumer group for the main worker consumers.
	// Default: "tq-workers".
	ConsumerGroup string

	// KGOOpts are additional franz-go options applied to all clients
	// (e.g. TLS config, SASL authentication).
	KGOOpts []kgo.Opt
}

func (c *Config) setDefaults() {
	if c.KeyPrefix == "" {
		c.KeyPrefix = "tq"
	}
	if c.ConsumerGroup == "" {
		c.ConsumerGroup = "tq-workers"
	}
}

// Broker is a Kafka-backed taskqueue.Broker.
type Broker struct {
	producerCl  *kgo.Client // for all produce operations
	consumerCl  *kgo.Client // consumer group on main topics
	scheduleCl  *kgo.Client // manual-offset client for retry topics
	prefix      string
	group       string
	baseOpts    []kgo.Opt

	inflight    sync.Map // taskID → *kgo.Record  (for commit on Ack/Nack)
	knownQueues sync.Map // queue name → struct{} (for Schedule topic list)
}

// ─── Topic helpers ────────────────────────────────────────────────────────────

func (b *Broker) mainTopic(q string) string  { return b.prefix + "-" + q }
func (b *Broker) retryTopic(q string) string { return b.prefix + "-" + q + "-retry" }
func (b *Broker) deadTopic(q string) string  { return b.prefix + "-" + q + "-dead" }

// ─── Constructor ──────────────────────────────────────────────────────────────

// New creates a Broker with three internal kgo.Client instances:
//   - producerCl: produce-only
//   - consumerCl: consumer group on main topics (added dynamically per Dequeue)
//   - scheduleCl: no consumer group, used by Schedule to consume retry topics
func New(cfg Config) (*Broker, error) {
	cfg.setDefaults()

	base := append([]kgo.Opt{kgo.SeedBrokers(cfg.Brokers...)}, cfg.KGOOpts...)

	producerCl, err := kgo.NewClient(base...)
	if err != nil {
		return nil, fmt.Errorf("taskqueue kafka: create producer client: %w", err)
	}

	consumerCl, err := kgo.NewClient(append(base,
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		kgo.DisableAutoCommit(),
	)...)
	if err != nil {
		producerCl.Close()
		return nil, fmt.Errorf("taskqueue kafka: create consumer client: %w", err)
	}

	scheduleCl, err := kgo.NewClient(append(base,
		kgo.DisableAutoCommit(),
	)...)
	if err != nil {
		producerCl.Close()
		consumerCl.Close()
		return nil, fmt.Errorf("taskqueue kafka: create schedule client: %w", err)
	}

	return &Broker{
		producerCl: producerCl,
		consumerCl: consumerCl,
		scheduleCl: scheduleCl,
		prefix:     cfg.KeyPrefix,
		group:      cfg.ConsumerGroup,
		baseOpts:   base,
	}, nil
}

// ─── Broker interface ─────────────────────────────────────────────────────────

// Enqueue produces the task to the appropriate Kafka topic.
// Tasks with ProcessAt in the future are placed in the retry topic so that
// Schedule can promote them to the main topic when due.
func (b *Broker) Enqueue(ctx context.Context, task *taskqueue.Task) error {
	now := time.Now()
	task.UpdatedAt = now

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue kafka: marshal task: %w", err)
	}

	b.knownQueues.Store(task.Queue, struct{}{})

	// Register the main topic with the consumer client (idempotent).
	b.consumerCl.AddConsumeTopics(b.mainTopic(task.Queue))

	var topic string
	var headers []kgo.RecordHeader

	if task.ProcessAt.After(now) {
		// Scheduled task → retry topic with x-process-at header.
		task.State = taskqueue.StateScheduled
		topic = b.retryTopic(task.Queue)
		headers = []kgo.RecordHeader{
			{Key: headerProcessAt, Value: []byte(strconv.FormatInt(task.ProcessAt.Unix(), 10))},
		}
		// Register retry topic with schedule client.
		b.scheduleCl.AddConsumeTopics(b.retryTopic(task.Queue))
	} else {
		task.State = taskqueue.StatePending
		topic = b.mainTopic(task.Queue)
	}

	record := &kgo.Record{
		Topic:   topic,
		Key:     []byte(task.ID),
		Value:   data,
		Headers: headers,
	}

	results := b.producerCl.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("taskqueue kafka: produce to %q: %w", topic, err)
	}
	return nil
}

// Dequeue polls the main consumer topic for one task and returns it.
// Returns ErrNoTask when no message is immediately available.
func (b *Broker) Dequeue(ctx context.Context, queues []string, _ time.Time) (*taskqueue.Task, error) {
	// Register all requested queues with the consumer client.
	for _, q := range queues {
		b.consumerCl.AddConsumeTopics(b.mainTopic(q))
		b.knownQueues.Store(q, struct{}{})
	}

	// Use a short-deadline context to implement non-blocking poll.
	pollCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	fetches := b.consumerCl.PollRecords(pollCtx, 1)
	if fetches.IsClientClosed() {
		return nil, taskqueue.ErrNoTask
	}
	if err := fetches.Err(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, taskqueue.ErrNoTask
	}

	var firstTask *taskqueue.Task
	fetches.EachRecord(func(r *kgo.Record) {
		if firstTask != nil {
			return
		}
		var task taskqueue.Task
		if err := json.Unmarshal(r.Value, &task); err != nil {
			// Poison message — commit and skip.
			_ = b.consumerCl.CommitRecords(ctx, r)
			return
		}
		if err := task.Validate(); err != nil {
			// Invalid task fields — treat as poison, commit and skip.
			_ = b.consumerCl.CommitRecords(ctx, r)
			return
		}
		task.State = taskqueue.StateActive
		b.inflight.Store(task.ID, r)
		firstTask = &task
	})

	if firstTask == nil {
		return nil, taskqueue.ErrNoTask
	}
	return firstTask, nil
}

// Ack commits the consumer offset for the task's Kafka record.
func (b *Broker) Ack(ctx context.Context, task *taskqueue.Task) error {
	record, ok := b.inflightLoad(task.ID)
	if !ok {
		return fmt.Errorf("taskqueue kafka: ack %q: not found in inflight map", task.ID)
	}
	if err := b.consumerCl.CommitRecords(ctx, record); err != nil {
		return fmt.Errorf("taskqueue kafka: commit ack %q: %w", task.ID, err)
	}
	b.inflight.Delete(task.ID)
	return nil
}

// Nack records task failure. For retry tasks it produces to the retry topic
// with an x-process-at header; for dead tasks it produces to the dead topic.
// The original record is committed in both cases.
func (b *Broker) Nack(ctx context.Context, task *taskqueue.Task, lastErr string, retryAt time.Time) error {
	task.LastError = lastErr
	task.UpdatedAt = time.Now()

	record, ok := b.inflightLoad(task.ID)
	if !ok {
		return fmt.Errorf("taskqueue kafka: nack %q: not found in inflight map", task.ID)
	}
	defer b.inflight.Delete(task.ID)

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue kafka: marshal task for nack: %w", err)
	}

	var destTopic string
	var headers []kgo.RecordHeader

	if retryAt.IsZero() {
		task.State = taskqueue.StateDead
		destTopic = b.deadTopic(task.Queue)
	} else {
		task.State = taskqueue.StateRetry
		destTopic = b.retryTopic(task.Queue)
		headers = []kgo.RecordHeader{
			{Key: headerProcessAt, Value: []byte(strconv.FormatInt(retryAt.Unix(), 10))},
		}
		// Make sure the schedule client is watching this retry topic.
		b.scheduleCl.AddConsumeTopics(destTopic)
	}

	newRecord := &kgo.Record{
		Topic:   destTopic,
		Key:     []byte(task.ID),
		Value:   data,
		Headers: headers,
	}
	if results := b.producerCl.ProduceSync(ctx, newRecord); results.FirstErr() != nil {
		return fmt.Errorf("taskqueue kafka: produce nack record to %q: %w", destTopic, results.FirstErr())
	}

	// Commit original record.
	if err := b.consumerCl.CommitRecords(ctx, record); err != nil {
		return fmt.Errorf("taskqueue kafka: commit nack original %q: %w", task.ID, err)
	}
	return nil
}

// Schedule polls retry topics and promotes messages whose x-process-at has
// elapsed to the corresponding main topic.
//
// The scheduleCl uses manual offset management: records that are due are
// committed after being re-produced to the main topic. Records that are not
// yet due cause the partition offset to be reset to their position so they
// are re-polled on the next Schedule call.
func (b *Broker) Schedule(ctx context.Context) error {
	pollCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	fetches := b.scheduleCl.PollRecords(pollCtx, 100)
	if fetches.IsClientClosed() {
		return nil
	}
	if err := fetches.Err(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("taskqueue kafka: schedule poll: %w", err)
	}

	now := time.Now()
	var toCommit []*kgo.Record
	// seeks: topic → partition → earliest offset of a not-yet-due record
	seeks := map[string]map[int32]int64{}

	fetches.EachRecord(func(r *kgo.Record) {
		processAt := int64(0)
		for _, h := range r.Headers {
			if h.Key == headerProcessAt {
				processAt, _ = strconv.ParseInt(string(h.Value), 10, 64)
			}
		}

		if processAt == 0 || time.Unix(processAt, 0).Before(now) {
			// Due — re-produce to main topic.
			var task taskqueue.Task
			if err := json.Unmarshal(r.Value, &task); err == nil {
				mainRecord := &kgo.Record{
					Topic: b.mainTopic(task.Queue),
					Key:   r.Key,
					Value: r.Value,
				}
				if results := b.producerCl.ProduceSync(ctx, mainRecord); results.FirstErr() == nil {
					toCommit = append(toCommit, r)
				}
			}
		} else {
			// Not yet due — record lowest offset per partition for seekback.
			if seeks[r.Topic] == nil {
				seeks[r.Topic] = map[int32]int64{}
			}
			if prev, exists := seeks[r.Topic][r.Partition]; !exists || r.Offset < prev {
				seeks[r.Topic][r.Partition] = r.Offset
			}
		}
	})

	// Commit promoted records.
	if len(toCommit) > 0 {
		if err := b.scheduleCl.CommitRecords(ctx, toCommit...); err != nil {
			return fmt.Errorf("taskqueue kafka: commit schedule records: %w", err)
		}
	}

	// Seek back to earliest unprocessed offset per partition.
	if len(seeks) > 0 {
		epochOffsets := make(map[string]map[int32]kgo.EpochOffset, len(seeks))
		for topic, parts := range seeks {
			epochOffsets[topic] = make(map[int32]kgo.EpochOffset, len(parts))
			for part, offset := range parts {
				epochOffsets[topic][part] = kgo.EpochOffset{Epoch: -1, Offset: offset}
			}
		}
		b.scheduleCl.SetOffsets(epochOffsets)
	}
	return nil
}

// ReapStale is a no-op for the Kafka broker. When a consumer's session
// expires, the consumer group rebalances and the uncommitted records are
// re-delivered to another consumer automatically.
func (b *Broker) ReapStale(_ context.Context) error { return nil }

// Close closes all three internal kgo.Client instances.
func (b *Broker) Close() error {
	b.producerCl.Close()
	b.consumerCl.Close()
	b.scheduleCl.Close()
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (b *Broker) inflightLoad(taskID string) (*kgo.Record, bool) {
	v, ok := b.inflight.Load(taskID)
	if !ok {
		return nil, false
	}
	return v.(*kgo.Record), true
}
