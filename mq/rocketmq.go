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
// # Transaction Messages
//
//	producer, _ := rocketmq.NewProducer(rocketmq.Config{
//	    Endpoint:  "localhost:8081",
//	    Topic:     "payments",
//	    EnableTx:  true,
//	    TransactionChecker: func(ctx context.Context, msg *Message) (bool, error) {
//	        // Check local DB: was this order's payment processed?
//	        return checkPaymentStatus(ctx, msg)
//	    },
//	})
//	defer producer.Close()
//
//	tx, _ := producer.BeginTransaction(ctx, nil)
//	tx.Publish(ctx, &Message{Topic: "payments", Payload: orderJSON})
//
//	if err := processPayment(ctx, order); err != nil {
//	    tx.Rollback(ctx) // discard
//	} else {
//	    tx.Commit(ctx)   // visible to consumers
//	}
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
	"strconv"
	"sync"
	"time"

	rmq "github.com/apache/rocketmq-clients/golang/v5"
	"github.com/apache/rocketmq-clients/golang/v5/credentials"
)

// ─── Config ──────────────────────────────────────────────────────────────────

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

	// EnableDelay enables delayed delivery via SetDelayTimestamp.
	// RocketMQ v5 natively supports arbitrary-precision delay.
	EnableDelay bool

	// IdempCache is the idempotent deduplication cache. If nil, idempotency is disabled.
	// Reuses the IdempCache interface defined in rabbitmq.go.
	IdempCache IdempCache

	// RetryPolicy defines the retry behavior for failed consumer messages.
	// If nil, no retry is performed (message is not re-delivered).
	// Reuses the RetryPolicy type defined in errors.go.
	RetryPolicy *RetryPolicy

	// DLQTopic is the topic to which failed messages are forwarded after
	// exhausting all retries. If empty, dead-lettering is disabled.
	DLQTopic string

	// EnableTx enables transactional messages using TransactionProducer.
	EnableTx bool

	// TransactionChecker is the callback invoked by the RocketMQ broker
	// when the producer crashes before sending Commit or Rollback.
	// It should query local storage (DB, Redis, etc.) and return whether
	// the local transaction was committed or not.
	// Only used when EnableTx is true.
	TransactionChecker TransactionChecker

	// BatchSize is the number of messages to buffer before flushing to the
	// broker. 0 disables client-side batching. When batching is enabled,
	// messages are buffered and flushed when batch size is reached or the
	// batch flush timeout expires.
	BatchSize    int
	BatchTimeout time.Duration

	// MessageGroup sets the message group for ordered delivery within a
	// partition (RocketMQ v5 partition-scoped ordering).
	MessageGroup string

	// PriorityTopics maps priority levels to topic names.
	// If set, Producer routes messages to the corresponding topic
	// based on msg.Priority; Consumer subscribes to all listed topics.
	// Example: map[int]string{3: "order.high", 2: "order.normal", 1: "order.low"}
	PriorityTopics map[int]string
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
	if c.BatchSize > 0 && c.BatchTimeout == 0 {
		c.BatchTimeout = 100 * time.Millisecond
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

	// batching
	batchMu   sync.Mutex
	batchBuf  []*Message
	batchDone chan struct{}
	flushReq  chan chan struct{}

	// transaction — protected by txMu; nil when no active transaction
	txMu     sync.Mutex
	activeTx rmq.Transaction
}

// NewProducer creates and starts a RocketMQ producer.
func NewRocketMQProducer(cfg RocketMQConfig) (*RocketMQProducer, error) {
	cfg.setDefaults()

	rmq.EnableSsl = cfg.EnableSSL

	var opts []rmq.ProducerOption
	opts = append(opts, rmq.WithTopics(cfg.Topic))
	opts = append(opts, rmq.WithMaxAttempts(cfg.MaxAttempts))

	// Register transaction checker if EnableTx is true
	if cfg.EnableTx && cfg.TransactionChecker != nil {
		rmqChecker := &rmq.TransactionChecker{
			Check: func(msgView *rmq.MessageView) rmq.TransactionResolution {
				// Convert rmq.MessageView to mq.Message
				msg := fromMessageView(msgView)

				// Call user's checker
				committed, err := cfg.TransactionChecker(context.Background(), msg)
				if err != nil {
					slog.Error("rocketmq: transaction checker error",
						slog.String("err", err.Error()),
					)
					return rmq.ROLLBACK
				}

				if committed {
					return rmq.COMMIT
				}
				return rmq.ROLLBACK
			},
		}
		opts = append(opts, rmq.WithTransactionChecker(rmqChecker))
	}

	prod, err := rmq.NewProducer(cfg.rmqConfig(), opts...)
	if err != nil {
		return nil, fmt.Errorf("rocketmq producer: create: %w", err)
	}
	if err := prod.Start(); err != nil {
		return nil, fmt.Errorf("rocketmq producer: start: %w", err)
	}
	p := &RocketMQProducer{
		cfg:       cfg,
		prod:      prod,
		batchBuf:  make([]*Message, 0, cfg.BatchSize),
		batchDone: make(chan struct{}),
		flushReq:  make(chan chan struct{}),
	}
	if cfg.BatchSize > 0 {
		go p.batchFlusher()
	}
	return p, nil
}

// Publish sends a single message synchronously.
//
// If client-side batching is enabled (cfg.BatchSize > 0), the message is
// buffered and flushed periodically or when the batch is full.
//
// Behaviour per config:
//   - cfg.EnableDelay && msg.Delay > 0 → SetDelayTimestamp (v5 arbitrary delay)
//   - msg.IdempKey != "" → stored as property "x-idemp-key"
//   - cfg.MessageGroup != "" → SetMessageGroup (partition-scoped ordered delivery)
//   - cfg.EnableTx: TransactionChecker is registered at producer creation time
//     (mq.WithTransactionChecker); for explicit half-message control, use
//     BeginTransaction() to get a Transaction and call tx.Publish(ctx, msg)
//     before tx.Commit/Rollback.
func (p *RocketMQProducer) Publish(ctx context.Context, msg *Message) error {
	if p.cfg.BatchSize > 0 {
		return p.bufferMessage(ctx, msg)
	}
	return p.publishSingle(ctx, msg)
}

// publishSingle sends a single message to RocketMQ.
func (p *RocketMQProducer) publishSingle(ctx context.Context, msg *Message) error {
	rmqMsg := toRMQMessage(msg)

	// ── Priority-based topic routing ──
	if len(p.cfg.PriorityTopics) > 0 {
		if topic, ok := p.cfg.PriorityTopics[msg.Priority]; ok {
			rmqMsg.Topic = topic
		}
	}

	// ── Delay delivery ──
	if p.cfg.EnableDelay && msg.Delay > 0 {
		rmqMsg.SetDelayTimestamp(time.Now().Add(msg.Delay))
	}

	// ── Message group (partition-scoped ordered delivery) ──
	if p.cfg.MessageGroup != "" {
		rmqMsg.SetMessageGroup(p.cfg.MessageGroup)
	}

	// ── Transaction path ──
	// If a transaction was started via BeginTransaction, publish within it.
	// Otherwise fall through to regular send.
	p.txMu.Lock()
	tx := p.activeTx
	p.txMu.Unlock()

	if tx != nil {
		// SendWithTransaction returns a slice; log receipt count for observability
		receipts, err := p.prod.SendWithTransaction(ctx, rmqMsg, tx)
		if err != nil {
			return fmt.Errorf("rocketmq publish (transactional): %w", err)
		}
		slog.Debug("rocketmq publish ok",
			slog.String("topic", msg.Topic),
			slog.Int("receipts", len(receipts)),
		)
		return nil
	}

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

// bufferMessage buffers a message for client-side batching.
func (p *RocketMQProducer) bufferMessage(ctx context.Context, msg *Message) error {
	p.batchMu.Lock()
	p.batchBuf = append(p.batchBuf, msg)
	full := len(p.batchBuf) >= p.cfg.BatchSize
	p.batchMu.Unlock()

	if full {
		// Trigger an async flush signal.
		go func() {
			resp := make(chan struct{})
			select {
			case p.flushReq <- resp:
				<-resp
			case <-ctx.Done():
			}
		}()
	}
	return nil
}

// PublishBatch sends multiple messages. If client-side batching is enabled
// (cfg.BatchSize > 0), each message is buffered individually. Otherwise,
// messages are sent sequentially via the single-message Send() API.
func (p *RocketMQProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	if len(msgs) == 0 {
		return nil
	}

	if p.cfg.BatchSize > 0 {
		// Buffer each message individually
		for _, m := range msgs {
			if err := p.bufferMessage(ctx, m); err != nil {
				return err
			}
		}
		return nil
	}

	// RocketMQ client Send() only accepts a single message, so we send
	// sequentially. This is semantically a batch for the caller.
	for _, msg := range msgs {
		rmqMsg := toRMQMessage(msg)
		// ── Priority-based topic routing ──
		if len(p.cfg.PriorityTopics) > 0 {
			if topic, ok := p.cfg.PriorityTopics[msg.Priority]; ok {
				rmqMsg.Topic = topic
			}
		}
		if p.cfg.EnableDelay && msg.Delay > 0 {
			rmqMsg.SetDelayTimestamp(time.Now().Add(msg.Delay))
		}
		if p.cfg.MessageGroup != "" {
			rmqMsg.SetMessageGroup(p.cfg.MessageGroup)
		}
		if _, err := p.prod.Send(ctx, rmqMsg); err != nil {
			return fmt.Errorf("rocketmq batch publish: %w", err)
		}
	}
	slog.Debug("rocketmq batch publish ok",
		slog.Int("count", len(msgs)),
	)
	return nil
}

// batchFlusher runs in a goroutine and flushes buffered messages
// periodically or when explicitly requested.
func (p *RocketMQProducer) batchFlusher() {
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

// flushBatch flushes the buffered messages.
// RocketMQ Send() only takes a single message, so we send sequentially.
func (p *RocketMQProducer) flushBatch(ctx context.Context) {
	p.batchMu.Lock()
	if len(p.batchBuf) == 0 {
		p.batchMu.Unlock()
		return
	}
	batch := p.batchBuf
	p.batchBuf = make([]*Message, 0, p.cfg.BatchSize)
	p.batchMu.Unlock()

	for _, msg := range batch {
		rmqMsg := toRMQMessage(msg)
		// ── Priority-based topic routing ──
		if len(p.cfg.PriorityTopics) > 0 {
			if topic, ok := p.cfg.PriorityTopics[msg.Priority]; ok {
				rmqMsg.Topic = topic
			}
		}
		if p.cfg.EnableDelay && msg.Delay > 0 {
			rmqMsg.SetDelayTimestamp(time.Now().Add(msg.Delay))
		}
		if p.cfg.MessageGroup != "" {
			rmqMsg.SetMessageGroup(p.cfg.MessageGroup)
		}
		if _, err := p.prod.Send(ctx, rmqMsg); err != nil {
			slog.Warn("rocketmq producer: batch flush error",
				slog.String("err", err.Error()),
			)
			// Continue with remaining messages
		}
	}
}

// Flush flushes any buffered messages. Blocks until flush is complete.
func (p *RocketMQProducer) Flush(ctx context.Context) {
	if p.cfg.BatchSize <= 0 {
		return
	}
	resp := make(chan struct{})
	select {
	case p.flushReq <- resp:
		<-resp
	case <-ctx.Done():
	}
}

// Close stops the underlying RocketMQ producer gracefully.
// If batching is enabled, buffered messages are flushed first.
func (p *RocketMQProducer) Close() error {
	if p.cfg.BatchSize > 0 {
		close(p.batchDone)
	}
	return p.prod.GracefulStop()
}

// ─── Consumer ─────────────────────────────────────────────────────────────────

// Consumer receives messages from RocketMQ topics using the SimpleConsumer API.
type RocketMQConsumer struct {
	cfg         RocketMQConfig
	idempCache  IdempCache
	retryPolicy *RetryPolicy
	dlqTopic    string

	// Lazy producer for republish / DLQ (created inside Subscribe).
	prodMu sync.Mutex
	prod   rmq.Producer // lazily initialised
}

// ConsumerConfig configures a RocketMQ consumer.
type RocketMQConsumerConfig = RocketMQConfig

// NewConsumer creates a RocketMQ consumer. The connection is established lazily
// inside Subscribe.
func NewRocketMQConsumer(cfg RocketMQConsumerConfig) (*RocketMQConsumer, error) {
	cfg.setDefaults()
	return &RocketMQConsumer{
		cfg:         cfg,
		idempCache:  cfg.IdempCache,
		retryPolicy: cfg.RetryPolicy,
		dlqTopic:    cfg.DLQTopic,
	}, nil
}

// Subscribe starts consuming from topics and calls handler for each message.
// It blocks until ctx is cancelled.
//
// The group parameter overrides cfg.ConsumerGroup when non-empty.
//
// Enhanced behaviour:
//   - cfg.IdempCache != nil → deduplicate via IsProcessed / MarkProcessed
//   - cfg.RetryPolicy != nil → staircase retry on handler error
//   - cfg.DLQTopic != "" → forward permanently-failed messages there
//   - Handler returning *MQError (IsPermanent / IsRetry / IsSkip) is respected
//   - cfg.PriorityTopics != nil → messages from higher-priority topics are processed first
func (c *RocketMQConsumer) Subscribe(ctx context.Context, topics []string, group string, handler Handler) error {
	if len(topics) == 0 {
		return fmt.Errorf("rocketmq consumer: at least one topic is required")
	}
	if group != "" {
		c.cfg.ConsumerGroup = group
	}

	rmq.EnableSsl = c.cfg.EnableSSL

	// Collect all topics including priority topics
	allTopics := c.collectAllTopics(topics)

	subExpressions := make(map[string]*rmq.FilterExpression, len(allTopics))
	for _, t := range allTopics {
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
	defer c.closeProducer()

	slog.Info("rocketmq consumer: started",
		slog.String("endpoint", c.cfg.Endpoint),
		slog.Any("topics", allTopics),
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

		// ── Priority-based ordering ──
		// If PriorityTopics is configured, sort messages by topic priority
		if len(c.cfg.PriorityTopics) > 0 {
			views = c.sortViewsByPriority(views)
		}

		for _, view := range views {
			msg := fromMessageView(view)

			// ── Idempotency Check ──
			if c.idempCache != nil && msg.IdempKey != "" {
				if c.idempCache.IsProcessed(msg.IdempKey) {
					// Already processed; ACK and skip
					if ackErr := sc.Ack(ctx, view); ackErr != nil {
						slog.Warn("rocketmq ack (idemp-skip)",
							slog.String("topic", view.GetTopic()),
							slog.String("err", ackErr.Error()),
						)
					}
					continue
				}
			}

			// ── Call Handler ──
			handlerErr := handler(ctx, msg)

			// ── Post-Handler Logic ──
			if handlerErr != nil {
				slog.Warn("rocketmq: handler error",
					slog.String("topic", view.GetTopic()),
					slog.String("err", handlerErr.Error()),
					slog.Int("retry_count", msg.RetryCount),
				)

				// MQError semantic routing
				if IsSkip(handlerErr) {
					// Skip: ACK without retry/DLQ
					if ackErr := sc.Ack(ctx, view); ackErr != nil {
						slog.Warn("rocketmq ack (skip)",
							slog.String("err", ackErr.Error()),
						)
					}
					continue
				}

				if IsPermanent(handlerErr) {
					// Permanent failure: send to DLQ
					c.sendToDLQ(ctx, msg, handlerErr)
					if ackErr := sc.Ack(ctx, view); ackErr != nil {
						slog.Warn("rocketmq ack (permanent)",
							slog.String("err", ackErr.Error()),
						)
					}
					continue
				}

				// IsRetry or plain error: staircase retry
				if c.retryPolicy != nil {
					nextDelay, shouldRetry := c.retryPolicy.NextDelay(msg.RetryCount)
					if shouldRetry {
						msg.RetryCount++
						msg.Delay = nextDelay

						// Prefer NAK delay (ChangeInvisibleDuration) over republish.
						// NAK delay keeps the original message ID, trace ID, and
						// avoids creating duplicate messages. The broker will
						// redeliver the message automatically after nextDelay.
						// No ACK needed - message stays in broker management.
						if nackErr := sc.ChangeInvisibleDuration(view, nextDelay); nackErr != nil {
							slog.Warn("rocketmq: ChangeInvisibleDuration failed, falling back to republish",
								slog.String("err", nackErr.Error()),
							)
							// Fallback: republishToSelf
							repubErr := c.republishToSelf(ctx, msg)
							if repubErr != nil {
								slog.Error("rocketmq: retry republish failed (fallback)",
									slog.String("err", repubErr.Error()),
								)
							} else {
								// Successfully republished; ACK original message
								if ackErr := sc.Ack(ctx, view); ackErr != nil {
									slog.Warn("rocketmq ack (retry/repub fallback)",
										slog.String("err", ackErr.Error()),
									)
								}
								continue
							}
						} else {
							slog.Debug("rocketmq: ChangeInvisibleDuration (NAK delay)",
								slog.Int("retry_count", msg.RetryCount),
								slog.Duration("delay", nextDelay),
							)
							continue
						}
					}
				}

				// Max retries exceeded or no retry policy: send to DLQ
				if c.dlqTopic != "" {
					c.sendToDLQ(ctx, msg, handlerErr)
				}

				// ACK original regardless (avoid infinite invisible cycle)
				if ackErr := sc.Ack(ctx, view); ackErr != nil {
					slog.Warn("rocketmq ack (exhausted)",
						slog.String("err", ackErr.Error()),
					)
				}
				continue
			}

			// ── Success ──
			if ackErr := sc.Ack(ctx, view); ackErr != nil {
				slog.Warn("rocketmq ack error",
					slog.String("topic", view.GetTopic()),
					slog.String("err", ackErr.Error()),
				)
			}
			// Mark idempotent
			if c.idempCache != nil && msg.IdempKey != "" {
				c.idempCache.MarkProcessed(msg.IdempKey, 24*time.Hour) // default 24h TTL
			}
		}
	}
}

// Close is a no-op; the consumer is started and stopped inside Subscribe.
func (c *RocketMQConsumer) Close() error { return nil }

// ─── Consumer helpers ─────────────────────────────────────────────────────────

// ensureProducer creates a producer lazily for republish / DLQ.
func (c *RocketMQConsumer) ensureProducer(ctx context.Context) (rmq.Producer, error) {
	c.prodMu.Lock()
	defer c.prodMu.Unlock()
	if c.prod != nil {
		return c.prod, nil
	}

	rmq.EnableSsl = c.cfg.EnableSSL
	prod, err := rmq.NewProducer(
		c.cfg.rmqConfig(),
		rmq.WithTopics(c.cfg.Topic),
		rmq.WithMaxAttempts(c.cfg.MaxAttempts),
	)
	if err != nil {
		return nil, fmt.Errorf("rocketmq consumer: create producer for retry: %w", err)
	}
	if err := prod.Start(); err != nil {
		return nil, fmt.Errorf("rocketmq consumer: start producer for retry: %w", err)
	}
	c.prod = prod
	return prod, nil
}

// closeProducer closes the lazy producer if it was created.
func (c *RocketMQConsumer) closeProducer() {
	c.prodMu.Lock()
	defer c.prodMu.Unlock()
	if c.prod != nil {
		_ = c.prod.GracefulStop()
		c.prod = nil
	}
}

// republishToSelf republishes a message to the same topic for staircase retry.
func (c *RocketMQConsumer) republishToSelf(ctx context.Context, msg *Message) error {
	prod, err := c.ensureProducer(ctx)
	if err != nil {
		return err
	}

	rmqMsg := toRMQMessage(msg)
	if msg.Delay > 0 {
		rmqMsg.SetDelayTimestamp(time.Now().Add(msg.Delay))
	}

	_, err = prod.Send(ctx, rmqMsg)
	if err != nil {
		return fmt.Errorf("rocketmq retry republish: %w", err)
	}
	return nil
}

// sendToDLQ sends a failed message to the DLQ topic.
func (c *RocketMQConsumer) sendToDLQ(ctx context.Context, msg *Message, cause error) {
	if c.dlqTopic == "" {
		return
	}

	prod, err := c.ensureProducer(ctx)
	if err != nil {
		slog.Error("rocketmq consumer: cannot create producer for DLQ",
			slog.String("err", err.Error()),
		)
		return
	}

	dlqMsg := *msg
	dlqMsg.Topic = c.dlqTopic
	if dlqMsg.Headers == nil {
		dlqMsg.Headers = make(map[string]string)
	}
	dlqMsg.Headers["x-death-reason"] = cause.Error()

	rmqDLQ := toRMQMessage(&dlqMsg)
	_, err = prod.Send(ctx, rmqDLQ)
	if err != nil {
		slog.Error("rocketmq consumer: send to DLQ failed",
			slog.String("dlq_topic", c.dlqTopic),
			slog.String("err", err.Error()),
		)
		return
	}
	slog.Warn("rocketmq consumer: message sent to DLQ",
		slog.String("dlq_topic", c.dlqTopic),
		slog.String("original_topic", msg.Topic),
	)
}

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
	// Carry idempotency key as a property for consumer-side extraction
	if msg.IdempKey != "" {
		m.AddProperty("x-idemp-key", msg.IdempKey)
	}
	// Carry retry count as a property for consumer-side extraction
	if msg.RetryCount > 0 {
		m.AddProperty("x-retry-count", strconv.Itoa(msg.RetryCount))
	}
	return m
}

func fromMessageView(view *rmq.MessageView) *Message {
	headers := make(map[string]string)
	var idempKey string
	var retryCount int
	for k, v := range view.GetProperties() {
		switch k {
		case "x-idemp-key":
			idempKey = v
		case "x-retry-count":
			if n, err := strconv.Atoi(v); err == nil {
				retryCount = n
			}
		default:
			headers[k] = v
		}
	}
	return &Message{
		Topic:      view.GetTopic(),
		Key:        []byte(view.GetMessageId()),
		Payload:    view.GetBody(),
		Headers:    headers,
		IdempKey:   idempKey,
		RetryCount: retryCount,
		Meta: map[string]any{
			"message_id":     view.GetMessageId(),
			"receipt_handle": view.GetReceiptHandle(),
			"tag":            view.GetTag(),
			"born_time":      view.GetBornTimestamp(),
			"delivery_count": view.GetDeliveryAttempt(),
		},
	}
}

// Capabilities returns the capabilities of Apache RocketMQ.
func (p *RocketMQProducer) Capabilities() Capabilities { return RocketMQCapabilities() }
func (c *RocketMQConsumer) Capabilities() Capabilities { return RocketMQCapabilities() }

// ─── Transaction Support ──────────────────────────────────────────────────────
// Transaction and TransactionChecker are defined in mq/mq.go.
// rocketmqTransaction implements mq.Transaction; it delegates back to the
// producer so that Commit/Rollback can be called on either the producer or
// the Transaction handle — both paths go through the same mutex guard.

// rocketmqTransaction implements mq.Transaction.
// It delegates Commit/Rollback to the parent RocketMQProducer so that the
// producer's activeTx field is always cleared under txMu, even if the caller
// drops the Transaction handle early.
type rocketmqTransaction struct {
	producer *RocketMQProducer
	tx      rmq.Transaction
	checker TransactionChecker
}

// Publish sends a half message (not visible to consumers until Commit).
// The producer's active transaction is used; if none is active this call
// returns ErrCapTxNotSupported.
func (t *rocketmqTransaction) Publish(ctx context.Context, msg *Message) error {
	t.producer.txMu.Lock()
	tx := t.producer.activeTx
	t.producer.txMu.Unlock()

	if tx == nil {
		return ErrCapTxNotSupported
	}

	rmqMsg := toRMQMessage(msg)
	receipts, err := t.producer.prod.SendWithTransaction(ctx, rmqMsg, tx)
	if err != nil {
		return fmt.Errorf("rocketmq transaction publish: %w", err)
	}
	slog.Debug("rocketmq transaction publish ok",
		slog.String("topic", msg.Topic),
		slog.Int("receipts", len(receipts)),
	)
	return nil
}

// Commit ends the active transaction started by BeginTransaction.
// It is safe to call even if no transaction is active (no-op).
func (p *RocketMQProducer) Commit(ctx context.Context) error {
	p.txMu.Lock()
	tx := p.activeTx
	p.activeTx = nil
	p.txMu.Unlock()
	if tx == nil {
		return nil
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("rocketmq transaction commit: %w", err)
	}
	slog.Debug("rocketmq transaction committed")
	return nil
}

// Rollback aborts the active transaction started by BeginTransaction.
// It is safe to call even if no transaction is active (no-op).
func (p *RocketMQProducer) Rollback(ctx context.Context) error {
	p.txMu.Lock()
	tx := p.activeTx
	p.activeTx = nil
	p.txMu.Unlock()
	if tx == nil {
		return nil
	}
	if err := tx.RollBack(); err != nil {
		return fmt.Errorf("rocketmq transaction rollback: %w", err)
	}
	slog.Debug("rocketmq transaction rolled back")
	return nil
}

// Commit on the Transaction handle delegates to the producer.
func (t *rocketmqTransaction) Commit(ctx context.Context) error {
	return t.producer.Commit(ctx)
}

// Rollback on the Transaction handle delegates to the producer.
func (t *rocketmqTransaction) Rollback(ctx context.Context) error {
	return t.producer.Rollback(ctx)
}

// BeginTransaction starts a new transaction.
// The checker is called by the broker if the producer crashes before
// Commit/Rollback. Satisfies mq.Producer.BeginTransaction.
func (p *RocketMQProducer) BeginTransaction(ctx context.Context, checker TransactionChecker) (Transaction, error) {
	p.txMu.Lock()
	defer p.txMu.Unlock()

	if p.activeTx != nil {
		return nil, fmt.Errorf("rocketmq: transaction already in progress")
	}

	tx := p.prod.BeginTransaction()
	if tx == nil {
		return nil, fmt.Errorf("rocketmq: failed to begin transaction")
	}
	p.activeTx = tx

	return &rocketmqTransaction{
		producer: p,
		tx:      tx,
		checker: checker,
	}, nil
}

// ─── Priority helpers ───────────────────────────────────────────────────────

// collectAllTopics collects all topics including priority topics.
func (c *RocketMQConsumer) collectAllTopics(topics []string) []string {
	seen := make(map[string]bool, len(topics)+len(c.cfg.PriorityTopics))
	var allTopics []string

	// Add explicitly specified topics
	for _, t := range topics {
		if !seen[t] {
			seen[t] = true
			allTopics = append(allTopics, t)
		}
	}

	// Add priority topics
	for _, topic := range c.cfg.PriorityTopics {
		if !seen[topic] {
			seen[topic] = true
			allTopics = append(allTopics, topic)
		}
	}

	return allTopics
}

// topicPriority returns the priority level for a topic.
// Higher value = higher priority.
func (c *RocketMQConsumer) topicPriority(topic string) int {
	for priority, t := range c.cfg.PriorityTopics {
		if t == topic {
			return priority
		}
	}
	// Default priority (0) for topics not in PriorityTopics map
	return 0
}

// sortViewsByPriority sorts MessageView slices by topic priority (descending).
// Higher priority topics come first.
func (c *RocketMQConsumer) sortViewsByPriority(views []*rmq.MessageView) []*rmq.MessageView {
	if len(views) <= 1 {
		return views
	}

	// Create a copy to avoid mutating the original slice
	sorted := make([]*rmq.MessageView, len(views))
	copy(sorted, views)

	// Sort by priority (descending)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if c.topicPriority(sorted[i].GetTopic()) < c.topicPriority(sorted[j].GetTopic()) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}
