package quic

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/quic-go/quic-go/logging"
)

func newTestMeterProvider(t *testing.T) (*sdkmetric.MeterProvider, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	exp, err := promexporter.New(promexporter.WithRegisterer(reg))
	if err != nil {
		t.Fatalf("promexporter.New: %v", err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exp))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return mp, reg
}

// flush forces pending OTel measurements into the Prometheus registry.
func flush(t *testing.T, mp *sdkmetric.MeterProvider) {
	t.Helper()
	if err := mp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
}

func gatherGauge(t *testing.T, mp *sdkmetric.MeterProvider, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	flush(t, mp)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			for _, m := range mf.GetMetric() {
				if g := m.GetGauge(); g != nil {
					return g.GetValue()
				}
			}
		}
	}
	return 0
}

func gatherCounterByLabel(t *testing.T, mp *sdkmetric.MeterProvider, reg *prometheus.Registry, name, labelKey, labelVal string) float64 {
	t.Helper()
	flush(t, mp)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == labelKey && lp.GetValue() == labelVal {
					if c := m.GetCounter(); c != nil {
						return c.GetValue()
					}
				}
			}
		}
	}
	return 0
}

func gatherHistogramCount(t *testing.T, mp *sdkmetric.MeterProvider, reg *prometheus.Registry, name string) uint64 {
	t.Helper()
	flush(t, mp)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			for _, m := range mf.GetMetric() {
				if h := m.GetHistogram(); h != nil {
					return h.GetSampleCount()
				}
			}
		}
	}
	return 0
}

func gatherCounterTotal(t *testing.T, mp *sdkmetric.MeterProvider, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	flush(t, mp)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var total float64
	for _, mf := range mfs {
		if mf.GetName() == name {
			for _, m := range mf.GetMetric() {
				if c := m.GetCounter(); c != nil {
					total += c.GetValue()
				}
			}
		}
	}
	return total
}

func TestQUICMetrics_ConnectionLifecycle(t *testing.T) {
	mp, reg := newTestMeterProvider(t)
	qm, err := newQUICMetrics(mp)
	if err != nil {
		t.Fatalf("newQUICMetrics: %v", err)
	}

	ct := &connectionTracer{metrics: qm, ctx: context.Background()}
	tracer := ct.asLoggingTracer()

	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	remote := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 443}

	tracer.StartedConnection(local, remote, logging.ConnectionID{}, logging.ConnectionID{})

	active := gatherGauge(t, mp, reg, "astra_quic_connections_active")
	if active != 1 {
		t.Errorf("active connections = %.0f, want 1", active)
	}

	tracer.ClosedConnection(nil)

	active = gatherGauge(t, mp, reg, "astra_quic_connections_active")
	if active != 0 {
		t.Errorf("active connections after close = %.0f, want 0", active)
	}

	success := gatherCounterByLabel(t, mp, reg, "astra_quic_connections_total", "result", "success")
	if success != 1 {
		t.Errorf("connections_total{result=success} = %.0f, want 1", success)
	}
}

func TestQUICMetrics_ConnectionError(t *testing.T) {
	mp, reg := newTestMeterProvider(t)
	qm, err := newQUICMetrics(mp)
	if err != nil {
		t.Fatalf("newQUICMetrics: %v", err)
	}

	ct := &connectionTracer{metrics: qm, ctx: context.Background()}
	tracer := ct.asLoggingTracer()

	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	remote := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 443}
	tracer.StartedConnection(local, remote, logging.ConnectionID{}, logging.ConnectionID{})
	tracer.ClosedConnection(errors.New("timeout"))

	errCount := gatherCounterByLabel(t, mp, reg, "astra_quic_connections_total", "result", "error")
	if errCount != 1 {
		t.Errorf("connections_total{result=error} = %.0f, want 1", errCount)
	}
}

func TestQUICMetrics_HandshakeDuration_Success(t *testing.T) {
	mp, reg := newTestMeterProvider(t)
	qm, err := newQUICMetrics(mp)
	if err != nil {
		t.Fatalf("newQUICMetrics: %v", err)
	}

	ct := &connectionTracer{metrics: qm, ctx: context.Background()}
	tracer := ct.asLoggingTracer()

	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	remote := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 443}
	tracer.StartedConnection(local, remote, logging.ConnectionID{}, logging.ConnectionID{})
	tracer.NegotiatedVersion(0, nil, nil)
	tracer.ClosedConnection(nil)

	count := gatherHistogramCount(t, mp, reg, "astra_quic_handshake_duration_seconds")
	if count != 1 {
		t.Errorf("handshake_duration sample count = %d, want 1", count)
	}
}

func TestQUICMetrics_HandshakeDuration_FailedBeforeNegotiation(t *testing.T) {
	mp, reg := newTestMeterProvider(t)
	qm, err := newQUICMetrics(mp)
	if err != nil {
		t.Fatalf("newQUICMetrics: %v", err)
	}

	ct := &connectionTracer{metrics: qm, ctx: context.Background()}
	tracer := ct.asLoggingTracer()

	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	remote := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 443}
	tracer.StartedConnection(local, remote, logging.ConnectionID{}, logging.ConnectionID{})
	tracer.ClosedConnection(errors.New("handshake rejected"))

	count := gatherHistogramCount(t, mp, reg, "astra_quic_handshake_duration_seconds")
	if count != 1 {
		t.Errorf("handshake_duration sample count = %d, want 1 (failed path)", count)
	}
}

func TestQUICMetrics_ZeroRTT(t *testing.T) {
	mp, reg := newTestMeterProvider(t)
	qm, err := newQUICMetrics(mp)
	if err != nil {
		t.Fatalf("newQUICMetrics: %v", err)
	}

	ct := &connectionTracer{metrics: qm, ctx: context.Background()}
	tracer := ct.asLoggingTracer()

	tracer.RestoredTransportParameters(nil)

	total := gatherCounterTotal(t, mp, reg, "astra_quic_handshake_zero_rtt_total")
	if total != 1 {
		t.Errorf("zero_rtt total = %.0f, want 1", total)
	}
}

func TestQUICMetrics_MigrationDetection(t *testing.T) {
	mp, reg := newTestMeterProvider(t)
	qm, err := newQUICMetrics(mp)
	if err != nil {
		t.Fatalf("newQUICMetrics: %v", err)
	}

	ct := &connectionTracer{metrics: qm, ctx: context.Background()}
	tracer := ct.asLoggingTracer()

	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	remote1 := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 443}
	remote2 := &net.TCPAddr{IP: net.ParseIP("192.168.1.5"), Port: 443} // WiFi→4G

	tracer.StartedConnection(local, remote1, logging.ConnectionID{}, logging.ConnectionID{})
	ct.recordMigration(remote2)

	migrations := gatherCounterTotal(t, mp, reg, "astra_quic_migration_events_total")
	if migrations != 1 {
		t.Errorf("migration_events = %.0f, want 1", migrations)
	}
}

func TestQUICMetrics_MigrationNoFalsePositive(t *testing.T) {
	mp, reg := newTestMeterProvider(t)
	qm, err := newQUICMetrics(mp)
	if err != nil {
		t.Fatalf("newQUICMetrics: %v", err)
	}

	ct := &connectionTracer{metrics: qm, ctx: context.Background()}
	tracer := ct.asLoggingTracer()

	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	remote := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 443}
	tracer.StartedConnection(local, remote, logging.ConnectionID{}, logging.ConnectionID{})
	ct.recordMigration(remote) // same address — no migration

	migrations := gatherCounterTotal(t, mp, reg, "astra_quic_migration_events_total")
	if migrations != 0 {
		t.Errorf("migration_events = %.0f, want 0 for same address", migrations)
	}
}

func TestNewQUICMetrics_ReturnsNonNil(t *testing.T) {
	mp, _ := newTestMeterProvider(t)
	qm, err := newQUICMetrics(mp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if qm == nil {
		t.Fatal("expected non-nil quicMetrics")
	}
}
