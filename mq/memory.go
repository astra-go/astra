// Memory broker — in-process, channel-based pub/sub for local development and testing.
//
// This broker is NOT distributed; it is suitable only for single-instance
// environments such as unit tests, integration tests, or local dev.
package mq

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryBrokerConfig configures the memory broker.
type MemoryBrokerConfig struct {
	// QueueSize is the per-topic/per-group channel buffer size.
	// Defaults to 1024. A size of 0 means unbuffered (sync send).
	QueueSize int

	// MaxTopics is the maximum number of topics that can be registered.
	// Defaults to 256. New topics beyond this limit cause Publish to return
	// ErrQueueFull after the timeout.
	MaxTopics int

	// FixedDelayLevels defines predefined fixed-delay levels in milliseconds.
	// When non-empty, the MemoryProducer.FixedDelay() method will map a level
	// index to the nearest delay duration.
	// Example: []int64{1000, 5000, 10000, 30000, 60000} (1s/5s/10s/30s/60s)
	FixedDelayLevels []int64
}

// MemoryConsumerConfig configures consumer-side behaviors (retry, DLQ, idempotency, NAK delay).
// All fields have safe zero-value defaults (feature disabled).
type MemoryConsumerConfig struct {
	// MaxRetries is the maximum number of retries when the handler returns an error.
	// 0 disables retry. Each retry uses exponential backoff (2^n seconds).
	MaxRetries int

	// DLQBuffer is the size of the dead-letter channel buffer.
	// 0 disables DLQ. When retries are exhausted, the message is pushed to DLQ.
	DLQBuffer int

	// Idempotent enables dedup based on msg.IdempKey.
	// When true, successfully processed message keys are tracked and ignored
	// on subsequent deliveries.
	Idempotent bool

	// NakDelay is the duration to wait before re-delivering a NAKed message.
	// When 0, NAK falls back to normal retry behavior (immediate requeue or exponential backoff).
	// When > 0, NAK delays re-delivery by this duration.
	NakDelay time.Duration
}

// MemoryBroker is an in-process broker using Go channels.
// It fan-outs published messages to all subscribers of a topic.
type MemoryBroker struct {
	cfg   MemoryBrokerConfig
	topics sync.Map // topic name → *memoryTopic

	// producerCount tracks active producers for metrics.
	producerCount atomic.Int64
	// consumerCount tracks active consumers for metrics.
	consumerCount atomic.Int64
}

// memoryTopic holds subscribers and the message channels for one topic.
// Supports a default channel (backward compatible) and named group channels.
type memoryTopic struct {
	name      string
	defaultCh chan *Message          // consumers without explicit group
	groups    map[string]*memoryGroup // named consumer groups (fan-out)
	mu        sync.RWMutex
	closed    bool
	onClose   func()
	subs      atomic.Int64
}

type memoryGroup struct {
	name string
	ch   chan *Message
}

// NewMemoryBroker creates a new in-process memory broker.
func NewMemoryBroker(cfg MemoryBrokerConfig) *MemoryBroker {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.MaxTopics <= 0 {
		cfg.MaxTopics = 256
	}
	return &MemoryBroker{cfg: cfg}
}

// getOrCreateTopic gets or creates a topic. Returns nil if max topics exceeded.
func (b *MemoryBroker) getOrCreateTopic(name string) (*memoryTopic, error) {
	if v, ok := b.topics.Load(name); ok {
		return v.(*memoryTopic), nil
	}

	// Count existing topics
	n := 0
	b.topics.Range(func(_, _ any) bool {
		n++
		return true
	})
	if n >= b.cfg.MaxTopics {
		return nil, fmt.Errorf("memory broker: max topics %d reached", b.cfg.MaxTopics)
	}

	mt := &memoryTopic{
		name: name,
	}
	mt.onClose = func() {
		b.topics.Delete(name)
		DefaultMetrics().SetMemoryQueueDepth(name, 0)
	}

	prev, loaded := b.topics.LoadOrStore(name, mt)
	if loaded {
		return prev.(*memoryTopic), nil
	}

	DefaultMetrics().IncActiveProducer()
	return mt, nil
}

// Publish delivers a message to the topic's subscriber channels.
// It is non-blocking: if any channel is full, the message is dropped for that channel.
func (b *MemoryBroker) Publish(ctx context.Context, msg *Message) error {
	if msg.Topic == "" {
		return fmt.Errorf("memory broker: empty topic")
	}

	mt, err := b.getOrCreateTopic(msg.Topic)
	if err != nil {
		return err
	}

	mt.mu.RLock()
	closed := mt.closed
	mt.mu.RUnlock()
	if closed {
		return fmt.Errorf("memory broker: topic %q is closed", msg.Topic)
	}

	// Honor delay
	if msg.Delay > 0 {
		go func() {
			t := time.NewTimer(msg.Delay)
			defer t.Stop()
			select {
			case <-t.C:
				b.deliver(mt, msg)
			case <-ctx.Done():
				// Context cancelled; drop the delayed message silently.
			}
		}()
		return nil
	}

	return b.deliver(mt, msg)
}

// deliver sends a message to all subscriber channels (default + all groups).
// It is called synchronously from Publish and from delayed delivery goroutines.
// Non-blocking per-channel: if a channel is full, that subscriber's copy is dropped
// but other subscribers still receive the message.
func (b *MemoryBroker) deliver(mt *memoryTopic, msg *Message) error {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	if mt.closed {
		return nil // Topic closed while waiting to deliver.
	}

	dropped := false

	// Deliver to the default channel (consumers without explicit group)
	if mt.defaultCh != nil {
		select {
		case mt.defaultCh <- msg:
		default:
			dropped = true
		}
	}

	// Deliver to all named group channels (fan-out)
	for _, g := range mt.groups {
		select {
		case g.ch <- msg:
		default:
			dropped = true
		}
	}

	if dropped {
		DefaultMetrics().RecordPublish(msg.Topic, "memory", "dropped", 0)
		return nil // Not an error; some channels may still receive it.
	}

	DefaultMetrics().RecordPublish(msg.Topic, "memory", "ok", 0)
	return nil
}

// Close closes all topic channels and releases resources.
func (b *MemoryBroker) Close() error {
	var errs []error
	b.topics.Range(func(_, v any) bool {
		mt := v.(*memoryTopic)
		mt.mu.Lock()
		if !mt.closed {
			mt.closed = true
			if mt.defaultCh != nil {
				close(mt.defaultCh)
			}
			for _, g := range mt.groups {
				close(g.ch)
			}
			mt.onClose()
		}
		mt.mu.Unlock()
		return true
	})
	DefaultMetrics().DecActiveProducer()
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// TopicNames returns the list of registered topic names.
func (b *MemoryBroker) TopicNames() []string {
	var names []string
	b.topics.Range(func(k, _ any) bool {
		names = append(names, k.(string))
		return true
	})
	return names
}

// QueueDepth returns the number of pending messages for a topic
// across all subscriber channels.
func (b *MemoryBroker) QueueDepth(topic string) int {
	v, ok := b.topics.Load(topic)
	if !ok {
		return 0
	}
	mt := v.(*memoryTopic)
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	depth := 0
	if mt.defaultCh != nil {
		depth += len(mt.defaultCh)
	}
	for _, g := range mt.groups {
		depth += len(g.ch)
	}
	return depth
}

// ErrQueueFull is returned by Publish when the topic channel is full.
var ErrQueueFull = fmt.Errorf("memory broker: channel full")

// ─── Memory Producer ─────────────────────────────────────────────────────────

// MemoryProducer is a single-topic view of a MemoryBroker.
// It implements the Producer interface and is preferred for simple use-cases.
type MemoryProducer struct {
	broker *MemoryBroker
	topic  string
}

// NewMemoryProducer creates a producer that publishes to the given topic.
// Uses a dedicated MemoryBroker. Use NewMemoryProducerWithBroker to share
// a broker with a consumer.
func NewMemoryProducer(topic string) *MemoryProducer {
	return &MemoryProducer{
		broker: NewMemoryBroker(MemoryBrokerConfig{}),
		topic:  topic,
	}
}

// NewMemoryProducerWithBroker creates a producer that shares an existing broker.
// This is the recommended way to connect a producer to a consumer.
func NewMemoryProducerWithBroker(broker *MemoryBroker, topic string) *MemoryProducer {
	return &MemoryProducer{broker: broker, topic: topic}
}

// Publish sends a message to the memory broker's topic.
func (p *MemoryProducer) Publish(ctx context.Context, msg *Message) error {
	msg.Topic = p.topic
	return p.broker.Publish(ctx, msg)
}

// PublishBatch delivers all messages to the same topic.
func (p *MemoryProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	for _, msg := range msgs {
		msg.Topic = p.topic
		if err := p.broker.Publish(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// FixedDelay sends a message with a fixed-delay level.
// Level is an index into MemoryBrokerConfig.FixedDelayLevels (0-based).
// The message will be delivered after the corresponding delay duration.
func (p *MemoryProducer) FixedDelay(ctx context.Context, msg *Message, level int) error {
	levels := p.broker.cfg.FixedDelayLevels
	if len(levels) == 0 {
		return fmt.Errorf("memory broker: FixedDelayLevels not configured")
	}
	if level < 0 || level >= len(levels) {
		return fmt.Errorf("memory broker: fixed delay level %d out of range [0, %d)", level, len(levels))
	}
	msg.Topic = p.topic
	msg.Delay = time.Duration(levels[level]) * time.Millisecond
	return p.broker.Publish(ctx, msg)
}

// Close closes the underlying broker.
func (p *MemoryProducer) Close() error { return p.broker.Close() }

// Capabilities returns the memory broker's capability set.
func (p *MemoryProducer) BeginTransaction(ctx context.Context, _ TransactionChecker) (Transaction, error) {
	return nil, ErrCapTxNotSupported
}

func (p *MemoryProducer) Capabilities() Capabilities { return MemoryCapabilities() }

// ─── Memory Consumer ─────────────────────────────────────────────────────────

// nakEntry holds a NAKed message and the time at which it should be re-delivered.
type nakEntry struct {
	msg       *Message
	deliverAt time.Time
}

// MemoryConsumer is a single-topic consumer of a MemoryBroker.
// It implements the Consumer interface.
//
// Supports retry, DLQ, and idempotency when configured via MemoryConsumerConfig.
type MemoryConsumer struct {
	broker *MemoryBroker
	topic  string
	group  string
	config MemoryConsumerConfig

	dedup      sync.Map // msg.IdempKey → struct{} (dedup after success)
	dlqCh      chan *Message
	nakCh      chan *nakEntry // NAK delay re-delivery queue
}

// NewMemoryConsumer creates a consumer that subscribes to the given topic.
// Uses a dedicated MemoryBroker. Use NewMemoryConsumerWithBroker to share
// a broker with a producer.
func NewMemoryConsumer(topic string, config ...MemoryConsumerConfig) *MemoryConsumer {
	var cfg MemoryConsumerConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	c := &MemoryConsumer{
		broker: NewMemoryBroker(MemoryBrokerConfig{}),
		topic:  topic,
		config: cfg,
		nakCh:  make(chan *nakEntry, 256),
	}
	if cfg.DLQBuffer > 0 {
		c.dlqCh = make(chan *Message, cfg.DLQBuffer)
	}
	return c
}

// NewMemoryConsumerWithBroker creates a consumer that shares an existing broker.
// This is the recommended way to connect a consumer to a producer.
func NewMemoryConsumerWithBroker(broker *MemoryBroker, topic string, config ...MemoryConsumerConfig) *MemoryConsumer {
	var cfg MemoryConsumerConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	c := &MemoryConsumer{
		broker: broker,
		topic:  topic,
		config: cfg,
		nakCh:  make(chan *nakEntry, 256),
	}
	if cfg.DLQBuffer > 0 {
		c.dlqCh = make(chan *Message, cfg.DLQBuffer)
	}
	return c
}

// Subscribe starts a goroutine that calls handler for each published message.
// It blocks until ctx is cancelled or a fatal error occurs.
//
// When group is non-empty, it registers a named consumer group (fan-out).
// When group is empty, it falls back to the default topic channel (competing consumer).
//
// Retry/DLQ/Idempotency behavior is governed by MemoryConsumerConfig.
func (c *MemoryConsumer) Subscribe(ctx context.Context, topics []string, group string, handler Handler) error {
	DefaultMetrics().IncActiveConsumer()
	defer DefaultMetrics().DecActiveConsumer()

	topic := c.topic
	if len(topics) > 0 && topics[0] != "" {
		topic = topics[0]
	}

	mt, err := c.broker.getOrCreateTopic(topic)
	if err != nil {
		return err
	}

	// Register group-specific or default channel
	var subCh chan *Message
	mt.mu.Lock()
	if group != "" {
		if mt.groups == nil {
			mt.groups = make(map[string]*memoryGroup)
		}
		g, ok := mt.groups[group]
		if !ok {
			qSize := c.broker.cfg.QueueSize
			g = &memoryGroup{name: group, ch: make(chan *Message, qSize)}
			mt.groups[group] = g
		}
		subCh = g.ch
	} else {
		if mt.defaultCh == nil {
			mt.defaultCh = make(chan *Message, c.broker.cfg.QueueSize)
		}
		subCh = mt.defaultCh
	}
	mt.subs.Add(1)
	mt.mu.Unlock()

	defer func() {
		mt.subs.Add(-1)
	}()

	c.group = group

	// Start NAK delay re-delivery goroutine
	nakDone := make(chan struct{})
	go func() {
		defer close(nakDone)
		for {
			select {
			case <-ctx.Done():
				return
			case entry, ok := <-c.nakCh:
				if !ok {
					return
				}
				wait := time.Until(entry.deliverAt)
				if wait > 0 {
					t := time.NewTimer(wait)
					select {
					case <-t.C:
						_ = c.broker.Publish(ctx, entry.msg)
					case <-ctx.Done():
						t.Stop()
						return
					}
				} else {
					_ = c.broker.Publish(ctx, entry.msg)
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-subCh:
			if !ok {
				return nil
			}

			// Idempotency check: skip if already processed
			if c.config.Idempotent && msg.IdempKey != "" {
				if _, seen := c.dedup.Load(msg.IdempKey); seen {
					continue
				}
			}

			start := time.Now()
			if err := handler(ctx, msg); err != nil {
				DefaultMetrics().RecordConsume(msg.Topic, "memory", "error", time.Since(start))

				// NAK delay: if configured, re-deliver after NakDelay
				if c.config.NakDelay > 0 {
					c.nakCh <- &nakEntry{
						msg:       msg,
						deliverAt: time.Now().Add(c.config.NakDelay),
					}
				} else if c.config.MaxRetries > 0 {
					// Retry logic (exponential backoff)
					c.handleRetry(ctx, msg)
				}
			} else {
				if c.config.Idempotent && msg.IdempKey != "" {
					c.dedup.Store(msg.IdempKey, struct{}{})
				}
				DefaultMetrics().RecordConsume(msg.Topic, "memory", "ok", time.Since(start))
			}
		}
	}
}

// handleRetry re-queues the message with exponential backoff, or sends it to DLQ.
func (c *MemoryConsumer) handleRetry(ctx context.Context, msg *Message) {
	if msg.RetryCount >= c.config.MaxRetries {
		// Retries exhausted — send to DLQ
		if c.dlqCh != nil {
			select {
			case c.dlqCh <- msg:
			default:
				// DLQ channel full, drop the message silently.
			}
		}
		return
	}

	// Increment retry count on the message
	msg.RetryCount++

	// Exponential backoff: 2^count seconds (truncated to nearest 100ms)
	backoff := time.Duration(math.Pow(2, float64(msg.RetryCount))) * time.Second
	backoff = backoff.Truncate(100 * time.Millisecond)

	go func() {
		time.Sleep(backoff)
		// Re-publish the message for retry.
		// Ignore publish errors (topic may have been closed).
		_ = c.broker.Publish(ctx, msg)
	}()
}

// DLQ returns a channel of dead-letter messages.
// Returns nil if DLQ is not enabled (config.DLQBuffer == 0).
func (c *MemoryConsumer) DLQ() <-chan *Message {
	return c.dlqCh
}

// Close closes the underlying broker.
func (c *MemoryConsumer) Close() error { return c.broker.Close() }

// Capabilities returns the memory broker's capability set.
func (c *MemoryConsumer) Capabilities() Capabilities { return MemoryCapabilities() }

// ─── Capabilities ────────────────────────────────────────────────────────────

// MemoryCapabilities returns the capability set for the in-process memory broker.
// Supports: arbitrary delay, fixed delay levels, NAK delay, ordered delivery,
// batch, multi-group, retry, DLQ, and idempotency.
//
// Does NOT support: priority, transactions.
// Suitable for testing and local development only.
func MemoryCapabilities() Capabilities {
	return Capabilities{
		CapArbitraryDelay: true,
		CapFixedDelay:     true,
		CapNakDelay:       true,
		CapIdempotency:    true,
		CapPriority:       false,
		CapOrdered:        true,
		CapDLQ:            true,
		CapRetry:          true,
		CapMultiGroup:     true,
		CapTx:             false,
		CapBatch:          true,
	}
}
