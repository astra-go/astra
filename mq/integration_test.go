//go:build integration

// Package mq_test contains integration tests that require a running MQ broker.
//
// Run with:
//
//	go test -v -tags=integration -run 'Test.*Kafka.*' ./mq/        # Kafka only
//	go test -v -tags=integration -run 'Test.*RabbitMQ.*' ./mq/     # RabbitMQ only
//	go test -v -tags=integration -run 'Test.*Nats.*' ./mq/         # NATS only
//	go test -v -tags=integration -run 'Test.*Mqtt.*' ./mq/         # MQTT only
//	go test -v -tags=integration ./mq/                             # all
//
// Environment variables:
//
//	MQ_TEST_TYPE   - Optional filter: kafka, rabbitmq, nats, mqtt
//	KAFKA_BROKERS  - Kafka brokers (default: localhost:9092)
//	RABBITMQ_URL   - RabbitMQ URL (default: amqp://guest:guest@localhost:5672/)
//	NATS_URL       - NATS URL (default: nats://localhost:4222)
//	MQTT_URL       - MQTT broker URL (default: tcp://localhost:1883)
package mq_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/astra-go/astra/mq"
)

const (
	testTopic    = "astra-integration-test"
	testGroup    = "astra-test-group"
	testTimeout  = 15 * time.Second
	pollInterval = 100 * time.Millisecond
)

// ── Kafka ──────────────────────────────────────────────────────────────────

func TestKafkaPublishSubscribe(t *testing.T) {
	skipUnlessType(t, "kafka")

	brokers := brokersFromEnv("KAFKA_BROKERS", "localhost:9092")

	// Producer
	p, err := mq.NewKafkaProducer(mq.KafkaProducerConfig{Brokers: brokers})
	if err != nil {
		t.Fatalf("create Kafka producer: %v", err)
	}
	defer p.Close()

	// Consumer
	c, err := mq.NewKafkaConsumer(mq.KafkaConsumerConfig{
		Brokers: brokers,
		Group:   testGroup,
	})
	if err != nil {
		t.Fatalf("create Kafka consumer: %v", err)
	}
	defer c.Close()

	testPubSub(t, p, c)
}

func TestKafkaPublishBatch(t *testing.T) {
	skipUnlessType(t, "kafka")

	brokers := brokersFromEnv("KAFKA_BROKERS", "localhost:9092")

	p, err := mq.NewKafkaProducer(mq.KafkaProducerConfig{Brokers: brokers})
	if err != nil {
		t.Fatalf("create Kafka producer: %v", err)
	}
	defer p.Close()

	msgs := make([]*mq.Message, 5)
	for i := range msgs {
		msgs[i] = &mq.Message{
			Topic:   testTopic + "-batch",
			Key:     []byte("batch-key"),
			Payload: []byte(`{"batch":true,"index":` + string(rune('0'+i)) + `}`),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := p.PublishBatch(ctx, msgs); err != nil {
		t.Fatalf("PublishBatch: %v", err)
	}
}

// ── RabbitMQ ──────────────────────────────────────────────────────────────────

func TestRabbitMQPublishSubscribe(t *testing.T) {
	skipUnlessType(t, "rabbitmq")

	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}

	p, err := mq.NewRabbitMQProducer(mq.RabbitMQProducerConfig{URL: url})
	if err != nil {
		t.Fatalf("create RabbitMQ producer: %v", err)
	}
	defer p.Close()

	c, err := mq.NewRabbitMQConsumer(mq.RabbitMQConsumerConfig{
		URL:   url,
		Queue: "astra-test-queue",
	})
	if err != nil {
		t.Fatalf("create RabbitMQ consumer: %v", err)
	}
	defer c.Close()

	testPubSub(t, p, c)
}

func TestRabbitMQPublishBatch(t *testing.T) {
	skipUnlessType(t, "rabbitmq")

	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}

	p, err := mq.NewRabbitMQProducer(mq.RabbitMQProducerConfig{URL: url})
	if err != nil {
		t.Fatalf("create RabbitMQ producer: %v", err)
	}
	defer p.Close()

	msgs := make([]*mq.Message, 3)
	for i := range msgs {
		msgs[i] = &mq.Message{
			Topic:   "astra-test-batch",
			Payload: []byte(`{"rabbitmq_batch":true,"i":` + string(rune('0'+i)) + `}`),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := p.PublishBatch(ctx, msgs); err != nil {
		t.Fatalf("PublishBatch: %v", err)
	}
}

// ── NATS ─────────────────────────────────────────────────────────────────────

func TestNatsPublishSubscribe(t *testing.T) {
	skipUnlessType(t, "nats")

	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}

	p, err := mq.NewNatsProducer(mq.NatsProducerConfig{URL: url})
	if err != nil {
		t.Fatalf("create NATS producer: %v", err)
	}
	defer p.Close()

	c, err := mq.NewNatsConsumer(mq.NatsConsumerConfig{URL: url})
	if err != nil {
		t.Fatalf("create NATS consumer: %v", err)
	}
	defer c.Close()

	testPubSub(t, p, c)
}

// ── MQTT ─────────────────────────────────────────────────────────────────────

func TestMqttPublishSubscribe(t *testing.T) {
	skipUnlessType(t, "mqtt")

	url := os.Getenv("MQTT_URL")
	if url == "" {
		url = "tcp://localhost:1883"
	}

	p, err := mq.NewMqttProducer(mq.MqttProducerConfig{URL: url, ClientID: "astra-test-pub"})
	if err != nil {
		t.Fatalf("create MQTT producer: %v", err)
	}
	defer p.Close()

	c, err := mq.NewMqttConsumer(mq.MqttConsumerConfig{URL: url, ClientID: "astra-test-sub"})
	if err != nil {
		t.Fatalf("create MQTT consumer: %v", err)
	}
	defer c.Close()

	testPubSub(t, p, c)
}

// ── Shared helpers ──────────────────────────────────────────────────────────

// testPubSub is a generic pub/sub smoke test that publishes a message
// and verifies the consumer receives it within testTimeout.
func testPubSub(t *testing.T, p mq.Producer, c mq.Consumer) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	payload := []byte(`{"hello":"world","ts":` + string(rune(time.Now().UnixNano())) + `}`)

	// Start consumer in background
	received := make(chan *mq.Message, 1)
	go func() {
		subCtx, subCancel := context.WithCancel(ctx)
		defer subCancel()
		err := c.Subscribe(subCtx, []string{testTopic}, testGroup, func(_ context.Context, msg *mq.Message) error {
			select {
			case received <- msg:
			default:
			}
			return nil
		})
		if err != nil && err != context.Canceled {
			t.Logf("Subscribe ended: %v", err)
		}
	}()

	// Give consumer time to subscribe
	time.Sleep(500 * time.Millisecond)

	// Publish
	if err := p.Publish(ctx, &mq.Message{
		Topic:   testTopic,
		Key:     []byte("test-key"),
		Payload: payload,
		Headers: map[string]string{"test": "integration"},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Wait for delivery
	select {
	case msg := <-received:
		t.Logf("Received message: topic=%s key=%s payload=%s", msg.Topic, string(msg.Key), string(msg.Payload))
		if string(msg.Payload) != string(payload) {
			t.Errorf("payload mismatch: got %q, want %q", string(msg.Payload), string(payload))
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for message delivery")
	}
}

func skipUnlessType(t *testing.T, mqType string) {
	t.Helper()
	envType := os.Getenv("MQ_TEST_TYPE")
	if envType != "" && envType != "all" && envType != mqType {
		t.Skipf("Skipping %s test (MQ_TEST_TYPE=%s)", mqType, envType)
	}
}

func brokersFromEnv(envVar, defaultVal string) []string {
	if v := os.Getenv(envVar); v != "" {
		return []string{v}
	}
	return []string{defaultVal}
}
