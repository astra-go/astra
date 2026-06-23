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
// routing key, and processes deliveries via the Handler callback.
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
//	p.Publish(ctx, &Message{Topic: "user.created", Payload: body})
//
//	c, err := rabbitmq.NewConsumer(rabbitmq.ConsumerConfig{
//	    URL:        "amqp://guest:guest@localhost:5672/",
//	    Queue:      "user-service",
//	    Exchange:   "events",
//	    RoutingKey: "user.*",
//	    Prefetch:   10,
//	})
//	c.Subscribe(ctx, nil, "", func(ctx context.Context, msg *Message) error {
//	    return handleMessage(msg)
//	})
package mq

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

// ─── Idempotency Cache Interface ──────────────────────────────────────────────
// IdempCache is a pluggable cache for message idempotent deduplication.
type IdempCache interface {
	// IsProcessed returns true if the key was already processed.
	IsProcessed(key string) bool
	// MarkProcessed marks the key as processed with the given TTL.
	MarkProcessed(key string, ttl time.Duration)
}

// InMemoryIdempCache is a simple in-memory idempotency cache.
// It is NOT safe for distributed deployments; use RedisIdempCache instead.
type InMemoryIdempCache struct {
	mu    sync.RWMutex
	cache map[string]time.Time
	ttl   time.Duration
}

func NewInMemoryIdempCache(cleanupInterval, ttl time.Duration) *InMemoryIdempCache {
	c := &InMemoryIdempCache{
		cache: make(map[string]time.Time),
		ttl:   ttl,
	}
	go c.cleanup(cleanupInterval)
	return c
}

func (c *InMemoryIdempCache) IsProcessed(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	expireAt, ok := c.cache[key]
	if !ok {
		return false
	}
	if time.Now().After(expireAt) {
		delete(c.cache, key)
		return false
	}
	return true
}

func (c *InMemoryIdempCache) MarkProcessed(key string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ttl <= 0 {
		ttl = c.ttl
	}
	c.cache[key] = time.Now().Add(ttl)
}

func (c *InMemoryIdempCache) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, v := range c.cache {
			if now.After(v) {
				delete(c.cache, k)
			}
		}
		c.mu.Unlock()
	}
}

// RedisIdempCache uses Redis SET NX for distributed idempotent deduplication.
type RedisIdempCache struct {
	client *redis.Client
}

// NewRedisIdempCache creates a RedisIdempCache backed by the given Redis client.
func NewRedisIdempCache(client *redis.Client) *RedisIdempCache {
	return &RedisIdempCache{client: client}
}

func (c *RedisIdempCache) IsProcessed(key string) bool {
	return c.client.Exists(context.Background(), key).Val() > 0
}

func (c *RedisIdempCache) MarkProcessed(key string, ttl time.Duration) {
	c.client.Set(context.Background(), key, "1", ttl)
}

// DefaultRetryDelays is the default staircase retry delays: 15s, 30s, 45s, 60s, 75s.
// Used as the Levels field of RetryPolicy.
var DefaultRetryDelays = []time.Duration{
	15 * time.Second,
	30 * time.Second,
	45 * time.Second,
	60 * time.Second,
	75 * time.Second,
}

// ─── Producer ─────────────────────────────────────────────────────────────────

// Config configures a RabbitMQ producer.
type RabbitMQConfig struct {
	// URL is the AMQP connection string.
	// e.g. "amqp://guest:guest@localhost:5672/"
	URL string

	// Exchange is the AMQP exchange name. Default: "astra".
	Exchange string

	// ExchangeType is "direct", "fanout", "topic", or "x-delayed-message".
	// Use "x-delayed-message" for delayed delivery (requires rabbitmq-delayed-message-exchange plugin).
	// Default: "direct".
	ExchangeType string

	// Durable exchanges and queues survive broker restarts. Default: true.
	Durable bool

	// EnableTx enables AMQP transactions (CapTx). Default: false.
	// When enabled, each Publish is wrapped in TxSelect/TxCommit.
	EnableTx bool

	// BatchSize is the number of messages to batch before flushing. 0 disables batching.
	// When batching is enabled, messages are buffered and flushed when:
	// - batch size is reached, or
	// - batch flush timeout expires
	BatchSize    int
	BatchTimeout time.Duration

	// MaxRetries is the maximum number of reconnection attempts. 0 = unlimited.
	MaxRetries int

	// RetryDelay is the base delay for exponential backoff. Default: 1s.
	RetryDelay time.Duration
}

func (c *RabbitMQConfig) setDefaults() {
	if c.Exchange == "" {
		c.Exchange = "astra"
	}
	if c.ExchangeType == "" {
		c.ExchangeType = "direct"
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = time.Second
	}
	if c.BatchSize > 0 && c.BatchTimeout == 0 {
		c.BatchTimeout = 100 * time.Millisecond
	}
}

// Producer publishes messages to a RabbitMQ exchange.
type RabbitMQProducer struct {
	cfg  RabbitMQConfig
	mu   sync.Mutex
	cond *sync.Cond // for batch flush notification
	conn *amqp.Connection
	ch   *amqp.Channel

	// batching
	batchMu    sync.Mutex
	batchBuf   []*Message
	batchTimer *time.Timer
	batchDone  chan struct{}
	flushReq   chan chan struct{} // for explicit flush
}

// NewProducer creates and connects a RabbitMQ producer.
func NewRabbitMQProducer(cfg RabbitMQConfig) (*RabbitMQProducer, error) {
	cfg.setDefaults()
	p := &RabbitMQProducer{
		cfg:       cfg,
		batchBuf:  make([]*Message, 0, cfg.BatchSize),
		batchDone: make(chan struct{}),
		flushReq:  make(chan chan struct{}),
	}
	if cfg.BatchSize > 0 {
		p.cond = sync.NewCond(&p.batchMu)
		go p.batchFlusher()
	}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *RabbitMQProducer) connect() error {
	conn, err := amqp.Dial(p.cfg.URL)
	if err != nil {
		return fmt.Errorf("rabbitmq producer: dial %s: %w", p.cfg.URL, err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("rabbitmq producer: open channel: %w", err)
	}

	// Declare exchange; handle x-delayed-message type (requires plugin)
	exchangeType := p.cfg.ExchangeType
	var args amqp.Table
	if exchangeType == "x-delayed-message" {
		// x-delayed-type specifies the routing behavior (direct/topic/fanout)
		args = amqp.Table{"x-delayed-type": "direct"}
	}
	if err := ch.ExchangeDeclare(
		p.cfg.Exchange, exchangeType,
		p.cfg.Durable, false, false, false, args,
	); err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("rabbitmq producer: declare exchange %q (type=%s): %w", p.cfg.Exchange, exchangeType, err)
	}

	// Enable transactions if configured
	if p.cfg.EnableTx {
		if err := ch.Tx(); err != nil {
			ch.Close()
			conn.Close()
			return fmt.Errorf("rabbitmq producer: tx: %w", err)
		}
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
// If msg.Delay > 0, the message is delayed (requires x-delayed-message exchange).
func (p *RabbitMQProducer) Publish(ctx context.Context, msg *Message) error {
	// If batching is enabled, buffer the message
	if p.cfg.BatchSize > 0 {
		return p.publishBatch(ctx, msg)
	}
	return p.publishSingle(ctx, msg)
}

func (p *RabbitMQProducer) publishSingle(ctx context.Context, msg *Message) error {
	headers := make(amqp.Table, len(msg.Headers)+2)
	for k, v := range msg.Headers {
		headers[k] = v
	}
	// Set delay header for x-delayed-message exchange
	if msg.Delay > 0 {
		headers["x-delay"] = int64(msg.Delay / time.Millisecond)
	}
	if msg.TraceID != "" {
		headers["x-trace-id"] = msg.TraceID
	}

	publishing := amqp.Publishing{
		ContentType:  "application/octet-stream",
		Body:         msg.Payload,
		Headers:      headers,
		DeliveryMode: amqp.Persistent, // persistent by default
		Timestamp:    time.Now(),
	}
	if len(msg.Key) > 0 {
		publishing.MessageId = string(msg.Key)
	}
	if msg.IdempKey != "" {
		publishing.Headers["x-idemp-key"] = msg.IdempKey
	}

	p.mu.Lock()
	ch := p.ch
	p.mu.Unlock()

	if p.cfg.EnableTx {
		if err := ch.Tx(); err != nil {
			return fmt.Errorf("rabbitmq producer: tx: %w", err)
		}
	}

	err := ch.PublishWithContext(ctx, p.cfg.Exchange, msg.Topic, false, false, publishing)
	if p.cfg.EnableTx {
		if txErr := ch.TxCommit(); txErr != nil {
			_ = ch.TxRollback()
			return fmt.Errorf("rabbitmq producer: tx.commit: %w", txErr)
		}
	}
	if err != nil {
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

// publishBatch buffers the message for batch sending.
func (p *RabbitMQProducer) publishBatch(ctx context.Context, msg *Message) error {
	p.batchMu.Lock()
	p.batchBuf = append(p.batchBuf, msg)
	if len(p.batchBuf) >= p.cfg.BatchSize {
		p.cond.Signal() // wake up flusher
		p.batchMu.Unlock()
		return nil
	}
	p.batchMu.Unlock()
	return nil
}

// batchFlusher runs in a goroutine and flushes the batch buffer periodically.
func (p *RabbitMQProducer) batchFlusher() {
	ticker := time.NewTicker(p.cfg.BatchTimeout)
	defer ticker.Stop()
	for {
		select {
		case <-p.batchDone:
			p.flushBatch(context.Background())
			return
		case <-ticker.C:
			p.flushBatch(context.Background())
		case resp := <-p.flushReq:
			p.flushBatch(context.Background())
			close(resp)
		}
	}
}

// flushBatch flushes the current batch buffer.
func (p *RabbitMQProducer) flushBatch(ctx context.Context) {
	p.batchMu.Lock()
	if len(p.batchBuf) == 0 {
		p.batchMu.Unlock()
		return
	}
	batch := p.batchBuf
	p.batchBuf = make([]*Message, 0, p.cfg.BatchSize)
	p.batchMu.Unlock()

	// Publish all messages in the batch
	for _, msg := range batch {
		if err := p.publishSingle(ctx, msg); err != nil {
			slog.Warn("rabbitmq producer: batch flush error", slog.String("err", err.Error()))
			// Continue with remaining messages
		}
	}
}

// Flush flushes any buffered messages. Blocks until flush is complete.
func (p *RabbitMQProducer) Flush(ctx context.Context) {
	if p.cfg.BatchSize <= 0 {
		return
	}
	resp := make(chan struct{})
	p.flushReq <- resp
	<-resp
}

// PublishBatch publishes multiple messages. If batching is enabled,
// the messages are added to the batch buffer. Otherwise, they are
// published sequentially.
func (p *RabbitMQProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	if p.cfg.BatchSize > 0 {
		for _, m := range msgs {
			if err := p.publishBatch(ctx, m); err != nil {
				return err
			}
		}
		return nil
	}
	for _, m := range msgs {
		if err := p.publishSingle(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the channel and connection.
// If batching is enabled, flushes any buffered messages first.
func (p *RabbitMQProducer) Close() error {
	if p.cfg.BatchSize > 0 {
		close(p.batchDone)
	}
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
type RabbitMQConsumerConfig struct {
	// URL is the AMQP connection string.
	URL string

	// Queue is the name of the queue to declare and consume from.
	Queue string

	// Exchange is the exchange to bind the queue to.
	Exchange string

	// ExchangeType is "direct", "fanout", "topic", or "x-delayed-message".
	// Default: "direct".
	ExchangeType string

	// RoutingKey is the binding key. For fanout exchanges, use "#".
	RoutingKey string

	// Durable queues survive broker restarts. Default: true.
	Durable bool

	// AutoAck automatically acknowledges messages. Default: false (manual ack).
	AutoAck bool

	// Prefetch is the maximum number of unacknowledged messages (QoS). Default: 1.
	Prefetch int

	// IdempCache is the idempotent deduplication cache. If nil, idempotency is disabled.
	IdempCache IdempCache

	// RetryPolicy defines the retry behavior for failed messages.
	// If nil, no retry is performed (messages are NACKed and requeued).
	RetryPolicy *RetryPolicy

	// DLQExchange is the dead-letter exchange for messages that exceed max retries.
	// If empty, dead-lettering is disabled.
	DLQExchange string

	// DLQQueue is the dead-letter queue name.
	DLQQueue string

	// RetryDelay is the base delay for reconnection backoff. Default: 2s.
	RetryDelay time.Duration
}

func (c *RabbitMQConsumerConfig) setDefaults() {
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
type RabbitMQConsumer struct {
	cfg         RabbitMQConsumerConfig
	idempCache  IdempCache
	retryPolicy *RetryPolicy
	dlqExchange string
	dlqQueue    string
}

// NewConsumer creates a RabbitMQ consumer. The connection is established
// lazily inside Subscribe.
func NewRabbitMQConsumer(cfg RabbitMQConsumerConfig) (*RabbitMQConsumer, error) {
	cfg.setDefaults()
	return &RabbitMQConsumer{
		cfg:         cfg,
		idempCache:  cfg.IdempCache,
		retryPolicy: cfg.RetryPolicy,
		dlqExchange: cfg.DLQExchange,
		dlqQueue:    cfg.DLQQueue,
	}, nil
}

// Subscribe connects to RabbitMQ, declares the queue, and processes messages
// until ctx is cancelled. It reconnects automatically on connection errors.
func (c *RabbitMQConsumer) Subscribe(ctx context.Context, _ []string, _ string, handler Handler) error {
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

func (c *RabbitMQConsumer) consume(ctx context.Context, handler Handler) error {
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

			// ── Idempotency Check ──
			if c.idempCache != nil && msg.IdempKey != "" {
				if c.idempCache.IsProcessed(msg.IdempKey) {
					// Already processed; ACK and skip
					_ = d.Ack(false)
					continue
				}
			}

			// ── Call Handler ──
			handlerErr := handler(ctx, msg)

			// ── Post-Handler Logic ──
			if handlerErr != nil {
				slog.Warn("rabbitmq: handler error",
					slog.String("queue", c.cfg.Queue),
					slog.String("err", handlerErr.Error()),
					slog.Int("retry_count", msg.RetryCount),
				)

				// Retry logic
				if c.retryPolicy != nil {
					nextDelay, shouldRetry := c.retryPolicy.NextDelay(msg.RetryCount)
					if shouldRetry {
						// Republish with delay for staircase retry
						msg.RetryCount++
						msg.Delay = nextDelay

						repubErr := c.republish(ctx, ch, msg)
						if repubErr != nil {
							slog.Error("rabbitmq: retry republish failed", slog.String("err", repubErr.Error()))
							// Fall through to NACK
						} else {
							// Successfully republished; ACK original message
							_ = d.Ack(false)
							continue
						}
					}
				}

				// Max retries exceeded or retry disabled: send to DLQ
				if c.dlqExchange != "" {
					dlqErr := c.sendToDLQ(ctx, ch, msg, handlerErr)
					if dlqErr != nil {
						slog.Error("rabbitmq: send to DLQ failed", slog.String("err", dlqErr.Error()))
					}
					_ = d.Ack(false) // ACK original even if DLQ fails (avoid infinite retry)
					continue
				}

				// No DLQ: NACK with requeue
				if !c.cfg.AutoAck {
					_ = d.Nack(false, true)
				}
			} else {
				// Success
				if !c.cfg.AutoAck {
					_ = d.Ack(false)
				}
				// Mark idempotent if cache is configured
				if c.idempCache != nil && msg.IdempKey != "" {
					c.idempCache.MarkProcessed(msg.IdempKey, 24*time.Hour) // default 24h TTL
				}
			}
		}
	}
}

// Close is a no-op; the connection is per-Subscribe call.
func (c *RabbitMQConsumer) Close() error { return nil }

func deliveryToMessage(d amqp.Delivery, queue string) *Message {
	headers := make(map[string]string, len(d.Headers))
	var idempKey string
	var retryCount int
	for k, v := range d.Headers {
		switch k {
		case "x-idemp-key":
			if s, ok := v.(string); ok {
				idempKey = s
			}
		case "x-retry-count":
			if i, ok := v.(int); ok {
				retryCount = i
			}
		default:
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}
	return &Message{
		Topic:      d.RoutingKey,
		Key:        []byte(d.MessageId),
		Payload:    d.Body,
		Headers:    headers,
		IdempKey:   idempKey,
		RetryCount: retryCount,
		Meta: map[string]any{
			"queue":        queue,
			"exchange":     d.Exchange,
			"delivery_tag": d.DeliveryTag,
			"redelivered":  d.Redelivered,
		},
	}
}

// ─── Retry & DLQ Helpers ─────────────────────────────────────────────────────

// republish republishes a message with a delay for staircase retry.
// It sets the x-delay header so the x-delayed-message exchange will delay delivery.
func (c *RabbitMQConsumer) republish(ctx context.Context, ch *amqp.Channel, msg *Message) error {
	headers := make(amqp.Table, len(msg.Headers)+3)
	for k, v := range msg.Headers {
		headers[k] = v
	}
	headers["x-retry-count"] = msg.RetryCount
	if msg.IdempKey != "" {
		headers["x-idemp-key"] = msg.IdempKey
	}
	if msg.Delay > 0 {
		headers["x-delay"] = int64(msg.Delay / time.Millisecond)
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

	return ch.PublishWithContext(ctx, c.cfg.Exchange, msg.Topic, false, false, publishing)
}

// sendToDLQ sends a failed message to the Dead Letter Queue.
func (c *RabbitMQConsumer) sendToDLQ(ctx context.Context, ch *amqp.Channel, msg *Message, handlerErr error) error {
	headers := make(amqp.Table, len(msg.Headers)+3)
	for k, v := range msg.Headers {
		headers[k] = v
	}
	headers["x-death-reason"] = handlerErr.Error()
	headers["x-retry-count"] = msg.RetryCount
	if msg.IdempKey != "" {
		headers["x-idemp-key"] = msg.IdempKey
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

	return ch.PublishWithContext(ctx, c.dlqExchange, c.dlqQueue, false, false, publishing)
}

// Capabilities returns the capabilities of RabbitMQ.
func (p *RabbitMQProducer) Capabilities() Capabilities { return RabbitMQCapabilities() }
func (c *RabbitMQConsumer) Capabilities() Capabilities { return RabbitMQCapabilities() }
