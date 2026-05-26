// Package nats provides NATS implementations of the mq.Producer and
// mq.Consumer interfaces.
//
// Both Core NATS and JetStream are supported. Core NATS offers at-most-once
// delivery. JetStream adds persistence, at-least-once delivery, and durable
// consumers.
//
// # Core NATS producer
//
//	p, _ := nats.NewProducer(nats.Config{URL: "nats://localhost:4222"})
//	defer p.Close()
//	p.Publish(ctx, &mq.Message{Topic: "orders.created", Payload: body})
//
// # JetStream producer
//
//	p, _ := nats.NewProducer(nats.Config{
//	    URL:       "nats://localhost:4222",
//	    JetStream: true,
//	})
//	p.Publish(ctx, &mq.Message{Topic: "orders.created", Payload: body})
//
// # Core NATS consumer (queue group)
//
//	c, _ := nats.NewConsumer(nats.ConsumerConfig{
//	    Config:     nats.Config{URL: "nats://localhost:4222"},
//	    QueueGroup: "order-workers",
//	})
//	c.Subscribe(ctx, []string{"orders.created"}, "order-workers", handler)
//
// # JetStream consumer (durable push)
//
//	c, _ := nats.NewConsumer(nats.ConsumerConfig{
//	    Config:     nats.Config{URL: "nats://localhost:4222", JetStream: true},
//	    Stream:     "ORDERS",
//	    Durable:    "order-processor",
//	    MaxInFlight: 10,
//	})
//	c.Subscribe(ctx, []string{"orders.>"}, "", handler)
package nats

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/astra-go/astra/mq"
)

// Config configures a NATS connection.
type Config struct {
	// URL is the NATS server URL. Default: nats.DefaultURL ("nats://127.0.0.1:4222").
	URL string

	// JetStream enables the JetStream API for persistent messaging.
	// When false (default), Core NATS is used.
	JetStream bool

	// Username and Password for NATS credentials.
	Username string
	Password string

	// Token is the NATS authentication token (alternative to user/password).
	Token string

	// CredsFile is the path to a NATS credentials file (.creds).
	CredsFile string

	// ConnectTimeout overrides the default connection timeout. Default: 5s.
	ConnectTimeout time.Duration

	// MaxReconnects is the maximum number of reconnect attempts. Default: 60.
	MaxReconnects int

	// ReconnectWait is the wait time between reconnect attempts. Default: 2s.
	ReconnectWait time.Duration
}

func (c *Config) opts() []nats.Option {
	opts := []nats.Option{
		nats.Name("astra"),
	}
	if c.ConnectTimeout > 0 {
		opts = append(opts, nats.Timeout(c.ConnectTimeout))
	}
	maxReconn := c.MaxReconnects
	if maxReconn == 0 {
		maxReconn = 60
	}
	opts = append(opts, nats.MaxReconnects(maxReconn))
	reconWait := c.ReconnectWait
	if reconWait == 0 {
		reconWait = 2 * time.Second
	}
	opts = append(opts, nats.ReconnectWait(reconWait))

	if c.Username != "" {
		opts = append(opts, nats.UserInfo(c.Username, c.Password))
	} else if c.Token != "" {
		opts = append(opts, nats.Token(c.Token))
	} else if c.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(c.CredsFile))
	}
	return opts
}

// ─── Producer ─────────────────────────────────────────────────────────────────

// Producer publishes messages to NATS (Core or JetStream).
type Producer struct {
	conn *nats.Conn
	js   nats.JetStreamContext
	cfg  Config
}

// NewProducer creates a new NATS Producer and connects to the server.
func NewProducer(cfg Config) (*Producer, error) {
	url := cfg.URL
	if url == "" {
		url = nats.DefaultURL
	}
	conn, err := nats.Connect(url, cfg.opts()...)
	if err != nil {
		return nil, fmt.Errorf("nats: connect %s: %w", url, err)
	}

	p := &Producer{conn: conn, cfg: cfg}
	if cfg.JetStream {
		js, err := conn.JetStream()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("nats: jetstream context: %w", err)
		}
		p.js = js
	}
	return p, nil
}

// Publish sends a single message. The message Topic is used as the NATS subject.
// Message Headers are sent as NATS headers (requires NATS ≥ 2.2).
func (p *Producer) Publish(ctx context.Context, msg *mq.Message) error {
	nm := nats.NewMsg(msg.Topic)
	nm.Data = msg.Payload
	for k, v := range msg.Headers {
		nm.Header.Set(k, v)
	}

	if p.js != nil {
		_, err := p.js.PublishMsg(nm)
		if err != nil {
			return fmt.Errorf("nats: js publish %s: %w", msg.Topic, err)
		}
		return nil
	}
	if err := p.conn.PublishMsg(nm); err != nil {
		return fmt.Errorf("nats: publish %s: %w", msg.Topic, err)
	}
	return nil
}

// PublishBatch sends multiple messages. Each message is published individually.
func (p *Producer) PublishBatch(ctx context.Context, msgs []*mq.Message) error {
	for _, msg := range msgs {
		if err := p.Publish(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// Close drains and closes the NATS connection.
func (p *Producer) Close() error {
	return p.conn.Drain()
}

// Compile-time assertion.
var _ mq.Producer = (*Producer)(nil)

// ─── Consumer ─────────────────────────────────────────────────────────────────

// ConsumerConfig configures a NATS consumer.
type ConsumerConfig struct {
	Config

	// QueueGroup is the NATS queue group for load-balanced Core NATS consumers.
	// When set, only one consumer in the group receives each message.
	QueueGroup string

	// Stream is the JetStream stream name. Required for JetStream consumers.
	// The stream must already exist on the server.
	Stream string

	// Durable is the JetStream durable consumer name.
	// Durable consumers survive consumer restarts and resume from where they left off.
	Durable string

	// MaxInFlight limits the number of in-flight (unacknowledged) messages.
	// Default: 64.
	MaxInFlight int
}

// Consumer subscribes to NATS subjects (Core or JetStream).
type Consumer struct {
	conn *nats.Conn
	js   nats.JetStreamContext
	cfg  ConsumerConfig
}

// NewConsumer creates a new NATS Consumer and connects to the server.
func NewConsumer(cfg ConsumerConfig) (*Consumer, error) {
	url := cfg.URL
	if url == "" {
		url = nats.DefaultURL
	}
	conn, err := nats.Connect(url, cfg.Config.opts()...)
	if err != nil {
		return nil, fmt.Errorf("nats: connect %s: %w", url, err)
	}

	c := &Consumer{conn: conn, cfg: cfg}
	if cfg.JetStream {
		js, err := conn.JetStream()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("nats: jetstream context: %w", err)
		}
		c.js = js
	}
	return c, nil
}

// Subscribe starts consuming from topics. It blocks until ctx is cancelled.
// The topics slice is used as the NATS subjects to subscribe to.
// group is used as the queue group for Core NATS (overrides ConsumerConfig.QueueGroup
// when non-empty).
//
// For JetStream consumers, the Stream and Durable fields in ConsumerConfig
// are used; topics[0] is the filter subject.
func (c *Consumer) Subscribe(ctx context.Context, topics []string, group string, handler mq.Handler) error {
	if len(topics) == 0 {
		return fmt.Errorf("nats: at least one topic is required")
	}

	qg := c.cfg.QueueGroup
	if group != "" {
		qg = group
	}
	maxInFlight := c.cfg.MaxInFlight
	if maxInFlight <= 0 {
		maxInFlight = 64
	}

	var subs []*nats.Subscription

	if c.js != nil {
		// JetStream push consumer.
		sub, err := c.subscribeJetStream(topics, qg, maxInFlight, handler)
		if err != nil {
			return err
		}
		subs = append(subs, sub)
	} else {
		// Core NATS — subscribe to each subject.
		for _, topic := range topics {
			sub, err := c.subscribeCore(topic, qg, handler)
			if err != nil {
				return err
			}
			subs = append(subs, sub)
		}
	}

	<-ctx.Done()
	for _, sub := range subs {
		if err := sub.Drain(); err != nil {
			slog.Warn("nats: drain subscription", slog.String("err", err.Error()))
		}
	}
	return nil
}

func (c *Consumer) subscribeCore(subject, qg string, handler mq.Handler) (*nats.Subscription, error) {
	cb := natsHandler(handler)
	if qg != "" {
		sub, err := c.conn.QueueSubscribe(subject, qg, cb)
		if err != nil {
			return nil, fmt.Errorf("nats: queue subscribe %s/%s: %w", subject, qg, err)
		}
		return sub, nil
	}
	sub, err := c.conn.Subscribe(subject, cb)
	if err != nil {
		return nil, fmt.Errorf("nats: subscribe %s: %w", subject, err)
	}
	return sub, nil
}

func (c *Consumer) subscribeJetStream(topics []string, qg string, maxInFlight int, handler mq.Handler) (*nats.Subscription, error) {
	subject := topics[0] // JetStream single subscription with filter subject

	var opts []nats.SubOpt
	opts = append(opts, nats.MaxDeliver(3))
	opts = append(opts, nats.MaxAckPending(maxInFlight))
	if c.cfg.Durable != "" {
		opts = append(opts, nats.Durable(c.cfg.Durable))
	}
	if c.cfg.Stream != "" {
		opts = append(opts, nats.BindStream(c.cfg.Stream))
	}

	cb := func(nm *nats.Msg) {
		msg := natsMsgToMQ(nm)
		if err := handler(context.Background(), msg); err != nil {
			slog.Warn("nats: handler error", slog.String("subject", nm.Subject), slog.String("err", err.Error()))
			nm.Nak()
			return
		}
		nm.Ack()
	}

	var sub *nats.Subscription
	var err error
	if qg != "" {
		sub, err = c.js.QueueSubscribe(subject, qg, cb, opts...)
	} else {
		sub, err = c.js.Subscribe(subject, cb, opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("nats: js subscribe %s: %w", subject, err)
	}
	return sub, nil
}

// Close drains and closes the NATS connection.
func (c *Consumer) Close() error {
	return c.conn.Drain()
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func natsHandler(handler mq.Handler) nats.MsgHandler {
	return func(nm *nats.Msg) {
		msg := natsMsgToMQ(nm)
		if err := handler(context.Background(), msg); err != nil {
			slog.Warn("nats: handler error",
				slog.String("subject", nm.Subject),
				slog.String("err", err.Error()))
		}
	}
}

func natsMsgToMQ(nm *nats.Msg) *mq.Message {
	headers := make(map[string]string, len(nm.Header))
	for k := range nm.Header {
		headers[k] = nm.Header.Get(k)
	}
	return &mq.Message{
		Topic:   nm.Subject,
		Payload: nm.Data,
		Headers: headers,
	}
}

// Compile-time assertion.
var _ mq.Consumer = (*Consumer)(nil)
