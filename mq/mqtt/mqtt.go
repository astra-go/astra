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
// # Usage
//
//	p, _ := mqtt.NewProducer(mqtt.Config{Broker: "tcp://localhost:1883", ClientID: "producer-1"})
//	p.Publish(ctx, &mq.Message{Topic: "sensors/temperature", Payload: []byte("22.5")})
//
//	c, _ := mqtt.NewConsumer(mqtt.Config{Broker: "tcp://localhost:1883", ClientID: "consumer-1"})
//	c.Subscribe(ctx, []string{"sensors/#"}, "", func(ctx context.Context, msg *mq.Message) error {
//	    fmt.Printf("topic=%s payload=%s\n", msg.Topic, msg.Payload)
//	    return nil
//	})
package mqtt

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/astra-go/astra/mq"
)

// Config configures an MQTT producer or consumer.
type Config struct {
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
}

func (c *Config) setDefaults() {
	if c.QoS == 0 && c.QoS != 1 {
		c.QoS = 1
	}
	if c.KeepAlive == 0 {
		c.KeepAlive = 30 * time.Second
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 10 * time.Second
	}
}

func buildOptions(cfg Config) *paho.ClientOptions {
	opts := paho.NewClientOptions().
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

func connect(opts *paho.ClientOptions) (paho.Client, error) {
	client := paho.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(opts.ConnectTimeout) {
		return nil, fmt.Errorf("mqtt: connect timeout")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("mqtt: connect: %w", err)
	}
	return client, nil
}

// ─── Producer ─────────────────────────────────────────────────────────────────

// Producer publishes MQTT messages.
type Producer struct {
	cfg    Config
	client paho.Client
}

// NewProducer creates and connects an MQTT producer.
func NewProducer(cfg Config) (*Producer, error) {
	cfg.setDefaults()
	opts := buildOptions(cfg)
	client, err := connect(opts)
	if err != nil {
		return nil, err
	}
	return &Producer{cfg: cfg, client: client}, nil
}

// Publish publishes a message to the MQTT broker.
// The msg.Topic field is the MQTT topic.
func (p *Producer) Publish(ctx context.Context, msg *mq.Message) error {
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

// PublishBatch publishes multiple MQTT messages sequentially.
func (p *Producer) PublishBatch(ctx context.Context, msgs []*mq.Message) error {
	for _, m := range msgs {
		if err := p.Publish(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// Close disconnects the MQTT client.
func (p *Producer) Close() error {
	p.client.Disconnect(500)
	return nil
}

// ─── Consumer ─────────────────────────────────────────────────────────────────

// Consumer subscribes to MQTT topics and processes messages via a handler.
// It reconnects automatically via the paho client's built-in auto-reconnect.
type Consumer struct {
	cfg    Config
	client paho.Client
	mu     sync.Mutex
}

// NewConsumer creates and connects an MQTT consumer.
func NewConsumer(cfg Config) (*Consumer, error) {
	cfg.setDefaults()
	c := &Consumer{cfg: cfg}
	opts := buildOptions(cfg)
	opts.SetOnConnectHandler(c.onConnect)
	client, err := connect(opts)
	if err != nil {
		return nil, err
	}
	c.client = client
	return c, nil
}

func (c *Consumer) onConnect(client paho.Client) {
	slog.Info("mqtt consumer: connected", slog.String("broker", c.cfg.Broker))
}

// Subscribe subscribes to the given MQTT topic filters and calls the handler
// for each received message. It blocks until ctx is cancelled.
//
// The group parameter is unused for MQTT (groups are not a native MQTT concept).
func (c *Consumer) Subscribe(ctx context.Context, topics []string, _ string, handler mq.Handler) error {
	if len(topics) == 0 {
		return fmt.Errorf("mqtt consumer: at least one topic filter is required")
	}

	// Build topic filter map with the configured QoS for each topic.
	filters := make(map[string]byte, len(topics))
	for _, t := range topics {
		filters[t] = c.cfg.QoS
	}

	token := c.client.SubscribeMultiple(filters, func(_ paho.Client, msg paho.Message) {
		m := &mq.Message{
			Topic:   msg.Topic(),
			Payload: msg.Payload(),
			Meta: map[string]any{
				"qos":       msg.Qos(),
				"retained":  msg.Retained(),
				"message_id": msg.MessageID(),
			},
		}
		if err := handler(ctx, m); err != nil {
			slog.Warn("mqtt handler error",
				slog.String("topic", msg.Topic()),
				slog.String("err", err.Error()),
			)
		}
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
	)

	<-ctx.Done()
	return ctx.Err()
}

// Close disconnects the MQTT client.
func (c *Consumer) Close() error {
	c.client.Disconnect(500)
	return nil
}
