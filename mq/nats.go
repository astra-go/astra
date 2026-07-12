// Package nats provides a NATS implementation of mq.Producer and mq.Consumer.
//
// NATS is a lightweight, high-performance messaging system. It supports both
// Core NATS (at-most-once delivery) and JetStream (persistent, at-least-once).
//
// # Core NATS Producer
//
//	p, err := nats.NewNATSProducer(nats.NATSConfig{URL: "nats://localhost:4222"})
//	defer p.Close()
//	p.Publish(ctx, &Message{Topic: "orders.created", Payload: body})
//
// # Core NATS Consumer
//
//	c, err := nats.NewNATSConsumer(nats.NATSConsumerConfig{
//	    Config: nats.NATSConfig{URL: "nats://localhost:4222"},
//	})
//	c.Subscribe(ctx, []string{"orders.*"}, "order-service", handler)
//
// # JetStream Consumer (with DLQ, retry, ordered delivery)
//
//	c, err := nats.NewNATSConsumer(nats.NATSConsumerConfig{
//	    NATSConfig: nats.NATSConfig{
//	        URL:              "nats://localhost:4222",
//	        EnableJetStream:  true,
//	        MaxDeliver:       5,
//	        DLQSubject:       "orders.dlq",
//	    },
//	    EnableOrdered: true,
//	})
package mq

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSConfig configures a NATS client connection.
type NATSConfig struct {
	// URL is the NATS server URL. Default: "nats://localhost:4222"
	URL string

	// Username and Password for authentication.
	Username string
	Password string

	// Token for token-based authentication.
	Token string

	// Name is the client name visible in server monitoring.
	Name string

	// MaxReconnects is the maximum number of reconnection attempts.
	// -1 = unlimited. Default: 60.
	MaxReconnects int

	// ReconnectWait is the delay between reconnection attempts. Default: 2s.
	ReconnectWait time.Duration

	// Timeout is the connection timeout. Default: 2s.
	Timeout time.Duration

	// EnableJetStream enables JetStream for persistent messaging.
	EnableJetStream bool

	// MaxDeliver is the maximum number of delivery attempts before moving to DLQ.
	// Only applies when EnableJetStream is true.
	// Default: 0 (unlimited, no DLQ).
	MaxDeliver int

	// DLQSubject is the subject for dead-letter messages.
	// When MaxDeliver > 0, messages exceeding the limit are forwarded here.
	DLQSubject string

	// EnableIdempotency enables JetStream KV-based idempotency deduplication.
	// Only applies when EnableJetStream is true.
	EnableIdempotency bool

	// IdempBucket is the JetStream KV bucket name for idempotency keys.
	// Default: "mq_idempotency".
	IdempBucket string

	// FixedDelayLevels defines predefined fixed-delay levels (in milliseconds).
	// When non-empty, FixedDelay() will use JetStream scheduled delivery.
	// Default: []int64{60000, 300000, 600000, 1800000, 3600000} (1m/5m/10m/30m/1h)
	FixedDelayLevels []int64

	// PriorityLevels defines number of priority levels for CapPriority.
	// When > 0, Publish with Priority field will route to topic.p0/p1/...pN subjects.
	// Consumer with EnablePriority will drain high-priority messages first.
	// Default: 3 (p0=high, p1=medium, p2=low)
	PriorityLevels int
}

func (c *NATSConfig) setDefaults() {
	if c.URL == "" {
		c.URL = nats.DefaultURL
	}
	if c.MaxReconnects == 0 {
		c.MaxReconnects = 60
	}
	if c.ReconnectWait == 0 {
		c.ReconnectWait = 2 * time.Second
	}
	if c.Timeout == 0 {
		c.Timeout = 2 * time.Second
	}
	if c.IdempBucket == "" {
		c.IdempBucket = "mq_idempotency"
	}
	if c.PriorityLevels == 0 {
		c.PriorityLevels = 3 // p0/p1/p2
	}
}

func (c *NATSConfig) buildOptions() []nats.Option {
	opts := []nats.Option{
		nats.MaxReconnects(c.MaxReconnects),
		nats.ReconnectWait(c.ReconnectWait),
		nats.Timeout(c.Timeout),
	}
	if c.Name != "" {
		opts = append(opts, nats.Name(c.Name))
	}
	if c.Token != "" {
		opts = append(opts, nats.Token(c.Token))
	}
	if c.Username != "" {
		opts = append(opts, nats.UserInfo(c.Username, c.Password))
	}
	return opts
}

// ─── Producer ─────────────────────────────────────────────────────────────────

// NATSProducer publishes messages to NATS.
type NATSProducer struct {
	cfg  NATSConfig
	conn *nats.Conn
	js   nats.JetStreamContext
	kv   nats.KeyValue // for idempotency deduplication
}

// NewNATSProducer creates and connects a NATS producer.
func NewNATSProducer(cfg NATSConfig) (*NATSProducer, error) {
	cfg.setDefaults()
	conn, err := nats.Connect(cfg.URL, cfg.buildOptions()...)
	if err != nil {
		return nil, fmt.Errorf("nats producer: connect: %w", err)
	}

	p := &NATSProducer{cfg: cfg, conn: conn}

	if cfg.EnableJetStream {
		js, err := conn.JetStream()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("nats producer: jetstream: %w", err)
		}
		p.js = js

		// Initialize KV store for idempotency if enabled
		if cfg.EnableIdempotency {
			kv, err := p.initKV()
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("nats producer: init kv: %w", err)
			}
			p.kv = kv
		}
	}

	return p, nil
}

// initKV creates or retrieves the KV bucket for idempotency.
func (p *NATSProducer) initKV() (nats.KeyValue, error) {
	kv, err := p.js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      p.cfg.IdempBucket,
		Description: "MQ idempotency key store",
		MaxValueSize: 16, // store small values (timestamp-ish)
	})
	if err != nil {
		// Bucket might already exist
		kv, err = p.js.KeyValue(p.cfg.IdempBucket)
		if err != nil {
			return nil, fmt.Errorf("create/get kv bucket %s: %w", p.cfg.IdempBucket, err)
		}
	}
	return kv, nil
}

// ArbitraryDelay sends a message with an arbitrary delay duration.
//
// Implementation for NATS v1 (JetStream):
//   - Publishes to internal subject "delay.arbitrary"
//   - Header x-deliver-at contains Unix timestamp (milliseconds)
//   - Re-publisher goroutine (started by StartArbitraryDelayPublisher) forwards after delay
//
// IMPORTANT: Call StartArbitraryDelayPublisher once per producer to enable forwarding.
// This is a client-side simulation; NATS server has no native arbitrary-delay support.
func (p *NATSProducer) ArbitraryDelay(ctx context.Context, msg *Message, delay time.Duration) error {
	if delay <= 0 {
		return fmt.Errorf("nats: delay must be positive, got %v", delay)
	}

	deliverAt := time.Now().Add(delay).UnixMilli()

	// Publish to internal delay subject
	delaySubj := "delay.arbitrary"
	natsMsg := nats.NewMsg(delaySubj)
	natsMsg.Data = msg.Payload
	natsMsg.Header = nats.Header{}
	natsMsg.Header.Set("x-original-topic", msg.Topic)
	natsMsg.Header.Set("x-deliver-at", fmt.Sprintf("%d", deliverAt))
	natsMsg.Header.Set("x-delay-ms", fmt.Sprintf("%d", delay.Milliseconds()))
	if len(msg.Key) > 0 {
		natsMsg.Header.Set("Nats-Msg-Key", string(msg.Key))
	}
	if msg.TraceID != "" {
		natsMsg.Header.Set("Nats-Trace-Id", msg.TraceID)
	}

	if p.js != nil {
		// JetStream: persist in delay subject
		_, err := p.js.PublishMsg(natsMsg)
		if err != nil {
			return fmt.Errorf("nats: arbitrary-delay publish: %w", err)
		}
	} else {
		// Core NATS: publish with timestamp header (consumer-side handling)
		natsMsg.Subject = msg.Topic
		natsMsg.Header.Set("x-deliver-at", fmt.Sprintf("%d", deliverAt))
		if err := p.conn.PublishMsg(natsMsg); err != nil {
			return fmt.Errorf("nats: arbitrary-delay publish: %w", err)
		}
	}

	slog.Debug("nats: arbitrary-delay published",
		slog.String("topic", msg.Topic),
		slog.Duration("delay", delay),
		slog.Int64("deliver_at", deliverAt),
	)
	return nil
}

// Publish sends a message to NATS.
// If msg.Priority >= 0 and p.cfg.PriorityLevels > 0, routes to priority subject.
func (p *NATSProducer) Publish(ctx context.Context, msg *Message) error {
	// Priority routing: rewrite subject to topic.pN
	if msg.Priority >= 0 && p.cfg.PriorityLevels > 0 {
		level := msg.Priority
		if level >= p.cfg.PriorityLevels {
			level = p.cfg.PriorityLevels - 1 // clamp to max
		}
		originalTopic := msg.Topic
		msg.Topic = fmt.Sprintf("%s.p%d", originalTopic, level)
		msg.Headers["x-original-topic"] = originalTopic
		msg.Headers["x-priority-level"] = fmt.Sprintf("%d", level)
	}

	err := p.publishInternal(ctx, msg)
	// Restore original topic for caller
	if orig := msg.Headers["x-original-topic"]; orig != "" {
		msg.Topic = orig
	}
	return err
}

// publishInternal is the original Publish logic.
func (p *NATSProducer) publishInternal(ctx context.Context, msg *Message) error {
	natsMsg := &nats.Msg{
		Subject: msg.Topic,
		Data:    msg.Payload,
		Header:  make(nats.Header),
	}
	for k, v := range msg.Headers {
		natsMsg.Header.Add(k, v)
	}

	// ── Set headers from Message fields ──
	if msg.IdempKey != "" {
		natsMsg.Header.Add("Nats-Idemp-Key", msg.IdempKey)
	}
	if msg.TraceID != "" {
		natsMsg.Header.Add("Nats-Trace-Id", msg.TraceID)
	}
	if msg.TTL > 0 {
		natsMsg.Header.Add("Nats-Msg-TTL", msg.TTL.String())
	}

	if p.js != nil {
		// ── JetStream publish with options ──
		opts := make([]nats.PubOpt, 0)
		if msg.TTL > 0 {
			opts = append(opts, nats.MsgTTL(msg.TTL))
		}

		// ── 幂等去重 ──
		if p.cfg.EnableIdempotency && msg.IdempKey != "" && p.kv != nil {
			// Check if already processed
			_, err := p.kv.Get(msg.IdempKey)
			if err == nil {
				// Key exists — duplicate detected
				slog.Warn("nats: duplicate message, skipping", "idemp_key", msg.IdempKey)
				return nil
			}
		}

		_, err := p.js.PublishMsg(natsMsg, opts...)
		if err != nil {
			return fmt.Errorf("nats jetstream publish: %w", err)
		}

		// Mark as processed in KV
		if p.cfg.EnableIdempotency && msg.IdempKey != "" && p.kv != nil {
			ttl := msg.TTL
			if ttl == 0 {
				ttl = 24 * time.Hour // default TTL
			}
			_, err := p.kv.Put(msg.IdempKey, []byte(time.Now().Format(time.RFC3339)))
			if err != nil {
				slog.Warn("nats: failed to store idempotency key", "idemp_key", msg.IdempKey, "err", err)
			}
		}
	} else {
		// Core NATS publish (fire-and-forget)
		if err := p.conn.PublishMsg(natsMsg); err != nil {
			return fmt.Errorf("nats publish: %w", err)
		}
	}
	return nil
}

// PublishBatch publishes multiple messages.
func (p *NATSProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	for _, m := range msgs {
		if err := p.Publish(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// FixedDelay sends a message with a fixed-delay level.
//
// Implementation for NATS v1 (JetStream):
//   - Publishes to a per-level delay subject (delay.level.N)
//   - Runs a background re-publisher goroutine (StartedRePublisher) that
//     consumes from delay subjects and forwards to the real topic after delay.
//   - Alternatively, use an external nats-bridge service for server-side delay.
//
// Usage:
//
//	// Start the re-publisher once
//	if err := p.StartRePublisher(ctx); err != nil {
//		log.Fatal(err)
//	}
//	defer p.StopRePublisher()
//
//	// Now FixedDelay works
//	if err := p.FixedDelay(ctx, msg, 2); err != nil {
//		log.Fatal(err)
//	}
//
// Example FixedDelayLevels: []int64{60000, 300000, 600000} (1m / 5m / 10m)
func (p *NATSProducer) FixedDelay(ctx context.Context, msg *Message, level int) error {
	levels := p.cfg.FixedDelayLevels
	if len(levels) == 0 {
		levels = []int64{60000, 300000, 600000, 1800000, 3600000}
	}

	if level < 1 || level > len(levels) {
		return fmt.Errorf("nats: invalid fixed-delay level %d (valid: 1-%d)", level, len(levels))
	}

	if p.cfg.EnableJetStream && p.js != nil {
		// JetStream: publish to delay subject; re-publisher goroutine forwards after delay
		delaySubj := fmt.Sprintf("delay.level.%d", level)
		natsMsg := nats.NewMsg(delaySubj)
		natsMsg.Data = msg.Payload
		natsMsg.Header = nats.Header{}
		natsMsg.Header.Set("x-original-topic", msg.Topic)
		if len(msg.Key) > 0 {
			natsMsg.Header.Set("Nats-Msg-Key", string(msg.Key))
		}
		natsMsg.Header.Set("x-delay-level", fmt.Sprintf("%d", level))

		_, err := p.js.PublishAsync(delaySubj, msg.Payload,
			nats.MsgId(fmt.Sprintf("delay-%d-%d", level, time.Now().UnixNano())),
		)
		if err != nil {
			return fmt.Errorf("nats: fixed-delay publish level %d: %w", level, err)
		}
	} else {
		// Core NATS: publish directly with timestamp header (consumer-side delay)
		natsMsg := nats.NewMsg(msg.Topic)
		natsMsg.Data = msg.Payload
		natsMsg.Header = nats.Header{}
		natsMsg.Header.Set("x-delay-level", fmt.Sprintf("%d", level))
		if err := p.conn.PublishMsg(natsMsg); err != nil {
			return fmt.Errorf("nats: fixed-delay publish level %d: %w", level, err)
		}
	}

	slog.Debug("nats: fixed-delay published",
		slog.String("topic", msg.Topic),
		slog.Int("level", level),
		slog.Int64("delay_ms", levels[level-1]),
	)
	return nil
}

// StartRePublisher starts background goroutines that consume from delay
// subjects (delay.level.N) and forward messages to their real topic after
// the configured delay. Call once per producer instance.
//
// IMPORTANT: For JetStream delayed delivery, an external nats-bridge service
// is recommended in production. This in-process re-publisher holds messages
// in-memory and is suitable for development/low-throughput scenarios.
func (p *NATSProducer) StartRePublisher(ctx context.Context) error {
	levels := p.cfg.FixedDelayLevels
	if len(levels) == 0 {
		levels = []int64{60000, 300000, 600000, 1800000, 3600000}
	}

	if !p.cfg.EnableJetStream || p.js == nil {
		return fmt.Errorf("nats: re-publisher requires EnableJetStream=true")
	}

	for i, delayMs := range levels {
		lvl := i + 1
		delay := time.Duration(delayMs) * time.Millisecond
		subj := fmt.Sprintf("delay.level.%d", lvl)

		// Create a channel to receive messages for this delay level
		ch := make(chan *nats.Msg, 256)
		sub, err := p.js.ChanSubscribe(subj, ch,
			nats.ManualAck(),
			nats.Durable(fmt.Sprintf("republisher-%d", lvl)),
		)
		if err != nil {
			return fmt.Errorf("nats: re-publisher subscribe %s: %w", subj, err)
		}
		_ = sub

		// Forwarding goroutine per delay level
		go func(delayLvl int, delayDuration time.Duration, delaySubj string) {
			for {
				select {
				case <-ctx.Done():
					return
				case natsMsg := <-ch:
					origTopic := natsMsg.Header.Get("x-original-topic")
					if origTopic == "" {
						natsMsg.Ack()
						return
					}
					// Wait for delay
					timer := time.NewTimer(delayDuration)
					select {
					case <-timer.C:
						_, err := p.js.PublishAsync(origTopic, natsMsg.Data)
						if err != nil {
							slog.Error("nats: re-publisher: forward failed",
								slog.String("topic", origTopic),
								slog.String("err", err.Error()),
							)
						}
						natsMsg.Ack()
					case <-ctx.Done():
						timer.Stop()
						natsMsg.Nak()
						return
					}
				}
			}
		}(lvl, delay, subj)
	}

	slog.Info("nats: re-publisher started",
		slog.Any("levels", levels),
	)
	return nil
}
// StopRePublisher stops the re-publisher goroutine.
func (p *NATSProducer) StopRePublisher() {
	// Subscription is managed by JetStream; closing context cancels pending sleeps
	slog.Info("nats: re-publisher stopped")
}

// StartArbitraryDelayPublisher starts a goroutine that forwards delayed messages.
// Call once per producer to enable ArbitraryDelay.
func (p *NATSProducer) StartArbitraryDelayPublisher(ctx context.Context) error {
	if !p.cfg.EnableJetStream || p.js == nil {
		return fmt.Errorf("nats: arbitrary-delay publisher requires EnableJetStream=true")
	}

	sub, err := p.js.Subscribe("delay.arbitrary", func(msg *nats.Msg) {
		deliverAtStr := msg.Header.Get("x-deliver-at")
		if deliverAtStr == "" {
			msg.Ack()
			return
		}

		var deliverAt int64
		fmt.Sscanf(deliverAtStr, "%d", &deliverAt)

		now := time.Now().UnixMilli()
		waitMs := deliverAt - now
		if waitMs <= 0 {
			// Already due, forward immediately
			p.forwardDelayed(msg)
			return
		}

		// Wait until delivery time
		go func() {
			timer := time.NewTimer(time.Duration(waitMs) * time.Millisecond)
			defer timer.Stop()

			select {
			case <-timer.C:
				p.forwardDelayed(msg)
			case <-ctx.Done():
				msg.Nak()
			}
		}()
	}, nats.ManualAck(), nats.Durable("arb-delay-publisher"))

	if err != nil {
		return fmt.Errorf("nats: subscribe delay.arbitrary: %w", err)
	}
	_ = sub

	slog.Info("nats: arbitrary-delay publisher started")
	return nil
}

func (p *NATSProducer) forwardDelayed(natsMsg *nats.Msg) {
	origTopic := natsMsg.Header.Get("x-original-topic")
	if origTopic == "" {
		natsMsg.Ack()
		return
	}

	_, err := p.js.PublishAsync(origTopic, natsMsg.Data)
	if err != nil {
		slog.Error("nats: forward delayed failed", "topic", origTopic, "err", err)
		// Don't ack, let it retry
		return
	}

	slog.Debug("nats: forwarded delayed message", "topic", origTopic)
	natsMsg.Ack()
}

// Close closes the NATS connection.
func (p *NATSProducer) Close() error {
	if p.conn != nil {
		p.conn.Close()
	}
	return nil
}

// Conn returns the underlying *nats.Conn for advanced usage.
// Use with caution — prefer NATSProducer methods when possible.
func (p *NATSProducer) Conn() *nats.Conn {
	return p.conn
}

// JetStream returns the JetStream context if enabled, nil otherwise.
func (p *NATSProducer) JetStream() nats.JetStreamContext {
	return p.js
}

// Compile-time assertion.
var _ Producer = (*NATSProducer)(nil)

// ─── Consumer ─────────────────────────────────────────────────────────────────

// NATSConsumerConfig configures a NATS consumer.
//
// EnablePriority (when true) makes the consumer drain high-priority subjects first
// using a priority heap. Requires Producer to route messages to topic.p0/p1/...pN.
type NATSConsumerConfig struct {
	NATSConfig

	// QueueGroup is the NATS queue group name for load balancing.
	// Messages are distributed among consumers in the same queue group.
	QueueGroup string

	// MaxPending is the maximum number of pending messages in the subscription.
	// Default: 65536.
	MaxPending int

	// EnableOrdered enables ordered delivery via JetStream OrderedConsumer.
	// Only applies when EnableJetStream is true.
	EnableOrdered bool

	// EnablePriority enables priority-based consumption.
	// When true, consumer subscribes to topic.p0...topic.pN (N = PriorityLevels-1)
	// and processes higher-priority messages first using an internal heap.
	// Producer must use matching PriorityLevels.
	EnablePriority bool
}


func (c *NATSConsumerConfig) setDefaults() {
	c.NATSConfig.setDefaults()
	if c.MaxPending == 0 {
		c.MaxPending = 65536
	}
	if c.PriorityLevels == 0 {
		c.PriorityLevels = c.NATSConfig.PriorityLevels
	}
	if c.PriorityLevels == 0 {
		c.PriorityLevels = 3
	}
}

// NATSConsumer subscribes to NATS subjects.
type NATSConsumer struct {
	cfg NATSConsumerConfig
}

// NewNATSConsumer creates a NATS consumer. Connection is established lazily in Subscribe.
func NewNATSConsumer(cfg NATSConsumerConfig) (*NATSConsumer, error) {
	cfg.setDefaults()
	return &NATSConsumer{cfg: cfg}, nil
}

// Subscribe connects to NATS and starts consuming messages.
// It blocks until ctx is cancelled.
//
// When EnablePriority is true, consumer drains topic.p0..pN using a priority heap.
// The group parameter overrides cfg.QueueGroup when non-empty.
func (c *NATSConsumer) Subscribe(ctx context.Context, topics []string, group string, handler Handler) error {
	if len(topics) == 0 {
		return fmt.Errorf("nats consumer: at least one subject is required")
	}

	conn, err := nats.Connect(c.cfg.URL, c.cfg.NATSConfig.buildOptions()...)
	if err != nil {
		return fmt.Errorf("nats consumer: connect: %w", err)
	}
	defer conn.Close()

	queueGroup := c.cfg.QueueGroup
	if group != "" {
		queueGroup = group
	}

	if c.cfg.EnablePriority {
		return c.subscribePriority(ctx, conn, topics, queueGroup, handler)
	}

	if c.cfg.EnableJetStream {
		return c.subscribeJetStream(ctx, conn, topics, queueGroup, handler)
	}
	return c.subscribeCore(ctx, conn, topics, queueGroup, handler)
}


// subscribeCore uses Core NATS subscriptions (at-most-once, no persistence).
func (c *NATSConsumer) subscribeCore(ctx context.Context, conn *nats.Conn, topics []string, queueGroup string, handler Handler) error {
	msgChan := make(chan *nats.Msg, c.cfg.MaxPending)

	subs := make([]*nats.Subscription, 0, len(topics))
	for _, subject := range topics {
		var sub *nats.Subscription
		var err error

		if queueGroup != "" {
			sub, err = conn.ChanQueueSubscribe(subject, queueGroup, msgChan)
		} else {
			sub, err = conn.ChanSubscribe(subject, msgChan)
		}
		if err != nil {
			for _, s := range subs {
				s.Unsubscribe()
			}
			return fmt.Errorf("nats consumer: subscribe to %s: %w", subject, err)
		}
		if err := sub.SetPendingLimits(c.cfg.MaxPending, -1); err != nil {
			slog.Warn("nats consumer: set pending limits",
				slog.String("subject", subject),
				slog.String("err", err.Error()),
			)
		}
		subs = append(subs, sub)
	}

	slog.Info("nats consumer: subscribed (core)",
		slog.String("url", c.cfg.URL),
		slog.Any("subjects", topics),
		slog.String("queue_group", queueGroup),
	)

	for {
		select {
		case <-ctx.Done():
			for _, sub := range subs {
				sub.Unsubscribe()
			}
			return ctx.Err()
		case natsMsg, ok := <-msgChan:
			if !ok {
				return fmt.Errorf("nats consumer: message channel closed")
			}
			msg := natsMessageToMessage(natsMsg)
			if err := handler(ctx, msg); err != nil {
				slog.Warn("nats handler error",
					slog.String("subject", natsMsg.Subject),
					slog.String("err", err.Error()),
				)
			}
		}
	}
}

// subscribeJetStream uses JetStream (persistent, at-least-once, with retry/DLQ/ordered).
func (c *NATSConsumer) subscribeJetStream(ctx context.Context, conn *nats.Conn, topics []string, queueGroup string, handler Handler) error {
	js, err := conn.JetStream()
	if err != nil {
		return fmt.Errorf("nats consumer: jetstream: %w", err)
	}

	slog.Info("nats consumer: subscribed (jetstream)",
		slog.String("url", c.cfg.URL),
		slog.Any("subjects", topics),
		slog.String("queue_group", queueGroup),
		slog.Int("max_deliver", c.cfg.MaxDeliver),
		slog.String("dlq", c.cfg.DLQSubject),
		slog.Bool("ordered", c.cfg.EnableOrdered),
	)

	// Subscribe to each topic
	for _, subject := range topics {
		subOpts := make([]nats.SubOpt, 0)

		// JetStream consumer configuration via SubOpt functions
		if c.cfg.MaxDeliver > 0 {
			subOpts = append(subOpts, nats.MaxDeliver(c.cfg.MaxDeliver))
		}
		if c.cfg.DLQSubject != "" {
			subOpts = append(subOpts, nats.DeliverSubject(c.cfg.DLQSubject))
		}
		if c.cfg.EnableOrdered {
			subOpts = append(subOpts, nats.OrderedConsumer())
		}
		subOpts = append(subOpts,
			nats.MaxAckPending(c.cfg.MaxPending),
			nats.ReplayInstant(),
		)

		msgChan := make(chan *nats.Msg, c.cfg.MaxPending)

		sub, err := js.ChanSubscribe(subject, msgChan, subOpts...)
		if err != nil {
			return fmt.Errorf("nats consumer: jetstream subscribe to %s: %w", subject, err)
		}
		defer sub.Unsubscribe()

		// Process messages in a goroutine
		go func(subject string) {
			for {
				select {
				case <-ctx.Done():
					return
				case natsMsg, ok := <-msgChan:
					if !ok {
						return
					}

					msg := natsMessageToMessage(natsMsg)
					handlerErr := handler(ctx, msg)

					if handlerErr != nil {
						switch {
						case IsPermanent(handlerErr):
							// Permanent → ACK (message in DLQ via MaxDeliver)
							natsMsg.Ack()
							slog.Error("nats: permanent error, acked", "subject", subject, "err", handlerErr)
						case IsRetry(handlerErr):
							// Retry → NAK (JetStream will redeliver based on MaxDeliver)
							natsMsg.Nak()
							slog.Warn("nats: retry error, nak", "subject", subject, "err", handlerErr)
						case IsSkip(handlerErr):
							// Skip → ACK (do not retry or DLQ)
							natsMsg.Ack()
							slog.Warn("nats: skip, acked", "subject", subject, "err", handlerErr)
						default:
							// Unknown → NAK
							natsMsg.Nak()
							slog.Warn("nats: handler error, nak", "subject", subject, "err", handlerErr)
						}
					} else {
						// Success → ACK
						natsMsg.Ack()
					}
				}
			}
		}(subject)
	}

	<-ctx.Done()
	return ctx.Err()
}

// subscribePriority uses a priority heap to process high-priority messages first.
// Subscribes to topic.p0...topic.p{N-1} and drains p0 before p1, etc.
func (c *NATSConsumer) subscribePriority(ctx context.Context, conn *nats.Conn, topics []string, queueGroup string, handler Handler) error {
	js, err := conn.JetStream()
	if err != nil {
		return fmt.Errorf("nats consumer: jetstream for priority: %w", err)
	}

	levels := c.cfg.PriorityLevels
	if levels == 0 {
		levels = 3
	}

	// Priority queue: lower level = higher priority
	pq := newPriorityQueue(levels)

	// Subscribe to all priority levels for each topic
	subs := make([]*nats.Subscription, 0)
	for _, topic := range topics {
		for level := 0; level < levels; level++ {
			subj := fmt.Sprintf("%s.p%d", topic, level)
			ch := make(chan *nats.Msg, 64)

			var sub *nats.Subscription
			var err error
			if queueGroup != "" {
				sub, err = js.ChanQueueSubscribe(subj, queueGroup, ch, nats.ManualAck())
			} else {
				sub, err = js.ChanSubscribe(subj, ch, nats.ManualAck())
			}
			if err != nil {
				for _, s := range subs {
					s.Unsubscribe()
				}
				return fmt.Errorf("nats consumer: priority subscribe %s: %w", subj, err)
			}
			subs = append(subs, sub)

			// Feeder goroutine: push messages into priority queue
			go func(level int, ch <-chan *nats.Msg) {
				for {
					select {
					case <-ctx.Done():
						return
					case msg, ok := <-ch:
						if !ok {
							return
						}
						pq.Push(level, msg)
					}
				}
			}(level, ch)
		}
	}

	slog.Info("nats consumer: subscribed (priority)",
		slog.Any("topics", topics),
		slog.Int("levels", levels),
	)

	// Processor: drain high priority first
	for {
		select {
		case <-ctx.Done():
			for _, s := range subs {
				s.Unsubscribe()
			}
			return ctx.Err()
		default:
			if msg := pq.Pop(); msg != nil {
				natsMsg := msg.(*nats.Msg)
				origTopic := natsMsg.Header.Get("x-original-topic")
				if origTopic == "" {
					origTopic = natsMsg.Subject
				}
				mqMsg := &Message{
					Topic:   origTopic,
					Payload: natsMsg.Data,
				}
				if err := handler(ctx, mqMsg); err != nil {
					slog.Warn("nats: priority handler error", "topic", origTopic, "err", err)
				}
				natsMsg.Ack()
			}
		}
	}
}

// priorityQueue implements a simple multi-level priority queue.
// level 0 = highest priority.
type priorityQueue struct {
	queues [][]any
}

func newPriorityQueue(levels int) *priorityQueue {
	q := make([][]any, levels)
	for i := range q {
		q[i] = make([]any, 0, 64)
	}
	return &priorityQueue{queues: q}
}

func (pq *priorityQueue) Push(level int, msg any) {
	if level >= len(pq.queues) {
		level = len(pq.queues) - 1
	}
	pq.queues[level] = append(pq.queues[level], msg)
}

func (pq *priorityQueue) Pop() any {
	for i := range pq.queues {
		if len(pq.queues[i]) > 0 {
			msg := pq.queues[i][0]
			pq.queues[i] = pq.queues[i][1:]
			return msg
		}
	}
	return nil
}

// Close is a no-op; connection is managed per-Subscribe call.
func (c *NATSConsumer) Close() error {
	return nil
}

// Compile-time assertion.
var _ Consumer = (*NATSConsumer)(nil)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func natsMessageToMessage(natsMsg *nats.Msg) *Message {
	headers := make(map[string]string)
	if natsMsg.Header != nil {
		for k, vals := range natsMsg.Header {
			if len(vals) > 0 {
				headers[k] = vals[0]
			}
		}
	}

	// Extract idempotency key from header if present
	idempKey := headers["Nats-Idemp-Key"]
	delete(headers, "Nats-Idemp-Key")

	return &Message{
		Topic:    natsMsg.Subject,
		Payload:  natsMsg.Data,
		Headers:  headers,
		IdempKey: idempKey,
		Meta: map[string]any{
			"reply": natsMsg.Reply,
		},
	}
}

// Capabilities returns the capabilities of NATS JetStream.
func (p *NATSProducer) BeginTransaction(ctx context.Context, _ TransactionChecker) (Transaction, error) {
	return nil, ErrCapTxNotSupported
}

func (p *NATSProducer) Capabilities() Capabilities { return NatsCapabilities() }
func (c *NATSConsumer) Capabilities() Capabilities { return NatsCapabilities() }
