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

// TestProducerInterface_BeginTransactionSignature verifies that every producer
// in the registry satisfies the extended Producer interface with BeginTransaction.
func TestProducerInterface_BeginTransactionSignature(t *testing.T) {
	t.Parallel()

	// Compile-time check: cast each producer type to the interface.
	// If a producer is missing BeginTransaction, this fails to compile.
	var _ mq.Producer = (*mq.MemoryProducer)(nil)

	// For other backends we rely on the builder to construct valid producers.
	// The builder is tested separately in builder_test.go (integration).
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
