#!/bin/bash
# Automated MQ v1 → v2 migration script

set -e

echo "🔄 Starting MQ v2 migration..."

# Step 1: Update package declarations in all implementation files
echo "📝 Updating package declarations..."
for file in mq/kafka.go mq/rabbitmq.go mq/nats.go mq/mqtt.go mq/pulsar.go mq/rocketmq.go; do
    if [ -f "$file" ]; then
        # Extract the base name (kafka, rabbitmq, etc.)
        base=$(basename "$file" .go)

        # Change package declaration to mq
        sed -i.bak "s/^package $base$/package mq/" "$file"

        # Remove import of parent mq package if it exists
        sed -i.bak '/^[[:space:]]*"github.com\/astra-go\/astra\/mq"$/d' "$file"
        sed -i.bak '/^import (/,/^)/ { /^[[:space:]]*"github.com\/astra-go\/astra\/mq"$/d; }' "$file"

        echo "  ✓ Updated $file"
    fi
done

# Step 2: Rename types and functions (using capitalized versions)
echo "📝 Renaming types and functions..."

# Kafka
if [ -f "mq/kafka.go" ]; then
    sed -i.bak \
        -e 's/type ProducerConfig struct/type KafkaProducerConfig struct/g' \
        -e 's/type ConsumerConfig struct/type KafkaConsumerConfig struct/g' \
        -e 's/type Producer struct/type KafkaProducer struct/g' \
        -e 's/type Consumer struct/type KafkaConsumer struct/g' \
        -e 's/func NewProducer(/func NewKafkaProducer(/g' \
        -e 's/func NewConsumer(/func NewKafkaConsumer(/g' \
        -e 's/\*Producer{/\*KafkaProducer{/g' \
        -e 's/\*Consumer{/\*KafkaConsumer{/g' \
        -e 's/func (p \*Producer)/func (p \*KafkaProducer)/g' \
        -e 's/func (c \*Consumer)/func (c \*KafkaConsumer)/g' \
        -e 's/cfg ProducerConfig/cfg KafkaProducerConfig/g' \
        -e 's/cfg ConsumerConfig/cfg KafkaConsumerConfig/g' \
        -e 's/mq\.Message/Message/g' \
        -e 's/mq\.Handler/Handler/g' \
        mq/kafka.go
    echo "  ✓ Updated Kafka types"
fi

# RabbitMQ
if [ -f "mq/rabbitmq.go" ]; then
    sed -i.bak \
        -e 's/^package rabbitmq$/package mq/' \
        -e 's/type Config struct/type RabbitMQConfig struct/g' \
        -e 's/type ConsumerConfig struct/type RabbitMQConsumerConfig struct/g' \
        -e 's/type Producer struct/type RabbitMQProducer struct/g' \
        -e 's/type Consumer struct/type RabbitMQConsumer struct/g' \
        -e 's/func NewProducer(/func NewRabbitMQProducer(/g' \
        -e 's/func NewConsumer(/func NewRabbitMQConsumer(/g' \
        -e 's/\*Producer{/\*RabbitMQProducer{/g' \
        -e 's/\*Consumer{/\*RabbitMQConsumer{/g' \
        -e 's/func (p \*Producer)/func (p \*RabbitMQProducer)/g' \
        -e 's/func (c \*Consumer)/func (c \*RabbitMQConsumer)/g' \
        -e 's/cfg Config/cfg RabbitMQConfig/g' \
        -e 's/\*Config)/\*RabbitMQConfig)/g' \
        -e 's/func (c \*Config)/func (c \*RabbitMQConfig)/g' \
        -e 's/cfg ConsumerConfig/cfg RabbitMQConsumerConfig/g' \
        -e 's/mq\.Message/Message/g' \
        -e 's/mq\.Handler/Handler/g' \
        mq/rabbitmq.go
    echo "  ✓ Updated RabbitMQ types"
fi

# NATS
if [ -f "mq/nats.go" ]; then
    sed -i.bak \
        -e 's/^package nats$/package mq/' \
        -e 's/type Config struct/type NATSConfig struct/g' \
        -e 's/type ProducerConfig struct/type NATSProducerConfig struct/g' \
        -e 's/type ConsumerConfig struct/type NATSConsumerConfig struct/g' \
        -e 's/type Producer struct/type NATSProducer struct/g' \
        -e 's/type Consumer struct/type NATSConsumer struct/g' \
        -e 's/func NewProducer(/func NewNATSProducer(/g' \
        -e 's/func NewConsumer(/func NewNATSConsumer(/g' \
        -e 's/\*Producer{/\*NATSProducer{/g' \
        -e 's/\*Consumer{/\*NATSConsumer{/g' \
        -e 's/func (p \*Producer)/func (p \*NATSProducer)/g' \
        -e 's/func (c \*Consumer)/func (c \*NATSConsumer)/g' \
        -e 's/cfg Config/cfg NATSConfig/g' \
        -e 's/cfg ProducerConfig/cfg NATSProducerConfig/g' \
        -e 's/cfg ConsumerConfig/cfg NATSConsumerConfig/g' \
        -e 's/mq\.Message/Message/g' \
        -e 's/mq\.Handler/Handler/g' \
        mq/nats.go
    echo "  ✓ Updated NATS types"
fi

# MQTT
if [ -f "mq/mqtt.go" ]; then
    sed -i.bak \
        -e 's/^package mqtt$/package mq/' \
        -e 's/type Config struct/type MQTTConfig struct/g' \
        -e 's/type ProducerConfig struct/type MQTTProducerConfig struct/g' \
        -e 's/type ConsumerConfig struct/type MQTTConsumerConfig struct/g' \
        -e 's/type Producer struct/type MQTTProducer struct/g' \
        -e 's/type Consumer struct/type MQTTConsumer struct/g' \
        -e 's/func NewProducer(/func NewMQTTProducer(/g' \
        -e 's/func NewConsumer(/func NewMQTTConsumer(/g' \
        -e 's/\*Producer{/\*MQTTProducer{/g' \
        -e 's/\*Consumer{/\*MQTTConsumer{/g' \
        -e 's/func (p \*Producer)/func (p \*MQTTProducer)/g' \
        -e 's/func (c \*Consumer)/func (c \*MQTTConsumer)/g' \
        -e 's/cfg Config/cfg MQTTConfig/g' \
        -e 's/cfg ProducerConfig/cfg MQTTProducerConfig/g' \
        -e 's/cfg ConsumerConfig/cfg MQTTConsumerConfig/g' \
        -e 's/mq\.Message/Message/g' \
        -e 's/mq\.Handler/Handler/g' \
        mq/mqtt.go
    echo "  ✓ Updated MQTT types"
fi

# Pulsar
if [ -f "mq/pulsar.go" ]; then
    sed -i.bak \
        -e 's/^package pulsar$/package mq/' \
        -e 's/type Config struct/type PulsarConfig struct/g' \
        -e 's/type ProducerConfig struct/type PulsarProducerConfig struct/g' \
        -e 's/type ConsumerConfig struct/type PulsarConsumerConfig struct/g' \
        -e 's/type Producer struct/type PulsarProducer struct/g' \
        -e 's/type Consumer struct/type PulsarConsumer struct/g' \
        -e 's/func NewProducer(/func NewPulsarProducer(/g' \
        -e 's/func NewConsumer(/func NewPulsarConsumer(/g' \
        -e 's/\*Producer{/\*PulsarProducer{/g' \
        -e 's/\*Consumer{/\*PulsarConsumer{/g' \
        -e 's/func (p \*Producer)/func (p \*PulsarProducer)/g' \
        -e 's/func (c \*Consumer)/func (c \*PulsarConsumer)/g' \
        -e 's/cfg Config/cfg PulsarConfig/g' \
        -e 's/\*Config)/\*PulsarConfig)/g' \
        -e 's/func (c \*Config)/func (c \*PulsarConfig)/g' \
        -e 's/cfg ProducerConfig/cfg PulsarProducerConfig/g' \
        -e 's/cfg ConsumerConfig/cfg PulsarConsumerConfig/g' \
        -e 's/mq\.Message/Message/g' \
        -e 's/mq\.Handler/Handler/g' \
        mq/pulsar.go
    echo "  ✓ Updated Pulsar types"
fi

# RocketMQ
if [ -f "mq/rocketmq.go" ]; then
    sed -i.bak \
        -e 's/^package rocketmq$/package mq/' \
        -e 's/type Config struct/type RocketMQConfig struct/g' \
        -e 's/type ProducerConfig struct/type RocketMQProducerConfig struct/g' \
        -e 's/type ConsumerConfig struct/type RocketMQConsumerConfig struct/g' \
        -e 's/type Producer struct/type RocketMQProducer struct/g' \
        -e 's/type Consumer struct/type RocketMQConsumer struct/g' \
        -e 's/func NewProducer(/func NewRocketMQProducer(/g' \
        -e 's/func NewConsumer(/func NewRocketMQConsumer(/g' \
        -e 's/\*Producer{/\*RocketMQProducer{/g' \
        -e 's/\*Consumer{/\*RocketMQConsumer{/g' \
        -e 's/func (p \*Producer)/func (p \*RocketMQProducer)/g' \
        -e 's/func (c \*Consumer)/func (c \*RocketMQConsumer)/g' \
        -e 's/cfg Config/cfg RocketMQConfig/g' \
        -e 's/cfg ProducerConfig/cfg RocketMQProducerConfig/g' \
        -e 's/cfg ConsumerConfig/cfg RocketMQConsumerConfig/g' \
        -e 's/mq\.Message/Message/g' \
        -e 's/mq\.Handler/Handler/g' \
        mq/rocketmq.go
    echo "  ✓ Updated RocketMQ types"
fi

# Step 3: Update test files
echo "📝 Updating test files..."
for testfile in mq/pulsar/pulsar_test.go mq/pulsar/pulsar_integration_test.go; do
    if [ -f "$testfile" ]; then
        sed -i.bak \
            -e 's/"github.com\/astra-go\/astra\/mq\/pulsar"/"github.com\/astra-go\/astra\/mq"/g' \
            -e 's/pulsar\.NewProducer/mq.NewPulsarProducer/g' \
            -e 's/pulsar\.NewConsumer/mq.NewPulsarConsumer/g' \
            -e 's/pulsar\.Config/mq.PulsarConfig/g' \
            -e 's/pulsar\.ProducerConfig/mq.PulsarProducerConfig/g' \
            -e 's/pulsar\.ConsumerConfig/mq.PulsarConsumerConfig/g' \
            -e 's/pulsar\.Producer/mq.PulsarProducer/g' \
            -e 's/pulsar\.Consumer/mq.PulsarConsumer/g' \
            "$testfile"
        echo "  ✓ Updated $testfile"
    fi
done

# Step 4: Clean up backup files
echo "🧹 Cleaning up backup files..."
find mq -name "*.go.bak" -delete

echo "✅ Migration script complete!"
echo "📝 Next steps:"
echo "   1. Review changes: git diff"
echo "   2. Run tests: go test ./mq/..."
echo "   3. Update go.mod: go mod tidy"
