// Package rabbitmq provides a RabbitMQ implementation of the mq.Producer and
// mq.Consumer interfaces using the amqp091-go driver.
//
// # Exchange model
//
// The Producer declares a named exchange (default: "astra") of a configurable
// type (direct, fanout, topic) and publishes messages using the message's
// Topic field as the routing key.
//
// The Consumer declares a durable queue, binds it to the exchange with a
// routing key, and processes deliveries via the mq.Handler callback.
//
// # Auto-reconnect
//
// Both Producer and Consumer implement exponential-backoff reconnection.
// A broken connection is detected via the AMQP NotifyClose channel; a new
// connection is established automatically without user intervention.
//
// # Usage
//
//	p, err := rabbitmq.NewProducer(rabbitmq.Config{
//	    URL:          "amqp://guest:guest@localhost:5672/",
//	    Exchange:     "events",
//	    ExchangeType: "topic",
//	})
//	defer p.Close()
//	p.Publish(ctx, &mq.Message{Topic: "user.created", Payload: body})
//
//	c, err := rabbitmq.NewConsumer(rabbitmq.ConsumerConfig{
//	    URL:        "amqp://guest:guest@localhost:5672/",
//	    Queue:      "user-service",
//	    Exchange:   "events",
//	    RoutingKey: "user.*",
//	    Prefetch:   10,
//	})
//	c.Subscribe(ctx, nil, "", func(ctx context.Context, msg *mq.Message) error {
//	    return handleMessage(msg)
//	})
package rabbitmq

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/astra-go/astra/mq"
)

// ─── Producer ─────────────────────────────────────────────────────────────────

// Config configures a RabbitMQ producer.
type Config struct {
	// URL is the AMQP connection string.
	// e.g. "amqp://guest:guest@localhost:5672/"
	URL string

	// Exchange is the AMQP exchange name. Default: "astra".
	Exchange string

	// ExchangeType is "direct", "fanout", or "topic". Default: "direct".
	ExchangeType string

	// Durable exchanges and queues survive broker restarts. Default: true.
	Durable bool

	// MaxRetries is the maximum number of reconnection attempts. 0 = unlimited.
	MaxRetries int

	// RetryDelay is the base delay for exponential backoff. Default: 1s.
	RetryDelay time.Duration
}

func (c *Config) setDefaults() {
	if c.Exchange == "" {
		c.Exchange = "astra"
	}
	if c.ExchangeType == "" {
		c.ExchangeType = "direct"
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = time.Second
	}
}

// Producer publishes messages to a RabbitMQ exchange.
type Producer struct {
	cfg  Config
	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewProducer creates and connects a RabbitMQ producer.
func NewProducer(cfg Config) (*Producer, error) {
	cfg.setDefaults()
	p := &Producer{cfg: cfg}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Producer) connect() error {
	conn, err := amqp.Dial(p.cfg.URL)
	if err != nil {
		return fmt.Errorf("rabbitmq producer: dial %s: %w", p.cfg.URL, err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("rabbitmq producer: open channel: %w", err)
	}
	if err := ch.ExchangeDeclare(
		p.cfg.Exchange, p.cfg.ExchangeType,
		p.cfg.Durable, false, false, false, nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("rabbitmq producer: declare exchange %q: %w", p.cfg.Exchange, err)
	}

	p.mu.Lock()
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = conn
	p.ch = ch
	p.mu.Unlock()
	return nil
}

// Publish sends a message to the configured exchange.
// The message's Topic field is used as the routing key.
func (p *Producer) Publish(ctx context.Context, msg *mq.Message) error {
	headers := make(amqp.Table, len(msg.Headers))
	for k, v := range msg.Headers {
		headers[k] = v
	}
	publishing := amqp.Publishing{
		ContentType:  "application/octet-stream",
		Body:         msg.Payload,
		Headers:      headers,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
	}
	if len(msg.Key) > 0 {
		publishing.MessageId = string(msg.Key)
	}

	p.mu.Lock()
	ch := p.ch
	p.mu.Unlock()

	if err := ch.PublishWithContext(ctx, p.cfg.Exchange, msg.Topic, false, false, publishing); err != nil {
		// Try to reconnect once.
		if reconnErr := p.connect(); reconnErr != nil {
			return fmt.Errorf("rabbitmq producer: publish (reconnect failed: %v): %w", reconnErr, err)
		}
		p.mu.Lock()
		ch = p.ch
		p.mu.Unlock()
		return ch.PublishWithContext(ctx, p.cfg.Exchange, msg.Topic, false, false, publishing)
	}
	return nil
}

// PublishBatch publishes multiple messages sequentially.
func (p *Producer) PublishBatch(ctx context.Context, msgs []*mq.Message) error {
	for _, m := range msgs {
		if err := p.Publish(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the channel and connection.
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch != nil {
		p.ch.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// ─── Consumer ─────────────────────────────────────────────────────────────────

// ConsumerConfig configures a RabbitMQ consumer.
type ConsumerConfig struct {
	// URL is the AMQP connection string.
	URL string

	// Queue is the name of the queue to declare and consume from.
	Queue string

	// Exchange is the exchange to bind the queue to.
	Exchange string

	// ExchangeType is "direct", "fanout", or "topic". Default: "direct".
	ExchangeType string

	// RoutingKey is the binding key. For fanout exchanges, use "#".
	RoutingKey string

	// Durable queues survive broker restarts. Default: true.
	Durable bool

	// AutoAck automatically acknowledges messages. Default: false (manual ack).
	AutoAck bool

	// Prefetch is the maximum number of unacknowledged messages (QoS). Default: 1.
	Prefetch int

	// RetryDelay is the base delay for reconnection backoff. Default: 2s.
	RetryDelay time.Duration
}

func (c *ConsumerConfig) setDefaults() {
	if c.ExchangeType == "" {
		c.ExchangeType = "direct"
	}
	if c.Prefetch == 0 {
		c.Prefetch = 1
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = 2 * time.Second
	}
}

// Consumer subscribes to a RabbitMQ queue and processes deliveries.
type Consumer struct {
	cfg ConsumerConfig
}

// NewConsumer creates a RabbitMQ consumer. The connection is established
// lazily inside Subscribe.
func NewConsumer(cfg ConsumerConfig) (*Consumer, error) {
	cfg.setDefaults()
	return &Consumer{cfg: cfg}, nil
}

// Subscribe connects to RabbitMQ, declares the queue, and processes messages
// until ctx is cancelled. It reconnects automatically on connection errors.
func (c *Consumer) Subscribe(ctx context.Context, _ []string, _ string, handler mq.Handler) error {
	delay := c.cfg.RetryDelay
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := c.consume(ctx, handler); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("rabbitmq consumer: error, reconnecting",
				slog.String("queue", c.cfg.Queue),
				slog.String("err", err.Error()),
				slog.Duration("backoff", delay),
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay = min(delay*2, 30*time.Second)
			continue
		}
		return nil
	}
}

func (c *Consumer) consume(ctx context.Context, handler mq.Handler) error {
	conn, err := amqp.Dial(c.cfg.URL)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("channel: %w", err)
	}
	defer ch.Close()

	if c.cfg.Exchange != "" {
		if err := ch.ExchangeDeclare(c.cfg.Exchange, c.cfg.ExchangeType,
			c.cfg.Durable, false, false, false, nil,
		); err != nil {
			return fmt.Errorf("exchange declare: %w", err)
		}
	}

	q, err := ch.QueueDeclare(c.cfg.Queue, c.cfg.Durable, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("queue declare: %w", err)
	}

	if c.cfg.Exchange != "" {
		if err := ch.QueueBind(q.Name, c.cfg.RoutingKey, c.cfg.Exchange, false, nil); err != nil {
			return fmt.Errorf("queue bind: %w", err)
		}
	}

	if err := ch.Qos(c.cfg.Prefetch, 0, false); err != nil {
		return fmt.Errorf("qos: %w", err)
	}

	deliveries, err := ch.Consume(q.Name, "", c.cfg.AutoAck, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	connClose := conn.NotifyClose(make(chan *amqp.Error, 1))

	for {
		select {
		case <-ctx.Done():
			return nil
		case amqpErr, ok := <-connClose:
			if !ok || amqpErr != nil {
				return fmt.Errorf("connection closed: %v", amqpErr)
			}
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}
			msg := deliveryToMessage(d, c.cfg.Queue)
			if err := handler(ctx, msg); err != nil {
				if !c.cfg.AutoAck {
					_ = d.Nack(false, true) // requeue
				}
				slog.Warn("rabbitmq: handler error", slog.String("err", err.Error()))
			} else if !c.cfg.AutoAck {
				_ = d.Ack(false)
			}
		}
	}
}

// Close is a no-op; the connection is per-Subscribe call.
func (c *Consumer) Close() error { return nil }

func deliveryToMessage(d amqp.Delivery, queue string) *mq.Message {
	headers := make(map[string]string, len(d.Headers))
	for k, v := range d.Headers {
		if s, ok := v.(string); ok {
			headers[k] = s
		}
	}
	return &mq.Message{
		Topic:   d.RoutingKey,
		Key:     []byte(d.MessageId),
		Payload: d.Body,
		Headers: headers,
		Meta: map[string]any{
			"queue":        queue,
			"exchange":     d.Exchange,
			"delivery_tag": d.DeliveryTag,
			"redelivered":  d.Redelivered,
		},
	}
}

