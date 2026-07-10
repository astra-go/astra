//go:build prometheus

// Package circuitbreaker — Prometheus metrics integration.
//
// This file is compiled only when the `prometheus` build tag is set, so the
// core circuit breaker has zero external dependencies by default. Enable it
// with:
//
//	go build -tags prometheus ./...
//	go test  -tags prometheus ./circuitbreaker/...
//
// Then wire it up with WithPrometheus:
//
//	cb := circuitbreaker.New(
//		circuitbreaker.WithName("orders"),
//		circuitbreaker.WithPrometheus(prometheus.DefaultRegisterer),
//	)
package circuitbreaker

import (
	"github.com/prometheus/client_golang/prometheus"
)

// prometheusReporter implements MetricsReporter using Prometheus collectors.
type prometheusReporter struct {
	state       *prometheus.GaugeVec
	calls       *prometheus.CounterVec
	transitions *prometheus.CounterVec
}

// NewPrometheusReporter builds a reporter and registers its collectors with
// reg. The "name" label is the breaker name supplied at construction.
func NewPrometheusReporter(name string, reg prometheus.Registerer) MetricsReporter {
	state := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state",
		Help: "Current state of the circuit breaker (0=closed, 1=open, 2=half-open).",
	}, []string{"name"})

	calls := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "circuit_breaker_calls_total",
		Help: "Total number of calls handled by the circuit breaker, by result.",
	}, []string{"name", "result"})

	transitions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "circuit_breaker_transitions_total",
		Help: "Total number of state transitions, labelled by from/to state.",
	}, []string{"name", "from", "to"})

	reg.MustRegister(state, calls, transitions)
	return &prometheusReporter{state: state, calls: calls, transitions: transitions}
}

func (r *prometheusReporter) ReportState(name string, s State) {
	r.state.WithLabelValues(name).Set(float64(s))
}

func (r *prometheusReporter) ReportCall(name, result string) {
	r.calls.WithLabelValues(name, result).Inc()
}

func (r *prometheusReporter) ReportTransition(name string, from, to State) {
	r.transitions.WithLabelValues(name, from.String(), to.String()).Inc()
}

// WithPrometheus attaches a Prometheus-backed MetricsReporter registered with
// reg. Only available when built with -tags prometheus.
func WithPrometheus(reg prometheus.Registerer) Option {
	return func(s *Settings) {
		s.metrics = NewPrometheusReporter(s.Name, reg)
	}
}
