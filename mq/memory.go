// Memory broker — in-process, channel-based pub/sub for local development and testing.
//
// This broker is NOT distributed; it is suitable only for single-instance
// environments such as unit tests, integration tests, or local dev.
package mq

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryBrokerConfig configures the memory broker.
type MemoryBrokerConfig struct {
	// QueueSize is the per-topic channel buffer size.
	// Defaults to 1024. A size of 0 means unbuffered (sync send).
	QueueSize int

	// MaxTopics is the maximum number of topics that can be registered.
	// Defaults to 256. New topics beyond this limit cause Publish to return
	// ErrQueueFull after the timeout.
	MaxTopics int
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

// memoryTopic holds subscribers and the message channel for one topic.
type memoryTopic struct {
	name      string
	ch        chan *Message
	subs      atomic.Int64
	mu        sync.RWMutex
	closed    bool
	onClose   func()
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
		ch:   make(chan *Message, b.cfg.QueueSize),
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
// It is non-blocking: if the channel is full, it returns ErrQueueFull
// after the context deadline.
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

// deliver sends a message to the topic channel.
// It is called synchronously from Publish and from delayed delivery goroutines.
func (b *MemoryBroker) deliver(mt *memoryTopic, msg *Message) error {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	if mt.closed {
		return nil // Topic closed while waiting to deliver.
	}

	select {
	case mt.ch <- msg:
		DefaultMetrics().RecordPublish(msg.Topic, "memory", "ok", 0)
		return nil
	default:
		DefaultMetrics().RecordPublish(msg.Topic, "memory", "dropped", 0)
		return ErrQueueFull
	}
}

// Close closes all topic channels and releases resources.
func (b *MemoryBroker) Close() error {
	var errs []error
	b.topics.Range(func(_, v any) bool {
		mt := v.(*memoryTopic)
		mt.mu.Lock()
		if !mt.closed {
			mt.closed = true
			close(mt.ch)
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

// QueueDepth returns the number of pending messages for a topic.
func (b *MemoryBroker) QueueDepth(topic string) int {
	v, ok := b.topics.Load(topic)
	if !ok {
		return 0
	}
	return len(v.(*memoryTopic).ch)
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
func NewMemoryProducer(topic string) *MemoryProducer {
	return &MemoryProducer{
		broker: NewMemoryBroker(MemoryBrokerConfig{}),
		topic:  topic,
	}
}

// Publish sends a message to the memory broker's topic.
func (p *MemoryProducer) Publish(ctx context.Context, msg *Message) error {
	msg.Topic = p.topic
	return p.broker.Publish(ctx, msg)
}

// PublishBatch delivers all messages to the same topic.
func (p *MemoryProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	for _, msg := range msgs {
		if err := p.Publish(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the underlying broker.
func (p *MemoryProducer) Close() error { return p.broker.Close() }

// Capabilities returns the memory broker's capability set.
func (p *MemoryProducer) Capabilities() Capabilities { return MemoryCapabilities() }

// ─── Memory Consumer ─────────────────────────────────────────────────────────

// MemoryConsumer is a single-topic consumer of a MemoryBroker.
// It implements the Consumer interface.
type MemoryConsumer struct {
	broker *MemoryBroker
	topic  string
	group  string
}

// NewMemoryConsumer creates a consumer that subscribes to the given topic.
func NewMemoryConsumer(topic string) *MemoryConsumer {
	return &MemoryConsumer{
		broker: NewMemoryBroker(MemoryBrokerConfig{}),
		topic:  topic,
	}
}

// Subscribe starts a goroutine that calls handler for each published message.
// It blocks until ctx is cancelled or a fatal error occurs.
func (c *MemoryConsumer) Subscribe(ctx context.Context, topics []string, group string, handler Handler) error {
	DefaultMetrics().IncActiveConsumer()
	defer DefaultMetrics().DecActiveConsumer()

	mt, err := c.broker.getOrCreateTopic(c.topic)
	if err != nil {
		return err
	}
	mt.subs.Add(1)
	defer mt.subs.Add(-1)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-mt.ch:
			if !ok {
				return nil
			}
			start := time.Now()
			if err := handler(ctx, msg); err != nil {
				DefaultMetrics().RecordConsume(msg.Topic, "memory", "error", time.Since(start))
			} else {
				DefaultMetrics().RecordConsume(msg.Topic, "memory", "ok", time.Since(start))
			}
		}
	}
}

// Close closes the underlying broker.
func (c *MemoryConsumer) Close() error { return c.broker.Close() }

// Capabilities returns the memory broker's capability set.
func (c *MemoryConsumer) Capabilities() Capabilities { return MemoryCapabilities() }
