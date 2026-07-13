package alert

import (
	"context"

	"github.com/astra-go/astra"
)

// Component wraps an Engine as an astra.Component, binding the engine's Start and
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
//	app.Register(alert.NewComponent(e))
type Component struct {
	engine *Engine
}

// NewComponent creates a Component that manages the given Engine's lifecycle.
// The engine is started in an OnStart hook (receiving the app's context) and
// stopped in an OnStop hook.
func NewComponent(e *Engine) *Component {
	return &Component{engine: e}
}

// Name implements astra.Component.
func (m *Component) Name() string { return "alert" }

// Init implements astra.Component.
func (m *Component) Init(app *astra.App) error {
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
func (m *Component) Engine() *Engine { return m.engine }

// Ensure *Component satisfies astra.Component at compile time.
var _ astra.Component = (*Component)(nil)
