//go:build prometheus

package circuitbreaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/astra-go/astra/circuitbreaker"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var errBoom = errors.New("boom")

// metricValue reads a single counter/gauge value from the registry by metric
// name and label set.
func metricValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if !labelsMatch(m.GetLabel(), labels) {
				continue
			}
			switch v := m.GetValue().(type) {
			case *dto.Metric_Counter:
				return v.Counter.GetValue()
			case *dto.Metric_Gauge:
				return v.Gauge.GetValue()
			}
		}
	}
	t.Fatalf("metric %q with labels %v not found", name, labels)
	return 0
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	got := make(map[string]string, len(pairs))
	for _, p := range pairs {
		got[p.GetName()] = p.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

func TestPrometheus_MetricsRecorded(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := circuitbreaker.New(
		circuitbreaker.WithName("prom"),
		circuitbreaker.WithMaxFailures(1),
		circuitbreaker.WithInterval(20*time.Millisecond),
		circuitbreaker.WithSuccessThreshold(2),
		circuitbreaker.WithPrometheus(reg),
	)

	_ = b.Call(context.Background(), func() error { return errBoom }) // Closed→Open, failure
	time.Sleep(30 * time.Millisecond)
	_ = b.Call(context.Background(), func() error { return nil }) // Open→HalfOpen, success
	_ = b.Call(context.Background(), func() error { return nil }) // HalfOpen→Closed, success

	// State gauge should currently read "closed" == 0.
	state := metricValue(t, reg, "circuit_breaker_state", map[string]string{"name": "prom"})
	if state != 0 {
		t.Errorf("expected state gauge 0 (closed), got %v", state)
	}

	// calls_total: 1 failure + 2 successes = 3 total.
	failCalls := metricValue(t, reg, "circuit_breaker_calls_total",
		map[string]string{"name": "prom", "result": "failure"})
	if failCalls != 1 {
		t.Errorf("expected 1 failure call, got %v", failCalls)
	}
	succCalls := metricValue(t, reg, "circuit_breaker_calls_total",
		map[string]string{"name": "prom", "result": "success"})
	if succCalls != 2 {
		t.Errorf("expected 2 success calls, got %v", succCalls)
	}

	// transitions: Closed→Open, Open→HalfOpen, HalfOpen→Closed.
	trans := metricValue(t, reg, "circuit_breaker_transitions_total",
		map[string]string{"name": "prom", "from": "closed", "to": "open"})
	if trans != 1 {
		t.Errorf("expected 1 closed→open transition, got %v", trans)
	}
}
