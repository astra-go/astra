package quic

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/logging"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const quicScope = "astra.quic"

// quicMetrics holds the OTel instruments shared across all QUIC connections.
// Created once in RunQUICWithOptions and referenced by every connectionTracer.
type quicMetrics struct {
	connectionsActive metric.Int64UpDownCounter
	connectionsTotal  metric.Int64Counter
	handshakeDuration metric.Float64Histogram
	zeroRTTTotal      metric.Int64Counter
	migrationTotal    metric.Int64Counter
}

func newQUICMetrics(mp metric.MeterProvider) (*quicMetrics, error) {
	m := mp.Meter(quicScope)

	connectionsActive, err := m.Int64UpDownCounter("astra.quic.connections.active",
		metric.WithDescription("Number of active QUIC connections."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	connectionsTotal, err := m.Int64Counter("astra.quic.connections.total",
		metric.WithDescription("Total QUIC connections closed, partitioned by result."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	handshakeDuration, err := m.Float64Histogram("astra.quic.handshake.duration",
		metric.WithDescription("QUIC handshake duration from connection start to version negotiation."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(.005, .01, .025, .05, .1, .25, .5, 1),
	)
	if err != nil {
		return nil, err
	}

	zeroRTTTotal, err := m.Int64Counter("astra.quic.handshake.zero_rtt",
		metric.WithDescription("Number of connections where 0-RTT transport parameters were restored."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	migrationTotal, err := m.Int64Counter("astra.quic.migration.events",
		metric.WithDescription("Number of detected QUIC path migration events (remote address changes)."),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	return &quicMetrics{
		connectionsActive: connectionsActive,
		connectionsTotal:  connectionsTotal,
		handshakeDuration: handshakeDuration,
		zeroRTTTotal:      zeroRTTTotal,
		migrationTotal:    migrationTotal,
	}, nil
}

// newConnectionTracer returns the quic.Config.Tracer factory function.
// The returned factory creates one connectionTracer per QUIC connection.
func (qm *quicMetrics) newTracerFactory() func(context.Context, logging.Perspective, quic.ConnectionID) *logging.ConnectionTracer {
	return func(ctx context.Context, _ logging.Perspective, _ quic.ConnectionID) *logging.ConnectionTracer {
		ct := &connectionTracer{
			metrics:   qm,
			ctx:       ctx,
			startTime: time.Now(),
		}
		return ct.asLoggingTracer()
	}
}

// connectionTracer tracks per-connection state and records OTel metrics.
type connectionTracer struct {
	metrics   *quicMetrics
	ctx       context.Context
	startTime time.Time

	mu            sync.Mutex
	remoteAddr    net.Addr
	handshakeDone bool
}

func (ct *connectionTracer) asLoggingTracer() *logging.ConnectionTracer {
	return &logging.ConnectionTracer{
		StartedConnection: func(local, remote net.Addr, _, _ logging.ConnectionID) {
			ct.mu.Lock()
			ct.remoteAddr = remote
			ct.mu.Unlock()
			ct.metrics.connectionsActive.Add(ct.ctx, 1)
		},

		// NegotiatedVersion fires once the QUIC version handshake completes.
		NegotiatedVersion: func(_ logging.Version, _, _ []logging.Version) {
			ct.mu.Lock()
			if !ct.handshakeDone {
				ct.handshakeDone = true
				duration := time.Since(ct.startTime).Seconds()
				ct.mu.Unlock()
				ct.metrics.handshakeDuration.Record(ct.ctx, duration)
				return
			}
			ct.mu.Unlock()
		},

		// RestoredTransportParameters fires when 0-RTT session data is reused.
		RestoredTransportParameters: func(_ *logging.TransportParameters) {
			ct.metrics.zeroRTTTotal.Add(ct.ctx, 1)
		},

		ClosedConnection: func(err error) {
			ct.metrics.connectionsActive.Add(ct.ctx, -1)

			result := "success"
			if err != nil {
				result = "error"
			}
			ct.metrics.connectionsTotal.Add(ct.ctx, 1,
				metric.WithAttributes(attribute.String("result", result)),
			)

			// Record handshake duration for connections that closed before
			// NegotiatedVersion fired (e.g. rejected during handshake).
			ct.mu.Lock()
			done := ct.handshakeDone
			start := ct.startTime
			ct.mu.Unlock()
			if !done {
				ct.metrics.handshakeDuration.Record(ct.ctx, time.Since(start).Seconds(),
					metric.WithAttributes(attribute.String("result", "failed")),
				)
			}
		},
	}
}

// recordMigration is called when we detect a remote address change.
// Exported for use in future quic-go versions that may add a dedicated callback.
func (ct *connectionTracer) recordMigration(newRemote net.Addr) {
	ct.mu.Lock()
	prev := ct.remoteAddr
	if prev != nil && prev.String() != newRemote.String() {
		ct.remoteAddr = newRemote
		ct.mu.Unlock()
		ct.metrics.migrationTotal.Add(ct.ctx, 1)
		return
	}
	ct.mu.Unlock()
}
