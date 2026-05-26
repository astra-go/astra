package alert_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astra-go/astra/alert"
	"github.com/astra-go/astra/testutil"
)

// ─── Engine — basic setup ─────────────────────────────────────────────────────

func TestEngine_AddRule_ValidExpr(t *testing.T) {
	e := alert.NewEngine(alert.EngineConfig{})
	e.RegisterMetric("cpu", func() float64 { return 50 })
	err := e.AddRule(alert.Rule{Name: "high-cpu", Expr: "cpu > 90"})
	testutil.AssertNoError(t, err)
}

func TestEngine_AddRule_InvalidExpr_ReturnsError(t *testing.T) {
	e := alert.NewEngine(alert.EngineConfig{})
	e.RegisterMetric("cpu", func() float64 { return 50 })
	err := e.AddRule(alert.Rule{Name: "bad", Expr: "cpu >>>"})
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}
	var rce *alert.RuleCompileError
	if !errors.As(err, &rce) {
		t.Errorf("expected RuleCompileError, got %T: %v", err, err)
	}
}

func TestEngine_AddRule_DuplicateName_ReturnsError(t *testing.T) {
	e := alert.NewEngine(alert.EngineConfig{})
	e.RegisterMetric("x", func() float64 { return 1 })
	_ = e.AddRule(alert.Rule{Name: "dup", Expr: "x > 0"})
	err := e.AddRule(alert.Rule{Name: "dup", Expr: "x > 0"})
	if err == nil {
		t.Fatal("expected DuplicateRuleError")
	}
	var dre *alert.DuplicateRuleError
	if !errors.As(err, &dre) {
		t.Errorf("expected DuplicateRuleError, got %T: %v", err, err)
	}
}

// ─── Engine — evaluation cycle ────────────────────────────────────────────────

func TestEngine_FiresAlertWhenExprTrue(t *testing.T) {
	var mu sync.Mutex
	var sent []*alert.Alert

	ch := &captureChannel{name: "cap", fn: func(a *alert.Alert) {
		mu.Lock()
		sent = append(sent, a)
		mu.Unlock()
	}}

	var val atomic.Int64 // stores value * 100 to represent float
	e := alert.NewEngine(alert.EngineConfig{EvalInterval: 20 * time.Millisecond})
	e.RegisterMetric("v", func() float64 { return float64(val.Load()) })
	e.AddChannel(ch)
	_ = e.AddRule(alert.Rule{Name: "r", Expr: "v > 50", Channels: []string{"cap"}})

	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)

	// Trigger: value above threshold.
	val.Store(100)
	time.Sleep(100 * time.Millisecond)
	cancel()

	mu.Lock()
	n := len(sent)
	mu.Unlock()
	if n == 0 {
		t.Error("expected at least one alert notification")
	}
}

func TestEngine_DoesNotFireWhenExprFalse(t *testing.T) {
	var mu sync.Mutex
	var sent []*alert.Alert

	ch := &captureChannel{name: "cap", fn: func(a *alert.Alert) {
		mu.Lock()
		sent = append(sent, a)
		mu.Unlock()
	}}

	e := alert.NewEngine(alert.EngineConfig{EvalInterval: 20 * time.Millisecond})
	e.RegisterMetric("v", func() float64 { return 10 }) // always below 50
	e.AddChannel(ch)
	_ = e.AddRule(alert.Rule{Name: "r", Expr: "v > 50", Channels: []string{"cap"}})

	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	cancel()

	mu.Lock()
	n := len(sent)
	mu.Unlock()
	if n != 0 {
		t.Errorf("expected no alerts when condition is false, got %d", n)
	}
}

func TestEngine_ForDuration_DelaysNotification(t *testing.T) {
	var mu sync.Mutex
	var sent []*alert.Alert

	ch := &captureChannel{name: "cap", fn: func(a *alert.Alert) {
		mu.Lock()
		sent = append(sent, a)
		mu.Unlock()
	}}

	e := alert.NewEngine(alert.EngineConfig{EvalInterval: 20 * time.Millisecond})
	e.RegisterMetric("v", func() float64 { return 100 })
	e.AddChannel(ch)
	_ = e.AddRule(alert.Rule{
		Name:     "r",
		Expr:     "v > 50",
		For:      150 * time.Millisecond, // must fire for 150ms before notifying
		Channels: []string{"cap"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)

	// Wait short — should not have fired yet.
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	earlyCount := len(sent)
	mu.Unlock()
	if earlyCount != 0 {
		t.Errorf("alert should not fire before For duration; got %d notifications", earlyCount)
	}

	// Wait long enough — should now have fired.
	time.Sleep(200 * time.Millisecond)
	cancel()

	mu.Lock()
	lateCount := len(sent)
	mu.Unlock()
	if lateCount == 0 {
		t.Error("expected alert after For duration elapsed")
	}
}

func TestEngine_ActiveAlerts_ReturnsFiring(t *testing.T) {
	e := alert.NewEngine(alert.EngineConfig{EvalInterval: 20 * time.Millisecond})
	e.RegisterMetric("v", func() float64 { return 100 })
	e.AddChannel(&captureChannel{name: "c", fn: func(*alert.Alert) {}})
	_ = e.AddRule(alert.Rule{Name: "r", Expr: "v > 50", Channels: []string{"c"}})

	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	time.Sleep(80 * time.Millisecond)

	active := e.ActiveAlerts()
	cancel()

	if len(active) == 0 {
		t.Error("expected at least one active alert")
	}
	testutil.AssertEqual(t, "r", active[0].Rule.Name)
}

func TestEngine_Stop_HaltsEvaluation(t *testing.T) {
	var mu sync.Mutex
	count := 0

	ch := &captureChannel{name: "c", fn: func(*alert.Alert) {
		mu.Lock()
		count++
		mu.Unlock()
	}}

	e := alert.NewEngine(alert.EngineConfig{EvalInterval: 20 * time.Millisecond})
	e.RegisterMetric("v", func() float64 { return 100 })
	e.AddChannel(ch)
	_ = e.AddRule(alert.Rule{Name: "r", Expr: "v > 0", Channels: []string{"c"}})

	ctx := context.Background()
	e.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	e.Stop()

	mu.Lock()
	before := count
	mu.Unlock()

	// After Stop, no more notifications should arrive.
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	after := count
	mu.Unlock()

	if after != before {
		t.Errorf("Stop() did not halt evaluation: count before=%d after=%d", before, after)
	}
}

func TestEngine_RegisterMetric_Chaining(t *testing.T) {
	e := alert.NewEngine(alert.EngineConfig{})
	returned := e.
		RegisterMetric("a", func() float64 { return 1 }).
		RegisterMetric("b", func() float64 { return 2 })
	if returned == nil {
		t.Error("RegisterMetric should return Engine for chaining")
	}
}

func TestEngine_AddChannel_Chaining(t *testing.T) {
	e := alert.NewEngine(alert.EngineConfig{})
	returned := e.AddChannel(&captureChannel{name: "c", fn: func(*alert.Alert) {}})
	if returned == nil {
		t.Error("AddChannel should return Engine for chaining")
	}
}

// ─── WebhookChannel ───────────────────────────────────────────────────────────

func TestWebhookChannel_SendsJSON(t *testing.T) {
	var mu sync.Mutex
	var received []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
			return
		}
		mu.Lock()
		received = append(received, payload)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := &alert.WebhookChannel{ChannelName: "wh", URL: srv.URL, Timeout: time.Second}
	a := &alert.Alert{
		Rule:    &alert.Rule{Name: "test-rule", Expr: "x > 0"},
		Metrics: map[string]float64{"x": 1},
		FiredAt: time.Now(),
	}

	err := ch.Send(context.Background(), a)
	testutil.AssertNoError(t, err)

	mu.Lock()
	n := len(received)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 webhook call, got %d", n)
	}
	if received[0]["rule"] != "test-rule" {
		t.Errorf("payload.rule = %v, want test-rule", received[0]["rule"])
	}
	// "resolved" should be false for a non-resolved alert.
	if received[0]["resolved"] != false {
		t.Errorf("payload.resolved = %v, want false", received[0]["resolved"])
	}
}

func TestWebhookChannel_ResolvedAlert_HasResolvedAt(t *testing.T) {
	var mu sync.Mutex
	var received []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload) //nolint
		mu.Lock()
		received = append(received, payload)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := &alert.WebhookChannel{ChannelName: "wh", URL: srv.URL, Timeout: time.Second}
	a := &alert.Alert{
		Rule:       &alert.Rule{Name: "r", Expr: "x > 0"},
		Metrics:    map[string]float64{},
		FiredAt:    time.Now().Add(-time.Minute),
		ResolvedAt: time.Now(),
	}
	err := ch.Send(context.Background(), a)
	testutil.AssertNoError(t, err)

	mu.Lock()
	n := len(received)
	mu.Unlock()
	if n == 0 {
		t.Fatal("no webhook call received")
	}
	if received[0]["resolved"] != true {
		t.Errorf("payload.resolved = %v, want true", received[0]["resolved"])
	}
	if _, ok := received[0]["resolved_at"]; !ok {
		t.Error("payload missing resolved_at field")
	}
}

func TestWebhookChannel_ErrorOn4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	ch := &alert.WebhookChannel{ChannelName: "wh", URL: srv.URL, Timeout: time.Second}
	a := &alert.Alert{
		Rule:    &alert.Rule{Name: "r", Expr: "x > 0"},
		Metrics: map[string]float64{},
		FiredAt: time.Now(),
	}
	err := ch.Send(context.Background(), a)
	if err == nil {
		t.Error("expected error on 4xx response")
	}
}

func TestWebhookChannel_ErrorOnBadURL(t *testing.T) {
	ch := &alert.WebhookChannel{ChannelName: "wh", URL: "http://127.0.0.1:0", Timeout: 50 * time.Millisecond}
	a := &alert.Alert{
		Rule:    &alert.Rule{Name: "r", Expr: "x > 0"},
		Metrics: map[string]float64{},
		FiredAt: time.Now(),
	}
	err := ch.Send(context.Background(), a)
	if err == nil {
		t.Error("expected error on connection refused")
	}
}

func TestWebhookChannel_Name(t *testing.T) {
	ch := &alert.WebhookChannel{ChannelName: "my-webhook"}
	testutil.AssertEqual(t, "my-webhook", ch.Name())
}

// ─── LogChannel ───────────────────────────────────────────────────────────────

func TestLogChannel_SendDoesNotPanic(t *testing.T) {
	ch := &alert.LogChannel{ChannelName: "log"}
	a := &alert.Alert{
		Rule:    &alert.Rule{Name: "r", Expr: "x > 0", Labels: map[string]string{"env": "test"}},
		Metrics: map[string]float64{"x": 5},
		FiredAt: time.Now(),
	}
	err := ch.Send(context.Background(), a)
	testutil.AssertNoError(t, err)
}

func TestLogChannel_ResolvedAlert_DoesNotPanic(t *testing.T) {
	ch := &alert.LogChannel{ChannelName: "log"}
	a := &alert.Alert{
		Rule:       &alert.Rule{Name: "r", Expr: "x > 0"},
		Metrics:    map[string]float64{"x": 0},
		FiredAt:    time.Now().Add(-time.Minute),
		ResolvedAt: time.Now(),
	}
	err := ch.Send(context.Background(), a)
	testutil.AssertNoError(t, err)
}

func TestLogChannel_NilLogger_UsesDefault(t *testing.T) {
	ch := &alert.LogChannel{ChannelName: "log", Logger: nil}
	a := &alert.Alert{
		Rule:    &alert.Rule{Name: "r", Expr: "x > 0"},
		Metrics: map[string]float64{},
		FiredAt: time.Now(),
	}
	err := ch.Send(context.Background(), a)
	testutil.AssertNoError(t, err)
}

func TestLogChannel_Name(t *testing.T) {
	ch := &alert.LogChannel{ChannelName: "mylog"}
	testutil.AssertEqual(t, "mylog", ch.Name())
}

// ─── helpers ──────────────────────────────────────────────────────────────────

type captureChannel struct {
	name string
	fn   func(*alert.Alert)
}

func (c *captureChannel) Name() string { return c.name }
func (c *captureChannel) Send(_ context.Context, a *alert.Alert) error {
	c.fn(a)
	return nil
}

// Compile-time interface checks.
var _ alert.Channel = (*alert.WebhookChannel)(nil)
var _ alert.Channel = (*alert.LogChannel)(nil)
var _ alert.Channel = (*captureChannel)(nil)
