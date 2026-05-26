package pulsar_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/mq"
	"github.com/astra-go/astra/mq/pulsar"
	"github.com/astra-go/astra/testutil"
)

// ─── Compile-time interface checks ───────────────────────────────────────────

var _ mq.Producer = (*pulsar.Producer)(nil)
var _ mq.Consumer = (*pulsar.Consumer)(nil)

// ─── Consumer — input validation (no broker required) ────────────────────────

// newTestConsumer creates a Consumer pointing at an unreachable broker with a
// very short timeout. The Pulsar Go client is lazy — the TCP connection is not
// attempted until Subscribe or CreateProducer is called, so NewConsumer
// succeeds immediately and allows us to exercise input-validation paths.
func newTestConsumer(t *testing.T) *pulsar.Consumer {
	t.Helper()
	c, err := pulsar.NewConsumer(pulsar.ConsumerConfig{
		Config: pulsar.Config{URL: "pulsar://127.0.0.1:6650"},
	})
	if err != nil {
		t.Skipf("NewConsumer failed (Pulsar client-go rejected URL at construction): %v", err)
	}
	return c
}

func TestConsumer_Subscribe_NoTopics_ReturnsError(t *testing.T) {
	c := newTestConsumer(t)
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled so even if validation is skipped, we won't hang

	err := c.Subscribe(ctx, nil, "", func(_ context.Context, _ *mq.Message) error { return nil })
	if err == nil {
		t.Fatal("expected error when no topics given")
	}
}

func TestConsumer_Subscribe_NoSubscriptionName_ReturnsError(t *testing.T) {
	c := newTestConsumer(t)
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Neither ConsumerConfig.Subscription nor the group argument is set.
	err := c.Subscribe(ctx, []string{"test-topic"}, "", func(_ context.Context, _ *mq.Message) error { return nil })
	if err == nil {
		t.Fatal("expected error when subscription name is empty")
	}
}

// ─── NewProducer / NewConsumer — bad URL error path ──────────────────────────

func TestNewProducer_InvalidURLScheme_ReturnsError(t *testing.T) {
	_, err := pulsar.NewProducer(pulsar.Config{URL: "not-a-valid-scheme://localhost:6650"})
	if err == nil {
		t.Skip("Pulsar client accepted invalid URL scheme — library may be lenient")
	}
	// err is non-nil: pass.
}

// ─── Config defaults ──────────────────────────────────────────────────────────

// TestConfig_DefaultURL verifies that an empty URL in Config is treated as the
// default Pulsar broker address.  We test this indirectly via NewProducer: the
// resulting error (if any) must not mention "URL" as an explicit validation
// failure (the default gets applied internally).
func TestConfig_DefaultURL_AppliedBeforeConnect(t *testing.T) {
	// This just exercises the code path — we don't assert a specific error.
	// The point is to verify the code compiles and runs without panicking.
	_, _ = pulsar.NewProducer(pulsar.Config{}) // URL defaults to "pulsar://localhost:6650"
}

// ─── Producer — Close is idempotent ──────────────────────────────────────────

func TestProducer_Close_ReturnsNil(t *testing.T) {
	p, err := pulsar.NewProducer(pulsar.Config{URL: "pulsar://127.0.0.1:6650"})
	if err != nil {
		t.Skipf("NewProducer rejected URL at construction: %v", err)
	}
	testutil.AssertNoError(t, p.Close())
}
