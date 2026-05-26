// Package kafka provides a Kafka implementation of mq.Producer and mq.Consumer
// using the franz-go high-performance Kafka client.
//
// # Producer
//
//	p, err := kafka.NewProducer(kafka.ProducerConfig{
//	    Brokers: []string{"localhost:9092"},
//	})
//	defer p.Close()
//	p.Publish(ctx, &mq.Message{Topic: "events", Key: []byte("key"), Payload: body})
//
// # Consumer
//
//	c, err := kafka.NewConsumer(kafka.ConsumerConfig{
//	    Brokers: []string{"localhost:9092"},
//	    Group:   "my-service",
//	})
//	c.Subscribe(ctx, []string{"events"}, "my-service", handler)
//
// # Batching
//
// PublishBatch maps all messages in a single ProduceSync call for maximum
// throughput. Each message is added as a separate record; ordering is
// preserved within a topic-partition.
package kafka

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/astra-go/astra/mq"
)

// ─── Producer ─────────────────────────────────────────────────────────────────

// ProducerConfig configures a Kafka producer.
type ProducerConfig struct {
	// Brokers is a list of bootstrap broker addresses.
	Brokers []string

	// MaxMessageBytes is the maximum size of a single message. Default: 1 MiB.
	MaxMessageBytes int

	// ExtraOptions passes additional kgo.Opt values.
	ExtraOptions []kgo.Opt
}

// Producer publishes records to Kafka.
type Producer struct {
	client *kgo.Client
}

// NewProducer creates a Kafka producer.
func NewProducer(cfg ProducerConfig) (*Producer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka producer: at least one broker is required")
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.RecordRetries(5),
		kgo.ProducerBatchMaxBytes(int32(maxOrDefault(cfg.MaxMessageBytes, 1<<20))),
	}
	opts = append(opts, cfg.ExtraOptions...)

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka producer: create client: %w", err)
	}
	return &Producer{client: client}, nil
}

// Publish sends a single record synchronously.
func (p *Producer) Publish(ctx context.Context, msg *mq.Message) error {
	record := msgToRecord(msg)
	results := p.client.ProduceSync(ctx, record)
	return results.FirstErr()
}

// PublishBatch sends multiple records in a single ProduceSync call.
func (p *Producer) PublishBatch(ctx context.Context, msgs []*mq.Message) error {
	records := make([]*kgo.Record, len(msgs))
	for i, m := range msgs {
		records[i] = msgToRecord(m)
	}
	return p.client.ProduceSync(ctx, records...).FirstErr()
}

// Close flushes pending records and closes the client.
func (p *Producer) Close() error {
	p.client.Close()
	return nil
}

func msgToRecord(msg *mq.Message) *kgo.Record {
	r := &kgo.Record{
		Topic: msg.Topic,
		Value: msg.Payload,
		Key:   msg.Key,
	}
	for k, v := range msg.Headers {
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}
	return r
}

// ─── Consumer ─────────────────────────────────────────────────────────────────

// ConsumerConfig configures a Kafka consumer.
type ConsumerConfig struct {
	// Brokers is a list of bootstrap broker addresses.
	Brokers []string

	// Group is the consumer group ID.
	Group string

	// InitialOffset controls where the consumer starts on first connect.
	// kgo.NewOffset().AtStart() for earliest, kgo.NewOffset().AtEnd() for latest.
	// Default: latest.
	InitialOffset kgo.Offset

	// MaxPollRecords is the maximum number of records fetched per poll.
	// Default: 100.
	MaxPollRecords int

	// ExtraOptions passes additional kgo.Opt values.
	ExtraOptions []kgo.Opt
}

// Consumer subscribes to Kafka topics within a consumer group.
type Consumer struct {
	cfg    ConsumerConfig
	client *kgo.Client
}

// NewConsumer creates a Kafka consumer client (not yet connected to any topic).
func NewConsumer(cfg ConsumerConfig) (*Consumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka consumer: at least one broker is required")
	}
	return &Consumer{cfg: cfg}, nil
}

// Subscribe starts consuming from topics and calls handler for each record.
// It blocks until ctx is cancelled.
func (c *Consumer) Subscribe(ctx context.Context, topics []string, group string, handler mq.Handler) error {
	if group == "" {
		group = c.cfg.Group
	}
	if len(topics) == 0 {
		return fmt.Errorf("kafka consumer: at least one topic is required")
	}

	offset := c.cfg.InitialOffset
	// zero value of kgo.Offset is invalid; default to latest
	if offset == (kgo.Offset{}) {
		offset = kgo.NewOffset().AtEnd()
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(c.cfg.Brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(offset),
	}
	opts = append(opts, c.cfg.ExtraOptions...)

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return fmt.Errorf("kafka consumer: create client: %w", err)
	}
	c.client = client
	defer func() {
		client.Close()
		c.client = nil
	}()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		fetches := client.PollRecords(ctx, maxOrDefault(c.cfg.MaxPollRecords, 100))
		if fetches.IsClientClosed() {
			return nil
		}

		var retErr error
		fetches.EachError(func(topic string, partition int32, err error) {
			slog.Error("kafka fetch error",
				slog.String("topic", topic),
				slog.Int("partition", int(partition)),
				slog.String("err", err.Error()),
			)
			retErr = err
		})
		if retErr != nil {
			return retErr
		}

		fetches.EachRecord(func(r *kgo.Record) {
			msg := recordToMsg(r)
			if err := handler(ctx, msg); err != nil {
				slog.Warn("kafka handler error",
					slog.String("topic", r.Topic),
					slog.String("err", err.Error()),
				)
			}
		})

		// Commit all polled offsets.
		if err := client.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("kafka commit offsets error", slog.String("err", err.Error()))
		}
	}
}

// Close closes the underlying Kafka client.
func (c *Consumer) Close() error {
	if c.client != nil {
		c.client.Close()
	}
	return nil
}

func recordToMsg(r *kgo.Record) *mq.Message {
	headers := make(map[string]string, len(r.Headers))
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)
	}
	return &mq.Message{
		Topic:   r.Topic,
		Key:     r.Key,
		Payload: r.Value,
		Headers: headers,
		Meta: map[string]any{
			"partition": r.Partition,
			"offset":    r.Offset,
			"timestamp": r.Timestamp,
		},
	}
}

func maxOrDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
