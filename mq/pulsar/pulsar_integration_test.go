//go:build integration

package pulsar_test

// Pulsar round-trip integration tests for mq/pulsar.
//
// Prerequisites — start a Pulsar standalone container:
//
//	docker run -d --name pulsar-test \
//	  -p 6650:6650 -p 8080:8080 \
//	  apachepulsar/pulsar:3.3.0 bin/pulsar standalone
//
// Then run:
//
//	PULSAR_URL="pulsar://localhost:6650" \
//	  go test -tags integration ./mq/pulsar/...

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra/mq"
	"github.com/astra-go/astra/mq/pulsar"
)

// pulsarURLFromEnv returns the broker URL or skips the test.
func pulsarURLFromEnv(t *testing.T) string {
	t.Helper()
	url := os.Getenv("PULSAR_URL")
	if url == "" {
		t.Skip("PULSAR_URL not set — skipping Pulsar integration tests")
	}
	return url
}

// uniqueTopic generates a unique persistent topic name per test run.
func uniqueTopic(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("persistent://public/default/astra-test-%s-%d",
		t.Name(), rand.Int63())
}

// ─── Producer round-trip ──────────────────────────────────────────────────────

func TestIntegration_Pulsar_PublishAndReceive(t *testing.T) {
	url := pulsarURLFromEnv(t)
	topic := uniqueTopic(t)

	producer, err := pulsar.NewProducer(pulsar.Config{URL: url})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	defer producer.Close()

	consumer, err := pulsar.NewConsumer(pulsar.ConsumerConfig{
		Config:       pulsar.Config{URL: url},
		Subscription: "astra-test-sub",
	})
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer consumer.Close()

	want := []byte("hello-astra")

	// Start consumer in background
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var (
		received []byte
		wg       sync.WaitGroup
		subErr   error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		subErr = consumer.Subscribe(ctx, []string{topic}, "", func(_ context.Context, msg *mq.Message) error {
			received = msg.Payload
			cancel() // stop after first message
			return nil
		})
	}()

	// Give consumer time to subscribe
	time.Sleep(500 * time.Millisecond)

	if err := producer.Publish(context.Background(), &mq.Message{
		Topic:   topic,
		Payload: want,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	wg.Wait()

	if subErr != nil && subErr != context.Canceled {
		t.Fatalf("Subscribe: %v", subErr)
	}
	if string(received) != string(want) {
		t.Errorf("payload mismatch: got %q, want %q", received, want)
	}
}

// ─── PublishBatch round-trip ──────────────────────────────────────────────────

func TestIntegration_Pulsar_PublishBatch(t *testing.T) {
	url := pulsarURLFromEnv(t)
	topic := uniqueTopic(t)

	producer, err := pulsar.NewProducer(pulsar.Config{URL: url})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	defer producer.Close()

	consumer, err := pulsar.NewConsumer(pulsar.ConsumerConfig{
		Config:       pulsar.Config{URL: url},
		Subscription: "astra-batch-sub",
	})
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer consumer.Close()

	msgs := []*mq.Message{
		{Topic: topic, Payload: []byte("msg-0")},
		{Topic: topic, Payload: []byte("msg-1")},
		{Topic: topic, Payload: []byte("msg-2")},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collected := make([]string, 0, 3)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer.Subscribe(ctx, []string{topic}, "", func(_ context.Context, msg *mq.Message) error { //nolint:errcheck
			mu.Lock()
			collected = append(collected, string(msg.Payload))
			if len(collected) >= 3 {
				cancel()
			}
			mu.Unlock()
			return nil
		})
	}()

	time.Sleep(500 * time.Millisecond)

	if err := producer.PublishBatch(context.Background(), msgs); err != nil {
		t.Fatalf("PublishBatch: %v", err)
	}

	wg.Wait()

	mu.Lock()
	n := len(collected)
	mu.Unlock()
	if n != 3 {
		t.Errorf("expected 3 messages, received %d", n)
	}
}

// ─── Message headers (properties) ────────────────────────────────────────────

func TestIntegration_Pulsar_MessageHeaders(t *testing.T) {
	url := pulsarURLFromEnv(t)
	topic := uniqueTopic(t)

	producer, err := pulsar.NewProducer(pulsar.Config{URL: url})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	defer producer.Close()

	consumer, err := pulsar.NewConsumer(pulsar.ConsumerConfig{
		Config:       pulsar.Config{URL: url},
		Subscription: "astra-hdr-sub",
	})
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer consumer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var receivedHeaders map[string]string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer.Subscribe(ctx, []string{topic}, "", func(_ context.Context, msg *mq.Message) error { //nolint:errcheck
			receivedHeaders = msg.Headers
			cancel()
			return nil
		})
	}()

	time.Sleep(500 * time.Millisecond)

	if err := producer.Publish(context.Background(), &mq.Message{
		Topic:   topic,
		Payload: []byte("hdr-test"),
		Headers: map[string]string{"x-trace-id": "abc123"},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	wg.Wait()

	if receivedHeaders["x-trace-id"] != "abc123" {
		t.Errorf("header mismatch: got %v", receivedHeaders)
	}
}
