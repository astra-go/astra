package mq_test

import (
	"context"
	"errors"
	"testing"

	"github.com/astra-go/astra/mq"
)

// TestBeginTransaction_NotSupported verifies that backends which do not
// implement CapTx return ErrCapTxNotSupported from BeginTransaction.
func TestBeginTransaction_NotSupported(t *testing.T) {
	t.Parallel()

	producer := mq.NewMemoryProducer("test-topic")
	defer producer.Close()

	caps := producer.Capabilities()
	if caps[mq.CapTx] {
		t.Fatalf("MemoryProducer should have CapTx=false, got CapTx=true")
	}

	tx, err := producer.BeginTransaction(context.Background(), nil)
	if !errors.Is(err, mq.ErrCapTxNotSupported) {
		t.Fatalf("expected ErrCapTxNotSupported, got: %v", err)
	}
	if tx != nil {
		t.Fatalf("expected nil Transaction on error, got: %v", tx)
	}
}

// TestBeginTransaction_NotSupported_Nats verifies NATS returns ErrCapTxNotSupported.
func TestBeginTransaction_NotSupported_Nats(t *testing.T) {
	t.Parallel()
	p, err := mq.NewNATSProducer(mq.NATSConfig{URL: "nats://localhost:4222"})
	if err != nil {
		t.Skipf("NATS unavailable (expected in unit-test env): %v", err)
	}
	defer p.Close()

	tx, err := p.BeginTransaction(context.Background(), nil)
	if !errors.Is(err, mq.ErrCapTxNotSupported) {
		t.Fatalf("NATS: expected ErrCapTxNotSupported, got: %v", err)
	}
	if tx != nil {
		t.Fatalf("NATS: expected nil Transaction, got: %v", tx)
	}
}

// TestBeginTransaction_NotSupported_RabbitMQ verifies RabbitMQ returns ErrCapTxNotSupported.
func TestBeginTransaction_NotSupported_RabbitMQ(t *testing.T) {
	t.Parallel()
	p, err := mq.NewRabbitMQProducer(mq.RabbitMQConfig{URL: "amqp://guest:guest@localhost:5672/"})
	if err != nil {
		t.Skipf("RabbitMQ unavailable (expected in unit-test env): %v", err)
	}
	defer p.Close()

	tx, err := p.BeginTransaction(context.Background(), nil)
	if !errors.Is(err, mq.ErrCapTxNotSupported) {
		t.Fatalf("RabbitMQ: expected ErrCapTxNotSupported, got: %v", err)
	}
	if tx != nil {
		t.Fatalf("RabbitMQ: expected nil Transaction, got: %v", tx)
	}
}

// TestBeginTransaction_NotSupported_Redis verifies Redis returns ErrCapTxNotSupported.
func TestBeginTransaction_NotSupported_Redis(t *testing.T) {
	t.Parallel()
	// NewRedisProducer returns (*RedisProducer, error) but the test package only
	// sees a *RedisProducer (no error in signature). Create directly.
	p := mq.NewRedisProducer(mq.RedisConfig{Addr: "localhost:6379"})

	tx, err := p.BeginTransaction(context.Background(), nil)
	if !errors.Is(err, mq.ErrCapTxNotSupported) {
		t.Fatalf("Redis: expected ErrCapTxNotSupported, got: %v", err)
	}
	if tx != nil {
		t.Fatalf("Redis: expected nil Transaction, got: %v", tx)
	}
}

// TestBeginTransaction_NotSupported_Pulsar verifies Pulsar returns ErrCapTxNotSupported.
func TestBeginTransaction_NotSupported_Pulsar(t *testing.T) {
	t.Parallel()
	p, err := mq.NewPulsarProducer(mq.PulsarConfig{URL: "pulsar://localhost:6650"})
	if err != nil {
		t.Skipf("Pulsar unavailable (expected in unit-test env): %v", err)
	}
	defer p.Close()

	tx, err := p.BeginTransaction(context.Background(), nil)
	if !errors.Is(err, mq.ErrCapTxNotSupported) {
		t.Fatalf("Pulsar: expected ErrCapTxNotSupported, got: %v", err)
	}
	if tx != nil {
		t.Fatalf("Pulsar: expected nil Transaction, got: %v", tx)
	}
}

// TestBeginTransaction_NotSupported_MQTT verifies MQTT returns ErrCapTxNotSupported.
func TestBeginTransaction_NotSupported_MQTT(t *testing.T) {
	t.Parallel()
	p, err := mq.NewMQTTProducer(mq.MQTTConfig{Broker: "tcp://localhost:1883", ClientID: "ut-mqtt"})
	if err != nil {
		t.Skipf("MQTT unavailable (expected in unit-test env): %v", err)
	}
	defer p.Close()

	tx, err := p.BeginTransaction(context.Background(), nil)
	if !errors.Is(err, mq.ErrCapTxNotSupported) {
		t.Fatalf("MQTT: expected ErrCapTxNotSupported, got: %v", err)
	}
	if tx != nil {
		t.Fatalf("MQTT: expected nil Transaction, got: %v", tx)
	}
}

// TestRocketMQProducer_CommitRollback_NoActiveTxn verifies that calling
// Commit/Rollback on a RocketMQProducer with no active transaction is a safe
// no-op (returns nil) rather than an error.
func TestRocketMQProducer_CommitRollback_NoActiveTxn(t *testing.T) {
	t.Parallel()
	p, err := mq.NewRocketMQProducer(mq.RocketMQConfig{
		Endpoint: "http://localhost:9876",
		Topic:    "test-topic",
	})
	if err != nil {
		t.Skipf("RocketMQ nameserver unavailable (expected in unit-test env): %v", err)
	}
	defer p.Close()

	ctx := context.Background()

	if err := p.Commit(ctx); err != nil {
		t.Errorf("Commit with no active transaction: expected nil, got %v", err)
	}
	if err := p.Rollback(ctx); err != nil {
		t.Errorf("Rollback with no active transaction: expected nil, got %v", err)
	}
}

// TestProducerInterface_AllBackends_BeginTransaction is a compile-time check
// that all producer types implement the extended Producer interface with
// BeginTransaction. Adding a new backend without BeginTransaction causes a
// build failure here.
func TestProducerInterface_AllBackends_BeginTransaction(t *testing.T) {
	t.Parallel()

	// If a backend's producer is missing BeginTransaction, this line fails to compile.
	var _ mq.Producer = (*mq.MemoryProducer)(nil)
	var _ mq.Producer = (*mq.KafkaProducer)(nil)
	var _ mq.Producer = (*mq.RocketMQProducer)(nil)
	var _ mq.Producer = (*mq.NATSProducer)(nil)
	var _ mq.Producer = (*mq.RabbitMQProducer)(nil)
	var _ mq.Producer = (*mq.RedisProducer)(nil)
	var _ mq.Producer = (*mq.PulsarProducer)(nil)
	var _ mq.Producer = (*mq.MQTTProducer)(nil)

	// If we reach here, all producers satisfy the interface.
	t.Log("All 8 backend producers implement mq.Producer with BeginTransaction")
}

// TestErrCapTxNotSupported_Sentinel verifies that ErrCapTxNotSupported is
// a proper sentinel error usable with errors.Is.
func TestErrCapTxNotSupported_Sentinel(t *testing.T) {
	t.Parallel()

	if !errors.Is(mq.ErrCapTxNotSupported, mq.ErrCapTxNotSupported) {
		t.Error("ErrCapTxNotSupported should satisfy errors.Is(self)")
	}
}

// TestTransactionChecker_Type verifies TransactionChecker is a concrete func type.
func TestTransactionChecker_Type(t *testing.T) {
	t.Parallel()

	var checker mq.TransactionChecker = func(ctx context.Context, msg *mq.Message) (bool, error) {
		return true, nil
	}

	result, err := checker(context.Background(), &mq.Message{Topic: "test"})
	if err != nil {
		t.Fatalf("checker returned error: %v", err)
	}
	if !result {
		t.Error("expected true from checker")
	}
}
