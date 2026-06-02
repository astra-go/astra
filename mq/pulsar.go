// Package pulsar provides Apache Pulsar implementations of the mq.Producer and
// mq.Consumer interfaces.
//
// Pulsar supports both At-Most-Once (non-persistent topics) and At-Least-Once
// delivery with acknowledgement. All subscription types are supported:
// Exclusive, Shared, Failover, and KeyShared.
//
// # Producer
//
//	p, err := pulsar.NewProducer(pulsar.Config{URL: "pulsar://localhost:6650"})
//	defer p.Close()
//	p.Publish(ctx, &Message{Topic: "persistent://public/default/orders", Payload: body})
//
// # Consumer (Shared subscription)
//
//	c, err := pulsar.NewConsumer(pulsar.ConsumerConfig{
//	    Config:           pulsar.Config{URL: "pulsar://localhost:6650"},
//	    Subscription:     "order-workers",
//	    SubscriptionType: gopulsar.Shared,
//	})
//	c.Subscribe(ctx, []string{"persistent://public/default/orders"}, "", handler)
//
// # Authentication
//
//	p, err := pulsar.NewProducer(pulsar.Config{
//	    URL:       "pulsar+ssl://broker:6651",
//	    AuthToken: os.Getenv("PULSAR_TOKEN"),
//	    TLSCertFile: "/etc/pulsar/certs/ca.cert.pem",
//	})
package mq

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	gopulsar "github.com/apache/pulsar-client-go/pulsar"
)

// Config configures a Pulsar client connection.
type PulsarConfig struct {
	// URL is the Pulsar broker URL.
	// Default: "pulsar://localhost:6650"
	URL string

	// AuthToken is the JWT token for Pulsar token authentication.
	AuthToken string

	// TLSCertFile is the path to the TLS CA certificate for TLS connections.
	TLSCertFile string

	// OperationTimeout is the timeout for producer/consumer operations.
	// Default: 5s.
	OperationTimeout time.Duration
}

func (c *PulsarConfig) clientOptions() gopulsar.ClientOptions {
	url := c.URL
	if url == "" {
		url = "pulsar://localhost:6650"
	}
	timeout := c.OperationTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	opts := gopulsar.ClientOptions{
		URL:               url,
		OperationTimeout:  timeout,
		ConnectionTimeout: timeout,
	}
	if c.AuthToken != "" {
		opts.Authentication = gopulsar.NewAuthenticationToken(c.AuthToken)
	}
	if c.TLSCertFile != "" {
		opts.TLSTrustCertsFilePath = c.TLSCertFile
		opts.TLSAllowInsecureConnection = false
	}
	return opts
}

// ─── Producer ─────────────────────────────────────────────────────────────────

// Producer publishes messages to Apache Pulsar.
type PulsarProducer struct {
	client    gopulsar.Client
	producers map[string]gopulsar.Producer // topic → producer (lazy init)
	cfg       PulsarConfig
}

// NewProducer creates a Pulsar Producer.
func NewPulsarProducer(cfg PulsarConfig) (*PulsarProducer, error) {
	client, err := gopulsar.NewClient(cfg.clientOptions())
	if err != nil {
		return nil, fmt.Errorf("pulsar: new client: %w", err)
	}
	return &PulsarProducer{
		client:    client,
		producers: make(map[string]gopulsar.Producer),
		cfg:       cfg,
	}, nil
}

// Publish sends a message to the given topic.
func (p *PulsarProducer) Publish(ctx context.Context, msg *Message) error {
	prod, err := p.getProducer(msg.Topic)
	if err != nil {
		return err
	}

	pm := &gopulsar.ProducerMessage{
		Payload: msg.Payload,
	}
	if len(msg.Key) > 0 {
		pm.Key = string(msg.Key)
	}
	for k, v := range msg.Headers {
		pm.Properties = map[string]string{}
		pm.Properties[k] = v
	}

	_, err = prod.Send(ctx, pm)
	if err != nil {
		return fmt.Errorf("pulsar: publish to %s: %w", msg.Topic, err)
	}
	return nil
}

// PublishBatch sends multiple messages.
func (p *PulsarProducer) PublishBatch(ctx context.Context, msgs []*Message) error {
	for _, msg := range msgs {
		if err := p.Publish(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// Close closes all producers and the client connection.
func (p *PulsarProducer) Close() error {
	for _, prod := range p.producers {
		prod.Close()
	}
	p.client.Close()
	return nil
}

func (p *PulsarProducer) getProducer(topic string) (gopulsar.Producer, error) {
	if prod, ok := p.producers[topic]; ok {
		return prod, nil
	}
	prod, err := p.client.CreateProducer(gopulsar.ProducerOptions{
		Topic: topic,
	})
	if err != nil {
		return nil, fmt.Errorf("pulsar: create producer for %s: %w", topic, err)
	}
	p.producers[topic] = prod
	return prod, nil
}

// Compile-time assertion.
var _ Producer = (*PulsarProducer)(nil)

// ─── Consumer ─────────────────────────────────────────────────────────────────

// ConsumerConfig configures a Pulsar consumer.
type PulsarConsumerConfig struct {
	PulsarConfig

	// Subscription is the Pulsar subscription name. Required.
	Subscription string

	// SubscriptionType controls how messages are distributed among consumers.
	// Options: Exclusive (default), Shared, Failover, KeyShared.
	SubscriptionType gopulsar.SubscriptionType

	// MaxPendingMessages limits the local queue of unconsumed messages.
	// Default: 1000.
	MaxPendingMessages int
}

// Consumer subscribes to Pulsar topics.
type PulsarConsumer struct {
	client gopulsar.Client
	cfg    PulsarConsumerConfig
}

// NewConsumer creates a Pulsar Consumer.
func NewPulsarConsumer(cfg PulsarConsumerConfig) (*PulsarConsumer, error) {
	client, err := gopulsar.NewClient(cfg.PulsarConfig.clientOptions())
	if err != nil {
		return nil, fmt.Errorf("pulsar: new client: %w", err)
	}
	return &PulsarConsumer{client: client, cfg: cfg}, nil
}

// Subscribe starts consuming messages from the given topics.
// Blocks until ctx is cancelled.
func (c *PulsarConsumer) Subscribe(ctx context.Context, topics []string, group string, handler Handler) error {
	if len(topics) == 0 {
		return fmt.Errorf("pulsar: at least one topic required")
	}
	if c.cfg.Subscription == "" && group == "" {
		return fmt.Errorf("pulsar: Subscription name is required")
	}
	sub := c.cfg.Subscription
	if group != "" {
		sub = group
	}

	maxPending := c.cfg.MaxPendingMessages
	if maxPending <= 0 {
		maxPending = 1000
	}

	consumer, err := c.client.Subscribe(gopulsar.ConsumerOptions{
		Topics:            topics,
		SubscriptionName:  sub,
		Type:              c.cfg.SubscriptionType,
		ReceiverQueueSize: maxPending,
	})
	if err != nil {
		return fmt.Errorf("pulsar: subscribe: %w", err)
	}
	defer consumer.Close()

	for {
		pMsg, err := consumer.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // context cancelled — clean shutdown
			}
			slog.Warn("pulsar: receive error", "err", err)
			continue
		}

		msg := &Message{
			Topic:   pMsg.Topic(),
			Payload: pMsg.Payload(),
		}
		if k := pMsg.Key(); k != "" {
			msg.Key = []byte(k)
		}
		if props := pMsg.Properties(); len(props) > 0 {
			msg.Headers = props
		}

		if err := handler(ctx, msg); err != nil {
			slog.Warn("pulsar: handler error", "topic", pMsg.Topic(), "err", err)
			consumer.Nack(pMsg)
		} else {
			consumer.Ack(pMsg)
		}
	}
}

// Close releases the client connection.
func (c *PulsarConsumer) Close() error {
	c.client.Close()
	return nil
}

// Compile-time assertion.
var _ Consumer = (*PulsarConsumer)(nil)
