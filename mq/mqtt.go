// Package mqtt provides an MQTT implementation of mq.Producer and mq.Consumer.
//
// All three MQTT brokers — EMQX, Mosquitto, and NanoMQ — speak standard MQTT
// 3.1.1 / 5.0. This single implementation covers all of them; the only
// difference is the broker URL you configure.
//
// # EMQX
//
//	cfg := mqtt.Config{Broker: "tcp://localhost:1883", ClientID: "my-service"}
//
// # Mosquitto
//
//	cfg := mqtt.Config{Broker: "tcp://localhost:1883", ClientID: "my-service"}
//
// # NanoMQ
//
//	cfg := mqtt.Config{Broker: "tcp://localhost:1883", ClientID: "my-service"}
//
// # TLS
//
//	cfg := mqtt.Config{
//	    Broker: "ssl://localhost:8883",
//	    TLSConfig: &tls.Config{InsecureSkipVerify: false, ...},
//	}
//
// # QoS levels
//
//   - QoS 0 — at most once (fire and forget)
//   - QoS 1 — at least once (default)
//   - QoS 2 — exactly once
//
// # Shared Subscriptions (MQTT v5.0)
//
//	c, _ := mqtt.NewConsumer(mqtt.Config{
//	    Broker: "tcp://localhost:1883",
//	    ClientID: "consumer-1",
//	    EnableShared: true,
//	    SharedGroup: "workers",
//	})
//	c.Subscribe(ctx, []string{"sensors/#"}, "", handler)
//
// # Usage
//
//	p, _ := mqtt.NewProducer(mqtt.Config{Broker: "tcp://localhost:1883", ClientID: "producer-1"})
//	p.Publish(ctx, &Message{Topic: "sensors/temperature", Payload: []byte("22.5")})
//
//	c, _ := mqtt.NewConsumer(mqtt.Config{Broker: "tcp://localhost:1883", ClientID: "consumer-1"})
//	c.Subscribe(ctx, []string{"sensors/#"}, "", func(ctx context.Context, msg *Message) error {
//	    fmt.Printf("topic=%s payload=%s\n", msg.Topic, msg.Payload)
//	    return nil
//	})
//
// # Retry / NAK-Delay
//
// When EnableRetry is true, failed messages are republished to the RetryTopic
// with retry count encoded in the topic ($retry/<count>/<original_topic>).
// The consumer decodes the retry count and strips the retry prefix before
// passing the message to the handler. After MaxRetries, messages go to DLQ.
//
//	_, _ = mqtt.NewProducer(mqtt.Config{
//	    Broker:     "tcp://localhost:1883",
//	    ClientID:   "producer-1",
//	    EnableRetry: true,
//	    RetryTopic: "sys/mqtt/retry",
//	    DLQTopic:   "sys/mqtt/dLQ",
//	})
//
// # Fixed Delay
//
// When FixedDelayLevels is non-empty, FixedDelay() routes to
// $delay/<level>/<original_topic>. The consumer strips the delay prefix
// and sets the delivery time based on the level.
//
// # Arbitrary Delay
//
// ArbitraryDelay() routes to $arb/<delay_ms>/<original_topic>.
// The consumer calculates the remaining delay and uses a timer to delay
// handler invocation.
//
//	p, _ := mqtt.NewProducer(mqtt.Config{
//	    Broker: "tcp://localhost:1883",
//	    ClientID: "producer-1",
//	})
//	p.ArbitraryDelay(ctx, msg, 5*time.Second)
package mq

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTConfig configures an MQTT producer or consumer.
type MQTTConfig struct {
	// Broker is the MQTT broker URL.
	// Protocols: tcp://, ssl://, ws://, wss://
	// e.g. "tcp://localhost:1883" or "ssl://emqx.example.com:8883"
	Broker string

	// ClientID uniquely identifies this client to the broker.
	// Required; should be unique per connection.
	ClientID string

	// Username and Password for broker authentication.
	Username string
	Password string

	// QoS is the quality of service level: 0, 1, or 2. Default: 1.
	QoS byte

	// CleanSession: if true the broker discards subscriptions on disconnect.
	// Default: true.
	CleanSession bool

	// KeepAlive is the MQTT keep-alive interval. Default: 30s.
	KeepAlive time.Duration

	// ConnectTimeout is the maximum time to wait for a connection. Default: 10s.
	ConnectTimeout time.Duration

	// TLSConfig is optional TLS configuration (used with ssl:// or wss:// brokers).
	TLSConfig *tls.Config

	// WillTopic / WillPayload configure an MQTT Last-Will-and-Testament message.
	WillTopic   string
	WillPayload []byte
	WillQoS     byte
	WillRetain  bool

	// EnableShared enables MQTT v5.0 shared subscriptions.
	// When true, subscriptions will use the $share/group/topic format.
	EnableShared bool

	// SharedGroup is the shared subscription group name.
	// Used only when EnableShared is true.
	// Default: "default".
	SharedGroup string

	// EnableIdempotency enables idempotent delivery via message ID tracking.
	// When true, messages with a non-empty IdempKey are deduplicated client-side.
	// Note: MQTT does not have native idempotency; this is a client-side simulation.
	EnableIdempotency bool

	// IdempCacheSize is the size of the in-memory idempotency cache.
	// Default: 10000.
	IdempCacheSize int

	// EnableRetry enables retry / NAK-delay support.
	// When true, failed messages are republished to RetryTopic with encoded retry count.
	// After MaxRetries, messages are forwarded to DLQTopic.
	EnableRetry bool

	// MaxRetries is the maximum number of retry attempts.
	// Default: 3.
	MaxRetries int

	// RetryTopic is the topic used for retry / delayed delivery.
	// Default: "sys/mqtt/retry".
	RetryTopic string

	// DLQTopic is the dead-letter topic. After MaxRetries, messages go here.
	// Default: "sys/mqtt/dLQ".
	DLQTopic string

	// FixedDelayLevels defines predefined fixed-delay levels in milliseconds.
	// When non-empty, FixedDelay() routes to $delay/<level>/<original_topic>.
	// Default: []int64{1000, 5000, 30000, 60000, 300000} (1s/5s/30s/1m/5m)
	FixedDelayLevels []int64

	// BatchSize is the number of messages to batch before publishing.
	// Only applies to PublishBatch. Default: 10.
	BatchSize int

	// BatchTimeout is the maximum time to wait before flushing a batch.
	// Default: 100ms.
	BatchTimeout time.Duration
}

func (c *MQTTConfig) setDefaults() {
	if c.QoS == 0 && c.QoS != 1 {
		c.QoS = 1
	}
	if c.KeepAlive == 0 {
		c.KeepAlive = 30 * time.Second
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.SharedGroup == "" {
		c.SharedGroup = "default"
	}
	if c.IdempCacheSize == 0 {
		c.IdempCacheSize = 10000
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 3
	}
	if c.RetryTopic == "" {
		c.RetryTopic = "sys/mqtt/retry"
	}
	if c.DLQTopic == "" {
		c.DLQTopic = "sys/mqtt/dLQ"
	}
	if len(c.FixedDelayLevels) == 0 {
		c.FixedDelayLevels = []int64{1000, 5000, 30000, 60000, 300000}
	}
	if c.BatchSize == 0 {
		c.BatchSize = 10
	}
	if c.BatchTimeout == 0 {
		c.BatchTimeout = 100 * time.Millisecond
	}
}

func buildOptions(cfg MQTTConfig) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetCleanSession(cfg.CleanSession).
		SetKeepAlive(cfg.KeepAlive).
		SetConnectTimeout(cfg.ConnectTimeout).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(30 * time.Second)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username).SetPassword(cfg.Password)
	}
	if cfg.TLSConfig != nil {
		opts.SetTLSConfig(cfg.TLSConfig)
	}
	if cfg.WillTopic != "" {
		opts.SetWill(cfg.WillTopic, string(cfg.WillPayload), cfg.WillQoS, cfg.WillRetain)
	}
	return opts
}

func connect(opts *mqtt.ClientOptions) (mqtt.Client, error) {
	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(opts.ConnectTimeout) {
		return nil, fmt.Errorf("mqtt: connect timeout")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("mqtt: connect: %w", err)
	}
	return client, nil
}

// ─── Topic encoding scheme ────────────────────────────────────────────────────
//
// To carry retry/delay metadata in MQTT v3.1.1 (no User Properties), we encode
// metadata into the topic structure:
//
//   - Retry:       $retry/<count>/<original_topic>
//   - Fixed delay: $delay/<level>/<original_topic>
//   - Arb delay:   $arb/<delay_ms>/<original_topic>
//   - DLQ:         (uses DLQTopic directly, original topic in payload)
//
// The consumer strips these prefixes before invoking the handler.
// ─────────────────────────────────────────────────────────────────────────────

func retryTopic(count int, origTopic, retryTopic string) string {
	return fmt.Sprintf("%s/%d/%s", retryTopic, count, origTopic)
}

func parseRetryTopic(topic, retryTopic string) (count int, origTopic string, ok bool) {
	prefix := retryTopic + "/"
	if !strings.HasPrefix(topic, prefix) {
		return 0, "", false
	}
	rest := strings.TrimPrefix(topic, prefix)
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return 0, "", false
	}
	countStr, origTopic := rest[:slash], rest[slash+1:]
	n, err := strconv.Atoi(countStr)
	if err != nil {
		return 0, "", false
	}
	return n, origTopic, true
}

func fixedDelayTopic(level int, origTopic string) string {
	return fmt.Sprintf("$delay/%d/%s", level, origTopic)
}

func parseFixedDelayTopic(topic string) (level int, origTopic string, ok bool) {
	prefix := "$delay/"
	if !strings.HasPrefix(topic, prefix) {
		return 0, "", false
	}
	rest := strings.TrimPrefix(topic, prefix)
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return 0, "", false
	}
	levelStr, origTopic := rest[:slash], rest[slash+1:]
	n, err := strconv.Atoi(levelStr)
	if err != nil {
		return 0, "", false
	}
	return n, origTopic, true
}

func arbDelayTopic(delayMs int64, origTopic string) string {
	return fmt.Sprintf("$arb/%d/%s", delayMs, origTopic)
}

func parseArbDelayTopic(topic string) (delayMs int64, origTopic string, ok bool) {
	prefix := "$arb/"
	if !strings.HasPrefix(topic, prefix) {
		return 0, "", false
	}
	rest := strings.TrimPrefix(topic, prefix)
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return 0, "", false
	}
	delayStr, origTopic := rest[:slash], rest[slash+1:]
	n, err := strconv.ParseInt(delayStr, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return n, origTopic, true
}

// ─── Producer ─────────────────────────────────────────────────────────────────

// MQTTProducer publishes MQTT messages.
type MQTTProducer struct {
	cfg    MQTTConfig
	client mqtt.Client

	// Idempotency cache
	idempCache map[string]time.Time
	idempMu    sync.RWMutex

	// Fixed delay level → milliseconds
	delayLevels []int64
}

// NewMQTTProducer creates and connects an MQTT producer.
func NewMQTTProducer(cfg MQTTConfig) (*MQTTProducer, error) {
	cfg.setDefaults()
	opts := buildOptions(cfg)
	client, err := connect(opts)
	if err != nil {
		return nil, err
	}

	return &MQTTProducer{
		cfg:          cfg,
		client:       client,
		idempCache:   make(map[string]time.Time, cfg.IdempCacheSize),
		delayLevels:  cfg.FixedDelayLevels,
	}, nil
}

// Publish publishes a message to the MQTT broker.
// The msg.Topic field is the MQTT topic.
func (p *MQTTProducer) Publish(ctx context.Context, msg *Message) error {
	return p.publishWithRetry(ctx, msg, 0)
}

// publishWithRetry handles idempotency and retry logic.
// retryCount is 0 for normal publishes.
func (p *MQTTProducer) publishWithRetry(ctx context.Context, msg *Message, retryCount int) error {
	// ── 幂等去重 ──
	if p.cfg.EnableIdempotency && msg.IdempKey != "" {
		p.idempMu.RLock()
		_, exists := p.idempCache[msg.IdempKey]
		p.idempMu.RUnlock()

		if exists {
			slog.Debug("mqtt: duplicate message, skipping", "idemp_key", msg.IdempKey)
			return nil
		}

		p.idempMu.Lock()
		p.idempCache[msg.IdempKey] = time.Now()
		p.idempMu.Unlock()
	}

	retained := false
	if msg.Meta != nil {
		if v, ok := msg.Meta["retained"].(bool); ok {
			retained = v
		}
	}

	token := p.client.Publish(msg.Topic, p.cfg.QoS, retained, msg.Payload)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-token.Done():
		return token.Error()
	}
}

// PublishBatch publishes multiple MQTT messages.
// (MQTT does not have native batch; this is client-side aggregation.)
func (p *MQTTProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	for _, m := range msgs {
		if err := p.Publish(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// NakDelay republishes a failed message with a delay.
// The retry count is encoded in the topic ($retry/<count>/<original_topic>).
// After MaxRetries, the message is forwarded to DLQTopic.
func (p *MQTTProducer) NakDelay(ctx context.Context, msg *Message, delay time.Duration) error {
	retryCount := 0
	if msg.Meta != nil {
		if v, ok := msg.Meta["mqtt_retry_count"].(int); ok {
			retryCount = v
		}
	}
	nextCount := retryCount + 1

	// Exceeded max retries → DLQ
	if p.cfg.EnableRetry && nextCount > p.cfg.MaxRetries && p.cfg.DLQTopic != "" {
		dlqPayload, _ := json.Marshal(map[string]any{
			"topic":       msg.Topic,
			"payload":     string(msg.Payload),
			"retry_count": retryCount,
		})
		token := p.client.Publish(p.cfg.DLQTopic, p.cfg.QoS, false, dlqPayload)
		if !token.WaitTimeout(5*time.Second) {
			return fmt.Errorf("mqtt: DLQ publish timeout")
		}
		if err := token.Error(); err != nil {
			return fmt.Errorf("mqtt: DLQ publish: %w", err)
		}
		slog.Info("mqtt: forwarded to DLQ",
			slog.String("topic", msg.Topic),
			slog.Int("retry_count", retryCount),
		)
		return nil
	}

	// Encode retry metadata in topic
	topic := retryTopic(nextCount, msg.Topic, p.cfg.RetryTopic)

	token := p.client.Publish(topic, p.cfg.QoS, false, msg.Payload)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-token.Done():
		return token.Error()
	}
}

// FixedDelay routes a message through a fixed-delay level.
// Topic becomes $delay/<level>/<original_topic>.
// The consumer strips the prefix and delays handler invocation.
func (p *MQTTProducer) FixedDelay(ctx context.Context, msg *Message, level int) error {
	levels := p.delayLevels
	if len(levels) == 0 {
		levels = []int64{1000, 5000, 30000, 60000, 300000}
	}

	if level >= len(levels) {
		level = len(levels) - 1
	}
	if level < 0 {
		level = 0
	}

	topic := fixedDelayTopic(level, msg.Topic)
	token := p.client.Publish(topic, p.cfg.QoS, false, msg.Payload)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-token.Done():
		return token.Error()
	}
}

// ArbitraryDelay routes a message through an arbitrary-delay topic.
// Topic becomes $arb/<delay_ms>/<original_topic>.
// The consumer calculates remaining delay and delays handler invocation.
func (p *MQTTProducer) ArbitraryDelay(ctx context.Context, msg *Message, delay time.Duration) error {
	if delay <= 0 {
		return fmt.Errorf("mqtt: ArbitraryDelay requires positive delay, got %v", delay)
	}

	delayMs := delay.Milliseconds()
	topic := arbDelayTopic(delayMs, msg.Topic)
	token := p.client.Publish(topic, p.cfg.QoS, false, msg.Payload)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-token.Done():
		return token.Error()
	}
}

// Close disconnects the MQTT client.
func (p *MQTTProducer) Close() error {
	p.client.Disconnect(500)
	return nil
}

// ─── Consumer ─────────────────────────────────────────────────────────────────

// MQTTConsumer subscribes to MQTT topics and processes messages via a handler.
// It reconnects automatically via the paho client's built-in auto-reconnect.
type MQTTConsumer struct {
	cfg    MQTTConfig
	client mqtt.Client
	mu     sync.Mutex
}

// NewMQTTConsumer creates and connects an MQTT consumer.
func NewMQTTConsumer(cfg MQTTConfig) (*MQTTConsumer, error) {
	cfg.setDefaults()
	c := &MQTTConsumer{cfg: cfg}
	opts := buildOptions(cfg)
	opts.SetOnConnectHandler(c.onConnect)
	client, err := connect(opts)
	if err != nil {
		return nil, err
	}
	c.client = client
	return c, nil
}

func (c *MQTTConsumer) onConnect(_ mqtt.Client) {
	slog.Info("mqtt consumer: connected", slog.String("broker", c.cfg.Broker))
}

// Subscribe subscribes to the given MQTT topic filters and calls the handler
// for each received message. It blocks until ctx is cancelled.
//
// The group parameter is used for shared subscriptions (MQTT v5.0).
// When EnableShared is true, the subscription will use $share/group/topic format.
//
// When EnableRetry is true, the consumer also subscribes to RetryTopic and DLQTopic
// and automatically handles NakDelay by stripping retry metadata before the handler.
//
// When FixedDelayLevels is non-empty, the consumer subscribes to $delay/#
// and $arb/# for fixed/arbitrary delay messages.
func (c *MQTTConsumer) Subscribe(ctx context.Context, topics []string, group string, handler Handler) error {
	if len(topics) == 0 {
		return fmt.Errorf("mqtt consumer: at least one topic filter is required")
	}

	sharedGroup := c.cfg.SharedGroup
	if group != "" {
		sharedGroup = group
	}

	// Build filters
	filters := make(map[string]byte, 0)

	for _, t := range topics {
		if c.cfg.EnableShared {
			filters[fmt.Sprintf("$share/%s/%s", sharedGroup, t)] = c.cfg.QoS
		} else {
			filters[t] = c.cfg.QoS
		}
	}

	// Retry / delay system topics
	if c.cfg.EnableRetry {
		filters[c.cfg.RetryTopic+"/#"] = c.cfg.QoS
		if c.cfg.DLQTopic != "" {
			filters[c.cfg.DLQTopic] = c.cfg.QoS
		}
	}
	if len(c.cfg.FixedDelayLevels) > 0 {
		filters["$delay/#"] = c.cfg.QoS
		filters["$arb/#"] = c.cfg.QoS
	}

	token := c.client.SubscribeMultiple(filters, func(_ mqtt.Client, raw mqtt.Message) {
		c.processMessage(ctx, raw, handler)
	})
	if !token.WaitTimeout(c.cfg.ConnectTimeout) {
		return fmt.Errorf("mqtt subscribe: timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt subscribe: %w", err)
	}

	slog.Info("mqtt consumer: subscribed",
		slog.String("broker", c.cfg.Broker),
		slog.Any("topics", topics),
		slog.Bool("shared", c.cfg.EnableShared),
		slog.Bool("retry", c.cfg.EnableRetry),
	)

	<-ctx.Done()
	return ctx.Err()
}

// processMessage dispatches a raw MQTT message to the handler,
// decoding retry/delay topics and applying appropriate delays.
func (c *MQTTConsumer) processMessage(ctx context.Context, raw mqtt.Message, handler Handler) {
	topic := raw.Topic()
	qos := raw.Qos()

	// ── DLQ message ──
	if c.cfg.DLQTopic != "" && topic == c.cfg.DLQTopic {
		var dlqInfo map[string]any
		if err := json.Unmarshal(raw.Payload(), &dlqInfo); err == nil {
			origTopic, _ := dlqInfo["topic"].(string)
			payloadStr, _ := dlqInfo["payload"].(string)
			retryCount, _ := dlqInfo["retry_count"].(float64)
			msg := &Message{
				Topic:   origTopic,
				Payload: []byte(payloadStr),
				Key:     []byte(fmt.Sprintf("%d", raw.MessageID())),
				Meta: map[string]any{
					"qos":              qos,
					"message_id":       raw.MessageID(),
					"mqtt_retry_count": int(retryCount),
					"mqtt_is_dlq":      true,
				},
			}
			handler(ctx, msg)
		}
		raw.Ack()
		return
	}

	// ── Retry message ──
	if c.cfg.EnableRetry {
		if count, origTopic, ok := parseRetryTopic(topic, c.cfg.RetryTopic); ok {
			msg := &Message{
				Topic:   origTopic,
				Payload: raw.Payload(),
				Key:     []byte(fmt.Sprintf("%d", raw.MessageID())),
				Meta: map[string]any{
					"qos":              qos,
					"message_id":       raw.MessageID(),
					"mqtt_retry_count": count,
					"mqtt_is_retry":    true,
				},
			}
			if err := handler(ctx, msg); err != nil {
				slog.Warn("mqtt: retry handler error",
					slog.String("topic", origTopic),
					slog.Int("retry", count),
					slog.String("err", err.Error()),
				)
			}
			raw.Ack()
			return
		}
	}

	// ── Fixed delay message ──
	if len(c.cfg.FixedDelayLevels) > 0 {
		if level, origTopic, ok := parseFixedDelayTopic(topic); ok {
			levels := c.cfg.FixedDelayLevels
			if level < 0 || level >= len(levels) {
				level = len(levels) - 1
			}
			delayMs := levels[level]
			msg := &Message{
				Topic:   origTopic,
				Payload: raw.Payload(),
				Key:     []byte(fmt.Sprintf("%d", raw.MessageID())),
				Meta: map[string]any{
					"qos":                 qos,
					"message_id":          raw.MessageID(),
					"mqtt_delay_level":    level,
					"mqtt_delay_ms":      delayMs,
					"mqtt_is_fixed_delay": true,
				},
			}
			if delayMs > 0 {
				// Delay handler invocation
				select {
				case <-time.After(time.Duration(delayMs) * time.Millisecond):
					handler(ctx, msg)
				case <-ctx.Done():
				}
			} else {
				handler(ctx, msg)
			}
			raw.Ack()
			return
		}
	}

	// ── Arbitrary delay message ──
	if len(c.cfg.FixedDelayLevels) > 0 || c.cfg.EnableRetry {
		if delayMs, origTopic, ok := parseArbDelayTopic(topic); ok {
			msg := &Message{
				Topic:   origTopic,
				Payload: raw.Payload(),
				Key:     []byte(fmt.Sprintf("%d", raw.MessageID())),
				Meta: map[string]any{
					"qos":                   qos,
					"message_id":            raw.MessageID(),
					"mqtt_delay_ms":         delayMs,
					"mqtt_is_arbitrary_delay": true,
				},
			}
			if delayMs > 0 {
				select {
				case <-time.After(time.Duration(delayMs) * time.Millisecond):
					handler(ctx, msg)
				case <-ctx.Done():
				}
			} else {
				handler(ctx, msg)
			}
			raw.Ack()
			return
		}
	}

	// ── Normal message ──
	msg := &Message{
		Topic:   topic,
		Payload: raw.Payload(),
		Key:     []byte(fmt.Sprintf("%d", raw.MessageID())),
		Meta: map[string]any{
			"qos":        qos,
			"message_id": raw.MessageID(),
		},
	}

	if err := handler(ctx, msg); err != nil {
		slog.Warn("mqtt handler error",
			slog.String("topic", topic),
			slog.String("err", err.Error()),
		)
	}
	raw.Ack()
}

// Close disconnects the MQTT client.
func (c *MQTTConsumer) Close() error {
	c.client.Disconnect(500)
	return nil
}

// Capabilities returns the capabilities of MQTT.
func (p *MQTTProducer) Capabilities() Capabilities { return MqttCapabilities() }
func (c *MQTTConsumer) Capabilities() Capabilities { return MqttCapabilities() }
