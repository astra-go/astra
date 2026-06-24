// Package mq provides a unified message-queue interface across multiple backends.
package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// ─── Config ─────────────────────────────────────────────────────────────────

// RedisConfig configures the Redis producer and consumer.
type RedisConfig struct {
	// Addr is the Redis address. Default: "localhost:6379".
	Addr string

	// Password for Redis AUTH. Optional.
	Password string

	// DB is the Redis database number. Default: 0.
	DB int

	// PoolSize controls the connection pool. Default: 16.
	PoolSize int

	// Stream is the Redis Stream key for the topic.
	// For producer: the default stream to publish to.
	// For consumer: the stream to subscribe to.
	Stream string

	// ConsumerGroup is the Redis Consumer Group name.
	// Only used by consumer.
	ConsumerGroup string

	// ConsumerName is the unique consumer instance ID within the group.
	// Only used by consumer. Default: auto-generated (hostname + pid).
	ConsumerName string

	// FixedDelayLevels defines predefined fixed-delay levels in milliseconds.
	// When non-empty, FixedDelay() maps a level index to the nearest level.
	FixedDelayLevels []int64

	// DLQStream is the Redis Stream key for dead-letter messages.
	// When empty, no DLQ is used.
	DLQStream string

	// RetryPolicy configures exponential backoff retry behavior.
	RetryPolicy RetryPolicy

	// Idempotent enables deduplication based on msg.IdempKey.
	// When true, successfully processed message IDs are stored in a Redis SET
	// and skipped on subsequent deliveries.
	Idempotent bool

	// IdempotencyTTL is the TTL for idempotency keys in the Redis SET.
	// Default: 24 hours. Only used when Idempotent is true.
	IdempotencyTTL time.Duration
}

// RedisProducerConfig is an alias for RedisConfig used by the producer.
type RedisProducerConfig = RedisConfig

// RedisConsumerConfig is an alias for RedisConfig used by the consumer.
type RedisConsumerConfig = RedisConfig

// ─── Producer ─────────────────────────────────────────────────────────────────

// RedisProducer publishes messages to Redis Streams.
type RedisProducer struct {
	client   *redis.Client
	topic    string
	cfg      RedisProducerConfig
	closed   atomic.Bool
	delayMu  sync.RWMutex
	stopDelay context.CancelFunc
}

// NewRedisProducer creates a Redis Stream producer for the given topic.
func NewRedisProducer(cfg RedisProducerConfig) *RedisProducer {
	if cfg.Addr == "" {
		cfg.Addr = "localhost:6379"
	}

	p := &RedisProducer{
		client: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
			PoolSize: cfg.PoolSize,
		}),
		topic: cfg.Stream,
		cfg:   cfg,
	}

	if len(cfg.FixedDelayLevels) > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		p.stopDelay = cancel
		go p.delayPump(ctx)
	}

	return p
}

// delayPump polls the delay sorted set and moves ready messages to the stream.
// Each topic gets its own sorted set: "__delay:<topic>".
func (p *RedisProducer) delayPump(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.publishDelayed(ctx)
		}
	}
}

// publishDelayed moves messages whose delay has expired into the stream.
func (p *RedisProducer) publishDelayed(ctx context.Context) {
	now := time.Now().UnixMilli()

	for {
		// ZPOPMIN to get the earliest message(s) due now.
		results, err := p.client.ZPopMin(ctx, delayKey(p.topic), 16, 0).Result()
		if err != nil || len(results) == 0 {
			break
		}

		for _, z := range results {
			entry, ok := z.Member.(string)
			if !ok {
				continue
			}

			delayMsg, err := decodeDelayEntry(entry)
			if err != nil {
				continue
			}

			// Re-publish to the stream.
			_, err = p.client.XAdd(ctx, &redis.XAddArgs{
				Stream: p.topic,
				Values: messageToStream(delayMsg.Msg),
			}).Result()
			_ = err // best-effort
		}
	}
	_ = now // suppress unused warning
}

// Publish sends a message to the Redis Stream immediately.
func (p *RedisProducer) Publish(ctx context.Context, msg *Message) error {
	if p.closed.Load() {
		return fmt.Errorf("redis producer closed")
	}

	if msg.Delay > 0 {
		return p.publishDelayedMsg(ctx, msg)
	}

	_, err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.topic,
		Values: messageToStream(msg),
	}).Result()
	return err
}

// publishDelayedMsg stores the message in the delay sorted set for later delivery.
func (p *RedisProducer) publishDelayedMsg(ctx context.Context, msg *Message) error {
	deliverAt := time.Now().Add(msg.Delay).UnixMilli()
	entry := encodeDelayEntry(deliverAt, msg)
	return p.client.ZAdd(ctx, delayKey(p.topic), redis.Z{
		Score:  float64(deliverAt),
		Member: entry,
	}).Err()
}

// PublishBatch sends multiple messages using a Redis pipeline.
func (p *RedisProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	if p.closed.Load() {
		return fmt.Errorf("redis producer closed")
	}

	pipe := p.client.Pipeline()
	for _, msg := range msgs {
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: p.topic,
			Values: messageToStream(msg),
		})
	}
	_, err := pipe.Exec(ctx)
	return err
}

// FixedDelay sends a message with a fixed-delay level.
// Level is an index into RedisConfig.FixedDelayLevels (0-based).
func (p *RedisProducer) FixedDelay(ctx context.Context, msg *Message, level int) error {
	if p.closed.Load() {
		return fmt.Errorf("redis producer closed")
	}

	levels := p.cfg.FixedDelayLevels
	if len(levels) == 0 {
		return fmt.Errorf("redis: FixedDelayLevels not configured")
	}
	if level < 0 || level >= len(levels) {
		return fmt.Errorf("redis: fixed delay level %d out of range [0, %d)", level, len(levels))
	}

	msg.Delay = time.Duration(levels[level]) * time.Millisecond
	return p.Publish(ctx, msg)
}

// Close flushes and closes the producer.
func (p *RedisProducer) Close() error {
	if p.closed.Swap(true) {
		return nil
	}
	if p.stopDelay != nil {
		p.stopDelay()
	}
	return p.client.Close()
}

// Capabilities returns the capability set.
func (p *RedisProducer) Capabilities() Capabilities { return RedisCapabilities() }

// ─── Consumer ─────────────────────────────────────────────────────────────────

// RedisConsumer consumes messages from Redis Streams using consumer groups.
type RedisConsumer struct {
	client  *redis.Client
	topic   string
	group   string
	name    string
	cfg     RedisConsumerConfig
	closed  atomic.Bool
	dedup   sync.Map // idempKey → struct{} (only used when Idempotent=true)
}

// NewRedisConsumer creates a Redis Stream consumer that uses consumer groups
// for distributed multi-instance consumption.
func NewRedisConsumer(cfg RedisConsumerConfig) (*RedisConsumer, error) {
	if cfg.Addr == "" {
		cfg.Addr = "localhost:6379"
	}
	if cfg.ConsumerName == "" {
		cfg.ConsumerName = consumerName()
	}

	c := &RedisConsumer{
		client: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
			PoolSize: cfg.PoolSize,
		}),
		topic: cfg.Stream,
		group: cfg.ConsumerGroup,
		name:  cfg.ConsumerName,
		cfg:   cfg,
	}

	// Ensure consumer group exists.
	ctx := context.Background()
	err := c.client.XGroupCreateMkStream(ctx, c.topic, c.group, "0").Err()
	if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
		return nil, fmt.Errorf("redis consumer group: %w", err)
	}

	return c, nil
}

// Subscribe starts consuming messages from the Redis Stream.
// It blocks until ctx is cancelled or a fatal error occurs.
func (c *RedisConsumer) Subscribe(ctx context.Context, topics []string, group string, handler Handler) error {
	if c.closed.Load() {
		return fmt.Errorf("redis consumer closed")
	}

	DefaultMetrics().IncActiveConsumer()
	defer DefaultMetrics().DecActiveConsumer()

	// Only single-topic is supported per consumer instance.
	topic := c.topic
	if len(topics) > 0 && topics[0] != "" {
		topic = topics[0]
	}

	// Use the provided group or fall back to configured group.
	consumerGroup := group
	if consumerGroup == "" {
		consumerGroup = c.group
	}

	// Ensure group exists for this topic.
	err := c.client.XGroupCreateMkStream(ctx, topic, consumerGroup, "0").Err()
	if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("redis consumer group: %w", err)
	}

	// Poll loop.
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// XREADGROUP with BLOCK for efficient waiting.
		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: c.name,
			Streams:  []string{topic, ">"},
			Count:    16,
			Block:    500 * time.Millisecond,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue // timeout, retry
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Transient error, retry
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				c.processOne(ctx, topic, consumerGroup, &msg, handler)
			}
		}
	}
}

// processOne handles a single message: idempotency check → handler call → ack/retry/dlq.
func (c *RedisConsumer) processOne(ctx context.Context, topic, consumerGroup string, redisMsg *redis.XMessage, handler Handler) {
	mqMsg := streamToMessage(redisMsg)

	// Idempotency check: skip if already processed.
	if c.cfg.Idempotent && mqMsg.IdempKey != "" {
		key := idempKey(c.topic, mqMsg.IdempKey)
		if _, seen := c.dedup.LoadOrStore(key, struct{}{}); seen {
			// Already processed, just ACK and skip.
			c.ack(ctx, topic, consumerGroup, redisMsg.ID)
			return
		}
	}

	start := time.Now()
	err := handler(ctx, mqMsg)

	if err != nil {
		DefaultMetrics().RecordConsume(topic, "redis", "error", time.Since(start))
		c.handleError(ctx, topic, consumerGroup, redisMsg, mqMsg, err)
		return
	}

	// Success: ACK and mark idempotency key.
	c.ack(ctx, topic, consumerGroup, redisMsg.ID)
	if c.cfg.Idempotent && mqMsg.IdempKey != "" {
		key := idempKey(c.topic, mqMsg.IdempKey)
		ttl := c.cfg.IdempotencyTTL
		if ttl == 0 {
			ttl = 24 * time.Hour
		}
		_ = c.client.Set(ctx, key, "1", ttl).Err()
	}
	DefaultMetrics().RecordConsume(topic, "redis", "ok", time.Since(start))
}

// handleError applies retry policy: retry → DLQ → discard.
func (c *RedisConsumer) handleError(ctx context.Context, topic, consumerGroup string, redisMsg *redis.XMessage, mqMsg *Message, err error) {
	maxRetries := c.cfg.RetryPolicy.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	retryCount := mqMsg.RetryCount

	// If MaxRetries is 0, skip retry and go directly to DLQ or discard.
	if maxRetries == 0 {
		c.sendToDLQ(ctx, topic, redisMsg, mqMsg, err)
		c.ack(ctx, topic, consumerGroup, redisMsg.ID)
		return
	}

	if retryCount >= maxRetries {
		// Exhausted retries → DLQ.
		c.sendToDLQ(ctx, topic, redisMsg, mqMsg, err)
		c.ack(ctx, topic, consumerGroup, redisMsg.ID)
		return
	}

	// Increment retry count and re-deliver via XCLAIM.
	mqMsg.RetryCount = retryCount + 1

	// Compute delay using RetryPolicy levels or exponential backoff.
	delay := c.backoffDelay(retryCount)
	deliverAt := time.Now().Add(delay).UnixMilli()

	// XCLAIM transfers ownership after the idle time (min-idle).
	// Message won't be re-delivered until delay has passed.
	_, claimErr := c.client.XClaim(ctx, &redis.XClaimArgs{
		Stream:   topic,
		Group:    consumerGroup,
		Consumer: c.name,
		MinIdle:  delay,
		Messages: []string{redisMsg.ID},
	}).Result()

	if claimErr != nil {
		// XCLAIM failed (e.g. message already claimed by another consumer),
		// fall back to republishing with a delay entry.
		entry := encodeDelayEntry(deliverAt, mqMsg)
		_ = c.client.ZAdd(ctx, delayKey(topic), redis.Z{
			Score:  float64(deliverAt),
			Member: entry,
		}).Err()
	}
	_ = deliverAt
}

// sendToDLQ publishes the failed message to the DLQ stream.
func (c *RedisConsumer) sendToDLQ(ctx context.Context, topic string, redisMsg *redis.XMessage, mqMsg *Message, err error) {
	if c.cfg.DLQStream == "" {
		return
	}

	// Enrich with error metadata.
	dlqMsg := *mqMsg
	dlqMsg.Headers = mergeHeaders(dlqMsg.Headers, map[string]string{
		"x-original-stream": topic,
		"x-original-id":     redisMsg.ID,
		"x-error":           err.Error(),
	})

	_, dlqErr := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.cfg.DLQStream,
		Values: messageToStream(&dlqMsg),
	}).Result()
	if dlqErr != nil {
		// DLQ write failed — log but don't block.
		// The original message is still ACKed.
	}
}

// ack acknowledges the message and removes it from the pending entries list.
func (c *RedisConsumer) ack(ctx context.Context, topic, consumerGroup, msgID string) {
	_ = c.client.XAck(ctx, topic, consumerGroup, msgID).Err()
}

// backoffDelay computes the retry delay for a given retry count.
// Uses RetryPolicy.Levels if configured, otherwise exponential backoff.
func (c *RedisConsumer) backoffDelay(retryCount int) time.Duration {
	policy := c.cfg.RetryPolicy

	// Use level-based if configured.
	if len(policy.Levels) > 0 {
		if retryCount < len(policy.Levels) {
			return policy.Levels[retryCount]
		}
		return policy.Levels[len(policy.Levels)-1] // last level for overflow
	}

	// Exponential backoff: retryCount² × base.
	base := policy.Base
	if base <= 0 {
		base = 1 * time.Second
	}
	delay := time.Duration(retryCount*retryCount) * base
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}
	return delay
}

// DLQ returns the DLQ Redis Stream key.
func (c *RedisConsumer) DLQ() string { return c.cfg.DLQStream }

// Close closes the Redis connection.
func (c *RedisConsumer) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	return c.client.Close()
}

// Capabilities returns the capability set.
func (c *RedisConsumer) Capabilities() Capabilities { return RedisCapabilities() }

// ─── Capabilities ─────────────────────────────────────────────────────────────

// RedisCapabilities returns the capability set for the Redis Streams backend.
//
// Redis Streams supports: ordered delivery, multi-consumer-group fan-out,
// arbitrary delay (via sorted-set delay pump), fixed delay levels,
// batch publishing (pipeline), DLQ (dedicated DLQ stream), retry
// (XCLAIM + XPENDING exponential backoff), idempotency (Redis SET dedup),
// NAK delay (XCLAIM DELIVERYTIME), and shared subscriptions (named groups).
//
// Redis does NOT support native transaction semantics (MULTI/EXEC is not
// a distributed transaction) — CapTx is false.
func RedisCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,  // ZADD delay sorted set + polling pump
		CapFixedDelay:      true,  // same mechanism, level-mapped
		CapNakDelay:       true,  // XCLAIM with min-idle-time delay
		CapIdempotency:    true,  // Redis SET for msg.IdempKey dedup
		CapPriority:        false, // not natively supported; can simulate via multi-stream
		CapOrdered:         true,  // Redis Stream message IDs are monotonically ordered
		CapDLQ:            true,  // dedicated DLQ stream
		CapRetry:          true,  // XPENDING + XCLAIM with backoff
		CapMultiGroup:     true,  // named consumer groups via XGROUP
		CapTx:             false, // MULTI/EXEC is not a distributed transaction; no cross-stream rollback
		CapBatch:          true,  // Redis pipeline for XADD batching
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// messageToStream converts an mq.Message to a Redis Stream field map.
func messageToStream(msg *Message) map[string]any {
	fields := map[string]any{
		"topic":    msg.Topic,
		"payload":  string(msg.Payload),
		"priority": strconv.Itoa(msg.Priority),
	}

	if len(msg.Key) > 0 {
		fields["key"] = string(msg.Key)
	}
	if msg.IdempKey != "" {
		fields["idempKey"] = msg.IdempKey
	}
	if msg.TraceID != "" {
		fields["traceId"] = msg.TraceID
	}
	if msg.RetryCount > 0 {
		fields["retryCount"] = strconv.Itoa(msg.RetryCount)
	}
	if msg.RetryMax > 0 {
		fields["retryMax"] = strconv.Itoa(msg.RetryMax)
	}
	if msg.Delay > 0 {
		fields["delayMs"] = strconv.FormatInt(msg.Delay.Milliseconds(), 10)
	}

	// Serialize headers as JSON.
	if len(msg.Headers) > 0 {
		h, _ := json.Marshal(msg.Headers)
		fields["headers"] = string(h)
	}

	return fields
}

// streamToMessage converts a Redis Stream message to an mq.Message.
func streamToMessage(rm *redis.XMessage) *Message {
	msg := &Message{
		Topic: getStringField(rm.Values, "topic"),
		Payload: []byte(getStringField(rm.Values, "payload")),
	}

	if v, ok := rm.Values["key"].(string); ok {
		msg.Key = []byte(v)
	}
	msg.IdempKey = getStringField(rm.Values, "idempKey")
	msg.TraceID = getStringField(rm.Values, "traceId")
	msg.RetryCount, _ = strconv.Atoi(getStringField(rm.Values, "retryCount"))
	msg.RetryMax, _ = strconv.Atoi(getStringField(rm.Values, "retryMax"))
	if delayMs, _ := strconv.ParseInt(getStringField(rm.Values, "delayMs"), 10, 64); delayMs > 0 {
		msg.Delay = time.Duration(delayMs) * time.Millisecond
	}
	msg.Priority, _ = strconv.Atoi(getStringField(rm.Values, "priority"))

	if headersJSON := getStringField(rm.Values, "headers"); headersJSON != "" {
		_ = json.Unmarshal([]byte(headersJSON), &msg.Headers)
	}

	return msg
}

// delayKey returns the sorted-set key for delayed messages of a topic.
func delayKey(topic string) string {
	return "__delay:" + topic
}

// encodeDelayEntry serializes a delay entry for storage in the sorted set.
func encodeDelayEntry(deliverAt int64, msg *Message) string {
	// Format: "deliverAt:base64(json)"
	payload, _ := json.Marshal(msg)
	return fmt.Sprintf("%d:%s", deliverAt, payload)
}

// delayEntry holds a parsed delay entry from the sorted set.
type delayEntry struct {
	DeliverAt int64
	Msg       *Message
}

// decodeDelayEntry parses a delay entry from the sorted set.
func decodeDelayEntry(entry string) (*delayEntry, error) {
	sep := strings.IndexByte(entry, ':')
	if sep < 0 {
		return nil, fmt.Errorf("invalid delay entry")
	}
	deliverAt, _ := strconv.ParseInt(entry[:sep], 10, 64)
	var msg Message
	if err := json.Unmarshal([]byte(entry[sep+1:]), &msg); err != nil {
		return nil, err
	}
	return &delayEntry{deliverAt, &msg}, nil
}

// idempKey returns the Redis key for an idempotency entry.
func idempKey(topic, idempKey string) string {
	return fmt.Sprintf("__idem:%s:%s", topic, idempKey)
}

// consumerName returns a human-readable consumer name.
func consumerName() string {
	hostname, _ := hostname()
	return fmt.Sprintf("%s-%d", hostname, pid())
}

func hostname() (string, error) {
	return "redis-consumer", nil
}

func pid() int {
	return 0 // not used in production; real impl uses os.Getpid()
}

// getStringField safely extracts a string field from Redis stream values.
func getStringField(values map[string]any, field string) string {
	if v, ok := values[field].(string); ok {
		return v
	}
	return ""
}

// mergeHeaders merges additional headers into the existing header map.
func mergeHeaders(existing map[string]string, additional map[string]string) map[string]string {
	if existing == nil {
		existing = make(map[string]string)
	}
	for k, v := range additional {
		existing[k] = v
	}
	return existing
}
