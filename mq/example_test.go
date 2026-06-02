package mq_test

import (
	"context"
	"fmt"

	"github.com/astra-go/astra/mq"
)

// Example demonstrating the new v2.x API with direct type constructors.
func ExampleNewKafkaProducer() {
	// Before (v1.x):
	// import "github.com/astra-go/astra/mq/kafka"
	// p, err := kafka.NewProducer(kafka.ProducerConfig{...})

	// After (v2.x):
	p, err := mq.NewKafkaProducer(mq.KafkaProducerConfig{
		Brokers: []string{"localhost:9092"},
	})
	if err != nil {
		panic(err)
	}
	defer p.Close()

	ctx := context.Background()
	msg := &mq.Message{
		Topic:   "events",
		Key:     []byte("user-123"),
		Payload: []byte(`{"event":"user.created"}`),
	}

	if err := p.Publish(ctx, msg); err != nil {
		fmt.Printf("publish error: %v\n", err)
	}
}

// Example demonstrating the new v2.x string-based factory method.
func ExampleNewProducer() {
	// v2.x new feature: string-based factory
	p, err := mq.NewProducer("kafka", mq.ProducerOptions{
		Brokers: []string{"localhost:9092"},
	})
	if err != nil {
		panic(err)
	}
	defer p.Close()

	ctx := context.Background()
	msg := &mq.Message{
		Topic:   "orders",
		Payload: []byte(`{"order_id":456}`),
	}

	if err := p.Publish(ctx, msg); err != nil {
		fmt.Printf("publish error: %v\n", err)
	}
}

// Example demonstrating Kafka consumer in v2.x.
func ExampleNewKafkaConsumer() {
	c, err := mq.NewKafkaConsumer(mq.KafkaConsumerConfig{
		Brokers: []string{"localhost:9092"},
		Group:   "my-service",
	})
	if err != nil {
		panic(err)
	}
	defer c.Close()

	ctx := context.Background()
	handler := func(ctx context.Context, msg *mq.Message) error {
		fmt.Printf("received: topic=%s payload=%s\n", msg.Topic, msg.Payload)
		return nil
	}

	if err := c.Subscribe(ctx, []string{"events"}, "my-service", handler); err != nil {
		fmt.Printf("subscribe error: %v\n", err)
	}
}

// Example demonstrating RabbitMQ producer in v2.x.
func ExampleNewRabbitMQProducer() {
	p, err := mq.NewRabbitMQProducer(mq.RabbitMQConfig{
		URL:      "amqp://guest:guest@localhost:5672/",
		Exchange: "events",
	})
	if err != nil {
		panic(err)
	}
	defer p.Close()

	ctx := context.Background()
	msg := &mq.Message{
		Topic:   "user.created",
		Payload: []byte(`{"user_id":789}`),
	}

	if err := p.Publish(ctx, msg); err != nil {
		fmt.Printf("publish error: %v\n", err)
	}
}

// Example demonstrating NATS producer in v2.x.
func ExampleNewNATSProducer() {
	p, err := mq.NewNATSProducer(mq.NATSConfig{
		URL: "nats://localhost:4222",
	})
	if err != nil {
		panic(err)
	}
	defer p.Close()

	ctx := context.Background()
	msg := &mq.Message{
		Topic:   "orders.created",
		Payload: []byte(`{"order_id":123}`),
	}

	if err := p.Publish(ctx, msg); err != nil {
		fmt.Printf("publish error: %v\n", err)
	}
}

// Example demonstrating MQTT producer in v2.x.
func ExampleNewMQTTProducer() {
	p, err := mq.NewMQTTProducer(mq.MQTTConfig{
		Broker:   "tcp://localhost:1883",
		ClientID: "producer-1",
	})
	if err != nil {
		panic(err)
	}
	defer p.Close()

	ctx := context.Background()
	msg := &mq.Message{
		Topic:   "sensors/temperature",
		Payload: []byte("22.5"),
	}

	if err := p.Publish(ctx, msg); err != nil {
		fmt.Printf("publish error: %v\n", err)
	}
}

// Example demonstrating Pulsar producer in v2.x.
func ExampleNewPulsarProducer() {
	p, err := mq.NewPulsarProducer(mq.PulsarConfig{
		URL: "pulsar://localhost:6650",
	})
	if err != nil {
		panic(err)
	}
	defer p.Close()

	ctx := context.Background()
	msg := &mq.Message{
		Topic:   "persistent://public/default/orders",
		Payload: []byte(`{"order_id":999}`),
	}

	if err := p.Publish(ctx, msg); err != nil {
		fmt.Printf("publish error: %v\n", err)
	}
}

// Example demonstrating RocketMQ producer in v2.x.
func ExampleNewRocketMQProducer() {
	p, err := mq.NewRocketMQProducer(mq.RocketMQConfig{
		Endpoint: "localhost:8081",
		Topic:    "orders",
	})
	if err != nil {
		panic(err)
	}
	defer p.Close()

	ctx := context.Background()
	msg := &mq.Message{
		Topic:   "orders",
		Payload: []byte(`{"order_id":888}`),
	}

	if err := p.Publish(ctx, msg); err != nil {
		fmt.Printf("publish error: %v\n", err)
	}
}
