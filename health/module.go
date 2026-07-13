package health

import "github.com/astra-go/astra"

// NewComponent returns an astra.Component that registers the standard Kubernetes
// health-check endpoints (/live, /ready, /health) when installed on an *App.
//
//	app.Register(
//	    health.NewComponent(
//	        health.WithProbe("db", func(ctx context.Context) error {
//	            return db.PingContext(ctx)
//	        }),
//	        health.WithPrefix("/internal"),
//	    ),
//	)
func NewComponent(opts ...Option) astra.Component {
	return astra.NewComponentFunc("health", func(app *astra.App) error {
		Register(app, opts...)
		return nil
	})
}

// NewIstioComponent returns an astra.Component that registers both the standard
// Kubernetes probe paths (/live, /ready, /health) AND the Istio-compatible
// paths (/healthz/live, /healthz/ready) on the same App.
//
// Use this instead of NewComponent when deploying behind an Istio sidecar.
//
//	app.Register(
//	    health.NewIstioComponent(
//	        health.WithProbe("db", dbProbe),
//	        health.WithIstioHeaders(),
//	    ),
//	)
func NewIstioComponent(opts ...Option) astra.Component {
	return astra.NewComponentFunc("health.istio", func(app *astra.App) error {
		Register(app, opts...)
		RegisterIstioProbes(app, opts...)
		return nil
	})
}
