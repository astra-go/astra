// Package rocketmq provides a RocketMQ 5.x implementation of mq.Producer and
// mq.Consumer using the official Apache RocketMQ gRPC-based pure-Go client.
//
// # Producer
//
//	p, err := rocketmq.NewProducer(rocketmq.Config{
//	    Endpoint:  "localhost:8081",
//	    Topic:     "orders",
//	    AccessKey: "ak", SecretKey: "sk",
//	})
//	defer p.Close()
//	p.Publish(ctx, &Message{Topic: "orders", Payload: body})
//
// # Consumer
//
//	c, err := rocketmq.NewConsumer(rocketmq.ConsumerConfig{
//	    Endpoint:      "localhost:8081",
//	    ConsumerGroup: "order-service",
//	    AccessKey: "ak", SecretKey: "sk",
//	})
//	c.Subscribe(ctx, []string{"orders"}, "order-service", handler)
//
// # TLS / plain
//
// TLS is disabled by default (development convenience).
// Set EnableSSL = true in the Config to enable TLS.
package mq

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	rmq "github.com/apache/rocketmq-clients/golang/v5"
	"github.com/apache/rocketmq-clients/golang/v5/credentials"
)

// Config configures a RocketMQ producer or consumer.
type RocketMQConfig struct {
	// Endpoint is the name-server / proxy address, e.g. "localhost:8081".
	Endpoint string

	// Topic is the default topic for the producer (required by RocketMQ 5.x
	// to pre-fetch routing before Start).
	Topic string

	// ConsumerGroup is used by the consumer; ignored for producers.
	ConsumerGroup string

	// AccessKey and SecretKey for ACL authentication.
	// Leave empty if the broker has ACL disabled.
	AccessKey string
	SecretKey string

	// NameSpace isolates topics in a multi-tenant broker deployment.
	NameSpace string

	// EnableSSL enables TLS. Default: false (plain-text).
	EnableSSL bool

	// MaxAttempts is the number of send retries for the producer. Default: 3.
	MaxAttempts int32

	// ReceiveBatchSize is the number of messages per Receive call. Default: 16.
	ReceiveBatchSize int32

	// InvisibleDuration is the visibility timeout for received messages.
	// Must be > 20 s according to RocketMQ requirements. Default: 30 s.
	InvisibleDuration time.Duration
}

func (c *RocketMQConfig) setDefaults() {
	if c.MaxAttempts == 0 {
		c.MaxAttempts = 3
	}
	if c.ReceiveBatchSize == 0 {
		c.ReceiveBatchSize = 16
	}
	if c.InvisibleDuration == 0 {
		c.InvisibleDuration = 30 * time.Second
	}
}

func (c *RocketMQConfig) rmqConfig() *rmq.Config {
	cred := &credentials.SessionCredentials{
		AccessKey:    c.AccessKey,
		AccessSecret: c.SecretKey,
	}
	return &rmq.Config{
		Endpoint:      c.Endpoint,
		NameSpace:     c.NameSpace,
		ConsumerGroup: c.ConsumerGroup,
		Credentials:   cred,
	}
}

// ─── Producer ─────────────────────────────────────────────────────────────────

// Producer publishes messages to a RocketMQ topic.
type RocketMQProducer struct {
	cfg  RocketMQConfig
	prod rmq.Producer
}

// NewProducer creates and starts a RocketMQ producer.
func NewRocketMQProducer(cfg RocketMQConfig) (*RocketMQProducer, error) {
	cfg.setDefaults()

	rmq.EnableSsl = cfg.EnableSSL

	prod, err := rmq.NewProducer(
		cfg.rmqConfig(),
		rmq.WithTopics(cfg.Topic),
		rmq.WithMaxAttempts(cfg.MaxAttempts),
	)
	if err != nil {
		return nil, fmt.Errorf("rocketmq producer: create: %w", err)
	}
	if err := prod.Start(); err != nil {
		return nil, fmt.Errorf("rocketmq producer: start: %w", err)
	}
	return &RocketMQProducer{cfg: cfg, prod: prod}, nil
}

// Publish sends a single message synchronously.
func (p *RocketMQProducer) Publish(ctx context.Context, msg *Message) error {
	rmqMsg := toRMQMessage(msg)
	receipts, err := p.prod.Send(ctx, rmqMsg)
	if err != nil {
		return fmt.Errorf("rocketmq publish: %w", err)
	}
	slog.Debug("rocketmq publish ok",
		slog.String("topic", msg.Topic),
		slog.Int("receipts", len(receipts)),
	)
	return nil
}

// PublishBatch sends multiple messages sequentially.
func (p *RocketMQProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	for _, m := range msgs {
		if err := p.Publish(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// Close stops the underlying RocketMQ producer gracefully.
func (p *RocketMQProducer) Close() error {
	return p.prod.GracefulStop()
}

// ─── Consumer ─────────────────────────────────────────────────────────────────

// Consumer receives messages from RocketMQ topics using the SimpleConsumer API.
type RocketMQConsumer struct {
	cfg RocketMQConsumerConfig
}

// ConsumerConfig configures a RocketMQ consumer.
type RocketMQConsumerConfig = RocketMQConfig

// NewConsumer creates a RocketMQ consumer. The connection is established lazily
// inside Subscribe.
func NewRocketMQConsumer(cfg RocketMQConsumerConfig) (*RocketMQConsumer, error) {
	cfg.setDefaults()
	return &RocketMQConsumer{cfg: cfg}, nil
}

// Subscribe starts consuming from topics and calls handler for each message.
// It blocks until ctx is cancelled.
//
// The group parameter overrides cfg.ConsumerGroup when non-empty.
func (c *RocketMQConsumer) Subscribe(ctx context.Context, topics []string, group string, handler Handler) error {
	if len(topics) == 0 {
		return fmt.Errorf("rocketmq consumer: at least one topic is required")
	}
	if group != "" {
		c.cfg.ConsumerGroup = group
	}

	rmq.EnableSsl = c.cfg.EnableSSL

	subExpressions := make(map[string]*rmq.FilterExpression, len(topics))
	for _, t := range topics {
		subExpressions[t] = rmq.SUB_ALL
	}

	sc, err := rmq.NewSimpleConsumer(
		c.cfg.rmqConfig(),
		rmq.WithSimpleAwaitDuration(5*time.Second),
		rmq.WithSimpleSubscriptionExpressions(subExpressions),
	)
	if err != nil {
		return fmt.Errorf("rocketmq consumer: create: %w", err)
	}
	if err := sc.Start(); err != nil {
		return fmt.Errorf("rocketmq consumer: start: %w", err)
	}
	defer sc.GracefulStop()

	slog.Info("rocketmq consumer: started",
		slog.String("endpoint", c.cfg.Endpoint),
		slog.Any("topics", topics),
		slog.String("group", c.cfg.ConsumerGroup),
	)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		views, err := sc.Receive(ctx, c.cfg.ReceiveBatchSize, c.cfg.InvisibleDuration)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Transient errors (no messages, timeout) are normal in long-poll.
			slog.Debug("rocketmq receive", slog.String("err", err.Error()))
			continue
		}

		for _, view := range views {
			msg := fromMessageView(view)
			if err := handler(ctx, msg); err != nil {
				slog.Warn("rocketmq handler error",
					slog.String("topic", view.GetTopic()),
					slog.String("err", err.Error()),
				)
				// Do not ack — the message will become visible again after
				// InvisibleDuration.
				continue
			}
			if ackErr := sc.Ack(ctx, view); ackErr != nil {
				slog.Warn("rocketmq ack error",
					slog.String("topic", view.GetTopic()),
					slog.String("err", ackErr.Error()),
				)
			}
		}
	}
}

// Close is a no-op; the consumer is started and stopped inside Subscribe.
func (c *RocketMQConsumer) Close() error { return nil }

// ─── helpers ──────────────────────────────────────────────────────────────────

func toRMQMessage(msg *Message) *rmq.Message {
	m := &rmq.Message{
		Topic: msg.Topic,
		Body:  msg.Payload,
	}
	if len(msg.Key) > 0 {
		m.SetKeys(string(msg.Key))
	}
	for k, v := range msg.Headers {
		m.AddProperty(k, v)
	}
	return m
}

func fromMessageView(view *rmq.MessageView) *Message {
	headers := make(map[string]string)
	for k, v := range view.GetProperties() {
		headers[k] = v
	}
	return &Message{
		Topic:   view.GetTopic(),
		Key:     []byte(view.GetMessageId()),
		Payload: view.GetBody(),
		Headers: headers,
		Meta: map[string]any{
			"message_id":     view.GetMessageId(),
			"receipt_handle": view.GetReceiptHandle(),
			"tag":            view.GetTag(),
			"born_time":      view.GetBornTimestamp(),
			"delivery_count": view.GetDeliveryAttempt(),
		},
	}
}
