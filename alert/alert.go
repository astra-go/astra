// Package alert provides a rule-based alerting engine for Astra applications.
//
// The engine periodically samples registered metric functions, evaluates
// expression-based rules using github.com/expr-lang/expr, and notifies
// configured channels when a rule transitions into or out of a firing state.
//
// # Quick start
//
//	engine := alert.NewEngine(alert.EngineConfig{EvalInterval: 30 * time.Second})
//
//	// Register metric sources
//	engine.
//	    RegisterMetric("error_rate", func() float64 { return metrics.ErrorRate() }).
//	    RegisterMetric("cpu_usage",  func() float64 { return sys.CPUPercent() })
//
//	// Define rules
//	engine.AddRule(alert.Rule{
//	    Name:     "high-error-rate",
//	    Expr:     "error_rate >= 0.05",
//	    For:      2 * time.Minute,  // must fire for 2 min before notifying
//	    Labels:   map[string]string{"severity": "critical"},
//	    Channels: []string{"webhook"},
//	})
//
//	// Add notification channels
//	engine.AddChannel(&alert.WebhookChannel{
//	    ChannelName: "webhook",
//	    URL:         "https://hooks.example.com/alerts",
//	})
//
//	engine.Start(ctx)
//	defer engine.Stop()
package alert

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

const defaultEvalInterval = 30 * time.Second

// MetricFunc is a function that returns the current value of a metric.
// It must be fast and non-blocking.
type MetricFunc func() float64

// Rule defines a threshold condition and the notification strategy.
type Rule struct {
	// Name is a unique human-readable identifier for this rule.
	Name string

	// Expr is an expression evaluated against the current metric snapshot.
	// Variable names correspond to names passed to RegisterMetric.
	// Examples: "cpu_usage > 90", "error_rate >= 0.05 && latency_p99 > 500"
	Expr string

	// For is the minimum continuous firing duration before a notification is sent.
	// Zero means notify immediately on the first firing evaluation.
	For time.Duration

	// Labels are arbitrary key-value pairs attached to fired alerts.
	Labels map[string]string

	// Channels lists the notification channel names to use for this rule.
	// Names must match those passed to AddChannel.
	Channels []string
}

// Alert is a snapshot of a rule that is firing or has just resolved.
type Alert struct {
	// Rule is a pointer to the rule that generated this alert.
	Rule *Rule

	// Metrics contains the metric snapshot at the time the alert fired.
	Metrics map[string]float64

	// FiredAt is the time the rule first started firing.
	FiredAt time.Time

	// ResolvedAt is the time the alert resolved (zero while still firing).
	ResolvedAt time.Time
}

// EngineConfig configures the alerting engine.
type EngineConfig struct {
	// EvalInterval controls how often metrics are sampled and rules evaluated.
	// Default: 30s.
	EvalInterval time.Duration
}

// ruleState tracks the firing state of one rule.
type ruleState struct {
	rule     Rule
	program  *vm.Program // compiled expr program
	firingAt time.Time   // when the rule started continuously firing (zero = not firing)
	notified bool        // whether a notification has already been sent for the current fire
	alert    *Alert      // current active alert (nil when not firing)
}

// Engine evaluates rules on a ticker and dispatches alerts to channels.
type Engine struct {
	cfg      EngineConfig
	mu       sync.RWMutex
	metrics  map[string]MetricFunc
	states   []*ruleState
	channels map[string]Channel
	stopCh   chan struct{}
	log      *slog.Logger
}

// NewEngine creates a new alerting Engine.
func NewEngine(cfg EngineConfig) *Engine {
	if cfg.EvalInterval <= 0 {
		cfg.EvalInterval = defaultEvalInterval
	}
	return &Engine{
		cfg:      cfg,
		metrics:  make(map[string]MetricFunc),
		channels: make(map[string]Channel),
		stopCh:   make(chan struct{}),
		log:      slog.Default(),
	}
}

// RegisterMetric adds a named metric source.
// Returns the Engine for chaining.
func (e *Engine) RegisterMetric(name string, fn MetricFunc) *Engine {
	e.mu.Lock()
	e.metrics[name] = fn
	e.mu.Unlock()
	return e
}

// AddRule compiles and registers a rule. Returns an error if the expression
// is invalid or a rule with the same Name already exists.
func (e *Engine) AddRule(r Rule) error {
	// Build a sample env with float64 zeros to validate compilation.
	e.mu.RLock()
	env := e.sampleMetrics()
	e.mu.RUnlock()

	// If no metrics registered yet, compile with an empty map.
	if len(env) == 0 {
		env = map[string]float64{}
	}

	program, err := expr.Compile(r.Expr,
		expr.Env(env),
		expr.AsBool(),
	)
	if err != nil {
		return &RuleCompileError{Rule: r.Name, Err: err}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, s := range e.states {
		if s.rule.Name == r.Name {
			return &DuplicateRuleError{Name: r.Name}
		}
	}
	e.states = append(e.states, &ruleState{rule: r, program: program})
	return nil
}

// AddChannel registers a notification channel. Returns the Engine for chaining.
func (e *Engine) AddChannel(ch Channel) *Engine {
	e.mu.Lock()
	e.channels[ch.Name()] = ch
	e.mu.Unlock()
	return e
}

// Start begins the evaluation loop in a background goroutine.
func (e *Engine) Start(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.EvalInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				e.evaluate(ctx)
			case <-e.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop halts the evaluation loop.
func (e *Engine) Stop() {
	close(e.stopCh)
}

// ActiveAlerts returns a snapshot of all currently firing alerts.
func (e *Engine) ActiveAlerts() []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var out []Alert
	for _, s := range e.states {
		if s.alert != nil && s.alert.ResolvedAt.IsZero() {
			out = append(out, *s.alert)
		}
	}
	return out
}

// evaluate samples metrics and processes each rule.
func (e *Engine) evaluate(ctx context.Context) {
	e.mu.Lock()
	snapshot := e.sampleMetrics()
	states := e.states
	e.mu.Unlock()

	now := time.Now()

	for _, s := range states {
		firing, err := evalRule(s.program, snapshot)
		if err != nil {
			e.log.Warn("alert: rule eval error", "rule", s.rule.Name, "err", err)
			continue
		}

		e.mu.Lock()
		if firing {
			if s.firingAt.IsZero() {
				s.firingAt = now
			}
			// Check For duration
			if !s.notified && now.Sub(s.firingAt) >= s.rule.For {
				a := &Alert{
					Rule:    &s.rule,
					Metrics: snapshot,
					FiredAt: s.firingAt,
				}
				s.alert = a
				s.notified = true
				e.mu.Unlock()
				e.notify(ctx, a)
				continue
			}
		} else {
			if s.notified && s.alert != nil {
				// Rule resolved — notify resolution.
				s.alert.ResolvedAt = now
				resolved := *s.alert
				s.firingAt = time.Time{}
				s.notified = false
				s.alert = nil
				e.mu.Unlock()
				e.notify(ctx, &resolved)
				continue
			}
			s.firingAt = time.Time{}
			s.notified = false
		}
		e.mu.Unlock()
	}
}

// sampleMetrics collects the current value of every registered metric.
// Must be called with mu held (read or write).
func (e *Engine) sampleMetrics() map[string]float64 {
	snap := make(map[string]float64, len(e.metrics))
	for name, fn := range e.metrics {
		snap[name] = fn()
	}
	return snap
}

// evalRule runs the compiled expr program against the metric snapshot.
func evalRule(program *vm.Program, env map[string]float64) (bool, error) {
	out, err := expr.Run(program, env)
	if err != nil {
		return false, err
	}
	result, ok := out.(bool)
	if !ok {
		return false, nil
	}
	return result, nil
}

// notify dispatches an alert to all configured channels.
func (e *Engine) notify(ctx context.Context, a *Alert) {
	e.mu.RLock()
	channels := make([]Channel, 0, len(a.Rule.Channels))
	for _, name := range a.Rule.Channels {
		if ch, ok := e.channels[name]; ok {
			channels = append(channels, ch)
		} else {
			e.log.Warn("alert: channel not found", "channel", name, "rule", a.Rule.Name)
		}
	}
	e.mu.RUnlock()

	for _, ch := range channels {
		if err := ch.Send(ctx, a); err != nil {
			e.log.Error("alert: send failed", "channel", ch.Name(), "rule", a.Rule.Name, "err", err)
		}
	}
}

// ─── Errors ───────────────────────────────────────────────────────────────────

// RuleCompileError is returned when an expression cannot be compiled.
type RuleCompileError struct {
	Rule string
	Err  error
}

func (e *RuleCompileError) Error() string {
	return "alert: compile rule " + e.Rule + ": " + e.Err.Error()
}
func (e *RuleCompileError) Unwrap() error { return e.Err }

// DuplicateRuleError is returned when AddRule is called with a duplicate name.
type DuplicateRuleError struct{ Name string }

func (e *DuplicateRuleError) Error() string {
	return "alert: rule " + e.Name + " already registered"
}
