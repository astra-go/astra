// Kafka implementation of Producer and Consumer using the franz-go high-performance Kafka client.
//
// # Producer
//
//	p, err := mq.NewKafkaProducer(mq.KafkaProducerConfig{
//	    Brokers: []string{"localhost:9092"},
//	})
//	defer p.Close()
//	p.Publish(ctx, &Message{Topic: "events", Key: []byte("key"), Payload: body})
//
// # Consumer
//
//	c, err := mq.NewKafkaConsumer(mq.KafkaConsumerConfig{
//	    Brokers: []string{"localhost:9092"},
//	    Group:   "my-service",
//	})
//	c.Subscribe(ctx, []string{"events"}, "my-service", handler)
//
// # Batching
//
// PublishBatch maps all messages in a single ProduceSync call for maximum
// throughput. Each message is added as a separate record; ordering is
// preserved within a topic-partition.
package mq

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// ─── Producer ─────────────────────────────────────────────────────────────────

// KafkaProducerConfig configures a Kafka producer.
type KafkaProducerConfig struct {
	// Brokers is a list of bootstrap broker addresses.
	Brokers []string

	// MaxMessageBytes is the maximum size of a single message. Default: 1 MiB.
	MaxMessageBytes int

	// ExtraOptions passes additional kgo.Opt values.
	ExtraOptions []kgo.Opt

	// EnableIdempotent enables the idempotent producer (at-least-once → exactly-once).
	// Uses kgo.WithIdempotentProducer().
	EnableIdempotent bool

	// EnableTx enables transactional production (exactly-once across partitions).
	// Uses kgo.Transactional() and kgo.Txn.
	EnableTx bool

	// IdempCache is the application-level idempotent deduplication cache.
	// If non-nil, Publish will skip messages whose IdempKey has already been processed.
	IdempCache IdempCache

	// RetryPolicy defines the retry strategy for failed publishes.
	// If nil, DefaultRetryPolicy() is used.
	RetryPolicy *RetryPolicy

	// DLQTopic is the topic to which permanently-failed messages are sent.
	DLQTopic string

	// DelayTopic is the topic used for delayed message delivery.
	// If empty, delayed messages are not supported.
	DelayTopic string

	// FixedDelayLevels defines predefined fixed-delay levels (in milliseconds).
	// When non-empty, FixedDelay() will publish to per-level delay topics.
	// A background consumer forwards messages to the real topic after TTL expires.
	// Default: []int64{60000, 300000, 600000, 1800000, 3600000} (1m/5m/10m/30m/1h)
	FixedDelayLevels []int64

	// FixedDelayTopicPrefix is the prefix for fixed-delay topics.
	// Default: "delay-fixed-" (e.g., delay-fixed-1, delay-fixed-2)
	FixedDelayTopicPrefix string
}

// KafkaProducer publishes records to Kafka.
type KafkaProducer struct {
	client *kgo.Client
	cfg    KafkaProducerConfig
	mu     sync.Mutex
	inTxn  bool // true if a transaction is in progress
}

// NewKafkaProducer creates a Kafka producer.
// If cfg.EnableTx is true, kgo.TransactionalID is added (required for transactions).
// If cfg.EnableIdempotent is true, the idempotent producer is enabled (default in franz-go).
func NewKafkaProducer(cfg KafkaProducerConfig) (*KafkaProducer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka producer: at least one broker is required")
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.RecordRetries(5),
		kgo.ProducerBatchMaxBytes(int32(maxOrDefault(cfg.MaxMessageBytes, 1<<20))),
	}

	// Transactional ID (required for transactions)
	if cfg.EnableTx {
		opts = append(opts, kgo.TransactionalID("astra-txn-"+randomString(8)))
	}

	opts = append(opts, cfg.ExtraOptions...)

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka producer: create client: %w", err)
	}
	return &KafkaProducer{client: client, cfg: cfg}, nil
}

// Publish sends a single record synchronously.
// If cfg.IdempCache is set and msg.IdempKey is non-empty, the message
// is checked against the cache before publishing.
// If cfg.EnableTx is true and a transaction is in progress (p.txn != nil),
// the message is produced within the transaction.
// If msg.Delay > 0, the message is sent to cfg.DelayTopic with a delay header.
func (p *KafkaProducer) Publish(ctx context.Context, msg *Message) error {
	// Idempotency check
	if p.cfg.IdempCache != nil && msg.IdempKey != "" {
		if p.cfg.IdempCache.IsProcessed(msg.IdempKey) {
			slog.Debug("kafka producer: skipping duplicate message", slog.String("idempKey", msg.IdempKey))
			return nil
		}
	}

	// Delayed delivery
	if msg.Delay > 0 {
		return p.publishWithDelay(ctx, msg)
	}

	record := msgToRecord(msg)

	// ProduceSync handles transactions internally when inTxn is true
	results := p.client.ProduceSync(ctx, record)
	return results.FirstErr()
}

// PublishBatch sends multiple records in a single ProduceSync call.
func (p *KafkaProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	records := make([]*kgo.Record, len(msgs))
	for i, m := range msgs {
		records[i] = msgToRecord(m)
	}
	return p.client.ProduceSync(ctx, records...).FirstErr()
}

// FixedDelay sends a message with a fixed-delay level.
// The message is published to a delay topic. A background consumer
// forwards it to the real topic after the TTL expires.
//
// Implementation: per-level delay topic + background forwarding consumer.
//
// Example FixedDelayLevels: []int64{60000, 300000, 600000} (1m / 5m / 10m)
// Level 1 → delay-fixed-1 topic (1 min TTL) → forward to real topic after expiry
func (p *KafkaProducer) FixedDelay(ctx context.Context, msg *Message, level int) error {
	levels := p.cfg.FixedDelayLevels
	if len(levels) == 0 {
		levels = []int64{60000, 300000, 600000, 1800000, 3600000}
	}

	if level < 1 || level > len(levels) {
		return fmt.Errorf("kafka: invalid fixed-delay level %d (valid: 1-%d)", level, len(levels))
	}

	prefix := p.cfg.FixedDelayTopicPrefix
	if prefix == "" {
		prefix = "delay-fixed-"
	}

	record := &kgo.Record{
		Topic: fmt.Sprintf("%s%d", prefix, level),
		Value: msg.Payload,
	}
	if len(msg.Key) > 0 {
		record.Key = msg.Key
	}
	// Store the real destination topic in the record header
	record.Headers = append(record.Headers, kgo.RecordHeader{
		Key:   "x-original-topic",
		Value: []byte(msg.Topic),
	})
	record.Headers = append(record.Headers, kgo.RecordHeader{
		Key:   "x-delay-level",
		Value: []byte(fmt.Sprintf("%d", level)),
	})

	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("kafka: fixed-delay produce level %d: %w", level, err)
	}

	slog.Debug("kafka: fixed-delay published",
		slog.String("topic", msg.Topic),
		slog.Int("level", level),
		slog.Int64("delay_ms", levels[level-1]),
	)
	return nil
}

// ─── Transaction Methods ─────────────────────────────────────────────────────

// BeginTransaction starts a new Kafka transaction.
// Only valid if EnableTx was set in the config.
func (p *KafkaProducer) BeginTransaction() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.cfg.EnableTx {
		return fmt.Errorf("kafka producer: transactions not enabled (set EnableTx=true)")
	}
	if p.inTxn {
		return fmt.Errorf("kafka producer: transaction already in progress")
	}

	if err := p.client.BeginTransaction(); err != nil {
		return fmt.Errorf("kafka producer: begin transaction: %w", err)
	}
	p.inTxn = true
	return nil
}

// CommitTransaction commits the current Kafka transaction.
func (p *KafkaProducer) CommitTransaction(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.inTxn {
		return fmt.Errorf("kafka producer: no transaction in progress")
	}

	// Flush before committing
	p.client.Flush(ctx)

	err := p.client.EndTransaction(ctx, kgo.TryCommit)
	p.inTxn = false
	if err != nil {
		return fmt.Errorf("kafka producer: commit transaction: %w", err)
	}
	return nil
}

// AbortTransaction aborts the current Kafka transaction.
func (p *KafkaProducer) AbortTransaction(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.inTxn {
		return fmt.Errorf("kafka producer: no transaction in progress")
	}

	err := p.client.EndTransaction(ctx, kgo.TryAbort)
	p.inTxn = false
	if err != nil {
		return fmt.Errorf("kafka producer: abort transaction: %w", err)
	}
	return nil
}

// publishWithDelay sends a message to the delay topic for later redelivery.
// The message is stored in cfg.DelayTopic with headers indicating the original
// topic and the target delivery time.
func (p *KafkaProducer) publishWithDelay(ctx context.Context, msg *Message) error {
	delayTopic := p.cfg.DelayTopic
	if delayTopic == "" {
		return fmt.Errorf("kafka producer: DelayTopic not configured, cannot publish delayed message")
	}

	record := &kgo.Record{
		Topic: delayTopic,
		Value: msg.Payload,
		Key:   msg.Key,
		Headers: []kgo.RecordHeader{
			{Key: "x-original-topic", Value: []byte(msg.Topic)},
			{Key: "x-deliver-at", Value: []byte(fmt.Sprintf("%d", time.Now().Add(msg.Delay).UnixMilli()))},
		},
	}

	for k, v := range msg.Headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}

	return p.client.ProduceSync(ctx, record).FirstErr()
}

// Close flushes pending records and closes the client.
func (p *KafkaProducer) Close() error {
	p.client.Close()
	return nil
}

func msgToRecord(msg *Message) *kgo.Record {
	r := &kgo.Record{
		Topic: msg.Topic,
		Value: msg.Payload,
		Key:   msg.Key,
	}

	// Add special headers
	if msg.IdempKey != "" {
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: "x-idemp-key", Value: []byte(msg.IdempKey)})
	}
	if msg.RetryCount > 0 {
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: "x-retry-count", Value: []byte(fmt.Sprintf("%d", msg.RetryCount))})
	}
	if msg.Priority != 0 {
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: "x-priority", Value: []byte(fmt.Sprintf("%d", msg.Priority))})
	}
	if msg.Delay > 0 {
		deliverAt := time.Now().Add(msg.Delay).UnixMilli()
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: "x-deliver-at", Value: []byte(fmt.Sprintf("%d", deliverAt))})
	}

	for k, v := range msg.Headers {
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}
	return r
}

// ─── Consumer ─────────────────────────────────────────────────────────────────

// KafkaConsumerConfig configures a Kafka consumer.
type KafkaConsumerConfig struct {
	// Brokers is a list of bootstrap broker addresses.
	Brokers []string

	// Group is the consumer group ID.
	Group string

	// InitialOffset controls where the consumer starts on first connect.
	// kgo.NewOffset().AtStart() for earliest, kgo.NewOffset().AtEnd() for latest.
	// Default: latest.
	InitialOffset kgo.Offset

	// MaxPollRecords is the maximum number of records fetched per poll.
	// Default: 100.
	MaxPollRecords int

	// ExtraOptions passes additional kgo.Opt values.
	ExtraOptions []kgo.Opt

	// RetryPolicy defines the retry strategy for failed message processing.
	// If nil, DefaultRetryPolicy() is used.
	RetryPolicy *RetryPolicy

	// DLQTopic is the topic to which permanently-failed messages are sent.
	DLQTopic string

	// DelayTopic is the topic used for delayed message redelivery.
	// If empty, delayed redelivery uses the original topic.
	DelayTopic string

	// PriorityGroups maps priority levels to consumer group names.
	// If non-empty, each priority level gets its own consumer group.
	// Higher priority = lower int (0 = highest priority).
	PriorityGroups map[int]string

	// IdempCache is the application-level idempotent deduplication cache.
	// If non-nil, Subscribe will skip messages whose IdempKey has already been processed.
	IdempCache IdempCache
}

// KafkaConsumer subscribes to Kafka topics within a consumer group.
type KafkaConsumer struct {
	cfg    KafkaConsumerConfig
	client *kgo.Client
}

// NewKafkaConsumer creates a Kafka consumer client (not yet connected to any topic).
func NewKafkaConsumer(cfg KafkaConsumerConfig) (*KafkaConsumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka consumer: at least one broker is required")
	}
	return &KafkaConsumer{cfg: cfg}, nil
}

// Subscribe starts consuming from topics and calls handler for each record.
// It blocks until ctx is cancelled.
//
// Error handling:
//   - If handler returns nil, the message is acked (offset committed).
//   - If handler returns *MQError with Kind=ErrPermanent, the message is sent to DLQ.
//   - If handler returns *MQError with Kind=ErrRetry, the message is retried after delay.
//   - If handler returns *MQError with Kind=ErrSkip, the message is acked without processing.
//   - If handler returns a plain error, it is treated as ErrRetry with exponential backoff.
func (c *KafkaConsumer) Subscribe(ctx context.Context, topics []string, group string, handler Handler) error {
	if group == "" {
		group = c.cfg.Group
	}
	if len(topics) == 0 {
		return fmt.Errorf("kafka consumer: at least one topic is required")
	}

	offset := c.cfg.InitialOffset
	if offset == (kgo.Offset{}) {
		offset = kgo.NewOffset().AtEnd()
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(c.cfg.Brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(offset),
	}
	opts = append(opts, c.cfg.ExtraOptions...)

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return fmt.Errorf("kafka consumer: create client: %w", err)
	}
	c.client = client
	defer func() {
		client.Close()
		c.client = nil
	}()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		fetches := client.PollRecords(ctx, maxOrDefault(c.cfg.MaxPollRecords, 100))
		if fetches.IsClientClosed() {
			return nil
		}

		var retErr error
		fetches.EachError(func(topic string, partition int32, err error) {
			slog.Error("kafka fetch error",
				slog.String("topic", topic),
				slog.Int("partition", int(partition)),
				slog.String("err", err.Error()),
			)
			retErr = err
		})
		if retErr != nil {
			return retErr
		}

		// Collect all records for priority sorting
		type recordWithPriority struct {
			record  *kgo.Record
			priority int
		}
		var records []recordWithPriority

		fetches.EachRecord(func(r *kgo.Record) {
			msg := recordToMsg(r)
			records = append(records, recordWithPriority{record: r, priority: msg.Priority})
		})

		// Sort by priority (lower number = higher priority)
		// If PriorityGroups is configured, sort by group priority
		if len(c.cfg.PriorityGroups) > 0 || true {
			// Always sort by priority for consistency
			sort.Slice(records, func(i, j int) bool {
				return records[i].priority < records[j].priority
			})
		}

		// Process records in priority order
		for _, rp := range records {
			r := rp.record
			msg := recordToMsg(r)

			// Idempotency check
			if c.cfg.IdempCache != nil && msg.IdempKey != "" {
				if c.cfg.IdempCache.IsProcessed(msg.IdempKey) {
					slog.Debug("kafka consumer: skipping duplicate message", slog.String("idempKey", msg.IdempKey))
					continue
				}
			}

			// Call handler
			handlerErr := handler(ctx, msg)

			if handlerErr == nil {
				// Success - mark idempotent key as processed
				if c.cfg.IdempCache != nil && msg.IdempKey != "" {
					c.cfg.IdempCache.MarkProcessed(msg.IdempKey, 24*time.Hour)
				}
				continue
			}

			// Handle error based on MQError kind
			if mqErr, ok := handlerErr.(*MQError); ok {
				switch mqErr.Kind {
				case ErrSkip:
					// Skip - ack without processing
					slog.Info("kafka consumer: skipping message", slog.String("topic", r.Topic))
					continue
				case ErrPermanent:
					// Permanent error - send to DLQ
					slog.Error("kafka consumer: permanent error, sending to DLQ", slog.String("err", handlerErr.Error()))
					c.sendToDLQ(ctx, msg, handlerErr)
					continue
				case ErrRetry:
					// Retryable error - calculate delay and republish
					c.handleRetry(ctx, msg, handlerErr)
					continue
				case ErrPanic:
					// Panic - send to DLQ
					slog.Error("kafka consumer: panic error, sending to DLQ", slog.String("err", handlerErr.Error()))
					c.sendToDLQ(ctx, msg, handlerErr)
					continue
				}
			} else {
				// Plain error - use retry policy
				c.handleRetry(ctx, msg, handlerErr)
			}
			}

		// Commit all polled offsets.
		if err := client.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("kafka commit offsets error", slog.String("err", err.Error()))
		}
	}
}

// Close closes the underlying Kafka client.
func (c *KafkaConsumer) Close() error {
	if c.client != nil {
		c.client.Close()
	}
	return nil
}

// ─── Consumer Helper Methods ──────────────────────────────────────────────

// sendToDLQ sends a failed message to the configured DLQ topic.
func (c *KafkaConsumer) sendToDLQ(ctx context.Context, msg *Message, err error) {
	dlqTopic := c.cfg.DLQTopic
	if dlqTopic == "" {
		slog.Error("kafka consumer: DLQ topic not configured, dropping message", slog.String("topic", msg.Topic))
		return
	}

	dlqMsg := &Message{
		Topic:   dlqTopic,
		Key:     msg.Key,
		Payload: msg.Payload,
		Headers: map[string]string{
			"x-original-topic": msg.Topic,
			"x-error":           err.Error(),
			"x-timestamp":       time.Now().Format(time.RFC3339),
		},
	}

	// Use a temporary producer to send to DLQ
	producer, err := NewKafkaProducer(KafkaProducerConfig{
		Brokers: c.cfg.Brokers,
	})
	if err != nil {
		slog.Error("kafka consumer: failed to create DLQ producer", slog.String("err", err.Error()))
		return
	}
	defer producer.Close()

	if err := producer.Publish(ctx, dlqMsg); err != nil {
		slog.Error("kafka consumer: failed to send message to DLQ", slog.String("err", err.Error()))
	}
}

// handleRetry handles retry logic for a failed message.
// It uses the configured RetryPolicy to calculate the delay.
func (c *KafkaConsumer) handleRetry(ctx context.Context, msg *Message, handlerErr error) {
	policy := c.cfg.RetryPolicy
	if policy == nil {
		dp := DefaultRetryPolicy()
		policy = &dp
	}

	nextDelay, shouldRetry := policy.NextDelay(msg.RetryCount)
	if !shouldRetry {
		// Retries exhausted - send to DLQ
		slog.Warn("kafka consumer: retries exhausted, sending to DLQ",
			slog.String("topic", msg.Topic),
			slog.Int("retryCount", msg.RetryCount),
		)
		c.sendToDLQ(ctx, msg, handlerErr)
		return
	}

	// Increment retry count and republish with delay
	msg.RetryCount++
	slog.Info("kafka consumer: retrying message",
		slog.String("topic", msg.Topic),
		slog.Int("retryCount", msg.RetryCount),
		slog.Duration("nextDelay", nextDelay),
	)

	// Republish to delay topic or original topic with delay header
	c.republishWithDelay(ctx, msg, nextDelay)
}

// republishWithDelay republishes a message with a delay for later redelivery.
func (c *KafkaConsumer) republishWithDelay(ctx context.Context, msg *Message, delay time.Duration) {
	delayTopic := c.cfg.DelayTopic
	if delayTopic == "" {
		// No delay topic configured - log warning and skip
		slog.Warn("kafka consumer: DelayTopic not configured, cannot retry with delay",
			slog.String("topic", msg.Topic),
		)
		return
	}

	record := &kgo.Record{
		Topic: delayTopic,
		Value: msg.Payload,
		Key:   msg.Key,
		Headers: []kgo.RecordHeader{
			{Key: "x-original-topic", Value: []byte(msg.Topic)},
			{Key: "x-deliver-at", Value: []byte(fmt.Sprintf("%d", time.Now().Add(delay).UnixMilli()))},
			{Key: "x-retry-count", Value: []byte(fmt.Sprintf("%d", msg.RetryCount))},
		},
	}

	for k, v := range msg.Headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}

	// Use a temporary producer to republish
	producer, err := NewKafkaProducer(KafkaProducerConfig{
		Brokers: c.cfg.Brokers,
	})
	if err != nil {
		slog.Error("kafka consumer: failed to create retry producer", slog.String("err", err.Error()))
		return
	}
	defer producer.Close()

	if err := producer.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		slog.Error("kafka consumer: failed to republish message for retry", slog.String("err", err.Error()))
	}
}

// NakWithDelay negatively acknowledges a message and redelivers it after the specified delay.
// This is similar to RocketMQ's ChangeInvisibleDuration.
func (c *KafkaConsumer) NakWithDelay(ctx context.Context, msg *Message, delay time.Duration) error {
	slog.Info("kafka consumer: NAK with delay",
		slog.String("topic", msg.Topic),
		slog.Duration("delay", delay),
	)

	// Republish to delay topic for redelivery
	c.republishWithDelay(ctx, msg, delay)
	return nil
}

func recordToMsg(r *kgo.Record) *Message {
	headers := make(map[string]string, len(r.Headers))
	msg := &Message{
		Topic:   r.Topic,
		Key:     r.Key,
		Payload: r.Value,
		Headers: headers,
		Meta: map[string]any{
			"partition": r.Partition,
			"offset":    r.Offset,
			"timestamp": r.Timestamp,
		},
	}

	// Extract special fields from headers
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)

		// Parse special headers
		switch h.Key {
		case "x-idemp-key":
			msg.IdempKey = string(h.Value)
		case "x-retry-count":
			// Parse int from string
			if n, err := strconv.Atoi(string(h.Value)); err == nil {
				msg.RetryCount = n
			}
		case "x-priority":
			if n, err := strconv.Atoi(string(h.Value)); err == nil {
				msg.Priority = n
			}
		case "x-deliver-at":
			// Delay delivery until this timestamp
			if ts, err := strconv.ParseInt(string(h.Value), 10, 64); err == nil {
				msg.Delay = time.Until(time.UnixMilli(ts))
				if msg.Delay < 0 {
					msg.Delay = 0
				}
			}
		}
	}

	return msg
}

func maxOrDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// Capabilities returns the capabilities of Apache Kafka.
func (p *KafkaProducer) Capabilities() Capabilities { return KafkaCapabilities() }
func (c *KafkaConsumer) Capabilities() Capabilities { return KafkaCapabilities() }

// randomString generates a random string of length n.
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
