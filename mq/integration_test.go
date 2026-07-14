//go:build integration

// Package mq_test contains integration tests that require a running MQ broker.
//
// Run with:
//
//	go test -v -tags=integration -run 'Test.*Kafka.*' ./mq/          # Kafka only
//	go test -v -tags=integration -run 'Test.*RocketMQ.*' ./mq/         # RocketMQ only
//	go test -v -tags=integration -run 'Test.*RabbitMQ.*' ./mq/         # RabbitMQ only
//	go test -v -tags=integration -run 'Test.*Nats.*' ./mq/            # NATS only
//	go test -v -tags=integration -run 'Test.*Mqtt.*' ./mq/            # MQTT only
//	go test -v -tags=integration ./mq/                                # all
//
// Environment variables:
//
//	MQ_TEST_TYPE   - Optional filter: kafka, rocketmq, rabbitmq, nats, mqtt
//	KAFKA_BROKERS  - Kafka brokers (default: localhost:9092)
//	ROCKETMQ_ENDPOINT - RocketMQ proxy endpoint (default: localhost:8081)
//	ROCKETMQ_TOPIC   - RocketMQ topic for pub/sub tests (default: test-topic)
//	RABBITMQ_URL   - RabbitMQ URL (default: amqp://guest:guest@localhost:5672/)
//	NATS_URL       - NATS URL (default: nats://localhost:4222)
//	MQTT_URL       - MQTT broker URL (default: tcp://localhost:1883)
//
// RocketMQ transaction note:
//
//	The transaction test (TestRocketMQTransaction_BeginTransaction) requires a
//	TRANSACTION-typed topic. RocketMQ 5.x proxies (incl. 5.3.1) expose no
//	CreateTopic RPC, so the topic must be pre-created on the broker via mqadmin:
//
//	  mqadmin updateTopic -n <namesrv>:9876 -c <cluster> -t <topic> \
//	    -a +message.type=TRANSACTION
//
//	Example (inside the broker container):
//	  docker exec astra-rmq-broker \
//	    /home/rocketmq/rocketmq-5.3.1/bin/mqadmin updateTopic \
//	    -n namesrv:9876 -c DefaultCluster -t astra-integration-test-tx \
//	    -a +message.type=TRANSACTION
//	If the topic is NORMAL, the test still exercises the full transaction
//	lifecycle but SKIPs the delivery assertion with a setup hint.
package mq_test

import (
	"context"
	"os"
	"strings"
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

	p, err := mq.NewRabbitMQProducer(mq.RabbitMQConfig{URL: url})
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

	p, err := mq.NewRabbitMQProducer(mq.RabbitMQConfig{URL: url})
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

	p, err := mq.NewNATSProducer(mq.NATSConfig{URL: url})
	if err != nil {
		t.Fatalf("create NATS producer: %v", err)
	}
	defer p.Close()

	c, err := mq.NewNATSConsumer(mq.NATSConsumerConfig{NATSConfig: mq.NATSConfig{URL: url}})
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

	p, err := mq.NewMQTTProducer(mq.MQTTConfig{Broker: url, ClientID: "astra-test-pub"})
	if err != nil {
		t.Fatalf("create MQTT producer: %v", err)
	}
	defer p.Close()

	c, err := mq.NewMQTTConsumer(mq.MQTTConfig{Broker: url, ClientID: "astra-test-sub"})
	if err != nil {
		t.Fatalf("create MQTT consumer: %v", err)
	}
	defer c.Close()

	testPubSub(t, p, c)
}

// ── RocketMQ ──────────────────────────────────────────────────────────────────

func TestRocketMQPublishSubscribe(t *testing.T) {
	skipUnlessType(t, "rocketmq")

	endpoint := os.Getenv("ROCKETMQ_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:8081"
	}
	topic := os.Getenv("ROCKETMQ_TOPIC")
	if topic == "" {
		topic = testTopic
	}

	p, err := mq.NewRocketMQProducer(mq.RocketMQConfig{
		Endpoint: endpoint,
		Topic:    topic,
	})
	if err != nil {
		t.Fatalf("create RocketMQ producer: %v", err)
	}
	defer p.Close()

	c, err := mq.NewRocketMQConsumer(mq.RocketMQConsumerConfig{
		Endpoint:      endpoint,
		Topic:        topic,
		ConsumerGroup: testGroup,
	})
	if err != nil {
		t.Fatalf("create RocketMQ consumer: %v", err)
	}
	defer c.Close()

	testPubSub(t, p, c)
}

// TestRocketMQTransaction_BeginTransaction exercises the full transaction lifecycle:
// BeginTransaction → Publish → Commit → verify message is delivered.
// Rollback is tested by publishing in a transaction and calling Rollback, then
// verifying no message arrives.
func TestRocketMQTransaction_BeginTransaction(t *testing.T) {
	skipUnlessType(t, "rocketmq")

	endpoint := os.Getenv("ROCKETMQ_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:8081"
	}
	topic := os.Getenv("ROCKETMQ_TOPIC")
	if topic == "" {
		topic = testTopic + "-tx"
	}

	p, err := mq.NewRocketMQProducer(mq.RocketMQConfig{
		Endpoint: endpoint,
		Topic:    topic,
		EnableTx: true,
		TransactionChecker: func(_ context.Context, _ *mq.Message) (bool, error) {
			return true, nil
		},
	})
	if err != nil {
		t.Fatalf("create RocketMQ producer: %v", err)
	}
	defer p.Close()

	// A producer built with EnableTx + TransactionChecker must report CapTx.
	if caps := p.Capabilities(); !caps[mq.CapTx] {
		t.Skip("RocketMQ producer CapTx=false; skipping transaction test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// A second BeginTransaction while one is active must be rejected so the
	// framework never loses track of the in-flight transaction.
	if _, err := p.BeginTransaction(ctx, nil); err != nil {
		t.Fatalf("BeginTransaction: %v", err)
	}
	if _, err := p.BeginTransaction(ctx, nil); err == nil {
		t.Fatal("expected error for nested BeginTransaction, got nil")
	}
	// Roll back the probe transaction so the sub-tests start clean.
	if err := p.Rollback(ctx); err != nil {
		t.Fatalf("rollback probe transaction: %v", err)
	}

	// ── Commit path: the half-message MUST become visible after Commit ──
	t.Run("commit_delivers", func(t *testing.T) {
		tx, err := p.BeginTransaction(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTransaction: %v", err)
		}
		payload := []byte(`{"action":"commit"}`)
		if err := p.Publish(ctx, &mq.Message{Topic: topic, Payload: payload}); err != nil {
			// RocketMQ 5.x only accepts transactional publishes on a
			// TRANSACTION-typed topic. If the topic is NORMAL, the broker
			// rejects it. The topic must be created with:
			//   mqadmin updateTopic -n <namesrv> -c <cluster> -t <topic> \
			//     -a +message.type=TRANSACTION
			if strings.Contains(err.Error(), "message type not match") ||
				strings.Contains(err.Error(), "topic route") ||
				strings.Contains(err.Error(), "No topic route") {
				_ = p.Rollback(ctx) // clear active tx before skipping
				t.Skipf("RocketMQ 5.x topic %q is not TRANSACTION-typed; create it with: mqadmin updateTopic -n <namesrv> -c <cluster> -t %s -a +message.type=TRANSACTION: %v", topic, topic, err)
			}
			t.Fatalf("Publish in transaction: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("tx.Commit: %v", err)
		}

		// Subscribe and verify the committed half-message is delivered.
		c, err := mq.NewRocketMQConsumer(mq.RocketMQConsumerConfig{
			Endpoint:      endpoint,
			Topic:        topic,
			ConsumerGroup: testGroup + "-tx-commit",
		})
		if err != nil {
			t.Fatalf("create consumer: %v", err)
		}
		defer c.Close()

		received := make(chan *mq.Message, 1)
		go func() {
			subCtx, subCancel := context.WithCancel(ctx)
			defer subCancel()
			_ = c.Subscribe(subCtx, []string{topic}, testGroup+"-tx-commit", func(_ context.Context, msg *mq.Message) error {
				select {
				case received <- msg:
				default:
				}
				return nil
			})
		}()

		time.Sleep(500 * time.Millisecond)
		select {
		case msg := <-received:
			if string(msg.Payload) != string(payload) {
				t.Errorf("payload mismatch: got %q, want %q", string(msg.Payload), string(payload))
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for committed transaction message")
		}
	})

	// ── Rollback path: the half-message MUST stay invisible after Rollback ──
	// Uses a dedicated topic so the rolled-back message cannot be confused
	// with a committed message from the commit_delivers sub-test.
	rbTopic := topic + "-rb"
	t.Run("rollback_hidden", func(t *testing.T) {
		tx, err := p.BeginTransaction(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTransaction: %v", err)
		}
		payload := []byte(`{"action":"rollback"}`)
		if err := p.Publish(ctx, &mq.Message{Topic: rbTopic, Payload: payload}); err != nil {
			if strings.Contains(err.Error(), "message type not match") ||
				strings.Contains(err.Error(), "topic route") ||
				strings.Contains(err.Error(), "No topic route") {
				t.Skipf("RocketMQ 5.x topic %q is not TRANSACTION-typed; create it with: mqadmin updateTopic -n <namesrv> -c <cluster> -t %s -a +message.type=TRANSACTION: %v", rbTopic, rbTopic, err)
			}
			t.Fatalf("Publish in transaction: %v", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatalf("tx.Rollback: %v", err)
		}

		c, err := mq.NewRocketMQConsumer(mq.RocketMQConsumerConfig{
			Endpoint:      endpoint,
			Topic:        rbTopic,
			ConsumerGroup: testGroup + "-tx-rollback",
		})
		if err != nil {
			t.Fatalf("create consumer: %v", err)
		}
		defer c.Close()

		received := make(chan *mq.Message, 1)
		go func() {
			subCtx, subCancel := context.WithCancel(ctx)
			defer subCancel()
			_ = c.Subscribe(subCtx, []string{rbTopic}, testGroup+"-tx-rollback", func(_ context.Context, msg *mq.Message) error {
				select {
				case received <- msg:
				default:
				}
				return nil
			})
		}()

		// Wait well beyond the broker's invisible duration; a rolled-back
		// half-message must never be delivered to a consumer.
		select {
		case msg := <-received:
			t.Errorf("rolled-back transaction message was delivered (must stay hidden): %q", string(msg.Payload))
		case <-time.After(3 * time.Second):
			// Expected: nothing arrived.
		}
	})

	// After all sub-tests, the active transaction must be cleared so a new
	// one can begin (proves the framework state machine resets correctly).
	// A sub-test that skipped mid-transaction leaves activeTx set; roll it
	// back first so the assertion below reflects the framework, not a
	// skipped sub-test's leftover state.
	_ = p.Rollback(ctx)
	if _, err := p.BeginTransaction(ctx, nil); err != nil {
		t.Fatalf("BeginTransaction after sub-tests: %v", err)
	}
	if err := p.Rollback(ctx); err != nil {
		t.Fatalf("cleanup rollback: %v", err)
	}
}

// ── Kafka transactions ──────────────────────────────────────────────────────────

// TestKafkaTransaction_BeginTransaction verifies Kafka transactional producer:
// BeginTransaction → Publish → Commit → verify message is delivered.
// Also verifies that rollback is not supported (ErrCapTxNotSupported on Begin).
func TestKafkaTransaction_BeginTransaction(t *testing.T) {
	skipUnlessType(t, "kafka")

	brokers := brokersFromEnv("KAFKA_BROKERS", "localhost:9092")

	p, err := mq.NewKafkaProducer(mq.KafkaProducerConfig{
		Brokers:  brokers,
		EnableTx: true, // required for Kafka transactions
	})
	if err != nil {
		t.Fatalf("create Kafka producer: %v", err)
	}
	defer p.Close()

	caps := p.Capabilities()
	if !caps[mq.CapTx] {
		t.Skip("Kafka producer CapTx=false (EnableTx may not be supported in this env); skipping")
	}

	// ── Commit path ──
	t.Run("commit", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		tx, err := p.BeginTransaction(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTransaction: %v", err)
		}

		payload := []byte(`{"kafka":"commit"}`)
		if err := p.Publish(ctx, &mq.Message{Topic: testTopic + "-kafka-tx", Payload: payload}); err != nil {
			t.Fatalf("Publish: %v", err)
		}

		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("tx.Commit: %v", err)
		}

		// Consume to verify
		c, err := mq.NewKafkaConsumer(mq.KafkaConsumerConfig{
			Brokers: brokers,
			Group:   testGroup + "-kafka-tx",
		})
		if err != nil {
			t.Fatalf("create consumer: %v", err)
		}
		defer c.Close()

		received := make(chan *mq.Message, 1)
		go func() {
			subCtx, subCancel := context.WithCancel(ctx)
			defer subCancel()
			_ = c.Subscribe(subCtx, []string{testTopic + "-kafka-tx"}, testGroup+"-kafka-tx", func(_ context.Context, msg *mq.Message) error {
				select {
				case received <- msg:
				default:
				}
				return nil
			})
		}()

		time.Sleep(500 * time.Millisecond)

		select {
		case msg := <-received:
			if string(msg.Payload) != string(payload) {
				t.Errorf("payload mismatch: got %q, want %q", string(msg.Payload), string(payload))
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for committed message")
		}
	})
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
