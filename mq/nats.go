// Package nats provides a NATS implementation of mq.Producer and mq.Consumer.
//
// NATS is a lightweight, high-performance messaging system. It supports both
// Core NATS (at-most-once delivery) and JetStream (persistent, at-least-once).
//
// # Core NATS Producer
//
//	p, err := nats.NewProducer(nats.Config{URL: "nats://localhost:4222"})
//	defer p.Close()
//	p.Publish(ctx, &Message{Topic: "orders.created", Payload: body})
//
// # Core NATS Consumer
//
//	c, err := nats.NewConsumer(nats.ConsumerConfig{
//	    Config: nats.Config{URL: "nats://localhost:4222"},
//	})
//	c.Subscribe(ctx, []string{"orders.*"}, "order-service", handler)
//
// # JetStream (optional)
//
// To use JetStream for guaranteed delivery, set EnableJetStream: true in the config.
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
	}

	return p, nil
}

// Publish sends a message to NATS.
func (p *NATSProducer) Publish(ctx context.Context, msg *Message) error {
	natsMsg := &nats.Msg{
		Subject: msg.Topic,
		Data:    msg.Payload,
	}
	for k, v := range msg.Headers {
		natsMsg.Header.Add(k, v)
	}

	if p.js != nil {
		// JetStream publish with acknowledgement
		_, err := p.js.PublishMsg(natsMsg, nats.Context(ctx))
		if err != nil {
			return fmt.Errorf("nats jetstream publish: %w", err)
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
type NATSConsumerConfig struct {
	NATSConfig

	// QueueGroup is the NATS queue group name for load balancing.
	// Messages are distributed among consumers in the same queue group.
	QueueGroup string

	// MaxPending is the maximum number of pending messages in the subscription.
	// Default: 65536.
	MaxPending int
}

func (c *NATSConsumerConfig) setDefaults() {
	c.NATSConfig.setDefaults()
	if c.MaxPending == 0 {
		c.MaxPending = 65536
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

	// Create a channel to receive messages from all subscriptions
	msgChan := make(chan *nats.Msg, c.cfg.MaxPending)

	// Subscribe to all subjects
	subs := make([]*nats.Subscription, 0, len(topics))
	for _, subject := range topics {
		var sub *nats.Subscription
		var err error

		if queueGroup != "" {
			// Queue subscription (load-balanced)
			sub, err = conn.ChanQueueSubscribe(subject, queueGroup, msgChan)
		} else {
			// Regular subscription
			sub, err = conn.ChanSubscribe(subject, msgChan)
		}

		if err != nil {
			// Clean up already created subscriptions
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

	slog.Info("nats consumer: subscribed",
		slog.String("url", c.cfg.URL),
		slog.Any("subjects", topics),
		slog.String("queue_group", queueGroup),
	)

	// Process messages until context is cancelled
	for {
		select {
		case <-ctx.Done():
			// Unsubscribe from all subjects
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
				// For NATS, there's no explicit NACK in Core NATS.
				// JetStream would handle this differently.
			}
		}
	}
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

	return &Message{
		Topic:   natsMsg.Subject,
		Payload: natsMsg.Data,
		Headers: headers,
		Meta: map[string]any{
			"reply": natsMsg.Reply,
		},
	}
}
