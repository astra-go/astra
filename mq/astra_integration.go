// Package mq - Astra lifecycle integration.
//
// This file provides Register() to integrate mq with the Astra app lifecycle.
// It uses dependency injection (AppRegistrar interface) to avoid circular imports
// with the parent astra/ module.
package mq

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// AppRegistrar is the interface that the Astra app must implement
// for mq lifecycle integration.
// It allows mq/ to register OnStart/OnStop hooks without importing astra/.
type AppRegistrar interface {
	// OnStart registers a function to be called when the app starts.
	OnStart(func(ctx context.Context) error)

	// OnStop registers a function to be called when the app stops.
	OnStop(func(ctx context.Context) error)
}

// IntegrationOptions configures the mq integration.
type IntegrationOptions struct {
	// Broker is the message broker type: "kafka", "rabbitmq", "rocketmq", "nats", "pulsar", "mqtt".
	Broker string

	// Addrs is the list of broker addresses.
	Addrs []string

	// EnableRedisCompensator enables Redis-based delay/idempotency compensation.
	EnableRedisCompensator bool

	// RedisAddr is the Redis address (used for compensation).
	// If empty, "localhost:6379" is used.
	RedisAddr string

	// RedisPass is the Redis password.
	RedisPass string
}

// RegisteredAdapter wraps the mq Producer and Consumer, with lifecycle management.
type RegisteredAdapter struct {
	Producer Producer
	Consumer Consumer
	scanner  *DelayScanner
}

// Register registers mq with the Astra app lifecycle via the AppRegistrar interface.
// It creates the producer/consumer, registers OnStart/OnStop hooks,
// and optionally starts the Redis DelayScanner.
//
// Usage (in the main module):
//
//	import mq "github.com/astra-go/astra/mq"
//
//	app := astra.New()
//	adapter, err := mq.Register(app, mq.IntegrationOptions{
//	    Broker: "kafka",
//	    Addrs:  []string{"localhost:9092"},
//	})
func Register(app AppRegistrar, opts IntegrationOptions) (*RegisteredAdapter, error) {
	// Create producer
	producer, err := createProducer(opts)
	if err != nil {
		return nil, fmt.Errorf("mq: create producer: %w", err)
	}

	// Create consumer
	consumer, err := createConsumer(opts)
	if err != nil {
		producer.Close()
		return nil, fmt.Errorf("mq: create consumer: %w", err)
	}

	adapter := &RegisteredAdapter{
		Producer: producer,
		Consumer: consumer,
	}

	// Register OnStop hook: close producer/consumer
	app.OnStop(func(_ context.Context) error {
		consumer.Close()
		producer.Close()
		return nil
	})

	// Optionally start Redis DelayScanner
	if opts.EnableRedisCompensator && opts.RedisAddr != "" {
		rdb := goredis.NewClient(&goredis.Options{
			Addr:     opts.RedisAddr,
			Password: opts.RedisPass,
		})
		scanner := NewDelayScanner(rdb, producer, 5*time.Second)
		adapter.scanner = scanner
		app.OnStart(func(ctx context.Context) error {
			go scanner.Run(ctx)
			return nil
		})
		app.OnStop(func(_ context.Context) error {
			scanner.Stop()
			return nil
		})
	}

	return adapter, nil
}

// createProducer creates a broker-specific producer.
func createProducer(opts IntegrationOptions) (Producer, error) {
	switch opts.Broker {
	case "kafka":
		return nil, fmt.Errorf("mq: kafka producer creation not yet implemented")
	case "rabbitmq":
		return nil, fmt.Errorf("mq: rabbitmq producer creation not yet implemented")
	default:
		return nil, fmt.Errorf("mq: unknown broker: %s", opts.Broker)
	}
}

// createConsumer creates a broker-specific consumer.
func createConsumer(opts IntegrationOptions) (Consumer, error) {
	switch opts.Broker {
	case "kafka":
		return nil, fmt.Errorf("mq: kafka consumer creation not yet implemented")
	case "rabbitmq":
		return nil, fmt.Errorf("mq: rabbitmq consumer creation not yet implemented")
	default:
		return nil, fmt.Errorf("mq: unknown broker: %s", opts.Broker)
	}
}
