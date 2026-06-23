// Package mq provides message queue abstractions with pluggable backends.
//
// This subpackage includes observability metrics via Prometheus.
package mq

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all MQ observability metrics.
// All fields are pre-registered with the default Prometheus registerer.
type Metrics struct {
	// Publish metrics
	PublishTotal   *prometheus.CounterVec // {topic, broker, status}
	PublishLatency *prometheus.HistogramVec // {topic, broker}

	// Consume metrics
	ConsumeTotal   *prometheus.CounterVec // {topic, broker, status}
	ConsumeLatency *prometheus.HistogramVec // {topic, broker}

	// Consumer lag (for streaming brokers)
	ConsumerLag *prometheus.GaugeVec // {topic, partition}

	// Memory broker queue depth
	MemoryQueueDepth *prometheus.GaugeVec // {topic}

	// Delay queue
	DelayQueueSize      prometheus.Gauge
	DelayMessageTotal   *prometheus.CounterVec // {topic}

	// Active connections / producers
	ActiveProducers  prometheus.Gauge
	ActiveConsumers  prometheus.Gauge
}

var globalMetrics *Metrics

func init() {
	globalMetrics = NewMetrics()
}

// DefaultMetrics returns the default (singleton) MQ metrics instance.
func DefaultMetrics() *Metrics { return globalMetrics }

// NewMetrics creates and registers a new Metrics instance.
// All metrics are registered with promauto using the default registerer.
func NewMetrics() *Metrics {
	m := &Metrics{
		PublishTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mq",
				Subsystem: "publish",
				Name:       "total",
				Help:       "Total number of published messages.",
			},
			[]string{"topic", "broker", "status"},
		),

		PublishLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "mq",
				Subsystem: "publish",
				Name:       "duration_seconds",
				Help:       "Publish latency in seconds.",
				Buckets:    []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
			},
			[]string{"topic", "broker"},
		),

		ConsumeTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mq",
				Subsystem: "consume",
				Name:       "total",
				Help:       "Total number of consumed messages.",
			},
			[]string{"topic", "broker", "status"},
		),

		ConsumeLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "mq",
				Subsystem: "consume",
				Name:       "duration_seconds",
				Help:       "Consume (handler execution) latency in seconds.",
				Buckets:    []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
			},
			[]string{"topic", "broker"},
		),

		ConsumerLag: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "mq",
				Subsystem: "consumer",
				Name:       "lag",
				Help:       "Consumer lag (messages behind).",
			},
			[]string{"topic", "partition"},
		),

		MemoryQueueDepth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "mq",
				Subsystem: "memory",
				Name:       "queue_depth",
				Help:       "Current queue depth for the memory broker.",
			},
			[]string{"topic"},
		),

		DelayQueueSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "mq",
				Subsystem: "delay",
				Name:       "queue_size",
				Help:       "Total number of delayed messages waiting to be delivered.",
			},
		),

		DelayMessageTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mq",
				Subsystem: "delay",
				Name:       "message_total",
				Help:       "Total number of delayed messages enqueued.",
			},
			[]string{"topic"},
		),

		ActiveProducers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "mq",
				Name:     "producers_active",
				Help:     "Number of currently active producers.",
			},
		),

		ActiveConsumers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "mq",
				Name:     "consumers_active",
				Help:     "Number of currently active consumers.",
			},
		),
	}
	return m
}

// RecordPublish records a publish operation.
func (m *Metrics) RecordPublish(topic, broker, status string, latency time.Duration) {
	m.PublishTotal.WithLabelValues(topic, broker, status).Inc()
	m.PublishLatency.WithLabelValues(topic, broker).Observe(latency.Seconds())
}

// RecordConsume records a consume operation.
func (m *Metrics) RecordConsume(topic, broker, status string, latency time.Duration) {
	m.ConsumeTotal.WithLabelValues(topic, broker, status).Inc()
	m.ConsumeLatency.WithLabelValues(topic, broker).Observe(latency.Seconds())
}

// SetConsumerLag sets the consumer lag for a topic/partition.
func (m *Metrics) SetConsumerLag(topic, partition string, lag float64) {
	m.ConsumerLag.WithLabelValues(topic, partition).Set(lag)
}

// SetMemoryQueueDepth sets the current memory broker queue depth for a topic.
func (m *Metrics) SetMemoryQueueDepth(topic string, depth int64) {
	m.MemoryQueueDepth.WithLabelValues(topic).Set(float64(depth))
}

// IncDelayQueueSize increments the delay queue size by 1.
func (m *Metrics) IncDelayQueueSize() {
	m.DelayQueueSize.Inc()
}

// DecDelayQueueSize decrements the delay queue size by 1.
func (m *Metrics) DecDelayQueueSize() {
	m.DelayQueueSize.Dec()
}

// SetDelayQueueSize sets the absolute delay queue size.
func (m *Metrics) SetDelayQueueSize(n float64) {
	m.DelayQueueSize.Set(n)
}

// RecordDelayMessage records a delayed message being enqueued.
func (m *Metrics) RecordDelayMessage(topic string) {
	m.DelayMessageTotal.WithLabelValues(topic).Inc()
}

// IncActiveProducer increments the active producers counter.
func (m *Metrics) IncActiveProducer() {
	m.ActiveProducers.Inc()
}

// DecActiveProducer decrements the active producers counter.
func (m *Metrics) DecActiveProducer() {
	m.ActiveProducers.Dec()
}

// IncActiveConsumer increments the active consumers counter.
func (m *Metrics) IncActiveConsumer() {
	m.ActiveConsumers.Inc()
}

// DecActiveConsumer decrements the active consumers counter.
func (m *Metrics) DecActiveConsumer() {
	m.ActiveConsumers.Dec()
}

// ----------------------------------------------------------------
// InstrumentedProducer wraps a Producer with metrics collection.
// ----------------------------------------------------------------

// instrumentedProducer provides metrics-instrumented Publish.
type instrumentedProducer struct {
	Producer
	topic  string
	broker string
}

// Publish records latency then delegates to the underlying Producer.
func (p *instrumentedProducer) Publish(ctx context.Context, msg *Message) error {
	start := time.Now()
	err := p.Producer.Publish(ctx, msg)
	status := "ok"
	if err != nil {
		status = "error"
	}
	DefaultMetrics().RecordPublish(p.topic, p.broker, status, time.Since(start))
	return err
}

// ----------------------------------------------------------------
// observabilityCounter is a simple atomic counter for internal use.
// Used for memory broker queue depth updates.
// ----------------------------------------------------------------

type observabilityCounter struct {
	n int64
}

func (c *observabilityCounter) Add(delta int64) { atomic.AddInt64(&c.n, delta) }
func (c *observabilityCounter) Load() int64    { return atomic.LoadInt64(&c.n) }
