package alert

import (
	"context"

	"github.com/astra-go/astra"
)

// Module wraps an Engine as an astra.Module, binding the engine's Start and
// Stop to the application's lifecycle hooks.
//
// Typical usage:
//
//	e := alert.NewEngine(alert.EngineConfig{EvalInterval: 30 * time.Second})
//	e.RegisterMetric("cpu", func() float64 { return cpuUsage() })
//	e.AddChannel(&alert.WebhookChannel{
//	    ChannelName: "ops-webhook",
//	    URL:         os.Getenv("ALERT_WEBHOOK_URL"),
//	})
//	_ = e.AddRule(alert.Rule{
//	    Name:     "high-cpu",
//	    Expr:     "cpu > 90",
//	    For:      2 * time.Minute,
//	    Channels: []string{"ops-webhook"},
//	})
//
//	app.Register(alert.NewModule(e))
type Module struct {
	engine *Engine
}

// NewModule creates a Module that manages the given Engine's lifecycle.
// The engine is started in an OnStart hook (receiving the app's context) and
// stopped in an OnStop hook.
func NewModule(e *Engine) *Module {
	return &Module{engine: e}
}

// Name implements astra.Module.
func (m *Module) Name() string { return "alert" }

// Install implements astra.Module.
func (m *Module) Install(app *astra.App) error {
	app.OnStart(func(ctx context.Context) error {
		m.engine.Start(ctx)
		return nil
	})
	app.OnStop(func(_ context.Context) error {
		m.engine.Stop()
		return nil
	})
	return nil
}

// Engine returns the underlying Engine so callers can inspect active alerts
// or add rules after installation.
func (m *Module) Engine() *Engine { return m.engine }

// Ensure *Module satisfies astra.Module at compile time.
var _ astra.Module = (*Module)(nil)
